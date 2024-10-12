// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sql

import (
	"database/sql"
	"errors"

	"gate.computer/gate/principal"
	"google.golang.org/protobuf/proto"

	. "import.name/type/context"
)

const InventorySchema = `
CREATE TABLE IF NOT EXISTS module (
	principal TEXT NOT NULL,
	module TEXT NOT NULL,
	buf BLOB NOT NULL,

	PRIMARY KEY (principal, module)
) WITHOUT ROWID, STRICT;

CREATE TABLE IF NOT EXISTS instance (
	principal TEXT NOT NULL,
	instance TEXT NOT NULL,
	buf BLOB NOT NULL,

	PRIMARY KEY (principal, instance)
) WITHOUT ROWID, STRICT;
`

func (x *Endpoint) InitInventory(ctx Context) error {
	_, err := x.db.ExecContext(ctx, x.adjustSchema(InventorySchema))
	return err
}

func (x *Endpoint) GetModule(ctx Context, pri principal.ID, module string, buf proto.Message) (bool, error) {
	q := "SELECT buf FROM module WHERE principal = $1 AND module = $2"
	return x.getInventory(ctx, buf, q, pri, module)
}

func (x *Endpoint) PutModule(ctx Context, pri principal.ID, module string, buf proto.Message) error {
	q := "INSERT INTO module (principal, module, buf) VALUES ($1, $2, $3)"
	return x.modifyInventory(ctx, q, pri, module, buf)
}

func (x *Endpoint) UpdateModule(ctx Context, pri principal.ID, module string, buf proto.Message) error {
	q := "UPDATE module SET buf = $3 WHERE principal = $1 AND module = $2"
	return x.modifyInventory(ctx, q, pri, module, buf)
}

func (x *Endpoint) RemoveModule(ctx Context, pri principal.ID, module string) error {
	q := "DELETE FROM module WHERE principal = $1 AND module = $2"
	_, err := x.db.ExecContext(ctx, q, pri, module)
	return err
}

func (x *Endpoint) GetInstance(ctx Context, pri principal.ID, instance string, buf proto.Message) (bool, error) {
	q := "SELECT buf FROM instance WHERE principal = $1 AND instance = $2"
	return x.getInventory(ctx, buf, q, pri, instance)
}

func (x *Endpoint) PutInstance(ctx Context, pri principal.ID, instance string, buf proto.Message) error {
	q := "INSERT INTO instance (principal, instance, buf) VALUES ($1, $2, $3)"
	return x.modifyInventory(ctx, q, pri, instance, buf)
}

func (x *Endpoint) UpdateInstance(ctx Context, pri principal.ID, instance string, buf proto.Message) error {
	q := "UPDATE instance SET buf = $3 WHERE principal = $1 AND instance = $2"
	return x.modifyInventory(ctx, q, pri, instance, buf)
}

func (x *Endpoint) RemoveInstance(ctx Context, pri principal.ID, instance string) error {
	q := "DELETE FROM instance WHERE principal = $1 AND instance = $2"
	_, err := x.db.ExecContext(ctx, q, pri, instance)
	return err
}

func (x *Endpoint) getInventory(ctx Context, msg proto.Message, query string, pri principal.ID, resource string) (bool, error) {
	var buf []byte

	if err := x.db.QueryRowContext(ctx, query, pri.String(), resource).Scan(&buf); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return true, proto.Unmarshal(buf, msg)
}

func (x *Endpoint) modifyInventory(ctx Context, query string, pri principal.ID, resource string, msg proto.Message) error {
	buf, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = x.db.ExecContext(ctx, query, pri.String(), resource, buf)
	return err
}
