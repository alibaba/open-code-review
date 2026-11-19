// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build !darwin && !linux

package tool

import (
	"fmt"
	"os"
)

func openRegularComponents(repo string, components []string) (*os.File, error) {
	root, err := os.OpenRoot(repo)
	if err != nil {
		return nil, err
	}
	defer func() { root.Close() }()
	for _, component := range components[:len(components)-1] {
		before, err := root.Lstat(component)
		if err != nil {
			return nil, err
		}
		if !before.IsDir() {
			return nil, fmt.Errorf("workspace path component is not a directory")
		}
		child, err := root.OpenRoot(component)
		if err != nil {
			return nil, err
		}
		after, err := child.Stat(".")
		if err != nil || !os.SameFile(before, after) {
			child.Close()
			return nil, fmt.Errorf("workspace directory changed while opening")
		}
		root.Close()
		root = child
	}
	name := components[len(components)-1]
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("workspace path is not a regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		file.Close()
		return nil, fmt.Errorf("workspace file changed while opening")
	}
	return file, nil
}
