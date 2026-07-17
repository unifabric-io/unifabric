// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/topology"
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

const (
	autoTopologyFinalizer = "unifabric.io/auto-discovered-topology-labels"
	autoTopologyResync    = 5 * time.Minute
)

var autoDomainPattern = regexp.MustCompile(`^tier([1-9][0-9]*)-group([1-9][0-9]*)$`)

type autoLabelReconciler struct {
	client            client.Client
	reader            client.Reader
	managedTopologies []managedTopology
	recorder          record.EventRecorder
	log               *slog.Logger
	mu                sync.Mutex
}

type managedTopology struct {
	name          string
	role          v1beta1.SwitchRole
	labelTemplate *topologylabel.Template
}

type autoLabelPlan struct {
	nodes    map[string]map[string]string
	switches map[string]map[string]string
}

type discoveredGroup struct {
	internalName string
	tier         int
	members      []string
	nodes        []string
	lower        []string
	assignedName string
}

func newAutoDiscoveredTopologyLabelController(mgr manager.Manager, cfg *config.ControllerConfig, log *slog.Logger) error {
	reconciler := &autoLabelReconciler{
		client:            mgr.GetClient(),
		reader:            mgr.GetAPIReader(),
		managedTopologies: configuredManagedTopologies(cfg),
		recorder:          mgr.GetEventRecorderFor("AutoDiscoveredTopologyLabel"), //nolint:staticcheck
		log:               log.With("controller", "AutoDiscoveredTopologyLabel"),
	}
	mapCluster := handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "cluster"}}}
	})

	return builder.ControllerManagedBy(mgr).
		Named("AutoDiscoveredTopologyLabel").
		For(&v1beta1.FabricNode{}, builder.WithPredicates(fabricNodeTopologyInputPredicate())).
		Watches(&v1beta1.Switch{}, mapCluster, builder.WithPredicates(switchTopologyInputPredicate())).
		Watches(&v1beta1.Topology{}, mapCluster, builder.WithPredicates(autoTopologyPredicate())).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(reconciler)
}

func configuredManagedTopologies(cfg *config.ControllerConfig) []managedTopology {
	topologies := make([]managedTopology, 0, 2)
	if cfg.ScaleOutDiscovery.Switches.ManageScaleOut {
		topologies = append(topologies, managedTopology{
			name:          v1beta1.TopologyScaleOut,
			role:          v1beta1.SwitchRoleScaleOut,
			labelTemplate: cfg.TopologyLabelTemplates.ScaleOut,
		})
	}
	if cfg.ScaleOutDiscovery.Switches.ManageStorage {
		topologies = append(topologies, managedTopology{
			name:          v1beta1.TopologyStorage,
			role:          v1beta1.SwitchRoleStorage,
			labelTemplate: cfg.TopologyLabelTemplates.Storage,
		})
	}
	return topologies
}

