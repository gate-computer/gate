// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"gate.computer/gate/server/event"
	"gate.computer/internal/error/subsystem"

	. "import.name/type/context"
)

func reportInternalError(ctx Context, s *webserver, sourceURI, progHash, function, instID string, err error) {
	s.eventFail(ctx, event.TypeFailInternal, &event.Fail{
		Source:    sourceURI,
		Module:    progHash,
		Function:  function,
		Instance:  instID,
		Subsystem: subsystem.Get(err),
	}, err)
}

func reportNetworkError(ctx Context, s *webserver, err error) {
	s.event(ctx, event.TypeFailNetwork, err)
}

func reportProtocolError(ctx Context, s *webserver, err error) {
	s.event(ctx, event.TypeFailProtocol, err)
}

func reportRequestError(ctx Context, s *webserver, failType event.FailType, sourceURI, progHash, function, instID string, err error) {
	s.eventFail(ctx, event.TypeFailRequest, &event.Fail{
		Type:     failType,
		Source:   sourceURI,
		Module:   progHash,
		Function: function,
		Instance: instID,
	}, err)
}

func reportRequestFailure(ctx Context, s *webserver, failType event.FailType) {
	s.eventFail(ctx, event.TypeFailRequest, &event.Fail{
		Type: failType,
	}, nil)
}

func reportPayloadError(ctx Context, s *webserver, err error) {
	s.eventFail(ctx, event.TypeFailRequest, &event.Fail{
		Type: event.FailPayloadError,
	}, err)
}
