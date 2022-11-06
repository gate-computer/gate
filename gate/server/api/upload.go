// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"io"
)

// ModuleUpload parameters.  Server may take possession of Stream; Close must
// be called in case it remains non-nil.
type ModuleUpload struct {
	Stream io.ReadCloser
	Length int64
	Hash   string
}

// TakeStream removes Stream from ModuleUpload.
func (opt *ModuleUpload) TakeStream() io.ReadCloser {
	r := opt.Stream
	opt.Stream = nil
	return r
}

// Close the stream unless it has been appropriated.
func (opts *ModuleUpload) Close() error {
	c := opts.TakeStream()
	if c == nil {
		return nil
	}
	return c.Close()
}
