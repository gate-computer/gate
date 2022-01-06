// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"context"

	"gate.computer/gate/internal/error/subsystem"
	server "gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
)

func reportInternalError(ctx context.Context, s *webserver, sourceURI, progHash, function, instID string, err error) {
	var subsys string
	if x, ok := err.(subsystem.Error); ok {
		subsys = x.Subsystem()
	}

	s.Monitor(&event.FailInternal{
		Ctx:       server.ContextDetail(ctx),
		Source:    sourceURI,
		Module:    progHash,
		Function:  function,
		Instance:  instID,
		Subsystem: subsys,
	}, err)
}

func reportNetworkError(ctx context.Context, s *webserver, err error) {
	s.Monitor(&event.FailNetwork{
		Ctx: server.ContextDetail(ctx),
	}, err)
}

func reportProtocolError(ctx context.Context, s *webserver, err error) {
	s.Monitor(&event.FailProtocol{
		Ctx: server.ContextDetail(ctx),
	}, err)
}

func reportRequestError(ctx context.Context, s *webserver, failType event.FailRequest_Type, sourceURI, progHash, function, instID string, err error) {
	s.Monitor(&event.FailRequest{
		Ctx:      server.ContextDetail(ctx),
		Failure:  failType,
		Source:   sourceURI,
		Module:   progHash,
		Function: function,
		Instance: instID,
	}, err)
}

func reportRequestFailure(ctx context.Context, s *webserver, failType event.FailRequest_Type) {
	s.Monitor(&event.FailRequest{
		Ctx:     server.ContextDetail(ctx),
		Failure: failType,
	}, nil)
}

func reportPayloadError(ctx context.Context, s *webserver, err error) {
	s.Monitor(&event.FailRequest{
		Ctx:     server.ContextDetail(ctx),
		Failure: event.FailPayloadError,
	}, err)
}
