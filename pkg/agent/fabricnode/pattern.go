// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package fabricnode

import (
	"regexp"
	"strings"
)

// matchWildcardPattern checks if a string matches a wildcard pattern.
// It supports * (any number of characters), ? (single character),
// and !prefix exclusion rules.
func matchWildcardPattern(s, pattern string) bool {
	if strings.HasPrefix(pattern, "!") {
		pattern = strings.TrimPrefix(pattern, "!")
		return !matchWildcardPattern(s, pattern)
	}

	regexPattern := strings.ReplaceAll(pattern, "*", ".*")
	regexPattern = strings.ReplaceAll(regexPattern, "?", ".")
	regexPattern = "^" + regexPattern + "$"

	matched, err := regexp.MatchString(regexPattern, s)
	return err == nil && matched
}

// matchMultiplePatterns checks a comma-separated pattern list with inclusion
// and exclusion rules.
func matchMultiplePatterns(s string, patternList string) bool {
	if patternList == "" {
		return false
	}

	patterns := strings.Split(patternList, ",")
	var inclusions, exclusions []string
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "!") {
			exclusions = append(exclusions, p)
		} else {
			inclusions = append(inclusions, p)
		}
	}

	if len(inclusions) == 0 && len(exclusions) == 1 {
		exclusionPattern := strings.TrimPrefix(exclusions[0], "!")
		return !matchWildcardPattern(s, exclusionPattern)
	}
	if len(inclusions) == 1 && len(exclusions) == 0 {
		return matchWildcardPattern(s, inclusions[0])
	}

	for _, p := range exclusions {
		exclusionPattern := strings.TrimPrefix(p, "!")
		if matchWildcardPattern(s, exclusionPattern) {
			return false
		}
	}

	if len(inclusions) == 0 && len(exclusions) == 0 {
		return false
	}
	if len(inclusions) == 0 {
		return true
	}

	for _, p := range inclusions {
		if matchWildcardPattern(s, p) {
			return true
		}
	}

	return false
}
