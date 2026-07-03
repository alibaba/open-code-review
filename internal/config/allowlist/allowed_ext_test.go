package allowedext

import (
	"testing"
)

func TestIsAllowedExt(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".go", true},
		{".GO", true},
		{".java", true},
		{".ts", true},
		{".tsx", true},
		{".astro", true},
		{".ASTRO", true},
		{".py", true},
		{".rs", true},
		{".ets", true},
		{".ETS", true},
		{".json5", true},
		{".JSON5", true},
		{".txt", false},
		{".md", false},
		{".png", false},
		{".lock", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			if got := IsAllowedExt(tt.ext); got != tt.want {
				t.Errorf("IsAllowedExt(%q) = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}

func TestIsExcludedPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		// Go test files
		{"go test in subdir", "foo/bar_test.go", true},
		{"go test at root", "bar_test.go", true},
		{"go test deeply nested", "a/b/c/d_test.go", true},
		{"go non-test file", "foo/bar.go", false},
		{"go file with test in name", "foo/testutil.go", false},

		// Java test directory
		{"java test dir", "src/test/java/com/example/FooTest.java", true},
		{"java main dir", "src/main/java/com/example/Foo.java", false},

		// Java *Test.java pattern
		{"java Test suffix", "com/example/FooTest.java", true},
		{"java Tests suffix", "com/example/FooTests.java", true},
		{"java non-test", "com/example/Foo.java", false},

		// Kotlin test directory
		{"kotlin test dir", "src/test/kotlin/FooTest.kt", true},
		{"kotlin main dir", "src/main/kotlin/Foo.kt", false},

		// JS/TS test files
		{"js test file", "src/utils.test.js", true},
		{"tsx test file", "src/Component.test.tsx", true},
		{"ts spec file", "src/utils.spec.ts", true},
		{"jsx spec file", "src/App.spec.jsx", true},
		{"ts non-test", "src/utils.ts", false},

		// __tests__ directory
		{"__tests__ dir", "src/__tests__/foo.js", true},
		{"__tests__ nested", "packages/ui/__tests__/Button.test.tsx", true},

		// Python test files
		{"python test file", "tests/test_utils.py", false}, // pattern is *_test.py, not test_*.py
		{"python _test suffix", "app/handler_test.py", true},
		{"python test dir", "test/unit/handler_test.py", true},
		{"python tests dir", "tests/unit/handler_test.py", true},
		{"python non-test", "app/handler.py", false},

		// Ruby spec files
		{"ruby spec file", "app/models/user_spec.rb", true},
		{"ruby spec dir", "spec/models/user_spec.rb", true},
		{"ruby non-spec", "app/models/user.rb", false},

		// Rust test files
		{"rust test file", "src/parser_test.rs", true},
		{"rust non-test", "src/parser.rs", false},

		// HarmonyOS oh_modules and test files
		{"oh_modules root", "oh_modules/some_lib/index.ets", true},
		{"oh_modules nested", "entry/oh_modules/lib/index.ets", true},
		{"ets test file", "entry/src/test/Component.test.ets", true},
		{"ets non-test", "entry/src/main/Component.ets", false},

		// Case insensitive
		{"case insensitive go", "Foo/Bar_Test.go", true},
		{"case insensitive java", "com/FooTEST.java", true}, // lowercase → "com/footest.java" matches "**/*test.java"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsExcludedPath(tt.path); got != tt.want {
				t.Errorf("IsExcludedPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
