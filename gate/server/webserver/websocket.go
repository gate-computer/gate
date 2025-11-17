// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"gate.computer/gate/web"
	"github.com/gorilla/websocket"
)

var (
	errWrongWebsocketMessageType       = errors.New("wrong websocket message type")
	errUnsupportedWebsocketContent     = errors.New("content type present in websocket message")
	errUnsupportedWebsocketContentType = errors.New("unsupported content type")
)

var (
	websocketNormalClosure          = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	websocketUnsupportedData        = websocket.FormatCloseMessage(websocket.CloseUnsupportedData, "")
	websocketUnsupportedContent     = websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "content not supported by action")
	websocketUnsupportedContentType = websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "unsupported content type")
	websocketNotConnected           = websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "not connected")
)

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

var connectionStatusInputMessage = mustMarshalJSON(web.ConnectionStatus{
	Status: web.Status{State: web.StateRunning},
	Input:  true,
})

type websocketResponseWriter struct {
	conn *websocket.Conn
}

func (w websocketResponseWriter) SetHeader(key, value string) {}

func (w websocketResponseWriter) Write(buf []byte) (int, error) {
	if err := w.conn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
		return 0, err
	}
	return len(buf), nil
}

func (w websocketResponseWriter) WriteError(httpStatus int, text string) {
	code := websocket.CloseInternalServerErr
	if httpStatus >= 400 && httpStatus < 500 { // Client error.
		code = websocket.ClosePolicyViolation
	}
	w.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, text))
}

type websocketReadWriter struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex // Guards writing and writeErr when inputHint is zero.
	writeErr  error
	readFrame io.Reader
	inputHint uint32 // Atomic.  If nonzero, the reader API has ceased writing.
}

func newWebsocketReadWriter(conn *websocket.Conn) *websocketReadWriter {
	return &websocketReadWriter{conn: conn}
}

func (rw *websocketReadWriter) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}

	if atomic.LoadUint32(&rw.inputHint) == 0 {
		rw.writeInputHint()
	}

	for {
		if rw.readFrame == nil {
			_, frame, err := rw.conn.NextReader()
			if err != nil {
				return 0, err
			}
			rw.readFrame = frame
		}

		n, err := rw.readFrame.Read(buf)
		if err != nil {
			if err != io.EOF {
				return n, err
			}
			rw.readFrame = nil
		}

		if n != 0 {
			return n, nil
		}
	}
}

func (rw *websocketReadWriter) writeInputHint() {
	rw.writeMu.Lock()
	defer rw.writeMu.Unlock()

	if rw.writeErr == nil {
		rw.writeErr = rw.conn.WriteMessage(websocket.TextMessage, connectionStatusInputMessage)
	}

	atomic.StoreUint32(&rw.inputHint, 1)
}

func (rw *websocketReadWriter) CloseRead() error {
	if atomic.LoadUint32(&rw.inputHint) == 0 {
		rw.writeMu.Lock()
		defer rw.writeMu.Unlock()

		atomic.StoreUint32(&rw.inputHint, 1)
	}

	return nil
}

func (rw *websocketReadWriter) Write(buf []byte) (int, error) {
	if atomic.LoadUint32(&rw.inputHint) == 0 {
		rw.writeMu.Lock()
		defer rw.writeMu.Unlock()
	}

	if rw.writeErr != nil {
		return 0, rw.writeErr
	}

	if err := rw.conn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
		rw.writeErr = err
		return 0, err
	}
	return len(buf), nil
}

func (rw *websocketReadWriter) Close() error {
	return nil
}

type websocketReadWriteCanceler struct {
	websocketReadWriter
	cancel func()
}

func newWebsocketReadWriteCanceler(conn *websocket.Conn, cancel func()) *websocketReadWriteCanceler {
	return &websocketReadWriteCanceler{
		websocketReadWriter{conn: conn},
		cancel,
	}
}

func (crw *websocketReadWriteCanceler) Read(buf []byte) (int, error) {
	n, err := crw.websocketReadWriter.Read(buf)
	if err != nil {
		crw.cancel()
	}
	return n, err
}

func (crw *websocketReadWriteCanceler) CloseRead() error {
	err := crw.websocketReadWriter.CloseRead()
	if err != nil {
		crw.cancel()
	}
	return err
}

func (crw *websocketReadWriteCanceler) Write(buf []byte) (int, error) {
	n, err := crw.websocketReadWriter.Write(buf)
	if err != nil {
		crw.cancel()
	}
	return n, err
}
