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
	typeEd25519 = "ed25519"
)

type ID struct {
	s string
}

func makeEd25519ID(encodedKey string) ID {
	return ID{typeEd25519 + ":" + encodedKey}
}

func ParseID(s string) (id ID, err error) {
	if s == "" {
		return
	}

	if x := strings.SplitN(s, ":", 2); len(x) == 2 {
		switch x[0] {
		case typeEd25519:
			if pri, e := ParseEd25519Key(x[1]); e == nil {
				id = pri.principalID
				return
			}
		}
	}

	err = fmt.Errorf("principal ID string is invalid: %q", s)
	return
}

func (id ID) String() string {
	return id.s
}

type contextKey int

const (
	ContextID contextKey = iota
)

func ContextWithID(ctx context.Context, id ID) context.Context {
	if id.s == "" {
		return ctx
	}

	return context.WithValue(ctx, ContextID, id)
}
