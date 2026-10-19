// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alibaba/open-code-review/internal/pathutil"
	"github.com/alibaba/open-code-review/internal/plugin"
)

// PluginRuleDetail identifies a rule pack contributing to a resolved rule.
type PluginRuleDetail struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	File    string `json:"file"`
	Pattern string `json:"pattern"`
}

type pluginRulePack struct {
	plugin plugin.Installed
	file   string
	rules  *ProjectRule
}

// pluginResolver keeps the regular priority chain intact and appends matching
// plugin rules. The installed snapshot also supplies the runtime's MCP config.
type pluginResolver struct {
	base         *composedResolver
	plugins      []plugin.Installed
	packs        []pluginRulePack
	reviewID     string
	reviewPlugin *plugin.Installed
}

func loadPluginRules(base *composedResolver, repoDir, reviewID string) (Resolver, error) {
	store, err := plugin.DefaultStore()
	if err != nil {
		return nil, fmt.Errorf("load plugins: %w", err)
	}
	var all []plugin.Installed
	var projectScoped bool
	if repoDir != "" {
		project, err := store.Project(repoDir)
		if err != nil {
			return nil, err
		}
		all, projectScoped, err = project.Load(context.Background())
		if err != nil {
			return nil, fmt.Errorf("load project plugins: %w", err)
		}
	}
	if !projectScoped {
		all, err = store.EnabledWith(reviewID)
		if err != nil {
			return nil, fmt.Errorf("load plugins: %w", err)
		}
	}
	var installed []plugin.Installed
	var selected *plugin.Installed
	for _, p := range all {
		if p.Manifest.ID == reviewID {
			_, _, err = plugin.ReviewCommand(p)
			if err != nil {
				return nil, fmt.Errorf("review plugin %q: %w", reviewID, err)
			}
			p.Enabled = true
		}
		if p.Enabled {
			if err := p.Manifest.CheckCompatibility(plugin.HostVersion); err != nil {
				return nil, err
			}
			p.ContentSHA256, err = plugin.ContentDigest(context.Background(), p.Dir)
			if err != nil {
				return nil, fmt.Errorf("identify plugin %q contents: %w", p.Manifest.ID, err)
			}
			if p.Manifest.ID == reviewID {
				selected = &p
			}
			installed = append(installed, p)
		}
	}
	if reviewID != "" && selected == nil {
		if projectScoped {
			return nil, fmt.Errorf("review plugin %q is not present in the project plugin lock", reviewID)
		}
		return nil, fmt.Errorf("review plugin %q is not installed", reviewID)
	}
	if !projectScoped {
		sort.Slice(installed, func(i, j int) bool { return installed[i].Manifest.ID < installed[j].Manifest.ID })
	}
	r := &pluginResolver{base: base, plugins: installed, reviewID: reviewID, reviewPlugin: selected}
	for _, p := range installed {
		for _, file := range p.Manifest.Rules {
			pack, err := loadPluginRulePack(p.Dir, file)
			if err != nil {
				return nil, fmt.Errorf("plugin %q rule pack %q: %w", p.Manifest.ID, file, err)
			}
			r.packs = append(r.packs, pluginRulePack{plugin: p, file: file, rules: pack})
		}
	}
	return r, nil
}

func loadPluginRulePack(dir, file string) (*ProjectRule, error) {
	root, err := pathutil.CanonicalPath(dir)
	if err != nil {
		return nil, err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(dir, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	if !pathutil.WithinBase(root, path) {
		return nil, fmt.Errorf("rule pack escapes the installed package")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > 512*1024 {
		return nil, fmt.Errorf("rule pack exceeds 512 KiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pack ProjectRule
	if err := json.Unmarshal(data, &pack); err != nil {
		return nil, fmt.Errorf("decode rules: %w", err)
	}
	if len(pack.Include) != 0 || len(pack.Exclude) != 0 {
		return nil, fmt.Errorf("plugin rule packs support rules only; include/exclude must be configured in the project or user rules")
	}
	for i := range pack.Rules {
		entry := &pack.Rules[i]
		for _, pattern := range expandBraces(entry.Path) {
			if _, err := doublestar.Match(pattern, ""); err != nil {
				return nil, fmt.Errorf("invalid rule pattern %q: %w", entry.Path, err)
			}
		}
		if looksLikeFilePath(entry.Rule) {
			if filepath.IsAbs(entry.Rule) || filepath.VolumeName(entry.Rule) != "" || strings.HasPrefix(entry.Rule, "/") || strings.HasPrefix(entry.Rule, "\\") || strings.Contains(entry.Rule, ":") {
				return nil, fmt.Errorf("rule reference %q must be relative to its rule pack", entry.Rule)
			}
			text, err := readRuleFileSafe(filepath.Join(filepath.Dir(path), entry.Rule), root)
			if err != nil {
				return nil, fmt.Errorf("read rule %q: %w", entry.Rule, err)
			}
			entry.Rule = text
		}
		// Plugin rules are always additive, so they do not select a second
		// copy of the system rule when merge_system_rule is present.
		entry.MergeSystemRule = false
	}
	return &pack, nil
}

// EnabledPlugins returns the installed packages loaded with these rules. Runtime
// initialization uses this snapshot so tool configuration matches resume identity.
func (r *pluginResolver) EnabledPlugins() []plugin.Installed {
	return append([]plugin.Installed(nil), r.plugins...)
}

// ReviewPlugin is set only by explicit invocation selection.
func (r *pluginResolver) ReviewPlugin() *plugin.Installed { return r.reviewPlugin }

func (r *pluginResolver) Resolve(path string) string {
	return r.ResolveDetail(path).Rule
}

func (r *pluginResolver) ResolveDetail(path string) RuleDetail {
	detail := r.base.ResolveDetail(path)
	for _, pack := range r.packs {
		entry := matchProjectRuleEntry(pack.rules, path)
		if entry == nil {
			continue
		}
		id := pack.plugin.Manifest.ID
		detail.PluginRules = append(detail.PluginRules, PluginRuleDetail{
			ID: id, Version: pack.plugin.Manifest.Version, File: pack.file, Pattern: entry.Path,
		})
		if detail.Rule != "" {
			detail.Rule += "\n\n---\n\n"
		}
		detail.Rule += "## Plugin Rules (" + id + ": " + pack.file + ")\n\n" + entry.Rule
	}
	return detail
}

func (r *pluginResolver) CanonicalConfig() []string {
	fields := r.base.CanonicalConfig()
	if r.reviewID != "" {
		fields = append(fields, "review_plugin", r.reviewID)
	}
	for _, p := range r.plugins {
		// Manifests are validated by the store. JSON map keys are sorted by
		// encoding/json; this also covers enabled plugins with only MCP tools.
		manifest, _ := json.Marshal(p.Manifest)
		fields = append(fields, "plugin", p.Manifest.ID, "manifest", string(manifest), "content", p.ContentSHA256)
		// Executable contributions can observe their installation path.
		if len(p.Manifest.MCPServers) > 0 || p.Manifest.ID == r.reviewID {
			fields = append(fields, "directory", p.Dir)
		}
	}
	for _, pack := range r.packs {
		fields = append(fields, "plugin", pack.plugin.Manifest.ID, "file", pack.file)
		for _, entry := range pack.rules.Rules {
			fields = append(fields, "path", entry.Path, "rule", entry.Rule)
		}
	}
	return fields
}