func (r *autoLabelReconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	autoLabelReconcileTotal.Inc()

	released, err := r.releaseUnmanagedFinalizers(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if released {
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	for _, topology := range r.managedTopologies {
		name := topology.name
		var object v1beta1.Topology
		if err := r.reader.Get(ctx, types.NamespacedName{Name: name}, &object); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return reconcile.Result{}, err
		}
		if !object.DeletionTimestamp.IsZero() {
			if containsString(object.Finalizers, autoTopologyFinalizer) {
				if err := r.resetTopology(ctx, &object, topology.labelTemplate, topology.role); err != nil {
					autoLabelErrorTotal.WithLabelValues(name, "reset").Inc()
					return reconcile.Result{}, err
				}
			}
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}
	}

	var fabricNodes v1beta1.FabricNodeList
	if err := r.reader.List(ctx, &fabricNodes); err != nil {
		return reconcile.Result{}, err
	}
	var switches v1beta1.SwitchList
	if err := r.reader.List(ctx, &switches); err != nil {
		return reconcile.Result{}, err
	}
	setManagedSwitchCount(len(switches.Items))
	var nodes corev1.NodeList
	if err := r.reader.List(ctx, &nodes); err != nil {
		return reconcile.Result{}, err
	}

	plan := newAutoLabelPlan()
	for _, topology := range r.managedTopologies {
		rolePlan, err := buildAutoLabelPlan(topology.role, topology.labelTemplate, fabricNodes.Items, switches.Items, nodes.Items)
		if err != nil {
			autoLabelErrorTotal.WithLabelValues(topology.name, "conflict").Inc()
			r.reportConflict(ctx, topology.name, err)
			return reconcile.Result{}, err
		}
		if err := r.ensureTopologyFinalizer(ctx, topology.name, !rolePlan.empty()); err != nil {
			autoLabelErrorTotal.WithLabelValues(topology.name, "write").Inc()
			return reconcile.Result{}, err
		}
		if err := plan.merge(rolePlan); err != nil {
			autoLabelErrorTotal.WithLabelValues(topology.name, "conflict").Inc()
			r.reportConflict(ctx, topology.name, err)
			return reconcile.Result{}, err
		}
	}

	if err := r.applyPlan(ctx, plan, nodes.Items, switches.Items); err != nil {
		autoLabelErrorTotal.WithLabelValues("all", "write").Inc()
		return reconcile.Result{}, err
	}
	autoLabelLastSuccess.SetToCurrentTime()
	return reconcile.Result{RequeueAfter: autoTopologyResync}, nil
}

func newAutoLabelPlan() autoLabelPlan {
	return autoLabelPlan{nodes: map[string]map[string]string{}, switches: map[string]map[string]string{}}
}

func (p *autoLabelPlan) addNode(name, key, value string) error {
	if p.nodes[name] == nil {
		p.nodes[name] = map[string]string{}
	}
	if existing, ok := p.nodes[name][key]; ok && existing != value {
		return fmt.Errorf("Node %q has conflicting desired values %q and %q for label %q", name, existing, value, key)
	}
	p.nodes[name][key] = value
	return nil
}

func (p *autoLabelPlan) addSwitch(name, key, value string) error {
	if p.switches[name] == nil {
		p.switches[name] = map[string]string{}
	}
	if existing, ok := p.switches[name][key]; ok && existing != value {
		return fmt.Errorf("Switch %q has conflicting desired values %q and %q for label %q", name, existing, value, key)
	}
	p.switches[name][key] = value
	return nil
}

func (p *autoLabelPlan) merge(other autoLabelPlan) error {
	for name, labels := range other.nodes {
		for key, value := range labels {
			if err := p.addNode(name, key, value); err != nil {
				return err
			}
		}
	}
	for name, labels := range other.switches {
		for key, value := range labels {
			if err := p.addSwitch(name, key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p autoLabelPlan) empty() bool {
	return len(p.nodes) == 0 && len(p.switches) == 0
}

func buildAutoLabelPlan(role v1beta1.SwitchRole, labelTemplate *topologylabel.Template, fabricNodes []v1beta1.FabricNode, switches []v1beta1.Switch, nodes []corev1.Node) (autoLabelPlan, error) {
	plan := newAutoLabelPlan()
	maxByTier := map[int]int{}
	for _, node := range nodes {
		if err := scanExistingAutoNames(labelTemplate, node.Labels, maxByTier, "Node", node.Name); err != nil {
			return plan, err
		}
	}
	for _, sw := range switches {
		if effectiveSwitchRole(sw) != role {
			continue
		}
		if err := scanExistingAutoMemberName(sw.Labels, maxByTier, sw.Name); err != nil {
			return plan, err
		}
	}

	nodeNames := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		nodeNames[node.Name] = struct{}{}
	}
	eligibleFabricNodes := make([]v1beta1.FabricNode, 0, len(fabricNodes))
	for _, fabricNode := range fabricNodes {
		if _, exists := nodeNames[fabricNode.Name]; exists {
			eligibleFabricNodes = append(eligibleFabricNodes, fabricNode)
		}
	}
	inputs, err := buildTopologyInputsForRole(role, eligibleFabricNodes, switches)
	if err != nil {
		return plan, err
	}
	if len(inputs.hosts) == 0 || len(inputs.switches) == 0 {
		return plan, nil
	}

	discovered := topology.DiscoverTopology(inputs.hosts, inputs.switches, topology.WithHashGroupName(), topology.WithHashLength(40))
	groups, err := normalizeDiscoveredGroups(discovered, inputs.hosts)
	if err != nil {
		return plan, err
	}

	nodesByName := make(map[string]corev1.Node, len(nodes))
	for _, node := range nodes {
		nodesByName[node.Name] = node
	}
	switchesByName := make(map[string]v1beta1.Switch, len(switches))
	for _, sw := range switches {
		switchesByName[sw.Name] = sw
	}

	groupsByTier := map[int][]*discoveredGroup{}
	for index := range groups {
		group := &groups[index]
		groupsByTier[group.tier] = append(groupsByTier[group.tier], group)
	}
	tiers := make([]int, 0, len(groupsByTier))
	for tier := range groupsByTier {
		tiers = append(tiers, tier)
	}
	sort.Ints(tiers)
	assignedOwners := map[string]string{}
	for _, tier := range tiers {
		if inputs.nodeOnly && tier != 1 {
			return plan, fmt.Errorf("node-only topology produced unexpected tier %d", tier)
		}
		tierGroups := groupsByTier[tier]
		sort.Slice(tierGroups, func(i, j int) bool {
			left := strings.Join(tierGroups[i].members, "\x00") + "\x01" + strings.Join(tierGroups[i].lower, "\x00")
			right := strings.Join(tierGroups[j].members, "\x00") + "\x01" + strings.Join(tierGroups[j].lower, "\x00")
			return left < right
		})
		key, err := labelTemplate.Render(tier)
		if err != nil {
			return plan, err
		}
		for _, group := range tierGroups {
			locked := map[string]struct{}{}
			for _, nodeName := range group.nodes {
				node, ok := nodesByName[nodeName]
				if !ok {
					return plan, fmt.Errorf("discovered Node %q does not exist", nodeName)
				}
				if value, exists := node.Labels[key]; exists {
					if _, err := parseAutoDomainName(value, tier); err != nil {
						return plan, fmt.Errorf("Node %q label %q: %w", nodeName, key, err)
					}
					locked[value] = struct{}{}
				}
			}
			for _, switchName := range group.members {
				sw, ok := switchesByName[switchName]
				if !ok {
					if inputs.syntheticSwitches[switchName] {
						continue
					}
					return plan, fmt.Errorf("discovered Switch %q does not exist", switchName)
				}
				if value := sw.Labels[v1beta1.TopologyDomainLabel]; value != "" {
					if _, err := parseAutoDomainName(value, tier); err != nil {
						return plan, fmt.Errorf("Switch %q label %q: %w", switchName, v1beta1.TopologyDomainLabel, err)
					}
					locked[value] = struct{}{}
				}
			}
			if len(locked) > 1 {
				return plan, fmt.Errorf("discovered tier %d group with Switches [%s] has multiple locked names: %s", tier, strings.Join(group.members, ", "), strings.Join(sortedStringSet(locked), ", "))
			}
			if len(locked) == 1 {
				group.assignedName = sortedStringSet(locked)[0]
			} else {
				maxByTier[tier]++
				group.assignedName = fmt.Sprintf("tier%d-group%d", tier, maxByTier[tier])
			}
			owner := fmt.Sprintf("tier%d:%s", tier, group.internalName)
			if existingOwner, exists := assignedOwners[group.assignedName]; exists && existingOwner != owner {
				return plan, fmt.Errorf("locked domain name %q is used by multiple discovered groups", group.assignedName)
			}
			assignedOwners[group.assignedName] = owner

			for _, nodeName := range group.nodes {
				node := nodesByName[nodeName]
				if current, exists := node.Labels[key]; exists {
					if current != group.assignedName {
						return plan, fmt.Errorf("Node %q is locked to %q at tier %d but discovery requires %q", nodeName, current, tier, group.assignedName)
					}
					continue
				}
				if err := plan.addNode(nodeName, key, group.assignedName); err != nil {
					return plan, err
				}
			}
			for _, switchName := range group.members {
				sw, ok := switchesByName[switchName]
				if !ok {
					if inputs.syntheticSwitches[switchName] {
						continue
					}
					return plan, fmt.Errorf("discovered Switch %q does not exist", switchName)
				}
				if current := sw.Labels[v1beta1.TopologyDomainLabel]; current != "" {
					if current != group.assignedName {
						return plan, fmt.Errorf("Switch %q domain is locked to %q but discovery requires %q", switchName, current, group.assignedName)
					}
					continue
				}
				if err := plan.addSwitch(switchName, v1beta1.TopologyDomainLabel, group.assignedName); err != nil {
					return plan, err
				}
			}
		}
	}

	return plan, nil
}

func normalizeDiscoveredGroups(discovered topology.Topology, hosts []topology.Host) ([]discoveredGroup, error) {
	groupsByName := map[string]topology.TopologyGroup{}
	deviceToGroup := map[string]string{}
	for tier, groups := range discovered.GroupsByTier {
		if tier < 1 {
			continue
		}
		for _, group := range groups {
			groupsByName[group.Name] = group
			for _, member := range group.Members {
				deviceToGroup[member] = group.Name
			}
		}
	}
	nodesByGroup := map[string]map[string]struct{}{}
	for _, host := range hosts {
		current := host.Name
		expectedTier := 1
		seen := map[string]struct{}{}
		for {
			parents := discovered.ParentIndex[current]
			if len(parents) == 0 {
				break
			}
			if len(parents) != 1 {
				return nil, fmt.Errorf("resource %q has multiple discovered parents: %s", current, strings.Join(parents, ", "))
			}
			parent := parents[0]
			if _, exists := seen[parent]; exists {
				return nil, fmt.Errorf("discovered topology contains a cycle through %q", parent)
			}
			seen[parent] = struct{}{}
			group, ok := groupsByName[parent]
			if !ok {
				return nil, fmt.Errorf("parent %q of %q is not a topology group", parent, current)
			}
			if group.Tier != expectedTier {
				return nil, fmt.Errorf("Node %q path is discontinuous: expected tier %d, found tier %d", host.Name, expectedTier, group.Tier)
			}
			if nodesByGroup[parent] == nil {
				nodesByGroup[parent] = map[string]struct{}{}
			}
			nodesByGroup[parent][host.Name] = struct{}{}
			current = parent
			expectedTier++
		}
	}

	result := []discoveredGroup{}
	for tier, groups := range discovered.GroupsByTier {
		if tier < 1 {
			continue
		}
		for _, group := range groups {
			lower := []string{}
			if tier == 1 {
				lower = append(lower, group.LowerTierNodes...)
			} else {
				lowerSet := map[string]struct{}{}
				for _, device := range group.LowerTierNodes {
					if lowerGroup := deviceToGroup[device]; lowerGroup != "" {
						lowerSet[lowerGroup] = struct{}{}
					}
				}
				lower = sortedStringSet(lowerSet)
			}
			members := append([]string(nil), group.Members...)
			sort.Strings(members)
			sort.Strings(lower)
			result = append(result, discoveredGroup{
				internalName: group.Name,
				tier:         tier,
				members:      members,
				nodes:        sortedStringSet(nodesByGroup[group.Name]),
				lower:        lower,
			})
		}
	}
	return result, nil
}

func scanExistingAutoNames(labelTemplate *topologylabel.Template, labels map[string]string, maxByTier map[int]int, kind, name string) error {
	for key, value := range labels {
		tier, ok := labelTemplate.MatchTier(key)
		if !ok {
			continue
		}
		group, err := parseAutoDomainName(value, tier)
		if err != nil {
			return fmt.Errorf("%s %q label %q: %w", kind, name, key, err)
		}
		if group > maxByTier[tier] {
			maxByTier[tier] = group
		}
	}
	return nil
}

func scanExistingAutoMemberName(labels map[string]string, maxByTier map[int]int, name string) error {
	value := labels[v1beta1.TopologyDomainLabel]
	if value == "" {
		return nil
	}
	tier, group, err := parseAutoDomainNameParts(value)
	if err != nil {
		return fmt.Errorf("Switch %q label %q: %w", name, v1beta1.TopologyDomainLabel, err)
	}
	if group > maxByTier[tier] {
		maxByTier[tier] = group
	}
	return nil
}

func parseAutoDomainName(value string, expectedTier int) (int, error) {
	tier, group, err := parseAutoDomainNameParts(value)
	if err != nil {
		return 0, err
	}
	if tier != expectedTier {
		return 0, fmt.Errorf("value %q encodes tier %d, expected tier %d", value, tier, expectedTier)
	}
	return group, nil
}

func parseAutoDomainNameParts(value string) (int, int, error) {
	match := autoDomainPattern.FindStringSubmatch(value)
	if len(match) != 3 {
		return 0, 0, fmt.Errorf("value %q is not in tierN-groupM format", value)
	}
	tier, err := strconv.Atoi(match[1])
	if err != nil || tier < 1 {
		return 0, 0, fmt.Errorf("value %q has an invalid tier", value)
	}
	group, err := strconv.Atoi(match[2])
	if err != nil || group < 1 {
		return 0, 0, fmt.Errorf("value %q has an invalid group number", value)
	}
	return tier, group, nil
}

func (r *autoLabelReconciler) applyPlan(ctx context.Context, plan autoLabelPlan, nodes []corev1.Node, switches []v1beta1.Switch) error {
	nodesByName := make(map[string]*corev1.Node, len(nodes))
	for index := range nodes {
		nodesByName[nodes[index].Name] = &nodes[index]
	}
	switchesByName := make(map[string]*v1beta1.Switch, len(switches))
	for index := range switches {
		switchesByName[switches[index].Name] = &switches[index]
	}

	for _, name := range sortedNestedMapKeys(plan.nodes) {
		node := nodesByName[name]
		if node == nil {
			return fmt.Errorf("Node %q disappeared before topology labels could be patched", name)
		}
		if err := patchNodeLabels(ctx, r.client, node, plan.nodes[name], false, nil); err != nil {
			return err
		}
	}
	for _, name := range sortedNestedMapKeys(plan.switches) {
		sw := switchesByName[name]
		if sw == nil {
			return fmt.Errorf("Switch %q disappeared before topology labels could be patched", name)
		}
		if err := patchSwitchLabels(ctx, r.client, sw, plan.switches[name], false); err != nil {
			return err
		}
	}
	return nil
}

func (r *autoLabelReconciler) ensureTopologyFinalizer(ctx context.Context, name string, createIfMissing bool) error {
	var object v1beta1.Topology
	if err := r.reader.Get(ctx, types.NamespacedName{Name: name}, &object); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if !createIfMissing {
			return nil
		}
		return r.client.Create(ctx, &v1beta1.Topology{ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Finalizers: []string{autoTopologyFinalizer},
		}})
	}
	if !object.DeletionTimestamp.IsZero() || containsString(object.Finalizers, autoTopologyFinalizer) {
		return nil
	}
	base := object.DeepCopy()
	object.Finalizers = append(object.Finalizers, autoTopologyFinalizer)
	sort.Strings(object.Finalizers)
	if err := r.client.Patch(ctx, &object, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return err
	}
	return nil
}

func (r *autoLabelReconciler) releaseUnmanagedFinalizers(ctx context.Context) (bool, error) {
	managedNames := make(map[string]struct{}, len(r.managedTopologies))
	for _, topology := range r.managedTopologies {
		managedNames[topology.name] = struct{}{}
	}

	for _, name := range []string{v1beta1.TopologyScaleOut, v1beta1.TopologyStorage} {
		if _, managed := managedNames[name]; managed {
			continue
		}
		var object v1beta1.Topology
		if err := r.reader.Get(ctx, types.NamespacedName{Name: name}, &object); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, err
		}
		if !containsString(object.Finalizers, autoTopologyFinalizer) {
			continue
		}
		base := object.DeepCopy()
		object.Finalizers = removeString(object.Finalizers, autoTopologyFinalizer)
		if err := r.client.Patch(ctx, &object, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
			return false, err
		}
		r.log.Info("released topology label ownership after discovery mode changed", "topology", name)
		return true, nil
	}
	return false, nil
}

