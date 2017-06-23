package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"

	"github.com/chzyer/readline"
	"github.com/gorilla/websocket"

	"github.com/tsavola/gate"
)

var webSocketClose = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")

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

	wasm := path.Join(dir, "cmd/talk/payload/prog.wasm")

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

	var d websocket.Dialer

	conn, _, err := d.Dial(url, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	err = sendFile(conn, wasm)
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

	for {
		typ, data, err := conn.ReadMessage()
		if err != nil {
			log.Print(err)
			return
		}

		switch typ {
		case websocket.TextMessage:
			var r gate.Result

			err := json.Unmarshal(data, &r)
			if err == nil {
				switch {
				case r.Error != "":
					log.Printf("payload error: %s", r.Error)

				case r.Trap != "":
					log.Printf("payload trap: %s", r.Trap)

				case r.Exit == 0:
					ok = true
					fallthrough
				default:
					log.Printf("payload exit: %d", r.Exit)
				}
			} else {
				log.Print(err)
			}

			return

		case websocket.BinaryMessage:
			log.Printf("%s", data)
		}
	}
}

func sendFile(conn *websocket.Conn, filename string) (err error) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	w, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return
	}
	defer w.Close()

	_, err = io.Copy(w, f)
	return
}
