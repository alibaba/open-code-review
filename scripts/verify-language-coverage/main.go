// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Command verify-language-coverage reports how the allowlist and the
// path-to-rule map fit together, so that adding a language becomes checkable
// instead of eyeballed.
//
// It answers two questions:
//   - which allowlisted extension resolves to a language rule, and which one
//     falls back to default.md;
//   - which declared rule doc no reviewed file reaches.
//
// Nothing fails by default. Pass -check to exit non-zero when a rule doc is
// unreachable, and -allow-unrouted=false to treat a fallback to the default rule
// as a finding too.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// systemRules mirrors the on-disk system_rules.json, keeping only what this
// command needs: which rule doc each glob pattern selects.
type systemRules struct {
	DefaultRule string            `json:"default_rule"`
	PathRuleMap map[string]string `json:"path_rule_map"`
}

type config struct {
	allowPath string
	rulesPath string
	docsDir   string
}

// report is the outcome of one run: the sections the command prints plus the
// findings that decide the exit status.
type report struct {
	allowlistSize int
	patternCount  int
	docCount      int
	routes        map[string][]string
	unrouted      []string
	unreachable   []string
}

func main() {
	root := flag.String("root", ".", "repository root")
	check := flag.Bool("check", false, "exit non-zero when a rule doc is unreachable")
	allowUnrouted := flag.Bool("allow-unrouted", true, "treat falling back to the default rule as intended, not a finding")
	flag.Parse()

	if err := run(os.Stdout, *root, *check, *allowUnrouted); err != nil {
		fmt.Fprintln(os.Stderr, "verify-language-coverage:", err)
		os.Exit(1)
	}
}

// run performs one audit, writes the report, and returns the finding that should
// decide the exit status. Keeping it separate from main lets the flags live in
// main, which keeps this function callable from tests with explicit arguments.
func run(out io.Writer, root string, check, allowUnrouted bool) error {
	rep, err := audit(configFor(root))
	if err != nil {
		return err
	}
	fmt.Fprint(out, render(rep))

	if !check {
		return nil
	}
	// Collect every finding rather than returning on the first one, so one run
	// reports all of them instead of hiding the rest behind an early return.
	var findings []error
	if len(rep.unrouted) > 0 && !allowUnrouted {
		findings = append(findings, fmt.Errorf("%d allowlisted extensions fall back to the default rule", len(rep.unrouted)))
	}
	if len(rep.unreachable) > 0 {
		findings = append(findings, fmt.Errorf("%d rule docs are unreachable", len(rep.unreachable)))
	}
	return errors.Join(findings...)
}

func configFor(root string) config {
	return config{
		allowPath: filepath.Join(root, "internal/config/allowlist/supported_file_types.json"),
		rulesPath: filepath.Join(root, "internal/config/rules/system_rules.json"),
		docsDir:   filepath.Join(root, "internal/config/rules/rule_docs"),
	}
}

// audit reads the configuration, checks that every referenced rule doc exists,
// then classifies every allowlisted extension and every declared pattern.
func audit(cfg config) (*report, error) {
	var allowed []string
	var rules systemRules
	if err := readJSON(cfg.allowPath, &allowed); err != nil {
		return nil, err
	}
	if err := readJSON(cfg.rulesPath, &rules); err != nil {
		return nil, err
	}

	declared := dedupe(append([]string{rules.DefaultRule}, docValues(rules.PathRuleMap)...))
	for _, doc := range declared {
		if _, err := os.Stat(filepath.Join(cfg.docsDir, doc)); err != nil {
			return nil, fmt.Errorf("rule doc %q: %w", doc, err)
		}
	}

	// Representative paths: one per allowlisted extension, plus one per pattern
	// so that rules keyed on a file name (pom.xml, package.json, ...) are
	// exercised too. Without the second set those rules look unreachable.
	probes := make([]string, 0, len(allowed)+len(rules.PathRuleMap))
	for _, ext := range allowed {
		probes = append(probes, "probe"+ext)
	}
	probes = append(probes, literalProbes(rules.PathRuleMap)...)
	probes = dedupe(probes)

	// JSON objects have no order, so the resolver's first-match order is not
	// visible here. Probing each pattern independently keeps the report
	// independent of that order.
	patterns := sortedKeys(rules.PathRuleMap)

	routes := map[string][]string{}
	for _, pattern := range patterns {
		for _, probe := range probes {
			if !matchesAny(pattern, probe) {
				continue
			}
			ext := extensionOf(probe)
			if ext == "" {
				continue
			}
			if doc := rules.PathRuleMap[pattern]; !contains(routes[ext], doc) {
				routes[ext] = append(routes[ext], doc)
			}
		}
	}

	unrouted := make([]string, 0)
	for _, ext := range allowed {
		if len(routes[ext]) == 0 {
			unrouted = append(unrouted, ext)
		}
	}
	sort.Strings(unrouted)

	reachable := map[string]bool{rules.DefaultRule: true}
	for _, docs := range routes {
		for _, doc := range docs {
			reachable[doc] = true
		}
	}
	unreachable := make([]string, 0)
	for _, doc := range declared {
		if !reachable[doc] {
			unreachable = append(unreachable, doc)
		}
	}

	return &report{
		allowlistSize: len(allowed),
		patternCount:  len(patterns),
		docCount:      len(declared),
		routes:        routes,
		unrouted:      unrouted,
		unreachable:   unreachable,
	}, nil
}

