// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/chzyer/readline"
	"github.com/gorilla/websocket"
	"github.com/tsavola/gate/webapi"
)

type REPLConfig struct {
	HistoryFile  string
	HistoryLimit int
}

func remoteREPL(uri string, params url.Values) (ok bool, err error) {
	conn, _, err := new(websocket.Dialer).Dial(makeWebsocketURL(uri, params), nil)
	if err != nil {
		return
	}
	defer conn.Close()

	err = conn.WriteJSON(webapi.IO{Authorization: makeAuthorization()})
	if err != nil {
		return
	}

	res := new(webapi.IOConnection)
	err = conn.ReadJSON(res)
	if err != nil {
		return
	}
	if !res.Connected {
		err = errors.New("connection rejected")
		return
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:       "> ",
		HistoryFile:  c.REPL.HistoryFile,
		HistoryLimit: c.REPL.HistoryLimit,
	})
	if err != nil {
		return
	}
	defer rl.Close()

	var closemsg []byte
	defer func() {
		if len(closemsg) > 0 {
			conn.WriteMessage(websocket.CloseMessage, closemsg)
		}
	}()

	data := []byte("\r\r\r\r\r\r\r\n")

	for {
		err = conn.WriteMessage(websocket.BinaryMessage, data)
		if err != nil {
			return
		}

		for {
			var typ int

			typ, data, err = conn.ReadMessage()
			if err != nil {
				closemsg = websocket.FormatCloseMessage(websocket.CloseProtocolError, fmt.Sprintf("read: %v", err))
				return
			}

			switch typ {
			case websocket.BinaryMessage:
				os.Stdout.Write(data)

			case websocket.TextMessage:
				var res webapi.ConnectionStatus

				err = json.Unmarshal(data, &res)
				if err != nil {
					closemsg = websocket.FormatCloseMessage(websocket.ClosePolicyViolation, fmt.Sprintf("connection status message: %v", err))
					return
				}

				closemsg = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")

				if res.Status.Error != "" {
					err = fmt.Errorf("instance error: %s", res.Status.Error)
					return
				}

				switch res.Status.State {
				case webapi.StateHalted, webapi.StateTerminated:
					ok = (res.Status.Result == 0)

				default:
					ok = false
				}

				if res.Status.Cause == "" {
					err = fmt.Errorf("instance %s", res.Status.State)
				} else {
					cause := strings.Replace(res.Status.Cause, "_", " ", -1)
					err = fmt.Errorf("instance %s: %s", res.Status.State, cause)
				}
				return
			}

			if len(data) == 0 || data[len(data)-1] == '\n' {
				break
			}
		}

		var line string

		line, err = rl.Readline()
		if err != nil {
			if err == io.EOF {
				ok = true
				err = nil
			}
			closemsg = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
			return
		}

		data = []byte(line + "\n")
	}
}
