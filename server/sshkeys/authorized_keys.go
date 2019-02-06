// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshkeys

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/tsavola/gate/server"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

var errUnauthorized = server.AccessUnauthorized("missing authentication credentials")
var errForbidden = server.AccessForbidden("key not authorized")

// AuthorizedKeys authorizes access for the supported (ssh-ed25519) public keys
// found in an SSH authorized_keys file.
//
// Request signatures must be verified separately by an API layer (e.g. package
// webserver).
type AuthorizedKeys struct {
	server.NoAccess
	server.AccessConfig
	Services func(uid string) server.InstanceServices

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

func (ak *AuthorizedKeys) Authenticate(pri *server.PrincipalKey) (uid string, err error) {
	if pri == nil {
		err = errUnauthorized
		return
	}

	if key, ok := pri.KeyPtr(ed25519.PublicKeySize).(*[ed25519.PublicKeySize]byte); ok {
		if x, found := ak.publicKeys[*key]; found {
			uid = x
			return
		}
	}

	err = errForbidden
	return
}

func (ak *AuthorizedKeys) ConfigureInstance(policy *server.InstancePolicy, uid string) {
	ak.AccessConfig.ConfigureInstance(policy)

	if ak.Services != nil {
		policy.Services = func() server.InstanceServices {
			return ak.Services(uid)
		}
	}
}

func (ak *AuthorizedKeys) AuthorizeProgramContent(_ context.Context, pri *server.PrincipalKey, resPolicy *server.ResourcePolicy, progPolicy *server.ProgramPolicy) error {
	_, err := ak.Authenticate(pri)
	if err == nil {
		ak.ConfigureResource(resPolicy)
		ak.ConfigureProgram(progPolicy)
	}
	return err
}

func (ak *AuthorizedKeys) AuthorizeInstanceProgramContent(_ context.Context, pri *server.PrincipalKey, resPolicy *server.ResourcePolicy, instPolicy *server.InstancePolicy, progPolicy *server.ProgramPolicy) error {
	uid, err := ak.Authenticate(pri)
	if err == nil {
		ak.ConfigureResource(resPolicy)
		ak.ConfigureProgram(progPolicy)
		ak.ConfigureInstance(instPolicy, uid)
	}
	return err
}

func (ak *AuthorizedKeys) AuthorizeInstanceProgramSource(_ context.Context, pri *server.PrincipalKey, resPolicy *server.ResourcePolicy, instPolicy *server.InstancePolicy, progPolicy *server.ProgramPolicy, _ server.Source) error {
	uid, err := ak.Authenticate(pri)
	if err == nil {
		ak.ConfigureResource(resPolicy)
		ak.ConfigureProgram(progPolicy)
		ak.ConfigureInstance(instPolicy, uid)
	}
	return err
}

func (ak *AuthorizedKeys) AuthorizeInstance(_ context.Context, pri *server.PrincipalKey, resPolicy *server.ResourcePolicy, instPolicy *server.InstancePolicy) error {
	uid, err := ak.Authenticate(pri)
	if err == nil {
		ak.ConfigureResource(resPolicy)
		ak.ConfigureInstance(instPolicy, uid)
	}
	return err
}

func (ak *AuthorizedKeys) Authorize(_ context.Context, pri *server.PrincipalKey) error {
	_, err := ak.Authenticate(pri)
	return err
}
