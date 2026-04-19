// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package scaleoutgroup

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/utils"
)

type Reconciler struct {
	sync.Mutex
	reconcileCount int
	cfg            *config.ControllerConfig
	client         client.Client
	recorder       record.EventRecorder
	log            *slog.Logger
}

func NewScaleOutLeafGroupController(
	mgr manager.Manager,
	cfg *config.ControllerConfig,
	logger *slog.Logger,
) error {
	r := &Reconciler{
		cfg:      cfg,
		client:   mgr.GetClient(),
		recorder: mgr.GetEventRecorderFor("ScaleOutLeafGroup"),
		log:      logger.With("controller", "ScaleOutLeafGroup"),
	}

	if cfg.Topology.ScaleOutLeafGroups.NodeLabelKey == "" {
		cfg.Topology.ScaleOutLeafGroups.NodeLabelKey = config.DefaultNodeTopologyLabelKey
	}

	c, err := controller.New("ScaleOutLeafGroup", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch FabricNode resources for creation, deletion, and status updates
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &v1beta1.FabricNode{}, handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, node *v1beta1.FabricNode) []reconcile.Request {
			// Watch for all FabricNode events (creation, updates, deletion)
			// This will trigger reconciliation when nodes are added, removed, or their status changes
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: node.Name,
					},
				},
			}
		})),
	); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1beta1.FabricNode{},
		"metadata.name",
		func(rawObj client.Object) []string {
			node := rawObj.(*v1beta1.FabricNode)
			return []string{node.Name}
		},
	); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.log.Info("reconciling ScaleOutLeafGroup", "request node", req.Name)
	existingGroupList := &v1beta1.ScaleOutLeafGroupList{}
	err := r.client.List(ctx, existingGroupList)
	if err != nil {
		r.log.Error("failed to list ScaleOutLeafGroup resources", "error", err)
		return reconcile.Result{}, err
	}

	if r.reconcileCount == 0 {
		// first reconcile, handle all nodes as a full sync
		fabricNodeList := &v1beta1.FabricNodeList{}
		err := r.client.List(ctx, fabricNodeList)
		if err != nil {
			return reconcile.Result{}, err
		}

		for _, fabricNode := range fabricNodeList.Items {
			// Skip Storage nodes during full sync
			if fabricNode.Status.NodeType == v1beta1.NodeTypeStorage {
				r.log.Debug("skipping Storage node during full sync", "node", fabricNode.Name)
				continue
			}
			_, err := r.syncNode(ctx, &fabricNode, existingGroupList.Items)
			if err != nil {
				r.log.Error("failed to handle node", "error", err, "node", fabricNode.Name)
				return reconcile.Result{}, err
			}
		}
		r.log.Info("full sync completed")
		r.Lock()
		r.reconcileCount++
		r.Unlock()
		return reconcile.Result{}, nil
	}

	// handle single node
	// Get the current FabricNode
	fabricNode := &v1beta1.FabricNode{}
	err = r.client.Get(ctx, req.NamespacedName, fabricNode)
	if err == nil {
		result, err := r.syncNode(ctx, fabricNode, existingGroupList.Items)
		if err != nil {
			r.log.Error("failed to handle node", "error", err, "node", fabricNode.Name)
			return reconcile.Result{}, err
		}
		r.log.Info("reconciled node", "node", fabricNode.Name)
		return result, nil
	}

	if client.IgnoreNotFound(err) == nil {
		// Node was deleted, check if it exists in any group and remove it
		r.log.Info("FabricNode not found (deleted), checking existing groups", "node", req.Name)
		result, err := r.handleDeletedNode(ctx, req.Name, existingGroupList.Items)
		if err != nil {
			r.log.Error("failed to handle deleted node", "error", err, "node", req.Name)
			return reconcile.Result{}, err
		}
		r.log.Info("reconciled deleted node", "node", req.Name)
		return result, nil
	}
	r.log.Error("failed to get FabricNode", "error", err)
	return reconcile.Result{}, err
}

