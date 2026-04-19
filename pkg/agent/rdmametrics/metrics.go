// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package rdmametrics

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/safchain/ethtool"
	"golang.org/x/sys/unix"

	"github.com/unifabric-io/unifabric/pkg/agent/fabricnode"
	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

// FabricNode example
// apiVersion: unifabric.io/v1beta1
// kind: FabricNode
// metadata:
//   name: sh-cube-master-1
// spec: {}
// status:
//   rdmaPods:
//   - containerList:
//     - containerd://3304d464fe5eb237956b925a24aa1b6edc22edbd72969356dd6c794be04cd95b
//     name: rdma-test-rdma-tools-h8pkd
//     namespace: rdma
//   - containerList:
//     - containerd://d1204386e83894e780243e43a856ef177ebe55b69ba83bfa7ff0fbc268c807d4
//     name: test-worker-0
//     namespace: yang
//     topOwner:
//       apiVersion: kubeflow.org/v1
//       kind: PyTorchJob
//       name: test
//       namespace: yang
//   - containerList:
//     - containerd://42fb25b4f261d1850ee464749842ad41aa9158bf45fda9d4e99660f506729b16
//     - containerd://bf4326921cd0e80b11a80ea10ddf62c9a21833d4d9ec9d3ed2e616b28d04d116
//     name: test-multi-container-rdma-tools
//     namespace: rdma

type Metrics struct {
	cli    fabricnode.Interface
	logger *slog.Logger
}

func (m *Metrics) Describe(ch chan<- *prometheus.Desc) {

}

// Host RDMA metrics cache structure
// map[device][port][counter]float64
type rdmaCounterValue struct {
	counter      string
	device       string
	ifname       string
	port         string
	parentIfname string
	value        float64
}

type ethtoolCounterValue struct {
	metricName string
	ifname     string
	priority   int
	value      uint64
}

type hostRdmaMetrics struct {
	counters        []rdmaCounterValue
	ethtoolCounters []ethtoolCounterValue
}

func buildPciToIfnameMap() map[string]string {
	m := make(map[string]string)
	netDir := "/sys/class/net"
	entries, _ := os.ReadDir(netDir)
	for _, entry := range entries {
		ifname := entry.Name()
		// skip interfaces
		if ifname == "lo" || strings.HasPrefix(ifname, "cali") {
			continue
		}
		devPath := filepath.Join(netDir, ifname, "device")
		absPci, err := filepath.EvalSymlinks(devPath)
		if err != nil {
			continue
		}
		m[absPci] = ifname
	}
	return m
}

func buildPciToPfIfnameMap() map[string]string {
	m := make(map[string]string)
	netDir := "/sys/class/net"
	entries, _ := os.ReadDir(netDir)
	for _, entry := range entries {
		ifname := entry.Name()
		// skip interfaces
		if ifname == "lo" || strings.HasPrefix(ifname, "cali") {
			continue
		}
		devPath := filepath.Join(netDir, ifname, "device")
		absPci, err := filepath.EvalSymlinks(devPath)
		if err != nil {
			continue
		}
		physfnPath := filepath.Join(devPath, "physfn")
		if _, err := os.Lstat(physfnPath); os.IsNotExist(err) {
			m[absPci] = ifname
		}
	}
	return m
}

func getIfNameForInfinibandDevice(device string, pciToIfname map[string]string) (string, error) {
	devicePath := filepath.Join("/sys/class/infiniband", device, "device")
	absPciPath, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return "", err
	}
	ifname, ok := pciToIfname[absPciPath]
	if !ok {
		return "", fmt.Errorf("no matching netdev for %s", device)
	}
	return ifname, nil
}

