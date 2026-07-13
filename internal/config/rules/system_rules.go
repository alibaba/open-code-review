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

// RuleDetail contains the resolved rule along with metadata about its source.
type RuleDetail struct {
	Rule    string // rule text
	Source  string // "custom" | "project" | "global" | "system"
	Pattern string // glob pattern that matched, or "default" for fallback
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
	lowerPath := strings.ToLower(path)
	for _, pr := range r.PathRules {
		expanded := expandBraces(pr.Pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return pr.Rule
			}
		}
	}
	return r.DefaultRule
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
func (f *FileFilter) IsUserExcluded(path string) bool {
	lowerPath := strings.ToLower(path)
	for _, pattern := range f.Exclude {
		expanded := expandBraces(pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(p, lowerPath); matched {
				return true
			}
		}
	}
	return false
}

// IsUserIncluded reports whether the given path matches any user include pattern.
// Returns false when Include is empty (no user include restriction defined).
func (f *FileFilter) IsUserIncluded(path string) bool {
	if !f.HasInclude() {
		return false
	}
	lowerPath := strings.ToLower(path)
	for _, pattern := range f.Include {
		expanded := expandBraces(pattern)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(p, lowerPath); matched {
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
	system  *SystemRule  // lowest: embedded default
}

// NewResolver builds a Resolver with the following priority:
//  1. Custom rule file specified via --rule flag (first match wins)
//  2. Project-local .opencodereview/rule.json (first match wins)
//  3. Global ~/.opencodereview/rule.json (first match wins)
//  4. Embedded system default rules
//
// It also returns a FileFilter with the merged include/exclude patterns from all layers.
func NewResolver(repoDir, customRulePath string) (Resolver, *FileFilter, error) {
	sysRule, err := LoadDefault()
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
		system:  sysRule,
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
	resolveRuleEntries(pr.Rules, filepath.Dir(path))
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
	resolveRuleEntries(pr.Rules, filepath.Dir(path))
	return &pr, nil
}

// loadProjectRule reads <repoDir>/.opencodereview/rule.json. Since #287 anchored
// RepoDir at the git top-level, `ocr review` from a monorepo subdirectory loads
// the repo-root rule file — which is consistent, since rule entries match against
// root-relative diff paths. A subproject-local rule.json under the subdirectory is
// intentionally not consulted; put shared rules at the repo root, or pass --rule.
func loadProjectRule(repoDir string) (*ProjectRule, error) {
	path := filepath.Join(repoDir, ".opencodereview", "rule.json")
	data, err := os.ReadFile(path)
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
	resolveRuleEntries(pr.Rules, repoDir)
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

func (c *composedResolver) matchProjectRuleDetail(pr *ProjectRule, path string, source string) *RuleDetail {
	entry := matchProjectRuleEntry(pr, path)
	if entry == nil {
		return nil
	}
	rule := entry.Rule
	if entry.MergeSystemRule {
		rule = c.mergeWithSystemRule(path, rule)
	}
	return &RuleDetail{Rule: rule, Source: source, Pattern: entry.Path}
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

// resolveRuleEntries scans each entry's Rule field. When the value looks like a file
// path, it reads the file content and replaces the Rule. Absolute paths are used
// directly; relative paths are resolved against repoDir only. Multi-line and short
// inline rules are left unchanged. If the file cannot be read, the Rule is cleared
// (set to empty) and a warning is emitted.
func resolveRuleEntries(entries []ProjectRuleEntry, repoDir string) {
	for i := range entries {
		e := &entries[i]
		if strings.TrimSpace(e.Rule) == "" || !looksLikeFilePath(e.Rule) {
			continue
		}
		if content := tryReadRuleFile(e.Rule, repoDir); content != nil {
			e.Rule = *content
		} else {
			e.Rule = ""
		}
	}
}

// tryReadRuleFile attempts to read a rule file. Absolute paths are used directly.
// Relative paths are resolved against repoDir and validated to stay within repoDir.
// Returns nil when the file cannot be read safely or does not exist.
func tryReadRuleFile(rule string, repoDir string) *string {
	if repoDir == "" {
		if !filepath.IsAbs(rule) {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot resolve relative rule path %q without a repo dir\n", rule)
			return nil
		}
	}
	if filepath.IsAbs(rule) {
		content, err := readRuleFileSafe(rule)
		if err == nil {
			return &content
		}
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file not found: %s\n", rule)
		} else {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read rule file %s: %v\n", rule, err)
		}
		return nil
	}

	// Relative path: resolve against repoDir, validate no traversal.
	resolved := filepath.Clean(filepath.Join(repoDir, rule))
	cleanRepo := filepath.Clean(repoDir)
	if !strings.HasPrefix(resolved, cleanRepo+string(os.PathSeparator)) {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file path escapes repo dir: %s\n", rule)
		return nil
	}

	content, err := readRuleFileSafe(resolved)
	if err == nil {
		return &content
	}
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: rule file not found: %s\n", rule)
	} else {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read rule file %s: %v\n", resolved, err)
	}
	return nil
}

// readRuleFileSafe reads and validates a rule file. It enforces extension whitelist
// (.md / .txt / .markdown), a 512 KB size cap, and resolves symlinks before checking
// the path. Symlinks are resolved first, then size is checked via Stat before reading.
// Returns the trimmed content on success.
func readRuleFileSafe(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}

	if !allowedRuleExts[strings.ToLower(filepath.Ext(resolved))] {
		return "", fmt.Errorf("unsupported extension %q, only .md/.txt/.markdown allowed", filepath.Ext(resolved))
	}

	const maxSize = 512 * 1024
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.Size() > maxSize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), maxSize)
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(string(content), "\n"), nil
}
