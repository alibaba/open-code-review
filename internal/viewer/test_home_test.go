// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import "testing"

// setTestHome redirects the user home resolved by os.UserHomeDir() to dir for
// the duration of the test. Setting HOME alone is NOT enough on Windows:
// os.UserHomeDir() prefers USERPROFILE there, so a test would still read and
// write the real ~/.opencodereview, and the sessions it creates would land
// outside the temp dir the test inspects. USERPROFILE is a no-op on non-Windows
// platforms.
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}
