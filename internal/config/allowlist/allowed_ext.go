// Package allowedext provides file-level filtering for code review:
// an extension allowlist (which file types to review) and a path-based
// exclude list (which files to skip regardless of extension).
//
// # default_exclude_patterns.json 配置说明
//
// 文件包含一个 JSON 字符串数组，每个元素是一个 glob 排除模式。
// 支持的通配符语法（基于 doublestar 库）:
//
//   - 匹配单层路径内的任意字符（不跨越 /）
//     例: "*_test.go" 匹配 "foo_test.go"，不匹配 "pkg/foo_test.go"
//
//     **       匹配零个或多个路径段（可跨越 /）
//     例: "**/*_test.go" 匹配 "foo_test.go" 和 "a/b/c_test.go"
//
//     {a,b,c}  花括号展开，匹配其中任意一项
//     例: "**/*.{js,ts}" 匹配所有层级的 .js 和 .ts 文件
//
// 组合示例:
//
//	"**/*_test.go"                 — 任意层级的 Go 测试文件
//	"**/src/test/java/**/*.java"   — Java 标准测试目录下所有文件
//	"**/*.spec.{js,jsx,ts,tsx}"    — 任意层级的前端 spec 测试文件
//	"*_test.go"                    — 仅匹配根目录下的 Go 测试文件（不跨目录）
package allowedext

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

//go:embed supported_file_types.json
var defaultData []byte

//go:embed default_exclude_patterns.json
var excludeData []byte

var (
	supported map[string]bool
	initOnce  sync.Once
)

var (
	excludePatterns []string // raw patterns from JSON (may contain {a,b} syntax)
	excludeOnce     sync.Once
)

func initMap() {
	var exts []string
	if err := json.Unmarshal(defaultData, &exts); err != nil {
		panic("allowedext: failed to parse supported_file_types.json: " + err.Error())
	}
	supported = make(map[string]bool, len(exts))
	for _, e := range exts {
		supported[strings.ToLower(e)] = true
	}
}

func initExclude() {
	if err := json.Unmarshal(excludeData, &excludePatterns); err != nil {
		panic("allowedext: failed to parse default_exclude_patterns.json: " + err.Error())
	}
	for i, p := range excludePatterns {
		excludePatterns[i] = strings.ToLower(p)
	}
}

// IsAllowedExt returns true when the given file extension is in the supported types list.
// The check is case-insensitive.
func IsAllowedExt(ext string) bool {
	initOnce.Do(initMap)
	return supported[strings.ToLower(ext)]
}

// IsExcludedPath returns true when the given file path matches any default exclude pattern.
// Patterns support ** (recursive directory matching), * (single-segment wildcard),
// and {a,b,c} brace expansion. The check is case-insensitive.
//
// Example patterns and their behavior:
//
//	"**/*_test.go"       matches "foo_test.go", "pkg/bar_test.go", "a/b/c_test.go"
//	"*_test.go"          matches "foo_test.go" only (no directory traversal)
//	"**/*.test.{js,ts}"  matches "src/app.test.js", "lib/util.test.ts"
func IsExcludedPath(path string) bool {
	excludeOnce.Do(initExclude)
	lowerPath := strings.ToLower(path)
	for _, pattern := range excludePatterns {
		if matched, _ := doublestar.Match(pattern, lowerPath); matched {
			return true
		}
	}
	return false
}
