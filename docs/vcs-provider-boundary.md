# Repository provider boundary

`internal/vcs` defines the repository operations used by review, preview, scan,
rule sniffing, and code tools. `internal/vcs/git` implements them with Git and
the existing diff parser. The CLI selects this Git adapter by default.

`ReviewSpec` distinguishes workspace changes, one commit, a merge-base
comparison, and a direct comparison. `ResolveReview` freezes historical
endpoints and the content revision once. Admission, preview, review, and context
reads reuse that `ResolvedReview`, including when a branch name moves. Root
commits use an empty comparison tree; workspace content stays mutable.

`ReviewSource.Changes` returns normalized `model.Diff` entries. Provider directory
exclusions and their changeset positions remain available to preview. Selection,
rule filters, token limits, and session reporting stay with their consumers.
Git repository identities and range strings retain their existing manifest
format; identities from other providers include the provider name.

`FileSource` reads and lists repository-relative paths at a `ContentRef`.
Providers enforce path containment and own stream cleanup, including when a
consumer stops after reading a prefix. A historical read failure is returned to
the caller. Scan reads workspace content through this interface and uses local
metadata for its existing regular-file, size, and path-selector checks.

Text search and branch labels are optional capabilities. `SearchSource` returns
structured matches, truncation information, and the total matching-file count;
Git pathspec handling and NUL-delimited output parsing stay inside the adapter.
`BranchSource` supplies session display metadata. Consumers report
`ErrUnsupported` when a required capability or comparison is unavailable.

An additional provider implements `ReviewSource` and `FileSource` and can add
these optional interfaces. The in-memory test providers exercise the boundary
without Git, including immutable input reuse and persisted failure reporting.
