// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func Run(check bool, args ...string) (string, error) {
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

func FetchJSON(resource string, out any) error {
	stdout, err := Run(true, "kubectl", "get", resource, "-o", "json")
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(stdout), out)
}

func Count(resource string) int {
	stdout, err := Run(false, "kubectl", "get", resource, "-o", "name")
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
