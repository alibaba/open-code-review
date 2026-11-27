// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	allowedext "github.com/alibaba/open-code-review/internal/config/allowlist"
)

const (
	CategorySchema           = "schema"
	CategoryBusinessRule     = "business-rule"
	CategoryDomainConstraint = "domain-constraint"
	CategoryCodingConvention = "coding-convention"
	CategoryInvariant        = "invariant"
	CategoryDesign           = "design"
	CategoryDecision         = "decision"
	CategoryDiscussionSource = "discussion-source"

	AuthorityStructured = "structured"
	AuthorityConfirmed  = "confirmed"
	AuthorityInferred   = "inferred"
	AuthorityHistorical = "historical"
)

type Fact struct {
	ID                 string              `json:"id"`
	Scope              string              `json:"scope"`
	Source             string              `json:"source"`
	Revision           string              `json:"revision"`
	Property           string              `json:"property,omitempty"`
	Keyword            string              `json:"keyword,omitempty"`
	Value              json.RawMessage     `json:"value,omitempty"`
	SourceHash         string              `json:"source_sha256,omitempty"`
	ReviewID           string              `json:"review_id,omitempty"`
	Finding            string              `json:"finding,omitempty"`
	Reason             string              `json:"reason,omitempty"`
	CodeHash           string              `json:"code_sha256,omitempty"`
	Category           string              `json:"category,omitempty"`
	Authority          string              `json:"authority,omitempty"`
	Origin             string              `json:"origin,omitempty"`
	Claim              string              `json:"claim,omitempty"`
	Subject            string              `json:"subject,omitempty"`
	Predicate          string              `json:"predicate,omitempty"`
	Identifiers        []string            `json:"identifiers,omitempty"`
	Citations          []Citation          `json:"citations,omitempty"`
	AcceptedCall       *AcceptedCall       `json:"accepted_call,omitempty"`
	DiscussionBody     string              `json:"discussion_body,omitempty"`
	DiscussionBindings *discussionBindings `json:"discussion_bindings,omitempty"`
	ScopeWitnesses     []Citation          `json:"scope_witnesses,omitempty"`
	Conditions         []Condition         `json:"conditions,omitempty"`
	ExpiresAt          string              `json:"expires_at,omitempty"`
	ObservedAt         string              `json:"observed_at,omitempty"`
	Supersedes         []string            `json:"supersedes,omitempty"`
	Feedback           []FeedbackRecord    `json:"feedback,omitempty"`

	contextStatus    string
	contextConflicts []string
	baseClaim        string
	baseCommit       string
	baseConditions   []string
	directAssertion  *DirectCalleeAssertion
	// A discussion notice changes eligibility without contributing evidence.
	discussionNotice   discussionNoticeKind
	editionScopeDigest string
	// Existing mappings that authorized a schema refresh. Used only while saving.
	schemaParents       []string
	explicitSchemaLearn bool
}

type Citation struct {
	Kind          string             `json:"kind"`
	Path          string             `json:"path,omitempty"`
	Revision      string             `json:"revision,omitempty"`
	Reference     string             `json:"reference,omitempty"`
	Symbol        string             `json:"symbol,omitempty"`
	URL           string             `json:"url,omitempty"`
	ObservedAt    string             `json:"observed_at,omitempty"`
	Landing       *DiscussionLanding `json:"landing,omitempty"`
	StartLine     int                `json:"start_line,omitempty"`
	EndLine       int                `json:"end_line,omitempty"`
	Anchor        string             `json:"anchor,omitempty"`
	Digest        string             `json:"sha256,omitempty"`
	ContextDigest string             `json:"context_sha256,omitempty"`
	Quote         string             `json:"quote,omitempty"`

	CurrentPath      string `json:"current_path,omitempty"`
	CurrentStartLine int    `json:"current_start_line,omitempty"`
	CurrentEndLine   int    `json:"current_end_line,omitempty"`
}

type Condition struct {
	Description string          `json:"description"`
	Citation    *Citation       `json:"citation,omitempty"`
	Key         string          `json:"key,omitempty"`
	Equals      json.RawMessage `json:"equals,omitempty"`
}

func (c Citation) live() bool {
	switch c.Kind {
	case "code", "config", "test", "documentation":
		return true
	}
	return false
}

func (f Fact) IsDecision() bool { return f.ReviewID != "" || f.Category == CategoryDecision }

