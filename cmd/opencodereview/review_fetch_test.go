// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
)

// fetchFixture reproduces the stale-base incident: the clone's local main and origin/main both
// trail the remote, and its feature branch was cut from the remote main of that
// time. Reviewing feature against the stale local main drags in shared.go, which
// is already on the remote base.
type fetchFixture struct {
	home      string // OCR home, to assert nothing was persisted
	remote    string // bare repository both clones talk to
	publisher string // pushes new commits to the remote's main
	user      string // the clone under review, on branch feature
	staleMain string // the user's local main
	forkPoint string // remote main when feature was cut: the correct merge base
	remoteTip string // remote main now
}

func newFetchFixture(t *testing.T) fetchFixture {
	t.Helper()
	fx := fetchFixture{home: freshOCRHome(t)}
	root := t.TempDir()
	fx.remote = filepath.Join(root, "remote.git")
	fx.publisher = filepath.Join(root, "publisher")
	fx.user = filepath.Join(root, "user")

	gitIn(t, root, "init", "--bare", "-b", "main", fx.remote)
	gitIn(t, root, "init", "-b", "main", fx.publisher)
	gitIn(t, fx.publisher, "remote", "add", "origin", fx.remote)
	commitFile(t, fx.publisher, "base.go", "package base\n", "base")
	gitIn(t, fx.publisher, "push", "origin", "main")

	gitIn(t, root, "clone", fx.remote, fx.user)
	fx.staleMain = revParse(t, fx.user, "HEAD")

	commitFile(t, fx.publisher, "shared.go", "package shared\n", "shared")
	gitIn(t, fx.publisher, "push", "origin", "main")
	fx.forkPoint = revParse(t, fx.publisher, "HEAD")

	gitIn(t, fx.user, "fetch", "origin")
	gitIn(t, fx.user, "switch", "-c", "feature", "origin/main")
	commitFile(t, fx.user, "feature.go", "package feature\n", "feature")

	commitFile(t, fx.publisher, "later.go", "package later\n", "later")
	gitIn(t, fx.publisher, "push", "origin", "main")
	fx.remoteTip = revParse(t, fx.publisher, "HEAD")
	return fx
}

func (fx fetchFixture) options(from string) reviewOptions {
	return reviewOptions{repoDir: fx.user, from: from, to: "HEAD", fetch: true, outputFormat: "text", audience: "agent"}
}

func (fx fetchFixture) commonContext(t *testing.T) *commonContext {
	t.Helper()
	cc, err := loadCommonContext(fx.user, "", "HEAD", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	return cc
}

// fetchBase runs both halves of --fetch the way executeReviewContext does:
// the local target resolution, then the fetch itself.
func fetchBase(ctx context.Context, cc *commonContext, opts *reviewOptions) (*diff.InputResolution, error) {
	target, err := resolveFetchTarget(ctx, cc, *opts)
	if err != nil {
		return nil, err
	}
	return fetchReviewBase(ctx, cc, opts, target)
}

func previewPaths(p model.Preview) []string {
	var paths []string
	for _, e := range p.Entries {
		paths = append(paths, e.Path)
	}
	return paths
}

// refSnapshot maps every ref in dir to what it points at; a symbolic ref maps to
// its target, so retargeting one shows up as a change too.
func refSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "for-each-ref", "--format=%(refname) %(objectname) %(symref)").Output()
	if err != nil {
		t.Fatalf("git for-each-ref: %v", err)
	}
	refs := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 {
			refs[f[0]] = "-> " + f[2]
		} else {
			refs[f[0]] = f[1]
		}
	}
	return refs
}

func fileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

