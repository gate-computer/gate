// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"context"
	"errors"
)

var errFileLimiterClosed = errors.New("file limiter closed")

type FileLimiter struct {
	get1 chan struct{}
	get4 chan struct{}
	get5 chan struct{}
	get6 chan struct{}
	get7 chan struct{}
	put  chan struct{}
}

func NewFileLimiter(maxFiles int) (limiter *FileLimiter) {
	limiter = new(FileLimiter)

	if maxFiles > 0 {
		limiter.get1 = make(chan struct{})
		limiter.get4 = make(chan struct{})
		limiter.get5 = make(chan struct{})
		limiter.get6 = make(chan struct{})
		limiter.get7 = make(chan struct{})
		limiter.put = make(chan struct{}, (1<<31)-1)

		go limiter.loop(maxFiles)
	}

	return
}

func (limiter *FileLimiter) Close() (err error) {
	if limiter.put != nil {
		close(limiter.put)
	}
	return
}

func (limiter *FileLimiter) loop(numFiles int) {
	defer func() {
		close(limiter.get1)
		close(limiter.get4)
		close(limiter.get5)
		close(limiter.get6)
		close(limiter.get7)
	}()

	for {
		var (
			get1 chan<- struct{}
			get4 chan<- struct{}
			get5 chan<- struct{}
			get6 chan<- struct{}
			get7 chan<- struct{}
		)

		switch {
		case numFiles >= 7:
			get7 = limiter.get7
			fallthrough

		case numFiles >= 6:
			get6 = limiter.get6
			fallthrough

		case numFiles >= 5:
			get5 = limiter.get5
			fallthrough

		case numFiles >= 4:
			get4 = limiter.get4
			fallthrough

		case numFiles >= 1:
			get1 = limiter.get1
		}

		select {
		case get1 <- struct{}{}:
			numFiles -= 1

		case get4 <- struct{}{}:
			numFiles -= 4

		case get5 <- struct{}{}:
			numFiles -= 5

		case get6 <- struct{}{}:
			numFiles -= 6

		case get7 <- struct{}{}:
			numFiles -= 7

		case _, ok := <-limiter.put:
			if !ok {
				return
			}

			numFiles++
		}
	}
}

func (limiter FileLimiter) acquire(ctx context.Context, num int) (err error) {
	var get <-chan struct{}

	switch num {
	case 1: // Image.Init, dialContainerDaemon
		get = limiter.get1

	case 4: // Process.Init without debug
		get = limiter.get4

	case 5: // InitImageAndProcess without debug
		get = limiter.get5

	case 6: // Process.Init with debug
		get = limiter.get6

	case 7: // InitImageAndProcess with debug, startContainer
		get = limiter.get7

	default:
		panic(num)
	}

	if get != nil {
		select {
		case _, ok := <-get:
			if !ok {
				err = errFileLimiterClosed
			}

		case <-ctx.Done():
			err = ctx.Err()
		}
	}

	return
}

func (limiter FileLimiter) release(num int) {
	if limiter.put != nil {
		for i := 0; i < num; i++ {
			limiter.put <- struct{}{}
		}
	}
}
