// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"io"
)

// Source of immutable data.
type Source interface {
	// CanonicalURI converts a source URI to its canonical form.  The result
	// should be byte-wise identical to all other canonicalized URIs which
	// refer to the same location.
	//
	// CanonicalURI is called with an absolute URI which doesn't contain
	// successive slashes.  It starts with the source name (e.g. "/foo/...").
	//
	// If the URI is know to be invalid, an error should be returned.
	CanonicalURI(uri string) (string, error)

	// OpenURI for reading an object.  The argument is a URI returned by
	// CanonicalizeURI.
	//
	// If the object's size exceeds maxSize, the object is not to be opened.
	// The reader is not necessarily drained, but it will be closed.  The
	// reader must produce exactly contentLength's worth of bytes when read in
	// full.
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
