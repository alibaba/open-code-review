// Package release provides utilities for checking and downloading
// OpenCodeReview releases from GitHub. It supports version comparison,
// asset naming, checksum verification, and atomic binary replacement.
package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// GitHubRepo is the GitHub repository identifier used for release lookups.
const GitHubRepo = "alibaba/open-code-review"

// Default HTTP request timeout for version checks and downloads.
const defaultTimeout = 30 * time.Second

// Max download sizes to prevent unbounded reads from a compromised server.
const (
	maxBinarySize   = 500 * 1024 * 1024 // 500 MB for binary downloads
	maxAPIBodySize  = 1 * 1024 * 1024   // 1 MB for API JSON responses
	maxChecksumSize = 64 * 1024         // 64 KB for sha256sum.txt
)

// NPMPackageName is the NPM package name for ocr, used in update hints.
const NPMPackageName = "@alibaba-group/open-code-review"

// urlPattern is the release download URL template. It mirrors the
// ocrConfig.urlPattern in package.json; TestURLPatternConsistency validates
// they stay in sync.
const urlPattern = "https://github.com/" + GitHubRepo + "/releases/download/v{version}/opencodereview-{os}-{arch}"

// checksumURL is a var (not const) so tests can override it.
var checksumURL = "https://github.com/" + GitHubRepo + "/releases/download/v{version}/sha256sum.txt"

// latestReleaseAPI is a var (not const) so tests can override it.
var latestReleaseAPI = "https://api.github.com/repos/" + GitHubRepo + "/releases/latest"

// ─── Version comparison ─────────────────────────────────────────────────────

