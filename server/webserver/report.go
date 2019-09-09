// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"

	"github.com/tsavola/gate/internal/error/subsystem"
	"github.com/tsavola/gate/principal"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/event"
)

func reportInternalError(ctx context.Context, s *webserver, pri *principal.Key, sourceURI, progHash, function, instID string, err error) {
	var subsys string

	if x, ok := err.(subsystem.Error); ok {
		subsys = x.Subsystem()
	}

	s.Server.Monitor(&event.FailInternal{
		Ctx:       server.Context(ctx, pri),
		Source:    sourceURI,
		Module:    progHash,
		Function:  function,
		Instance:  instID,
		Subsystem: subsys,
	}, err)
}

func reportNetworkError(ctx context.Context, s *webserver, err error) {
	s.Server.Monitor(&event.FailNetwork{
		Ctx: server.Context(ctx, nil),
	}, err)
}

func reportProtocolError(ctx context.Context, s *webserver, pri *principal.Key, err error) {
	s.Server.Monitor(&event.FailProtocol{
		Ctx: server.Context(ctx, pri),
	}, err)
}

func reportRequestError(ctx context.Context, s *webserver, pri *principal.Key, failType event.FailRequest_Type, sourceURI, progHash, function, instID string, err error) {
	s.Server.Monitor(&event.FailRequest{
		Ctx:      server.Context(ctx, pri),
		Failure:  failType,
		Source:   sourceURI,
		Module:   progHash,
		Function: function,
		Instance: instID,
	}, err)
}

func reportRequestFailure(ctx context.Context, s *webserver, pri *principal.Key, failType event.FailRequest_Type) {
	s.Server.Monitor(&event.FailRequest{
		Ctx:     server.Context(ctx, pri),
		Failure: failType,
	}, nil)
}

func reportPayloadError(ctx context.Context, s *webserver, pri *principal.Key, err error) {
	s.Server.Monitor(&event.FailRequest{
		Ctx:     server.Context(ctx, pri),
		Failure: event.FailPayloadError,
	}, err)
}
