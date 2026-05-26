// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package fabricnode

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestResolveLldpCliInvocationUsesLocalBinaryWhenAvailable(t *testing.T) {
	originalLookup := commandLookup
	originalPathCheck := pathCheck
	t.Cleanup(func() {
		commandLookup = originalLookup
		pathCheck = originalPathCheck
	})

	commandLookup = func(name string) (string, error) {
		switch name {
		case LLDPCLI:
			return "/usr/bin/lldpcli", nil
		default:
			return "", fmt.Errorf("unexpected lookup for %s", name)
		}
	}
	pathCheck = func(path string) error {
		t.Fatalf("pathCheck should not be called when local lldpcli exists, got %s", path)
		return nil
	}

	command, args, err := resolveLldpCliInvocation(LldpCliOptions{FallbackToHost: true})
	if err != nil {
		t.Fatalf("resolveLldpCliInvocation returned error: %v", err)
	}
	if command != "/usr/bin/lldpcli" {
		t.Fatalf("expected local lldpcli path, got %s", command)
	}
	expectedArgs := []string{"-f", "json0", "show", "neighbors"}
	if len(args) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d", len(expectedArgs), len(args))
	}
	for index := range expectedArgs {
		if args[index] != expectedArgs[index] {
			t.Fatalf("expected arg %d to be %s, got %s", index, expectedArgs[index], args[index])
		}
	}
}

func TestResolveLldpCliInvocationFallsBackToNsenter(t *testing.T) {
	originalLookup := commandLookup
	originalPathCheck := pathCheck
	t.Cleanup(func() {
		commandLookup = originalLookup
		pathCheck = originalPathCheck
	})

	commandLookup = func(name string) (string, error) {
		switch name {
		case LLDPCLI:
			return "", os.ErrNotExist
		case NSENTER:
			return "/usr/bin/nsenter", nil
		default:
			return "", fmt.Errorf("unexpected lookup for %s", name)
		}
	}
	pathCheck = func(path string) error {
		switch path {
		case hostMountNSPath, hostNetNSPath:
			return nil
		default:
			return fmt.Errorf("unexpected path check for %s", path)
		}
	}

	command, args, err := resolveLldpCliInvocation(LldpCliOptions{FallbackToHost: true})
	if err != nil {
		t.Fatalf("resolveLldpCliInvocation returned error: %v", err)
	}
	if command != "/usr/bin/nsenter" {
		t.Fatalf("expected nsenter path, got %s", command)
	}
	if len(args) != 9 {
		t.Fatalf("expected 9 nsenter args, got %d", len(args))
	}
	if args[0] != "--mount="+hostMountNSPath {
		t.Fatalf("unexpected mount namespace arg: %s", args[0])
	}
	if args[1] != "--net="+hostNetNSPath {
		t.Fatalf("unexpected net namespace arg: %s", args[1])
	}
	if args[2] != "/usr/bin/env" {
		t.Fatalf("expected nsenter to execute /usr/bin/env, got %s", args[2])
	}
	expectedArgs := []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"lldpcli",
		"-f",
		"json0",
		"show",
		"neighbors",
	}
	for index := range expectedArgs {
		if args[index+3] != expectedArgs[index] {
			t.Fatalf("expected arg %d to be %s, got %s", index+3, expectedArgs[index], args[index+3])
		}
	}
}

func TestResolveLldpCliInvocationCanForceHostNamespace(t *testing.T) {
	originalLookup := commandLookup
	originalPathCheck := pathCheck
	t.Cleanup(func() {
		commandLookup = originalLookup
		pathCheck = originalPathCheck
	})

	commandLookup = func(name string) (string, error) {
		switch name {
		case NSENTER:
			return "/usr/bin/nsenter", nil
		default:
			return "", fmt.Errorf("unexpected lookup for %s", name)
		}
	}
	pathCheck = func(path string) error {
		switch path {
		case hostMountNSPath, hostNetNSPath:
			return nil
		default:
			return fmt.Errorf("unexpected path check for %s", path)
		}
	}

	command, args, err := resolveLldpCliInvocation(LldpCliOptions{UseHostNamespace: true})
	if err != nil {
		t.Fatalf("resolveLldpCliInvocation returned error: %v", err)
	}
	if command != "/usr/bin/nsenter" {
		t.Fatalf("expected nsenter path, got %s", command)
	}
	if args[0] != "--mount="+hostMountNSPath || args[1] != "--net="+hostNetNSPath {
		t.Fatalf("expected host namespace args, got %#v", args[:2])
	}
}

func TestResolveLldpCliInvocationFailsWhenNeitherPathWorks(t *testing.T) {
	originalLookup := commandLookup
	originalPathCheck := pathCheck
	t.Cleanup(func() {
		commandLookup = originalLookup
		pathCheck = originalPathCheck
	})

	commandLookup = func(name string) (string, error) {
		return "", os.ErrNotExist
	}
	pathCheck = func(path string) error {
		return nil
	}

	_, _, err := resolveLldpCliInvocation(LldpCliOptions{FallbackToHost: true})
	if err == nil {
		t.Fatal("expected resolveLldpCliInvocation to fail when lldpcli and nsenter are unavailable")
	}
}

func TestResolveLldpCliInvocationUsesConfiguredSocket(t *testing.T) {
	originalLookup := commandLookup
	originalPathCheck := pathCheck
	t.Cleanup(func() {
		commandLookup = originalLookup
		pathCheck = originalPathCheck
	})

	commandLookup = func(name string) (string, error) {
		return "", fmt.Errorf("unexpected lookup for %s", name)
	}
	pathCheck = func(path string) error {
		switch path {
		case "/usr/local/sbin/lldpcli-1.0.4", "/run/lldpd.socket":
			return nil
		default:
			return fmt.Errorf("unexpected path check for %s", path)
		}
	}

	command, args, err := resolveLldpCliInvocation(LldpCliOptions{
		BinaryPath: "/usr/local/sbin/lldpcli-1.0.4",
		SocketPath: "/run/lldpd.socket",
	})
	if err != nil {
		t.Fatalf("resolveLldpCliInvocation returned error: %v", err)
	}
	if command != "/usr/local/sbin/lldpcli-1.0.4" {
		t.Fatalf("expected configured binary path, got %s", command)
	}
	expectedArgs := []string{"-f", "json0", "-u", "/run/lldpd.socket", "show", "neighbors"}
	if len(args) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d", len(expectedArgs), len(args))
	}
	for index := range expectedArgs {
		if args[index] != expectedArgs[index] {
			t.Fatalf("expected arg %d to be %s, got %s", index, expectedArgs[index], args[index])
		}
	}
}

func TestLldpCliShowNeighborsIncludesCommandOutputOnFailure(t *testing.T) {
	originalLookup := commandLookup
	originalExecCommand := execCommand
	t.Cleanup(func() {
		commandLookup = originalLookup
		execCommand = originalExecCommand
	})

	commandLookup = func(name string) (string, error) {
		switch name {
		case LLDPCLI:
			return "/usr/bin/lldpcli", nil
		default:
			return "", fmt.Errorf("unexpected lookup for %s", name)
		}
	}
	execCommand = func(command string, args ...string) *exec.Cmd {
		return exec.Command("/bin/sh", "-lc", "printf 'permission denied\n' >&2; exit 1")
	}

	_, err := LldpCliShowNeighbors()
	if err == nil {
		t.Fatal("expected LldpCliShowNeighbors to return an error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected command output to be included in error, got %v", err)
	}
}