func TestParseFetchTarget(t *testing.T) {
	remotes := []string{"origin", "release", "team", "team/up", "upstream"}
	tests := []struct {
		name    string
		from    string
		remote  string
		want    fetchTarget
		wantErr string
	}{
		{name: "bare branch defaults to origin", from: "main", want: fetchTarget{remote: "origin", branch: "main"}},
		{name: "remote-qualified branch", from: "origin/main", want: fetchTarget{remote: "origin", branch: "main"}},
		{name: "slash inside a branch name", from: "feature/login", want: fetchTarget{remote: "origin", branch: "feature/login"}},
		{name: "longest remote prefix wins", from: "team/up/main", want: fetchTarget{remote: "team/up", branch: "main"}},
		{name: "explicit remote", from: "main", remote: "upstream", want: fetchTarget{remote: "upstream", branch: "main"}},
		{name: "explicit remote agreeing with the prefix", from: "upstream/main", remote: "upstream", want: fetchTarget{remote: "upstream", branch: "main"}},
		{name: "explicit remote takes a remote-like prefix literally", from: "origin/main", remote: "upstream", want: fetchTarget{remote: "upstream", branch: "origin/main"}},
		{name: "branch namespace named like a remote, no --remote", from: "release/1.2", want: fetchTarget{remote: "release", branch: "1.2"}},
		{name: "branch namespace named like a remote, explicit --remote", from: "release/1.2", remote: "origin", want: fetchTarget{remote: "origin", branch: "release/1.2"}},
		{name: "unknown remote", from: "main", remote: "nosuch", wantErr: `"nosuch" is not a configured git remote`},
		{name: "option-like value", from: "-x", wantErr: "must not start with '-'"},
		{name: "HEAD", from: "HEAD", wantErr: "name a branch"},
		{name: "remote HEAD", from: "origin/HEAD", wantErr: "name a branch"},
		{name: "fully qualified ref", from: "refs/remotes/origin/main", wantErr: "short branch name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFetchTarget(tc.from, tc.remote, remotes)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseFetchTarget(%q, %q) error = %v, want it to contain %q", tc.from, tc.remote, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFetchTarget(%q, %q): %v", tc.from, tc.remote, err)
			}
			if got != tc.want {
				t.Errorf("parseFetchTarget(%q, %q) = %+v, want %+v", tc.from, tc.remote, got, tc.want)
			}
		})
	}
}