func (r *Reconciler) syncNode(ctx context.Context, fabricNode *v1beta1.FabricNode, existingGroupList []v1beta1.ScaleOutLeafGroup) (reconcile.Result, error) {
	if fabricNode.Status.NodeType == v1beta1.NodeTypeStorage {
		return reconcile.Result{}, nil
	}
	// Step 1: Check if fabricNode is being deleted or has incomplete LLDP info
	if fabricNode.DeletionTimestamp != nil && !fabricNode.DeletionTimestamp.IsZero() {
		r.log.Info("FabricNode is being deleted, removing from groups", "node", fabricNode.Name)
		return r.handleDeletedNode(ctx, fabricNode.Name, existingGroupList)
	}

	// Step 2: Get all neighbors for this node
	neighbors := r.getNodeNeighbors(fabricNode)
	r.log.Debug("neighbors for node", "node", fabricNode.Name, "neighbors", neighbors)
	return reconcile.Result{}, r.handleNodeForGroup(ctx, fabricNode.Name, neighbors, existingGroupList)
}

func (r *Reconciler) handleDeletedNode(ctx context.Context, nodeName string, existingGroupList []v1beta1.ScaleOutLeafGroup) (reconcile.Result, error) {
	// Check if node exists in any group
	for _, g := range existingGroupList {
		if slices.ContainsFunc(g.Status.Nodes, func(node v1beta1.Node) bool {
			return node.Name == nodeName
		}) {
			err := r.updateNodeFromGroup(ctx, nodeName, &g)
			if err != nil {
				return reconcile.Result{}, err
			}
			err = r.updateNodeLabel(ctx, nodeName, "")
			if err != nil {
				return reconcile.Result{}, err
			}
			r.recorder.Eventf(&g, "Normal", "NodeRemoved", "Node %s is deleting, removed from group %s", nodeName, g.Name)
			return reconcile.Result{}, nil
		}
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) updateNodeFromGroup(ctx context.Context, nodeName string, g *v1beta1.ScaleOutLeafGroup) error {
	if g == nil {
		return fmt.Errorf("unexpected nil group")
	}

	// Node belongs to an existing group
	g.Status.Nodes = slices.DeleteFunc(g.Status.Nodes, func(node v1beta1.Node) bool {
		return node.Name == nodeName
	})

	if len(g.Status.Nodes) == 0 {
		r.log.Debug("group has no nodes, deleting group", "group", g.Name)
		if err := r.client.Delete(ctx, g); err != nil {
			r.log.Error("failed to delete group", "error", err, "group", g.Name)
			return err
		}
		r.log.Debug("deleted group", "group", g.Name)
		return nil
	}

	r.updateHealthyNode(g)
	// Update the group status
	r.log.Debug("updating group after removing node", "group", g)
	if err := r.client.Status().Update(ctx, g); err != nil {
		r.log.Error("failed to update group after removing node", "error", err, "group", g.Name)
		return err
	}
	r.log.Info("updated group after removing node", "group", g.Name)
	return nil
}

func (r *Reconciler) addNodeToGroup(ctx context.Context, node v1beta1.Node, g v1beta1.ScaleOutLeafGroup) error {
	// Add node as healthy by default
	g.Status.Nodes = append(g.Status.Nodes, node)

	// Sort nodes by name for consistency
	slices.SortFunc(g.Status.Nodes, func(a, b v1beta1.Node) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	r.updateHealthyNode(&g)
	r.log.Debug("updating group", "group", g)
	if err := r.client.Status().Update(ctx, &g); err != nil {
		r.log.Error("failed to update group", "error", err, "group", g.Name)
		return err
	}

	r.log.Debug("added node to existing group", "node", node.Name, "group", g.Name)
	return nil
}

func (r *Reconciler) removeNodeFromGroup(ctx context.Context, nodeName string, g v1beta1.ScaleOutLeafGroup) error {
	g.Status.Nodes = slices.DeleteFunc(g.Status.Nodes, func(node v1beta1.Node) bool {
		return node.Name == nodeName
	})

	if len(g.Status.Nodes) == 0 {
		r.log.Debug("group has no nodes, deleting group", "group", g.Name)
		if err := r.client.Delete(ctx, &g); err != nil {
			r.log.Error("failed to delete group", "error", err, "group", g.Name)
			return err
		}
		r.log.Debug("deleted group", "group", g.Name)
		r.recorder.Eventf(&g, "Normal", "GroupDeleted", "Group %s has no nodes, deleted", g.Name)
		return nil
	}

	r.updateHealthyNode(&g)
	r.log.Debug("updating group", "group", g)
	if err := r.client.Status().Update(ctx, &g); err != nil {
		r.log.Error("failed to update group", "error", err, "group", g.Name)
		return err
	}

	r.log.Debug("removed node from group", "node", nodeName, "group", g.Name)
	return nil
}

func (r *Reconciler) createNewGroup(ctx context.Context, nodeName string, neighbors []v1beta1.Switch) error {
	switchNames := getSwitchNames(neighbors)
	groupName := utils.HashNodesToShortSHA(r.log, switchNames)
	var newGroup v1beta1.ScaleOutLeafGroup

	// Create the resource first (without status)
	newGroup = v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: groupName,
		},
		Spec: v1beta1.ScaleOutLeafGroupSpec{},
	}

	r.log.Debug("creating new group", "groupName", groupName)
	if err := r.client.Create(ctx, &newGroup); err != nil {
		r.log.Error("failed to create new group", "error", err, "name", groupName)
		return err
	}
	r.log.Info("successfully created group", "groupName", newGroup.ObjectMeta.Name, "groupObject", newGroup)

	// Then update the status
	newGroup.Status = v1beta1.ScaleOutLeafGroupStatus{
		Healthy:      true,
		Nodes:        []v1beta1.Node{{Name: nodeName, Healthy: true}},
		Switches:     neighbors,
		TotalNodes:   1,
		HealthyNodes: 1,
	}

	if err := r.client.Status().Update(ctx, &newGroup); err != nil {
		r.log.Error("failed to update group status", "error", err, "name", groupName)
		return err
	}
	r.log.Info("successfully updated group status", "groupName", newGroup.ObjectMeta.Name, "status", newGroup.Status)

	// Add label to nodes
	if err := r.updateNodeLabel(ctx, nodeName, groupName); err != nil {
		r.log.Error("failed to add label to node", "error", err, "node", nodeName)
		return err
	}
	r.recorder.Eventf(&newGroup, "Normal", "GroupCreated", "Create new group for node %s", nodeName)
	return nil
}

