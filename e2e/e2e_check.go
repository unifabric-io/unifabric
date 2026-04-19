// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
)

var (
	useColor  = stdoutIsTerminal() || forceColorEnabled()
	ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

type config struct {
	timeoutMinutes             int
	sleepSeconds               int
	expectedFabricNodes        int
	expectedScaleoutLeafGroups int
	topologyDir                string
}

type row []string

type checkFunc func() (bool, string)

type netplanFile struct {
	Network struct {
		Ethernets map[string]struct {
			Addresses []string `yaml:"addresses"`
		} `yaml:"ethernets"`
	} `yaml:"network"`
}

type expectedInterfaces struct {
	gpu     map[string]string
	storage map[string]string
}

type fabricNodeList struct {
	Items []fabricNode `json:"items"`
}

type fabricNode struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		ScaleOutNics []nicInfo `json:"scaleOutNics"`
		StorageNics  []nicInfo `json:"storageNics"`
	} `json:"status"`
}

type nicInfo struct {
	Name string `json:"name"`
	IPv4 string `json:"ipv4"`
}

type leafGroupList struct {
	Items []leafGroup `json:"items"`
}

type leafGroup struct {
	Status struct {
		Nodes    []namedResource `json:"nodes"`
		Switches []namedResource `json:"switches"`
	} `json:"status"`
}

type namedResource struct {
	Name string `json:"name"`
}

func main() {
	os.Exit(run())
}

func run() int {
	cfg := parseFlags()
	deadline := time.Now().Add(time.Duration(cfg.timeoutMinutes) * time.Minute)
	rows := []row{}

	if err := runChecks(cfg, deadline, &rows); err != nil {
		rows = append(rows, row{"overall", "all stages pass", err.Error(), "FAIL"})
		fmt.Println(renderTable(row{"Check", "Expected", "Actual", "Result"}, rows))
		fmt.Println(colorize("E2E checks failed.", colorRed))
		return 1
	}

	fmt.Println(renderTable(row{"Check", "Expected", "Actual", "Result"}, rows))
	fmt.Println(colorize("E2E checks passed.", colorGreen))
	return 0
}

func parseFlags() config {
	cfg := config{}
	flag.IntVar(&cfg.timeoutMinutes, "timeout-minutes", 30, "timeout minutes for convergence checks")
	flag.IntVar(&cfg.sleepSeconds, "sleep-seconds", 20, "sleep seconds between check attempts")
	flag.IntVar(&cfg.expectedFabricNodes, "expected-fabricnodes", 4, "expected FabricNode resource count")
	flag.IntVar(&cfg.expectedScaleoutLeafGroups, "expected-scaleoutleafgroups", 2, "expected ScaleOutLeafGroup resource count")
	flag.StringVar(&cfg.topologyDir, "topology-dir", "e2e/topology", "topology directory containing node-gpu-*.yaml")
	flag.Parse()
	return cfg
}

