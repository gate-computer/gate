// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"time"

	"gate.computer/wag/wa"

	. "import.name/type/context"
)

const (
	DefaultMaxModules        = 64
	DefaultMaxProcs          = 4
	DefaultTotalStorageSize  = 256 * 1024 * 1024
	DefaultTotalResidentSize = 64 * 1024 * 1024
	DefaultMaxModuleSize     = 32 * 1024 * 1024
	DefaultMaxTextSize       = 16 * 1024 * 1024
	DefaultMaxMemorySize     = 32 * 1024 * 1024
	DefaultStackSize         = wa.PageSize
	DefaultTimeResolution    = time.Second / 100
)

// TODO: ResourcePolicy is not yet enforced by server
type ResourcePolicy struct {
	MaxModules        int // Pinned module limit.
	MaxProcs          int // Active instance limit.
	TotalStorageSize  int // Sum of pinned module and metadata sizes.
	TotalResidentSize int // Sum of all memory mapping and buffer sizes.
}

type ProgramPolicy struct {
	MaxModuleSize int // WebAssembly module size.
	MaxTextSize   int // Native program code size.
	MaxStackSize  int // Suspended stack size.
}

type InstancePolicy struct {
	MaxMemorySize  int           // Linear memory growth limit.
	StackSize      int           // Including system/runtime overhead.
	TimeResolution time.Duration // Granularity of time functions.

	// Services function defines which services are discoverable by the
	// instance.
	Services func(Context) InstanceServices
}

// Authorizer and moderator of server access.
//
// The methods should return Unauthenticated, PermissionDenied or Unavailable
// errors to signal successful prevention of access.  Other types of errors are
// interpreted as failures of the authorization mechanism.  Returning a nil
// error grants access.
//
// An implementation should adjust the ResourcePolicy, ProgramPolicy and
// InstancePolicy objects' fields.  The limits are enforced automatically by
// the server, which may also lead to denial of access.
//
// Principal id can be obtained using the principal.ContextID(Context)
// function.  If it is nil, the request didn't contain credentials, and the
// access should be denied unless the policy allows anonymous access.  If the
// principal id is non-nil, it should be checked unless the policy allows
// access to everyone.
//
// An implementation may choose to discriminate based on server operation type.
// It can be obtained using the ContextOp(Context) function.
//
// Authorizer may be expanded with new methods (prefixed with the Authorize
// namespace) also between major releases.  Implementations must inherit
// methods from a concrete access authorization type, and must not add
// unrelated methods with the Authorize prefix to avoid breakage.  The
// conservative choice is to inherit from NoAccess.  That way, new
// functionality will be effectively disabled.
type Authorizer interface {
	Authorize(Context) (Context, error)
	AuthorizeProgram(Context, *ResourcePolicy, *ProgramPolicy) (Context, error)
	AuthorizeProgramSource(Context, *ResourcePolicy, *ProgramPolicy, string) (Context, error)
	AuthorizeInstance(Context, *ResourcePolicy, *InstancePolicy) (Context, error)
	AuthorizeProgramInstance(Context, *ResourcePolicy, *ProgramPolicy, *InstancePolicy) (Context, error)
	AuthorizeProgramInstanceSource(Context, *ResourcePolicy, *ProgramPolicy, *InstancePolicy, string) (Context, error)

	authorizer() // Force inheritance.
}

// NoAccess permitted to any resource.
type NoAccess struct{}

var errNoAccess = PermissionDenied("no access policy")

