// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type githubSource struct {
	owner     string
	repo      string
	ref       string
	directory string
	source    string
}

func parseGitHubSource(source string) (githubSource, bool, error) {
	var result githubSource
	u, err := url.Parse(source)
	if err != nil || !strings.EqualFold(u.Host, "github.com") {
		return result, false, nil
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) < 2 {
		return result, true, fmt.Errorf("GitHub source requires an owner and repository")
	}
	for index := range parts {
		parts[index], err = url.PathUnescape(parts[index])
		if err != nil {
			return result, true, fmt.Errorf("invalid GitHub source path")
		}
	}
	result.owner, result.repo = parts[0], strings.TrimSuffix(parts[1], ".git")
	for _, name := range []string{result.owner, result.repo} {
		if strings.Contains(name, "/") || safeRelativePath(name) != nil {
			return result, true, fmt.Errorf("invalid GitHub owner or repository")
		}
	}
	result.ref, result.source = "HEAD", source
	if len(parts) > 2 {
		// Release assets and archive URLs are ordinary HTTPS archive sources.
		if parts[2] == "releases" || parts[2] == "archive" {
			return githubSource{}, false, nil
		}
		if parts[2] != "tree" || len(parts) < 4 || parts[3] == "" {
			return result, true, fmt.Errorf("GitHub source must be a repository URL or /tree/REF/PATH URL")
		}
		result.ref = parts[3]
		if strings.ContainsAny(result.ref, "\x00\r\n") || len(result.ref) > 256 {
			return result, true, fmt.Errorf("invalid GitHub ref")
		}
		result.directory = strings.Join(parts[4:], "/")
		if result.directory != "" {
			if err := safeRelativePath(result.directory); err != nil {
				return result, true, fmt.Errorf("invalid GitHub plugin directory: %w", err)
			}
		}
	}
	return result, true, nil
}

func (s *Store) prepareGitHub(ctx context.Context, repository githubSource, pluginID, directory string, artifact *packageArtifact) (string, string, string, string, error) {
	base := url.PathEscape(repository.owner) + "/" + url.PathEscape(repository.repo)
	ref := repository.ref
	revision := ""
	if decoded, err := hex.DecodeString(ref); err == nil && len(decoded) == 20 {
		revision = strings.ToLower(ref)
	} else {
		data, _, err := s.download(ctx, "https://api.github.com/repos/"+base+"/commits/"+url.PathEscape(ref), maxManifestBytes)
		if err != nil {
			return "", "", "", "", fmt.Errorf("resolve GitHub revision: %w; retry when the API is available or use a /tree/FULL_COMMIT_SHA URL", err)
		}
		var commit struct {
			SHA string `json:"sha"`
		}
		if err := json.Unmarshal(data, &commit); err != nil {
			return "", "", "", "", fmt.Errorf("parse GitHub revision: %w", err)
		}
		decoded, err := hex.DecodeString(commit.SHA)
		if err != nil || len(decoded) != 20 {
			return "", "", "", "", fmt.Errorf("GitHub returned an invalid commit revision")
		}
		revision = strings.ToLower(commit.SHA)
	}
	ref = revision
	artifact.Source = "https://codeload.github.com/" + base + "/tar.gz/" + url.PathEscape(ref)
	archive, _, err := s.download(ctx, artifact.Source, maxArchiveBytes)
	if err != nil {
		return "", "", "", "", err
	}
	if err := extractTARGZ(ctx, archive, directory); err != nil {
		return "", "", "", "", err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", "", "", "", err
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return "", "", "", "", fmt.Errorf("GitHub archive must contain one repository directory")
	}
	repoRoot := filepath.Join(directory, entries[0].Name())
	packageRoot := filepath.Join(repoRoot, filepath.FromSlash(repository.directory))
	if info, err := os.Stat(packageRoot); err != nil || !info.IsDir() {
		return "", "", "", "", fmt.Errorf("GitHub plugin directory %q does not exist", repository.directory)
	}
	selected, err := selectPackage(ctx, packageRoot, repository.source, pluginID)
	if err != nil {
		return "", "", "", "", err
	}
	manifest, err := inspectPackage(ctx, selected)
	if err != nil {
		return "", "", "", "", err
	}
	return selected, archiveDigest(archive), revision, manifest.ID, nil
}
