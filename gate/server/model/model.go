// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

import (
	"errors"
	"time"

	"gate.computer/gate/principal"
	"google.golang.org/protobuf/proto"

	. "import.name/type/context"
)

// ErrNonceReused may be returned by [NonceChecker.CheckNonce].
var ErrNonceReused = errors.New("nonce reused")

type Inventory interface {
	GetModule(ctx Context, pri principal.ID, key string, buf proto.Message) (found bool, err error)
	PutModule(ctx Context, pri principal.ID, key string, buf proto.Message) error
	UpdateModule(ctx Context, pri principal.ID, key string, buf proto.Message) error
	RemoveModule(ctx Context, pri principal.ID, key string) error

	GetInstance(ctx Context, pri principal.ID, key string, buf proto.Message) (found bool, err error)
	PutInstance(ctx Context, pri principal.ID, key string, buf proto.Message) error
	UpdateInstance(ctx Context, pri principal.ID, key string, buf proto.Message) error
	RemoveInstance(ctx Context, pri principal.ID, key string) error
}

type SourceCache interface {
	GetSourceSHA256(ctx Context, uri string) (hash string, err error)
	PutSourceSHA256(ctx Context, uri, hash string) error
}

type NonceChecker interface {
	CheckNonce(ctx Context, scope []byte, nonce string, expires time.Time) error
}
