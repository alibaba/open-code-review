// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package rules loads system review rules and matches file paths against glob patterns.
package rules

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/pathutil"
)

// Resolver resolves a review rule for a file path.
type Resolver interface {
	Resolve(path string) string
}

// PathRule is a single pattern→rule entry preserving declaration order.
type PathRule struct {
	Pattern string
	Rule    string
}

// SystemRule holds review rules loaded from an external JSON config.
type SystemRule struct {
	DefaultRule string     `json:"default_rule"`
	PathRules   []PathRule // ordered; first match wins
}

// UnmarshalJSON preserves the key order from JSON's path_rule_map object.
func (r *SystemRule) UnmarshalJSON(data []byte) error {
	// Decode default_rule normally.
	var wrapper struct {
		DefaultRule string `json:"default_rule"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}
	r.DefaultRule = wrapper.DefaultRule

	// Use json.Decoder with UseNumber to preserve order of path_rule_map keys.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	mapData, ok := raw["path_rule_map"]
	if !ok || len(mapData) == 0 || string(mapData) == "null" {
		return nil
	}

	// Parse ordered keys using a streaming decoder.
	dec := json.NewDecoder(strings.NewReader(string(mapData)))
	// Read opening '{'
	t, err := dec.Token()
	if err != nil {
		return fmt.Errorf("expected '{' in path_rule_map: %w", err)
	}
	if t != json.Delim('{') {
		return fmt.Errorf("expected '{' in path_rule_map, got %v", t)
	}
	for dec.More() {
		// Read key
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("read path_rule_map key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected string key in path_rule_map, got %T", keyTok)
		}
		// Read value
		var value string
		if err := dec.Decode(&value); err != nil {
			return fmt.Errorf("read path_rule_map value for %q: %w", key, err)
		}
		r.PathRules = append(r.PathRules, PathRule{Pattern: key, Rule: value})
	}
	return nil
}

//go:embed system_rules.json rule_docs/*
var rulesFS embed.FS

// LoadDefault parses the embedded system_rules.json and resolves rule file references.
func LoadDefault() (*SystemRule, error) {
	data, err := rulesFS.ReadFile("system_rules.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded system_rules.json: %w", err)
	}
	var rule SystemRule
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("unmarshal default system rules: %w", err)
	}
	content, err := rulesFS.ReadFile("rule_docs/" + rule.DefaultRule)
	if err != nil {
		return nil, fmt.Errorf("read default rule file %q: %w", rule.DefaultRule, err)
	}
	rule.DefaultRule = strings.TrimRight(string(content), "\n")
	for i := range rule.PathRules {
		content, err := rulesFS.ReadFile("rule_docs/" + rule.PathRules[i].Rule)
		if err != nil {
			return nil, fmt.Errorf("read rule file %q for pattern %q: %w", rule.PathRules[i].Rule, rule.PathRules[i].Pattern, err)
		}
		rule.PathRules[i].Rule = strings.TrimRight(string(content), "\n")
	}
	return &rule, nil
}

// loadObjCRule reads the embedded Objective-C rule doc used by the ".m"
// content sniff.
func loadObjCRule() (string, error) {
	content, err := rulesFS.ReadFile("rule_docs/objc.md")
	if err != nil {
		return "", fmt.Errorf("read objc rule file: %w", err)
	}
	return strings.TrimRight(string(content), "\n"), nil
}

// RuleDetail contains the resolved rule along with metadata about its source.
type RuleDetail struct {
	Rule    string // rule text
	Source  string // "custom" | "project" | "global" | "system"
	Pattern string // glob pattern that matched, or "default" for fallback — always a plain glob, never annotated
	// RuleFiles lists the source rule files (or "<inline>") that produced Rule,
	// for ocr-rules-check traceability. Empty for the system layer. It is a
	// debugging aid only: it never enters the LLM prompt or delegate JSON.
	RuleFiles []string `json:"-"`
	// SniffedAs is "" for a plain path match, or the sniffed language (e.g.
	// "objc") when content sniffing overrode the path-based rule. Internal
	// only: callers that serialize RuleDetail (e.g. delegateRuleGroupJSON)
	// must not surface this, since Pattern is a versioned "the glob that
	// matched" contract that a sniff annotation would silently break.
	SniffedAs string `json:"-"`
}

// DetailResolver extends Resolver with source metadata.
type DetailResolver interface {
	ResolveDetail(path string) RuleDetail
}

// Resolve returns the rule text for a given file path.
// Patterns with brace expansion like "*.{go,py}" are expanded into "*.go", "*.py".
// The first match wins; if none match, it falls back to DefaultRule.
// Supports full glob syntax including ** for recursive directory matching.
func (r *SystemRule) Resolve(path string) string {
	return r.resolveDetail(path).Rule
}

// CanonicalConfig returns a deterministic, order-stable field list describing this
// rule set's effective rule-text configuration, for hashing into the run manifest's
// rule_config_sha256. It covers only rule-text resolution (default plus ordered
// pattern rules); include/exclude filtering is carried by FileFilter and hashed
// separately. Order is preserved because first match wins.
func (r *SystemRule) CanonicalConfig() []string {
	fields := []string{"layer", "system", "default", r.DefaultRule}
	for _, pr := range r.PathRules {
		fields = append(fields, "layer", "system", "pattern", pr.Pattern, "rule", pr.Rule)
	}
	return fields
}

func (r *SystemRule) resolveDetail(path string) RuleDetail {
	lowerPath := strings.ToLower(path)
	for _, pr := range r.PathRules {
		expanded := expandBraces(pr.Pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return RuleDetail{Rule: pr.Rule, Source: "system", Pattern: pr.Pattern}
			}
		}
	}
	return RuleDetail{Rule: r.DefaultRule, Source: "system", Pattern: "default"}
}

// expandBraces turns "{a,b,c}" style patterns into individual strings.
// e.g. "*.go.{java,kotlin}" → ["*.go.java", "*.go.kotlin"].
// If no braces exist, returns the original pattern unchanged.
func expandBraces(s string) []string {
	openIdx := strings.IndexByte(s, '{')
	if openIdx < 0 {
		return []string{s}
	}

	closeIdx := strings.IndexByte(s[openIdx:], '}')
	if closeIdx < 0 {
		return []string{s}
	}
	closeIdx += openIdx

	prefix := s[:openIdx]
	suffix := s[closeIdx+1:]
	options := strings.Split(s[openIdx+1:closeIdx], ",")

	results := make([]string, 0, len(options))
	for _, opt := range options {
		results = append(results, prefix+opt+suffix)
	}
	return results
}

// ProjectRuleEntry is a single entry in .opencodereview/rule.json.
type ProjectRuleEntry struct {
	Path            string `json:"path"`
	Rule            string `json:"rule"`
	MergeSystemRule bool   `json:"merge_system_rule,omitempty"`

	// ruleInputs records the raw "rule" inputs (file paths or inline text) as
	// written by the user, before resolveRuleEntries reads/merges them into
	// Rule. It drives the multi-file merge path (len >= 2) and preserves the
	// original source for ocr-rules-check traceability after Rule becomes file
	// content. resolveRuleEntries keeps the user's original inputs verbatim
	// (including files that failed to read or were rejected by #1100), so the
	// "Rule Files:" display reports what the user wrote, while Rule contains
	// only the inputs that actually contributed — with one exception: when an
	// array exceeds maxRuleInputs, resolveRuleEntries truncates ruleInputs to
	// the first maxRuleInputs survivors (before any file I/O), so the display
	// then reports only those survivors, not the dropped trailing inputs.
	// This makes the multi-input path naturally idempotent: a repeat call
	// re-reads the same surviving inputs and produces the same Rule. The
	// single-value path reads e.Rule like the legacy code (not idempotent,
	// but every caller invokes it exactly once per load).
	// Unexported: it never serializes. nil for Go-literal-constructed entries
	// (no UnmarshalJSON), which take the single-value path and get back-filled
	// here so RuleFiles can still report their source.
	ruleInputs []string
}

// UnmarshalJSON accepts "rule" as either a string (legacy, byte-identical) or
// an array of strings (multi-file merge). A single-element array degrades
// to the single-value path so its output is byte-identical to the string form.
func (e *ProjectRuleEntry) UnmarshalJSON(data []byte) error {
	// raw mirrors ProjectRuleEntry's exported JSON fields, keeping Rule as a
	// RawMessage so it can be decoded polymorphically (string or array) below.
	// Decoding into e directly would recurse into this method, so a shadow
	// struct is the idiomatic way. Keep these fields in sync with
	// ProjectRuleEntry's json tags: a new JSON field added to the entry must
	// be added here too, or it is silently dropped during decoding.
	var raw struct {
		Path            string          `json:"path"`
		Rule            json.RawMessage `json:"rule"`
		MergeSystemRule bool            `json:"merge_system_rule,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		// Clear stale values for contract consistency: encoding/json does
		// not zero the receiver before calling UnmarshalJSON. A
		// shadow-struct decode error (e.g. a "path" field type mismatch)
		// fails here before any field is read from raw, so it must not
		// leave stale values behind. Clear every field the success path
		// sets below (Path, MergeSystemRule, Rule, ruleInputs), not just
		// Rule/ruleInputs — leaving a stale Path/MergeSystemRule on a
		// re-decode into an already-populated entry would be the same
		// contract violation the other non-productive branches guard
		// against. (Syntactically malformed JSON never reaches
		// UnmarshalJSON: the stdlib validates syntax before dispatching,
		// so a field type mismatch is the realistic way this initial
		// decode fails.)
		e.Path = ""
		e.MergeSystemRule = false
		e.Rule = ""
		e.ruleInputs = nil
		return err
	}
	e.Path = raw.Path
	e.MergeSystemRule = raw.MergeSystemRule

	if len(raw.Rule) == 0 || string(raw.Rule) == "null" {
		// "rule" absent or null: reset to zero values. encoding/json does not
		// clear the receiver before calling UnmarshalJSON, so re-decoding into
		// a previously populated entry must not leave stale Rule/ruleInputs.
		e.Rule = ""
		e.ruleInputs = nil
		return nil
	}

	// Track the decode errors so a malformed "rule" value reports its cause
	// instead of only a generic "must be a string or array of strings".
	var arrayErr, stringErr error

	// String form: legacy single value. Back-fill ruleInputs for traceability.
	var s string
	if stringErr = json.Unmarshal(raw.Rule, &s); stringErr == nil {
		if s == "" {
			// Empty string is equivalent to absent: clear Rule and ruleInputs so
			// the entry is skipped downstream (same as the omitted case).
			// encoding/json does not zero the receiver before calling
			// UnmarshalJSON, so re-decoding must not leave stale values.
			e.Rule = ""
			e.ruleInputs = nil
			return nil
		}
		e.Rule = s
		e.ruleInputs = []string{s}
		return nil
	}

	// Array form: multi-file merge. A single-element array degrades to the
	// single-value path (Rule set, byte-identical to the string form).
	var arr []string
	if err := json.Unmarshal(raw.Rule, &arr); err == nil {
		if len(arr) == 1 {
			// Single-element array degrades to the single-value path (Rule
			// set, byte-identical to the string form). An empty element is
			// equivalent to the empty-string form: clear Rule and ruleInputs
			// so the entry is skipped downstream, matching the string path.
			if arr[0] == "" {
				e.Rule = ""
				e.ruleInputs = nil
			} else {
				e.Rule = arr[0]
				e.ruleInputs = arr
			}
		} else {
			// Empty or multi-element array: Rule is resolved later by
			// resolveRuleEntries, so clear any stale value left from a prior
			// decode (encoding/json does not zero the receiver first). An
			// empty array behaves like absent (skip the entry), so nil out
			// ruleInputs rather than keeping an empty non-nil slice.
			e.Rule = ""
			if len(arr) == 0 {
				e.ruleInputs = nil
			} else {
				e.ruleInputs = arr
			}
		}
		return nil
	} else {
		arrayErr = err
	}

	// Neither a string nor a valid string array: both decodes failed, so
	// arrayErr and stringErr are non-nil. Pick the cause that best describes
	// the actual input. For an array-shaped value, arrayErr pinpoints the
	// offending element (e.g. a non-string element in a mixed-type array);
	// for any other shape (number, bool, object), the string decode error
	// names the actual offending type ("cannot unmarshal number into string")
	// rather than the []string target the user never wrote.
	cause := arrayErr
	if !strings.HasPrefix(strings.TrimSpace(string(raw.Rule)), "[") {
		cause = stringErr
	}
	// Clear stale values for contract consistency: encoding/json does not zero
	// the receiver first, and the other non-productive branches above reset
	// Rule/ruleInputs — the error path should too, so a re-decode of an
	// already-populated entry with an invalid rule type leaves nothing behind.
	// Clear Path/MergeSystemRule as well, matching the shadow-decode-error
	// branch above: although those were validly decoded from the new JSON, an
	// invalid rule means the whole entry is unusable (the caller drops it on
	// error), so leaving them set would be an inconsistent half-cleared state
	// that a future caller ignoring the error could mistake for a usable entry.
	e.Path = ""
	e.MergeSystemRule = false
	e.Rule = ""
	e.ruleInputs = nil
	return fmt.Errorf("rule: must be a string or array of strings: %w", cause)
}

