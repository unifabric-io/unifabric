// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[36m"
)

var (
	useColor  = stdoutIsTerminal() || forceColorEnabled()
	ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

func Colorize(text, color string) string {
	if !useColor {
		return text
	}
	return color + text + Reset
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

func StageLog(stage, message string) {
	fmt.Printf("[%s] %s\n", Colorize(stage, Cyan), message)
}

func RenderTable(headers Row, rows []Row) string {
	renderedRows := make([]Row, 0, len(rows))
	for _, input := range rows {
		copied := append(Row(nil), input...)
		if len(copied) >= 4 {
			switch copied[3] {
			case "PASS":
				copied[3] = Colorize("PASS", Green)
			case "FAIL":
				copied[3] = Colorize("FAIL", Red)
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

func renderTableRow(r Row, widths []int) string {
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

type Row []string
