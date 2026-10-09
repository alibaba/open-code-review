// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RepoContext validates refs against the target repository. Phase 5 supplies the
// real repository root; phase 4 only needs the seam so tests stay offline.
type RepoContext interface {
	// ResolveCommit reports whether ref resolves to a commit in the target repo.
	ResolveCommit(ctx context.Context, ref string) error
}

// GitRepo validates refs with git rev-parse in Dir. An empty Dir uses the
// current working directory. --end-of-options keeps a ref that starts with a
// dash from being read as a flag.
type GitRepo struct{ Dir string }

// ResolveCommit implements RepoContext.
func (g GitRepo) ResolveCommit(ctx context.Context, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("empty ref")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", g.Dir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ref %q does not resolve to a commit: %s", ref, msg)
	}
	return nil
}
