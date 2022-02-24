// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"io"

	"gate.computer/gate/server/api/pb"
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
	DebugInstance(context.Context, string, *DebugRequest) (*DebugResponse, error)
	DeleteInstance(context.Context, string) error
	Features() *Features
	InstanceConnection(context.Context, string) (Instance, func(context.Context, io.Reader, io.WriteCloser) error, error)
	InstanceInfo(context.Context, string) (*InstanceInfo, error)
	Instances(context.Context) (*Instances, error)
	KillInstance(context.Context, string) (Instance, error)
	ModuleContent(context.Context, string) (io.ReadCloser, int64, error)
	ModuleInfo(context.Context, string) (*ModuleInfo, error)
	Modules(context.Context) (*Modules, error)
	NewInstance(context.Context, string, *LaunchOptions) (Instance, error)
	PinModule(context.Context, string, *ModuleOptions) error
	ResumeInstance(context.Context, string, *ResumeOptions) (Instance, error)
	Snapshot(context.Context, string, *ModuleOptions) (string, error)
	SourceModule(context.Context, string, *ModuleOptions) (string, error)
	SourceModuleInstance(context.Context, string, *ModuleOptions, *LaunchOptions) (string, Instance, error)
	SuspendInstance(context.Context, string) (Instance, error)
	UnpinModule(context.Context, string) error
	UpdateInstance(context.Context, string, *InstanceUpdate) (*InstanceInfo, error)
	UploadModule(context.Context, *ModuleUpload, *ModuleOptions) (string, error)
	UploadModuleInstance(context.Context, *ModuleUpload, *ModuleOptions, *LaunchOptions) (string, Instance, error)
	WaitInstance(context.Context, string) (*Status, error)
}

type Instance interface {
	Connect(context.Context, io.Reader, io.WriteCloser) error
	ID() string
	Kill(context.Context) error
	Status(context.Context) *Status
	Suspend(context.Context) error
	Wait(context.Context) *Status
}