// ProjectRule holds rules loaded from <repoDir>/.opencodereview/rule.json.
type ProjectRule struct {
	Rules   []ProjectRuleEntry `json:"rules"`
	Include []string           `json:"include,omitempty"`
	Exclude []string           `json:"exclude,omitempty"`
}

// FileFilter holds the merged user-configured include/exclude glob patterns
// collected from all rule.json layers (custom, project, global).
type FileFilter struct {
	Include []string
	Exclude []string
}

// HasInclude reports whether any include patterns are configured.
func (f *FileFilter) HasInclude() bool {
	return len(f.Include) > 0
}

// IsUserExcluded reports whether the given path matches any user exclude pattern.
// The check is case-insensitive: both path and pattern are lowercased.
func (f *FileFilter) IsUserExcluded(path string) bool {
	lowerPath := strings.ToLower(path)
	for _, pattern := range f.Exclude {
		expanded := expandBraces(pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return true
			}
		}
	}
	return false
}

// IsUserIncluded reports whether the given path matches any user include pattern.
// The check is case-insensitive: both path and pattern are lowercased.
// Returns false when Include is empty (no user include restriction defined).
func (f *FileFilter) IsUserIncluded(path string) bool {
	if !f.HasInclude() {
		return false
	}
	lowerPath := strings.ToLower(path)
	for _, pattern := range f.Include {
		expanded := expandBraces(pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return true
			}
		}
	}
	return false
}

