// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"reflect"
	"testing"
)

func TestNetworkStatusIPs(t *testing.T) {
	raw := `[
		{"name":"spiderpool/sriov-net1","ips":["172.18.1.2","2001:db8::2"]},
		{"name":"other/net","ips":["172.18.1.2/24","not-an-ip"]}
	]`
	want := []string{"172.18.1.2", "2001:db8::2"}
	if got := networkStatusIPs(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("networkStatusIPs() = %v, want %v", got, want)
	}
}

func TestNetworkStatusIPsSingleObject(t *testing.T) {
	raw := `{"name":"spiderpool/sriov-net1","ips":["172.18.1.3"]}`
	want := []string{"172.18.1.3"}
	if got := networkStatusIPs(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("networkStatusIPs() = %v, want %v", got, want)
	}
}

func TestNetworkStatusIPsInvalid(t *testing.T) {
	if got := networkStatusIPs(`not-json`); got != nil {
		t.Fatalf("networkStatusIPs() = %v, want nil", got)
	}
}

func TestPodAnnotationIPs(t *testing.T) {
	annotations := map[string]string{
		calicoPodIPAnnotation:  "10.233.68.236/32",
		calicoPodIPsAnnotation: `["10.233.68.236/32"]`,
		networkStatusAnnotation: `[
			{"name":"spiderpool/calico","ips":["10.233.68.236"],"default":true},
			{"name":"spiderpool/sriov-net1","interface":"net1","ips":["172.18.1.3"]}
		]`,
	}
	want := []string{"10.233.68.236", "172.18.1.3"}
	if got := podAnnotationIPs(annotations); !reflect.DeepEqual(got, want) {
		t.Fatalf("podAnnotationIPs() = %v, want %v", got, want)
	}
}

func TestAnnotationIPsFormats(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "CIDR", raw: "10.233.68.236/32", want: []string{"10.233.68.236"}},
		{name: "JSON array", raw: `["10.233.68.236/32","2001:db8::3/128"]`, want: []string{"10.233.68.236", "2001:db8::3"}},
		{name: "comma separated", raw: "10.233.68.236/32, 172.18.1.3", want: []string{"10.233.68.236", "172.18.1.3"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := annotationIPs(test.raw); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("annotationIPs() = %v, want %v", got, test.want)
			}
		})
	}
}
