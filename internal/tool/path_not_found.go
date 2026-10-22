// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"errors"
	"fmt"
)

// ErrPathNotFound reports that a read path is not present at the read target:
// the reviewed ref in range and commit mode, the working tree in workspace
// mode.
//
// It is a structural rejection, not a flaky failure: the path will not appear
// by reading it again, so the retry ladder must not spend its retry budget on
// it or reshape it into the wording it uses for failures a model can fix.
var ErrPathNotFound = errors.New("path not found at the read target")

// PathNotFoundError describes a read path that was rejected because it is not
// present at the read target.
//
// It owns its message so that the text recorded in the run report identifies
// the rejection on its own, without the reader having to be consulted: the
// message names the path, states where the path was looked for, and says
// that reading it again will not help.
type PathNotFoundError struct {
	// Path is the path the caller asked for.
	Path string
	// Target names where the path was looked for, as a prepositional phrase:
	// `at the reviewed ref "abc1234"` or `in the working tree`.
	Target string
	// Err is the underlying cause, when there is one. A workspace read
	// carries os.ErrNotExist so that callers matching on it keep working.
	Err error
}

// Error returns the rejection message.
func (e *PathNotFoundError) Error() string {
	return fmt.Sprintf("read file %q: the path does not exist %s; reading it again will not help", e.Path, e.Target)
}

// Unwrap returns the underlying cause, so that a workspace rejection still
// satisfies errors.Is(err, os.ErrNotExist).
func (e *PathNotFoundError) Unwrap() error { return e.Err }

// Is reports whether the error matches ErrPathNotFound, so that a caller can
// recognise the rejection class without knowing the concrete type.
func (e *PathNotFoundError) Is(target error) bool { return target == ErrPathNotFound }

// pathNotFoundAtRef builds the rejection for a path that is absent from the
// tree at ref.
func pathNotFoundAtRef(path, ref string) *PathNotFoundError {
	return &PathNotFoundError{
		Path:   path,
		Target: fmt.Sprintf("at the reviewed ref %q", ref),
	}
}

// pathNotFoundInWorkspace builds the rejection for a path that is absent from
// the working tree. The os.ErrNotExist cause is preserved so that the
// existing workspace read contract still holds.
func pathNotFoundInWorkspace(path string, cause error) *PathNotFoundError {
	return &PathNotFoundError{
		Path:   path,
		Target: "in the working tree",
		Err:    cause,
	}
}