// composedResolver implements Resolver with layered priority.
type composedResolver struct {
	custom  *ProjectRule // highest: --rule flag
	project *ProjectRule // high: .opencodereview/rule.json
	global  *ProjectRule // low: ~/.opencodereview/rule.json
	system  systemLayer  // lowest: embedded default, decorated by sniffer
}

// ResolverOptions carries the optional git context the resolver needs to read
// file content when disambiguating extensions shared by several languages
// (currently only ".m": MATLAB vs Objective-C). The zero value is valid and
// makes content reads fall back to the working tree.
type ResolverOptions struct {
	// Ref is the git ref whose content should be inspected — the review head
	// (--to) in range mode, or --commit in commit mode. Empty reads the
	// working tree, which is what `ocr scan` and `ocr rules check` want.
	Ref string

	// Runner bounds concurrent git subprocesses. Optional; when nil the
	// resolver shells out to git directly.
	Runner *gitcmd.Runner
}

// NewResolver builds a Resolver with the following priority:
//  1. Custom rule file specified via --rule flag (first match wins)
//  2. Project-local .opencodereview/rule.json (first match wins)
//  3. Global ~/.opencodereview/rule.json (first match wins)
//  4. Embedded system default rules
//
// The system layer is wrapped in a sniffer so ".m" files can be resolved as
// Objective-C when their content says so. Wrapping the *system* layer (rather
// than the composed resolver) keeps user layers outranking the sniff.
//
// It also returns a FileFilter with the merged include/exclude patterns from all layers.
func NewResolver(repoDir, customRulePath string, opts ResolverOptions) (Resolver, *FileFilter, error) {
	sysRule, err := LoadDefault()
	if err != nil {
		return nil, nil, err
	}

	objcRule, err := loadObjCRule()
	if err != nil {
		return nil, nil, err
	}

	var customRule *ProjectRule
	if customRulePath != "" {
		cr, err := loadRuleFile(customRulePath)
		if err != nil {
			return nil, nil, err
		}
		customRule = cr
	}

	var projectRule *ProjectRule
	if repoDir != "" {
		pr, err := loadProjectRule(repoDir)
		if err != nil {
			return nil, nil, err
		}
		projectRule = pr
	}

	globalRule, err := loadGlobalRule()
	if err != nil {
		return nil, nil, err
	}

	filter := buildFileFilter(customRule, projectRule, globalRule)

	return &composedResolver{
		custom:  customRule,
		project: projectRule,
		global:  globalRule,
		system: &sniffer{
			inner:    sysRule,
			repoDir:  repoDir,
			ref:      opts.Ref,
			runner:   opts.Runner,
			objcRule: objcRule,
		},
	}, filter, nil
}

