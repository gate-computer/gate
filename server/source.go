// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"
)

// Close the stream unless it has been appropriated.
func (opts *ModuleUpload) Close() error {
	c := opts.takeStream()
	if c == nil {
		return nil
	}
	return c.Close()
}

// Source of immutable data.
type Source interface {
	// OpenURI for reading an object.  If the object's size exceeds maxSize,
	// the object is not to be opened.  The reader is not necessarily drained,
	// but it will be closed.  The reader must produce exactly contentLength's
	// worth of bytes when read in full.
	//
	// Not-found condition can be signaled by returning nil content with zero
	// length.  Content-too-long condition can be signaled by returning nil
	// content with nonzero length (doesn't have to be actual content length).
	OpenURI(
		ctx context.Context,
		uri string,
		maxSize int,
	) (
		content io.ReadCloser,
		contentLength int64,
		err error,
	)
}

// ModuleSource is like a Source, but the uri parameter is a struct member.
type ModuleSource struct {
	Source Source
	URI    string
}

// Open is like Source.OpenURI, but the uri argument is implicit.
func (x *ModuleSource) Open(ctx context.Context, maxSize int) (io.ReadCloser, int64, error) {
	return x.Source.OpenURI(ctx, x.URI, maxSize)
}
