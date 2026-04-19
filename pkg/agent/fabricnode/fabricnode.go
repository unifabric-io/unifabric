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
	"sync"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/utils"
	v1 "k8s.io/api/core/v1"
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
	reconcileInterval    time.Duration
	timeToStartReconcile time.Time
	client               client.Client
	recorder             record.EventRecorder
	Log                  *slog.Logger
	fabricNode           *v1beta1.FabricNode
	mu                   sync.RWMutex
	gpuRdmaPattern       RdmaInterfaceMethod
	storageRdmaPattern   RdmaInterfaceMethod
	reconcileChan        chan struct{}
	storageNode          bool
	defaultRouteProbe    string
}

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

	reconcileInterval, err := time.ParseDuration(cfg.Discovery.ReconcileInterval)
	if err != nil {
		return nil, fmt.Errorf("unexpected discovery.reconcileInterval: %s", cfg.Discovery.ReconcileInterval)
	}

	timeToWaitSyncLLDPToFabricNode, err := time.ParseDuration(cfg.Discovery.StartupDelay)
	if err != nil {
		return nil, fmt.Errorf("unexpected discovery.startupDelay: %s", cfg.Discovery.StartupDelay)
	}

	r := &fabricNodeReconciler{
		nodeName:             cfg.Node.Name,
		reconcileInterval:    reconcileInterval,
		timeToStartReconcile: time.Now().Add(timeToWaitSyncLLDPToFabricNode),
		client:               mgr.GetClient(),
		recorder:             mgr.GetEventRecorderFor("fabric-node"),
		Log:                  log.With("controller", "fabric-node"),
		gpuRdmaPattern:       RdmaInterfaceMethod{},
		storageRdmaPattern:   RdmaInterfaceMethod{},
		reconcileChan:        make(chan struct{}, 1),
		storageNode:          cfg.Node.Role == config.NodeRoleStorage,
		defaultRouteProbe:    cfg.Node.DefaultRouteProbe,
	}

	if cfg.Discovery.GPUInterfaceFilter != "" {
		err := r.gpuRdmaPattern.CheckOrParsePattern(cfg.Discovery.GPUInterfaceFilter)
		if err != nil {
			return nil, err
		}
	}

	if cfg.Discovery.StorageInterfaceFilter != "" {
		err := r.storageRdmaPattern.CheckOrParsePattern(cfg.Discovery.StorageInterfaceFilter)
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
	ticker := time.NewTicker(r.reconcileInterval)
	defer ticker.Stop()

	// 立即执行一次 reconcile
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

// cleanupStorageNodeFabricNode 清理存储节点的 FabricNode 资源
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

// checkAndSetStorageLeader 检查当前节点是否应该成为存储节点的主节点
// 选主逻辑：如果集群中没有其他存储节点的 FabricNode，则当前节点成为主节点
// 只有主节点才能作为客户端参与 RDMA 延时检测
func (r *fabricNodeReconciler) checkAndSetStorageLeader(ctx context.Context) (bool, error) {
	// 列出所有 FabricNode
	fabricNodeList := &v1beta1.FabricNodeList{}
	if err := r.client.List(ctx, fabricNodeList); err != nil {
		return false, fmt.Errorf("failed to list FabricNodes: %w", err)
	}

	// 检查是否存在其他存储节点的 FabricNode
	for _, fn := range fabricNodeList.Items {
		// 跳过当前节点
		if fn.Name == r.nodeName {
			continue
		}
		// 检查是否为存储节点
		if fn.Status.NodeType == v1beta1.NodeTypeStorage {
			// 已存在其他存储节点，当前节点不是主节点
			r.Log.Info("found existing storage node FabricNode", "existingNode", fn.Name)
			return false, nil
		}
	}

	// 没有其他存储节点，当前节点成为主节点
	r.Log.Info("no other storage node found, this node will be the leader")
	return true, nil
}

// IsStorageLeader 检查当前节点是否为存储节点主节点
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

	updateNodeType := r.updateNodeType(&fabricNode.Status)
	updateNodeIP := r.updateNodeIP(&fabricNode.Status)

	r.mu.Lock()
	if r.fabricNode == nil || fabricNode.Status.RdmaHealthy != r.fabricNode.Status.RdmaHealthy {
		if fabricNode.Status.RdmaHealthy {
			r.recorder.Eventf(fabricNode, "Normal", "RdmaHealthy", "Rdma is healthy")
		} else {
			r.recorder.Eventf(fabricNode, "Warning", "RdmaUnhealthy", "Rdma is unhealthy")
		}
	}
	r.mu.Unlock()

	if rdmaPodNeedUpdate || topologyNeedUpdate || updateNodeType || updateNodeIP {
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
	neighborsMap, err := r.getLLDPInfo(r.Log)
	if err != nil {
		r.Log.Error("failed to get lldp info", "error", err)
	}

	links, err := netlink.LinkList()
	if err != nil {
		return false, err
	}

	var gpuLinks []v1beta1.NicInfo
	var storageLinks []v1beta1.NicInfo
	rdmaNeighborHealthy := true

	for _, l := range links {
		if l.Type() != "device" {
			continue
		}

		if utils.IsSriovVfForNetDev(l.Attrs().Name) {
			r.Log.Debug("GetGpuNicInfos: skip sriov vf", "ifname", l.Attrs().Name)
			continue
		}

		if !rdmamap.IsRDmaDeviceForNetdevice(l.Attrs().Name) {
			r.Log.Debug("GetGpuNicInfos: skip non-rdma device", "ifname", l.Attrs().Name)
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

		// Check if this NIC has LLDP neighbor info
		// Only mark as healthy if ALL RDMA NICs have neighbors
		if nic.State == "up" && nic.LLDPNeighbor.Hostname == "" {
			rdmaNeighborHealthy = false
		}

		storageMatched := false
		if r.storageRdmaPattern.Method != "" {
			storageMatched = r.matchInterface(nic, r.storageRdmaPattern, addrs)
			if storageMatched {
				storageLinks = append(storageLinks, nic)
				continue
			}
		}

		if r.gpuRdmaPattern.Method == "" {
			gpuLinks = append(gpuLinks, nic)
			continue
		}

		gpuMatched := r.matchInterface(nic, r.gpuRdmaPattern, addrs)
		if gpuMatched {
			gpuLinks = append(gpuLinks, nic)
		}
	}

	totalNics := len(gpuLinks) + len(storageLinks)
	gpuNics := gpuLinks
	storageNics := storageLinks
	rdmaHealthy := rdmaNeighborHealthy
	if totalNics == 0 {
		status.RdmaHealthy = false
	}

	changed := false

	if status.TotalNics != totalNics {
		changed = true
		status.TotalNics = totalNics
	}
	if status.RdmaHealthy != rdmaHealthy {
		changed = true
		status.RdmaHealthy = rdmaHealthy
	}
	if !reflect.DeepEqual(status.GpuNics, gpuNics) {
		changed = true
		status.GpuNics = gpuNics
	}
	if !reflect.DeepEqual(status.StorageNics, storageNics) {
		changed = true
		status.StorageNics = storageNics
	}
	return changed, nil
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

func (r *fabricNodeReconciler) updateNodeType(status *v1beta1.FabricNodeStatus) bool {
	if r.storageNode {
		if status.NodeType != v1beta1.NodeTypeStorage {
			status.NodeType = v1beta1.NodeTypeStorage
			return true
		}
	} else {
		if status.NodeType != v1beta1.NodeTypeGPU {
			status.NodeType = v1beta1.NodeTypeGPU
			return true
		}
	}
	return false
}

func (r *fabricNodeReconciler) updateNodeIP(status *v1beta1.FabricNodeStatus) bool {
	probe := r.defaultRouteProbe
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
