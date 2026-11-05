// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package check

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/unifabric-io/unifabric/e2e/kube"
)

// SFlowPipeline confirms the sFlow data path end to end: the mock
// sender Job emits sFlow datagrams carrying the macvlan test pod IPs, the
// collector decodes them and attributes the flows to the pods via the pod IP
// cache, and the enriched documents land in the Elasticsearch data stream.
func SFlowPipeline() (bool, string) {
	stdout, err := kube.Run(false, "kubectl", "-n", "unifabric-system", "get", "pod",
		"-l", "app.kubernetes.io/component=unifabric-elasticsearch",
		"-o", "jsonpath={.items[0].metadata.name}")
	pod := strings.TrimSpace(stdout)
	if err != nil || pod == "" {
		return false, "elasticsearch pod not found"
	}

	countDocs := func(query string) (int, string, bool) {
		cmd := `curl -fsS -G -u "elastic:${ELASTIC_PASSWORD}" localhost:9200/unifabric-switch-sflow/_count`
		if query != "" {
			cmd += ` --data-urlencode "q=` + query + `"`
		}
		out, err := kube.Run(false, "kubectl", "-n", "unifabric-system", "exec", pod, "-c", "elasticsearch", "--",
			"sh", "-c", cmd)
		if err != nil {
			return 0, fmt.Sprintf("query sflow data stream failed: %s", strings.TrimSpace(out)), false
		}
		var result struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			return 0, fmt.Sprintf("parse count response: %v", err), false
		}
		return result.Count, "", true
	}

	total, detail, ok := countDocs("")
	if !ok {
		return false, detail
	}
	if total == 0 {
		return false, "unifabric-switch-sflow data stream has no documents yet"
	}

	attributed, detail, ok := countDocs(
		"src_pod_top_owner_name:pd-prefill AND dst_pod_top_owner_name:pd-decoder")
	if !ok {
		return false, detail
	}
	if attributed == 0 {
		return false, fmt.Sprintf(
			"%d documents stored, but none attributed to pd-prefill -> pd-decoder yet", total)
	}

	detail = fmt.Sprintf("elasticsearch received %d documents (%d attributed to pd-prefill -> pd-decoder)",
		total, attributed)
	if sent, ok := mockSenderSentFlows(); ok {
		detail = fmt.Sprintf("mock sender sent %d flow samples, %s", sent, detail)
	}
	return true, detail
}

// mockSenderSentFlows parses the sflow-mock-sender Job logs for the number of
// flow samples the mock sender emitted.
func mockSenderSentFlows() (int, bool) {
	stdout, err := kube.Run(false, "kubectl", "-n", "default",
		"logs", "job/sflow-mock-sender", "--tail", "5")
	if err != nil {
		return 0, false
	}
	m := regexp.MustCompile(`done, sent \d+ datagrams, (\d+) flow samples`).FindStringSubmatch(stdout)
	if m == nil {
		return 0, false
	}
	sent, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return sent, true
}
