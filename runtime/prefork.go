// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"context"
	"errors"
)

type ProcessFactory <-chan *Process

// PrepareProcesses in advance.
func PrepareProcesses(ctx context.Context, exec *Executor, bufsize int) (ProcessFactory, <-chan error) {
	if bufsize <= 0 {
		bufsize = 1
	}

	procs := make(chan *Process, bufsize-1)
	errs := make(chan error)

	go func() {
		defer func() {
			close(errs)
			close(procs)
			for proc := range procs {
				proc.Kill()
			}
		}()

		for {
			if proc, err := NewProcess(ctx, exec, nil); err == nil {
				select {
				case procs <- proc:

				case <-ctx.Done():
					proc.Kill()
					return
				}
			} else {
				select {
				case errs <- err:

				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ProcessFactory(procs), errs
}

func (channel ProcessFactory) NewProcess(ctx context.Context) (proc *Process, err error) {
	select {
	case proc, ok := <-channel:
		if !ok {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()

			default:
				return nil, errors.New("process factory terminated")
			}
		}

		return proc, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
