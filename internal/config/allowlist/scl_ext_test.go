// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package allowedext

import "testing"

func TestSCLIsAllowedCaseInsensitive(t *testing.T) {
	for _, ext := range []string{".scl", ".SCL"} {
		t.Run(ext, func(t *testing.T) {
			if !IsAllowedExt(ext) {
				t.Fatalf("IsAllowedExt(%q) = false, want true", ext)
			}
		})
	}
}
