// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologystatus

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/topologylabel"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const topologyStatusDebounce = 250 * time.Millisecond

type Reconciler struct {
	client    client.Client
	templates *topologylabel.Set
	recorder  record.EventRecorder
	log       *slog.Logger
	debounce  time.Duration
	notBefore map[string]time.Time
}

func NewTopologyStatusController(mgr manager.Manager, cfg *config.ControllerConfig, logger *slog.Logger) error {
	if cfg == nil {
		return fmt.Errorf("topology status controller config must not be nil")
	}
	if logger == nil {
		return fmt.Errorf("topology status controller logger must not be nil")
	}
	if err := config.EnsureTopologyLabelTemplates(cfg); err != nil {
		return err
	}

	reconciler := &Reconciler{
		client:    mgr.GetClient(),
		templates: cfg.TopologyLabelTemplates,
		recorder:  mgr.GetEventRecorderFor("TopologyStatus"), //nolint:staticcheck
		log:       logger.With("controller", "TopologyStatus"),
		debounce:  topologyStatusDebounce,
		notBefore: map[string]time.Time{},
	}
	labelPredicate := topologyLabelPredicate(cfg.TopologyLabelTemplates)
	mapAll := handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
		return fixedTopologyRequests()
	})

	return builder.ControllerManagedBy(mgr).
		Named("TopologyStatus").
		For(&v1beta1.Topology{}, builder.WithPredicates(fixedTopologyPredicate())).
		Watches(&corev1.Node{}, mapAll, builder.WithPredicates(labelPredicate)).
		Watches(&v1beta1.Switch{}, mapAll, builder.WithPredicates(labelPredicate)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(reconciler)
}

func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	name := request.Name
	if !v1beta1.IsFixedTopologyName(name) {
		return reconcile.Result{}, nil
	}
	if delay := r.debounceDelay(name, time.Now()); delay > 0 {
		return reconcile.Result{RequeueAfter: delay}, nil
	}
	topologyStatusReconcileTotal.WithLabelValues(name).Inc()

	var object v1beta1.Topology
	objectExists := true
	if err := r.client.Get(ctx, types.NamespacedName{Name: name}, &object); err != nil {
		if !apierrors.IsNotFound(err) {
			topologyStatusErrorTotal.WithLabelValues(name, "read").Inc()
			topologyStatusStale.WithLabelValues(name).Set(1)
			return reconcile.Result{}, err
		}
		objectExists = false
	} else if !object.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	var nodeList corev1.NodeList
	if err := r.client.List(ctx, &nodeList); err != nil {
		topologyStatusErrorTotal.WithLabelValues(name, "read").Inc()
		topologyStatusStale.WithLabelValues(name).Set(1)
		return reconcile.Result{}, err
	}
	var switchList v1beta1.SwitchList
	if err := r.client.List(ctx, &switchList); err != nil {
		topologyStatusErrorTotal.WithLabelValues(name, "read").Inc()
		topologyStatusStale.WithLabelValues(name).Set(1)
		return reconcile.Result{}, err
	}

	snapshot := LabelSnapshot{
		Nodes:    make([]LabeledResource, 0, len(nodeList.Items)),
		Switches: make([]LabeledResource, 0, len(switchList.Items)),
	}
	for _, node := range nodeList.Items {
		snapshot.Nodes = append(snapshot.Nodes, LabeledResource{Name: node.Name, Labels: node.Labels})
	}
	for _, sw := range switchList.Items {
		if !switchBelongsToTopology(sw, name) {
			continue
		}
		snapshot.Switches = append(snapshot.Switches, LabeledResource{Name: sw.Name, Labels: sw.Labels})
	}

	result, err := BuildTopologyStatus(r.templates.ForTopology(name), snapshot)
	if err != nil {
		topologyStatusErrorTotal.WithLabelValues(name, "conflict").Inc()
		topologyStatusStale.WithLabelValues(name).Set(1)
		if objectExists {
			r.recorder.Event(&object, corev1.EventTypeWarning, "TopologyLabelConflict", err.Error())
		}
		r.log.Error("topology label snapshot is invalid; preserving previous status", "topology", name, "error", err)
		return reconcile.Result{}, err
	}
	topologyStatusPending.WithLabelValues(name).Set(float64(len(result.Pending)))
	for _, pending := range result.Pending {
		if objectExists {
			r.recorder.Event(&object, corev1.EventTypeWarning, "TopologyMemberPending", pending)
		}
		r.log.Warn("topology Switch member is pending a matching Node domain", "topology", name, "detail", pending)
	}

	if !objectExists {
		if topologyStatusEmpty(result.Status) {
			topologyStatusLastSuccess.WithLabelValues(name).SetToCurrentTime()
			topologyStatusStale.WithLabelValues(name).Set(0)
			r.log.Debug("skipped empty topology resource", "topology", name)
			return reconcile.Result{}, nil
		}

		object = newFixedTopology(name)
		if err := r.client.Create(ctx, &object); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return reconcile.Result{Requeue: true}, nil
			}
			topologyStatusErrorTotal.WithLabelValues(name, "create").Inc()
			topologyStatusStale.WithLabelValues(name).Set(1)
			return reconcile.Result{}, err
		}
		r.log.Debug("created topology resource after topology data became available", "topology", name)
	}

	if reflect.DeepEqual(object.Status, result.Status) {
		topologyStatusLastSuccess.WithLabelValues(name).SetToCurrentTime()
		topologyStatusStale.WithLabelValues(name).Set(0)
		return reconcile.Result{}, nil
	}
	base := object.DeepCopy()
	object.Status = result.Status
	patch := client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})
	if err := r.client.Status().Patch(ctx, &object, patch); err != nil {
		topologyStatusErrorTotal.WithLabelValues(name, "write").Inc()
		topologyStatusStale.WithLabelValues(name).Set(1)
		return reconcile.Result{}, err
	}
	topologyStatusLastSuccess.WithLabelValues(name).SetToCurrentTime()
	topologyStatusStale.WithLabelValues(name).Set(0)
	r.log.Debug("updated topology status", "topology", name, "domains", len(result.Status.Domains), "nodeGroups", len(result.Status.Nodes))
	return reconcile.Result{}, nil
}

