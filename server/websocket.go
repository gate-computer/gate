package server

import (
	"io"

	"github.com/gorilla/websocket"
)

var webSocketClose = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")

type webSocketReader struct {
	conn  *websocket.Conn
	frame io.Reader
}

func newWebSocketReader(conn *websocket.Conn) *webSocketReader {
	return &webSocketReader{
		conn: conn,
	}
}

func (r *webSocketReader) Read(buf []byte) (n int, err error) {
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

type webSocketWriter struct {
	conn *websocket.Conn
}

func (w webSocketWriter) Write(buf []byte) (n int, err error) {
	err = w.conn.WriteMessage(websocket.BinaryMessage, buf)
	if err == nil {
		n = len(buf)
	}
	return
}
