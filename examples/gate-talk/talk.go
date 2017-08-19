// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/chzyer/readline"
	"github.com/gorilla/websocket"
	"github.com/tsavola/gate/webapi"
)

const (
	websocketStarting = iota
	websocketWaiting
)

type websocketEvent struct {
	webapi.Running
	webapi.Result
}

func main() {
	if !mainResult() {
		os.Exit(1)
	}
}

func mainResult() (ok bool) {
	log.SetFlags(0)

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	wasm := path.Join(dir, "examples/gate-talk/payload/prog.wasm")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] url\n\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.StringVar(&wasm, "wasm", wasm, "WebAssembly module filename")
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	url := flag.Arg(0)

	wasmData, err := ioutil.ReadFile(wasm)
	if err != nil {
		return
	}

	hash := sha512.New()
	hash.Write(wasmData)
	wasmHash := hex.EncodeToString(hash.Sum(nil))

	var d websocket.Dialer

	conn, _, err := d.Dial(url+"/run", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	err = conn.WriteJSON(webapi.Run{ProgramSHA512: wasmHash})
	if err != nil {
		log.Fatal(err)
	}

	err = conn.WriteMessage(websocket.BinaryMessage, wasmData)
	if err != nil {
		log.Fatal(err)
	}

	rl, err := readline.New("> ")
	if err != nil {
		log.Fatal(err)
	}
	defer rl.Close()

	log.SetOutput(rl.Stderr())

	exit := make(chan bool, 1)
	go readLoop(conn, rl, exit)

	for {
		var data []byte

		line, err := rl.Readline()
		if err == nil {
			data = append([]byte{'<', ' '}, line...)
		} else {
			log.SetOutput(os.Stderr)
			log.Printf("readline: %v", err)
			data = []byte{0}
		}

		err = conn.WriteMessage(websocket.BinaryMessage, data)
		if err != nil {
			log.Print(err)
			return
		}

		if data[0] == 0 {
			ok = <-exit
			return
		}
	}
}

func readLoop(conn *websocket.Conn, rl *readline.Instance, exit chan<- bool) {
	defer rl.Close()

	ok := false
	defer func() { exit <- ok }()

	state := websocketStarting

	for {
		typ, data, err := conn.ReadMessage()
		if err != nil {
			log.Print(err)
			return
		}

		switch typ {
		case websocket.TextMessage:
			var x websocketEvent

			err := json.Unmarshal(data, &x)
			if err == nil {
				switch state {
				case websocketStarting:
					log.Printf("payload running: program %s, instance %s", x.ProgramId, x.InstanceId)
					state = websocketWaiting

				case websocketWaiting:
					switch {
					case x.TrapId != 0:
						log.Printf("payload trap: %s (%d)", x.Trap, x.TrapId)

					case x.ExitStatus == nil:
						log.Print("payload result is invalid")

					case *x.ExitStatus == 0:
						ok = true
						fallthrough
					default:
						log.Printf("payload exit: %d", *x.ExitStatus)
					}
					return
				}
			} else {
				log.Print(err)
				return
			}

		case websocket.BinaryMessage:
			log.Printf("%s", data)
		}
	}
}