// render formats a report as the command's stdout.
func render(rep *report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "allowlist %d extensions | rules %d patterns, %d docs\n\n",
		rep.allowlistSize, rep.patternCount, rep.docCount)

	routed := make([]string, 0, len(rep.routes))
	for ext := range rep.routes {
		routed = append(routed, ext)
	}
	sort.Strings(routed)
	fmt.Fprintf(&b, "extensions with a language rule (%d):\n", len(routed))
	for _, ext := range routed {
		fmt.Fprintf(&b, "  %-12s %s\n", ext, strings.Join(dedupe(rep.routes[ext]), ", "))
	}

	fmt.Fprintf(&b, "\nextensions falling back to the default rule (%d):\n  %s\n",
		len(rep.unrouted), strings.Join(rep.unrouted, " "))
	fmt.Fprintf(&b, "\nrule docs no reviewed file reaches (%d):\n  %s\n",
		len(rep.unreachable), strings.Join(rep.unreachable, " "))
	return b.String()
}

// literalProbes turns a pattern into one concrete path, so that rules keyed on a
// file name rather than an extension are exercised too. A leading "**/" needs no
// directory, "**" elsewhere becomes a segment, and every other segment becomes a
// literal with its wildcards replaced by "probe", so "*{mapper,dao}*.xml" yields
// "probemapperprobe.xml" rather than leaving a glob character in the probe.
func literalProbes(pathRuleMap map[string]string) []string {
	out := make([]string, 0, len(pathRuleMap))
	for pattern := range pathRuleMap {
		out = append(out, probeForPattern(expandBraces(pattern)[0]))
	}
	return out
}

func probeForPattern(pattern string) string {
	segments := strings.Split(strings.ToLower(pattern), "/")
	parts := make([]string, 0, len(segments))
	for i, segment := range segments {
		switch {
		case segment == "**":
			if i < len(segments)-1 {
				parts = append(parts, "dir")
				continue
			}
			parts = append(parts, "probe")
		case segment == "":
			continue
		case strings.ContainsAny(segment, "*?"):
			parts = append(parts, wildcardsToProbe(segment))
		default:
			parts = append(parts, segment)
		}
	}
	return strings.Join(parts, "/")
}

// wildcardsToProbe replaces each run of glob metacharacters with "probe", keeping
// the literal part of the segment so the probe still ends in the right extension.
func wildcardsToProbe(segment string) string {
	parts := make([]string, 0, 4)
	literal := strings.Builder{}
	flush := func() {
		if literal.Len() > 0 {
			parts = append(parts, literal.String())
			literal.Reset()
		}
	}
	inWildcard := false
	for _, r := range segment {
		if r == '*' || r == '?' {
			if !inWildcard {
				flush()
				parts = append(parts, "probe")
				inWildcard = true
			}
			continue
		}
		inWildcard = false
		literal.WriteRune(r)
	}
	flush()
	return strings.Join(parts, "")
}

func extensionOf(path string) string {
	base := path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.Index(base, "."); i >= 0 {
		return base[i:]
	}
	return ""
}

func contains(in []string, want string) bool {
	for _, s := range in {
		if s == want {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func docValues(pathRuleMap map[string]string) []string {
	out := make([]string, 0, len(pathRuleMap))
	for _, doc := range pathRuleMap {
		out = append(out, doc)
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func matchesAny(pattern, path string) bool {
	for _, expanded := range expandBraces(strings.ToLower(pattern)) {
		if matched, _ := doublestar.Match(expanded, strings.ToLower(path)); matched {
			return true
		}
	}
	return false
}

// expandBraces expands one brace group per call, which is exactly what the
// resolver's own helper in internal/config/rules does. A pattern with two groups,
// such as "**/{src,test}/*.{java,kt}", is therefore only partly literal here; no
// pattern in system_rules.json currently nests groups, and staying non-recursive
// keeps this probe generator matching the resolver's semantics instead of
// diverging from them.
func expandBraces(pattern string) []string {
	open := strings.IndexByte(pattern, '{')
	if open < 0 {
		return []string{pattern}
	}
	close := strings.IndexByte(pattern[open:], '}')
	if close < 0 {
		return []string{pattern}
	}
	close += open
	out := make([]string, 0, 4)
	for _, option := range strings.Split(pattern[open+1:close], ",") {
		out = append(out, pattern[:open]+option+pattern[close+1:])
	}
	return out
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