// buildFileFilter picks the highest-priority layer that has any include/exclude
// configured. Priority order: custom (--rule) > project > global.
func buildFileFilter(layers ...*ProjectRule) *FileFilter {
	for _, pr := range layers {
		if pr == nil {
			continue
		}
		if len(pr.Include) == 0 && len(pr.Exclude) == 0 {
			continue
		}
		f := &FileFilter{}
		for _, p := range pr.Include {
			f.Include = append(f.Include, strings.ToLower(p))
		}
		for _, p := range pr.Exclude {
			f.Exclude = append(f.Exclude, strings.ToLower(p))
		}
		return f
	}
	return nil
}

func loadGlobalRule() (*ProjectRule, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	path := filepath.Join(home, ".opencodereview", "rule.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read global rule %s: %w", path, err)
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal global rule: %w", err)
	}
	resolveRuleEntries(pr.Rules, filepath.Dir(path), "")
	return &pr, nil
}

func loadRuleFile(path string) (*ProjectRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rule file %s: %w", path, err)
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal rule file %s: %w", path, err)
	}
	resolveRuleEntries(pr.Rules, filepath.Dir(path), "")
	return &pr, nil
}

// loadProjectRule reads <repoDir>/.opencodereview/rule.json. Since #287 anchored
// RepoDir at the git top-level, `ocr review` from a monorepo subdirectory loads
// the repo-root rule file — which is consistent, since rule entries match against
// root-relative diff paths. A subproject-local rule.json under the subdirectory is
// intentionally not consulted; put shared rules at the repo root, or pass --rule.
func loadProjectRule(repoDir string) (*ProjectRule, error) {
	confineRoot, err := pathutil.CanonicalPath(repoDir)
	if err != nil {
		return nil, fmt.Errorf("resolve repo dir %s: %w", repoDir, err)
	}

	path := filepath.Join(repoDir, ".opencodereview", "rule.json")
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve project rule %s: %w", path, err)
	}
	if !pathutil.WithinBase(confineRoot, resolved) {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: project rule file escapes repo dir: %s\n", path)
		return nil, nil
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read project rule %s: %w", path, err)
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal project rule: %w", err)
	}
	resolveRuleEntries(pr.Rules, repoDir, confineRoot)
	return &pr, nil
}

// Resolve checks each layer in priority order; first match wins. User rules
// replace the system rule by default; rules with merge_system_rule keep the
// matched system rule alongside the user rule.
func (c *composedResolver) Resolve(path string) string {
	for _, layer := range []*ProjectRule{c.custom, c.project, c.global} {
		if entry := matchProjectRuleEntry(layer, path); entry != nil {
			if entry.MergeSystemRule {
				return c.mergeWithSystemRule(path, entry.Rule)
			}
			return entry.Rule
		}
	}
	return c.system.Resolve(path)
}

// CanonicalConfig returns a deterministic, order-stable field list describing the
// resolver's effective rule-text configuration across every layer (custom >
// project > global > system, each in declaration order), for hashing into the run
// manifest's rule_config_sha256. It covers only rule-text resolution; the
// include/exclude file filter is carried by FileFilter and hashed separately. Each
// field is tagged with its layer and role so two structurally different configs
// cannot collide once length-prefixed. Order is never sorted — first match wins.
func (c *composedResolver) CanonicalConfig() []string {
	var fields []string
	appendLayer := func(name string, pr *ProjectRule) {
		if pr == nil {
			return
		}
		for _, e := range pr.Rules {
			merge := "0"
			if e.MergeSystemRule {
				merge = "1"
			}
			fields = append(fields, "layer", name, "path", e.Path, "rule", e.Rule, "merge", merge)
		}
	}
	appendLayer("custom", c.custom)
	appendLayer("project", c.project)
	appendLayer("global", c.global)
	if c.system != nil {
		fields = append(fields, c.system.CanonicalConfig()...)
	}
	return fields
}

func (c *composedResolver) mergeWithSystemRule(path, rule string) string {
	systemRule := c.system.Resolve(path)

	if systemRule == "" {
		return rule
	}
	if rule == "" {
		return systemRule
	}

	return "## System-Specific Rules (Mandatory)\n\n" +
		systemRule +
		"\n\n---\n\n" +
		"## User-Specific Rules (Mandatory)\n\n" +
		rule
}

