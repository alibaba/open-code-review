package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ─── Semver tests ───────────────────────────────────────────────────────────

func TestParseSemver(t *testing.T) {
	cases := []struct {
		input              string
		major, minor, patc int
		ok                 bool
	}{
		{"v1.2.3", 1, 2, 3, true},
		{"1.2.3", 1, 2, 3, true},
		{"v1.7.17", 1, 7, 17, true},
		{"v0.0.0", 0, 0, 0, true},
		{"v1.2.3-beta.1", 1, 2, 3, true},
		{"v1.2.3+build.456", 1, 2, 3, true},
		{"1.2", 0, 0, 0, false},
		{"v1", 0, 0, 0, false},
		{"invalid", 0, 0, 0, false},
		{"", 0, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			maj, min, pat, ok := ParseSemver(tc.input)
			if ok != tc.ok {
				t.Fatalf("ParseSemver(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			}
			if ok && (maj != tc.major || min != tc.minor || pat != tc.patc) {
				t.Fatalf("ParseSemver(%q) = (%d,%d,%d), want (%d,%d,%d)",
					tc.input, maj, min, pat, tc.major, tc.minor, tc.patc)
			}
		})
	}
}

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v1.7.17", "v1.7.16", true},
		{"1.7.17", "1.7.16", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.8.0", "v1.7.9", true},
		{"v1.7.16", "v1.7.17", false},
		{"v1.7.17", "v1.7.17", false},
		{"v1.7.0", "v1.7.0", false},
		{"v0.9.0", "v1.0.0", false},
		// Dev builds: any valid release is considered newer.
		{"v1.0.0", "dev", true},
		{"v1.0.0", "", true},
		// Invalid candidate is never newer.
		{"invalid", "v1.0.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.candidate+"_"+tc.current, func(t *testing.T) {
			got := IsNewerVersion(tc.candidate, tc.current)
			if got != tc.want {
				t.Fatalf("IsNewerVersion(%q, %q) = %v, want %v",
					tc.candidate, tc.current, got, tc.want)
			}
		})
	}
}

// ─── Version normalisation tests (F8) ───────────────────────────────────────

func TestNormaliseVersion(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"1.7.17", "v1.7.17"},
		{"v1.7.17", "v1.7.17"},
		{" 1.7.17 ", "v1.7.17"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := NormaliseVersion(tc.input); got != tc.want {
			t.Fatalf("NormaliseVersion(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBareVersion(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"v1.7.17", "1.7.17"},
		{"1.7.17", "1.7.17"},
		{"  v1.7.17  ", "1.7.17"},
	}
	for _, tc := range cases {
		if got := BareVersion(tc.input); got != tc.want {
			t.Fatalf("BareVersion(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ─── Asset naming tests ─────────────────────────────────────────────────────

func TestAssetName(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "opencodereview-linux-amd64"},
		{"linux", "arm64", "opencodereview-linux-arm64"},
		{"darwin", "amd64", "opencodereview-darwin-amd64"},
		{"darwin", "arm64", "opencodereview-darwin-arm64"},
		{"windows", "amd64", "opencodereview-windows-amd64.exe"},
		{"windows", "arm64", "opencodereview-windows-arm64.exe"},
	}
	for _, tc := range cases {
		t.Run(tc.goos+"-"+tc.goarch, func(t *testing.T) {
			got := AssetName(tc.goos, tc.goarch)
			if got != tc.want {
				t.Fatalf("AssetName(%q, %q) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
			}
		})
	}
}

func TestBinaryFilenameMatchesRuntimePlatform(t *testing.T) {
	name := BinaryFilename()
	expected := AssetName(runtime.GOOS, runtime.GOARCH)
	if name != expected {
		t.Fatalf("BinaryFilename() = %q, want %q", name, expected)
	}
}

// ─── URL pattern consistency test ───────────────────────────────────────────

func TestURLPatternConsistency(t *testing.T) {
	root := projectRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	var pkg struct {
		OcrConfig struct {
			URLPattern string `json:"urlPattern"`
		} `json:"ocrConfig"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("parse package.json: %v", err)
	}

	testVersion := "1.7.17"
	renderedGo := strings.ReplaceAll(urlPattern, "{version}", testVersion)
	renderedGo = strings.ReplaceAll(renderedGo, "{os}", "linux")
	renderedGo = strings.ReplaceAll(renderedGo, "{arch}", "amd64")

	renderedPkg := strings.ReplaceAll(pkg.OcrConfig.URLPattern, "{version}", testVersion)
	renderedPkg = strings.ReplaceAll(renderedPkg, "{os}", "linux")
	renderedPkg = strings.ReplaceAll(renderedPkg, "{arch}", "amd64")

	if renderedGo != renderedPkg {
		t.Errorf("URL pattern mismatch:\n  Go:       %s\n  pkg.json: %s", renderedGo, renderedPkg)
	}
}

// ─── FetchLatestRelease tests (F5: now tests the real function) ─────────────

func TestFetchLatestReleaseSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v1.7.17","name":"Release v1.7.17"}`)
	}))
	defer server.Close()

	// Override the package-level var to point to the test server.
	orig := latestReleaseAPI
	latestReleaseAPI = server.URL
	defer func() { latestReleaseAPI = orig }()

	rel, err := FetchLatestRelease(http.DefaultClient)
	if err != nil {
		t.Fatalf("FetchLatestRelease: %v", err)
	}
	if rel.TagName != "v1.7.17" {
		t.Fatalf("TagName = %q, want %q", rel.TagName, "v1.7.17")
	}
}

func TestFetchLatestReleaseNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	orig := latestReleaseAPI
	latestReleaseAPI = server.URL
	defer func() { latestReleaseAPI = orig }()

	_, err := FetchLatestRelease(http.DefaultClient)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Fatalf("error should mention status 404, got: %v", err)
	}
}

func TestFetchLatestReleaseMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not json`)
	}))
	defer server.Close()

	orig := latestReleaseAPI
	latestReleaseAPI = server.URL
	defer func() { latestReleaseAPI = orig }()

	_, err := FetchLatestRelease(http.DefaultClient)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestFetchLatestReleaseEmptyTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":""}`)
	}))
	defer server.Close()

	orig := latestReleaseAPI
	latestReleaseAPI = server.URL
	defer func() { latestReleaseAPI = orig }()

	_, err := FetchLatestRelease(http.DefaultClient)
	if err == nil {
		t.Fatal("expected error for empty tag_name, got nil")
	}
}

// ─── Install method detection tests (F4: now tests real ClassifyInstallMethod) ─

func TestClassifyInstallMethodNPM(t *testing.T) {
	path := "/home/user/.nvm/versions/node/v24/lib/node_modules/@alibaba-group/ocr-linux-amd64/bin/opencodereview"
	if got := ClassifyInstallMethod(path); got != InstallNPM {
		t.Fatalf("ClassifyInstallMethod(%q) = %v, want %v", path, got, InstallNPM)
	}
}

func TestClassifyInstallMethodStatic(t *testing.T) {
	cases := []string{
		"/usr/local/bin/ocr",
		"/opt/ocr/ocr",
		"/home/user/bin/ocr",
		"/usr/local/bin/opencodereview",
	}
	for _, p := range cases {
		if got := ClassifyInstallMethod(p); got != InstallStatic {
			t.Fatalf("ClassifyInstallMethod(%q) = %v, want %v", p, got, InstallStatic)
		}
	}
}

func TestClassifyInstallMethodSource(t *testing.T) {
	cases := []string{
		"/home/user/projects/open-code-review/dist/opencodereview",
		"/home/user/go/src/ocr/build/opencodereview",
	}
	for _, p := range cases {
		if got := ClassifyInstallMethod(p); got != InstallSource {
			t.Fatalf("ClassifyInstallMethod(%q) = %v, want %v", p, got, InstallSource)
		}
	}
}

// ─── verifyChecksum tests (F6: now tests the real function) ─────────────────

func TestVerifyChecksumSuccess(t *testing.T) {
	binaryContent := []byte("fake binary content for testing")
	sum := sha256.Sum256(binaryContent)
	correctHash := hex.EncodeToString(sum[:])
	assetName := AssetName(runtime.GOOS, runtime.GOARCH)

	// Mock checksum server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", correctHash, assetName)
		fmt.Fprintf(w, "deadbeef  opencodereview-other-platform\n")
	}))
	defer server.Close()

	// Override checksumURL to point to the test server.
	orig := checksumURL
	checksumURL = server.URL + "/sha256sum.txt"
	defer func() { checksumURL = orig }()

	// Write the binary to a temp file.
	tmp := t.TempDir()
	assetPath := filepath.Join(tmp, assetName)
	if err := os.WriteFile(assetPath, binaryContent, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verifyChecksum(http.DefaultClient, "v1.7.17", assetName, assetPath); err != nil {
		t.Fatalf("verifyChecksum: %v", err)
	}
}

