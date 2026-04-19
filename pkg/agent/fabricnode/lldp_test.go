// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package fabricnode

import (
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/logger"
)

func TestParseLLDPNeighbors(t *testing.T) {
	log := logger.MustNew(logger.LevelDebug)
	// Test case 1: mixed mgmt-ip types (strings and arrays).
	testJSON1 := `{
  "lldp": [
    {
      "interface": [
        {
          "name": "enp129s0f0",
          "via": "LLDP",
          "rid": "1",
          "age": "0 day, 00:00:00",
          "chassis": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "58:c7:ac:5f:2d:2c"
                }
              ],
              "name": [
                {
                  "value": "DaoCloud_ShangPu_H3C_D01_AccessSwitch"
                }
              ],
              "descr": [
                {
                  "value": "H3C Comware Platform Software, Software Version 7.1.070, Release 6328P03\r\nH3C S5130S-52S-EI\r\nCopyright (c) 2004-2021 New H3C Technologies Co., Ltd. All rights reserved."
                }
              ],
              "capability": [
                {
                  "type": "Bridge",
                  "enabled": true
                },
                {
                  "type": "Router",
                  "enabled": true
                }
              ]
            }
          ],
          "port": [
            {
              "id": [
                {
                  "type": "ifname",
                  "value": "GigabitEthernet1/0/17"
                }
              ],
              "descr": [
                {
                  "value": "GigabitEthernet1/0/17 Interface"
                }
              ],
              "ttl": [
                {
                  "value": "121"
                }
              ]
            }
          ],
          "unknown-tlvs": [
            {
              "unknown-tlv": [
                {
                  "oui": "00,80,C2",
                  "subtype": "7",
                  "len": "5",
                  "value": "01,00,00,00,00"
                }
              ]
            }
          ]
        },
        {
          "name": "enp5s0f0np0",
          "via": "LLDP",
          "rid": "2",
          "age": "0 day, 00:00:00",
          "chassis": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "ac:1f:6b:25:86:64"
                }
              ],
              "name": [
                {
                  "value": "10-20-1-50"
                }
              ],
              "descr": [
                {
                  "value": "Ubuntu 24.04.2 LTS Linux 6.8.0-53-generic #55-Ubuntu SMP PREEMPT_DYNAMIC Fri Jan 17 15:37:52 UTC 2025 x86_64"
                }
              ],
              "mgmt-ip": [
                {
                  "value": "10.20.1.50"
                },
                {
                  "value": "fe80::ae1f:6bff:fe25:8664"
                }
              ],
              "mgmt-iface": [
                {
                  "value": "2"
                },
                {
                  "value": "2"
                }
              ],
              "capability": [
                {
                  "type": "Bridge",
                  "enabled": true
                },
                {
                  "type": "Router",
                  "enabled": true
                },
                {
                  "type": "Wlan",
                  "enabled": false
                },
                {
                  "type": "Station",
                  "enabled": false
                }
              ]
            }
          ],
          "port": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "e8:eb:d3:93:ae:10"
                }
              ],
              "descr": [
                {
                  "value": "enp5s0f0np0"
                }
              ],
              "ttl": [
                {
                  "value": "120"
                }
              ]
            }
          ]
        },
        {
          "name": "enp11s0f0np0",
          "via": "LLDP",
          "rid": "2",
          "age": "0 day, 00:00:00",
          "chassis": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "ac:1f:6b:25:86:64"
                }
              ],
              "name": [
                {
                  "value": "10-20-1-50"
                }
              ],
              "descr": [
                {
                  "value": "Ubuntu 24.04.2 LTS Linux 6.8.0-53-generic #55-Ubuntu SMP PREEMPT_DYNAMIC Fri Jan 17 15:37:52 UTC 2025 x86_64"
                }
              ],
              "mgmt-ip": [
                {
                  "value": "10.20.1.50"
                },
                {
                  "value": "fe80::ae1f:6bff:fe25:8664"
                }
              ],
              "mgmt-iface": [
                {
                  "value": "2"
                },
                {
                  "value": "2"
                }
              ],
              "capability": [
                {
                  "type": "Bridge",
                  "enabled": true
                },
                {
                  "type": "Router",
                  "enabled": true
                },
                {
                  "type": "Wlan",
                  "enabled": false
                },
                {
                  "type": "Station",
                  "enabled": false
                }
              ]
            }
          ],
          "port": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "b8:3f:d2:9f:09:42"
                }
              ],
              "descr": [
                {
                  "value": "enp11s0f0np0"
                }
              ],
              "ttl": [
                {
                  "value": "120"
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}`

	testJSON2 := `{
  "lldp": [
    {
      "interface": [
        {
          "name": "ens1np0",
          "via": "LLDP",
          "rid": "1",
          "age": "0 day, 01:48:20",
          "chassis": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "70:06:92:6e:32:64"
                }
              ],
              "name": [
                {
                  "value": "StorageSW"
                }
              ],
              "descr": [
                {
                  "value": "SONiC Software Version: SONiC.CuOS_4.0-0.R_X86_64_ztp - HwSku: ds730-32d - Distribution: 10.13 - Kernel: 4.19.0-12-2-amd64"
                }
              ],
              "mgmt-ip": [
                {
                  "value": "10.193.77.211"
                }
              ],
              "capability": [
                {
                  "type": "Bridge",
                  "enabled": true
                },
                {
                  "type": "Router",
                  "enabled": true
                },
                {
                  "type": "Wlan",
                  "enabled": false
                },
                {
                  "type": "Station",
                  "enabled": false
                }
              ]
            }
          ],
          "port": [
            {
              "id": [
                {
                  "type": "local",
                  "value": "Ethernet120"
                }
              ],
              "descr": [
                {
                  "value": "Ethernet120"
                }
              ],
              "ttl": [
                {
                  "value": "120"
                }
              ]
            }
          ]
        },
        {
          "name": "ens841np0",
          "via": "LLDP",
          "rid": "3",
          "age": "0 day, 01:48:15",
          "chassis": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "70:06:92:6e:32:1c"
                }
              ],
              "name": [
                {
                  "value": "LEAF03"
                }
              ],
              "descr": [
                {
                  "value": "SONiC Software Version: SONiC.CuOS_4.0-0.R_X86_64_ztp - HwSku: ds730-32d - Distribution: 10.13 - Kernel: 4.19.0-12-2-amd64"
                }
              ],
              "mgmt-ip": [
                {
                  "value": "10.193.77.203"
                }
              ],
              "capability": [
                {
                  "type": "Bridge",
                  "enabled": true
                },
                {
                  "type": "Router",
                  "enabled": true
                },
                {
                  "type": "Wlan",
                  "enabled": false
                },
                {
                  "type": "Station",
                  "enabled": false
                }
              ]
            }
          ],
          "port": [
            {
              "id": [
                {
                  "type": "local",
                  "value": "Ethernet184"
                }
              ],
              "descr": [
                {
                  "value": "Ethernet184"
                }
              ],
              "ttl": [
                {
                  "value": "120"
                }
              ]
            }
          ]
        },
        {
          "name": "ens842np0",
          "via": "LLDP",
          "rid": "2",
          "age": "0 day, 01:48:19",
          "chassis": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "70:06:92:6e:34:5c"
                }
              ],
              "name": [
                {
                  "value": "LEAF04"
                }
              ],
              "descr": [
                {
                  "value": "SONiC Software Version: SONiC.CuOS_4.0-0.R_X86_64_ztp - HwSku: ds730-32d - Distribution: 10.13 - Kernel: 4.19.0-12-2-amd64"
                }
              ],
              "mgmt-ip": [
                {
                  "value": "10.193.77.204"
                }
              ],
              "capability": [
                {
                  "type": "Bridge",
                  "enabled": true
                },
                {
                  "type": "Router",
                  "enabled": true
                },
                {
                  "type": "Wlan",
                  "enabled": false
                },
                {
                  "type": "Station",
                  "enabled": false
                }
              ]
            }
          ],
          "port": [
            {
              "id": [
                {
                  "type": "local",
                  "value": "Ethernet184"
                }
              ],
              "descr": [
                {
                  "value": "Ethernet184"
                }
              ],
              "ttl": [
                {
                  "value": "120"
                }
              ]
            }
          ]
        },
        {
          "name": "ens110np0",
          "via": "LLDP",
          "rid": "7",
          "age": "0 day, 01:25:24",
          "chassis": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "70:06:92:6e:34:80"
                }
              ],
              "name": [
                {
                  "value": "LEAF02"
                }
              ],
              "descr": [
                {
                  "value": "SONiC Software Version: SONiC.CuOS_4.0-0.R_X86_64_ztp - HwSku: ds730-32d - Distribution: 10.13 - Kernel: 4.19.0-12-2-amd64"
                }
              ],
              "mgmt-ip": [
                {
                  "value": "10.193.77.202"
                }
              ],
              "capability": [
                {
                  "type": "Bridge",
                  "enabled": true
                },
                {
                  "type": "Router",
                  "enabled": true
                },
                {
                  "type": "Wlan",
                  "enabled": false
                },
                {
                  "type": "Station",
                  "enabled": false
                }
              ]
            }
          ],
          "port": [
            {
              "id": [
                {
                  "type": "local",
                  "value": "Ethernet184"
                }
              ],
              "descr": [
                {
                  "value": "Ethernet184"
                }
              ],
              "ttl": [
                {
                  "value": "120"
                }
              ]
            }
          ]
        },
        {
          "name": "ens108np0",
          "via": "LLDP",
          "rid": "6",
          "age": "0 day, 01:25:26",
          "chassis": [
            {
              "id": [
                {
                  "type": "mac",
                  "value": "70:06:92:6e:33:18"
                }
              ],
              "name": [
                {
                  "value": "LEAF01"
                }
              ],
              "descr": [
                {
                  "value": "SONiC Software Version: SONiC.CuOS_4.0-0.R_X86_64_ztp - HwSku: ds730-32d - Distribution: 10.13 - Kernel: 4.19.0-12-2-amd64"
                }
              ],
              "mgmt-ip": [
                {
                  "value": "10.193.77.201"
                }
              ],
              "capability": [
                {
                  "type": "Bridge",
                  "enabled": true
                },
                {
                  "type": "Router",
                  "enabled": true
                },
                {
                  "type": "Wlan",
                  "enabled": false
                },
                {
                  "type": "Station",
                  "enabled": false
                }
              ]
            }
          ],
          "port": [
            {
              "id": [
                {
                  "type": "local",
                  "value": "Ethernet184"
                }
              ],
              "descr": [
                {
                  "value": "Ethernet184"
                }
              ],
              "ttl": [
                {
                  "value": "120"
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}`
	// Expected result 1: mixed mgmt-ip types.
	expected1 := map[string]v1beta1.LLDPNeighbor{
		"enp129s0f0": {
			Hostname: "DaoCloud_ShangPu_H3C_D01_AccessSwitch",
			Mac:      "58:c7:ac:5f:2d:2c",
			Port:     "GigabitEthernet1/0/17",
		},
		"enp5s0f0np0": {
			Hostname: "10-20-1-50",
			Mac:      "ac:1f:6b:25:86:64",
			Port:     "e8:eb:d3:93:ae:10",
			MgmtIP:   "10.20.1.50",
		},
		"enp11s0f0np0": {
			Hostname: "10-20-1-50",
			Mac:      "ac:1f:6b:25:86:64",
			Port:     "b8:3f:d2:9f:09:42",
			MgmtIP:   "10.20.1.50",
		},
	}

	// Expected result 2: mgmt-ip values are all strings.
	expected2 := map[string]v1beta1.LLDPNeighbor{
		"ens1np0": {
			Hostname: "StorageSW",
			Mac:      "70:06:92:6e:32:64",
			Port:     "Ethernet120",
			MgmtIP:   "10.193.77.211",
		},
		"ens841np0": {
			Hostname: "LEAF03",
			Mac:      "70:06:92:6e:32:1c",
			Port:     "Ethernet184",
			MgmtIP:   "10.193.77.203",
		},
		"ens842np0": {
			Hostname: "LEAF04",
			Mac:      "70:06:92:6e:34:5c",
			Port:     "Ethernet184",
			MgmtIP:   "10.193.77.204",
		},
		"ens110np0": {
			Hostname: "LEAF02",
			Mac:      "70:06:92:6e:34:80",
			Port:     "Ethernet184",
			MgmtIP:   "10.193.77.202",
		},
		"ens108np0": {
			Hostname: "LEAF01",
			Mac:      "70:06:92:6e:33:18",
			Port:     "Ethernet184",
			MgmtIP:   "10.193.77.201",
		},
	}

	// Test case 1.
	t.Run("Mixed mgmt-ip types", func(t *testing.T) {
		neighbors, err := ParseLLDPNeighbors(log, []byte(testJSON1))
		if err != nil {
			t.Fatalf("ParseLLDPNeighbors failed: %v", err)
		}

		// Verify the result.
		if len(neighbors) != len(expected1) {
			t.Errorf("Expected %d neighbors, got %d", len(expected1), len(neighbors))
		}

		for iface, expectedNeighbor := range expected1 {
			actualNeighbor, ok := neighbors[iface]
			if !ok {
				t.Errorf("Missing neighbor for interface %s", iface)
				continue
			}

			if actualNeighbor.Hostname != expectedNeighbor.Hostname {
				t.Errorf("Interface %s: expected hostname %s, got %s",
					iface, expectedNeighbor.Hostname, actualNeighbor.Hostname)
			}

			if actualNeighbor.Mac != expectedNeighbor.Mac {
				t.Errorf("Interface %s: expected MAC %s, got %s",
					iface, expectedNeighbor.Mac, actualNeighbor.Mac)
			}

			if actualNeighbor.Port != expectedNeighbor.Port {
				t.Errorf("Interface %s: expected port %s, got %s",
					iface, expectedNeighbor.Port, actualNeighbor.Port)
			}

			if actualNeighbor.MgmtIP != expectedNeighbor.MgmtIP {
				t.Errorf("Interface %s: expected IP %s, got %s",
					iface, expectedNeighbor.MgmtIP, actualNeighbor.MgmtIP)
			}
		}
	})

	// Test case 2.
	t.Run("String mgmt-ip types", func(t *testing.T) {
		neighbors, err := ParseLLDPNeighbors(log, []byte(testJSON2))
		if err != nil {
			t.Fatalf("ParseLLDPNeighbors failed: %v", err)
		}

		// Verify the result.
		if len(neighbors) != len(expected2) {
			t.Errorf("Expected %d neighbors, got %d", len(expected2), len(neighbors))
		}

		for iface, expectedNeighbor := range expected2 {
			actualNeighbor, ok := neighbors[iface]
			if !ok {
				t.Errorf("Missing neighbor for interface %s", iface)
				continue
			}

			if actualNeighbor.Hostname != expectedNeighbor.Hostname {
				t.Errorf("Interface %s: expected hostname %s, got %s",
					iface, expectedNeighbor.Hostname, actualNeighbor.Hostname)
			}

			if actualNeighbor.Mac != expectedNeighbor.Mac {
				t.Errorf("Interface %s: expected MAC %s, got %s",
					iface, expectedNeighbor.Mac, actualNeighbor.Mac)
			}

			if actualNeighbor.Port != expectedNeighbor.Port {
				t.Errorf("Interface %s: expected port %s, got %s",
					iface, expectedNeighbor.Port, actualNeighbor.Port)
			}

			if actualNeighbor.MgmtIP != expectedNeighbor.MgmtIP {
				t.Errorf("Interface %s: expected IP %s, got %s",
					iface, expectedNeighbor.MgmtIP, actualNeighbor.MgmtIP)
			}
		}
	})
}