func collectHostRdmaMetrics(logger *slog.Logger) (*hostRdmaMetrics, map[string]string, map[string]string) {
	metrics := &hostRdmaMetrics{}
	var pciToIfname map[string]string
	var pciToPfIfname map[string]string

	var rootIfNameList []string

	_ = withMountNamespace(1, func() error {
		pciToIfname = buildPciToIfnameMap()
		pciToPfIfname = buildPciToPfIfnameMap()
		basePath := "/sys/class/infiniband"
		devices, err := os.ReadDir(basePath)
		if err != nil {
			logger.Error("failed to list infiniband base path", "error", err, "basePath", basePath)
			return nil
		}
		for _, dev := range devices {
			ifname, err := getIfNameForInfinibandDevice(dev.Name(), pciToIfname)
			if err != nil {
				logger.Debug("no matching netdev for infiniband device", "rdma_device", dev.Name())
				ifname = ""
			}
			parentIfname := ifname
			if ifname != "" {
				physfnPath := filepath.Join("/sys/class/net", ifname, "device", "physfn")
				pfPciPath, err := filepath.EvalSymlinks(physfnPath)
				if err == nil {
					if pfIfname, ok := pciToPfIfname[pfPciPath]; ok {
						parentIfname = pfIfname
					}
				}
			}
			if parentIfname == ifname {
				rootIfNameList = append(rootIfNameList, ifname)
			}
			portsPath := filepath.Join(basePath, dev.Name(), "ports")
			ports, err := os.ReadDir(portsPath)
			if err != nil {
				logger.Error("failed to list ports", "error", err, "portsPath", portsPath)
				continue
			}
			collectDeviceTosToCache(basePath, dev.Name(), ifname, parentIfname, metrics, logger)
			for _, port := range ports {
				hwCountersPath := filepath.Join(portsPath, port.Name(), "hw_counters")
				collectCountersToCacheWithParentIfname(hwCountersPath, dev.Name(), ifname, port.Name(), parentIfname, metrics, logger)
				countersPath := filepath.Join(portsPath, port.Name(), "counters")
				collectCountersToCacheWithParentIfname(countersPath, dev.Name(), ifname, port.Name(), parentIfname, metrics, logger)
				collectIfInfoToCache(ifname, dev.Name(), port.Name(), ifname, metrics, logger)
			}
		}
		return nil
	})

	err := ns.WithNetNSPath("/host/proc/1/ns/net", func(hostNS ns.NetNS) error {
		collectEthToolToCache(metrics, rootIfNameList, logger)
		return nil
	})
	if err != nil {
		logger.Error("failed to enter netns for ethtool collection", "error", err)
	}

	return metrics, pciToIfname, pciToPfIfname
}

func collectCountersToCacheWithParentIfname(dirPath, device, ifname, port, parentIfname string, metrics *hostRdmaMetrics, logger *slog.Logger) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	for _, f := range files {
		if !f.Type().IsRegular() {
			continue
		}
		name := f.Name()
		valuePath := filepath.Join(dirPath, name)
		valueStr, err := os.ReadFile(valuePath)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) || err.Error() == "operation not supported" {
				continue
			}
			continue
		}
		value := strings.TrimSpace(string(valueStr))
		if strings.Contains(value, "N/A (no PMA)") {
			continue
		}
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		if name == "port_xmit_data" || name == "port_rcv_data" {
			v = v * 4
		}
		metrics.counters = append(metrics.counters, rdmaCounterValue{
			device:       device,
			ifname:       ifname,
			port:         port,
			parentIfname: parentIfname,
			counter:      name,
			value:        v,
		})
	}
}

