// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshkey

import (
	"crypto/ed25519"
	"fmt"

	"gate.computer/gate/web"
	"golang.org/x/crypto/ssh"
)

func ParsePublicKey(line []byte) (*web.PublicKey, error) {
	sshKey, _, _, _, err := ssh.ParseAuthorizedKey(line)
	if err != nil {
		return nil, err
	}

	switch algo := sshKey.Type(); algo {
	case ssh.KeyAlgoED25519:
		cryptoKey := sshKey.(ssh.CryptoPublicKey).CryptoPublicKey()
		jwk := web.PublicKeyEd25519(cryptoKey.(ed25519.PublicKey))
		return jwk, nil

	default:
		return nil, fmt.Errorf("unsupported key type: %s", algo)
	}
}