func (f Fact) schema() bool {
	return !f.IsDecision() && (f.Category == "" || f.Category == CategorySchema) &&
		(f.Keyword == "const" || f.Keyword == "enum") && f.Property != ""
}

func (f Fact) ContextStatus() string { return f.contextStatus }

func (f Fact) CurrentSchema() bool { return f.contextStatus == StatusVerified && f.schema() }

func (f Fact) EvidencePaths() []string {
	var paths []string
	add := func(p string) {
		if p != "" && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	add(f.Source)
	f.eachCitation(func(c Citation) {
		add(c.Path)
		add(c.CurrentPath)
	})
	return paths
}

func (f Fact) eachCitation(visit func(Citation)) {
	for _, c := range f.Citations {
		visit(c)
	}
	for _, cond := range f.Conditions {
		if cond.Citation != nil {
			visit(*cond.Citation)
		}
	}
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validPath(p string) bool {
	return p != "." && fs.ValidPath(p) && filepath.IsLocal(p) && !strings.ContainsAny(p, "\\:*?[]") && !strings.Contains("/"+strings.ToLower(p)+"/", "/.git/") && !strings.ContainsFunc(p, unicode.IsControl) && !allowedext.IsSecretPath(p)
}

func validSource(p string) bool { return validPath(p) && strings.EqualFold(path.Ext(p), ".json") }

func validScope(p string) bool { return p == "." || validPath(p) }

type Constraint struct {
	Property string
	Keyword  string
	Value    json.RawMessage
}

// Constraints extracts direct property const/enum assertions. Conditional or
// referenced subschemas are not flattened into unconditional requirements.
func Constraints(data []byte) ([]Constraint, error) {
	// Boolean schemas are legal alongside object schemas. Decode each separately.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("read JSON schema: %w", err)
	}
	if root["$ref"] != nil {
		return nil, nil
	}
	// An unversioned input uses modern schema semantics. Unknown dialects
	// can redefine keywords, so they do not establish a constraint here.
	var dialect string
	if raw := root["$schema"]; raw != nil {
		if err := json.Unmarshal(raw, &dialect); err != nil {
			return nil, err
		}
		dialect = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(dialect, "https://"), "http://"), "#")
		switch dialect {
		case "json-schema.org/draft-04/schema", "json-schema.org/draft-06/schema", "json-schema.org/draft-07/schema",
			"json-schema.org/draft/2019-09/schema", "json-schema.org/draft/2020-12/schema":
		default:
			return nil, fmt.Errorf("unsupported JSON Schema dialect")
		}
	}
	var properties map[string]json.RawMessage
	if raw := root["properties"]; raw != nil {
		if err := json.Unmarshal(raw, &properties); err != nil {
			return nil, fmt.Errorf("read schema properties: %w", err)
		}
	}
	var result []Constraint
	for property, raw := range properties {
		if property == "" || len(property) > 160 || strings.ContainsFunc(property, unicode.IsControl) {
			continue
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields["$ref"] != nil {
			continue
		}
		for _, kw := range []string{"const", "enum"} {
			if kw == "const" && dialect == "json-schema.org/draft-04/schema" {
				continue
			}
			value, ok := fields[kw]
			if !ok {
				continue
			}
			// Decode via RawMessage to preserve integer precision. Keep only small
			// scalar literals (or sets of them), never examples or free-form documents.
			var vs []json.RawMessage
			if kw == "enum" {
				if json.Unmarshal(value, &vs) != nil || len(vs) == 0 || len(vs) > 32 {
					continue
				}
			} else {
				vs = []json.RawMessage{value}
			}
			valid := true
			for _, v := range vs {
				if len(v) > 256 || v[0] == '{' || v[0] == '[' {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			compact, _ := json.Marshal(value)
			if kw == "enum" {
				// Reordering an allowed set does not change the contract.
				slices.SortFunc(vs, func(a, b json.RawMessage) int { return bytes.Compare(a, b) })
				compact, _ = json.Marshal(vs)
			}
			if len(compact) <= 1024 {
				result = append(result, Constraint{property, kw, compact})
			}
		}
	}
	slices.SortFunc(result, func(a, b Constraint) int {
		if v := strings.Compare(a.Property, b.Property); v != 0 {
			return v
		}
		return strings.Compare(a.Keyword, b.Keyword)
	})
	return result, nil
}
