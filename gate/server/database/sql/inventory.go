// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sql

import (
	. "import.name/type/context"
)

const InventorySchema = `
CREATE TABLE IF NOT EXISTS module_source (
	module TEXT NOT NULL,
	source TEXT NOT NULL,

	PRIMARY KEY (source)
);
`

func (x *Endpoint) InitInventory(ctx Context) error {
	_, err := x.db.ExecContext(ctx, InventorySchema)
	return err
}

func (x *Endpoint) GetSourceModule(ctx Context, source string) (string, error) {
	conn, err := x.db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	var module string

	row := conn.QueryRowContext(ctx, "SELECT module FROM module_source WHERE source = $1", source)
	if err := row.Scan(&module); err != nil {
		return "", err
	}

	return module, nil
}

func (x *Endpoint) AddModuleSource(ctx Context, module, source string) error {
	conn, err := x.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, "INSERT INTO module_source (module, source) VALUES ($1, $2) ON CONFLICT (source) DO UPDATE SET module = $1", module, source)
	return err
}
