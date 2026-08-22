// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"os"
	"testing"
)

// TestMain clears the three globals parseEnvOverrides validates before any test
// runs.
//
// These are unlike the rest of the environment this package reads. The others
// only contribute a value, so a stray export shifts one assertion at worst. An
// unparseable one of these aborts ResolveEndpointWithOptions before any
// resolution strategy runs, so a single bad export on a developer machine or a
// self-hosted runner fails around a hundred tests that never meant to involve
// it — with an error naming a variable the test does not mention.
//
// clearAllEnv resets them too, for the tests that call it. This is the backstop
// for the ones that do not: bedrock_test.go and friends build a config file and
// call ResolveEndpoint directly, and a test added tomorrow is under no
// obligation to remember either helper.
//
// Tests that want one of these set do so with t.Setenv, which takes effect
// after this and is restored per-test.
func TestMain(m *testing.M) {
	for _, k := range []string{envOCRLLMTimeout, envOCRLLMExtraHeaders, envOCRLLMPromptCache} {
		os.Unsetenv(k)
	}
	os.Exit(m.Run())
}
