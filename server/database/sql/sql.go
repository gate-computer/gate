// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sql implements an AccessTracker backed by an SQL database.
//
// Supports at least PostgreSQL 9.5+ (github.com/lib/pq) and SQLite 3.24+
// (github.com/mattn/go-sqlite3).
package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/tsavola/gate/server/database"
)

func init() {
	database.Register("sql", database.Adapter{
		NewConfig: func() interface{} {
			return new(Config)
		},

		OpenNonceChecker: func(ctx context.Context, config interface{}) (database.NonceChecker, error) {
			nr, err := OpenNonceChecker(ctx, *config.(*Config))
			if err != nil {
				return nil, err
			}

			return nr, err
		},
	})
}

const NonceSchema = `
CREATE TABLE IF NOT EXISTS nonce (
	scope BYTEA NOT NULL,
	nonce TEXT NOT NULL,
	expire BIGINT NOT NULL,

	PRIMARY KEY (scope, nonce)
);

CREATE INDEX IF NOT EXISTS nonce_expire ON nonce (expire);
`

type Config struct {
	Driver string // Ignored if Connector is defined.
	DSN    string //

	Connector driver.Connector // Overrides Driver and DSN.
}

type NonceChecker struct {
	db *sql.DB
}

func OpenNonceChecker(ctx context.Context, config Config) (*NonceChecker, error) {
	var err error
	var nr = new(NonceChecker)

	if config.Connector != nil {
		nr.db = sql.OpenDB(config.Connector)
	} else {
		nr.db, err = sql.Open(config.Driver, config.DSN)
		if err != nil {
			return nil, err
		}
	}
	defer func() {
		if err != nil {
			nr.db.Close()
		}
	}()

	_, err = nr.db.ExecContext(ctx, NonceSchema)
	if err != nil {
		return nil, err
	}

	return nr, nil
}

func (nr *NonceChecker) Close() error {
	return nr.db.Close()
}

func (nr *NonceChecker) CheckNonce(ctx context.Context, scope []byte, nonce string, expire time.Time,
) (err error) {
	conn, err := nr.db.Conn(ctx)
	if err != nil {
		return
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, "DELETE FROM nonce WHERE expire < $1", time.Now().Unix())
	if err != nil {
		return
	}

	result, err := conn.ExecContext(ctx, "INSERT INTO nonce (scope, nonce, expire) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", scope, nonce, expire.Unix())
	if err != nil {
		return
	}

	n, err := result.RowsAffected()
	if err != nil {
		return
	}
	if n == 0 {
		err = database.ErrNonceReused
		return
	}

	return
}
