// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ibdiscovery

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/ib"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	switchGUIDAnnotation     = "unifabric.io/ib-switch-guid"
	switchSubnetAnnotation   = "unifabric.io/ib-subnet-prefix"
	switchLastSeenAnnotation = "unifabric.io/ib-last-seen"
)

var invalidNameChars = regexp.MustCompile(`[^a-z0-9.-]+`)

type switchWriter struct {
	client client.Client
	reader client.Reader
	ttl    time.Duration
	log    *slog.Logger
	now    func() time.Time
}

// sync makes the Switch objects of one subnet match the scan result: it
// creates or updates one Switch per discovered switch and deletes the ones that
// have not been reported for longer than the TTL.
func (w *switchWriter) sync(ctx context.Context, subnet string, discovered []*ib.Discovered) error {
	names := switchNames(discovered)
	now := w.now()

	for _, d := range discovered {
		if err := w.upsert(ctx, subnet, names[d.ID], d, names, now); err != nil {
			return err
		}
	}
	return w.collectGarbage(ctx, subnet, names, now)
}

func (w *switchWriter) upsert(ctx context.Context, subnet, name string, d *ib.Discovered, names map[string]string, now time.Time) error {
	neighbors := neighborsOf(d, names)

	var existing v1beta1.Switch
	err := w.reader.Get(ctx, types.NamespacedName{Name: name}, &existing)
	if apierrors.IsNotFound(err) {
		sw := &v1beta1.Switch{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Labels:      map[string]string{v1beta1.SwitchDiscoveredByLabel: v1beta1.SwitchDiscoveredByAgent},
				Annotations: ibAnnotations(subnet, d.ID, now),
			},
		}
		if err := w.client.Create(ctx, sw); err != nil {
			return fmt.Errorf("create Switch %q: %w", name, err)
		}
		return w.patchStatus(ctx, sw, neighbors)
	}
	if err != nil {
		return fmt.Errorf("get Switch %q: %w", name, err)
	}

	if existing.Labels[v1beta1.SwitchDiscoveredByLabel] != v1beta1.SwitchDiscoveredByAgent {
		w.log.Warn("skipping Switch that is not managed by the agent", "switch", name, "guid", d.ID)
		return nil
	}
	if guid := existing.Annotations[switchGUIDAnnotation]; guid != "" && guid != d.ID {
		w.log.Warn("skipping Switch name collision with another discovered Switch", "switch", name, "guid", d.ID)
		return nil
	}

	if w.needsMetadataUpdate(&existing, subnet, d.ID, now) {
		patch := client.MergeFrom(existing.DeepCopy())
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		for k, v := range ibAnnotations(subnet, d.ID, now) {
			existing.Annotations[k] = v
		}
		if err := w.client.Patch(ctx, &existing, patch); err != nil {
			return fmt.Errorf("patch Switch %q: %w", name, err)
		}
	}
	return w.patchStatus(ctx, &existing, neighbors)
}

// needsMetadataUpdate throttles the last-seen refresh so a stable fabric does
// not rewrite every Switch on every scan.
func (w *switchWriter) needsMetadataUpdate(sw *v1beta1.Switch, subnet, guid string, now time.Time) bool {
	if sw.Annotations[switchSubnetAnnotation] != subnet || sw.Annotations[switchGUIDAnnotation] != guid {
		return true
	}
	seen, err := time.Parse(time.RFC3339, sw.Annotations[switchLastSeenAnnotation])
	return err != nil || now.Sub(seen) > w.ttl/3
}

