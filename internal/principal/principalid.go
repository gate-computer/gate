// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package principal

import (
	"context"
	"fmt"
	"strings"
)

type Type string

const (
	TypeLocal   Type = "local"
	TypeEd25519      = "ed25519"
)

type ID struct {
	key [keySize]byte
	s   string
}

var LocalID = &ID{s: string(TypeLocal)}

func ParseID(s string) (*ID, error) {
	if x := strings.SplitN(s, ":", 2); len(x) == 2 {
		switch x[0] {
		case TypeEd25519:
			id := &ID{s: s}
			if parseEd25519Key(id.key[:], x[1]) == nil {
				return id, nil
			}
		}
	}

	if s == string(TypeLocal) {
		return LocalID, nil
	}

	return nil, fmt.Errorf("principal ID string is invalid: %q", s)
}

func (id *ID) Type() Type {
	t, _ := Split(id)
	return t
}

// PublicKey associated with the ID.  Panics if there isn't one.
//
// If the ID type is ed25519, a base64url-encoded public key is returned.
func (id *ID) PublicKey() string {
	t, k := Split(id)
	if t != TypeEd25519 {
		panic(t)
	}
	return k
}

func (id *ID) String() string {
	return id.s
}

func Split(id *ID) (Type, string) {
	if x := strings.SplitN(id.s, ":", 2); len(x) == 2 {
		return Type(x[0]), x[1]
	}
	return Type(id.s), ""
}

func Raw(id *ID) [keySize]byte {
	return id.key
}

type contextIDValueKey struct{}

func ContextWithID(ctx context.Context, id *ID) context.Context {
	return context.WithValue(ctx, contextIDValueKey{}, id)
}

func ContextID(ctx context.Context) *ID {
	id, _ := ctx.Value(contextIDValueKey{}).(*ID)
	return id
}
