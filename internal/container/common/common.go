// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

var (
	ContainerName = "gate-runtime-container-0"
	ExecutorName  = "gate-runtime-executor-0"
	LoaderName    = "gate-runtime-loader-0"
)

// File descriptors passed from the parent to the child process.
const (
	LoaderFD   = 4
	ExecutorFD = 5
)

// User/group ids inside the container's user namespace.
const (
	ContainerCred = 1
	ExecutorCred  = 2
)

// Command-line flags passed between binaries.
const (
	ArgNamespaceDisabled = "-n"
	ArgSingleUID         = "-u"
)
