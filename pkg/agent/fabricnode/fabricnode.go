// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package fabricnode

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Mellanox/rdmamap"
	"github.com/vishvananda/netlink"
)

type Interface interface {
	GetFabricNode() *v1beta1.FabricNode
	IsStorageLeader() bool
}

type fabricNodeReconciler struct {
	nodeName             string
	refreshInterval      time.Duration
	timeToStartReconcile time.Time
	client               client.Client
	recorder             record.EventRecorder
	Log                  *slog.Logger
	fabricNode           *v1beta1.FabricNode
	mu                   sync.RWMutex
	scaleOutRdmaPattern  RdmaInterfaceMethod
	storageRdmaPattern   RdmaInterfaceMethod
	scaleUpRdmaPattern   RdmaInterfaceMethod
	reconcileChan        chan struct{}
	storageNode          bool
	nodeIPProbeAddress   string
}

type topologyNICClass string

const (
	topologyNICClassNone     topologyNICClass = ""
	topologyNICClassScaleOut topologyNICClass = "scaleOut"
	topologyNICClassStorage  topologyNICClass = "storage"
)

func NewFabricNodeController(
	ctx context.Context,
	mgr manager.Manager,
	log *slog.Logger,
	cfg *config.AgentConfig,
) (Interface, error) {
	// Check if lldpcli command exists
	if err := checkLldpcliExists(); err != nil {
		return nil, fmt.Errorf("lldpcli command not found: %w", err)
	}

	refreshInterval, err := time.ParseDuration(cfg.NodeTopologyDiscovery.RefreshInterval)
	if err != nil {
		return nil, fmt.Errorf("unexpected nodeTopologyDiscovery.refreshInterval: %s", cfg.NodeTopologyDiscovery.RefreshInterval)
	}

	initialScanDelay, err := time.ParseDuration(cfg.NodeTopologyDiscovery.InitialScanDelay)
	if err != nil {
		return nil, fmt.Errorf("unexpected nodeTopologyDiscovery.initialScanDelay: %s", cfg.NodeTopologyDiscovery.InitialScanDelay)
	}

	r := &fabricNodeReconciler{
		nodeName:             cfg.Node.Name,
		refreshInterval:      refreshInterval,
		timeToStartReconcile: time.Now().Add(initialScanDelay),
		client:               mgr.GetClient(),
		recorder:             mgr.GetEventRecorderFor("fabric-node"),
		Log:                  log.With("controller", "fabric-node"),
		scaleOutRdmaPattern:  RdmaInterfaceMethod{},
		storageRdmaPattern:   RdmaInterfaceMethod{},
		scaleUpRdmaPattern:   RdmaInterfaceMethod{},
		reconcileChan:        make(chan struct{}, 1),
		storageNode:          cfg.Node.Role == config.AgentNodeRoleStorage,
		nodeIPProbeAddress:   cfg.Node.DefaultRouteProbe,
	}

	if cfg.NodeTopologyDiscovery.ScaleOutInterfaceSelector != "" {
		err := r.scaleOutRdmaPattern.CheckOrParsePattern(cfg.NodeTopologyDiscovery.ScaleOutInterfaceSelector)
		if err != nil {
			return nil, err
		}
	}

	if cfg.NodeTopologyDiscovery.StorageInterfaceSelector != "" {
		err := r.storageRdmaPattern.CheckOrParsePattern(cfg.NodeTopologyDiscovery.StorageInterfaceSelector)
		if err != nil {
			return nil, err
		}
	}

	if cfg.NodeTopologyDiscovery.ScaleUpInterfaceSelector != "" {
		err := r.scaleUpRdmaPattern.CheckOrParsePattern(cfg.NodeTopologyDiscovery.ScaleUpInterfaceSelector)
		if err != nil {
			return nil, err
		}
	}

	err = builder.ControllerManagedBy(mgr).
		For(&v1beta1.FabricNode{}).
		Watches(
			&v1.Pod{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				pod := obj.(*v1.Pod)
				if pod.Spec.NodeName != r.nodeName {
					return nil
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name: r.nodeName,
						},
					},
				}
			}),
		).
		WithOptions(controller.Options{}).
		Complete(r)
	if err != nil {
		return nil, fmt.Errorf("failed to build controller: %w", err)
	}
	// Add field indexing for efficient pod lookups
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&v1.Pod{},
		"spec.nodeName",
		func(rawObj client.Object) []string {
			pod := rawObj.(*v1.Pod)
			if pod.Spec.NodeName != r.nodeName {
				return nil
			}
			return []string{pod.Spec.NodeName}
		},
	); err != nil {
		return nil, fmt.Errorf("failed to setup pod indexer: %w", err)
	}

	go r.reconcileLoop(ctx)

	return r, nil
}

