// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologylabel

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"

	"k8s.io/apimachinery/pkg/util/validation"
)

const tierExpression = `([1-9][0-9]*)`

type TierData struct {
	Tier int
}

// Template is a parsed and validated topology label key template. The same
// syntax tree is used for forward rendering and reverse tier matching.
type Template struct {
	raw     string
	parsed  *template.Template
	matcher *regexp.Regexp
	prefix  string
	suffix  string
}

type Set struct {
	ScaleUp  *Template
	ScaleOut *Template
	Storage  *Template
}

func Compile(name, raw string) (*Template, error) {
	if raw == "" {
		return nil, fmt.Errorf("%s: topology label template must not be empty", name)
	}

	parsed, err := template.New(name).Option("missingkey=error").Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: parse topology label template: %w", name, err)
	}
	if parsed.Tree == nil || parsed.Tree.Root == nil {
		return nil, fmt.Errorf("%s: topology label template has no syntax tree", name)
	}

	var matcher strings.Builder
	var prefix strings.Builder
	var suffix strings.Builder
	matcher.WriteString("^")
	actions := 0
	actionSeen := false
	for _, node := range parsed.Tree.Root.Nodes {
		switch typed := node.(type) {
		case *parse.TextNode:
			matcher.WriteString(regexp.QuoteMeta(string(typed.Text)))
			if actionSeen {
				suffix.Write(typed.Text)
			} else {
				prefix.Write(typed.Text)
			}
		case *parse.ActionNode:
			actions++
			if err := validateTierAction(typed); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			matcher.WriteString(tierExpression)
			actionSeen = true
		default:
			return nil, fmt.Errorf("%s: only fixed text and one {{ .Tier }} action are allowed", name)
		}
	}
	if actions != 1 {
		return nil, fmt.Errorf("%s: template must contain exactly one {{ .Tier }} action", name)
	}
	matcher.WriteString("$")

	compiled := &Template{
		raw:     raw,
		parsed:  parsed,
		matcher: regexp.MustCompile(matcher.String()),
		prefix:  prefix.String(),
		suffix:  suffix.String(),
	}
	for _, tier := range []int{1, 12} {
		key, err := compiled.Render(tier)
		if err != nil {
			return nil, fmt.Errorf("%s: render tier %d: %w", name, tier, err)
		}
		if problems := validation.IsQualifiedName(key); len(problems) != 0 {
			return nil, fmt.Errorf("%s: rendered tier %d label key %q is invalid: %s", name, tier, key, strings.Join(problems, ", "))
		}
	}

	return compiled, nil
}

func CompileSet(scaleUp, scaleOut, storage string) (*Set, error) {
	values := []struct {
		name string
		raw  string
	}{
		{name: "topologyLabels.scaleUp", raw: scaleUp},
		{name: "topologyLabels.scaleOut", raw: scaleOut},
		{name: "topologyLabels.storage", raw: storage},
	}
	compiled := make([]*Template, 0, len(values))
	for _, value := range values {
		item, err := Compile(value.name, value.raw)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, item)
	}

	for left := range compiled {
		for right := left + 1; right < len(compiled); right++ {
			if example, overlaps := intersectionExample(compiled[left], compiled[right]); overlaps {
				return nil, fmt.Errorf("%s and %s generate overlapping label keys (for example %q)", values[left].name, values[right].name, example)
			}
		}
	}

	return &Set{ScaleUp: compiled[0], ScaleOut: compiled[1], Storage: compiled[2]}, nil
}

// tierLanguage is an epsilon-NFA for prefix + [1-9][0-9]* + suffix. It lets
// CompileSet prove that two configured template languages are disjoint rather
// than relying on a finite set of representative tiers.
type tierLanguage struct {
	prefix      []byte
	suffix      []byte
	digitState  int
	suffixStart int
	acceptState int
}

func newTierLanguage(value *Template) tierLanguage {
	digitState := len(value.prefix) + 1
	suffixStart := digitState + 1
	return tierLanguage{
		prefix:      []byte(value.prefix),
		suffix:      []byte(value.suffix),
		digitState:  digitState,
		suffixStart: suffixStart,
		acceptState: suffixStart + len(value.suffix),
	}
}

func (n tierLanguage) closure(states map[int]struct{}) map[int]struct{} {
	result := cloneStateSet(states)
	if _, ok := result[n.digitState]; ok {
		result[n.suffixStart] = struct{}{}
	}
	return result
}

func (n tierLanguage) transition(states map[int]struct{}, character byte) map[int]struct{} {
	next := map[int]struct{}{}
	for state := range states {
		switch {
		case state < len(n.prefix):
			if n.prefix[state] == character {
				next[state+1] = struct{}{}
			}
		case state == len(n.prefix):
			if character >= '1' && character <= '9' {
				next[n.digitState] = struct{}{}
			}
		case state == n.digitState:
			if character >= '0' && character <= '9' {
				next[n.digitState] = struct{}{}
			}
		case state >= n.suffixStart && state < n.acceptState:
			if n.suffix[state-n.suffixStart] == character {
				next[state+1] = struct{}{}
			}
		}
	}
	return n.closure(next)
}

