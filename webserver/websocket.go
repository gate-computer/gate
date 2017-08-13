package webserver

import (
	"io"

	"github.com/gorilla/websocket"
)

var (
	websocketNormalClosure        = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	websocketUnsupportedData      = websocket.FormatCloseMessage(websocket.CloseUnsupportedData, "")
	websocketAlreadyCommunicating = websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Already communicating")
	websocketInternalServerErr    = websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "")
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
