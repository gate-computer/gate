// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"gate.computer/gate/server/detail"
)

// Server operation types.  Returned by ContextOp(context.Context) function.
const (
	OpModuleList       = detail.Op_MODULE_LIST
	OpModuleInfo       = detail.Op_MODULE_INFO
	OpModuleDownload   = detail.Op_MODULE_DOWNLOAD
	OpModuleUpload     = detail.Op_MODULE_UPLOAD
	OpModuleSource     = detail.Op_MODULE_SOURCE
	OpModulePin        = detail.Op_MODULE_PIN
	OpModuleUnpin      = detail.Op_MODULE_UNPIN
	OpCallExtant       = detail.Op_CALL_EXTANT
	OpCallUpload       = detail.Op_CALL_UPLOAD
	OpCallSource       = detail.Op_CALL_SOURCE
	OpLaunchExtant     = detail.Op_LAUNCH_EXTANT
	OpLaunchUpload     = detail.Op_LAUNCH_UPLOAD
	OpLaunchSource     = detail.Op_LAUNCH_SOURCE
	OpInstanceList     = detail.Op_INSTANCE_LIST
	OpInstanceInfo     = detail.Op_INSTANCE_INFO
	OpInstanceConnect  = detail.Op_INSTANCE_CONNECT
	OpInstanceWait     = detail.Op_INSTANCE_WAIT
	OpInstanceKill     = detail.Op_INSTANCE_KILL
	OpInstanceSuspend  = detail.Op_INSTANCE_SUSPEND
	OpInstanceResume   = detail.Op_INSTANCE_RESUME
	OpInstanceSnapshot = detail.Op_INSTANCE_SNAPSHOT
	OpInstanceDelete   = detail.Op_INSTANCE_DELETE
	OpInstanceUpdate   = detail.Op_INSTANCE_UPDATE
	OpInstanceDebug    = detail.Op_INSTANCE_DEBUG
)