func (r *fabricNodeReconciler) reconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()

	// Run one reconcile immediately.
	r.Log.Info("periodic reconcile triggered")
	if err := r.doReconcile(ctx); err != nil {
		r.Log.Error("reconcile failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			r.Log.Info("reconcile loop stopped")
			if r.storageNode {
				r.cleanupStorageNodeFabricNode()
			}
			return
		case <-ticker.C:
			r.Log.Info("periodic reconcile triggered")
			if err := r.doReconcile(ctx); err != nil {
				r.Log.Error("reconcile failed", "error", err)
			}
		case <-r.reconcileChan:
			r.Log.Info("pod-triggered reconcile")
			if err := r.doReconcile(ctx); err != nil {
				r.Log.Error("reconcile failed", "error", err)
			}
		}
	}
}

// cleanupStorageNodeFabricNode removes the FabricNode resource for a storage node.
func (r *fabricNodeReconciler) cleanupStorageNodeFabricNode() {
	r.Log.Info("cleaning up storage node FabricNode", "nodeName", r.nodeName)

	fabricNode := &v1beta1.FabricNode{}
	err := r.client.Get(context.Background(), types.NamespacedName{Name: r.nodeName}, fabricNode)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			r.Log.Error("failed to get FabricNode for cleanup", "error", err)
		}
		return
	}

	if err := r.client.Delete(context.Background(), fabricNode); err != nil {
		r.Log.Error("failed to delete storage node FabricNode", "error", err)
		return
	}
	r.Log.Info("successfully deleted storage node FabricNode", "nodeName", r.nodeName)
}

// checkAndSetStorageLeader checks whether the current node should be the storage node leader.
// If the cluster has no other storage FabricNode, the current node becomes the leader.
// Only the leader participates as the client in RDMA latency checks.
func (r *fabricNodeReconciler) checkAndSetStorageLeader(ctx context.Context) (bool, error) {
	// List all FabricNodes.
	fabricNodeList := &v1beta1.FabricNodeList{}
	if err := r.client.List(ctx, fabricNodeList); err != nil {
		return false, fmt.Errorf("failed to list FabricNodes: %w", err)
	}

	// Check whether another storage FabricNode already exists.
	for _, fn := range fabricNodeList.Items {
		// Skip the current node.
		if fn.Name == r.nodeName {
			continue
		}
		// Check whether this is a storage node.
		if fn.Status.NodeRole == v1beta1.NodeRoleStorage {
			// Another storage node already exists, so the current node is not the leader.
			r.Log.Info("found existing storage node FabricNode", "existingNode", fn.Name)
			return false, nil
		}
	}

	// No other storage node exists, so the current node becomes the leader.
	r.Log.Info("no other storage node found, this node will be the leader")
	return true, nil
}

// IsStorageLeader reports whether the current node is the storage node leader.
func (r *fabricNodeReconciler) IsStorageLeader() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.fabricNode == nil {
		return false
	}
	if r.fabricNode.Annotations == nil {
		return false
	}
	return r.fabricNode.Annotations[config.StorageNodeLeaderAnnotationKey] == "true"
}

