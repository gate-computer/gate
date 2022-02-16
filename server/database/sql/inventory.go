// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sql

import (
	"context"
)

const InventorySchema = `
`

func (x *Endpoint) InitInventory(ctx context.Context) error {
	_, err := x.db.ExecContext(ctx, InventorySchema)
	return err
}
