// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"net/http"
	"time"
)

var requestIDs = newRequestIDChannel(128)

func newRequestIDChannel(bufsize int) <-chan uint64 {
	c := make(chan uint64, bufsize)

	go func() {
		i := uint64((time.Now().UnixNano())/1e6) * 1e6

		for {
			i++
			c <- i
		}
	}()

	return c
}

func defaultNewRequestID(_ *http.Request) uint64 {
	return <-requestIDs
}
