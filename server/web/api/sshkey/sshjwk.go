// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshkey

import (
	"fmt"

	"gate.computer/gate/server/web/api"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

func ParsePublicKey(line []byte) (jwk *api.PublicKey, err error) {
	sshKey, _, _, _, err := ssh.ParseAuthorizedKey(line)
	if err != nil {
		return
	}

	switch algo := sshKey.Type(); algo {
	case ssh.KeyAlgoED25519:
		cryptoKey := sshKey.(ssh.CryptoPublicKey).CryptoPublicKey()
		jwk = api.PublicKeyEd25519(cryptoKey.(ed25519.PublicKey))
		return

	default:
		err = fmt.Errorf("unsupported key type: %s", algo)
		return
	}
}
