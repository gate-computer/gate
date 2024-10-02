// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"io"

	"gate.computer/gate/server/api/pb"

	. "import.name/type/context"
)

type (
	DebugConfig    = pb.DebugConfig
	DebugRequest   = pb.DebugRequest
	DebugResponse  = pb.DebugResponse
	Features       = pb.Features
	InstanceUpdate = pb.InstanceUpdate
	InvokeOptions  = pb.InvokeOptions
	LaunchOptions  = pb.LaunchOptions
	ModuleOptions  = pb.ModuleOptions
	ResumeOptions  = pb.ResumeOptions
)

type Server interface {
	DebugInstance(Context, string, *DebugRequest) (*DebugResponse, error)
	DeleteInstance(Context, string) error
	Features() *Features
	InstanceConnection(Context, string) (Instance, func(Context, io.Reader, io.WriteCloser) error, error)
	InstanceInfo(Context, string) (*InstanceInfo, error)
	Instances(Context) (*Instances, error)
	KillInstance(Context, string) (Instance, error)
	ModuleContent(Context, string) (io.ReadCloser, int64, error)
	ModuleInfo(Context, string) (*ModuleInfo, error)
	Modules(Context) (*Modules, error)
	NewInstance(Context, string, *LaunchOptions) (Instance, error)
	PinModule(Context, string, *ModuleOptions) error
	ResumeInstance(Context, string, *ResumeOptions) (Instance, error)
	Snapshot(Context, string, *ModuleOptions) (string, error)
	SourceModule(Context, string, *ModuleOptions) (string, error)
	SourceModuleInstance(Context, string, *ModuleOptions, *LaunchOptions) (string, Instance, error)
	SuspendInstance(Context, string) (Instance, error)
	UnpinModule(Context, string) error
	UpdateInstance(Context, string, *InstanceUpdate) (*InstanceInfo, error)
	UploadModule(Context, *ModuleUpload, *ModuleOptions) (string, error)
	UploadModuleInstance(Context, *ModuleUpload, *ModuleOptions, *LaunchOptions) (string, Instance, error)
	WaitInstance(Context, string) (*Status, error)
}

type Instance interface {
	Connect(Context, io.Reader, io.WriteCloser) error
	ID() string
	Kill(Context) error
	Status(Context) *Status
	Suspend(Context) error
	Wait(Context) *Status
}
