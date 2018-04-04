// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"errors"
	"io"

	"github.com/gorilla/websocket"
)

var (
	errWrongWebsocketMessageType = errors.New("wrong websocket message type")
)

var (
	websocketNormalClosure     = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	websocketUnsupportedData   = websocket.FormatCloseMessage(websocket.CloseUnsupportedData, "")
	websocketIOAlreadyAttached = websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "I/O origin already attached")
	websocketInternalServerErr = websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "")
)

type websocketReader struct {
	conn  *websocket.Conn
	frame io.Reader
}

func newWebsocketReader(conn *websocket.Conn) *websocketReader {
	return &websocketReader{
		conn: conn,
	}
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

func newWebsocketReadCanceler(conn *websocket.Conn, cancel context.CancelFunc,
) *websocketReadCanceler {
	return &websocketReadCanceler{
		reader: websocketReader{
			conn: conn,
		},
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

type websocketWriter struct {
	conn *websocket.Conn
}

func (w websocketWriter) Write(buf []byte) (n int, err error) {
	err = w.conn.WriteMessage(websocket.BinaryMessage, buf)
	if err == nil {
		n = len(buf)
	}
	return
}