func TestReviewFlags_FetchValidation(t *testing.T) {
	tests := []struct {
		args    []string
		wantErr string
	}{
		{args: []string{"--fetch", "--from", "main", "--to", "HEAD"}},
		{args: []string{"--fetch", "--remote", "upstream", "--from", "main", "--to", "HEAD", "--preview"}},
		{args: []string{"--remote", "upstream", "--from", "main", "--to", "HEAD"}, wantErr: "--remote requires --fetch"},
		{args: []string{"--fetch"}, wantErr: "--fetch requires --from and --to"},
		{args: []string{"--fetch", "--commit", "HEAD"}, wantErr: "--fetch requires --from and --to"},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			opts, err := parseReviewFlags(tc.args)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !opts.fetch {
					t.Error("--fetch was not recorded")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// The stale-base incident, end to end through the command: the same range widens
// with the stale local base and narrows to the branch's own change with --fetch.
func TestReviewFetch_PreviewReviewsAgainstFetchedBase(t *testing.T) {
	fx := newFetchFixture(t)
	preview := func(fetch bool) ([]string, string) {
		opts := fx.options("main")
		opts.fetch = fetch
		opts.preview = true
		opts.outputFormat = "json"
		opts.audience = "human"
		var out string
		errOut := captureStderr(t, func() {
			out = captureStdout(t, func() {
				if err := executeReviewContext(context.Background(), opts); err != nil {
					t.Fatalf("preview (fetch=%v): %v", fetch, err)
				}
			})
		})
		return previewPaths(decodeSinglePreviewJSON(t, out)), errOut
	}

	if got, _ := preview(false); !slices.Equal(got, []string{"feature.go", "shared.go"}) {
		t.Fatalf("fixture must reproduce the widened range without --fetch, got %v", got)
	}
	got, errOut := preview(true)
	if !slices.Equal(got, []string{"feature.go"}) {
		t.Errorf("--fetch must review only the branch's own change, got %v", got)
	}
	if !strings.Contains(errOut, "origin/main") {
		t.Errorf("with --format json, fetch progress belongs on stderr, got:\n%s", errOut)
	}
}

func TestFetchReviewBase_FreezesFetchedEndpoints(t *testing.T) {
	fx := newFetchFixture(t)
	opts := fx.options("main")

	sealed, err := fetchBase(context.Background(), fx.commonContext(t), &opts)
	if err != nil {
		t.Fatalf("fetchReviewBase: %v", err)
	}
	if opts.from != "origin/main" {
		t.Errorf("opts.from = %q, want the fetched tracking branch origin/main", opts.from)
	}
	if sealed == nil || sealed.ResolvedBase != fx.forkPoint || sealed.ResolvedHead != revParse(t, fx.user, "HEAD") {
		t.Errorf("sealed = %+v, want base %s (fork point) and head %s", sealed, fx.forkPoint, revParse(t, fx.user, "HEAD"))
	}
	if got := revParse(t, fx.user, "refs/remotes/origin/main"); got != fx.remoteTip {
		t.Errorf("origin/main = %s, want the remote tip %s", got, fx.remoteTip)
	}
	if got := revParse(t, fx.user, "refs/heads/main"); got != fx.staleMain {
		t.Errorf("--fetch moved the local main branch to %s", got)
	}
}

// Config a plain `git fetch origin main` would honor must not widen what --fetch
// writes: a second mapping for every branch, tag following, and tag pruning.
func TestFetchReviewBase_WritesOnlyTheTrackingRef(t *testing.T) {
	fx := newFetchFixture(t)
	gitIn(t, fx.user, "config", "--add", "remote.origin.fetch", "+refs/heads/*:refs/remotes/mirror/*")
	gitIn(t, fx.user, "config", "fetch.prune", "true")
	gitIn(t, fx.user, "config", "fetch.pruneTags", "true")
	gitIn(t, fx.user, "tag", "local-only")
	gitIn(t, fx.publisher, "tag", "v1")
	gitIn(t, fx.publisher, "push", "origin", "v1")
	fetchHead := filepath.Join(fx.user, ".git", "FETCH_HEAD")
	if err := os.Remove(fetchHead); err != nil {
		t.Fatalf("remove FETCH_HEAD: %v", err)
	}
	before := refSnapshot(t, fx.user)

	opts := fx.options("main")
	if _, err := fetchBase(context.Background(), fx.commonContext(t), &opts); err != nil {
		t.Fatalf("fetchReviewBase: %v", err)
	}

	after := refSnapshot(t, fx.user)
	var changed []string
	for ref, target := range after {
		if before[ref] != target {
			changed = append(changed, ref)
		}
	}
	for ref := range before {
		if _, ok := after[ref]; !ok {
			changed = append(changed, ref+" (deleted)")
		}
	}
	slices.Sort(changed)
	if want := []string{"refs/remotes/origin/main"}; !slices.Equal(changed, want) {
		t.Errorf("refs changed by --fetch = %v, want only %v", changed, want)
	}
	if _, err := os.Stat(fetchHead); !os.IsNotExist(err) {
		t.Errorf("--fetch wrote FETCH_HEAD (stat err = %v)", err)
	}
}

func TestFetchReviewBase_FollowsForcePushedBase(t *testing.T) {
	fx := newFetchFixture(t)
	gitIn(t, fx.user, "fetch", "origin")
	gitIn(t, fx.publisher, "reset", "--hard", fx.forkPoint)
	commitFile(t, fx.publisher, "rewritten.go", "package rewritten\n", "rewritten")
	gitIn(t, fx.publisher, "push", "--force", "origin", "main")
	rewritten := revParse(t, fx.publisher, "HEAD")

	opts := fx.options("main")
	sealed, err := fetchBase(context.Background(), fx.commonContext(t), &opts)
	if err != nil || sealed == nil {
		t.Fatalf("a rewritten base must still be fetched and sealed: sealed = %v, err = %v", sealed, err)
	}
	if got := revParse(t, fx.user, "refs/remotes/origin/main"); got != rewritten {
		t.Errorf("origin/main = %s, want the rewritten tip %s", got, rewritten)
	}
	if sealed.ResolvedBase != fx.forkPoint {
		t.Errorf("merge base = %s, want %s", sealed.ResolvedBase, fx.forkPoint)
	}
}

func TestFetchReviewBase_RejectsNonBranchesBeforeFetching(t *testing.T) {
	fx := newFetchFixture(t)
	cc := fx.commonContext(t)
	for _, from := range []string{"HEAD~1", "a..b", "@{u}", "x:y", "origin/"} {
		t.Run(from, func(t *testing.T) {
			opts := fx.options(from)
			if _, err := fetchBase(context.Background(), cc, &opts); err == nil || !strings.Contains(err.Error(), "name a branch") {
				t.Fatalf("fetchReviewBase(--from %q) error = %v, want a branch-name rejection", from, err)
			}
			if got := revParse(t, fx.user, "refs/remotes/origin/main"); got != fx.forkPoint {
				t.Errorf("a rejected --from still fetched: origin/main moved to %s", got)
			}
		})
	}
}

// Fetching into a symbolic ref writes through to its target, which here is the
// local main branch; --fetch must never move a local branch.
func TestFetchReviewBase_RefusesSymbolicDestination(t *testing.T) {
	fx := newFetchFixture(t)
	gitIn(t, fx.publisher, "push", "origin", "main:trap")
	gitIn(t, fx.user, "symbolic-ref", "refs/remotes/origin/trap", "refs/heads/main")

	opts := fx.options("trap")
	if _, err := fetchBase(context.Background(), fx.commonContext(t), &opts); err == nil || !strings.Contains(err.Error(), "symbolic") {
		t.Fatalf("error = %v, want a refusal to fetch into a symbolic ref", err)
	}
	if got := revParse(t, fx.user, "refs/heads/main"); got != fx.staleMain {
		t.Errorf("local main moved to %s", got)
	}
}

// The branch is gone from the remote but a stale tracking ref for it is still
// around: the review must stop on the failed fetch, not carry on with that ref.
func TestReviewFetch_MissingBranchStopsBeforeTheReview(t *testing.T) {
	fx := newFetchFixture(t)
	llmServer := newFakeLLM()
	startFakeLLM(t, llmServer)
	gitIn(t, fx.user, "update-ref", "refs/remotes/origin/no-such-branch", fx.forkPoint)
	opts := fx.options("no-such-branch")

	err := executeReviewContext(context.Background(), opts)
	if err == nil || !strings.HasPrefix(err.Error(), "fetch no-such-branch from origin:") {
		t.Fatalf("error = %v, want the failed fetch of no-such-branch from origin", err)
	}
	if requests := llmServer.attemptCounts(); len(requests) != 0 {
		t.Errorf("a failed fetch still reached the LLM: %v", requests)
	}
	assertNoSessionStore(t, os.Getenv("HOME"))
}

// Git exits 0 when it refuses a ref update that would need new shallow roots,
// as when the remote itself is a shallow clone. The stale tracking ref it leaves
// behind must not be reviewed as if it were fresh.
func TestFetchReviewBase_RejectedUpdateIsAnError(t *testing.T) {
	fx := newFetchFixture(t)
	commitFile(t, fx.publisher, "more.go", "package more\n", "more")
	gitIn(t, fx.publisher, "push", "origin", "main")
	shallowRemote := filepath.Join(t.TempDir(), "shallow-remote")
	gitIn(t, fx.user, "clone", "--depth", "1", fileURL(fx.remote), shallowRemote)
	gitIn(t, fx.user, "remote", "add", "mirror", shallowRemote)
	gitIn(t, fx.user, "update-ref", "refs/remotes/mirror/main", fx.forkPoint)

	opts := fx.options("mirror/main")
	sealed, err := fetchBase(context.Background(), fx.commonContext(t), &opts)
	if err == nil || !strings.Contains(err.Error(), "did not update refs/remotes/mirror/main") {
		t.Fatalf("sealed = %+v, err = %v; want the rejected update reported as an error", sealed, err)
	}
}

func TestFetchReviewBase_ShallowCloneExplainsMissingHistory(t *testing.T) {
	fx := newFetchFixture(t)
	gitIn(t, fx.user, "push", "origin", "feature")
	shallow := filepath.Join(t.TempDir(), "shallow")
	gitIn(t, fx.user, "clone", "--depth", "1", "--branch", "feature", fileURL(fx.remote), shallow)
	cc, err := loadCommonContext(shallow, "", "HEAD", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}

	opts := reviewOptions{repoDir: shallow, from: "main", to: "HEAD", fetch: true, outputFormat: "text", audience: "agent"}
	if _, err := fetchBase(context.Background(), cc, &opts); err == nil || !strings.Contains(err.Error(), "shallow") {
		t.Fatalf("error = %v, want a hint that the clone is shallow", err)
	}
}

// --fetch is the only way a review refreshes remote refs; GIT_TRACE records every
// git command the run starts, including the positive control.
func TestReviewFetch_RunsGitFetchOnlyWhenAsked(t *testing.T) {
	fx := newFetchFixture(t)
	trace := filepath.Join(t.TempDir(), "git-trace.log")
	t.Setenv("GIT_TRACE", trace)
	gitCommands := func(fetch bool) string {
		t.Helper()
		_ = os.Remove(trace)
		opts := fx.options("main")
		opts.fetch = fetch
		opts.preview = true
		silenceStdout(t, func() {
			if err := executeReviewContext(context.Background(), opts); err != nil {
				t.Fatalf("preview (fetch=%v): %v", fetch, err)
			}
		})
		b, err := os.ReadFile(trace)
		if err != nil {
			t.Fatalf("read GIT_TRACE output: %v", err)
		}
		return string(b)
	}

	if !strings.Contains(gitCommands(true), "git fetch") {
		t.Fatal("control: GIT_TRACE did not record the --fetch run's git fetch")
	}
	if strings.Contains(gitCommands(false), "git fetch") {
		t.Error("a review without --fetch ran git fetch")
	}
}

func TestFetchReviewBase_ProgressFollowsAudience(t *testing.T) {
	fx := newFetchFixture(t)
	for _, tc := range []struct {
		audience string
		want     bool
	}{{"human", true}, {"agent", false}} {
		t.Run(tc.audience, func(t *testing.T) {
			var buf bytes.Buffer
			defer stdout.Swap(&buf)()
			opts := fx.options("main")
			opts.audience = tc.audience
			if _, err := fetchBase(context.Background(), fx.commonContext(t), &opts); err != nil {
				t.Fatalf("fetchReviewBase: %v", err)
			}
			out := buf.String()
			shown := strings.Contains(out, fx.remoteTip[:7]) && strings.Contains(out, fx.forkPoint[:7])
			if shown != tc.want {
				t.Errorf("audience %s: base and merge-base shown = %v, want %v; output:\n%s", tc.audience, shown, tc.want, out)
			}
		})
	}
}

func TestReviewE2E_FetchRecordsTheFetchedBase(t *testing.T) {
	fx := newFetchFixture(t)
	startFakeLLM(t, newFakeLLM())

	var runErr error
	out := captureStdout(t, func() {
		runErr = runReview([]string{"--repo", fx.user, "--fetch", "--from", "main", "--to", "HEAD", "--format", "json", "--audience", "agent"})
	})
	if runErr != nil {
		t.Fatalf("review: %v", runErr)
	}
	var doc struct {
		Manifest *session.RunManifest `json:"manifest"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Manifest == nil {
		t.Fatalf("decode review JSON (err = %v):\n%s", err, out)
	}
	in := doc.Manifest.Input
	if in.RequestedFrom != "origin/main" || in.ResolvedBase != fx.forkPoint || in.ResolvedHead != revParse(t, fx.user, "HEAD") {
		t.Errorf("manifest input = %+v, want requested origin/main, base %s, head %s", in, fx.forkPoint, revParse(t, fx.user, "HEAD"))
	}
	var selected []string
	for _, item := range doc.Manifest.Coverage.Selected {
		selected = append(selected, item.Path)
	}
	if !slices.Equal(selected, []string{"feature.go"}) {
		t.Errorf("selected = %v, want only feature.go", selected)
	}
}

// A branch namespace can share its name with another configured remote. An
// explicit --remote must reach that branch rather than be overridden by the
// remote-looking prefix, which would silently review against another base.
func TestFetchReviewBase_ExplicitRemoteReachesBranchNamedLikeARemote(t *testing.T) {
	fx := newFetchFixture(t)
	gitIn(t, fx.publisher, "push", "origin", "main:release/1.2")
	gitIn(t, fx.publisher, "push", "origin", fx.staleMain+":refs/heads/1.2")
	gitIn(t, fx.user, "remote", "add", "release", fx.remote)

	opts := fx.options("release/1.2")
	opts.remote = "origin"
	if _, err := fetchBase(context.Background(), fx.commonContext(t), &opts); err != nil {
		t.Fatalf("fetchReviewBase: %v", err)
	}
	if opts.from != "origin/release/1.2" {
		t.Errorf("opts.from = %q, want origin/release/1.2", opts.from)
	}
	if got := revParse(t, fx.user, "refs/remotes/origin/release/1.2"); got != fx.remoteTip {
		t.Errorf("origin/release/1.2 = %s, want %s", got, fx.remoteTip)
	}
}

// Once --fetch has frozen both endpoints, the short "origin/main" spelling is
// only a label. A tag of that name pointing at a tree must not fail the run by
// being re-resolved in place of the fetched commit.
func TestReviewFetch_ShadowingTagDoesNotFailTheRun(t *testing.T) {
	fx := newFetchFixture(t)
	gitIn(t, fx.user, "tag", "origin/main", "HEAD^{tree}")

	opts := fx.options("main")
	opts.preview = true
	opts.outputFormat = "json"
	out := captureStdout(t, func() {
		if err := executeReviewContext(context.Background(), opts); err != nil {
			t.Fatalf("preview: %v", err)
		}
	})
	if got := previewPaths(decodeSinglePreviewJSON(t, out)); !slices.Equal(got, []string{"feature.go"}) {
		t.Errorf("preview paths = %v, want only feature.go", got)
	}
}

// Checks that need no network run before the fetch, so a run that was going to
// fail on them never contacts the remote or moves the tracking ref.
func TestReviewFetch_LocalFailureStopsBeforeFetching(t *testing.T) {
	fx := newFetchFixture(t)
	opts := fx.options("main")
	opts.resume = "no-such-session"

	err := executeReviewContext(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "resume") {
		t.Fatalf("error = %v, want the unknown resume session reported", err)
	}
	if got := revParse(t, fx.user, "refs/remotes/origin/main"); got != fx.forkPoint {
		t.Errorf("origin/main moved to %s: the fetch ran before the local checks", got)
	}
}

// A preview in a format it can never produce fails before the fetch runs.
func TestReviewFetch_UnsupportedPreviewFormatStopsBeforeFetching(t *testing.T) {
	fx := newFetchFixture(t)
	opts := fx.options("main")
	opts.preview = true
	opts.outputFormat = "sarif"

	err := executeReviewContext(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "--format sarif is not supported with --preview") {
		t.Fatalf("error = %v, want the unsupported preview format reported", err)
	}
	if got := revParse(t, fx.user, "refs/remotes/origin/main"); got != fx.forkPoint {
		t.Errorf("origin/main moved to %s: the fetch ran before the format check", got)
	}
}

// The preview path applies the same rule: its own local checks, such as
// loading the config it reads max tokens from, run before the fetch.
func TestReviewFetch_PreviewLocalFailureStopsBeforeFetching(t *testing.T) {
	fx := newFetchFixture(t)
	configDir := filepath.Join(fx.home, ".opencodereview")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	opts := fx.options("main")
	opts.preview = true

	if err := executeReviewContext(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "load app config") {
		t.Fatalf("error = %v, want the broken config reported", err)
	}
	if got := revParse(t, fx.user, "refs/remotes/origin/main"); got != fx.forkPoint {
		t.Errorf("origin/main moved to %s: the fetch ran before the preview's local checks", got)
	}
}

// A bad --to is reported as such, before anything else is loaded or fetched,
// exactly as it is without --fetch.
func TestReviewFetch_BadToIsReportedFirst(t *testing.T) {
	fx := newFetchFixture(t)
	opts := fx.options("main")
	opts.to = "typo-branch"

	if err := executeReviewContext(context.Background(), opts); err == nil || !strings.Contains(err.Error(), `--to value "typo-branch"`) {
		t.Fatalf("error = %v, want the invalid --to reported", err)
	}
	if got := revParse(t, fx.user, "refs/remotes/origin/main"); got != fx.forkPoint {
		t.Errorf("origin/main moved to %s: the fetch ran despite the invalid --to", got)
	}
}

// A --from or --remote that --fetch can never use is reported before any
// config, resume or preview check runs, as a bad --from is without --fetch,
// and nothing is fetched.
func TestReviewFetch_BadFromIsReportedFirst(t *testing.T) {
	for _, tc := range []struct {
		name    string
		from    string
		remote  string
		preview bool
		wantErr string
	}{
		{name: "revision expression", from: "HEAD~1", wantErr: "name a branch"},
		{name: "unknown remote", from: "main", remote: "nosuch", wantErr: `"nosuch" is not a configured git remote`},
		{name: "revision expression in preview", from: "HEAD~1", preview: true, wantErr: "name a branch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFetchFixture(t)
			configDir := filepath.Join(fx.home, ".opencodereview")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{not json"), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			opts := fx.options(tc.from)
			opts.remote = tc.remote
			opts.preview = tc.preview

			if err := executeReviewContext(context.Background(), opts); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q reported before the broken config", err, tc.wantErr)
			}
			if got := revParse(t, fx.user, "refs/remotes/origin/main"); got != fx.forkPoint {
				t.Errorf("origin/main moved to %s", got)
			}
		})
	}
}

// Without --fetch a preview keeps its original order of checks: the config it
// reads max tokens from is loaded before the output format is rejected.
func TestReviewPreview_WithoutFetchKeepsItsErrorOrder(t *testing.T) {
	fx := newFetchFixture(t)
	configDir := filepath.Join(fx.home, ".opencodereview")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	opts := fx.options("main")
	opts.fetch = false
	opts.preview = true
	opts.outputFormat = "sarif"

	if err := executeReviewContext(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "load app config") {
		t.Fatalf("error = %v, want the config error first, as before --fetch existed", err)
	}
}

func TestFetchedTip(t *testing.T) {
	const dest = "refs/remotes/origin/main"
	const oldID = "1111111111111111111111111111111111111111"
	const newID = "2222222222222222222222222222222222222222"
	tests := []struct {
		name      string
		porcelain string
		want      string
		wantOK    bool
	}{
		{name: "fast-forward", porcelain: "  " + oldID + " " + newID + " " + dest + "\n", want: newID, wantOK: true},
		{name: "forced update", porcelain: "+ " + oldID + " " + newID + " " + dest + "\n", want: newID, wantOK: true},
		{name: "new ref", porcelain: "* 0000000000000000000000000000000000000000 " + newID + " " + dest + "\n", want: newID, wantOK: true},
		{name: "up to date", porcelain: "= " + newID + " " + newID + " " + dest + "\n", want: newID, wantOK: true},
		{name: "rejected", porcelain: "! " + oldID + " " + newID + " " + dest + "\n"},
		{name: "refused and left out", porcelain: ""},
		{name: "another ref only", porcelain: "  " + oldID + " " + newID + " refs/remotes/origin/other\n"},
		{name: "human-readable summary", porcelain: " = [up to date]      main       -> origin/main\n"},
		{name: "windows line ending", porcelain: "  " + oldID + " " + newID + " " + dest + "\r\n", want: newID, wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := fetchedTip(tc.porcelain, dest)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("fetchedTip() = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestFetchFailure(t *testing.T) {
	target := fetchTarget{remote: "origin", branch: "gone"}
	exitErr := errors.New("exit status 128")

	missing := fetchFailure(target, "fatal: couldn't find remote ref refs/heads/gone\n", exitErr)
	if !strings.HasPrefix(missing.Error(), "fetch gone from origin:") || !strings.Contains(missing.Error(), "drop --fetch") {
		t.Errorf("missing branch: %v, want the fetch named and the tag/commit hint", missing)
	}
	if !errors.Is(missing, exitErr) {
		t.Error("the git error must stay in the chain")
	}
	if escaped := fetchFailure(target, "fatal: bad\x1b[31m remote\n", exitErr); strings.ContainsRune(escaped.Error(), '\x1b') {
		t.Errorf("terminal control characters from the remote reached the error: %q", escaped)
	}
	if bare := fetchFailure(target, "", exitErr); !errors.Is(bare, exitErr) || strings.Contains(bare.Error(), "drop --fetch") {
		t.Errorf("empty stderr: %v, want only the wrapped git error", bare)
	}
}

// CI checkouts are often single-branch: the configured refspec covers only the
// checked-out branch, so origin/main does not exist until --fetch creates it.
func TestFetchReviewBase_SingleBranchClone(t *testing.T) {
	fx := newFetchFixture(t)
	gitIn(t, fx.user, "push", "origin", "feature")
	clone := filepath.Join(t.TempDir(), "single")
	gitIn(t, fx.user, "clone", "--single-branch", "--branch", "feature", fx.remote, clone)
	cc, err := loadCommonContext(clone, "", "HEAD", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}

	opts := reviewOptions{repoDir: clone, from: "main", to: "HEAD", fetch: true, outputFormat: "text", audience: "agent"}
	sealed, err := fetchBase(context.Background(), cc, &opts)
	if err != nil || sealed == nil {
		t.Fatalf("fetchBase: sealed = %v, err = %v", sealed, err)
	}
	if got := revParse(t, clone, "refs/remotes/origin/main"); got != fx.remoteTip {
		t.Errorf("origin/main = %s, want the remote tip %s", got, fx.remoteTip)
	}
	if sealed.ResolvedBase != fx.forkPoint {
		t.Errorf("merge base = %s, want %s", sealed.ResolvedBase, fx.forkPoint)
	}
}

// --fetch depends on `git fetch --porcelain` (Git 2.41). On older Git the run
// stops with a clear message before contacting the remote, instead of git's
// usage text.
func TestReviewFetch_OldGitIsReportedBeforeFetching(t *testing.T) {
	fx := newFetchFixture(t)
	old := checkFetchGitVersion
	checkFetchGitVersion = func() error {
		return &gitcmd.VersionTooOldError{Current: gitcmd.GitVersion{Major: 2, Minor: 34}, Minimum: gitcmd.GitVersion{Major: 2, Minor: 41}}
	}
	t.Cleanup(func() { checkFetchGitVersion = old })

	opts := fx.options("main")
	opts.preview = true
	err := executeReviewContext(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "--fetch") || !strings.Contains(err.Error(), "older than the minimum") {
		t.Fatalf("error = %v, want --fetch rejected for the old Git", err)
	}
	if got := revParse(t, fx.user, "refs/remotes/origin/main"); got != fx.forkPoint {
		t.Errorf("origin/main moved to %s", got)
	}
}
