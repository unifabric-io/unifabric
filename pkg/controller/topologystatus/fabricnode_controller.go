// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologystatus

import (
	"context"
	"log/slog"
	"reflect"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/topologylabel"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// fabricNodeTopologyReconciler keeps FabricNode.Status.Topologies in sync with
// the fixed Topology names (v1beta1.FixedTopologyNames) that a Node's own
// topology labels currently match. The agent always names a FabricNode after
// its Node (pkg/agent/fabricnode), so both are addressed by the same name.
//
// It is the only writer of status.topologies and watches FabricNode Create
// events only, never Update: reacting to Update would requeue a reconcile for
// every status patch this controller itself makes.
type fabricNodeTopologyReconciler struct {
	client    client.Client
	templates *topologylabel.Set
	log       *slog.Logger
}

func newFabricNodeTopologyController(mgr manager.Manager, cfg *config.ControllerConfig, log *slog.Logger) error {
	reconciler := &fabricNodeTopologyReconciler{
		client:    mgr.GetClient(),
		templates: cfg.TopologyLabelTemplates,
		log:       log.With("controller", "FabricNodeTopology"),
	}

	return builder.ControllerManagedBy(mgr).
		Named("FabricNodeTopology").
		For(&corev1.Node{}, builder.WithPredicates(topologyLabelPredicate(cfg.TopologyLabelTemplates))).
		Watches(&v1beta1.FabricNode{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(createOnlyPredicate())).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(reconciler)
}

func (r *fabricNodeTopologyReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	var node corev1.Node
	if err := r.client.Get(ctx, request.NamespacedName, &node); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	var fabricNode v1beta1.FabricNode
	if err := r.client.Get(ctx, request.NamespacedName, &fabricNode); err != nil {
		if apierrors.IsNotFound(err) {
			// No agent has registered this Node yet; the Watch below will
			// reconcile once its FabricNode is created.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	desired := matchingTopologies(node.Labels, r.templates)
	if reflect.DeepEqual(fabricNode.Status.Topologies, desired) {
		return reconcile.Result{}, nil
	}

	base := fabricNode.DeepCopy()
	fabricNode.Status.Topologies = desired
	patch := client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})
	if err := r.client.Status().Patch(ctx, &fabricNode, patch); err != nil {
		if apierrors.IsConflict(err) {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}
	r.log.Debug("updated FabricNode topology membership", "fabricNode", fabricNode.Name, "topologies", desired)
	return reconcile.Result{}, nil
}

// matchingTopologies returns the fixed Topology names (in
// v1beta1.FixedTopologyNames order) whose label template matches at least one
// key in labels.
func matchingTopologies(labels map[string]string, templates *topologylabel.Set) []string {
	if templates == nil || len(labels) == 0 {
		return nil
	}
	matches := make([]string, 0, len(v1beta1.FixedTopologyNames))
	for _, name := range v1beta1.FixedTopologyNames {
		template := templates.ForTopology(name)
		for key := range labels {
			if _, ok := template.MatchTier(key); ok {
				matches = append(matches, name)
				break
			}
		}
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

// createOnlyPredicate passes Create events through and filters out every
// Update, Delete, and Generic event.
func createOnlyPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}
