// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/gorilla/websocket"
)

func main() {
	var (
		url      = "ws://localhost:8888"
		wasmName = "tests/calc/prog.wasm"
	)

	flag.StringVar(&url, "url", url, "WebSocket address")
	flag.StringVar(&wasmName, "wasm", wasmName, "WebAssembly binary module")
	flag.Parse()

	if flag.NArg() != 0 {
		flag.PrintDefaults()
		os.Exit(2)
	}

	wasmFile, err := os.Open(wasmName)
	if err != nil {
		log.Fatal(err)
	}

	var d websocket.Dialer

	conn, _, err := d.Dial(url, nil)
	if err != nil {
		log.Fatal(err)
	}

	w, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		log.Fatal(err)
	}

	if _, err := io.Copy(w, wasmFile); err != nil {
		log.Fatal(err)
	}

	wasmFile.Close()

	if err := w.Close(); err != nil {
		log.Fatal(err)
	}

	go outputLoop(conn)
	inputLoop(conn)
}

func inputLoop(conn *websocket.Conn) {
	buf := make([]byte, 32768)

	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Fatal(err)
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
			log.Fatal(err)
		}
	}
}

func outputLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Fatal(err)
		}

		if _, err := fmt.Printf("%#v\n", data); err != nil {
			log.Fatal(err)
		}
	}
}
