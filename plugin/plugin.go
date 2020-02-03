// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"gate.computer/localhost"
	"github.com/tsavola/gate/service"
)

func ServiceConfig() interface{} {
	return localhost.ServiceConfig()
}

func InitServices(r *service.Registry) error {
	return localhost.InitServices(r)
}

func main() {}