func (r *fabricNodeReconciler) doReconcile(ctx context.Context) error {
	r.Log.Info("reconciling FabricNode", "NodeName", r.nodeName)

	fabricNode := &v1beta1.FabricNode{}
	err := r.client.Get(ctx, types.NamespacedName{Name: r.nodeName}, fabricNode)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			r.Log.Error("failed to get FabricNode", "error", err)
			return err
		}
		fabricNode = &v1beta1.FabricNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: r.nodeName,
			},
		}

		if !r.storageNode {
			node := &v1.Node{}
			err := r.client.Get(ctx, types.NamespacedName{Name: r.nodeName}, node)
			if err != nil {
				if client.IgnoreNotFound(err) != nil {
					r.Log.Error("failed to get Node", "error", err)
					return err
				}
				r.Log.Info("node not found", "name", r.nodeName)
			}

			fabricNode.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Node",
				Name:       r.nodeName,
				UID:        node.UID,
			}}
		}

		if err := r.client.Create(ctx, fabricNode); err != nil {
			r.Log.Error("failed to create FabricNode", "error", err)
			return err
		}
	}

	r.Log.Info("found FabricNode", "name", fabricNode.Name)

	var topologyNeedUpdate bool
	if time.Now().After(r.timeToStartReconcile) {
		topologyNeedUpdate, err = r.updateTopologyStatus(&fabricNode.Status)
		if err != nil {
			r.Log.Error("failed to update topology status", "error", err, "name", r.nodeName)
		}
	}

	var rdmaPodNeedUpdate bool
	if !r.storageNode {
		rdmaPodNeedUpdate, err = r.updateRdmaPodsStatus(ctx, &fabricNode.Status)
		if err != nil {
			r.Log.Error("failed to get rdma pods status", "error", err, "name", r.nodeName)
		}
	}

	updateNodeRole := r.updateNodeRole(&fabricNode.Status)
	updateNodeIP := r.updateNodeIP(&fabricNode.Status)
	updateReady := updateReadyCondition(&fabricNode.Status)

	r.mu.Lock()
	r.emitLLDPNeighborsReadyEventLocked(fabricNode)
	r.mu.Unlock()

	if rdmaPodNeedUpdate || topologyNeedUpdate || updateNodeRole || updateNodeIP || updateReady {
		if err := r.client.Status().Update(ctx, fabricNode); err != nil {
			r.Log.Error("failed to update FabricNode status", "error", err)
			return err
		}
	}

	r.mu.Lock()
	r.fabricNode = fabricNode.DeepCopy()
	r.mu.Unlock()

	r.Log.Info("reconciled fabric node success", "node", r.nodeName)
	return nil
}

func (r *fabricNodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.Log.Info("Reconcile triggered by Pod watch", "NodeName", req.NamespacedName.Name)

	select {
	case r.reconcileChan <- struct{}{}:
		r.Log.Debug("signal sent to reconcile channel")
	default:
		r.Log.Debug("reconcile channel full, signal dropped")
	}

	return reconcile.Result{}, nil
}

func (r *fabricNodeReconciler) handleNodeDeletion(ctx context.Context, nodeName string) (reconcile.Result, error) {
	// Check if FabricNode exists and delete it
	fabricNode := &v1beta1.FabricNode{}
	err := r.client.Get(ctx, types.NamespacedName{Name: nodeName}, fabricNode)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			r.Log.Error("failed to get FabricNode", "error", err)
			return reconcile.Result{}, err
		}
		// FabricNode not found, nothing to do
		return reconcile.Result{}, nil
	}
	// Delete the FabricNode
	if err := r.client.Delete(ctx, fabricNode); err != nil {
		r.Log.Error("failed to delete FabricNode", "error", err)
		return reconcile.Result{}, err
	}
	r.Log.Info("deleted FabricNode", "name", nodeName)
	return reconcile.Result{}, nil
}

func (r *fabricNodeReconciler) GetFabricNode() *v1beta1.FabricNode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.fabricNode != nil {
		return r.fabricNode.DeepCopy()
	}
	return nil
}