// ResolveDetail returns the matched rule along with its source layer and pattern.
// When a user rule sets merge_system_rule, Rule contains the merged system+user
// rule text while Source and Pattern still describe the user rule that won the
// priority chain.
func (c *composedResolver) ResolveDetail(path string) RuleDetail {
	if detail := c.matchProjectRuleDetail(c.custom, path, "custom"); detail != nil {
		return *detail
	}
	if detail := c.matchProjectRuleDetail(c.project, path, "project"); detail != nil {
		return *detail
	}
	if detail := c.matchProjectRuleDetail(c.global, path, "global"); detail != nil {
		return *detail
	}
	return c.system.resolveDetail(path)
}

func (c *composedResolver) matchProjectRuleDetail(pr *ProjectRule, path, source string) *RuleDetail {
	entry := matchProjectRuleEntry(pr, path)
	if entry == nil {
		return nil
	}
	rule := entry.Rule
	if entry.MergeSystemRule {
		rule = c.mergeWithSystemRule(path, rule)
	}
	return &RuleDetail{
		Rule:      rule,
		Source:    source,
		Pattern:   entry.Path,
		RuleFiles: classifyRuleFiles(entry.ruleInputs, entry.Rule),
	}
}

// classifyRuleFiles maps an entry's raw rule inputs to display labels for the
// "Rule Files:" line printed by ocr-rules-check. inputs is the user's original
// ruleInputs, kept verbatim by resolveRuleEntries except when the maxRuleInputs
// count cap trims it to the first maxRuleInputs survivors, so the display
// reports exactly what the user wrote — including files that failed to read or
// were rejected by #1100, which appear here even though they contributed nothing
// to Rule. File-path inputs are kept verbatim (the full field as written, e.g.
// "rules/kotlin.md"), so same-named files in different directories stay
// distinguishable. This is local debug output only: RuleFiles has json:"-"
// and never reaches the LLM prompt or delegate JSON, so the display label
// preserves the input as-is. Path-traversal safety is already enforced by
// #1100's confineRoot on the untrusted project layer, so no extra
// sanitization is applied here. Inline inputs (anything looksLikeFilePath
// rejects) become "<inline>". In a multi-element array, an empty or
// whitespace-only element (e.g. a trailing comma) is skipped from the Rule
// body by resolveMultiRuleParts with a warning, so it contributed nothing —
// it is omitted from the "Rule Files:" display entirely rather than shown as
// a misleading label. A single-value whitespace rule is different: it is the
// whole rule and wins its layer (legacy leaves it byte-identical), so it is
// genuine inline text and stays "<inline>". When inputs is empty/nil
// (Go-literal-constructed entry with no UnmarshalJSON, so ruleInputs was
// never populated), it falls back to inferring a single label from the
// resolved Rule: a file path stays verbatim; inline text with non-whitespace
// content becomes "<inline>"; empty or whitespace-only yields nil — there is
// no file and no real text to attribute. When every input in a multi-element
// array is an empty/whitespace element (all omitted), the inputs branch also
// returns nil, so "no rule files" is a single nil representation across both
// paths — a caller comparing != nil sees the same result either way.
func classifyRuleFiles(inputs []string, fallbackRule string) []string {
	if len(inputs) > 0 {
		// A single input is the whole rule: if it is whitespace-only inline
		// text it still wins its layer (legacy), so label it "<inline>".
		// Empty/whitespace elements inside a multi-element array contributed
		// nothing (resolveMultiRuleParts skips them), so omit them from the
		// display rather than show a label for an input that is not a file and
		// carries no inline text.
		multi := len(inputs) > 1
		out := make([]string, 0, len(inputs))
		for _, in := range inputs {
			if looksLikeFilePath(in) {
				out = append(out, in)
			} else if multi && strings.TrimSpace(in) == "" {
				// Omitted: an empty element in a multi-element array is not a
				// file and carries no inline text, so it has no label.
				continue
			} else {
				out = append(out, "<inline>")
			}
		}
		if len(out) == 0 {
			// All inputs were empty/whitespace elements in a multi-element
			// array: nothing is a file or inline text, so there is nothing to
			// display. Return nil to match the fallback path's "no rule files"
			// convention, so the two paths use a single representation (a
			// caller comparing != nil sees the same result either way).
			return nil
		}
		return out
	}
	if looksLikeFilePath(fallbackRule) {
		return []string{fallbackRule}
	}
	if strings.TrimSpace(fallbackRule) != "" {
		return []string{"<inline>"}
	}
	return nil
}

func matchProjectRuleEntry(pr *ProjectRule, path string) *ProjectRuleEntry {
	if pr == nil {
		return nil
	}
	lowerPath := strings.ToLower(path)
	for i := range pr.Rules {
		entry := &pr.Rules[i]
		if entry.Rule == "" && !entry.MergeSystemRule {
			continue
		}
		expanded := expandBraces(entry.Path)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return entry
			}
		}
	}
	return nil
}

// allowedRuleExts is the set of file extensions permitted for rule file references.
var allowedRuleExts = map[string]bool{".md": true, ".txt": true, ".markdown": true}

