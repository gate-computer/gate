// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"

	"gate.computer/gate/service"
	"gate.computer/localhost"
)

func ServiceConfig() interface{} {
	return localhost.ServiceConfig()
}

func InitServices(ctx context.Context, r *service.Registry) error {
	return localhost.InitServices(ctx, r)
}

func main() {}
