// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package debug contains functionality useful for accessing the instance
// debugging API via HTTP.
//
// This package may have more dependencies than the parent package.
package debug

import (
	"io"

	server "gate.computer/gate/server/api"
	"gate.computer/gate/server/web/internal/protojson"
)

// Request content for api.ActionDebug.  It can be serialized using
// MustMarshalRequest.
type Request = server.DebugRequest

// Response content for api.ActionDebug.  It can be deserialized using
// DecodeResponse.
type Response = server.DebugResponse

// MustMarshalRequest to JSON content.
func MustMarshalRequest(req *Request) []byte {
	return protojson.MustMarshal(req)
}

// DecodeResponse from JSON content.
func DecodeResponse(r io.Reader) (*Response, error) {
	res := new(Response)
	if err := protojson.Decode(r, res); err != nil {
		return nil, err
	}
	return res, nil
}
