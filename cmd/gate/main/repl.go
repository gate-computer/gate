// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"io"

	"github.com/chzyer/readline"
	"github.com/gorilla/websocket"

	. "import.name/pan/check"
)

type REPLConfig struct {
	HistoryFile  string
	HistoryLimit int
}

func repl(r io.Reader, w io.Writer) {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:       "> ",
		HistoryFile:  c.REPL.HistoryFile,
		HistoryLimit: c.REPL.HistoryLimit,
	})
	Check(err)
	defer rl.Close()

	readErr := make(chan error, 1)
	go func() {
		defer close(readErr)
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				if _, e := rl.Write(b[:n]); e != nil {
					readErr <- e
					break
				}
			}
			if err != nil {
				if err != io.EOF {
					readErr <- err
				}
				break
			}
		}
	}()

	outbuf := []byte("\r\r\r\r\r\r\r\n")

	for {
		check_(w.Write(outbuf))

		line, err := rl.ReadSlice()
		if err != nil {
			if err == io.EOF {
				break
			}
			Check(err)
		}

		outbuf = append(line, '\n')
	}

	Check(<-readErr)
}

type websocketConn struct {
	ws *websocket.Conn
	r  io.Reader
}

func (conn *websocketConn) Read(b []byte) (int, error) {
	for {
		for conn.r == nil {
			t, r, err := conn.ws.NextReader()
			if err != nil {
				return 0, err
			}
			if t == websocket.BinaryMessage {
				conn.r = r
			}
		}

		n, err := conn.r.Read(b)
		if err == io.EOF {
			conn.r = nil
		}
		if n != 0 || err != nil {
			return n, err
		}
	}
}

func (conn *websocketConn) Write(b []byte) (n int, err error) {
	if len(b) > 0 {
		err = conn.ws.WriteMessage(websocket.BinaryMessage, b)
		if err == nil {
			n = len(b)
		}
	}
	return
}

func replWebsocket(conn *websocket.Conn) {
	defer conn.Close()
	defer conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))

	c := &websocketConn{ws: conn}
	repl(c, c)
}
