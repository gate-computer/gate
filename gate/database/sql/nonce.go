// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sql

import (
	"time"

	"gate.computer/gate/server/model"

	. "import.name/type/context"
)

const NonceSchema = `
CREATE TABLE IF NOT EXISTS nonce (
	scope BLOB NOT NULL,
	nonce TEXT NOT NULL,
	expire BIGINT NOT NULL,

	PRIMARY KEY (scope, nonce)
) WITHOUT ROWID, STRICT;

CREATE INDEX IF NOT EXISTS nonce_expire ON nonce (expire);
`

func (x *Endpoint) InitNonceChecker(ctx Context) error {
	_, err := x.db.ExecContext(ctx, x.adjustSchema(NonceSchema))
	return err
}

func (x *Endpoint) CheckNonce(ctx Context, scope []byte, nonce string, expire time.Time) error {
	conn, err := x.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	q := "DELETE FROM nonce WHERE expire < $1"
	if _, err := conn.ExecContext(ctx, q, time.Now().Unix()); err != nil {
		return err
	}

	q = "INSERT INTO nonce (scope, nonce, expire) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING"
	result, err := conn.ExecContext(ctx, q, scope, nonce, expire.Unix())
	if err != nil {
		return err
	}

	switch n, err := result.RowsAffected(); {
	case err != nil:
		return err
	case n == 0:
		return model.ErrNonceReused
	default:
		return nil
	}
}
