// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologyapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("add API types to scheme: %v", err)
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&v1beta1.Topology{ObjectMeta: objectMeta("scaleout")},
			&v1beta1.Switch{ObjectMeta: objectMeta("leaf1")},
			&v1beta1.FabricNode{ObjectMeta: objectMeta("node1")},
		).
		Build()

	return NewHandler(SingleCluster(client), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestClustersRoute(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodGet, "/clusters", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /clusters returned status %d, want %d", response.Code, http.StatusOK)
	}
	var clusters []clusterItem
	decodeJSON(t, response, &clusters)
	if len(clusters) != 1 || clusters[0].Name != DefaultClusterName {
		t.Fatalf("GET /clusters returned %#v, want single cluster %q", clusters, DefaultClusterName)
	}
}

func TestResourceRoutes(t *testing.T) {
	handler := newTestHandler(t)

	tests := []struct {
		name       string
		path       string
		list       bool
		resource   string
		wantStatus int
	}{
		{name: "topology list", path: "/clusters/default/apis/unifabric.io/v1beta1/topology", list: true, resource: "scaleout", wantStatus: http.StatusOK},
		{name: "topology object", path: "/clusters/default/apis/unifabric.io/v1beta1/topology/scaleout", resource: "scaleout", wantStatus: http.StatusOK},
		{name: "switch list", path: "/clusters/default/apis/unifabric.io/v1beta1/switch", list: true, resource: "leaf1", wantStatus: http.StatusOK},
		{name: "switch object", path: "/clusters/default/apis/unifabric.io/v1beta1/switch/leaf1", resource: "leaf1", wantStatus: http.StatusOK},
		{name: "fabric node list", path: "/clusters/default/apis/unifabric.io/v1beta1/fabricnode", list: true, resource: "node1", wantStatus: http.StatusOK},
		{name: "fabric node object", path: "/clusters/default/apis/unifabric.io/v1beta1/fabricnode/node1", resource: "node1", wantStatus: http.StatusOK},
		{name: "unknown cluster", path: "/clusters/remote/apis/unifabric.io/v1beta1/topology", wantStatus: http.StatusNotFound},
		{name: "foreign group", path: "/clusters/default/apis/apps/v1/deployment", wantStatus: http.StatusNotFound},
		{name: "unknown kind", path: "/clusters/default/apis/unifabric.io/v1beta1/pod", wantStatus: http.StatusNotFound},
		{name: "list kind rejected", path: "/clusters/default/apis/unifabric.io/v1beta1/topologylist", wantStatus: http.StatusNotFound},
		{name: "unknown topology", path: "/clusters/default/apis/unifabric.io/v1beta1/topology/missing", wantStatus: http.StatusNotFound},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != testCase.wantStatus {
				t.Fatalf("GET %s returned status %d, want %d", testCase.path, response.Code, testCase.wantStatus)
			}
			if testCase.wantStatus != http.StatusOK {
				return
			}

			if testCase.list {
				var body struct {
					Items []struct {
						Metadata struct {
							Name string `json:"name"`
						} `json:"metadata"`
					} `json:"items"`
				}
				decodeJSON(t, response, &body)
				if len(body.Items) != 1 || body.Items[0].Metadata.Name != testCase.resource {
					t.Fatalf("GET %s returned items %#v, want one item named %q", testCase.path, body.Items, testCase.resource)
				}
				return
			}

			var body struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
			}
			decodeJSON(t, response, &body)
			if body.Metadata.Name != testCase.resource {
				t.Fatalf("GET %s returned resource %q, want %q", testCase.path, body.Metadata.Name, testCase.resource)
			}
		})
	}
}

func objectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name}
}

func decodeJSON(t *testing.T, response *httptest.ResponseRecorder, value any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), value); err != nil {
		t.Fatalf("decode response JSON: %v; body=%s", err, response.Body.String())
	}
}
