// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"context"

	"gate.computer/gate/server/event"
	"gate.computer/internal/error/subsystem"
)

func reportInternalError(ctx context.Context, s *webserver, sourceURI, progHash, function, instID string, err error) {
	s.monitorFail(ctx, event.TypeFailInternal, &event.Fail{
		Source:    sourceURI,
		Module:    progHash,
		Function:  function,
		Instance:  instID,
		Subsystem: subsystem.Get(err),
	}, err)
}

func reportNetworkError(ctx context.Context, s *webserver, err error) {
	s.monitorError(ctx, event.TypeFailNetwork, err)
}

func reportProtocolError(ctx context.Context, s *webserver, err error) {
	s.monitorError(ctx, event.TypeFailProtocol, err)
}

func reportRequestError(ctx context.Context, s *webserver, failType event.FailType, sourceURI, progHash, function, instID string, err error) {
	s.monitorFail(ctx, event.TypeFailRequest, &event.Fail{
		Type:     failType,
		Source:   sourceURI,
		Module:   progHash,
		Function: function,
		Instance: instID,
	}, err)
}

func reportRequestFailure(ctx context.Context, s *webserver, failType event.FailType) {
	s.monitorFail(ctx, event.TypeFailRequest, &event.Fail{
		Type: failType,
	}, nil)
}

func reportPayloadError(ctx context.Context, s *webserver, err error) {
	s.monitorFail(ctx, event.TypeFailRequest, &event.Fail{
		Type: event.FailPayloadError,
	}, err)
}
