// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	pb "gate.computer/gate/pb/server"
	"gate.computer/internal/serverapi"

	. "import.name/type/context"
)

type Op = pb.Op

// Op types.
const (
	OpModuleList       = pb.Op_MODULE_LIST
	OpModuleInfo       = pb.Op_MODULE_INFO
	OpModuleDownload   = pb.Op_MODULE_DOWNLOAD
	OpModuleUpload     = pb.Op_MODULE_UPLOAD
	OpModuleSource     = pb.Op_MODULE_SOURCE
	OpModulePin        = pb.Op_MODULE_PIN
	OpModuleUnpin      = pb.Op_MODULE_UNPIN
	OpCallExtant       = pb.Op_CALL_EXTANT
	OpCallUpload       = pb.Op_CALL_UPLOAD
	OpCallSource       = pb.Op_CALL_SOURCE
	OpLaunchExtant     = pb.Op_LAUNCH_EXTANT
	OpLaunchUpload     = pb.Op_LAUNCH_UPLOAD
	OpLaunchSource     = pb.Op_LAUNCH_SOURCE
	OpInstanceList     = pb.Op_INSTANCE_LIST
	OpInstanceInfo     = pb.Op_INSTANCE_INFO
	OpInstanceConnect  = pb.Op_INSTANCE_CONNECT
	OpInstanceWait     = pb.Op_INSTANCE_WAIT
	OpInstanceKill     = pb.Op_INSTANCE_KILL
	OpInstanceSuspend  = pb.Op_INSTANCE_SUSPEND
	OpInstanceResume   = pb.Op_INSTANCE_RESUME
	OpInstanceSnapshot = pb.Op_INSTANCE_SNAPSHOT
	OpInstanceDelete   = pb.Op_INSTANCE_DELETE
	OpInstanceUpdate   = pb.Op_INSTANCE_UPDATE
	OpInstanceDebug    = pb.Op_INSTANCE_DEBUG
)

func ContextOp(ctx Context) Op {
	return serverapi.ContextOp(ctx)
}
