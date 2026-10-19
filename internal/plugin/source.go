// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DisplaySource removes URL credentials and query values from diagnostics and
// listings. Local filesystem paths keep their spelling.
func DisplaySource(source string) string {
	u, err := url.Parse(source)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return source
	}
	return displayURL(u)
}

func displayURL(u *url.URL) string {
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}

type downloadError struct {
	message string
	cause   error
}

type httpStatusError struct {
	status int
	source string
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("download %s: HTTP %d", DisplaySource(e.source), e.status)
}

func (e downloadError) Error() string { return e.message }
func (e downloadError) Unwrap() error { return e.cause }

func redactDownloadError(err error, source string) error {
	message := strings.ReplaceAll(err.Error(), source, DisplaySource(source))
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		if requestError, ok := cause.(*url.Error); ok {
			// net/http may report the relative Location value for a redirect,
			// rather than an absolute URL. It still carries a URL query.
			if parsed, parseErr := url.Parse(requestError.URL); parseErr == nil {
				message = strings.ReplaceAll(message, requestError.URL, displayURL(parsed))
			}
		}
	}
	return downloadError{message: message, cause: err}
}

func canonicalSource(source string) (string, error) {
	if id, market, ok := strings.Cut(source, "@"); ok && validID(id) && validID(market) {
		return "", fmt.Errorf("marketplace references are no longer supported; use a plugin URL or explicit local path")
	}
	if strings.HasPrefix(strings.ToLower(source), "https:") {
		if err := validateHTTPS(source); err != nil {
			return "", err
		}
		return source, nil
	}
	// Do not interpret another URL scheme as a local filename. Windows drive
	// letters are still accepted by filepath.IsAbs on Windows.
	if strings.Contains(source, "://") || strings.HasPrefix(strings.ToLower(source), "http:") {
		return "", fmt.Errorf("remote sources must use HTTPS")
	}
	if strings.TrimSpace(source) == "" {
		return "", fmt.Errorf("source is required")
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	return absolute, nil
}

func validateHTTPS(source string) error {
	u, err := url.Parse(source)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("remote source must be an HTTPS URL without credentials or fragment")
	}
	return nil
}

func (s *Store) download(ctx context.Context, source string, limit int64) ([]byte, string, error) {
	if err := validateHTTPS(source); err != nil {
		return nil, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, "", err
	}
	client := *s.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return validateHTTPS(req.URL.String())
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("download %s: %w", DisplaySource(source), redactDownloadError(err, source))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", httpStatusError{status: response.StatusCode, source: source}
	}
	if response.ContentLength > limit {
		return nil, "", fmt.Errorf("download exceeds %d byte limit", limit)
	}
	data, err := readBounded(response.Body, limit)
	if err != nil {
		return nil, "", fmt.Errorf("download %s: %w", DisplaySource(source), redactDownloadError(err, source))
	}
	return data, response.Request.URL.String(), nil
}

func parseDigest(value string) ([]byte, error) {
	digest, err := hex.DecodeString(value)
	if err != nil || len(digest) != sha256.Size {
		return nil, fmt.Errorf("sha256 must contain exactly 64 hexadecimal characters")
	}
	return digest, nil
}

func (s *Store) prepare(ctx context.Context, source, digest, stage string) error {
	_, err := s.preparePackage(ctx, source, digest, stage)
	return err
}

func (s *Store) preparePackage(ctx context.Context, source, digest, stage string) (string, error) {
	if strings.HasPrefix(strings.ToLower(source), "https:") {
		data, _, err := s.download(ctx, source, maxArchiveBytes)
		if err != nil {
			return "", err
		}
		if digest != "" {
			if err := verifyDigest(data, digest); err != nil {
				return "", err
			}
		}
		return archiveDigest(data), extractArchive(ctx, data, stage)
	}
	info, err := os.Lstat(source)
	if err != nil {
		return "", fmt.Errorf("read plugin source: %w", err)
	}
	if info.IsDir() {
		if digest != "" {
			return "", fmt.Errorf("sha256 applies to archives, not directory sources")
		}
		absoluteStore, err := filepath.Abs(s.root)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(source, absoluteStore)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return "", fmt.Errorf("plugin source directory cannot contain the plugin store")
		}
		absoluteStage, err := filepath.Abs(stage)
		if err != nil {
			return "", err
		}
		rel, err = filepath.Rel(source, absoluteStage)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return "", fmt.Errorf("plugin source directory cannot contain the staging directory")
		}
		root, err := openPackageRoot(source)
		if err != nil {
			return "", err
		}
		defer root.Close()
		if _, err := inspectPackageRoot(ctx, root); err != nil {
			return "", err
		}
		return "", copyPackageRoot(ctx, root, stage)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("plugin source is a symlink or special file")
	}
	data, err := readFileBounded(source, maxArchiveBytes)
	if err != nil {
		return "", err
	}
	if digest != "" {
		if err := verifyDigest(data, digest); err != nil {
			return "", err
		}
	}
	return archiveDigest(data), extractArchive(ctx, data, stage)
}

func archiveDigest(data []byte) string {
	actual := sha256.Sum256(data)
	return hex.EncodeToString(actual[:])
}

func verifyDigest(data []byte, expected string) error {
	if _, err := parseDigest(expected); err != nil {
		return err
	}
	actual := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(actual[:]), expected) {
		return fmt.Errorf("plugin archive sha256 does not match expected digest")
	}
	return nil
}
