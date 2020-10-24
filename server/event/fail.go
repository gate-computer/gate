// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

const (
	FailUnspecified        = FailRequest_UNSPECIFIED
	FailClientDenied       = FailRequest_CLIENT_DENIED
	FailPayloadError       = FailRequest_PAYLOAD_ERROR
	FailPrincipalKeyError  = FailRequest_PRINCIPAL_KEY_ERROR
	FailAuthMissing        = FailRequest_AUTH_MISSING
	FailAuthInvalid        = FailRequest_AUTH_INVALID
	FailAuthExpired        = FailRequest_AUTH_EXPIRED
	FailAuthReused         = FailRequest_AUTH_REUSED
	FailAuthDenied         = FailRequest_AUTH_DENIED
	FailScopeTooLarge      = FailRequest_SCOPE_TOO_LARGE
	FailResourceDenied     = FailRequest_RESOURCE_DENIED
	FailResourceLimit      = FailRequest_RESOURCE_LIMIT
	FailRateLimit          = FailRequest_RATE_LIMIT
	FailModuleNotFound     = FailRequest_MODULE_NOT_FOUND
	FailModuleHashMismatch = FailRequest_MODULE_HASH_MISMATCH
	FailModuleError        = FailRequest_MODULE_ERROR
	FailFunctionNotFound   = FailRequest_FUNCTION_NOT_FOUND
	FailProgramError       = FailRequest_PROGRAM_ERROR
	FailInstanceNotFound   = FailRequest_INSTANCE_NOT_FOUND
	FailInstanceIDInvalid  = FailRequest_INSTANCE_ID_INVALID
	FailInstanceIDExists   = FailRequest_INSTANCE_ID_EXISTS
	FailInstanceStatus     = FailRequest_INSTANCE_STATUS
	FailInstanceNoConnect  = FailRequest_INSTANCE_NO_CONNECT
	FailInstanceTransient  = FailRequest_INSTANCE_TRANSIENT
	FailInstanceDebugger   = FailRequest_INSTANCE_DEBUGGER
)
