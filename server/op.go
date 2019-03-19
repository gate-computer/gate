// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"github.com/tsavola/gate/server/detail"
)

// Server operation types.  Returned by Op(context.Context) function.
const (
	OpModuleList       = detail.Op_ModuleList
	OpModuleDownload   = detail.Op_ModuleDownload
	OpModuleUpload     = detail.Op_ModuleUpload
	OpModuleSource     = detail.Op_ModuleSource
	OpModuleUnref      = detail.Op_ModuleUnref
	OpCallExtant       = detail.Op_CallExtant
	OpCallUpload       = detail.Op_CallUpload
	OpCallSource       = detail.Op_CallSource
	OpLaunchExtant     = detail.Op_LaunchExtant
	OpLaunchUpload     = detail.Op_LaunchUpload
	OpLaunchSource     = detail.Op_LaunchSource
	OpInstanceList     = detail.Op_InstanceList
	OpInstanceConnect  = detail.Op_InstanceConnect
	OpInstanceStatus   = detail.Op_InstanceStatus
	OpInstanceWait     = detail.Op_InstanceWait
	OpInstanceSuspend  = detail.Op_InstanceSuspend
	OpInstanceResume   = detail.Op_InstanceResume
	OpInstanceSnapshot = detail.Op_InstanceSnapshot
)
