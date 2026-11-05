// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package wait

import (
	"fmt"
	"strconv"
	"time"

	"github.com/unifabric-io/unifabric/e2e/kube"
	"github.com/unifabric-io/unifabric/e2e/report"
)

type Check func() (bool, string)

func ForCount(stage, resource string, expected int, deadline time.Time, interval time.Duration) (int, error) {
	attempt := 0
	for {
		attempt++
		actual := kube.Count(resource)
		report.StageLog(
			stage,
			fmt.Sprintf(
				"attempt=%d expected=%s actual=%s",
				attempt,
				report.Colorize(strconv.Itoa(expected), report.Yellow),
				report.Colorize(strconv.Itoa(actual), report.Blue),
			),
		)
		if actual == expected {
			report.StageLog(stage, report.Colorize("count reached expected value", report.Green))
			return actual, nil
		}
		if time.Now().After(deadline) || time.Now().Equal(deadline) {
			return 0, fmt.Errorf("timeout waiting for %s: expected %d, got %d", resource, expected, actual)
		}
		time.Sleep(interval)
	}
}

func ForCheck(stage string, checker Check, deadline time.Time, interval time.Duration) (string, error) {
	attempt := 0
	for {
		attempt++
		ok, message := checker()
		okText := report.Colorize("false", report.Red)
		if ok {
			okText = report.Colorize("true", report.Green)
		}
		report.StageLog(stage, fmt.Sprintf("attempt=%d ok=%s detail=%s", attempt, okText, message))
		if ok {
			return message, nil
		}
		if time.Now().After(deadline) || time.Now().Equal(deadline) {
			return "", fmt.Errorf("timeout in %s: %s", stage, message)
		}
		time.Sleep(interval)
	}
}
