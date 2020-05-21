// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package principal

import (
	"context"
	"fmt"
	"strings"
)

const (
	typeLocal   = "local"
	typeEd25519 = "ed25519"
)

type ID struct {
	key [keySize]byte
	s   string
}

var LocalID = &ID{s: typeLocal}

func ParseID(s string) (*ID, error) {
	if x := strings.SplitN(s, ":", 2); len(x) == 2 {
		switch x[0] {
		case typeEd25519:
			id := &ID{s: s}
			if parseEd25519Key(id.key[:], x[1]) == nil {
				return id, nil
			}
		}
	}

	if s == typeLocal {
		return LocalID, nil
	}

	return nil, fmt.Errorf("principal ID string is invalid: %q", s)
}

func (id *ID) String() string {
	return id.s
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