func collectEthToolCounters(nodeName string, pod *v1beta1.RdmaPod, ch chan<- prometheus.Metric, logger *slog.Logger, ifNameToDevice, pciToIfname, pciToPfIfname map[string]string) error {
	et, err := ethtool.NewEthtool()
	if err != nil {
		return fmt.Errorf("failed to get ethtool: %w", err)
	}
	defer et.Close()

	hostRDMAValue := "false"
	podName, podNamespace := "", ""
	topOwnerAPIVersion, topOwnerKind, topOwnerNamespace, topOwnerName := "", "", "", ""
	if pod != nil {
		hostRDMAValue = fmt.Sprintf("%v", pod.HostRDMA)
		podName = pod.Name
		podNamespace = pod.Namespace
		if pod.TopOwner != nil {
			topOwnerAPIVersion = pod.TopOwner.APIVersion
			topOwnerKind = pod.TopOwner.Kind
			topOwnerNamespace = pod.TopOwner.Namespace
			topOwnerName = pod.TopOwner.Name
		}
	}

	labels := []string{"node_name", "device", "ifname", "priority", "pod_name", "pod_namespace", "topowner_api_version", "topowner_kind", "topowner_namespace", "topowner_name", "host_rdma"}

	for ifname, device := range ifNameToDevice {
		stats, err := et.Stats(ifname)
		if err != nil {
			logger.Debug("failed to get ethtool stats for interface", "interface", ifname, "error", err)
			continue
		}
		for statName, statValue := range stats {
			if !expectPriorityMetrics(statName) {
				continue
			}
			name, priority, ok := extractNameWithPriority(statName)
			if !ok {
				continue
			}
			metricName := "unifabric_" + name
			desc := prometheus.NewDesc(metricName, "", labels, nil)
			labelValues := []string{nodeName, device, ifname, fmt.Sprintf("%d", priority), podName, podNamespace, topOwnerAPIVersion, topOwnerKind, topOwnerNamespace, topOwnerName, hostRDMAValue}
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(statValue), labelValues...)
		}
	}
	return nil
}

func collectEthToolToCache(metrics *hostRdmaMetrics, rootIfNameList []string, logger *slog.Logger) {
	et, err := ethtool.NewEthtool()
	if err != nil {
		logger.Error("failed to init ethtool", "error", err)
		return
	}
	defer et.Close()
	for _, iface := range rootIfNameList {
		stats, err := et.Stats(iface)
		if err != nil {
			logger.Debug("failed to get ethtool stats for interface", "interface", iface, "error", err)
			continue
		}
		for statName, statValue := range stats {
			if !expectPriorityMetrics(statName) {
				continue
			}
			name, priority, ok := extractNameWithPriority(statName)
			if !ok {
				continue
			}
			metrics.ethtoolCounters = append(metrics.ethtoolCounters, ethtoolCounterValue{
				metricName: name,
				ifname:     iface,
				priority:   priority,
				value:      statValue,
			})
		}
	}
}

func exportHostRdmaMetrics(nodeName string, metrics *hostRdmaMetrics, ch chan<- prometheus.Metric, logger *slog.Logger, pod *v1beta1.RdmaPod, pciToIfname map[string]string, pciToPfIfname map[string]string) {
	hostRDMAValue := ""
	podName, podNamespace := "", ""
	topOwnerAPIVersion, topOwnerKind, topOwnerNamespace, topOwnerName := "", "", "", ""
	if pod != nil {
		if pod.HostRDMA {
			hostRDMAValue = "true"
		} else {
			hostRDMAValue = "false"
		}
		podName = pod.Name
		podNamespace = pod.Namespace
		if pod.TopOwner != nil {
			topOwnerAPIVersion = pod.TopOwner.APIVersion
			topOwnerKind = pod.TopOwner.Kind
			topOwnerNamespace = pod.TopOwner.Namespace
			topOwnerName = pod.TopOwner.Name
		}
	}
	labels := []string{"node_name", "device", "ifname", "parent_ifname", "port", "pod_name", "pod_namespace", "topowner_api_version", "topowner_kind", "topowner_namespace", "topowner_name", "host_rdma", "is_root"}
	deviceLabels := []string{"node_name", "device", "ifname", "parent_ifname", "pod_name", "pod_namespace", "topowner_api_version", "topowner_kind", "topowner_namespace", "topowner_name", "host_rdma", "is_root"}
	for _, c := range metrics.counters {
		metricName := "unifabric_" + c.counter
		isRoot := "false"
		if c.parentIfname == c.ifname {
			isRoot = "true"
		}
		if c.counter == "rdma_device_tos" {
			if hostRDMAValue == "true" {
				continue
			}
			desc := prometheus.NewDesc(metricName, "RDMA device ToS value", deviceLabels, nil)
			labelValues := []string{nodeName, c.device, c.ifname, c.parentIfname, podName, podNamespace, topOwnerAPIVersion, topOwnerKind, topOwnerNamespace, topOwnerName, hostRDMAValue, isRoot}
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, c.value, labelValues...)
		} else {
			desc := prometheus.NewDesc(metricName, "", labels, nil)
			labelValues := []string{nodeName, c.device, c.ifname, c.parentIfname, c.port, podName, podNamespace, topOwnerAPIVersion, topOwnerKind, topOwnerNamespace, topOwnerName, hostRDMAValue, isRoot}
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, c.value, labelValues...)
		}
	}
	ethtoolLabels := []string{"node_name", "ifname", "priority", "pod_name", "pod_namespace", "topowner_api_version", "topowner_kind", "topowner_namespace", "topowner_name", "host_rdma"}
	for _, c := range metrics.ethtoolCounters {
		metricName := "unifabric_" + c.metricName
		desc := prometheus.NewDesc(metricName, "", ethtoolLabels, nil)
		labelValues := []string{nodeName, c.ifname, fmt.Sprintf("%d", c.priority), podName, podNamespace, topOwnerAPIVersion, topOwnerKind, topOwnerNamespace, topOwnerName, hostRDMAValue}
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(c.value), labelValues...)
	}
}

