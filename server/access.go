// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"
	"time"

	"github.com/tsavola/wag/wa"
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
	MaxModules        int // Module reference limit.
	MaxProcs          int // Active instance limit.
	TotalStorageSize  int // Sum of referenced module and metadata sizes.
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
	TimeResolution time.Duration // Granularity of gate.time function.

	// Services function defines which services are discoverable by the
	// instance.
	Services func(ctx context.Context) InstanceServices

	// Debug function parses the debug option string and provides a debug
	// output writer, with status indicating what is happening.
	Debug func(ctx context.Context, option string) (status string, output io.WriteCloser, err error)
}

// Authorizer and moderator of server access.
//
// The methods should return Unauthorized, Forbidden or TooManyRequests errors
// to signal successful prevention of access.  Other types of errors are
// interpreted as failures of the authorization mechanism.  Returning a nil
// error grants access.
//
// An implementation should adjust the ResourcePolicy, ProgramPolicy and
// InstancePolicy objects' fields.  The limits are enforced automatically by
// the server, which may also lead to denial of access.
//
// Principal id can be obtained using the principal.ContextID(context.Context)
// function.  If it is nil, the request didn't contain credentials, and the
// access should be denied unless the policy allows anonymous access.  If the
// principal id is non-nil, it should be checked unless the policy allows
// access to everyone.
//
// An implementation may choose to discriminate based on server operation type.
// It can be obtained using the ContextOp(context.Context) function.
//
// Authorizer may be expanded with new methods (prefixed with the Authorize
// namespace) also between major releases.  Implementations must inherit
// methods from a concrete access authorization type, and must not add
// unrelated methods with the Authorize prefix to avoid breakage.  The
// conservative choice is to inherit from NoAccess.  That way, new
// functionality will be effectively disabled.
type Authorizer interface {
	Authorize(context.Context) (context.Context, error)
	AuthorizeProgram(context.Context, *ResourcePolicy, *ProgramPolicy) (context.Context, error)
	AuthorizeProgramSource(context.Context, *ResourcePolicy, *ProgramPolicy, Source) (context.Context, error)
	AuthorizeInstance(context.Context, *ResourcePolicy, *InstancePolicy) (context.Context, error)
	AuthorizeProgramInstance(context.Context, *ResourcePolicy, *ProgramPolicy, *InstancePolicy) (context.Context, error)
	AuthorizeProgramInstanceSource(context.Context, *ResourcePolicy, *ProgramPolicy, *InstancePolicy, Source) (context.Context, error)

	authorizer() // Force inheritance.
}

// Unauthorized access error.  The client is denied access to the server.
type Unauthorized interface {
	error
	Unauthorized()
}

// AccessUnauthorized error.  The reason will be shown to the client.
func AccessUnauthorized(publicReason string) Unauthorized {
	return accessUnauthorized(publicReason)
}

type accessUnauthorized string

func (s accessUnauthorized) Error() string       { return string(s) }
func (s accessUnauthorized) PublicError() string { return string(s) }
func (s accessUnauthorized) Unauthorized()       {}

// Forbidden access error.  The client is denied access to a resource.
type Forbidden interface {
	error
	Forbidden()
}

// AccessForbidden error.  The details are not exposed to the client.
func AccessForbidden(internalDetails string) Forbidden {
	return accessForbidden(internalDetails)
}

type accessForbidden string

func (s accessForbidden) Error() string       { return string(s) }
func (s accessForbidden) PublicError() string { return "access denied" }
func (s accessForbidden) Forbidden()          {}

// NoAccess permitted to any resource.
type NoAccess struct{}

var errNoAccess = AccessForbidden("no access policy")

func (NoAccess) Authorize(ctx context.Context) (context.Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeProgram(ctx context.Context, _ *ResourcePolicy, _ *ProgramPolicy) (context.Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeProgramSource(ctx context.Context, _ *ResourcePolicy, _ *ProgramPolicy, _ Source) (context.Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeInstance(ctx context.Context, _ *ResourcePolicy, _ *InstancePolicy) (context.Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeProgramInstance(ctx context.Context, _ *ResourcePolicy, _ *ProgramPolicy, _ *InstancePolicy) (context.Context, error) {
	return ctx, errNoAccess
}

func (NoAccess) AuthorizeProgramInstanceSource(ctx context.Context, _ *ResourcePolicy, _ *ProgramPolicy, _ *InstancePolicy, _ Source) (context.Context, error) {
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

func NewPublicAccess(services func(context.Context) InstanceServices) *PublicAccess {
	a := new(PublicAccess)
	a.Services = services
	return a
}

func (*PublicAccess) Authorize(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (a *PublicAccess) AuthorizeProgram(ctx context.Context, res *ResourcePolicy, prog *ProgramPolicy) (context.Context, error) {
	a.ConfigureResource(res)
	a.ConfigureProgram(prog)
	return ctx, nil
}

func (a *PublicAccess) AuthorizeProgramSource(ctx context.Context, res *ResourcePolicy, prog *ProgramPolicy, _ Source) (context.Context, error) {
	a.ConfigureResource(res)
	a.ConfigureProgram(prog)
	return ctx, nil
}

func (a *PublicAccess) AuthorizeInstance(ctx context.Context, res *ResourcePolicy, inst *InstancePolicy) (context.Context, error) {
	a.ConfigureResource(res)
	a.ConfigureInstance(inst)
	return ctx, nil
}

func (a *PublicAccess) AuthorizeProgramInstance(ctx context.Context, res *ResourcePolicy, prog *ProgramPolicy, inst *InstancePolicy) (context.Context, error) {
	a.ConfigureResource(res)
	a.ConfigureProgram(prog)
	a.ConfigureInstance(inst)
	return ctx, nil
}

func (a *PublicAccess) AuthorizeProgramInstanceSource(ctx context.Context, res *ResourcePolicy, prog *ProgramPolicy, inst *InstancePolicy, _ Source) (context.Context, error) {
	a.ConfigureResource(res)
	a.ConfigureProgram(prog)
	a.ConfigureInstance(inst)
	return ctx, nil
}

func (p *PublicAccess) authorizer() {}
