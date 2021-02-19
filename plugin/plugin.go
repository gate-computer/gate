// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"

	"gate.computer/gate/service"
	"gate.computer/localhost"
)

var config localhost.Config

func ServiceConfig() interface{} {
	return &config
}

func InitServices(ctx context.Context, r *service.Registry) error {
	l, err := localhost.New(&config)
	if err != nil {
		return err
	}

	return r.Register(l)
}

func main() {}