func (r *Reconciler) getNodeNeighbors(fabricNode *v1beta1.FabricNode) []v1beta1.Switch {
	var neighbors []v1beta1.Switch
	for _, nic := range fabricNode.Status.GpuNics {
		if nic.State != "up" || nic.LLDPNeighbor.Hostname == "" {
			continue
		}
		neighbors = append(neighbors, v1beta1.Switch{
			Name:   nic.LLDPNeighbor.Hostname,
			MgmtIP: nic.LLDPNeighbor.MgmtIP,
		})
	}

	// Remove duplicates and sort
	switchMap := make(map[string]string)
	for _, sw := range neighbors {
		if _, ok := switchMap[sw.Name]; ok {
			continue
		}
		switchMap[sw.Name] = sw.MgmtIP
	}
	neighbors = make([]v1beta1.Switch, 0, len(switchMap))
	for name, mgmtIP := range switchMap {
		neighbors = append(neighbors, v1beta1.Switch{
			Name:   name,
			MgmtIP: mgmtIP,
		})
	}
	slices.SortFunc(neighbors, func(a, b v1beta1.Switch) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return neighbors
}

func (r *Reconciler) handleNodeForGroup(ctx context.Context, nodeName string, neighbors []v1beta1.Switch, groups []v1beta1.ScaleOutLeafGroup) error {
	var finalLabelValue, currentNodeGroupName, expectedNodeGroupName string
	groupMapName := make(map[string]v1beta1.ScaleOutLeafGroup, len(groups))
	healthyNode := false
	foundNodeInGroup := false

	for _, g := range groups {
		r.log.Debug("checking group", "group", g.Name, "status", g.Status)

		if len(g.Status.Nodes) == 0 {
			// Group is empty, delete it
			r.log.Debug("group is empty, deleting group", "group", g.Name)
			if err := r.client.Delete(ctx, &g); err != nil {
				r.log.Error("failed to delete group", "error", err, "group", g.Name)
				return err
			}
			continue
		}

		groupMapName[g.Name] = g
		// Check if node is already in this group
		nodeInGroup := slices.ContainsFunc(g.Status.Nodes, func(node v1beta1.Node) bool {
			return node.Name == nodeName
		})

		if nodeInGroup {
			currentNodeGroupName = g.Name
			foundNodeInGroup = true
		}

		// Node is not in this group, check if it should join
		if utils.ContainsSlice(g.Status.Switches, neighbors) {
			// Partial match - add node to group but marked unhealthy
			expectedNodeGroupName = g.Name
		}
	}

	if !foundNodeInGroup && len(neighbors) == 0 {
		return fmt.Errorf("no neighbors found for node %s", nodeName)
	}

	if expectedNodeGroupName == "" {
		// 1. for node new added, node is not in any group, try to create a new group
		if currentNodeGroupName == "" {
			// node is not in any group, create a new group
			if len(neighbors) == 0 {
				return fmt.Errorf("no neighbors found for node %s", nodeName)
			}

			r.log.Debug("node is not in any group, creating new group", "node", nodeName, "neighbors", neighbors)
			return r.createNewGroup(ctx, nodeName, neighbors)
		}

		// 2. current node is in a group, but neighbors not match any group, remove it from group
		r.log.Debug("node neighbors updated, not belongs to this group, removing it from group", "node", nodeName, "group", currentNodeGroupName)
		currentNodeGroup := groupMapName[currentNodeGroupName]
		err := r.removeNodeFromGroup(ctx, nodeName, currentNodeGroup)
		if err != nil {
			return err
		}

		err = r.updateNodeLabel(ctx, nodeName, "")
		if err != nil {
			return err
		}

		r.recorder.Eventf(&currentNodeGroup, "Normal", "RemovedNodeFromGroup", "Node %s doesn't belong to group %s, removing it from group", nodeName, currentNodeGroupName)
		return nil
	}

	expectedNodeGroup := groupMapName[expectedNodeGroupName]
	healthyNode = false
	if slices.Equal(expectedNodeGroup.Status.Switches, neighbors) {
		finalLabelValue = expectedNodeGroupName
		healthyNode = true
	}
	// 3. node matches a group and the node is not in any group, try to add it to the group
	if currentNodeGroupName == "" {
		r.log.Debug("node not belongs to any group, adding it to the group", "node", nodeName, "group", expectedNodeGroupName)
		expectedNodeGroup := groupMapName[expectedNodeGroupName]
		err := r.addNodeToGroup(ctx, v1beta1.Node{Name: nodeName, Healthy: healthyNode}, expectedNodeGroup)
		if err != nil {
			return err
		}

		err = r.updateNodeLabel(ctx, nodeName, finalLabelValue)
		if err != nil {
			return err
		}

		r.recorder.Eventf(&expectedNodeGroup, "Normal", "AddedNodeToGroup", "Node %s added to existing group %s", nodeName, expectedNodeGroupName)
		return nil
	}

	if currentNodeGroupName != expectedNodeGroupName {
		// 4. node's rdma zone maybe changed, remove it from current group, and try to add it to the expected group
		r.log.Debug("node's rdma zone maybe changed, removing it from current group and adding it to expected group", "node", nodeName, "currentGroup", currentNodeGroupName, "expectedGroup", expectedNodeGroupName)
		currentNodeGroup := groupMapName[currentNodeGroupName]
		err := r.removeNodeFromGroup(ctx, nodeName, currentNodeGroup)
		if err != nil {
			return err
		}
		err = r.addNodeToGroup(ctx, v1beta1.Node{Name: nodeName, Healthy: healthyNode}, groupMapName[expectedNodeGroupName])
		if err != nil {
			return err
		}

		err = r.updateNodeLabel(ctx, nodeName, finalLabelValue)
		if err != nil {
			return err
		}

		r.recorder.Eventf(&currentNodeGroup, "Normal", "NodeRdmaZoneChanged", "Node %s moved from group %s to group %s", nodeName, currentNodeGroupName, expectedNodeGroupName)
		return nil
	}

	// 5. the node is still in the group
	r.log.Debug("node is still in the group, checking node health", "node", nodeName, "group", expectedNodeGroupName, "healthy", healthyNode)
	groupHealthy := healthyNode
	for idx, node := range expectedNodeGroup.Status.Nodes {
		if node.Name == nodeName {
			if node.Healthy != healthyNode {
				expectedNodeGroup.Status.Nodes[idx].Healthy = healthyNode
			}
		}
		if node.Healthy == false {
			groupHealthy = false
		}
	}

	expectedNodeGroup.Status.Healthy = groupHealthy
	expectedNodeGroup.Status.TotalNodes = len(expectedNodeGroup.Status.Nodes)
	expectedNodeGroup.Status.HealthyNodes = r.healthyNodeNum(expectedNodeGroup)

	r.log.Debug("updating group", "group", expectedNodeGroup)
	if err := r.client.Status().Update(ctx, &expectedNodeGroup); err != nil {
		return err
	}

	err := r.updateNodeLabel(ctx, nodeName, finalLabelValue)
	if err != nil {
		return err
	}

	r.recorder.Eventf(&expectedNodeGroup, "Normal", "NodeRdmaNeighborUpdated", "Node %s still in group %s, group healthy is %v", nodeName, currentNodeGroupName, groupHealthy)
	return nil
}

// updateNodeLabel adds or removes the configured leaf topology label on a Node.
func (r *Reconciler) updateNodeLabel(ctx context.Context, nodeName, groupName string) error {
	node := &corev1.Node{}
	err := r.client.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		return err
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	finalLabelValue := node.Labels[r.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey]
	switch groupName {
	case "":
		r.log.Debug("removing group label from node", "node", nodeName, "label", r.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey)
		delete(node.Labels, r.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey)
	case finalLabelValue:
		r.log.Debug("node topology label has no change, skipping", "node", nodeName, "label", r.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey, "value", groupName)
		return nil
	default:
		r.log.Debug("updating group label to node", "node", nodeName, "oldGroup", finalLabelValue, "newGroup", groupName)
		node.Labels[r.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey] = groupName
	}

	if err := r.client.Update(ctx, node); err != nil {
		return err
	}

	r.log.Info("updated node label", "node", nodeName, "label", r.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey, "value", groupName)
	return nil
}

func (r *Reconciler) healthyNodeNum(g v1beta1.ScaleOutLeafGroup) int {
	healthyNodeNum := 0
	for _, node := range g.Status.Nodes {
		if node.Healthy {
			healthyNodeNum++
		}
	}
	return healthyNodeNum
}

func (r *Reconciler) updateHealthyNode(g *v1beta1.ScaleOutLeafGroup) {
	num := r.healthyNodeNum(*g)
	g.Status.Healthy = num == len(g.Status.Nodes)
	g.Status.HealthyNodes = num
	g.Status.TotalNodes = len(g.Status.Nodes)
}

func getSwitchNames(switches []v1beta1.Switch) []string {
	var names []string
	for _, sw := range switches {
		names = append(names, sw.Name)
	}
	return names
}