func (w *switchWriter) patchStatus(ctx context.Context, sw *v1beta1.Switch, neighbors []v1beta1.SwitchNeighbor) error {
	same := len(sw.Status.LLDPNeighbors) == 0 && len(neighbors) == 0 || reflect.DeepEqual(sw.Status.LLDPNeighbors, neighbors)
	if same && sw.Status.LLDPNeighborCount == int32(len(neighbors)) {
		return nil
	}
	patch := client.MergeFrom(sw.DeepCopy())
	sw.Status.LLDPNeighbors = neighbors
	sw.Status.LLDPNeighborCount = int32(len(neighbors))
	if err := w.client.Status().Patch(ctx, sw, patch); err != nil {
		return fmt.Errorf("patch Switch %q status: %w", sw.Name, err)
	}
	return nil
}

func (w *switchWriter) collectGarbage(ctx context.Context, subnet string, current map[string]string, now time.Time) error {
	var list v1beta1.SwitchList
	if err := w.reader.List(ctx, &list, client.MatchingLabels{v1beta1.SwitchDiscoveredByLabel: v1beta1.SwitchDiscoveredByAgent}); err != nil {
		return fmt.Errorf("list discovered Switches: %w", err)
	}
	live := map[string]struct{}{}
	for _, name := range current {
		live[name] = struct{}{}
	}
	for i := range list.Items {
		sw := &list.Items[i]
		if _, ok := live[sw.Name]; ok || sw.Annotations[switchSubnetAnnotation] != subnet {
			continue
		}
		seen, err := time.Parse(time.RFC3339, sw.Annotations[switchLastSeenAnnotation])
		if err == nil && now.Sub(seen) <= w.ttl {
			continue
		}
		if err := w.client.Delete(ctx, sw); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("delete stale Switch %q: %w", sw.Name, err)
		}
		w.log.Info("deleted Switch that is no longer reported", "switch", sw.Name, "subnet", subnet)
	}
	return nil
}

func ibAnnotations(subnet, guid string, now time.Time) map[string]string {
	return map[string]string{
		switchGUIDAnnotation:     guid,
		switchSubnetAnnotation:   subnet,
		switchLastSeenAnnotation: now.UTC().Format(time.RFC3339),
	}
}

// neighborsOf converts the links of a switch into SwitchNeighbors, nodes first,
// each group sorted by name.
func neighborsOf(d *ib.Discovered, names map[string]string) []v1beta1.SwitchNeighbor {
	neighbors := make([]v1beta1.SwitchNeighbor, 0, len(d.Nodes)+len(d.Switches))
	for node, links := range d.Nodes {
		neighbors = append(neighbors, v1beta1.SwitchNeighbor{
			RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode,
			RemoteSystemName: node,
			LinkCount:        links,
		})
	}
	for id, links := range d.Switches {
		neighbors = append(neighbors, v1beta1.SwitchNeighbor{
			RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch,
			RemoteSystemName: names[id],
			LinkCount:        links,
		})
	}
	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].RemoteSystemType != neighbors[j].RemoteSystemType {
			return neighbors[i].RemoteSystemType == v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode
		}
		return neighbors[i].RemoteSystemName < neighbors[j].RemoteSystemName
	})
	return neighbors
}

// switchNames maps every switch GUID to a Kubernetes object name: the parsed
// switch name, or the GUID when there is none. Names shared by several GUIDs
// get the GUID appended so they stay unique.
func switchNames(discovered []*ib.Discovered) map[string]string {
	count := map[string]int{}
	base := make(map[string]string, len(discovered))
	for _, d := range discovered {
		name := sanitizeName(d.Name)
		if name == "" {
			name = sanitizeName(d.ID)
		}
		base[d.ID] = name
		count[name]++
	}
	names := make(map[string]string, len(discovered))
	for _, d := range discovered {
		name := base[d.ID]
		if count[name] > 1 {
			name = trimName(name + "-" + sanitizeName(d.ID))
		}
		names[d.ID] = name
	}
	return names
}

func sanitizeName(raw string) string {
	name := invalidNameChars.ReplaceAllString(strings.ToLower(raw), "-")
	return trimName(name)
}

func trimName(name string) string {
	if len(name) > 253 {
		name = name[:253]
	}
	return strings.Trim(name, "-.")
}