func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
	m.logger.Info("starting RDMA metrics collection")

	node := m.cli.GetFabricNode()
	if node == nil {
		m.logger.Debug("FabricNode is nil, skipping metrics collection")
		return
	}

	m.logger.Debug("FabricNode retrieved", "numRdmaPods", len(node.Status.RdmaPods))

	hostMetrics, pciToIfname, pciToPfIfname := collectHostRdmaMetrics(m.logger)

	for _, pod := range node.Status.RdmaPods {
		m.logger.Debug("processing RDMA pod", "pod", pod.Name, "namespace", pod.Namespace)
		if pod.HostRDMA {
			exportHostRdmaMetrics(node.Name, hostMetrics, ch, m.logger, &pod, pciToIfname, pciToPfIfname)
			continue
		}
		for _, containerID := range pod.ContainerList {
			m.logger.Debug("attempting to get init pid for container", "containerID", containerID)
			pid, err := getContainerInitPid(containerID)
			if err != nil {
				m.logger.Error("failed to get init pid for container", "error", err, "containerID", containerID, "pod_name", pod.Name, "pod_namespace", pod.Namespace)
				continue
			}
			m.logger.Debug("got init pid for container", "pid", pid, "containerID", containerID)

			var pciToIfname, ifNameToDevice map[string]string

			err = withMountNamespace(pid, func() error {
				m.logger.Debug("entering mount namespace for container", "pid", pid, "containerID", containerID)
				var err error
				pciToIfname, ifNameToDevice, err = collectRdmaCounters(node.Name, &pod, ch, m.logger, pciToPfIfname)
				return err
			})
			if err != nil {
				m.logger.Error("failed to collect RDMA counters in namespace", "error", err, "containerID", containerID)
			}

			err = ns.WithNetNSPath(fmt.Sprintf("/host/proc/%v/ns/net", pid), func(hostNS ns.NetNS) error {
				return collectEthToolCounters(node.Name, &pod, ch, m.logger, ifNameToDevice, pciToIfname, pciToPfIfname)
			})
			if err != nil {
				m.logger.Error("failed to enter netns for ethtool collection", "error", err)
			}

			m.logger.Debug("successfully collected RDMA counters", "containerID", containerID)
			break
		}
	}

	exportHostRdmaMetrics(node.Name, hostMetrics, ch, m.logger, nil, pciToIfname, pciToPfIfname)

	m.logger.Info("completed RDMA metrics collection")
}

func getContainerInitPid(containerID string) (int, error) {
	if !strings.HasPrefix(containerID, "containerd://") {
		return 0, fmt.Errorf("not a containerd container: %s", containerID)
	}
	realContainerID := strings.TrimPrefix(containerID, "containerd://")
	pidPath := filepath.Join("/host/run/containerd/io.containerd.runtime.v2.task/k8s.io", realContainerID, "init.pid")
	pidBytes, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %w", pidPath, err)
	}
	pidStr := strings.TrimSpace(string(pidBytes))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse pid from %s: %w", pidPath, err)
	}
	return pid, nil
}

