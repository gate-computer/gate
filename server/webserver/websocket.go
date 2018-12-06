// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"errors"
	"io"
	"net/http"

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
	websocketInternalServerErr      = websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "")
)

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

type websocketWriter struct {
	conn *websocket.Conn
}

func newWebsocketWriter(conn *websocket.Conn) *websocketWriter {
	return &websocketWriter{conn}
}

func (*websocketWriter) SetHeader(key, value string) {}

func (w *websocketWriter) Write(buf []byte) (n int, err error) {
	err = w.conn.WriteMessage(websocket.BinaryMessage, buf)
	if err == nil {
		n = len(buf)
	}
	return
}

func (w *websocketWriter) WriteError(httpStatus int, text string) {
	code := websocket.CloseInternalServerErr
	if httpStatus >= 400 && httpStatus < 500 { // Client error
		code = websocket.ClosePolicyViolation
	}
	w.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, text))
}

type websocketReader struct {
	conn  *websocket.Conn
	frame io.Reader
}

func newWebsocketReader(conn *websocket.Conn) *websocketReader {
	return &websocketReader{conn: conn}
}

func (r *websocketReader) Read(buf []byte) (n int, err error) {
	for {
		if r.frame == nil {
			_, r.frame, err = r.conn.NextReader()
			if err != nil {
				return
			}
		}

		n, err = r.frame.Read(buf)
		if err == io.EOF {
			r.frame = nil
			err = nil
		}

		if n != 0 || err != nil {
			return
		}
	}
}

type websocketReadCanceler struct {
	reader websocketReader
	cancel context.CancelFunc
}

func newWebsocketReadCanceler(conn *websocket.Conn, cancel context.CancelFunc) *websocketReadCanceler {
	return &websocketReadCanceler{
		reader: websocketReader{conn: conn},
		cancel: cancel,
	}
}

func (r *websocketReadCanceler) Read(buf []byte) (n int, err error) {
	n, err = r.reader.Read(buf)
	if err != nil {
		r.cancel()
	}
	return
}
