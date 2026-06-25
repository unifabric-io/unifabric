// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestServerOptions(t *testing.T) {
	opts := serverOptions(":9090")
	if opts.BindAddress != ":9090" {
		t.Fatalf("BindAddress = %q, want :9090", opts.BindAddress)
	}
	if len(opts.ExtraHandlers) != 0 {
		t.Fatalf("ExtraHandlers = %#v, want empty", opts.ExtraHandlers)
	}
}
