// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshkeys

import (
	"context"
	"fmt"
	"io/ioutil"

	"gate.computer/gate/internal/principal"
	"gate.computer/gate/scope/program/system"
	"gate.computer/gate/server"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

var errUnauthorized = server.AccessUnauthorized("missing authentication credentials")
var errForbidden = server.AccessForbidden("key not authorized")

// AuthorizedKeys authorizes access for the supported (ssh-ed25519) public keys
// found in an SSH authorized_keys file.
//
// Request signatures must be verified separately by an API layer (e.g. package
// server/web).
type AuthorizedKeys struct {
	server.NoAccess
	server.AccessConfig

	publicKeys map[[ed25519.PublicKeySize]byte]string
}

func (ak *AuthorizedKeys) ParseFile(uid, filename string) (err error) {
	text, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}

	return ak.Parse(uid, text)
}

func (ak *AuthorizedKeys) Parse(uid string, text []byte) error {
	if ak.publicKeys == nil {
		ak.publicKeys = make(map[[ed25519.PublicKeySize]byte]string)
	}

	for len(text) > 0 {
		sshKey, comment, _, rest, err := ssh.ParseAuthorizedKey(text)
		if err != nil {
			return err
		}

		if sshKey.Type() == ssh.KeyAlgoED25519 {
			cryptoKey := sshKey.(ssh.CryptoPublicKey).CryptoPublicKey()

			var buf [ed25519.PublicKeySize]byte

			key := cryptoKey.(ed25519.PublicKey)
			if len(key) != len(buf) {
				return fmt.Errorf("invalid %s public key (%s)", sshKey.Type(), comment)
			}

			copy(buf[:], key)

			if x, exists := ak.publicKeys[buf]; exists && x != uid {
				return fmt.Errorf("%s public key with multiple uids", sshKey.Type())
			}

			ak.publicKeys[buf] = uid
		}

		text = rest
	}

	return nil
}

func (ak *AuthorizedKeys) Authorize(ctx context.Context) (context.Context, error) {
	pri := principal.ContextID(ctx)
	if pri == nil {
		return ctx, errUnauthorized
	}

	uid, found := ak.publicKeys[principal.Raw(pri)]
	if !found {
		return ctx, errForbidden
	}

	if server.ScopeContains(ctx, system.Scope) {
		ctx = system.ContextWithUserID(ctx, uid)
	}
	return ctx, nil
}

func (ak *AuthorizedKeys) AuthorizeProgram(ctx context.Context, res *server.ResourcePolicy, prog *server.ProgramPolicy) (context.Context, error) {
	ak.ConfigureResource(res)
	ak.ConfigureProgram(prog)
	return ak.Authorize(ctx)
}

func (ak *AuthorizedKeys) AuthorizeProgramSource(ctx context.Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, _ server.Source) (context.Context, error) {
	return ak.AuthorizeProgram(ctx, res, prog)
}

func (ak *AuthorizedKeys) AuthorizeInstance(ctx context.Context, res *server.ResourcePolicy, inst *server.InstancePolicy) (context.Context, error) {
	ak.ConfigureResource(res)
	ak.ConfigureInstance(inst)
	return ak.Authorize(ctx)
}

func (ak *AuthorizedKeys) AuthorizeProgramInstance(ctx context.Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, inst *server.InstancePolicy) (context.Context, error) {
	ak.ConfigureResource(res)
	ak.ConfigureProgram(prog)
	ak.ConfigureInstance(inst)
	return ak.Authorize(ctx)
}

func (ak *AuthorizedKeys) AuthorizeProgramInstanceSource(ctx context.Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, inst *server.InstancePolicy, _ server.Source) (context.Context, error) {
	return ak.AuthorizeProgramInstance(ctx, res, prog, inst)
}