func (r *autoLabelReconciler) resetTopology(ctx context.Context, object *v1beta1.Topology, labelTemplate *topologylabel.Template, role v1beta1.SwitchRole) error {
	var nodes corev1.NodeList
	if err := r.reader.List(ctx, &nodes); err != nil {
		return err
	}
	var switches v1beta1.SwitchList
	if err := r.reader.List(ctx, &switches); err != nil {
		return err
	}
	for index := range nodes.Items {
		if err := patchNodeLabels(ctx, r.client, &nodes.Items[index], nil, true, labelTemplate); err != nil {
			return err
		}
	}
	for index := range switches.Items {
		if effectiveSwitchRole(switches.Items[index]) != role {
			continue
		}
		if err := patchSwitchLabels(ctx, r.client, &switches.Items[index], nil, true); err != nil {
			return err
		}
	}

	base := object.DeepCopy()
	object.Finalizers = removeString(object.Finalizers, autoTopologyFinalizer)
	if err := r.client.Patch(ctx, object, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return err
	}
	r.log.Info("cleared auto-discovered topology labels after Topology deletion", "topology", object.Name)
	return nil
}

func patchNodeLabels(ctx context.Context, writer client.Client, object *corev1.Node, desired map[string]string, removeMatching bool, labelTemplate *topologylabel.Template) error {
	base := object.DeepCopy()
	var removeLabel func(string) bool
	if removeMatching {
		removeLabel = func(key string) bool {
			_, ok := labelTemplate.MatchTier(key)
			return ok
		}
	}
	labels, changed, err := reconcileLabelMap(object.Labels, desired, removeLabel)
	if err != nil {
		return fmt.Errorf("Node %q: %w", object.Name, err)
	}
	if !changed {
		return nil
	}
	object.Labels = labels
	return writer.Patch(ctx, object, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
}

func patchSwitchLabels(ctx context.Context, writer client.Client, object *v1beta1.Switch, desired map[string]string, removeMatching bool) error {
	base := object.DeepCopy()
	var removeLabel func(string) bool
	if removeMatching {
		removeLabel = func(key string) bool { return key == v1beta1.TopologyDomainLabel }
	}
	labels, changed, err := reconcileLabelMap(object.Labels, desired, removeLabel)
	if err != nil {
		return fmt.Errorf("Switch %q: %w", object.Name, err)
	}
	if !changed {
		return nil
	}
	object.Labels = labels
	return writer.Patch(ctx, object, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
}

func reconcileLabelMap(existing map[string]string, desired map[string]string, removeLabel func(string) bool) (map[string]string, bool, error) {
	result := make(map[string]string, len(existing)+len(desired))
	for key, value := range existing {
		result[key] = value
	}
	changed := false
	if removeLabel != nil {
		for key := range result {
			if removeLabel(key) {
				delete(result, key)
				changed = true
			}
		}
	}
	for key, value := range desired {
		if current, exists := result[key]; exists && current != "" {
			if current != value {
				return nil, false, fmt.Errorf("label %q is locked to %q, refusing to replace it with %q", key, current, value)
			}
			continue
		}
		result[key] = value
		changed = true
	}
	if len(result) == 0 {
		result = nil
	}
	return result, changed, nil
}

func (r *autoLabelReconciler) reportConflict(ctx context.Context, topologyName string, conflict error) {
	var object v1beta1.Topology
	if err := r.client.Get(ctx, types.NamespacedName{Name: topologyName}, &object); err == nil {
		r.recorder.Event(&object, corev1.EventTypeWarning, "AutoDiscoveryConflict", conflict.Error())
	}
	r.log.Error("auto-discovered topology conflicts with locked labels; preserving existing labels", "topology", topologyName, "error", conflict)
}

type nicInputFingerprint struct {
	State    string
	Hostname string
	Port     string
}

type fabricNodeInputFingerprint struct {
	Role     v1beta1.NodeRole
	ScaleOut []nicInputFingerprint
	Storage  []nicInputFingerprint
}

func fabricNodeFingerprint(node *v1beta1.FabricNode) fabricNodeInputFingerprint {
	return fabricNodeInputFingerprint{
		Role:     node.Status.NodeRole,
		ScaleOut: nicFingerprints(node.Status.ScaleOutNics),
		Storage:  nicFingerprints(node.Status.StorageNics),
	}
}

func nicFingerprints(nics []v1beta1.NicInfo) []nicInputFingerprint {
	result := make([]nicInputFingerprint, 0, len(nics))
	for _, nic := range nics {
		result = append(result, nicInputFingerprint{State: nic.State, Hostname: nic.LLDPNeighbor.Hostname, Port: nic.LLDPNeighbor.Port})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Hostname != result[j].Hostname {
			return result[i].Hostname < result[j].Hostname
		}
		if result[i].Port != result[j].Port {
			return result[i].Port < result[j].Port
		}
		return result[i].State < result[j].State
	})
	if len(result) < 2 {
		return result
	}
	normalized := result[:1]
	for index := 1; index < len(result); index++ {
		if result[index] != result[index-1] {
			normalized = append(normalized, result[index])
		}
	}
	return normalized
}

type switchNeighborFingerprint struct {
	Type v1beta1.SwitchLLDPRemoteSystemType
	Name string
}

type switchInputFingerprint struct {
	Role               v1beta1.SwitchRole
	Hostname           string
	HasManualNeighbors bool
	ManualNeighbors    string
	Neighbors          []switchNeighborFingerprint
}

func switchFingerprint(sw *v1beta1.Switch) switchInputFingerprint {
	neighbors := make([]switchNeighborFingerprint, 0, len(sw.Status.LLDPNeighbors))
	for _, neighbor := range sw.Status.LLDPNeighbors {
		neighbors = append(neighbors, switchNeighborFingerprint{Type: neighbor.RemoteSystemType, Name: neighbor.RemoteSystemName})
	}
	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].Name != neighbors[j].Name {
			return neighbors[i].Name < neighbors[j].Name
		}
		return neighbors[i].Type < neighbors[j].Type
	})
	if len(neighbors) > 1 {
		normalized := neighbors[:1]
		for index := 1; index < len(neighbors); index++ {
			if neighbors[index] != neighbors[index-1] {
				normalized = append(normalized, neighbors[index])
			}
		}
		neighbors = normalized
	}
	manualNeighbors, hasManualNeighbors := sw.Annotations[v1beta1.SwitchNeighborsAnnotation]
	return switchInputFingerprint{
		Role:               sw.Spec.Role,
		Hostname:           sw.Status.Hostname,
		HasManualNeighbors: hasManualNeighbors,
		ManualNeighbors:    manualNeighbors,
		Neighbors:          neighbors,
	}
}

func fabricNodeTopologyInputPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, oldOK := e.ObjectOld.(*v1beta1.FabricNode)
			newNode, newOK := e.ObjectNew.(*v1beta1.FabricNode)
			return !oldOK || !newOK || !reflect.DeepEqual(fabricNodeFingerprint(oldNode), fabricNodeFingerprint(newNode))
		},
	}
}

func switchTopologyInputPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSwitch, oldOK := e.ObjectOld.(*v1beta1.Switch)
			newSwitch, newOK := e.ObjectNew.(*v1beta1.Switch)
			return !oldOK || !newOK || !reflect.DeepEqual(switchFingerprint(oldSwitch), switchFingerprint(newSwitch))
		},
	}
}

func autoTopologyPredicate() predicate.Predicate {
	owned := func(object client.Object) bool {
		if object == nil {
			return false
		}
		return object.GetName() == v1beta1.TopologyScaleOut || object.GetName() == v1beta1.TopologyStorage
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return owned(e.Object) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return owned(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return owned(e.Object) },
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !owned(e.ObjectNew) {
				return false
			}
			return !reflect.DeepEqual(e.ObjectOld.GetDeletionTimestamp(), e.ObjectNew.GetDeletionTimestamp()) ||
				!reflect.DeepEqual(e.ObjectOld.GetFinalizers(), e.ObjectNew.GetFinalizers())
		},
	}
}

func sortedStringSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedNestedMapKeys(values map[string]map[string]string) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}
