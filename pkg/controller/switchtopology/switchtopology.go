// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const topologyResyncInterval = 5 * time.Minute

type Reconciler struct {
	client client.Client
	cfg    *config.ControllerConfig
	log    *slog.Logger
}

func NewSwitchTopologyDiscoveryController(mgr manager.Manager, cfg *config.ControllerConfig, logger *slog.Logger) error {
	if cfg == nil {
		return fmt.Errorf("switch topology controller config must not be nil")
	}
	if logger == nil {
		return fmt.Errorf("switch topology controller logger must not be nil")
	}

	reconciler := &Reconciler{
		client: mgr.GetClient(),
		cfg:    cfg,
		log:    logger.With("controller", "SwitchTopologyDiscovery"),
	}

	if err := addSwitchSubscriptionManager(mgr, cfg, reconciler.log); err != nil {
		return err
	}
	if !internalTopologyLabelWriterEnabled(cfg) {
		return nil
	}

	controllerBuilder := builder.ControllerManagedBy(mgr).
		Named("SwitchTopologyDiscovery").
		For(&v1beta1.FabricNode{}).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "cluster"}}}
			}),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return hasAnyNodeTopologyLabel(obj.GetLabels(), cfg.TopologyLabels)
			})),
		)
	if switchDiscoveryEnabled(cfg) {
		controllerBuilder = controllerBuilder.Watches(
			&v1beta1.Switch{},
			handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "cluster"}}}
			}),
		)
	}

	return controllerBuilder.
		WithOptions(controller.Options{}).
		Complete(reconciler)
}

func addSwitchSubscriptionManager(mgr manager.Manager, cfg *config.ControllerConfig, log *slog.Logger) error {
	if !switchDiscoveryEnabled(cfg) {
		return nil
	}

	subscriptionManager, err := newSubscriptionManager(mgr.GetClient(), cfg, log)
	if err != nil {
		return err
	}
	return mgr.Add(subscriptionManager)
}

func switchDiscoveryEnabled(cfg *config.ControllerConfig) bool {
	return cfg.ScaleOutDiscovery.Switches.Enabled
}

func internalTopologyLabelWriterEnabled(cfg *config.ControllerConfig) bool {
	return cfg.InternalTopologyLabelWriter.Enabled == nil || *cfg.InternalTopologyLabelWriter.Enabled
}

func (r *Reconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	if !internalTopologyLabelWriterEnabled(r.cfg) {
		return reconcile.Result{}, nil
	}

	switches := []v1beta1.Switch(nil)
	managedSwitchCount := 0
	if switchDiscoveryEnabled(r.cfg) {
		var switchList v1beta1.SwitchList
		if err := r.client.List(ctx, &switchList); err != nil {
			return reconcile.Result{}, err
		}
		switches = switchList.Items
		managedSwitchCount = countTopologyManagedSwitches(switches)
	}
	setManagedSwitchCount(managedSwitchCount)

	var fabricNodes v1beta1.FabricNodeList
	if err := r.client.List(ctx, &fabricNodes); err != nil {
		return reconcile.Result{}, err
	}

	desiredGroups, desiredNodeLabels, managedNodes := buildDesiredStateWithOptions(r.cfg, fabricNodes.Items, switches, desiredStateOptions{
		allowNodeOnlyScaleOutLeaf: !switchDiscoveryEnabled(r.cfg),
	})
	labeledNodes, err := r.nodeNamesWithTopologyLabels(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	managedNodes = mergeNodeNames(managedNodes, labeledNodes)
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