func TestVerifyChecksumMismatch(t *testing.T) {
	binaryContent := []byte("fake binary content")
	assetName := AssetName(runtime.GOOS, runtime.GOARCH)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately wrong hash.
		fmt.Fprintf(w, "0000000000000000000000000000000000000000000000000000000000000000  %s\n", assetName)
	}))
	defer server.Close()

	orig := checksumURL
	checksumURL = server.URL + "/sha256sum.txt"
	defer func() { checksumURL = orig }()

	tmp := t.TempDir()
	assetPath := filepath.Join(tmp, assetName)
	if err := os.WriteFile(assetPath, binaryContent, 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifyChecksum(http.DefaultClient, "v1.7.17", assetName, assetPath)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected 'checksum mismatch' in error, got: %v", err)
	}
}

func TestVerifyChecksumNoEntry(t *testing.T) {
	assetName := AssetName(runtime.GOOS, runtime.GOARCH)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Checksum file that doesn't contain our asset.
		fmt.Fprint(w, "deadbeef  some-other-asset\n")
	}))
	defer server.Close()

	orig := checksumURL
	checksumURL = server.URL + "/sha256sum.txt"
	defer func() { checksumURL = orig }()

	tmp := t.TempDir()
	assetPath := filepath.Join(tmp, assetName)
	if err := os.WriteFile(assetPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifyChecksum(http.DefaultClient, "v1.7.17", assetName, assetPath)
	if err == nil {
		t.Fatal("expected 'no checksum entry' error, got nil")
	}
}

// ─── moveFile / copyFile tests (F6) ─────────────────────────────────────────

func TestMoveFileSameDir(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	if err := os.WriteFile(src, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(src, dst); err != nil {
		t.Fatalf("moveFile: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should not exist after move")
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "content" {
		t.Fatalf("content = %q, want %q", string(data), "content")
	}
}

func TestMoveFileCrossDevice(t *testing.T) {
	// Simulate cross-device by making os.Rename fail — we can't easily force
	// EXDEV in a test, but we can test that copyFile works correctly and
	// moveFile falls back to it. Use a separate temp dir to increase the
	// chance of a different filesystem.
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()
	src := filepath.Join(tmp1, "src")
	dst := filepath.Join(tmp2, "dst")
	if err := os.WriteFile(src, []byte("cross-device content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(src, dst); err != nil {
		t.Fatalf("moveFile: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should not exist after move")
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "cross-device content" {
		t.Fatalf("content = %q, want %q", string(data), "cross-device content")
	}
}

func TestCopyFile(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	if err := os.WriteFile(src, []byte("copy me"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "copy me" {
		t.Fatalf("content = %q, want %q", string(data), "copy me")
	}
	// Source should still exist (copy, not move).
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should still exist after copy: %v", err)
	}
}

// ─── Download tests ─────────────────────────────────────────────────────────

func TestDownloadFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "binary content here")
	}))
	defer server.Close()

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "downloaded")
	if err := downloadFile(nil, server.URL, dest); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != "binary content here" {
		t.Fatalf("content = %q, want %q", string(data), "binary content here")
	}
}

func TestDownloadFileNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "downloaded")
	err := downloadFile(nil, server.URL, dest)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

// ─── Full download + verify integration test ────────────────────────────────

func TestDownloadAndVerifyIntegration(t *testing.T) {
	binaryContent := []byte("fake binary for integration test")
	sum := sha256.Sum256(binaryContent)
	correctHash := hex.EncodeToString(sum[:])

	assetName := AssetName(runtime.GOOS, runtime.GOARCH)

	binaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	}))
	defer binaryServer.Close()

	checksumServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", correctHash, assetName)
	}))
	defer checksumServer.Close()

	// Download the binary to a temp file.
	tmp := t.TempDir()
	assetPath := filepath.Join(tmp, assetName)
	if err := downloadFile(nil, binaryServer.URL, assetPath); err != nil {
		t.Fatalf("download binary: %v", err)
	}

	// Verify the file exists and has correct content.
	data, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("read asset: %v", err)
	}
	if string(data) != string(binaryContent) {
		t.Fatalf("content mismatch")
	}

	// Verify checksum using the real verifyChecksum function.
	orig := checksumURL
	checksumURL = checksumServer.URL + "/sha256sum.txt"
	defer func() { checksumURL = orig }()

	if err := verifyChecksum(http.DefaultClient, "v1.7.17", assetName, assetPath); err != nil {
		t.Fatalf("verifyChecksum: %v", err)
	}
}

// ─── Helpers ────────────────────────────────────────────────────────────────
