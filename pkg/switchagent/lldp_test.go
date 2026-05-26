// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchagent

import (
	"testing"

	"github.com/unifabric-io/unifabric/pkg/config"
)

func TestParseSnapshotParsesAllPorts(t *testing.T) {
	output := []byte(`{
	  "lldp": [
	    {
	      "interface": [
	        {
	          "name": "Ethernet1",
	          "chassis": [
	            {
	              "name": [
	                {
	                  "value": "spine-1"
	                }
	              ]
	            }
	          ],
	          "port": [
	            {
	              "id": [
	                {
	                  "value": "Ethernet49"
	                }
	              ]
	            }
	          ]
	        },
	        {
	          "name": "mgmt0",
	          "chassis": [
	            {
	              "name": [
	                {
	                  "value": "oob-switch"
	                }
	              ]
	            }
	          ],
	          "port": [
	            {
	              "id": [
	                {
	                  "value": "Management1"
	                }
	              ]
	            }
	          ]
	        }
	      ]
	    }
	  ]
	}`)

	snapshot, err := parseSnapshot("Leaf_1", 3, output)
	if err != nil {
		t.Fatalf("parseSnapshot returned error: %v", err)
	}

	if snapshot.GetSwitchName() != "leaf-1" {
		t.Fatalf("expected normalized switch name leaf-1, got %s", snapshot.GetSwitchName())
	}
	if snapshot.GetGeneration() != 3 {
		t.Fatalf("expected generation 3, got %d", snapshot.GetGeneration())
	}
	if len(snapshot.GetLldpNeighbors()) != 2 {
		t.Fatalf("expected 2 parsed neighbors, got %d", len(snapshot.GetLldpNeighbors()))
	}

	neighbor := snapshot.GetLldpNeighbors()[0]
	if neighbor.GetLocalDeviceName() != "leaf-1" {
		t.Fatalf("expected local device name leaf-1, got %s", neighbor.GetLocalDeviceName())
	}
	if neighbor.GetLocalPort() != "Ethernet1" {
		t.Fatalf("expected local port Ethernet1, got %s", neighbor.GetLocalPort())
	}
	if neighbor.GetRemoteSystemName() != "spine-1" {
		t.Fatalf("expected remote system name spine-1, got %s", neighbor.GetRemoteSystemName())
	}
	if neighbor.GetRemotePortId() != "Ethernet49" {
		t.Fatalf("expected remote port Ethernet49, got %s", neighbor.GetRemotePortId())
	}

	neighbor = snapshot.GetLldpNeighbors()[1]
	if neighbor.GetLocalPort() != "mgmt0" {
		t.Fatalf("expected local port mgmt0, got %s", neighbor.GetLocalPort())
	}
	if neighbor.GetRemoteSystemName() != "oob-switch" {
		t.Fatalf("expected remote system name oob-switch, got %s", neighbor.GetRemoteSystemName())
	}
	if neighbor.GetRemotePortId() != "Management1" {
		t.Fatalf("expected remote port Management1, got %s", neighbor.GetRemotePortId())
	}
}

func TestLldpCollectionOptionsUsesConfiguredSocketVersion(t *testing.T) {
	cfg := &config.SwitchAgentConfig{}
	cfg.LLDP.CollectionMode = "socket"
	cfg.LLDP.SocketPath = "/run/lldpd.socket"
	cfg.LLDP.CLIVersion = "1.0.4"

	options := lldpCollectionOptions(cfg)
	if options.BinaryPath != lldpCLI104Path {
		t.Fatalf("expected 1.0.4 binary, got %s", options.BinaryPath)
	}
	if options.SocketPath != "/run/lldpd.socket" {
		t.Fatalf("expected socket path to be propagated, got %s", options.SocketPath)
	}
	if options.FallbackToHost || options.UseHostNamespace {
		t.Fatalf("expected socket-only options, got %#v", options)
	}
}

func TestLldpCollectionOptionsDefaultsTo116Socket(t *testing.T) {
	cfg := &config.SwitchAgentConfig{}
	cfg.LLDP.CollectionMode = "socket"
	cfg.LLDP.SocketPath = "/run/lldpd.socket"
	cfg.LLDP.CLIVersion = "1.0.16"

	options := lldpCollectionOptions(cfg)
	if options.BinaryPath != lldpCLI116Path {
		t.Fatalf("expected 1.0.16 binary, got %s", options.BinaryPath)
	}
	if options.SocketPath != "/run/lldpd.socket" {
		t.Fatalf("expected socket path to be propagated, got %s", options.SocketPath)
	}
}

func TestLldpCollectionOptionsUsesHostProcMode(t *testing.T) {
	cfg := &config.SwitchAgentConfig{}
	cfg.LLDP.CollectionMode = "hostProc"
	cfg.LLDP.SocketPath = "/run/lldpd.socket"
	cfg.LLDP.CLIVersion = "1.0.4"

	options := lldpCollectionOptions(cfg)
	if !options.UseHostNamespace {
		t.Fatalf("expected host namespace mode, got %#v", options)
	}
	if options.BinaryPath != "" || options.SocketPath != "" {
		t.Fatalf("expected host namespace mode to ignore socket and binary, got %#v", options)
	}
}

func TestSnapshotsEqualIgnoresGeneration(t *testing.T) {
	previous := &LLDPNeighborSnapshot{
		SwitchName: "leaf-1",
		Generation: 1,
		LldpNeighbors: []*LLDPNeighbor{
			{LocalDeviceName: "leaf-1", LocalPort: "Ethernet1", RemoteSystemName: "spine-1", RemotePortId: "Ethernet49"},
		},
	}
	next := &LLDPNeighborSnapshot{
		SwitchName: "leaf-1",
		Generation: 2,
		LldpNeighbors: []*LLDPNeighbor{
			{LocalDeviceName: "leaf-1", LocalPort: "Ethernet1", RemoteSystemName: "spine-1", RemotePortId: "Ethernet49"},
		},
	}

	if !snapshotsEqual(previous, next) {
		t.Fatal("expected snapshots with same neighbor payload to be equal despite different generations")
	}
}

func TestSwitchNameMatchesUsesNormalizedForm(t *testing.T) {
	if !switchNameMatches("LEAF_1", "leaf-1") {
		t.Fatal("expected normalized switch names to match")
	}
	if switchNameMatches("leaf-1", "leaf-2") {
		t.Fatal("expected different switch names not to match")
	}
}

func TestSwitchNameMatchesTreatsEmptyExpectedAsWildcardAtCallSite(t *testing.T) {
	if !switchNameMatches("", "leaf-1") {
		t.Fatal("expected empty expected switch name to be accepted")
	}
}