func (r *Reconciler) debounceDelay(name string, now time.Time) time.Duration {
	if r.debounce <= 0 {
		return 0
	}
	if r.notBefore == nil {
		r.notBefore = map[string]time.Time{}
	}
	deadline, exists := r.notBefore[name]
	if !exists {
		r.notBefore[name] = now.Add(r.debounce)
		return r.debounce
	}
	if now.Before(deadline) {
		return deadline.Sub(now)
	}
	delete(r.notBefore, name)
	return 0
}

func topologyLabelPredicate(templates *topologylabel.Set) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return hasTopologyLabels(e.Object, templates) },
		DeleteFunc: func(e event.DeleteEvent) bool { return hasTopologyLabels(e.Object, templates) },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSwitch, oldIsSwitch := e.ObjectOld.(*v1beta1.Switch)
			newSwitch, newIsSwitch := e.ObjectNew.(*v1beta1.Switch)
			if oldIsSwitch && newIsSwitch && effectiveTopologySwitchRole(*oldSwitch) != effectiveTopologySwitchRole(*newSwitch) {
				return true
			}
			return !reflect.DeepEqual(filteredTopologyLabels(e.ObjectOld, templates), filteredTopologyLabels(e.ObjectNew, templates))
		},
		GenericFunc: func(e event.GenericEvent) bool { return hasTopologyLabels(e.Object, templates) },
	}
}

func switchBelongsToTopology(sw v1beta1.Switch, topologyName string) bool {
	role := effectiveTopologySwitchRole(sw)
	switch topologyName {
	case v1beta1.TopologyScaleOut:
		return role == v1beta1.SwitchRoleScaleOut
	case v1beta1.TopologyScaleUp:
		return role == v1beta1.SwitchRoleScaleUp
	case v1beta1.TopologyStorage:
		return role == v1beta1.SwitchRoleStorage
	default:
		return false
	}
}

func effectiveTopologySwitchRole(sw v1beta1.Switch) v1beta1.SwitchRole {
	if sw.Spec.Role == "" {
		return v1beta1.SwitchRoleScaleOut
	}
	return sw.Spec.Role
}

func fixedTopologyPredicate() predicate.Predicate {
	fixed := func(object client.Object) bool {
		return object != nil && v1beta1.IsFixedTopologyName(object.GetName())
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return fixed(e.Object) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return fixed(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return fixed(e.Object) },
		UpdateFunc: func(e event.UpdateEvent) bool {
			return fixed(e.ObjectNew) && !reflect.DeepEqual(e.ObjectOld.GetDeletionTimestamp(), e.ObjectNew.GetDeletionTimestamp())
		},
	}
}

func filteredTopologyLabels(object client.Object, templates *topologylabel.Set) map[string]string {
	result := map[string]string{}
	if object == nil || templates == nil {
		return result
	}
	for key, value := range object.GetLabels() {
		if v1beta1.IsTopologyDomainLabel(key) {
			result[key] = value
			continue
		}
		for _, labelTemplate := range templates.All() {
			if _, ok := labelTemplate.MatchTier(key); ok {
				result[key] = value
				break
			}
		}
	}
	return result
}

func hasTopologyLabels(object client.Object, templates *topologylabel.Set) bool {
	return len(filteredTopologyLabels(object, templates)) != 0
}

func fixedTopologyRequests() []reconcile.Request {
	requests := make([]reconcile.Request, 0, len(v1beta1.FixedTopologyNames))
	for _, name := range v1beta1.FixedTopologyNames {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: name}})
	}
	return requests
}

func topologyStatusEmpty(status v1beta1.TopologyStatus) bool {
	return len(status.Domains) == 0 && len(status.Nodes) == 0
}

func newFixedTopology(name string) v1beta1.Topology {
	return v1beta1.Topology{
		TypeMeta: metav1.TypeMeta{APIVersion: v1beta1.GroupVersion.String(), Kind: "Topology"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
