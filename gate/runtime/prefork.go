// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"errors"

	. "import.name/type/context"
)

var errProcessChanClosed = errors.New("process preparation loop terminated")

type ResultProcess struct {
	Process *Process
	Err     error
}

type ProcessChan <-chan ResultProcess

// PrepareProcesses in advance.
func PrepareProcesses(ctx Context, f ProcessFactory, bufsize int) ProcessChan {
	c := make(chan ResultProcess, bufsize-1)

	go func() {
		defer func() {
			close(c)
			for x := range c {
				if x.Err == nil {
					x.Process.Kill()
				}
			}
		}()

		for {
			p, err := f.NewProcess(ctx)

			select {
			case c <- ResultProcess{p, err}:

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

func (c ProcessChan) NewProcess(ctx Context) (*Process, error) {
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

		return x.Process, x.Err

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
