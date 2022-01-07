// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

// Server operation types.  Returned by ContextOp(context.Context) function.
const (
	OpModuleList       = Op_MODULE_LIST
	OpModuleInfo       = Op_MODULE_INFO
	OpModuleDownload   = Op_MODULE_DOWNLOAD
	OpModuleUpload     = Op_MODULE_UPLOAD
	OpModuleSource     = Op_MODULE_SOURCE
	OpModulePin        = Op_MODULE_PIN
	OpModuleUnpin      = Op_MODULE_UNPIN
	OpCallExtant       = Op_CALL_EXTANT
	OpCallUpload       = Op_CALL_UPLOAD
	OpCallSource       = Op_CALL_SOURCE
	OpLaunchExtant     = Op_LAUNCH_EXTANT
	OpLaunchUpload     = Op_LAUNCH_UPLOAD
	OpLaunchSource     = Op_LAUNCH_SOURCE
	OpInstanceList     = Op_INSTANCE_LIST
	OpInstanceInfo     = Op_INSTANCE_INFO
	OpInstanceConnect  = Op_INSTANCE_CONNECT
	OpInstanceWait     = Op_INSTANCE_WAIT
	OpInstanceKill     = Op_INSTANCE_KILL
	OpInstanceSuspend  = Op_INSTANCE_SUSPEND
	OpInstanceResume   = Op_INSTANCE_RESUME
	OpInstanceSnapshot = Op_INSTANCE_SNAPSHOT
	OpInstanceDelete   = Op_INSTANCE_DELETE
	OpInstanceUpdate   = Op_INSTANCE_UPDATE
	OpInstanceDebug    = Op_INSTANCE_DEBUG
)