func (r *fabricNodeReconciler) updateTopologyStatus(status *v1beta1.FabricNodeStatus) (bool, error) {
	r.Log.Debug("getting the neighbors by lldpcli command")
	neighborsMap, lldpErr := r.getLLDPInfo(r.Log)
	if lldpErr != nil {
		r.Log.Error("failed to get lldp info", "error", lldpErr)
	}

	links, err := netlink.LinkList()
	if err != nil {
		changed := meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:    v1beta1.FabricNodeConditionLLDPNeighborsReady,
			Status:  metav1.ConditionUnknown,
			Reason:  v1beta1.FabricNodeReasonDiscoveryFailed,
			Message: fmt.Sprintf("Failed to list network interfaces: %v", err),
		})
		return changed, err
	}

	var scaleOutLinks []v1beta1.NicInfo
	var storageLinks []v1beta1.NicInfo

	for _, l := range links {
		if l.Type() != "device" {
			continue
		}

		if utils.IsSriovVfForNetDev(l.Attrs().Name) {
			r.Log.Debug("GetScaleOutNicInfos: skip sriov vf", "ifname", l.Attrs().Name)
			continue
		}

		if !rdmamap.IsRDmaDeviceForNetdevice(l.Attrs().Name) {
			r.Log.Debug("GetScaleOutNicInfos: skip non-rdma device", "ifname", l.Attrs().Name)
			continue
		}

		rdmaDevice, err := rdmamap.GetRdmaDeviceForNetdevice(l.Attrs().Name)
		if err != nil {
			r.Log.Error("failed to get rdma device for netdevice", "error", err)
		}

		nic := v1beta1.NicInfo{
			Name:           l.Attrs().Name,
			RDMA:           true,
			State:          l.Attrs().OperState.String(),
			RdmaDeviceName: rdmaDevice,
		}

		// Check if neighborsMap is not nil before accessing it
		if neighborsMap != nil {
			neighbor, ok := neighborsMap[nic.Name]
			if ok {
				nic.LLDPNeighbor = neighbor
			}
		}

		addrs, err := netlink.AddrList(l, netlink.FAMILY_ALL)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if addr.IP.IsLinkLocalUnicast() || addr.IP.IsLinkLocalMulticast() {
				continue
			}
			if addr.IP.To4() != nil {
				networkCIDR := &net.IPNet{IP: addr.IP, Mask: addr.IPNet.Mask}
				nic.IPv4 = networkCIDR.String()
			} else if addr.IP.To16() != nil {
				networkCIDR := &net.IPNet{IP: addr.IP, Mask: addr.IPNet.Mask}
				nic.IPv6 = networkCIDR.String()
			}
		}

		switch r.classifyTopologyNIC(nic, addrs) {
		case topologyNICClassStorage:
			storageLinks = append(storageLinks, nic)
		case topologyNICClassScaleOut:
			scaleOutLinks = append(scaleOutLinks, nic)
		}
	}

	totalNics := len(scaleOutLinks) + len(storageLinks)
	scaleOutNics := scaleOutLinks
	storageNics := storageLinks
	selectedNics := make([]v1beta1.NicInfo, 0, totalNics)
	selectedNics = append(selectedNics, scaleOutLinks...)
	selectedNics = append(selectedNics, storageLinks...)

	changed := setLLDPNeighborsReadyCondition(status, totalNics, selectedNics, lldpErr)

	if status.TotalNics != totalNics {
		changed = true
		status.TotalNics = totalNics
	}
	if !reflect.DeepEqual(status.ScaleOutNics, scaleOutNics) {
		changed = true
		status.ScaleOutNics = scaleOutNics
	}
	if !reflect.DeepEqual(status.StorageNics, storageNics) {
		changed = true
		status.StorageNics = storageNics
	}
	return changed, nil
}

func (r *fabricNodeReconciler) classifyTopologyNIC(nic v1beta1.NicInfo, addrs []netlink.Addr) topologyNICClass {
	if r.storageRdmaPattern.Method != "" && r.matchInterface(nic, r.storageRdmaPattern, addrs) {
		return topologyNICClassStorage
	}
	if r.scaleUpRdmaPattern.Method != "" && r.matchInterface(nic, r.scaleUpRdmaPattern, addrs) {
		return topologyNICClassNone
	}
	if r.scaleOutRdmaPattern.Method == "" || r.matchInterface(nic, r.scaleOutRdmaPattern, addrs) {
		return topologyNICClassScaleOut
	}
	return topologyNICClassNone
}

func setLLDPNeighborsReadyCondition(status *v1beta1.FabricNodeStatus, totalNics int, nics []v1beta1.NicInfo, discoveryErr error) bool {
	if totalNics == 0 {
		return meta.RemoveStatusCondition(&status.Conditions, v1beta1.FabricNodeConditionLLDPNeighborsReady)
	}

	condition := metav1.Condition{
		Type:    v1beta1.FabricNodeConditionLLDPNeighborsReady,
		Status:  metav1.ConditionTrue,
		Reason:  v1beta1.FabricNodeReasonLLDPNeighborsReady,
		Message: "All selected RDMA interfaces have LLDP neighbors",
	}
	if discoveryErr != nil {
		condition.Status = metav1.ConditionUnknown
		condition.Reason = v1beta1.FabricNodeReasonDiscoveryFailed
		condition.Message = fmt.Sprintf("Failed to discover LLDP neighbors: %v", discoveryErr)
		return meta.SetStatusCondition(&status.Conditions, condition)
	}

	var missing []string
	for _, nic := range nics {
		if nic.State == "up" && nic.LLDPNeighbor.Hostname == "" {
			missing = append(missing, nic.Name)
		}
	}
	if len(missing) > 0 {
		condition.Status = metav1.ConditionFalse
		condition.Reason = v1beta1.FabricNodeReasonLLDPNeighborMissing
		condition.Message = fmt.Sprintf("Selected RDMA interfaces are missing LLDP neighbors: %s", strings.Join(missing, ", "))
	}
	return meta.SetStatusCondition(&status.Conditions, condition)
}

