// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"

	"github.com/tsavola/gate/server"
)

func mustDecodeContent(ctx context.Context, wr *requestResponseWriter, s *webserver, pri *server.PrincipalKey) io.ReadCloser {
	var encoding string

	switch fields := wr.request.Header["Content-Encoding"]; len(fields) {
	case 0:
		// identity

	case 1:
		encoding = fields[0]

	default:
		goto bad
	}

	switch encoding {
	case "", "identity":
		return wr.request.Body

	case "gzip":
		decoder, err := gzip.NewReader(wr.request.Body)
		if err != nil {
			respondContentDecodeError(ctx, wr, s, pri, err)
			panic(nil)
		}

		return http.MaxBytesReader(wr.response, decoder, wr.request.ContentLength)
	}

bad:
	wr.response.Header().Set("Accept-Encoding", "gzip")
	respondUnsupportedEncoding(ctx, wr, s, pri)
	panic(nil)
}