// TestMatchMultiplePatterns tests the matchMultiplePatterns function with various pattern combinations
func TestMatchMultiplePatterns(t *testing.T) {
	tests := []struct {
		name          string
		interfaceName string
		patternList   string
		expected      bool
	}{
		// Single pattern tests
		{"SingleExactMatch", "eth0", "eth0", true},
		{"SingleNoMatch", "eth0", "eth1", false},
		{"SingleWildcardMatch", "eth0", "eth*", true},
		{"SingleWildcardNoMatch", "eth0", "ens*", false},
		{"SingleQuestionMatch", "eth0", "eth?", true},
		{"SingleQuestionNoMatch", "eth10", "eth?", false},
		{"SingleExclusionMatch", "eth0", "!eth1", true},
		{"SingleExclusionNoMatch", "eth0", "!eth0", false},

		// Multiple inclusion patterns
		{"MultiInclusionFirstMatch", "eth0", "eth*,ens*", true},
		{"MultiInclusionSecondMatch", "ens0", "eth*,ens*", true},
		{"MultiInclusionNoMatch", "wlan0", "eth*,ens*", false},
		{"MultiInclusionWithSpaces", "eth0", "eth*, ens*", true},

		// Multiple patterns with exclusions
		{"IncludeExcludeMatch", "eth1", "eth*,!eth0", true},
		{"IncludeExcludeNoMatch", "eth0", "eth*,!eth0", false},
		{"ExcludeOverridesInclude", "eth0", "eth0,!eth0", false},
		{"MultipleExclusions", "eth2", "eth*,!eth0,!eth1", true},
		{"MultipleExclusionsNoMatch", "eth0", "eth*,!eth0,!eth1", false},

		// Complex patterns
		{"ComplexPatternNoMatch", "eth10", "eth*,!eth*0", false},
		{"MixedWildcards", "enp1s0f1", "enp?s*,!*f2,!*f3", true},

		// Edge cases
		{"EmptyPattern", "eth0", "", false},
		{"OnlyCommas", "eth0", ",,,", false},
		{"EmptyPatterns", "eth0", ",,eth0,", true},
		{"AllExclusions", "eth0", "!eth1,!eth2", true},
		{"AllExclusionsWithMatch", "eth0", "!eth0,!eth1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchMultiplePatterns(tt.interfaceName, tt.patternList)
			if result != tt.expected {
				t.Errorf("matchMultiplePatterns(%q, %q) = %v, want %v",
					tt.interfaceName, tt.patternList, result, tt.expected)
			}
		})
	}
}
