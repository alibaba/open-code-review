package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// CanonicalPath returns an absolute path with symlinks resolved.
func CanonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// WithinBase reports whether target is base itself or contained under base.
func WithinBase(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
