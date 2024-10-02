// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sql

import (
	"time"

	"gate.computer/gate/server/database"

	. "import.name/type/context"
)

const NonceSchema = `
CREATE TABLE IF NOT EXISTS nonce (
	scope BYTEA NOT NULL,
	nonce TEXT NOT NULL,
	expire BIGINT NOT NULL,

	PRIMARY KEY (scope, nonce)
);

CREATE INDEX IF NOT EXISTS nonce_expire ON nonce (expire);
`

func (x *Endpoint) InitNonceChecker(ctx Context) error {
	_, err := x.db.ExecContext(ctx, NonceSchema)
	return err
}

func (x *Endpoint) CheckNonce(ctx Context, scope []byte, nonce string, expire time.Time) error {
	conn, err := x.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, "DELETE FROM nonce WHERE expire < $1", time.Now().Unix())
	if err != nil {
		return err
	}

	result, err := conn.ExecContext(ctx, "INSERT INTO nonce (scope, nonce, expire) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", scope, nonce, expire.Unix())
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return database.ErrNonceReused
	}

	return nil
}