func withMountNamespace(pid int, fn func() error) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := unix.Unshare(unix.CLONE_FS); err != nil {
		return fmt.Errorf("unshare: %v", err)
	}

	originalNS, err := os.Open("/proc/self/ns/mnt")
	if err != nil {
		return fmt.Errorf("could not open original mount namespace: %w", err)
	}
	defer originalNS.Close()

	mntPath := fmt.Sprintf("/host/proc/%d/ns/mnt", pid)
	targetNS, err := os.Open(mntPath)
	if err != nil {
		return fmt.Errorf("could not open target mount namespace %s: %w", mntPath, err)
	}
	defer targetNS.Close()

	if err := unix.Setns(int(targetNS.Fd()), unix.CLONE_NEWNS); err != nil {
		return fmt.Errorf("could not switch to mount namespace %s: %w", mntPath, err)
	}
	defer func() {
		if restoreErr := unix.Setns(int(originalNS.Fd()), unix.CLONE_NEWNS); restoreErr != nil && err == nil {
			err = fmt.Errorf("could not switch back to original mount namespace: %w", restoreErr)
		}
	}()

	if err := fn(); err != nil {
		return err
	}
	return nil
}

func collectCountersDir(dirPath string, labels []string, labelValues []string, ch chan<- prometheus.Metric, logger *slog.Logger) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		logger.Error("failed to list counters", "error", err, "countersPath", dirPath)
		return
	}
	for _, f := range files {
		if !f.Type().IsRegular() {
			continue
		}
		name := f.Name()
		valuePath := filepath.Join(dirPath, name)
		valueStr, err := os.ReadFile(valuePath)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) || err.Error() == "operation not supported" {
				continue
			}
			logger.Error("failed to read counter file", "error", err, "counterFile", valuePath)
			continue
		}
		value := strings.TrimSpace(string(valueStr))
		if strings.Contains(value, "N/A (no PMA)") {
			continue
		}
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			logger.Error("failed to parse counter value", "error", err, "counterFile", valuePath, "valueStr", value)
			continue
		}
		if name == "port_xmit_data" || name == "port_rcv_data" {
			v = v * 4
		}
		metricName := "unifabric_" + name
		desc := prometheus.NewDesc(metricName, "", labels, nil)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, labelValues...)
	}
}

