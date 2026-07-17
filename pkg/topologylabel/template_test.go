// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologylabel

import "testing"

func TestCompileRendersAndMatchesTier(t *testing.T) {
	compiled, err := Compile("scaleOut", "scale-out.unifabric.io/tier-{{ .Tier }}")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	key, err := compiled.Render(14)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if key != "scale-out.unifabric.io/tier-14" {
		t.Fatalf("Render() = %q", key)
	}
	if tier, ok := compiled.MatchTier(key); !ok || tier != 14 {
		t.Fatalf("MatchTier() = %d, %v", tier, ok)
	}
	if _, ok := compiled.MatchTier("scale-out.unifabric.io/tier-01"); ok {
		t.Fatal("MatchTier() accepted a non-canonical tier")
	}
}

func TestCompileRejectsAnythingExceptOneTierAction(t *testing.T) {
	for _, raw := range []string{
		"unifabric.io/tier-1",
		"unifabric.io/{{ .Other }}",
		"unifabric.io/{{ printf \"%d\" .Tier }}",
		"unifabric.io/{{ .Tier }}-{{ .Tier }}",
		"unifabric.io/{{ if .Tier }}x{{ end }}",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := Compile("test", raw); err == nil {
				t.Fatalf("Compile(%q) succeeded", raw)
			}
		})
	}
}

func TestCompileSetRejectsOverlappingTemplates(t *testing.T) {
	_, err := CompileSet(
		"unifabric.io/tier-{{ .Tier }}",
		"unifabric.io/tier-{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err == nil {
		t.Fatal("CompileSet() accepted overlapping templates")
	}
}

func TestCompileSetRejectsCrossTierOverlap(t *testing.T) {
	_, err := CompileSet(
		"unifabric.io/tier-{{ .Tier }}",
		"unifabric.io/tier-1{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err == nil {
		t.Fatal("CompileSet() accepted templates that overlap at different tiers")
	}
}
