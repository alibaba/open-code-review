// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build darwin || linux

package tool

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openRegularComponents(repo string, components []string) (*os.File, error) {
	parent, err := unix.Open(repo, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for index, component := range components {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if index+1 < len(components) {
			flags |= unix.O_DIRECTORY
		}
		child, err := unix.Openat(parent, component, flags, 0)
		unix.Close(parent)
		if err != nil {
			return nil, err
		}
		parent = child
	}
	file := os.NewFile(uintptr(parent), components[len(components)-1])
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("workspace path is not a regular file")
	}
	return file, nil
}