func collectRdmaCounters(nodeName string, pod *v1beta1.RdmaPod, ch chan<- prometheus.Metric, logger *slog.Logger, pciToPfIfname map[string]string) (map[string]string, map[string]string, error) {
	hostRDMAValue := "false"
	if pod.HostRDMA {
		hostRDMAValue = "true"
	}
	topOwnerAPIVersion, topOwnerKind, topOwnerNamespace, topOwnerName := "", "", "", ""
	if pod.TopOwner != nil {
		topOwnerAPIVersion = pod.TopOwner.APIVersion
		topOwnerKind = pod.TopOwner.Kind
		topOwnerNamespace = pod.TopOwner.Namespace
		topOwnerName = pod.TopOwner.Name
	}
	deviceLabels := []string{"node_name", "device", "ifname", "parent_ifname", "port", "pod_name", "pod_namespace", "topowner_api_version", "topowner_kind", "topowner_namespace", "topowner_name", "host_rdma", "is_root"}
	speedLabels := []string{"node_name", "device", "ifname", "parent_ifname", "pod_name", "pod_namespace", "topowner_api_version", "topowner_kind", "topowner_namespace", "topowner_name", "host_rdma", "is_root"}
	pciToIfname := buildPciToIfnameMap()

	basePath := "/sys/class/infiniband"
	devices, err := os.ReadDir(basePath)
	if err != nil {
		logger.Error("failed to list infiniband base path", "error", err, "basePath", basePath)
		return nil, nil, fmt.Errorf("failed to list %s: %w", basePath, err)
	}

	ifNameToDevice := make(map[string]string, len(devices))

	for _, dev := range devices {
		ifname, err := getIfNameForInfinibandDevice(dev.Name(), pciToIfname)
		if err != nil {
			logger.Debug("no matching netdev for infiniband device", "rdma_device", dev.Name())
			ifname = ""
			continue
		}
		parentIfname := ifname
		if ifname != "" {
			physfnPath := filepath.Join("/sys/class/net", ifname, "device", "physfn")
			pfPciPath, err := filepath.EvalSymlinks(physfnPath)
			if err == nil {
				if pfIfname, ok := pciToPfIfname[pfPciPath]; ok {
					parentIfname = pfIfname
				}
			}
		}
		portsPath := filepath.Join(basePath, dev.Name(), "ports")
		ports, err := os.ReadDir(portsPath)
		if err != nil {
			logger.Error("failed to list ports", "error", err, "portsPath", portsPath)
			continue
		}
		isRoot := "false"
		if parentIfname == ifname {
			isRoot = "true"
		}
		for _, port := range ports {
			labelValues := []string{nodeName, dev.Name(), ifname, parentIfname, port.Name(), pod.Name, pod.Namespace, topOwnerAPIVersion, topOwnerKind, topOwnerNamespace, topOwnerName, hostRDMAValue, isRoot}
			hwCountersPath := filepath.Join(portsPath, port.Name(), "hw_counters")
			collectCountersDir(hwCountersPath, deviceLabels, labelValues, ch, logger)
			countersPath := filepath.Join(portsPath, port.Name(), "counters")
			collectCountersDir(countersPath, deviceLabels, labelValues, ch, logger)
		}
		deviceMetricLabelValues := []string{nodeName, dev.Name(), ifname, parentIfname, pod.Name, pod.Namespace, topOwnerAPIVersion, topOwnerKind, topOwnerNamespace, topOwnerName, hostRDMAValue, isRoot}
		if ifname != "" {
			collectIfSpeed(ifname, speedLabels, deviceMetricLabelValues, ch, logger)
		}
		// TOS metric for device
		tosValue := getDeviceTos(basePath, dev.Name(), logger)
		desc := prometheus.NewDesc("unifabric_rdma_device_tos", "RDMA device ToS value", speedLabels, nil)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, tosValue, deviceMetricLabelValues...)

		ifNameToDevice[ifname] = dev.Name()
	}
	return pciToIfname, ifNameToDevice, nil
}

func collectEthtoolCounters(ifname string, ch chan<- prometheus.Metric, logger *slog.Logger) {
}

func getIfSpeed(ifname string) (float64, error) {
	if ifname == "" {
		return 0, fmt.Errorf("empty ifname")
	}
	speedPath := filepath.Join("/sys/class/net", ifname, "speed")
	speedBytes, err := os.ReadFile(speedPath)
	if err != nil {
		return 0, err
	}
	speedStr := strings.TrimSpace(string(speedBytes))
	speedVal, err := strconv.ParseFloat(speedStr, 64)
	if err != nil {
		return 0, err
	}
	return speedVal, nil
}

func getIfMTU(ifname string) (float64, error) {
	if ifname == "" {
		return 0, fmt.Errorf("empty ifname")
	}
	path := filepath.Join("/sys/class/net", ifname, "mtu")
	bytes, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	str := strings.TrimSpace(string(bytes))
	res, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func getIfOperState(ifname string) (float64, error) {
	if ifname == "" {
		return 0, fmt.Errorf("empty ifname")
	}
	path := filepath.Join("/sys/class/net", ifname, "operstate")
	bytes, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	str := strings.TrimSpace(string(bytes))
	if str == "up" {
		return 1, nil
	}
	return 0, nil
}

func collectIfSpeed(ifname string, labels []string, labelValues []string, ch chan<- prometheus.Metric, logger *slog.Logger) {
	speedVal, err := getIfSpeed(ifname)
	if err != nil {
		logger.Error("failed to collect interface speed", "error", err, "ifname", ifname)
	} else {
		desc := prometheus.NewDesc("unifabric_port_speed_mbps", "", labels, nil)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, speedVal, labelValues...)
	}
	mtuVal, err := getIfMTU(ifname)
	if err != nil {
		logger.Error("failed to collect interface MTU", "error", err, "ifname", ifname)
	} else {
		desc := prometheus.NewDesc("unifabric_port_mtu", "", labels, nil)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, mtuVal, labelValues...)
	}
	operState, err := getIfOperState(ifname)
	if err != nil {
		logger.Error("failed to collect interface operstate", "error", err, "ifname", ifname)
	} else {
		desc := prometheus.NewDesc("unifabric_port_oper_state", "", labels, nil)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, operState, labelValues...)
	}
}