// maxRuleBodySize is the maximum byte budget of a resolved rule body. It bounds
// both a single rule file (readRuleFileSafe rejects larger files) and the merged
// body of a multi-input array entry (resolveMultiRuleParts skips an input whole
// once adding it — segment content plus the "\n\n---\n\n" separator — would make
// the running total exceed the budget; the running total is unchanged by the
// skip, so a later smaller input can still fit and contribute). The two paths
// share the same budget so a multi-file merge never expands the surface beyond
// a single md file, regardless of how many files an untrusted project rule.json
// references.
const maxRuleBodySize = 512 * 1024

// rulePartSep is the separator joinRuleParts inserts between merged rule parts.
// It is shared with resolveMultiRuleParts, which counts len(rulePartSep) in the
// running size total, so the size cap and the actual join can never drift apart
// (the two stay in sync structurally rather than by convention).
const rulePartSep = "\n\n---\n\n"

// maxRuleInputs bounds the number of inputs a single multi-input array entry
// (rule: ["...", ...]) resolves. The first maxRuleInputs inputs are processed;
// any beyond that are dropped from both the resolved Rule body and ruleInputs
// (with a warning), so they cost no file I/O and do not appear in "Rule Files:".
// This caps both the file-read I/O (at most maxRuleInputs file reads per entry,
// each ~3 syscalls) and the retained ruleInputs memory — two surfaces the byte
// cap maxRuleBodySize alone cannot bound, because empty/missing/empty-file
// inputs are skipped without growing the running total. Every input — valid file,
// inline text, or an empty "" element — counts toward the limit, so a user who
// pads the array with junk inputs (e.g. trailing commas) only starves their own
// budget. 32 is far beyond any legitimate config (a real entry references a
// handful of files); the byte cap still bounds the merged body, so 32 is a
// count guard, not a content guard — 32 large files are still bounded to 512 KB.
const maxRuleInputs = 32

// entryPathCtx returns the entry-path suffix appended to resolve warnings,
// or "" when path is empty/whitespace so no confusing "(path: )" leaks. The
// warnings in resolveRuleEntries, resolveMultiRuleParts, and tryReadRuleFile
// all share this form, so changing the suffix format here updates every site.
func entryPathCtx(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return fmt.Sprintf(" (path: %s)", path)
}

// looksLikeFilePath returns true when s is likely a file path (not inline content).
// Heuristic: multi-line text is always inline; single-line text without spaces
// ending in .md/.txt/.markdown is treated as a file path. Values containing spaces
// (e.g. "Follow rules from team.md") are treated as inline to avoid false positives.
func looksLikeFilePath(s string) bool {
	if strings.Contains(s, "\n") {
		return false
	}
	if strings.Contains(s, " ") {
		return false
	}
	return allowedRuleExts[strings.ToLower(filepath.Ext(s))]
}

// resolveRuleEntries reads file references in each rule entry and replaces them with
// the file content. confineRoot is the canonical repo root for the untrusted project
// layer (empty for trusted layers, meaning no confinement).
//
// Multi-input entries (rule came in as a >=2-element array) take a merge
// path: each input is read/kept and the results joined with separators. Single
// inputs and Go-literal-constructed entries (ruleInputs nil) take the legacy
// single-value path, which is byte-identical to the previous behavior.
//
// For multi-input entries, ruleInputs is first capped to maxRuleInputs: only
// the first maxRuleInputs inputs are resolved and retained (the rest are dropped
// with a warning, before any file I/O). This bounds both the per-entry file-read
// I/O and the retained ruleInputs memory that the byte cap alone cannot reach.
// The surviving inputs are then resolved. When the cap triggers, ruleInputs is
// trimmed to those survivors (not the full original array) so "Rule Files:"
// reports only what was actually processed; when it does not trigger, ruleInputs
// is the original array unchanged. The multi-input path is naturally idempotent
// — a repeat call re-reads the same surviving inputs and rebuilds the same
// Rule — so no resolved flag is needed. The single-value path uses e.Rule like
// the legacy code; all current callers (loadGlobalRule/loadRuleFile/loadProjectRule)
// call it exactly once per load.
func resolveRuleEntries(entries []ProjectRuleEntry, repoDir string, confineRoot string) {
	for i := range entries {
		e := &entries[i]
		if len(e.ruleInputs) >= 2 {
			inputs := e.ruleInputs
			if len(inputs) > maxRuleInputs {
				// Cap before any file I/O: only the first maxRuleInputs inputs
				// are resolved and retained. Dropping here (not inside
				// resolveMultiRuleParts) bounds both the read I/O and the
				// retained ruleInputs memory that the byte cap cannot reach,
				// since empty/missing/empty-file inputs are skipped without
				// growing the running total. Trailing inputs are dropped with
				// a warning naming the first dropped index; the entry path is
				// included so the user can locate the offending entry.
				ctx := entryPathCtx(e.Path)
				fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule array has %d inputs; only the first %d are resolved, the rest (from index %d) are dropped%s\n", len(inputs), maxRuleInputs, maxRuleInputs, ctx)
				// Copy survivors into a fresh slice so the dropped inputs' string
				// data can be GC'd. A plain subslice (inputs[:maxRuleInputs])
				// aliases the original backing array populated by UnmarshalJSON,
				// so the dropped entries' strings would stay alive for the
				// entry's lifetime — undermining the "bounds retained
				// ruleInputs memory" goal above for an untrusted rule.json with
				// many large inline inputs.
				survivors := make([]string, maxRuleInputs)
				copy(survivors, inputs)
				inputs = survivors
				e.ruleInputs = survivors
			}
			parts := resolveMultiRuleParts(inputs, repoDir, confineRoot, e.Path)
			e.Rule = joinRuleParts(parts)
			// ruleInputs now holds only the surviving (capped) inputs. Empty
			// or unreadable inputs among them are skipped from Rule by
			// resolveMultiRuleParts but remain in ruleInputs so "Rule Files:"
			// can report them — though empty "" elements are omitted from
			// that display by classifyRuleFiles. The cap above already
			// dropped everything past maxRuleInputs.
			continue
		}
		// Single-value / Go-literal path (byte-identical to legacy behavior):
		// the rule is a single file reference, read like the legacy path.
		if strings.TrimSpace(e.Rule) == "" || !looksLikeFilePath(e.Rule) {
			continue
		}
		if e.ruleInputs == nil {
			// Go-literal-constructed entry: back-fill the source for RuleFiles
			// tracing. This does not affect resolution, which uses e.Rule.
			e.ruleInputs = []string{e.Rule}
		}
		if content := tryReadRuleFile(e.Rule, repoDir, confineRoot, ""); content != nil {
			e.Rule = *content
			// An empty/whitespace file resolves to its (possibly empty) content,
			// byte-identical to legacy: an empty file resolves to "" (skipped
			// unless MergeSystemRule) and a whitespace-only file still wins its
			// layer. ruleInputs is left as the original input so "Rule Files:"
			// reports the file the user wrote, even though it contributed no
			// text — consistent with the multi-input path.
		} else {
			// The file is missing/unreadable: Rule is cleared (legacy behavior
			// — the entry is skipped and falls through). ruleInputs is left as
			// the original input so "Rule Files:" still reports what the user
			// wrote, consistent with the multi-input path (which keeps missing
			// files in ruleInputs but skips them from Rule).
			e.Rule = ""
		}
	}
}

