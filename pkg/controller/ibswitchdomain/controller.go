// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ibswitchdomain

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	corev1 "k8s.io/api/core/v1"
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

const (
	resyncInterval = 5 * time.Minute
	pendingRetry   = 30 * time.Second
)

type reconciler struct {
	client   client.Client
	labelKey func(tier int) (string, error)
	recorder record.EventRecorder
	log      *slog.Logger
}

// NewIBSwitchDomainController labels agent-discovered Switches with the
// unifabric.io/domain value that Topograph wrote on the Nodes below them. It is
// a no-op unless scaleOutDiscovery.ibSwitchDomain.enabled is set.
func NewIBSwitchDomainController(mgr manager.Manager, cfg *config.ControllerConfig, log *slog.Logger) error {
	if cfg == nil || !cfg.ScaleOutDiscovery.IBSwitchDomain.Enabled {
		return nil
	}
	if err := config.EnsureTopologyLabelTemplates(cfg); err != nil {
		return err
	}
	template := cfg.TopologyLabelTemplates.ScaleOut

	r := &reconciler{
		client:   mgr.GetClient(),
		labelKey: template.Render,
		recorder: mgr.GetEventRecorderFor("IBSwitchDomain"), //nolint:staticcheck
		log:      log.With("controller", "IBSwitchDomain"),
	}
	mapCluster := handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "cluster"}}}
	})

	return builder.ControllerManagedBy(mgr).
		Named("IBSwitchDomain").
		Watches(&v1beta1.Switch{}, mapCluster, builder.WithPredicates(switchPredicate())).
		Watches(&corev1.Node{}, mapCluster, builder.WithPredicates(nodeLabelPredicate())).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func (r *reconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	var switchList v1beta1.SwitchList
	if err := r.client.List(ctx, &switchList); err != nil {
		return reconcile.Result{}, err
	}
	managed := make([]v1beta1.Switch, 0, len(switchList.Items))
	for _, sw := range switchList.Items {
		if isManaged(&sw) {
			managed = append(managed, sw)
		}
	}
	if len(managed) == 0 {
		return reconcile.Result{RequeueAfter: resyncInterval}, nil
	}

	var nodeList corev1.NodeList
	if err := r.client.List(ctx, &nodeList); err != nil {
		return reconcile.Result{}, err
	}
	nodeLabels := make(map[string]map[string]string, len(nodeList.Items))
	for _, node := range nodeList.Items {
		nodeLabels[node.Name] = node.Labels
	}

	results := computeDomains(managed, nodeLabels, r.labelKey)
	retry := false
	for i := range managed {
		sw := &managed[i]
		result := results[sw.Name]
		switch result.state {
		case stateAssigned:
			if err := r.applyDomain(ctx, sw, result.domain); err != nil {
				return reconcile.Result{}, err
			}
		case statePending:
			retry = true
			r.log.Info("Switch domain is pending", "switch", sw.Name, "reason", result.message)
			r.recorder.Event(sw, corev1.EventTypeNormal, "DomainPending", result.message)
		case stateConflict:
			r.log.Warn("Switch domain is ambiguous, label left unchanged", "switch", sw.Name, "reason", result.message)
			r.recorder.Event(sw, corev1.EventTypeWarning, "DomainConflict", result.message)
		}
	}

	if retry {
		return reconcile.Result{RequeueAfter: pendingRetry}, nil
	}
	return reconcile.Result{RequeueAfter: resyncInterval}, nil
}

func (r *reconciler) applyDomain(ctx context.Context, sw *v1beta1.Switch, domain string) error {
	if sw.Labels[v1beta1.TopologyDomainLabel] == domain {
		return nil
	}
	patch := client.MergeFrom(sw.DeepCopy())
	if sw.Labels == nil {
		sw.Labels = map[string]string{}
	}
	sw.Labels[v1beta1.TopologyDomainLabel] = domain
	if err := r.client.Patch(ctx, sw, patch); err != nil {
		return fmt.Errorf("label Switch %q with domain %q: %w", sw.Name, domain, err)
	}
	r.log.Info("labeled Switch with domain", "switch", sw.Name, "domain", domain)
	return nil
}

func isManaged(sw *v1beta1.Switch) bool {
	if sw.Labels[v1beta1.SwitchDiscoveredByLabel] != v1beta1.SwitchDiscoveredByAgent {
		return false
	}
	return sw.Spec.Role == "" || sw.Spec.Role == v1beta1.SwitchRoleScaleOut
}

func switchPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSwitch, oldOK := e.ObjectOld.(*v1beta1.Switch)
			newSwitch, newOK := e.ObjectNew.(*v1beta1.Switch)
			if !oldOK || !newOK {
				return true
			}
			return !reflect.DeepEqual(oldSwitch.Labels, newSwitch.Labels) ||
				oldSwitch.Spec.Role != newSwitch.Spec.Role ||
				!reflect.DeepEqual(oldSwitch.Status.LLDPNeighbors, newSwitch.Status.LLDPNeighbors)
		},
	}
}

func nodeLabelPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !reflect.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels())
		},
	}
}
