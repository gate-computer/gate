// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/tsavola/wag/wa"
)

const (
	DefaultMaxModules        = 64
	DefaultMaxProcs          = 4
	DefaultTotalStorageSize  = 256 * 1024 * 1024
	DefaultTotalResidentSize = 64 * 1024 * 1024
	DefaultMaxModuleSize     = 16 * 1024 * 1024
	DefaultMaxTextSize       = 16 * 1024 * 1024
	DefaultMaxMemorySize     = 32 * 1024 * 1024
	DefaultStackSize         = wa.PageSize
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
}

type InstancePolicy struct {
	MaxMemorySize int           // Linear memory growth limit.
	StackSize     int           // Including system/runtime overhead.
	Services      ServicePolicy // Defines the set of available services.
}

// AccessAuthorizer authenticates, authorizes and limits access to Server
// methods.  If PrincipalKey is nil, the request didn't contain credentials,
// and the access should be denied unless the policy allows anonymous access.
// If PrincipalKey is non-nil, it should be checked unless the policy allows
// access to everyone.
//
// The methods should return Unauthorized, Forbidden or TooManyRequests errors
// to signal successful prevention of access.  Other types of errors are
// interpreted as failures of the authorization mechanism.  Returning nil
// grants access.
//
// The implementation should adjust the ResourcePolicy, InstancePolicy and
// ProgramPolicy objects' fields.  The limits are enforced automatically by the
// server, which may also lead to denial of access.
//
// AccessAuthorizer may be expanded with new methods (prefixed with the
// Authorize namespace) also between major releases.  Implementations must
// inherit methods from a concrete access authorization type, and must not add
// unrelated methods with the Authorize prefix to avoid breakage.
//
// The conservative choice is to inherit from NoAccess.  That way, new
// functionality will be effectively disabled.
type AccessAuthorizer interface {
	AuthorizeProgramContent(context.Context, *PrincipalKey, *ResourcePolicy, *ProgramPolicy) error
	AuthorizeInstanceProgramContent(context.Context, *PrincipalKey, *ResourcePolicy, *InstancePolicy, *ProgramPolicy) error
	AuthorizeInstanceProgramSource(context.Context, *PrincipalKey, *ResourcePolicy, *InstancePolicy, *ProgramPolicy, Source) error
	AuthorizeInstance(context.Context, *PrincipalKey, *ResourcePolicy, *InstancePolicy) error
	Authorize(context.Context, *PrincipalKey) error

	accessAuthorizer() // Force inheritance.
}

// Unauthorized access error.  The client is denied to access the server.
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

func (NoAccess) AuthorizeProgramContent(context.Context, *PrincipalKey, *ResourcePolicy, *ProgramPolicy) error {
	return errNoAccess
}

func (NoAccess) AuthorizeInstanceProgramContent(context.Context, *PrincipalKey, *ResourcePolicy, *InstancePolicy, *ProgramPolicy) error {
	return errNoAccess
}

func (NoAccess) AuthorizeInstanceProgramSource(context.Context, *PrincipalKey, *ResourcePolicy, *InstancePolicy, *ProgramPolicy, Source) error {
	return errNoAccess
}

func (NoAccess) AuthorizeInstance(context.Context, *PrincipalKey, *ResourcePolicy, *InstancePolicy) error {
	return errNoAccess
}

func (NoAccess) Authorize(context.Context, *PrincipalKey) error {
	return errNoAccess
}

func (NoAccess) accessAuthorizer() {}

// AccessConfig utility for AccessAuthorizer implementations.
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
	},
	InstancePolicy{
		DefaultMaxMemorySize,
		DefaultStackSize,
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
}

func (config *AccessConfig) ConfigureInstance(policy *InstancePolicy) {
	*policy = config.InstancePolicy

	if policy.MaxMemorySize == 0 {
		policy.MaxMemorySize = DefaultMaxMemorySize
	}
	if policy.StackSize == 0 {
		policy.StackSize = DefaultStackSize
	}
}

// PublicAccess authorization for everyone, including anonymous requests.
// Configurable resource limits.
type PublicAccess struct {
	AccessConfig
}

func NewPublicAccess(services ServicePolicy) (p *PublicAccess) {
	p = new(PublicAccess)
	p.Services = services
	return
}

func (p *PublicAccess) AuthorizeProgramContent(_ context.Context, _ *PrincipalKey, res *ResourcePolicy, prog *ProgramPolicy) error {
	p.ConfigureResource(res)
	p.ConfigureProgram(prog)
	return nil
}

func (p *PublicAccess) AuthorizeInstanceProgramContent(_ context.Context, _ *PrincipalKey, res *ResourcePolicy, inst *InstancePolicy, prog *ProgramPolicy) error {
	p.ConfigureResource(res)
	p.ConfigureProgram(prog)
	p.ConfigureInstance(inst)
	return nil
}

func (p *PublicAccess) AuthorizeInstanceProgramSource(_ context.Context, _ *PrincipalKey, res *ResourcePolicy, inst *InstancePolicy, prog *ProgramPolicy, _ Source) error {
	p.ConfigureResource(res)
	p.ConfigureProgram(prog)
	p.ConfigureInstance(inst)
	return nil
}

func (p *PublicAccess) AuthorizeInstance(_ context.Context, _ *PrincipalKey, res *ResourcePolicy, inst *InstancePolicy) error {
	p.ConfigureResource(res)
	p.ConfigureInstance(inst)
	return nil
}

func (p *PublicAccess) Authorize(_ context.Context, _ *PrincipalKey) error {
	return nil
}

func (*PublicAccess) accessAuthorizer() {}

func init() {
	var _ AccessAuthorizer = NoAccess{}
	var _ AccessAuthorizer = new(PublicAccess)
}
