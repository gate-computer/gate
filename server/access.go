// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"
	"time"

	"github.com/tsavola/gate/principal"
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

// Authorizer and moderator of server access.  If principal key is nil, the
// request didn't contain credentials, and the access should be denied unless
// the policy allows anonymous access.  If principal key is non-nil, it should
// be checked unless the policy allows access to everyone.
//
// The methods should return Unauthorized, Forbidden or TooManyRequests errors
// to signal successful prevention of access.  Other types of errors are
// interpreted as failures of the authorization mechanism.  Returning nil
// grants access.
//
// An implementation should adjust the ResourcePolicy, ProgramPolicy and
// InstancePolicy objects' fields.  The limits are enforced automatically by
// the server, which may also lead to denial of access.
//
// An implementation may choose to discriminate based on server operation type.
// It can be obtained using the ContextOp(context.Context) function.
//
// Authorizer may be expanded with new methods (prefixed with the Authorize
// namespace) also between major releases.  Implementations must inherit
// methods from a concrete access authorization type, and must not add
// unrelated methods with the Authorize prefix to avoid breakage.
//
// The conservative choice is to inherit from NoAccess.  That way, new
// functionality will be effectively disabled.
type Authorizer interface {
	Authorize(context.Context, *principal.ID) error
	AuthorizeProgram(context.Context, *principal.ID, *ResourcePolicy, *ProgramPolicy) error
	AuthorizeProgramSource(context.Context, *principal.ID, *ResourcePolicy, *ProgramPolicy, Source) error
	AuthorizeInstance(context.Context, *principal.ID, *ResourcePolicy, *InstancePolicy) error
	AuthorizeProgramInstance(context.Context, *principal.ID, *ResourcePolicy, *ProgramPolicy, *InstancePolicy) error
	AuthorizeProgramInstanceSource(context.Context, *principal.ID, *ResourcePolicy, *ProgramPolicy, *InstancePolicy, Source) error

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

func (NoAccess) Authorize(context.Context, *principal.ID) error {
	return errNoAccess
}

func (NoAccess) AuthorizeProgram(context.Context, *principal.ID, *ResourcePolicy, *ProgramPolicy) error {
	return errNoAccess
}

func (NoAccess) AuthorizeProgramSource(context.Context, *principal.ID, *ResourcePolicy, *ProgramPolicy, Source) error {
	return errNoAccess
}

func (NoAccess) AuthorizeInstance(context.Context, *principal.ID, *ResourcePolicy, *InstancePolicy) error {
	return errNoAccess
}

func (NoAccess) AuthorizeProgramInstance(context.Context, *principal.ID, *ResourcePolicy, *ProgramPolicy, *InstancePolicy) error {
	return errNoAccess
}

func (NoAccess) AuthorizeProgramInstanceSource(context.Context, *principal.ID, *ResourcePolicy, *ProgramPolicy, *InstancePolicy, Source) error {
	return errNoAccess
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

func (config *AccessConfig) ConfigureResource(policy *ResourcePolicy) {
	*policy = config.ResourcePolicy

	if policy.MaxModules == 0 {
		policy.MaxModules = DefaultMaxModules
	}
	if policy.MaxProcs == 0 {
		policy.MaxProcs = DefaultMaxProcs
	}
	if policy.TotalStorageSize == 0 {
		policy.TotalStorageSize = DefaultTotalStorageSize
	}
	if policy.TotalResidentSize == 0 {
		policy.TotalResidentSize = DefaultTotalResidentSize
	}
}

func (config *AccessConfig) ConfigureProgram(policy *ProgramPolicy) {
	*policy = config.ProgramPolicy

	if policy.MaxModuleSize == 0 {
		policy.MaxModuleSize = DefaultMaxModuleSize
	}
	if policy.MaxTextSize == 0 {
		policy.MaxTextSize = DefaultMaxTextSize
	}
	if policy.MaxStackSize == 0 {
		policy.MaxStackSize = DefaultStackSize
	}
}

func (config *AccessConfig) ConfigureInstance(policy *InstancePolicy) {
	*policy = config.InstancePolicy

	if policy.MaxMemorySize == 0 {
		policy.MaxMemorySize = DefaultMaxMemorySize
	}
	if policy.StackSize == 0 {
		policy.StackSize = DefaultStackSize
	}
	if policy.TimeResolution == 0 {
		policy.TimeResolution = DefaultTimeResolution
	}
}

// PublicAccess authorization for everyone, including anonymous requests.
// Configurable resource limits.
type PublicAccess struct {
	AccessConfig
}

func NewPublicAccess(services func(context.Context) InstanceServices) (p *PublicAccess) {
	p = new(PublicAccess)
	p.Services = services
	return
}

func (p *PublicAccess) Authorize(_ context.Context, _ *principal.ID) error {
	return nil
}

func (p *PublicAccess) AuthorizeProgram(_ context.Context, _ *principal.ID, res *ResourcePolicy, prog *ProgramPolicy) error {
	p.ConfigureResource(res)
	p.ConfigureProgram(prog)
	return nil
}

func (p *PublicAccess) AuthorizeProgramSource(_ context.Context, _ *principal.ID, res *ResourcePolicy, prog *ProgramPolicy, _ Source) error {
	p.ConfigureResource(res)
	p.ConfigureProgram(prog)
	return nil
}

func (p *PublicAccess) AuthorizeInstance(_ context.Context, _ *principal.ID, res *ResourcePolicy, inst *InstancePolicy) error {
	p.ConfigureResource(res)
	p.ConfigureInstance(inst)
	return nil
}

func (p *PublicAccess) AuthorizeProgramInstance(_ context.Context, _ *principal.ID, res *ResourcePolicy, prog *ProgramPolicy, inst *InstancePolicy) error {
	p.ConfigureResource(res)
	p.ConfigureProgram(prog)
	p.ConfigureInstance(inst)
	return nil
}

func (p *PublicAccess) AuthorizeProgramInstanceSource(_ context.Context, _ *principal.ID, res *ResourcePolicy, prog *ProgramPolicy, inst *InstancePolicy, _ Source) error {
	p.ConfigureResource(res)
	p.ConfigureProgram(prog)
	p.ConfigureInstance(inst)
	return nil
}

func (p *PublicAccess) authorizer() {}
