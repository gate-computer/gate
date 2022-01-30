// Code generated by gen.go, DO NOT EDIT!

package event

import "gate.computer/gate/server/event/pb"

const (
	TypeFailInternal         = pb.Type_FAIL_INTERNAL
	TypeFailNetwork          = pb.Type_FAIL_NETWORK
	TypeFailProtocol         = pb.Type_FAIL_PROTOCOL
	TypeFailRequest          = pb.Type_FAIL_REQUEST
	TypeIfaceAccess          = pb.Type_IFACE_ACCESS
	TypeInstanceConnect      = pb.Type_INSTANCE_CONNECT
	TypeInstanceCreateKnown  = pb.Type_INSTANCE_CREATE_KNOWN
	TypeInstanceCreateStream = pb.Type_INSTANCE_CREATE_STREAM
	TypeInstanceDebug        = pb.Type_INSTANCE_DEBUG
	TypeInstanceDelete       = pb.Type_INSTANCE_DELETE
	TypeInstanceDisconnect   = pb.Type_INSTANCE_DISCONNECT
	TypeInstanceInfo         = pb.Type_INSTANCE_INFO
	TypeInstanceKill         = pb.Type_INSTANCE_KILL
	TypeInstanceList         = pb.Type_INSTANCE_LIST
	TypeInstanceResume       = pb.Type_INSTANCE_RESUME
	TypeInstanceSnapshot     = pb.Type_INSTANCE_SNAPSHOT
	TypeInstanceStop         = pb.Type_INSTANCE_STOP
	TypeInstanceSuspend      = pb.Type_INSTANCE_SUSPEND
	TypeInstanceUpdate       = pb.Type_INSTANCE_UPDATE
	TypeInstanceWait         = pb.Type_INSTANCE_WAIT
	TypeModuleDownload       = pb.Type_MODULE_DOWNLOAD
	TypeModuleInfo           = pb.Type_MODULE_INFO
	TypeModuleList           = pb.Type_MODULE_LIST
	TypeModulePin            = pb.Type_MODULE_PIN
	TypeModuleSourceExist    = pb.Type_MODULE_SOURCE_EXIST
	TypeModuleSourceNew      = pb.Type_MODULE_SOURCE_NEW
	TypeModuleUnpin          = pb.Type_MODULE_UNPIN
	TypeModuleUploadExist    = pb.Type_MODULE_UPLOAD_EXIST
	TypeModuleUploadNew      = pb.Type_MODULE_UPLOAD_NEW
)

const (
	FailAuthDenied         = pb.Fail_AUTH_DENIED
	FailAuthExpired        = pb.Fail_AUTH_EXPIRED
	FailAuthInvalid        = pb.Fail_AUTH_INVALID
	FailAuthMissing        = pb.Fail_AUTH_MISSING
	FailAuthReused         = pb.Fail_AUTH_REUSED
	FailClientDenied       = pb.Fail_CLIENT_DENIED
	FailFunctionNotFound   = pb.Fail_FUNCTION_NOT_FOUND
	FailInstanceDebugState = pb.Fail_INSTANCE_DEBUG_STATE
	FailInstanceIDExists   = pb.Fail_INSTANCE_ID_EXISTS
	FailInstanceIDInvalid  = pb.Fail_INSTANCE_ID_INVALID
	FailInstanceNotFound   = pb.Fail_INSTANCE_NOT_FOUND
	FailInstanceNoConnect  = pb.Fail_INSTANCE_NO_CONNECT
	FailInstanceStatus     = pb.Fail_INSTANCE_STATUS
	FailInternal           = pb.Fail_INTERNAL
	FailModuleError        = pb.Fail_MODULE_ERROR
	FailModuleHashMismatch = pb.Fail_MODULE_HASH_MISMATCH
	FailModuleNotFound     = pb.Fail_MODULE_NOT_FOUND
	FailPayloadError       = pb.Fail_PAYLOAD_ERROR
	FailPrincipalKeyError  = pb.Fail_PRINCIPAL_KEY_ERROR
	FailProgramError       = pb.Fail_PROGRAM_ERROR
	FailRateLimit          = pb.Fail_RATE_LIMIT
	FailResourceDenied     = pb.Fail_RESOURCE_DENIED
	FailResourceLimit      = pb.Fail_RESOURCE_LIMIT
	FailScopeTooLarge      = pb.Fail_SCOPE_TOO_LARGE
	FailUnsupported        = pb.Fail_UNSUPPORTED
)