// semverRe matches a semantic version string, optionally prefixed with "v".
var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:[-+].+)?$`)

// ParseSemver extracts major, minor, patch from a version string like
// "v1.2.3" or "1.2.3-beta". Returns false if the string is not valid semver.
func ParseSemver(v string) (major, minor, patch int, ok bool) {
	m := semverRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return 0, 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	patch, _ = strconv.Atoi(m[3])
	return major, minor, patch, true
}

// IsNewerVersion reports whether candidate is strictly newer than current.
// Pre-release suffixes are ignored for comparison simplicity (matching the
// Node.js wrapper's semverGt behaviour). If current is not valid semver
// (e.g. "dev"), any valid candidate is considered newer.
func IsNewerVersion(candidate, current string) bool {
	cMaj, cMin, cPat, ok := ParseSemver(candidate)
	if !ok {
		return false
	}
	curMaj, curMin, curPat, ok := ParseSemver(current)
	if !ok {
		// Current version is not semver (e.g. "dev") — any valid release
		// is an upgrade.
		return true
	}
	if cMaj != curMaj {
		return cMaj > curMaj
	}
	if cMin != curMin {
		return cMin > curMin
	}
	return cPat > curPat
}

// ─── GitHub release lookup ──────────────────────────────────────────────────

// LatestRelease holds the minimal information from a GitHub release needed
// for the update flow.
type LatestRelease struct {
	TagName string // e.g. "v1.7.17"
}

// githubReleaseResponse is the JSON shape returned by the GitHub releases API.
type githubReleaseResponse struct {
	TagName string `json:"tag_name"`
}

// FetchLatestRelease queries the GitHub API for the latest release.
// Returns an error if the request fails or the response is malformed.
func FetchLatestRelease(client *http.Client) (*LatestRelease, error) {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	req, err := http.NewRequest(http.MethodGet, latestReleaseAPI, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ocr-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned status %d", resp.StatusCode)
	}

	var body githubReleaseResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIBodySize)).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode release response: %w", err)
	}
	if body.TagName == "" {
		return nil, fmt.Errorf("release response has empty tag_name")
	}
	return &LatestRelease{TagName: body.TagName}, nil
}

// ─── Asset naming ───────────────────────────────────────────────────────────

// AssetName returns the release asset filename for the current platform
// (or a specified os/arch). Windows binaries include the ".exe" suffix.
func AssetName(goos, goarch string) string {
	name := fmt.Sprintf("opencodereview-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// BinaryFilename returns the expected on-disk filename for the current
// platform's binary.
func BinaryFilename() string {
	return AssetName(runtime.GOOS, runtime.GOARCH)
}

// ─── Install method detection ───────────────────────────────────────────────

// InstallMethod describes how ocr was installed.
type InstallMethod string

const (
	InstallNPM    InstallMethod = "npm"
	InstallStatic InstallMethod = "static"
	InstallSource InstallMethod = "source"
)

// DetectInstallMethod determines how the running binary was installed by
// examining the executable path. NPM installs place the binary under a
// node_modules directory; static installs are typically in /usr/local/bin,
// /opt, or a user-chosen path.
func DetectInstallMethod() InstallMethod {
	exe, err := os.Executable()
	if err != nil {
		return InstallStatic
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return ClassifyInstallMethod(resolved)
}

// ClassifyInstallMethod classifies an install method from a resolved binary
// path. This is extracted from DetectInstallMethod so tests can verify the
// path heuristics without relying on os.Executable().
func ClassifyInstallMethod(resolved string) InstallMethod {
	lower := strings.ToLower(resolved)
	if strings.Contains(lower, "node_modules") {
		return InstallNPM
	}
	// Heuristic: if the binary is named "opencodereview" (not "ocr") and
	// lives in a dist/ or build output dir, it's likely a source build.
	base := filepath.Base(resolved)
	dir := filepath.Dir(resolved)
	if base == "opencodereview" && (strings.Contains(dir, "dist") || strings.Contains(dir, "build")) {
		return InstallSource
	}
	return InstallStatic
}

// ─── Download & replace ─────────────────────────────────────────────────────

// DownloadResult summarises a completed download.
type DownloadResult struct {
	Version string // the version that was downloaded (e.g. "v1.7.17")
	Path    string // path to the newly installed binary
}

// NormaliseVersion ensures a version string has a leading "v" prefix.
// Accepts "1.7.17" or "v1.7.17" and returns "v1.7.17".
func NormaliseVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// BareVersion strips a leading "v" prefix, returning e.g. "1.7.17" from
// "v1.7.17".
func BareVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// renderURL replaces {version}, {os}, {arch} placeholders in a URL template.
// The version is normalised to bare form (no "v" prefix) to match the
// package.json ocrConfig.urlPattern which expects bare version numbers
// (e.g. "1.7.17" not "v1.7.17").
func renderURL(template, version string) string {
	url := template
	url = strings.ReplaceAll(url, "{version}", BareVersion(version))
	url = strings.ReplaceAll(url, "{os}", runtime.GOOS)
	url = strings.ReplaceAll(url, "{arch}", runtime.GOARCH)
	return url
}

// downloadFile downloads a URL to a local path with a timeout.
func downloadFile(client *http.Client, url, dest string) error {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer out.Close()

	limited := io.LimitReader(resp.Body, maxBinarySize)
	n, err := io.Copy(out, limited)
	if err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if n >= maxBinarySize {
		return fmt.Errorf("download %s: file exceeds maximum allowed size (%d bytes)", url, maxBinarySize)
	}
	return nil
}

// verifyChecksum downloads sha256sum.txt for the given release version and
// verifies that the local file at assetPath matches the expected hash.
func verifyChecksum(client *http.Client, version, assetName, assetPath string) error {
	resolvedChecksumURL := renderURL(checksumURL, version)

	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	resp, err := client.Get(resolvedChecksumURL)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download checksums: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksumSize))
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}

	// Parse sha256sum format: "<hash>  <filename>" (two spaces, per GNU coreutils).
	want := ""
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s in sha256sum.txt", assetName)
	}

	f, err := os.Open(assetPath)
	if err != nil {
		return fmt.Errorf("read downloaded binary: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash downloaded binary: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("checksum mismatch for %s (got %s, want %s)", assetName, got, want)
	}
	return nil
}

// replaceBinary atomically replaces the current binary with the downloaded
// file. On Unix, it uses rename (atomic on the same filesystem). On Windows,
// it renames the running binary to *.old before moving the new one in.
func replaceBinary(newBinaryPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("determine executable path: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	// Preserve the original file mode.
	info, err := os.Stat(exe)
	if err != nil {
		return fmt.Errorf("stat current binary: %w", err)
	}
	mode := info.Mode()

	if err := os.Chmod(newBinaryPath, mode); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}

	if runtime.GOOS == "windows" {
		// Windows can't overwrite a running executable, but can rename it.
		oldPath := exe + ".old"
		_ = os.Remove(oldPath) // clean up any previous leftover
		if err := moveFile(exe, oldPath); err != nil {
			return fmt.Errorf("rename current binary: %w", err)
		}
		if err := moveFile(newBinaryPath, exe); err != nil {
			// Try to restore the original on failure.
			_ = moveFile(oldPath, exe)
			return fmt.Errorf("install new binary: %w", err)
		}
		_ = os.Remove(oldPath) // best-effort cleanup
		return nil
	}

	// Unix: rename is atomic when source and dest are on the same filesystem.
	// Place the temp file in the same directory to guarantee this.
	dir := filepath.Dir(exe)
	tmpInDir := filepath.Join(dir, ".ocr-update-"+BinaryFilename())
	if err := moveFile(newBinaryPath, tmpInDir); err != nil {
		return fmt.Errorf("stage new binary: %w", err)
	}
	if err := os.Rename(tmpInDir, exe); err != nil {
		_ = os.Rename(tmpInDir, newBinaryPath) // try to restore temp
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

// moveFile moves a file, falling back to copy+delete only for cross-device
// errors (EXDEV). Other rename errors are returned immediately to avoid
// masking real problems like permission denied or file busy.
func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if !isCrossDeviceError(err) {
		return err
	}
	// Fall back to copy + delete for cross-device moves.
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// isCrossDeviceError reports whether err is a cross-device link error
// (EXDEV on Unix).
func isCrossDeviceError(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		var sysErr syscall.Errno
		if errors.As(linkErr.Err, &sysErr) {
			return sysErr == syscall.EXDEV
		}
	}
	return false
}

// copyFile copies a regular file, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// DownloadAndReplace downloads the binary for the given version from GitHub
// releases, verifies its sha256 checksum, and atomically replaces the
// currently running binary. It returns the installation path.
func DownloadAndReplace(client *http.Client, version string) (*DownloadResult, error) {
	assetName := BinaryFilename()
	binaryURL := renderURL(urlPattern, version)

	tmpDir, err := os.MkdirTemp("", "ocr-update-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpBinary := filepath.Join(tmpDir, assetName)

	fmt.Printf("Downloading ocr %s (%s/%s)...\n", version, runtime.GOOS, runtime.GOARCH)

	if err := downloadFile(client, binaryURL, tmpBinary); err != nil {
		return nil, err
	}

	fmt.Printf("Verifying checksum...\n")
	if err := verifyChecksum(client, version, assetName, tmpBinary); err != nil {
		return nil, err
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("determine executable path: %w", err)
	}
	fmt.Printf("Installing to %s...\n", exe)

	if err := replaceBinary(tmpBinary); err != nil {
		return nil, err
	}

	return &DownloadResult{Version: version, Path: exe}, nil
}
