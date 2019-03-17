// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"context"
	"errors"
)

var errProcessChanClosed = errors.New("process preparation loop terminated")

type ProcessErr struct {
	Proc *Process
	Err  error
}

type ProcessChan <-chan ProcessErr

// PrepareProcesses in advance.
func PrepareProcesses(ctx context.Context, f ProcessFactory, bufsize int) ProcessChan {
	c := make(chan ProcessErr, bufsize-1)

	go func() {
		defer func() {
			close(c)
			for x := range c {
				if x.Err == nil {
					x.Proc.Kill()
				}
			}
		}()

		for {
			p, err := f.NewProcess(ctx)

			select {
			case c <- ProcessErr{p, err}:

			case <-ctx.Done():
				if err == nil {
					p.Kill()
				}
				return
			}
		}
	}()

	return ProcessChan(c)
}

func (c ProcessChan) NewProcess(ctx context.Context) (proc *Process, err error) {
	select {
	case x, ok := <-c:
		if !ok {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()

			default:
				return nil, errProcessChanClosed
			}
		}

		return x.Proc, x.Err

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
