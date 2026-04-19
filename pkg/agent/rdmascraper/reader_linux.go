// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package rdmascraper

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readCounterValue(path string) (float64, bool) {
	valueBytes, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	valueText := strings.TrimSpace(string(valueBytes))
	if strings.Contains(valueText, "N/A (no PMA)") {
		return 0, false
	}
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil {
		return 0, false
	}
	switch filepath.Base(path) {
	case "port_xmit_data", "port_rcv_data":
		value *= 4
	}
	return value, true
}

func readFloatFile(path string) (float64, error) {
	valueBytes, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(valueBytes)), 64)
}

func readOperStateFile(path string) (float64, error) {
	valueBytes, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(string(valueBytes)) == "up" {
		return 1, nil
	}
	return 0, nil
}

func readDeviceTOS(path string) (float64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	const prefix = "Global tclass="
	valueText := strings.TrimSpace(string(content))
	if !strings.HasPrefix(valueText, prefix) {
		return 0, nil
	}
	return strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(valueText, prefix)), 64)
}
