// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchagent

import (
	"testing"
	"time"
)

func TestGRPCKeepaliveEnforcementPolicyAllowsControllerDefault(t *testing.T) {
	policy := grpcKeepaliveEnforcementPolicy()
	if policy.MinTime > 30*time.Second {
		t.Fatalf("expected keepalive enforcement min time to allow controller default 30s, got %s", policy.MinTime)
	}
	if !policy.PermitWithoutStream {
		t.Fatal("expected keepalive enforcement to permit keepalive probes without an active stream")
	}
}
