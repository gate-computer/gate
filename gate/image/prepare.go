// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"

	"gate.computer/internal/file"

	. "import.name/type/context"
)

// PreparePrograms wraps a ProgramStorage.  The wrapper creates program
// resources in advance.
func PreparePrograms(ctx Context, storage ProgramStorage, bufsize int) ProgramStorage {
	c := make(chan fileResult, bufsize-1)
	go preparePrograms(ctx, c, storage)
	return &preparedPrograms{storage, c}
}

type preparedPrograms struct {
	ProgramStorage
	c <-chan fileResult
}

func (pp *preparedPrograms) newProgramFile() (*file.File, error) {
	r, ok := <-pp.c
	if !ok {
		return nil, context.Canceled // TODO: actual error
	}

	return r.file, r.err
}

func preparePrograms(ctx Context, c chan fileResult, storage ProgramStorage) {
	defer func() {
		close(c)
		for r := range c {
			if r.err == nil {
				r.file.Close()
			}
		}
	}()

	for {
		f, err := storage.newProgramFile()

		select {
		case c <- fileResult{f, err}:

		case <-ctx.Done():
			if err == nil {
				f.Close()
			}
			return
		}
	}
}

// PrepareInstances wraps an InstanceStorage.  The wrapper creates instance
// resources in advance.
func PrepareInstances(ctx Context, storage InstanceStorage, bufsize int) InstanceStorage {
	if bufsize <= 0 {
		bufsize = 1
	}
	c := make(chan fileResult, bufsize-1)
	go prepareInstances(ctx, c, storage)
	return &preparedInstances{storage, c}
}

type preparedInstances struct {
	InstanceStorage
	c <-chan fileResult
}

func (pi *preparedInstances) newInstanceFile() (*file.File, error) {
	r, ok := <-pi.c
	if !ok {
		return nil, context.Canceled // TODO: actual error
	}

	return r.file, r.err
}

func prepareInstances(ctx Context, c chan fileResult, storage InstanceStorage) {
	defer func() {
		close(c)
		for r := range c {
			if r.err == nil {
				r.file.Close()
			}
		}
	}()

	for {
		f, err := storage.newInstanceFile()

		select {
		case c <- fileResult{f, err}:

		case <-ctx.Done():
			if err == nil {
				f.Close()
			}
			return
		}
	}
}

type fileResult struct {
	file *file.File
	err  error
}
