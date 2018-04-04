// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webapi

const (
	HeaderProgramSHA384 = "X-Gate-Program-Sha384" // unpadded base64url-encoded digest
	HeaderProgramId     = "X-Gate-Program-Id"     // unpadded base64url-encoded token
	HeaderInstanceArg   = "X-Gate-Instance-Arg"   // signed 32-bit integer
	HeaderInstanceId    = "X-Gate-Instance-Id"    // unpadded base64url-encoded token
	HeaderExitStatus    = "X-Gate-Exit-Status"    // non-negative integer
	HeaderTrapId        = "X-Gate-Trap-Id"        // positive integer
	HeaderTrap          = "X-Gate-Trap"           // human-readable string
	HeaderError         = "X-Gate-Error"          // human-readable string
)

type Run struct {
	ProgramSHA384 string `json:"program_sha384,omitempty"`
	ProgramId     string `json:"program_id,omitempty"`
	InstanceArg   int32  `json:"instance_arg"`
}

type Running struct {
	InstanceId string `json:"instance_id"`
	ProgramId  string `json:"program_id"`
}

type IO struct {
	InstanceId string `json:"instance_id"`
}

type IOState struct {
}

type Result struct {
	ExitStatus *int   `json:"exit_status,omitempty"`
	TrapId     int    `json:"trap_id,omitempty"`
	Trap       string `json:"trap,omitempty"`
}
