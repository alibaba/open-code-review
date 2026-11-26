// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// DescribeSchemaChanges distinguishes a contract edited in this change from
// an old record that had already stopped applying before the review.
func (s Snapshot) DescribeSchemaChanges(ctx context.Context, before, after Tree, checked []Checked) error {
	if before.Commit == "" {
		return nil
	}
	var changed []Fact
	for _, c := range checked {
		if c.Status == StatusChanged && c.schema() {
			changed = append(changed, c.Fact)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	base, err := before.Check(ctx, changed)
	if err != nil {
		return err
	}
	details := make(map[string]string)
	for _, c := range base {
		if c.Status != StatusVerified {
			continue
		}
		data, err := after.read(ctx, c.Source)
		if err != nil {
			return err
		}
		current, err := Constraints(data)
		if err != nil {
			return err
		}
		var constraints []string
		for _, rule := range current {
			if rule.Property == c.Property {
				constraints = append(constraints, rule.Keyword+" "+string(rule.Value))
			}
		}
		head := "no direct const/enum for this property"
		if len(constraints) != 0 {
			head = strings.Join(constraints, ", ")
		}
		details[c.ID] = fmt.Sprintf("Contract transition within this change for %q. Base %s %s; reviewed version: %s. Check affected consumers and intended migrations.", c.Property, c.Keyword, c.Value, head)
	}
	for i := range s {
		if detail, ok := details[s[i].ID]; ok {
			s[i].Claim = detail
		}
	}
	return nil
}

// RefreshSchemas retains the source-to-consumer mapping supplied to learn.
// A change to the accepted contract may establish a new value for that same
// property, even if a language model does not mention the change.
func (t Tree) RefreshSchemas(ctx context.Context, facts []Fact, scope string) ([]Fact, error) {
	var bindings []Fact
	for _, f := range facts {
		if f.schema() && (scope == "" || scope == "." || inDiscoveryScope(f.Source, scope) || inDiscoveryScope(f.Scope, scope)) {
			bindings = append(bindings, f)
		}
	}
	checked, err := t.Check(ctx, bindings)
	if err != nil {
		return nil, err
	}
	type binding struct{ source, scope, property string }
	seen := map[binding]bool{}
	var refreshed []Fact
	for _, item := range checked {
		b := binding{item.Source, item.Scope, item.Property}
		if item.Status != StatusChanged || seen[b] {
			continue
		}
		seen[b] = true
		data, err := t.read(ctx, b.source)
		if err != nil {
			return nil, err
		}
		constraints, err := Constraints(data)
		if err != nil {
			return nil, err
		}
		for _, c := range constraints {
			if c.Property == b.property {
				refreshed = append(refreshed, Fact{Source: b.source, Scope: b.scope, Revision: t.Commit, Property: c.Property, Keyword: c.Keyword, Value: c.Value})
			}
		}
	}
	return refreshed, nil
}

type PropertyCheck struct {
	Rule       string `json:"rule_id"`
	Path       string `json:"path"`
	Property   string `json:"property"`
	Outcome    string `json:"outcome"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Introduced bool   `json:"introduced,omitempty"`
}

type documentField struct {
	value      json.RawMessage
	start, end int
}

func documentFields(data []byte) map[string]documentField {
	if !json.Valid(data) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	open, err := dec.Token()
	if err != nil || open != json.Delim('{') {
		return nil
	}
	fields := map[string]documentField{}
	lastOffset, line := 0, 1
	for dec.More() {
		start := int(dec.InputOffset())
		for start < len(data) && data[start] != '"' {
			start++
		}
		key, err := dec.Token()
		if err != nil {
			return nil
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil
		}
		end := int(dec.InputOffset())
		line += bytes.Count(data[lastOffset:start], []byte{'\n'})
		startLine := line
		line += bytes.Count(data[start:end], []byte{'\n'})
		lastOffset = end
		name := key.(string)
		if _, duplicate := fields[name]; duplicate {
			fields[name] = documentField{}
		} else {
			fields[name] = documentField{v, startLine, line}
		}
	}
	return fields
}

// CheckDocument evaluates only an explicit document-to-schema mapping. A
// directory scope can contain schemas, examples and source files side by side.
func (s Snapshot) CheckDocument(path string, data []byte) []PropertyCheck {
	if len(data) > maxEvidenceBytes {
		return nil
	}
	document := documentFields(data)
	if document == nil {
		return nil
	}
	var checks []PropertyCheck
	for _, f := range s {
		if !f.CurrentSchema() || f.Scope != path || f.Source == path {
			continue
		}
		field, present := document[f.Property]
		if !present {
			// Property constraints do not imply "required".
			continue
		}
		if field.start == 0 {
			checks = append(checks, PropertyCheck{Rule: f.ID, Path: path, Property: f.Property, Outcome: "uncertain"})
			continue
		}
		var allowed []json.RawMessage
		if f.Keyword == "enum" {
			if err := json.Unmarshal(f.Value, &allowed); err != nil {
				continue
			}
		} else {
			allowed = []json.RawMessage{f.Value}
		}
		outcome := "violated"
		for _, option := range allowed {
			equal, comparable := equalJSONLiteral(field.value, option)
			if equal {
				outcome = "satisfied"
				break
			}
			if !comparable {
				outcome = "uncertain"
			}
		}
		checks = append(checks, PropertyCheck{Rule: f.ID, Path: path, Property: f.Property, Outcome: outcome, StartLine: field.start, EndLine: field.end})
	}
	return checks
}

func equalJSONLiteral(a, b json.RawMessage) (bool, bool) {
	decode := func(input json.RawMessage) (any, bool) {
		dec := json.NewDecoder(bytes.NewReader(input))
		dec.UseNumber()
		var v any
		err := dec.Decode(&v)
		return v, err == nil
	}
	left, ok1 := decode(a)
	right, ok2 := decode(b)
	if !ok1 || !ok2 {
		return false, false
	}
	switch v := left.(type) {
	case json.Number:
		other, ok := right.(json.Number)
		if !ok {
			return false, true
		}
		x, y := scalarNumber(v), scalarNumber(other)
		if x == nil || y == nil {
			return false, false
		}
		return x.Cmp(y) == 0, true
	case string:
		other, ok := right.(string)
		return ok && v == other, true
	case bool:
		other, ok := right.(bool)
		return ok && v == other, true
	case nil:
		return right == nil, true
	default:
		// This extractor only admits scalar allowed values.
		return false, true
	}
}

func scalarNumber(n json.Number) *big.Rat {
	s := n.String()
	if len(s) > 2048 {
		return nil
	}
	if idx := strings.IndexAny(s, "eE"); idx >= 0 {
		exp, err := strconv.Atoi(s[idx+1:])
		if err != nil || exp < -2048 || exp > 2048 {
			return nil
		}
	}
	value, _ := new(big.Rat).SetString(s)
	return value
}
