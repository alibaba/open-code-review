package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

type ocrConfig struct {
	URLPattern      string `json:"urlPattern"`
	ChecksumPattern string `json:"checksumPattern"`
}

type packageJSON struct {
	OcrConfig ocrConfig `json:"ocrConfig"`
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine project root")
	}
	return filepath.Join(filepath.Dir(file), "../..")
}

func loadPackageJSON(t *testing.T) packageJSON {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot(t), "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("parse package.json: %v", err)
	}
	return pkg
}

func loadFile(t *testing.T, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot(t), relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	return string(data)
}

// extractFilenameFromURL returns the filename part (last path segment) of a URL pattern.
func extractFilenameFromURL(pattern string) string {
	parts := strings.Split(pattern, "/")
	return parts[len(parts)-1]
}

func TestURLPatternNoVersionInFilename(t *testing.T) {
	pkg := loadPackageJSON(t)
	filename := extractFilenameFromURL(pkg.OcrConfig.URLPattern)
	if strings.Contains(filename, "{version}") {
		t.Errorf("binary filename %q should not contain {version}", filename)
	}
}

func TestURLPatternFormat(t *testing.T) {
	pkg := loadPackageJSON(t)
	platforms := []struct{ os, arch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
	}
	for _, p := range platforms {
		url := pkg.OcrConfig.URLPattern
		url = strings.ReplaceAll(url, "{version}", "1.0.7")
		url = strings.ReplaceAll(url, "{os}", p.os)
		url = strings.ReplaceAll(url, "{arch}", p.arch)
		filename := extractFilenameFromURL(url)
		expected := "opencodereview-" + p.os + "-" + p.arch
		if filename != expected {
			t.Errorf("urlPattern produces %q, want %q", filename, expected)
		}
	}
}

func TestReleaseYmlMatchesURLPattern(t *testing.T) {
	pkg := loadPackageJSON(t)
	yml := loadFile(t, ".github/workflows/release.yml")

	// Match: BIN_NAME=opencodereview-${{ matrix.goos }}-${{ matrix.goarch }}
	re := regexp.MustCompile(`BIN_NAME=(opencodereview[^\n]*\}\})`)
	m := re.FindStringSubmatch(yml)
	if m == nil {
		t.Fatal("cannot find BIN_NAME pattern in release.yml")
	}
	baseTemplate := strings.TrimSpace(m[1])

	hasWindowsExe := strings.Contains(yml, `BIN_NAME="${BIN_NAME}.exe"`)

	platforms := []struct{ os, arch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
		{"windows", "arm64"},
	}
	for _, p := range platforms {
		rendered := baseTemplate
		rendered = strings.ReplaceAll(rendered, "${{ matrix.goos }}", p.os)
		rendered = strings.ReplaceAll(rendered, "${{ matrix.goarch }}", p.arch)
		if p.os == "windows" && hasWindowsExe {
			rendered += ".exe"
		}

		url := pkg.OcrConfig.URLPattern
		url = strings.ReplaceAll(url, "{version}", "1.0.7")
		url = strings.ReplaceAll(url, "{os}", p.os)
		url = strings.ReplaceAll(url, "{arch}", p.arch)
		expected := extractFilenameFromURL(url)
		if p.os == "windows" {
			expected += ".exe"
		}

		if rendered != expected {
			t.Errorf("release.yml produces %q, urlPattern expects %q", rendered, expected)
		}
	}
}

func TestMakefileBuildMatchesURLPattern(t *testing.T) {
	pkg := loadPackageJSON(t)
	makefile := loadFile(t, "Makefile")

	re := regexp.MustCompile(`-o\s+\$\(DIST_DIR\)/([^\s\\]+)`)
	m := re.FindStringSubmatch(makefile)
	if m == nil {
		t.Fatal("cannot find BUILD_PLATFORM -o pattern in Makefile")
	}
	outputTemplate := m[1]

	platforms := []struct{ os, arch, ext string }{
		{"linux", "amd64", ""},
		{"linux", "arm64", ""},
		{"darwin", "amd64", ""},
		{"darwin", "arm64", ""},
		{"windows", "amd64", ".exe"},
		{"windows", "arm64", ".exe"},
	}
	for _, p := range platforms {
		rendered := outputTemplate
		rendered = strings.ReplaceAll(rendered, "$(BINARY_NAME)", "opencodereview")
		rendered = strings.ReplaceAll(rendered, "$(1)", p.os)
		rendered = strings.ReplaceAll(rendered, "$(2)", p.arch)
		rendered = strings.ReplaceAll(rendered, "$(3)", p.ext)

		url := pkg.OcrConfig.URLPattern
		url = strings.ReplaceAll(url, "{version}", "1.0.7")
		url = strings.ReplaceAll(url, "{os}", p.os)
		url = strings.ReplaceAll(url, "{arch}", p.arch)
		expected := extractFilenameFromURL(url)
		if p.ext != "" {
			expected += p.ext
		}

		if rendered != expected {
			t.Errorf("Makefile produces %q, urlPattern expects %q", rendered, expected)
		}
	}
}

func TestChecksumFilenameNoVersion(t *testing.T) {
	pkg := loadPackageJSON(t)
	filename := extractFilenameFromURL(pkg.OcrConfig.ChecksumPattern)
	if filename != "sha256sum.txt" {
		t.Errorf("checksumPattern filename = %q, want %q", filename, "sha256sum.txt")
	}
}

func TestURLPatternHTTPS(t *testing.T) {
	pkg := loadPackageJSON(t)
	if !strings.HasPrefix(pkg.OcrConfig.URLPattern, "https://github.com/alibaba/open-code-review/releases/download/") {
		t.Errorf("urlPattern should point to GitHub releases via HTTPS, got %q", pkg.OcrConfig.URLPattern)
	}
}
