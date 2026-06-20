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
	Path string `json:"path"`
	Rule string `json:"rule"`
}

// ProjectRule holds rules loaded from <repoDir>/.opencodereview/rule.json.
type ProjectRule struct {
	Rules   []ProjectRuleEntry `json:"rules"`
	Include []string           `json:"include,omitempty"`
	Exclude []string           `json:"exclude,omitempty"`
}

type ResolverOptions struct {
	CustomRulePath string
	RulesDir       string
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
	custom            *ProjectRule // highest: --rule flag
	project           *ProjectRule // high: .opencodereview/rule.json
	enterpriseProject *ProjectRule // shared: <rules-dir>/projects/<project>/rule.json
	enterpriseGlobal  *ProjectRule // shared: <rules-dir>/global.json
	global            *ProjectRule // low: ~/.opencodereview/rule.json
	system            *SystemRule  // lowest: embedded default
}

// NewResolver builds a Resolver with the following priority:
//  1. Custom rule file specified via --rule flag (first match wins)
//  2. Project-local .opencodereview/rule.json (first match wins)
//  3. Enterprise project rule from <rules-dir>/projects/<project>/rule.json
//  4. Enterprise global rule from <rules-dir>/global.json
//  5. Global ~/.opencodereview/rule.json (first match wins)
//  6. Embedded system default rules
//
// It also returns a FileFilter with the merged include/exclude patterns from all layers.
func NewResolver(repoDir, customRulePath string) (Resolver, *FileFilter, error) {
	return NewResolverWithOptions(repoDir, ResolverOptions{CustomRulePath: customRulePath})
}

func NewResolverWithOptions(repoDir string, opts ResolverOptions) (Resolver, *FileFilter, error) {
	sysRule, err := LoadDefault()
	if err != nil {
		return nil, nil, err
	}

	var customRule *ProjectRule
	if opts.CustomRulePath != "" {
		cr, err := loadRuleFile(opts.CustomRulePath)
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

	var enterpriseProjectRule, enterpriseGlobalRule *ProjectRule
	if opts.RulesDir != "" {
		enterpriseProjectRule, enterpriseGlobalRule, err = loadRulesDir(opts.RulesDir, repoDir)
		if err != nil {
			return nil, nil, err
		}
	}

	globalRule, err := loadGlobalRule()
	if err != nil {
		return nil, nil, err
	}

	filter := buildFileFilter(customRule, projectRule, enterpriseProjectRule, enterpriseGlobalRule, globalRule)

	return &composedResolver{
		custom:            customRule,
		project:           projectRule,
		enterpriseProject: enterpriseProjectRule,
		enterpriseGlobal:  enterpriseGlobalRule,
		global:            globalRule,
		system:            sysRule,
	}, filter, nil
}

// buildFileFilter picks the highest-priority layer that has any include/exclude
// configured. Priority order: custom > project > enterprise project >
// enterprise global > global.
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
	return &pr, nil
}

func loadRulesDir(dir, repoDir string) (*ProjectRule, *ProjectRule, error) {
	global, err := loadOptionalRuleFile(filepath.Join(dir, "global.json"), "enterprise global rule")
	if err != nil {
		return nil, nil, err
	}

	var project *ProjectRule
	for _, path := range enterpriseProjectRulePaths(dir, repoDir) {
		pr, err := loadOptionalRuleFile(path, "enterprise project rule")
		if err != nil {
			return nil, nil, err
		}
		if pr != nil {
			project = pr
			break
		}
	}
	return project, global, nil
}

func enterpriseProjectRulePaths(dir, repoDir string) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(project string) {
		project = strings.Trim(project, "/")
		if project == "" || seen[project] {
			return
		}
		path, ok := enterpriseProjectRulePath(dir, project)
		if !ok {
			return
		}
		seen[project] = true
		paths = append(paths, path)
	}
	add(os.Getenv("OCR_PROJECT"))
	add(os.Getenv("CI_PROJECT_PATH"))
	if repoDir != "" {
		add(filepath.Base(repoDir))
	}
	return paths
}

func enterpriseProjectRulePath(dir, project string) (string, bool) {
	parts := strings.Split(project, "/")
	for _, part := range parts {
		if !isSafeRulePathSegment(part) {
			return "", false
		}
	}
	base := filepath.Clean(filepath.Join(dir, "projects"))
	path := filepath.Clean(filepath.Join(append([]string{base}, append(parts, "rule.json")...)...))
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return path, true
}

func isSafeRulePathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." {
		return false
	}
	if strings.Contains(segment, `\`) || filepath.IsAbs(segment) || filepath.Base(segment) != segment {
		return false
	}
	return true
}

func loadOptionalRuleFile(path, label string) (*ProjectRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s %s: %w", label, path, err)
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal %s %s: %w", label, path, err)
	}
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
	return &pr, nil
}

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
	return &pr, nil
}

// Resolve checks each layer in priority order; first match wins.
func (c *composedResolver) Resolve(path string) string {
	if rule := matchProjectRule(c.custom, path); rule != "" {
		return rule
	}
	if rule := matchProjectRule(c.project, path); rule != "" {
		return rule
	}
	if rule := matchProjectRule(c.enterpriseProject, path); rule != "" {
		return rule
	}
	if rule := matchProjectRule(c.enterpriseGlobal, path); rule != "" {
		return rule
	}
	if rule := matchProjectRule(c.global, path); rule != "" {
		return rule
	}
	return c.system.Resolve(path)
}

// ResolveDetail returns the matched rule along with its source layer and pattern.
func (c *composedResolver) ResolveDetail(path string) RuleDetail {
	if detail := matchProjectRuleDetail(c.custom, path, "custom"); detail != nil {
		return *detail
	}
	if detail := matchProjectRuleDetail(c.project, path, "project"); detail != nil {
		return *detail
	}
	if detail := matchProjectRuleDetail(c.enterpriseProject, path, "enterprise_project"); detail != nil {
		return *detail
	}
	if detail := matchProjectRuleDetail(c.enterpriseGlobal, path, "enterprise_global"); detail != nil {
		return *detail
	}
	if detail := matchProjectRuleDetail(c.global, path, "global"); detail != nil {
		return *detail
	}
	return c.system.resolveDetail(path)
}

func matchProjectRule(pr *ProjectRule, path string) string {
	if pr == nil {
		return ""
	}
	lowerPath := strings.ToLower(path)
	for _, entry := range pr.Rules {
		expanded := expandBraces(entry.Path)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return entry.Rule
			}
		}
	}
	return ""
}

func matchProjectRuleDetail(pr *ProjectRule, path, source string) *RuleDetail {
	if pr == nil {
		return nil
	}
	lowerPath := strings.ToLower(path)
	for _, entry := range pr.Rules {
		if entry.Rule == "" {
			continue
		}
		expanded := expandBraces(entry.Path)
		for _, p := range expanded {
			if matched, _ := doublestar.Match(strings.ToLower(p), lowerPath); matched {
				return &RuleDetail{Rule: entry.Rule, Source: source, Pattern: entry.Path}
			}
		}
	}
	return nil
}
