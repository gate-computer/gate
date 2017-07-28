package server

import (
	"io"

	"github.com/gorilla/websocket"
)

var (
	websocketNormalClosure     = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	websocketUnsupportedData   = websocket.FormatCloseMessage(websocket.CloseUnsupportedData, "")
	websocketAlreadyAttached   = websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Already attached")
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

type websocketWriteCloser struct {
	websocketWriter
	close chan<- struct{}
}

func newWebsocketWriteCloser(conn *websocket.Conn, close chan<- struct{}) *websocketWriteCloser {
	return &websocketWriteCloser{
		websocketWriter{conn},
		close,
	}
}

func (w *websocketWriteCloser) Close() (err error) {
	close(w.close)
	return
}
