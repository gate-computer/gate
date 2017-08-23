// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webapi

const (
	HeaderProgramId     = "X-Gate-Program-Id"     // opaque
	HeaderProgramSHA512 = "X-Gate-Program-Sha512" // hexadecimal
	HeaderInstanceArg   = "X-Gate-Instance-Arg"   // 32-bit signed integer
	HeaderInstanceId    = "X-Gate-Instance-Id"    // opaque
	HeaderExitStatus    = "X-Gate-Exit-Status"    // non-negative integer
	HeaderTrapId        = "X-Gate-Trap-Id"        // positive integer
	HeaderTrap          = "X-Gate-Trap"           // human-readable string
	HeaderError         = "X-Gate-Error"          // human-readable string
)

type Run struct {
	ProgramId     string `json:"program_id,omitempty"`
	ProgramSHA512 string `json:"program_sha512,omitempty"`
	InstanceArg   int32  `json:"instance_arg"`
}

type Running struct {
	InstanceId string `json:"instance_id"`
	ProgramId  string `json:"program_id,omitempty"`
}

type Communicate struct {
	InstanceId string `json:"instance_id"`
}

type Communicating struct {
}

type Result struct {
	ExitStatus *int   `json:"exit_status,omitempty"`
	TrapId     int    `json:"trap_id,omitempty"`
	Trap       string `json:"trap,omitempty"`
}