func collectIfInfoToCache(ifname, device, port, parentIfname string, metrics *hostRdmaMetrics, logger *slog.Logger) {
	speedVal, err := getIfSpeed(ifname)
	if err != nil {
		logger.Error("failed to collect interface speed", "error", err, "ifname", ifname)
	} else {
		metrics.counters = append(metrics.counters, rdmaCounterValue{
			device:       device,
			ifname:       ifname,
			port:         port,
			parentIfname: parentIfname,
			counter:      "port_speed_mbps",
			value:        speedVal,
		})
	}
	mtuVal, err := getIfMTU(ifname)
	if err != nil {
		logger.Error("failed to collect interface MTU", "error", err, "ifname", ifname)
	} else {
		metrics.counters = append(metrics.counters, rdmaCounterValue{
			device:       device,
			ifname:       ifname,
			port:         port,
			parentIfname: parentIfname,
			counter:      "port_mtu",
			value:        mtuVal,
		})
	}
	operState, err := getIfOperState(ifname)
	if err != nil {
		logger.Error("failed to collect interface oper state", "error", err, "ifname", ifname)
	} else {
		metrics.counters = append(metrics.counters, rdmaCounterValue{
			device:       device,
			ifname:       ifname,
			port:         port,
			parentIfname: parentIfname,
			counter:      "port_oper_state",
			value:        operState,
		})
	}
}

func collectDeviceTosToCache(basePath, device, ifname, parentIfname string, metrics *hostRdmaMetrics, logger *slog.Logger) {
	tosValue := getDeviceTos(basePath, device, logger)
	metrics.counters = append(metrics.counters, rdmaCounterValue{
		device:       device,
		ifname:       ifname,
		port:         "",
		parentIfname: parentIfname,
		counter:      "rdma_device_tos",
		value:        tosValue,
	})
}

func getDeviceTos(basePath, deviceName string, logger *slog.Logger) float64 {
	tosPath := filepath.Join(basePath, deviceName, "tc", "1", "traffic_class")
	content, err := os.ReadFile(tosPath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Debug("failed to read ToS file", "path", tosPath, "error", err)
		}
		return 0
	}

	s := string(content)
	const prefix = "Global tclass="
	if !strings.HasPrefix(s, prefix) {
		return 0
	}

	valStr := strings.TrimSpace(strings.TrimPrefix(s, prefix))
	v, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		logger.Debug("failed to parse ToS value", "path", tosPath, "value", valStr)
		return 0
	}
	return v
}

func expectPriorityMetrics(s string) bool {
	if len(s) == 14 {
		if (strings.HasPrefix(s, "rx_prio") || strings.HasPrefix(s, "tx_prio")) && strings.HasSuffix(s, "_pause") {
			return true
		}
	}
	if len(s) == 17 {
		if (strings.HasPrefix(s, "rx_prio") || strings.HasPrefix(s, "tx_prio")) && strings.HasSuffix(s, "_discards") {
			return true
		}
	}
	return false
}

func extractNameWithPriority(str string) (name string, priority int, ok bool) {
	parts := strings.Split(str, "_")
	if len(parts) != 3 {
		return "", 0, false
	}

	name = parts[0] + "_" + parts[2]

	rawPriority := parts[1]
	if len(rawPriority) < 5 {
		return "", 0, false
	}

	var err error

	if strings.HasPrefix(rawPriority, "prio") {
		priority, err = strconv.Atoi(strings.TrimPrefix(rawPriority, "prio"))
		if err != nil {
			return "", 0, false
		}
	} else {
		return "", 0, false
	}

	return name, priority, true
}

func NewMetrics(cli fabricnode.Interface, logger *slog.Logger) prometheus.Collector {
	return &Metrics{
		cli:    cli,
		logger: logger,
	}
}
