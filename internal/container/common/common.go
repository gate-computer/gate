// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

// Internal container API/ABI compatibility version.
const (
	CompatMajor   = "0"
	CompatVersion = CompatMajor + ".0"
)

const (
	ContainerName = "gate-runtime-container"
	ExecutorName  = "gate-runtime-executor"
	LoaderName    = "gate-runtime-loader"
)

var (
	ExecutorFilename = ExecutorName + "." + CompatMajor
	LoaderFilename   = LoaderName + "." + CompatVersion
)

// File descriptors passed from the parent to the child process.
const (
	LoaderFD   = 4
	ExecutorFD = 5
	CgroupFD   = 6
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