func updateReadyCondition(status *v1beta1.FabricNodeStatus) bool {
	readyCondition := metav1.Condition{
		Type:    v1beta1.FabricNodeConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  v1beta1.FabricNodeReasonReady,
		Message: "All FabricNode conditions are ready",
	}

	for _, condition := range status.Conditions {
		if condition.Type == v1beta1.FabricNodeConditionReady {
			continue
		}
		if condition.Status == metav1.ConditionFalse {
			readyCondition.Status = metav1.ConditionFalse
			readyCondition.Reason = v1beta1.FabricNodeReasonConditionNotReady
			readyCondition.Message = fmt.Sprintf("%s is not ready: %s", condition.Type, condition.Message)
			break
		}
		if condition.Status == metav1.ConditionUnknown && readyCondition.Status != metav1.ConditionFalse {
			readyCondition.Status = metav1.ConditionUnknown
			readyCondition.Reason = v1beta1.FabricNodeReasonConditionUnknown
			readyCondition.Message = fmt.Sprintf("%s readiness is unknown: %s", condition.Type, condition.Message)
		}
	}

	return meta.SetStatusCondition(&status.Conditions, readyCondition)
}

func (r *fabricNodeReconciler) emitLLDPNeighborsReadyEventLocked(fabricNode *v1beta1.FabricNode) {
	current := meta.FindStatusCondition(fabricNode.Status.Conditions, v1beta1.FabricNodeConditionLLDPNeighborsReady)
	if current == nil {
		return
	}

	var previous *metav1.Condition
	if r.fabricNode != nil {
		previous = meta.FindStatusCondition(r.fabricNode.Status.Conditions, v1beta1.FabricNodeConditionLLDPNeighborsReady)
	}
	if previous != nil && previous.Status == current.Status && previous.Reason == current.Reason {
		return
	}

	if current.Status == metav1.ConditionTrue {
		if previous == nil || previous.Status != metav1.ConditionTrue {
			r.recorder.Eventf(fabricNode, "Normal", v1beta1.FabricNodeReasonLLDPNeighborsReady, "LLDP neighbors are ready")
		}
		return
	}
	if previous == nil || previous.Status == metav1.ConditionTrue {
		reason := current.Reason
		if reason == v1beta1.FabricNodeReasonDiscoveryFailed {
			reason = "LLDPDiscoveryFailed"
		}
		r.recorder.Eventf(fabricNode, "Warning", reason, "LLDP neighbors are not ready: %s", current.Message)
	}
}

// matchInterface checks if the interface matches the given pattern
func (r *fabricNodeReconciler) matchInterface(nic v1beta1.NicInfo, pattern RdmaInterfaceMethod, addrs []netlink.Addr) bool {
	switch pattern.Method {
	case "interface":
		return matchMultiplePatterns(nic.Name, pattern.Value)
	case "cidr":
		_, cidrNet, err := net.ParseCIDR(pattern.Value)
		if err != nil {
			return false
		}

		// Check if any address matches the CIDR
		for _, addr := range addrs {
			if addr.IP != nil && cidrNet.Contains(addr.IP) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (r *fabricNodeReconciler) getLLDPInfo(logger *slog.Logger) (map[string]v1beta1.LLDPNeighbor, error) {
	return GetLLDPNeighbors(logger)
}

// checkLldpcliExists checks if lldpcli command is available in the system
func checkLldpcliExists() error {
	_, err := exec.LookPath(LLDPCLI)
	if err != nil {
		return fmt.Errorf("lldpcli command not found in PATH, please install lldpd package")
	}
	return nil
}

func (r *fabricNodeReconciler) updateNodeRole(status *v1beta1.FabricNodeStatus) bool {
	if r.storageNode {
		if status.NodeRole != v1beta1.NodeRoleStorage {
			status.NodeRole = v1beta1.NodeRoleStorage
			return true
		}
	} else {
		if status.NodeRole != v1beta1.NodeRoleGPU {
			status.NodeRole = v1beta1.NodeRoleGPU
			return true
		}
	}
	return false
}

func (r *fabricNodeReconciler) updateNodeIP(status *v1beta1.FabricNodeStatus) bool {
	probe := r.nodeIPProbeAddress
	if probe == "" {
		return false
	}
	conn, err := net.DialTimeout("udp", probe, 2*time.Second)
	if err != nil {
		r.Log.Error("failed to detect node IP via UDP dial", "error", err, "probe", probe)
		return false
	}
	defer conn.Close()
	localAddr := conn.LocalAddr()
	udpAddr, ok := localAddr.(*net.UDPAddr)
	if !ok {
		return false
	}
	ip := udpAddr.IP.String()
	if ip == "" || ip == status.NodeIP {
		return false
	}
	status.NodeIP = ip
	r.Log.Info("updated node IP", "node", r.nodeName, "nodeIP", ip)
	return true
}
