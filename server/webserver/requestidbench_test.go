// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"runtime"
	"strconv"
	"testing"
)

func BenchmarkRequestIDChannel(b *testing.B) {
	b.Skip("manually disabled")

	maxprocs := runtime.GOMAXPROCS(0)

	for bufsize := 0; bufsize < 1024; bufsize += 8 {
		bufsize := bufsize

		b.Run(strconv.Itoa(bufsize), func(b *testing.B) {
			requestIDs := newRequestIDChannel(bufsize)
			done := make(chan struct{})

			b.ResetTimer()

			for i := 0; i < maxprocs; i++ {
				go func() {
					for i := 0; i < b.N; i++ {
						<-requestIDs
					}
					done <- struct{}{}
				}()
			}

			for i := 0; i < maxprocs; i++ {
				<-done
			}
		})
	}
}