// resolveMultiRuleParts reads each input (file path or inline text) and returns
// the segments that successfully contributed. confineRoot is forwarded to
// every file read so #1100's path confinement applies to each array element,
// never relaxed by merging. A missing/unreadable file is warned and skipped;
// if all inputs drop out, the result is empty so the entry is skipped
// downstream (same semantics as a single missing file). entryPath is the
// owning entry's path pattern, included in the empty-element warning so the
// user can locate which rule entry the skipped element belongs to.
//
// inputs is already capped to maxRuleInputs by the caller (resolveRuleEntries),
// so the loop iterates at most maxRuleInputs times — that bounds the file-read
// I/O. Within the loop, maxRuleBodySize — the same 512 KB budget as a single
// rule file — bounds the merged body so merging many files never expands the
// memory/LLM-prompt surface beyond a single md file; once adding an input
// (segment content plus the "\n\n---\n\n" separator) would make the running
// total exceed it, that input is skipped whole with a warning (the running
// total is unchanged by the skip, so a later smaller input may still fit and
// contribute), each part kept whole (never mid-file truncated). The caller's
// count cap and the byte cap are complementary: the count cap bounds the
// number of file reads (I/O), the byte cap bounds the merged content (LLM
// prompt size).
func resolveMultiRuleParts(inputs []string, repoDir string, confineRoot string, entryPath string) []string {
	// ctx is the owning entry's path pattern, appended to warnings so the
	// user can locate the offending entry. entryPathCtx keeps a
	// whitespace-only path from producing a confusing "(path: )" suffix.
	ctx := entryPathCtx(entryPath)
	const sepLen = len(rulePartSep) // joinRuleParts separator, shared with the join
	var parts []string
	total := 0
	for i, raw := range inputs {
		if strings.TrimSpace(raw) == "" {
			// An empty/whitespace array element (e.g. a trailing comma) is
			// invalid config; warn with the element index and the owning entry's
			// path so the user can locate it, consistent with missing-file
			// warnings that carry the rule reference. The element still counts
			// toward the caller's maxRuleInputs cap (it was not dropped there
			// — only inputs past the cap were), so junk inputs only starve the
			// user's own budget.
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: empty rule input skipped in array at index %d%s\n", i, ctx)
			continue
		}
		var content string
		if looksLikeFilePath(raw) {
			c := tryReadRuleFile(raw, repoDir, confineRoot, entryPath)
			if c == nil {
				// Read failure: warned by tryReadRuleFile (which appends the
				// entry path, consistent with the empty-element/empty-file
				// warnings below), skip this input.
				continue
			}
			if strings.TrimSpace(*c) == "" {
				// The file exists but is empty or whitespace-only: it
				// contributes no rule text, so skip it like a missing
				// file rather than add a content-less segment that would
				// produce a dangling separator with no body in the
				// merged output.
				// readRuleFileSafe only trims trailing newlines, so
				// strings.TrimSpace is what catches all whitespace here
				// (leading/internal newlines, spaces, tabs). Warn so the
				// user knows the listed file added nothing, parallel to
				// the not-found warning above. The empty-array-element
				// check above uses TrimSpace and includes the owning
				// entry's path for the same reason; this stays consistent
				// with it, so the user can tell
				// which rule entry an empty file belongs to when several
				// entries reference same-named files.
				fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file is empty: %s%s\n", raw, ctx)
				continue
			}
			content = *c
		} else {
			content = raw
		}
		// Total-size cap: the merged body shares a single md file's budget.
		// Count the separator joinRuleParts inserts before this part (none
		// before the first). Drop the WHOLE part on overflow — never mid-file
		// truncate — so each contributing file stays intact. A `break` would
		// stop reading all later inputs, but `continue` is used instead: the
		// overflowing part is skipped (total is unchanged, since the add below
		// is not reached), so a later smaller input can still fit the budget
		// and contribute. The overall file-read I/O is bounded separately by
		// the maxRuleInputs count cap applied in resolveRuleEntries before this
		// loop runs (at most maxRuleInputs reads per entry), so `continue`
		// cannot re-open the I/O-amplification surface that the count cap
		// closed. Each skipped part is warned individually; ruleInputs still
		// lists it so "Rule Files:" reports what the user wrote.
		//
		// The file is read (via tryReadRuleFile above) before this cap
		// decides to skip it, so an overflowing file is read then discarded
		// rather than skipped unread. This is a deliberate trade-off, not
		// an oversight. readRuleFileSafe already calls os.Stat internally
		// and checks the size before os.ReadFile, so the size that would
		// overflow is known before the read — but tryReadRuleFile returns
		// only content or nil, so propagating that size to this caller to
		// enable an early skip would require restructuring its API. The
		// trade-off is that restructuring cost versus the bounded waste
		// here (at most maxRuleInputs reads of ≤512 KB each, transient, on
		// the config-load path, not a hot path). A separate pre-read os.Stat
		// outside tryReadRuleFile is a worse alternative: it adds a
		// redundant Stat to every file input — readRuleFileSafe already
		// Stats each file once — and runs before readRuleFileSafe's
		// EvalSymlinks+confineRoot (#1100) ordering, following a symlink
		// to stat a target the read path would reject. Keeping the
		// read-then-skip form preserves that ordering and avoids the
		// redundant syscall.
		delta := len(content)
		if len(parts) > 0 {
			delta += sepLen
		}
		if total+delta > maxRuleBodySize {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: merged rule body exceeds %d bytes at input index %d; this input skipped%s\n", maxRuleBodySize, i, ctx)
			continue
		}
		total += delta
		parts = append(parts, content)
	}
	return parts
}

