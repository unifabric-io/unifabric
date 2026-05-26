// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/switchagent"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	grpcstatus "google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	controllerMTLSCertFile = "/etc/unifabric/switch-mtls/tls.crt"
	controllerMTLSKeyFile  = "/etc/unifabric/switch-mtls/tls.key"
	controllerMTLSPeerFile = "/etc/unifabric/switch-mtls/peer.crt"
)

type switchSubscription struct {
	target string
	cancel context.CancelFunc
}

type subscriptionManager struct {
	client           client.Client
	cfg              *config.ControllerConfig
	log              *slog.Logger
	dialTimeout      time.Duration
	reconnectBackoff time.Duration
	keepaliveTime    time.Duration

	mu            sync.Mutex
	subscriptions map[string]switchSubscription
}

func newSubscriptionManager(client client.Client, cfg *config.ControllerConfig, logger *slog.Logger) (*subscriptionManager, error) {
	dialTimeout, err := time.ParseDuration(cfg.ScaleOutDiscovery.Switches.DialTimeout)
	if err != nil {
		return nil, err
	}
	reconnectBackoff, err := time.ParseDuration(cfg.ScaleOutDiscovery.Switches.ReconnectBackoff)
	if err != nil {
		return nil, err
	}
	keepaliveTime, err := time.ParseDuration(cfg.ScaleOutDiscovery.Switches.KeepaliveTime)
	if err != nil {
		return nil, err
	}

	return &subscriptionManager{
		client:           client,
		cfg:              cfg,
		log:              logger.With("component", "switch-subscription"),
		dialTimeout:      dialTimeout,
		reconnectBackoff: reconnectBackoff,
		keepaliveTime:    keepaliveTime,
		subscriptions:    map[string]switchSubscription{},
	}, nil
}

func (m *subscriptionManager) NeedLeaderElection() bool {
	return true
}

func (m *subscriptionManager) Start(ctx context.Context) error {
	if err := m.syncSubscriptions(ctx); err != nil {
		m.log.Error("initial subscription sync failed", "error", err)
	}

	ticker := time.NewTicker(m.reconnectBackoff)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.stopAllSubscriptions()
			return nil
		case <-ticker.C:
			if err := m.syncSubscriptions(ctx); err != nil {
				m.log.Error("subscription sync failed", "error", err)
			}
		}
	}
}

func (m *subscriptionManager) syncSubscriptions(ctx context.Context) error {
	var switchList v1beta1.SwitchList
	if err := m.client.List(ctx, &switchList); err != nil {
		return err
	}

	desiredTargets := make(map[string]string, len(switchList.Items))
	for _, sw := range switchList.Items {
		if sw.Spec.MgmtIP == "" {
			m.stopSubscription(sw.Name)
			if err := m.markSwitchDisconnected(ctx, sw.Name, v1beta1.SwitchReasonDialFailed, "switch spec.mgmtIP is empty"); err != nil {
				return err
			}
			continue
		}

		grpcPort := m.cfg.ScaleOutDiscovery.Switches.DefaultGrpcPort
		if sw.Spec.GrpcPort != nil {
			grpcPort = *sw.Spec.GrpcPort
		}
		desiredTargets[sw.Name] = fmt.Sprintf("%s:%d", sw.Spec.MgmtIP, grpcPort)
	}

	m.mu.Lock()
	for switchName, subscription := range m.subscriptions {
		target, ok := desiredTargets[switchName]
		if ok && target == subscription.target {
			continue
		}
		subscription.cancel()
		delete(m.subscriptions, switchName)
	}
	m.mu.Unlock()

	for switchName, target := range desiredTargets {
		if m.hasSubscription(switchName, target) {
			continue
		}

		subscriptionCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.subscriptions[switchName] = switchSubscription{target: target, cancel: cancel}
		m.mu.Unlock()

		go m.runSubscription(subscriptionCtx, switchName, target)
	}

	return nil
}

func (m *subscriptionManager) hasSubscription(switchName, target string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscription, ok := m.subscriptions[switchName]
	return ok && subscription.target == target
}

func (m *subscriptionManager) stopSubscription(switchName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscription, ok := m.subscriptions[switchName]
	if !ok {
		return
	}
	subscription.cancel()
	delete(m.subscriptions, switchName)
}

func (m *subscriptionManager) stopAllSubscriptions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for switchName, subscription := range m.subscriptions {
		subscription.cancel()
		delete(m.subscriptions, switchName)
	}
}