func (NoAccess) Authorize(ctx Context) (Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeProgram(ctx Context, _ *ResourcePolicy, _ *ProgramPolicy) (Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeProgramSource(ctx Context, _ *ResourcePolicy, _ *ProgramPolicy, _ string) (Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeInstance(ctx Context, _ *ResourcePolicy, _ *InstancePolicy) (Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeProgramInstance(ctx Context, _ *ResourcePolicy, _ *ProgramPolicy, _ *InstancePolicy) (Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeProgramInstanceSource(ctx Context, _ *ResourcePolicy, _ *ProgramPolicy, _ *InstancePolicy, _ string) (Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) authorizer() {}

// AccessConfig utility for Authorizer implementations.
// InstancePolicy.Services must be set explicitly, other fields have defaults.
type AccessConfig struct {
	ResourcePolicy
	ProgramPolicy
	InstancePolicy
}

var DefaultAccessConfig = AccessConfig{
	ResourcePolicy{
		DefaultMaxModules,
		DefaultMaxProcs,
		DefaultTotalStorageSize,
		DefaultTotalResidentSize,
	},
	ProgramPolicy{
		DefaultMaxModuleSize,
		DefaultMaxTextSize,
		DefaultStackSize,
	},
	InstancePolicy{
		DefaultMaxMemorySize,
		DefaultStackSize,
		DefaultTimeResolution,
		nil,
	},
}

func (config *AccessConfig) ConfigureResource(p *ResourcePolicy) {
	*p = config.ResourcePolicy
	if p.MaxModules == 0 {
		p.MaxModules = DefaultMaxModules
	}
	if p.MaxProcs == 0 {
		p.MaxProcs = DefaultMaxProcs
	}
	if p.TotalStorageSize == 0 {
		p.TotalStorageSize = DefaultTotalStorageSize
	}
	if p.TotalResidentSize == 0 {
		p.TotalResidentSize = DefaultTotalResidentSize
	}
}

func (config *AccessConfig) ConfigureProgram(p *ProgramPolicy) {
	*p = config.ProgramPolicy
	if p.MaxModuleSize == 0 {
		p.MaxModuleSize = DefaultMaxModuleSize
	}
	if p.MaxTextSize == 0 {
		p.MaxTextSize = DefaultMaxTextSize
	}
	if p.MaxStackSize == 0 {
		p.MaxStackSize = DefaultStackSize
	}
}

func (config *AccessConfig) ConfigureInstance(p *InstancePolicy) {
	*p = config.InstancePolicy
	if p.MaxMemorySize == 0 {
		p.MaxMemorySize = DefaultMaxMemorySize
	}
	if p.StackSize == 0 {
		p.StackSize = DefaultStackSize
	}
	if p.TimeResolution == 0 {
		p.TimeResolution = DefaultTimeResolution
	}
}

// PublicAccess authorization for everyone, including anonymous requests.
// Configurable resource limits.
type PublicAccess struct {
	AccessConfig
}

func NewPublicAccess(services func(Context) InstanceServices) *PublicAccess {
	a := new(PublicAccess)
	a.Services = services
	return a
}

func (*PublicAccess) Authorize(ctx Context) (Context, error) {
	return ctx, nil
}

func (a *PublicAccess) AuthorizeProgram(ctx Context, res *ResourcePolicy, prog *ProgramPolicy) (Context, error) {
	a.ConfigureResource(res)
	a.ConfigureProgram(prog)
	return ctx, nil
}

func (a *PublicAccess) AuthorizeProgramSource(ctx Context, res *ResourcePolicy, prog *ProgramPolicy, _ string) (Context, error) {
	a.ConfigureResource(res)
	a.ConfigureProgram(prog)
	return ctx, nil
}

func (a *PublicAccess) AuthorizeInstance(ctx Context, res *ResourcePolicy, inst *InstancePolicy) (Context, error) {
	a.ConfigureResource(res)
	a.ConfigureInstance(inst)
	return ctx, nil
}

func (a *PublicAccess) AuthorizeProgramInstance(ctx Context, res *ResourcePolicy, prog *ProgramPolicy, inst *InstancePolicy) (Context, error) {
	a.ConfigureResource(res)
	a.ConfigureProgram(prog)
	a.ConfigureInstance(inst)
	return ctx, nil
}

func (a *PublicAccess) AuthorizeProgramInstanceSource(ctx Context, res *ResourcePolicy, prog *ProgramPolicy, inst *InstancePolicy, _ string) (Context, error) {
	a.ConfigureResource(res)
	a.ConfigureProgram(prog)
	a.ConfigureInstance(inst)
	return ctx, nil
}

func (p *PublicAccess) authorizer() {}
