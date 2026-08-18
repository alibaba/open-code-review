// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
		{".php", true},
		{".PHP", true},
		{".phtml", true},
		{".PHTML", true},
		{".rs", true},
		{".R", true},
		{".r", true},
		{".ets", true},
		{".ETS", true},
		{".json5", true},
		{".JSON5", true},
		{".ftl", true},
		{".FTL", true},
		{".ftlh", true},
		{".FTLH", true},
		{".ftlx", true},
		{".FTLX", true},
		{".graphql", true},
		{".GRAPHQL", true},
		{".gql", true},
		{".GQL", true},
		{".prisma", true},
		{".PRISMA", true},
		{".jl", true},
		{".JL", true},
		{".hcl", true},
		{".HCL", true},
		{".tfvars", true},
		{".TFVARS", true},
		{".bicep", true},
		{".BICEP", true},
		{".proto", true},
		{".PROTO", true},
		{".nix", true},
		{".NIX", true},
		{".hs", true},
		{".HS", true},
		{".lhs", true},
		{".LHS", true},
		{".nim", true},
		{".NIM", true},
		{".nims", true},
		{".NIMS", true},
		{".nimble", true},
		{".NIMBLE", true},
		{".elm", true},
		{".ELM", true},
		{".properties", true},
		{".PROPERTIES", true},
		{".po", true},
		{".PO", true},
		{".pot", true},
		{".POT", true},
		{".jsonnet", true},
		{".JSONNET", true},
		{".libsonnet", true},
		{".LIBSONNET", true},
		{".zig", true},
		{".ZIG", true},
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

		// Prisma schemas have no conventional default test-file exclusion.
		{"prisma schema", "prisma/schema.prisma", false},

		// HarmonyOS oh_modules and test files
		{"oh_modules root", "oh_modules/some_lib/index.ets", true},
		{"oh_modules nested", "entry/oh_modules/lib/index.ets", true},
		{"ets test file", "entry/src/test/Component.test.ets", true},
		{"ets non-test", "entry/src/main/Component.ets", false},

		// Julia test files
		{"julia test file", "test/runtests.jl", true},
		{"julia test nested", "MyPkg/test/unit/foo.jl", true},
		{"julia non-test", "src/model.jl", false},

		// Swift test files
		{"swift Tests suffix", "MyAppTests/UserTests.swift", true},
		{"swift Tests suffix nested", "Tests/AppTests/UserTests.swift", true},
		{"swift UITests suffix", "MyAppUITests/LaunchTests.swift", true},
		{"swift Test suffix", "MyAppTests/UserTest.swift", true},
		{"swift Test dir helper", "Tests/AppTests/Mocks/MockService.swift", true},
		{"swift tests dir lowercase", "tests/AppTests/Helpers/Helper.swift", true},
		{"swift non-test", "Sources/App/User.swift", false},
		{"swift helper with test in name", "Sources/App/TestSupport.swift", false},

		// Haskell test files
		{"haskell test directory", "test/Parser.hs", true},
		{"haskell nested test directory", "packages/core/test/unit/Parser.hs", true},
		{"haskell spec file", "src/ParserSpec.hs", true},
		{"haskell root spec file", "ParserSpec.hs", true},
		{"haskell non-test", "src/Parser.hs", false},
		{"lhs test directory", "test/Tutorial.lhs", true},
		{"lhs nested test directory", "packages/core/test/unit/Tutorial.lhs", true},
		{"lhs spec file", "src/ParserSpec.lhs", true},
		{"lhs root spec file", "ParserSpec.lhs", true},
		{"lhs non-test", "src/Tutorial.lhs", false},

		// R test files
		{"r test directory", "tests/parser_test.R", true},
		{"r nested test directory", "packages/core/tests/unit/parser_test.R", true},
		{"r non-test", "src/parser.R", false},
		{"r tests in filename", "src/tests_helper.R", false},

		// Nim test files
		{"nim test directory", "tests/parser_test.nim", true},
		{"nim nested test directory", "packages/core/tests/unit/parser_test.nim", true},
		{"nim non-test", "src/parser.nim", false},
		{"nim tests in filename", "src/tests_helper.nim", false},

		// Elm test files
		{"elm test directory", "tests/ParserTest.elm", true},
		{"elm nested test directory", "packages/core/tests/unit/ParserTest.elm", true},
		{"elm non-test", "src/Parser.elm", false},
		{"elm tests in filename", "src/TestsHelper.elm", false},

		// Jsonnet vendored dependencies (written by `jb install`, wiped by `rm -rf vendor`).
		// The pattern is extension-scoped: IsExcludedPath applies every pattern to every
		// path, so a bare **/vendor/** would also drop vendored Go and PHP sources.
		{"jsonnet vendor root", "vendor/github.com/grafana/jsonnet-libs/ksonnet-util/kausal.libsonnet", true},
		{"jsonnet vendor nested dir", "jsonnet/vendor/foo/main.jsonnet", true},
		{"jsonnet non-vendor lib", "lib/config.libsonnet", false},
		{"jsonnet non-vendor env", "environments/prod/main.jsonnet", false},
		{"go under vendor still reviewed", "vendor/github.com/pkg/errors/errors.go", false},
		{"php under vendor still reviewed", "vendor/monolog/monolog/src/Logger.php", false},
		// Zig test files
		{"zig test directory", "test/parser.zig", true},
		{"zig nested test directory", "src/test/unit/parser.zig", true},
		{"zig _test suffix", "src/parser_test.zig", true},
		{"zig non-test", "src/parser.zig", false},
		{"zig test in filename", "src/testutil.zig", false},

		// Snapshot files
		{"jest snapshot dir", "src/__snapshots__/App.test.js.snap", true},
		{"snap file", "src/components/Button.snap", true},
		{"snap deeply nested", "packages/ui/src/__snapshots__/util.snap", true},

		// Test data directories
		{"testdata go", "internal/parser/testdata/input.json", true},
		{"testdata nested", "pkg/a/b/testdata/golden.txt", true},
		{"fixtures dir", "test/fixtures/sample.json", true},
		{"fixtures nested", "spec/fixtures/users.yml", true},

		// Generated code
		{"generated go", "api/types.generated.go", true},
		{"generated ts", "src/graphql/schema.generated.ts", true},
		{"gen go", "proto/message.gen.go", true},
		{"pb go", "api/v1/service.pb.go", true},
		{"pb cc", "proto/message.pb.cc", true},
		{"pb h", "proto/message.pb.h", true},

		// Non-matches for new patterns
		{"snapshots in name", "src/snapshots/util.ts", false},
		{"testdata in filename", "src/testdata.go", false},
		{"fixtures in filename", "src/fixtures.ts", false},
		{"generated not dotted", "src/generated/code.go", false},
		{"gen not suffix", "src/gen/util.go", false},
		{"pb not suffix", "src/pb/client.go", false},

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