func (n tierLanguage) accepts(states map[int]struct{}) bool {
	_, ok := states[n.acceptState]
	return ok
}

type languagePairState struct {
	left    map[int]struct{}
	right   map[int]struct{}
	example string
}

func intersectionExample(left, right *Template) (string, bool) {
	leftNFA := newTierLanguage(left)
	rightNFA := newTierLanguage(right)
	start := languagePairState{
		left:  leftNFA.closure(map[int]struct{}{0: {}}),
		right: rightNFA.closure(map[int]struct{}{0: {}}),
	}
	queue := []languagePairState{start}
	visited := map[string]struct{}{pairStateKey(start.left, start.right): {}}
	alphabet := languageAlphabet(leftNFA, rightNFA)
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if leftNFA.accepts(current.left) && rightNFA.accepts(current.right) {
			return current.example, true
		}
		for _, character := range alphabet {
			nextLeft := leftNFA.transition(current.left, character)
			nextRight := rightNFA.transition(current.right, character)
			if len(nextLeft) == 0 || len(nextRight) == 0 {
				continue
			}
			key := pairStateKey(nextLeft, nextRight)
			if _, seen := visited[key]; seen {
				continue
			}
			visited[key] = struct{}{}
			queue = append(queue, languagePairState{left: nextLeft, right: nextRight, example: current.example + string(character)})
		}
	}
	return "", false
}

func languageAlphabet(values ...tierLanguage) []byte {
	set := map[byte]struct{}{}
	for character := byte('0'); character <= '9'; character++ {
		set[character] = struct{}{}
	}
	for _, value := range values {
		for _, character := range append(append([]byte(nil), value.prefix...), value.suffix...) {
			set[character] = struct{}{}
		}
	}
	result := make([]byte, 0, len(set))
	for character := range set {
		result = append(result, character)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func pairStateKey(left, right map[int]struct{}) string {
	return stateSetKey(left) + "|" + stateSetKey(right)
}

func stateSetKey(states map[int]struct{}) string {
	values := make([]int, 0, len(states))
	for state := range states {
		values = append(values, state)
	}
	sort.Ints(values)
	var result strings.Builder
	for _, state := range values {
		result.WriteString(strconv.Itoa(state))
		result.WriteByte(',')
	}
	return result.String()
}

func cloneStateSet(states map[int]struct{}) map[int]struct{} {
	result := make(map[int]struct{}, len(states)+1)
	for state := range states {
		result[state] = struct{}{}
	}
	return result
}

func validateTierAction(action *parse.ActionNode) error {
	if action.Pipe == nil || len(action.Pipe.Decl) != 0 || action.Pipe.IsAssign || len(action.Pipe.Cmds) != 1 {
		return fmt.Errorf("the only allowed action is {{ .Tier }}")
	}
	command := action.Pipe.Cmds[0]
	if command == nil || len(command.Args) != 1 {
		return fmt.Errorf("the .Tier pipeline cannot contain functions or additional arguments")
	}
	field, ok := command.Args[0].(*parse.FieldNode)
	if !ok || len(field.Ident) != 1 || field.Ident[0] != "Tier" {
		return fmt.Errorf("the only allowed field is .Tier")
	}
	return nil
}

func (t *Template) Raw() string {
	if t == nil {
		return ""
	}
	return t.raw
}

func (t *Template) Render(tier int) (string, error) {
	if t == nil || t.parsed == nil {
		return "", fmt.Errorf("topology label template is not compiled")
	}
	if tier < 1 {
		return "", fmt.Errorf("tier must be greater than or equal to 1")
	}
	var rendered bytes.Buffer
	if err := t.parsed.Execute(&rendered, TierData{Tier: tier}); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func (t *Template) MatchTier(key string) (int, bool) {
	if t == nil || t.matcher == nil {
		return 0, false
	}
	match := t.matcher.FindStringSubmatch(key)
	if len(match) != 2 {
		return 0, false
	}
	tier, err := strconv.Atoi(match[1])
	if err != nil || tier < 1 {
		return 0, false
	}
	return tier, true
}

func (s *Set) ForTopology(name string) *Template {
	if s == nil {
		return nil
	}
	switch name {
	case "scaleup":
		return s.ScaleUp
	case "scaleout":
		return s.ScaleOut
	case "storage":
		return s.Storage
	default:
		return nil
	}
}

func (s *Set) All() []*Template {
	if s == nil {
		return nil
	}
	return []*Template{s.ScaleUp, s.ScaleOut, s.Storage}
}
