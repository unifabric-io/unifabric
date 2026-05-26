// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"context"
	"log/slog"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const topologyResyncInterval = 5 * time.Minute

type Reconciler struct {
	client client.Client
	cfg    *config.ControllerConfig
	log    *slog.Logger
}

func NewSwitchTopologyDiscoveryController(mgr manager.Manager, cfg *config.ControllerConfig, logger *slog.Logger) error {
	reconciler := &Reconciler{
		client: mgr.GetClient(),
		cfg:    cfg,
		log:    logger.With("controller", "SwitchTopologyDiscovery"),
	}

	subscriptionManager, err := newSubscriptionManager(mgr.GetClient(), cfg, reconciler.log)
	if err != nil {
		return err
	}
	if err := mgr.Add(subscriptionManager); err != nil {
		return err
	}

	return builder.ControllerManagedBy(mgr).
		Named("SwitchTopologyDiscovery").
		For(&v1beta1.Switch{}).
		Watches(
			&v1beta1.FabricNode{},
			handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "cluster"}}}
			}),
		).
		WithOptions(controller.Options{}).
		Complete(reconciler)
}

func (r *Reconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	var switches v1beta1.SwitchList
	if err := r.client.List(ctx, &switches); err != nil {
		return reconcile.Result{}, err
	}
	managedSwitchCount := countTopologyManagedSwitches(switches.Items)
	setManagedSwitchCount(managedSwitchCount)

	var fabricNodes v1beta1.FabricNodeList
	if err := r.client.List(ctx, &fabricNodes); err != nil {
		return reconcile.Result{}, err
	}

	desiredGroups, desiredNodeLabels, managedNodes := buildDesiredState(r.cfg, fabricNodes.Items, switches.Items)
	r.log.Debug(
		"computed switch topology summary",
		"managedSwitchCount", managedSwitchCount,
		"fabricNodeCount", len(fabricNodes.Items),
		"topologyGroupCount", len(desiredGroups),
		"managedNodeCount", len(managedNodes),
		"nodeLabelCount", len(desiredNodeLabels),
	)

	if err := r.reconcileNodeLabels(ctx, managedNodes, desiredNodeLabels); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: topologyResyncInterval}, nil
}

func countTopologyManagedSwitches(switches []v1beta1.Switch) int {
	count := 0
	for _, sw := range switches {
		if sw.Status.Healthy {
			count++
		}
	}
	return count
}