func runChecks(cfg config, deadline time.Time, rows *[]row) error {
	stageLog("wait_fabricnodes", colorize("start", colorBold))
	actualFabricNodes, err := waitForExpectedCount(
		"wait_fabricnodes",
		"fabricnodes.unifabric.io",
		cfg.expectedFabricNodes,
		deadline,
		time.Duration(cfg.sleepSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, row{"fabricnodes count", strconv.Itoa(cfg.expectedFabricNodes), strconv.Itoa(actualFabricNodes), "PASS"})

	stageLog("wait_scaleoutleafgroups", colorize("start", colorBold))
	actualScaleout, err := waitForExpectedCount(
		"wait_scaleoutleafgroups",
		"scaleoutleafgroups.unifabric.io",
		cfg.expectedScaleoutLeafGroups,
		deadline,
		time.Duration(cfg.sleepSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, row{"scaleoutleafgroups count", strconv.Itoa(cfg.expectedScaleoutLeafGroups), strconv.Itoa(actualScaleout), "PASS"})

	stageLog("check_fabricnode_interfaces", colorize("start", colorBold))
	nicDetail, err := waitForCheckPass(
		"check_fabricnode_interfaces",
		func() (bool, string) {
			return validateFabricNodeInterfaces(cfg.topologyDir)
		},
		deadline,
		time.Duration(cfg.sleepSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, row{"fabricnode interfaces", "match topology yaml", nicDetail, "PASS"})

	stageLog("check_leaf_group_membership", colorize("start", colorBold))
	groupDetail, err := waitForCheckPass(
		"check_leaf_group_membership",
		validateLeafGroupMembership,
		deadline,
		time.Duration(cfg.sleepSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, row{
		"leaf group membership",
		"{gpu1,gpu2}<->{leaf1,leaf2}; {gpu3,gpu4}<->{leaf3,leaf4}",
		groupDetail,
		"PASS",
	})

	return nil
}

func colorize(text, color string) string {
	if !useColor {
		return text
	}
	return color + text + colorReset
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func forceColorEnabled() bool {
	switch strings.ToLower(os.Getenv("FORCE_COLOR")) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func runCommand(check bool, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := exec.Command(args[0], args[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if check && err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = fmt.Sprintf("command failed: %s", strings.Join(args, " "))
		}
		return stdout.String(), fmt.Errorf("%s", msg)
	}

	return stdout.String(), err
}

func fetchResourceJSON(resource string, out any) error {
	stdout, err := runCommand(true, "kubectl", "get", resource, "-o", "json")
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(stdout), out)
}

func getResourceCount(resource string) int {
	stdout, err := runCommand(false, "kubectl", "get", resource, "-o", "name")
	if err != nil {
		return 0
	}

	count := 0
	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func stageLog(stage, message string) {
	fmt.Printf("[%s] %s\n", colorize(stage, colorCyan), message)
}

func renderTable(headers row, rows []row) string {
	renderedRows := make([]row, 0, len(rows))
	for _, input := range rows {
		copied := append(row(nil), input...)
		if len(copied) >= 4 {
			switch copied[3] {
			case "PASS":
				copied[3] = colorize("PASS", colorGreen)
			case "FAIL":
				copied[3] = colorize("FAIL", colorRed)
			}
		}
		renderedRows = append(renderedRows, copied)
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = visibleLen(h)
	}
	for _, r := range renderedRows {
		for i, cell := range r {
			if i >= len(widths) {
				break
			}
			widths[i] = max(widths[i], visibleLen(cell))
		}
	}

	parts := []string{renderTableRow(headers, widths), renderSeparator(widths)}
	for _, r := range renderedRows {
		parts = append(parts, renderTableRow(r, widths))
	}
	return strings.Join(parts, "\n")
}

func renderTableRow(r row, widths []int) string {
	cells := make([]string, len(widths))
	for i := range widths {
		cell := ""
		if i < len(r) {
			cell = r[i]
		}
		padding := widths[i] - visibleLen(cell)
		if padding < 0 {
			padding = 0
		}
		cells[i] = cell + strings.Repeat(" ", padding)
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

func renderSeparator(widths []int) string {
	parts := make([]string, len(widths))
	for i, width := range widths {
		parts[i] = strings.Repeat("-", width)
	}
	return "|-" + strings.Join(parts, "-|-") + "-|"
}

func visibleLen(s string) int {
	return len(ansiCodes.ReplaceAllString(s, ""))
}

func waitForExpectedCount(stage, resource string, expected int, deadline time.Time, interval time.Duration) (int, error) {
	attempt := 0
	for {
		attempt++
		actual := getResourceCount(resource)
		stageLog(
			stage,
			fmt.Sprintf(
				"attempt=%d expected=%s actual=%s",
				attempt,
				colorize(strconv.Itoa(expected), colorYellow),
				colorize(strconv.Itoa(actual), colorBlue),
			),
		)
		if actual == expected {
			stageLog(stage, colorize("count reached expected value", colorGreen))
			return actual, nil
		}
		if time.Now().After(deadline) || time.Now().Equal(deadline) {
			return 0, fmt.Errorf("timeout waiting for %s: expected %d, got %d", resource, expected, actual)
		}
		time.Sleep(interval)
	}
}

func waitForCheckPass(stage string, checker checkFunc, deadline time.Time, interval time.Duration) (string, error) {
	attempt := 0
	for {
		attempt++
		ok, message := checker()
		okText := colorize("false", colorRed)
		if ok {
			okText = colorize("true", colorGreen)
		}
		stageLog(stage, fmt.Sprintf("attempt=%d ok=%s detail=%s", attempt, okText, message))
		if ok {
			return message, nil
		}
		if time.Now().After(deadline) || time.Now().Equal(deadline) {
			return "", fmt.Errorf("timeout in %s: %s", stage, message)
		}
		time.Sleep(interval)
	}
}

func loadExpectedInterfaces(topologyDir string) (map[string]expectedInterfaces, error) {
	paths, err := filepath.Glob(filepath.Join(topologyDir, "node-gpu-*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	expectedGPUIfaces := map[string]struct{}{}
	for i := 1; i <= 8; i++ {
		expectedGPUIfaces[fmt.Sprintf("eth%d", i)] = struct{}{}
	}
	expectedStorageIfaces := map[string]struct{}{"eth9": {}}

	expected := make(map[string]expectedInterfaces, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		var data netplanFile
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, err
		}

		roles := expectedInterfaces{
			gpu:     map[string]string{},
			storage: map[string]string{},
		}
		for iface, cfg := range data.Network.Ethernets {
			if len(cfg.Addresses) == 0 {
				continue
			}
			if _, ok := expectedGPUIfaces[iface]; ok {
				roles.gpu[iface] = cfg.Addresses[0]
				continue
			}
			if _, ok := expectedStorageIfaces[iface]; ok {
				roles.storage[iface] = cfg.Addresses[0]
			}
		}

		expected[strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))] = roles
	}
	return expected, nil
}

func validateFabricNodeInterfaces(topologyDir string) (bool, string) {
	expectedByNode, err := loadExpectedInterfaces(topologyDir)
	if err != nil {
		return false, fmt.Sprintf("failed to load topology interfaces: %v", err)
	}
	if len(expectedByNode) == 0 {
		return false, fmt.Sprintf("no node-gpu-*.yaml found under %s", topologyDir)
	}

	var nodes fabricNodeList
	if err := fetchResourceJSON("fabricnodes.unifabric.io", &nodes); err != nil {
		return false, fmt.Sprintf("failed to read fabricnodes: %v", err)
	}

	actualGPUByNode := map[string]map[string]string{}
	actualStorageByNode := map[string]map[string]string{}
	for _, item := range nodes.Items {
		name := item.Metadata.Name
		if !strings.HasPrefix(name, "node-gpu-") {
			continue
		}

		gpuIfaces := map[string]string{}
		storageIfaces := map[string]string{}
		for _, nic := range item.Status.ScaleOutNics {
			if nic.Name != "" {
				gpuIfaces[nic.Name] = nic.IPv4
			}
		}
		for _, nic := range item.Status.StorageNics {
			if nic.Name != "" {
				storageIfaces[nic.Name] = nic.IPv4
			}
		}
		actualGPUByNode[name] = gpuIfaces
		actualStorageByNode[name] = storageIfaces
	}

	errs := []string{}
	for nodeName, expectedRoles := range expectedByNode {
		actualGPUIfaces, okGPU := actualGPUByNode[nodeName]
		actualStorageIfaces, okStorage := actualStorageByNode[nodeName]
		if !okGPU || !okStorage {
			errs = append(errs, fmt.Sprintf("%s: FabricNode missing", nodeName))
			continue
		}

		for iface, expectedIP := range expectedRoles.gpu {
			actualIP, ok := actualGPUIfaces[iface]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: missing gpu iface %s", nodeName, iface))
				continue
			}
			if actualIP != "" && actualIP != expectedIP {
				errs = append(errs, fmt.Sprintf("%s:gpu:%s expected %s, got %s", nodeName, iface, expectedIP, actualIP))
			}
		}

		for iface, expectedIP := range expectedRoles.storage {
			actualIP, ok := actualStorageIfaces[iface]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: missing storage iface %s", nodeName, iface))
				continue
			}
			if actualIP != "" && actualIP != expectedIP {
				errs = append(errs, fmt.Sprintf("%s:storage:%s expected %s, got %s", nodeName, iface, expectedIP, actualIP))
			}
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return false, strings.Join(errs, "; ")
	}
	return true, fmt.Sprintf("validated gpu eth1-eth8 and storage eth9 for %d GPU nodes", len(expectedByNode))
}

func validateLeafGroupMembership() (bool, string) {
	expected := map[string]struct{}{
		groupKey([]string{"node-gpu-1", "node-gpu-2"}, []string{"switch-gpu-leaf1", "switch-gpu-leaf2"}): {},
		groupKey([]string{"node-gpu-3", "node-gpu-4"}, []string{"switch-gpu-leaf3", "switch-gpu-leaf4"}): {},
	}

	var groups leafGroupList
	if err := fetchResourceJSON("scaleoutleafgroups.unifabric.io", &groups); err != nil {
		return false, fmt.Sprintf("failed to read scaleoutleafgroups: %v", err)
	}

	actual := map[string]struct{}{}
	for _, group := range groups.Items {
		actual[groupKey(resourceNames(group.Status.Nodes), resourceNames(group.Status.Switches))] = struct{}{}
	}

	if !stringSetEqual(actual, expected) {
		actualGroups := make([]string, 0, len(actual))
		for key := range actual {
			actualGroups = append(actualGroups, formatGroupKey(key))
		}
		sort.Strings(actualGroups)
		return false, fmt.Sprintf("group membership mismatch: actual=%v", actualGroups)
	}
	return true, "leaf group membership matches expected topology"
}

func resourceNames(resources []namedResource) []string {
	names := make([]string, 0, len(resources))
	for _, resource := range resources {
		if resource.Name != "" {
			names = append(names, resource.Name)
		}
	}
	return names
}

func groupKey(nodes, switches []string) string {
	return strings.Join(sortedUnique(nodes), ",") + "|" + strings.Join(sortedUnique(switches), ",")
}

func formatGroupKey(key string) string {
	parts := strings.SplitN(key, "|", 2)
	if len(parts) != 2 {
		return key
	}
	return fmt.Sprintf("nodes={%s} switches={%s}", parts[0], parts[1])
}

func sortedUnique(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		if value != "" {
			seen[value] = struct{}{}
		}
	}

	unique := make([]string, 0, len(seen))
	for value := range seen {
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

func stringSetEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if _, ok := b[key]; !ok {
			return false
		}
	}
	return true
}
