// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build aix || freebsd || solaris

package tool

import "golang.org/x/sys/unix"

const directoryAccess = unix.O_SEARCH
