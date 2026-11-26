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

// DescribeReviewChanges compares both ends of the review. A rule that stopped
// applying before the base cannot identify a contract changed by this patch.
func (s Snapshot) DescribeReviewChanges(ctx context.Context, before, after Tree, checked []Checked) error {
	rechecker := newRechecker(ctx, after)
	positions := map[string]int{}
	for i, f := range s {
		if f.contextStatus == StatusChanged {
			positions[f.ID] = i
		}
		// Older decisions bind the whole source file instead of a range.
		// Give the assessor current, quotable witnesses after its hash check.
		if f.IsDecision() && f.contextStatus == StatusHistorical && len(f.Citations) == 0 {
			s[i].Citations = rechecker.decisionExcerpts(f)
		}
	}
	if before.Commit == "" || len(positions) == 0 {
		return nil
	}
	// Include other observations when resolving conflicts at the base.
	var facts []Fact
	for _, c := range checked {
		facts = append(facts, c.Fact)
	}
	base, err := before.Check(ctx, facts)
	if err != nil {
		return err
	}
	for _, c := range base {
		i, changed := positions[c.ID]
		if !changed {
			continue
		}
		if (c.Status == StatusCurrent || c.Status == StatusCandidate) && !c.IsDecision() {
			s[i] = rechecker.transition(c.Fact, before.Commit)
			continue
		}
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
		s[i].Claim = fmt.Sprintf("Contract transition within this change for %q. Base %s %s; reviewed version: %s. Check affected consumers and intended migrations.", c.Property, c.Keyword, c.Value, head)
	}
	return ctx.Err()
}

func (r *rechecker) decisionExcerpts(f Fact) []Citation {
	file := r.file(f.Source)
	if file.err != nil || file.digest != f.SourceHash {
		return nil
	}
	sources := discoveryExcerpts(file.data, discoverySource{Kind: discoveryKind(f.Source), Path: f.Source}, 1200, &DiscoveryResult{}, nil)
	var citations []Citation
	for _, s := range sources {
		end := s.StartLine + strings.Count(s.Text, "\n")
		c, err := file.cite(f.Source, f.Revision, s.StartLine, end)
		if err == nil {
			citations = append(citations, c.atCurrent(f.Source, lineAnchor{start: s.StartLine, end: end}))
		}
	}
	return citations
}

// Surviving text can explain how a caller or test was affected. It does not
// reinstate the authority of the old rule, even when every quote still exists.
func (r *rechecker) transition(f Fact, base string) Fact {
	prior := f
	f.contextStatus, f.baseCommit, f.baseClaim = StatusTransition, base, prior.Claim
	f.Claim, f.Reason, f.Authority, f.Value = "", "", AuthorityHistorical, nil
	f.Citations, f.Conditions, f.Feedback = nil, nil, nil
	for _, condition := range prior.Conditions {
		f.baseConditions = append(f.baseConditions, shortText(condition.Description, 160))
	}
	prior.eachCitation(func(c Citation) {
		if !c.live() {
			return
		}
		current, _, _ := r.matchCitation(f.Revision, c)
		p := current.CurrentPath
		if p == "" {
			p = c.Path
		}
		file := r.file(p)
		if file.err != nil {
			return
		}
		// Recover only a verbatim, unique complete-line witness at the head.
		content := "\n" + file.text + "\n"
		witness := "\n" + c.Quote + "\n"
		pos := strings.Index(content, witness)
		if c.Quote == "" || pos < 0 || strings.LastIndex(content, witness) != pos {
			return
		}
		line := 1 + strings.Count(content[:pos], "\n")
		current = c.atCurrent(p, lineAnchor{start: line, end: line + strings.Count(c.Quote, "\n")})
		for _, included := range f.Citations {
			if included.CurrentPath == current.CurrentPath && included.Quote == current.Quote {
				return
			}
		}
		f.Citations = append(f.Citations, current)
	})
	return f
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