func (m *subscriptionManager) runSubscription(ctx context.Context, switchName, target string) {
	var lastGeneration uint64

	for {
		if err := m.subscribeUntilDisconnect(ctx, switchName, target, &lastGeneration); err != nil && !errors.Is(err, context.Canceled) {
			reason := classifySubscriptionError(err)
			m.log.Warn("switch subscription failed", "switchName", switchName, "target", target, "reason", reason, "error", err)
			if updateErr := m.markSwitchDisconnected(ctx, switchName, reason, err.Error()); updateErr != nil {
				m.log.Error("failed to update disconnected switch status", "switchName", switchName, "error", updateErr)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(m.reconnectBackoff):
		}
	}
}

func (m *subscriptionManager) subscribeUntilDisconnect(ctx context.Context, switchName, target string, lastGeneration *uint64) error {
	dialCtx, cancel := context.WithTimeout(ctx, m.dialTimeout)
	defer cancel()

	transportCredentials, err := m.transportCredentials()
	if err != nil {
		return err
	}

	conn, err := grpc.DialContext(
		dialCtx,
		target,
		grpc.WithBlock(),
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(m.cfg.ScaleOutDiscovery.Switches.MaxRecvMsgSize)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                m.keepaliveTime,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	stream, err := switchagent.NewSwitchReporterClient(conn).WatchLLDPNeighbors(ctx, &switchagent.WatchLLDPNeighborsRequest{})
	if err != nil {
		return err
	}

	for {
		snapshot, err := stream.Recv()
		if err != nil {
			return err
		}

		updated, err := m.handleSnapshot(ctx, switchName, snapshot, lastGeneration)
		if err != nil {
			return err
		}
		if !updated {
			continue
		}
	}
}

func (m *subscriptionManager) handleSnapshot(ctx context.Context, switchName string, snapshot *switchagent.LLDPNeighborSnapshot, lastGeneration *uint64) (bool, error) {
	if snapshot.GetGeneration() <= *lastGeneration {
		m.log.Debug("ignore stale switch snapshot", "switchName", switchName, "generation", snapshot.GetGeneration(), "lastGeneration", *lastGeneration)
		return false, nil
	}

	nodeNames, normalizedNodeNames, err := m.kubernetesNodeNames(ctx)
	if err != nil {
		m.log.Warn("failed to list kubernetes nodes for lldp neighbor classification", "switchName", switchName, "error", err)
		nodeNames = map[string]bool{}
		normalizedNodeNames = map[string]bool{}
	}

	var ignoreSwitchPorts []string
	if m.cfg != nil {
		ignoreSwitchPorts = m.cfg.ScaleOutDiscovery.Switches.IgnoreSwitchPorts
	}

	neighbors, err := normalizeSnapshotNeighbors(snapshot, switchName, ignoreSwitchPorts, nodeNames, normalizedNodeNames)
	if err != nil {
		observeLLDPParseFailure()
		return false, err
	}
	hostname := snapshot.GetSwitchName()
	if hostname == "" {
		hostname = switchName
	}

	if err := m.mutateSwitchStatus(ctx, switchName, func(sw *v1beta1.Switch) {
		sw.Status.Hostname = hostname
		sw.Status.Healthy = true
		sw.Status.LLDPNeighborCount = int32(len(neighbors))
		sw.Status.LLDPNeighbors = neighbors
		setSwitchCondition(&sw.Status, sw.Generation, v1beta1.SwitchConditionConnected, metav1.ConditionTrue, v1beta1.SwitchReasonStreamReady, "controller is receiving LLDP snapshots from switch agent")
		setSwitchCondition(&sw.Status, sw.Generation, v1beta1.SwitchConditionReady, metav1.ConditionTrue, v1beta1.SwitchReasonSnapshotAccepted, fmt.Sprintf("accepted LLDP snapshot generation %d", snapshot.GetGeneration()))
	}); err != nil {
		return false, err
	}
	observeLLDPParseSuccess()
	m.log.Debug(
		"accepted switch snapshot",
		"switchName", switchName,
		"generation", snapshot.GetGeneration(),
		"reportedNeighborCount", len(snapshot.GetLldpNeighbors()),
		"storedNeighborCount", len(neighbors),
	)

	*lastGeneration = snapshot.GetGeneration()
	return true, nil
}

func normalizeSnapshotNeighbors(
	snapshot *switchagent.LLDPNeighborSnapshot,
	switchName string,
	ignoreSwitchPorts []string,
	nodeNames map[string]bool,
	normalizedNodeNames map[string]bool,
) ([]v1beta1.SwitchNeighbor, error) {
	neighbors := make([]v1beta1.SwitchNeighbor, 0, len(snapshot.GetLldpNeighbors()))
	seenNeighbors := map[string]bool{}

	for _, neighbor := range snapshot.GetLldpNeighbors() {
		if neighbor.GetRemoteSystemName() == "" {
			return nil, grpcstatus.Error(codes.InvalidArgument, "snapshot contains malformed lldp neighbor entry")
		}
		if shouldIgnoreSwitchPort(neighbor.GetLocalPort(), ignoreSwitchPorts) {
			continue
		}

		remoteSystemType := classifyRemoteSystemType(neighbor.GetRemoteSystemName(), nodeNames, normalizedNodeNames)
		normalized := v1beta1.SwitchNeighbor{
			RemoteSystemType: remoteSystemType,
			RemoteSystemName: neighbor.GetRemoteSystemName(),
		}
		key := fmt.Sprintf("%s|%s", normalized.RemoteSystemType, normalized.RemoteSystemName)
		if seenNeighbors[key] {
			continue
		}
		seenNeighbors[key] = true
		neighbors = append(neighbors, normalized)
	}

	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].RemoteSystemType != neighbors[j].RemoteSystemType {
			return neighbors[i].RemoteSystemType < neighbors[j].RemoteSystemType
		}
		return neighbors[i].RemoteSystemName < neighbors[j].RemoteSystemName
	})

	return neighbors, nil
}

