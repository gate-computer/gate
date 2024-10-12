// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sql

import (
	"database/sql"
	"errors"

	. "import.name/type/context"
)

const SourceSchema = `
CREATE TABLE IF NOT EXISTS source_sha256 (
	source TEXT NOT NULL,
	module TEXT NOT NULL,

	PRIMARY KEY (source)
) WITHOUT ROWID, STRICT;
`

func (x *Endpoint) InitSourceCache(ctx Context) error {
	_, err := x.db.ExecContext(ctx, x.adjustSchema(SourceSchema))
	return err
}

func (x *Endpoint) GetSourceSHA256(ctx Context, source string) (string, error) {
	var module string

	q := "SELECT module FROM source_sha256 WHERE source = $1"
	if err := x.db.QueryRowContext(ctx, q, source).Scan(&module); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}

	return module, nil
}

func (x *Endpoint) PutSourceSHA256(ctx Context, source, module string) error {
	q := "INSERT INTO source_sha256 (source, module) VALUES ($1, $2) ON CONFLICT DO NOTHING"
	_, err := x.db.ExecContext(ctx, q, source, module)
	return err
}