// joinRuleParts concatenates rule parts with a blank line, "---", and a blank
// line between segments. The merged Rule body is pure rule content only — no
// per-file headings. File-source traceability is carried by the "Rule Files:"
// line in `ocr rules check` (populated from ruleInputs via classifyRuleFiles),
// which never enters the LLM prompt. Keeping the body heading-free makes the
// single-value, single-element-array, and multi-element-array cases uniform
// and keeps the `ocr rules check` body byte-identical to the `ocr review` LLM
// input. Each part is just the segment text (file content or inline text); no
// per-part metadata is carried here — source labels are derived at display
// time by classifyRuleFiles from the original ruleInputs.
func joinRuleParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString(rulePartSep)
		}
		b.WriteString(p)
	}
	return b.String()
}

// tryReadRuleFile reads a rule file reference. Absolute paths are used directly;
// relative paths resolve against repoDir. When confineRoot is non-empty, the resolved
// path must stay inside it. Returns nil when the file cannot be read safely.
//
// entryPath is the owning entry's path pattern; when non-empty it is appended to
// each warning so a missing/unreadable file in a multi-element array carries the
// same context as the empty-element and empty-file warnings in resolveMultiRuleParts.
// The single-value legacy caller passes "" so its warnings stay byte-identical
// to the legacy behavior.
func tryReadRuleFile(rule string, repoDir string, confineRoot string, entryPath string) *string {
	// ctx is appended to warnings; empty for the legacy single-value caller so
	// its warnings are byte-identical, non-empty for the multi-input caller.
	// entryPathCtx guards a whitespace-only path against "(path: )".
	ctx := entryPathCtx(entryPath)
	if repoDir == "" {
		if !filepath.IsAbs(rule) {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot resolve relative rule path %q without a repo dir%s\n", rule, ctx)
			return nil
		}
	}
	if filepath.IsAbs(rule) {
		content, err := readRuleFileSafe(rule, confineRoot)
		if err == nil {
			return &content
		}
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file not found: %s%s\n", rule, ctx)
		} else {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read rule file %s: %v%s\n", rule, err, ctx)
		}
		return nil
	}

	// Relative path: resolve against repoDir, validate no traversal.
	resolved := filepath.Clean(filepath.Join(repoDir, rule))
	cleanRepo := filepath.Clean(repoDir)
	if !strings.HasPrefix(resolved, cleanRepo+string(os.PathSeparator)) {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file path escapes repo dir: %s%s\n", rule, ctx)
		return nil
	}

	content, err := readRuleFileSafe(resolved, confineRoot)
	if err == nil {
		return &content
	}
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file not found: %s%s\n", rule, ctx)
	} else {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read rule file %s: %v%s\n", resolved, err, ctx)
	}
	return nil
}

// readRuleFileSafe reads and validates a rule file: extension whitelist,
// maxRuleBodySize cap (512 KB, the same budget a multi-input merge shares), and
// symlink resolution. When confineRoot is non-empty, the resolved path must
// stay inside it. Returns the trimmed content on success.
func readRuleFileSafe(path string, confineRoot string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}

	if confineRoot != "" && !pathutil.WithinBase(confineRoot, resolved) {
		return "", fmt.Errorf("rule file path %q escapes repo dir %q", resolved, confineRoot)
	}

	if !allowedRuleExts[strings.ToLower(filepath.Ext(resolved))] {
		return "", fmt.Errorf("unsupported extension %q, only .md/.txt/.markdown allowed", filepath.Ext(resolved))
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.Size() > maxRuleBodySize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), maxRuleBodySize)
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(string(content), "\n"), nil
}