func (m *subscriptionManager) kubernetesNodeNames(ctx context.Context) (map[string]bool, map[string]bool, error) {
	var nodeList corev1.NodeList
	if err := m.client.List(ctx, &nodeList); err != nil {
		return nil, nil, err
	}

	nodeNames := make(map[string]bool, len(nodeList.Items))
	normalizedNodeNames := make(map[string]bool, len(nodeList.Items))
	for _, node := range nodeList.Items {
		nodeNames[node.Name] = true
		normalizedNodeNames[normalizeResourceName(node.Name)] = true
	}

	return nodeNames, normalizedNodeNames, nil
}

func classifyRemoteSystemType(remoteSystemName string, nodeNames map[string]bool, normalizedNodeNames map[string]bool) v1beta1.SwitchLLDPRemoteSystemType {
	if nodeNames[remoteSystemName] || normalizedNodeNames[normalizeResourceName(remoteSystemName)] {
		return v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode
	}
	return v1beta1.SwitchLLDPRemoteSystemTypeSwitch
}

func shouldIgnoreSwitchPort(port string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, port)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func (m *subscriptionManager) markSwitchDisconnected(ctx context.Context, switchName, reason, message string) error {
	return m.mutateSwitchStatus(ctx, switchName, func(sw *v1beta1.Switch) {
		sw.Status.Healthy = false
		setSwitchCondition(&sw.Status, sw.Generation, v1beta1.SwitchConditionConnected, metav1.ConditionFalse, reason, message)
		setSwitchCondition(&sw.Status, sw.Generation, v1beta1.SwitchConditionReady, metav1.ConditionFalse, v1beta1.SwitchReasonDataStale, message)
	})
}

func (m *subscriptionManager) mutateSwitchStatus(ctx context.Context, switchName string, mutate func(*v1beta1.Switch)) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var sw v1beta1.Switch
		if err := m.client.Get(ctx, types.NamespacedName{Name: switchName}, &sw); err != nil {
			return client.IgnoreNotFound(err)
		}

		before := sw.Status.DeepCopy()
		mutate(&sw)
		if equality.Semantic.DeepEqual(before, &sw.Status) {
			return nil
		}

		return m.client.Status().Update(ctx, &sw)
	})
}

func (m *subscriptionManager) transportCredentials() (credentials.TransportCredentials, error) {
	if m.cfg.ScaleOutDiscovery.Switches.MTLS.Enabled == nil || *m.cfg.ScaleOutDiscovery.Switches.MTLS.Enabled {
		tlsConfig, err := loadPinnedMTLSClientConfig()
		if err != nil {
			return nil, err
		}
		return credentials.NewTLS(tlsConfig), nil
	}

	return insecure.NewCredentials(), nil
}

func loadPinnedMTLSClientConfig() (*tls.Config, error) {
	clientCert, err := tls.LoadX509KeyPair(controllerMTLSCertFile, controllerMTLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load controller client keypair: %w", err)
	}

	peerPEM, err := os.ReadFile(controllerMTLSPeerFile)
	if err != nil {
		return nil, fmt.Errorf("load pinned switch certificate: %w", err)
	}

	block, _ := pem.Decode(peerPEM)
	if block == nil {
		return nil, fmt.Errorf("parse pinned switch certificate: no PEM data found")
	}
	expectedPeer, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pinned switch certificate: %w", err)
	}

	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		Certificates:       []tls.Certificate{clientCert},
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2"},
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("missing server certificate")
			}

			presentedPeer, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("parse presented server certificate: %w", err)
			}
			if !bytes.Equal(presentedPeer.Raw, expectedPeer.Raw) {
				return fmt.Errorf("pinned switch certificate mismatch")
			}
			return nil
		},
	}, nil
}

func setSwitchCondition(status *v1beta1.SwitchStatus, generation int64, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             conditionStatus,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

func classifySubscriptionError(err error) string {
	if err == nil {
		return v1beta1.SwitchReasonDialFailed
	}
	if code := grpcstatus.Code(err); code == codes.FailedPrecondition || code == codes.Unauthenticated || code == codes.PermissionDenied || code == codes.InvalidArgument {
		return v1beta1.SwitchReasonAuthenticationFailed
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "certificate") || strings.Contains(message, "tls") {
		return v1beta1.SwitchReasonAuthenticationFailed
	}
	return v1beta1.SwitchReasonDialFailed
}
