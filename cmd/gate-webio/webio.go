// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	chanBufSize = 100
)

type client struct {
	r2w chan []byte
	w2r chan []byte
}

var (
	clientLock sync.Mutex
	clientMap  = make(map[string]*client)
)

func getClient(key string) (c *client) {
	clientLock.Lock()
	defer clientLock.Unlock()

	c = clientMap[key]
	if c == nil {
		c = &client{
			make(chan []byte, chanBufSize),
			make(chan []byte, chanBufSize),
		}
		clientMap[key] = c
	}
	return
}

func main() {
	var (
		addr = ":80"
		dir  = "."
	)

	flag.StringVar(&addr, "addr", addr, "[host]:port")
	flag.StringVar(&dir, "dir", dir, ".")
	flag.Parse()

	http.HandleFunc("/io/work/nonblock", handleWorkNonblock)
	http.HandleFunc("/io/work", handleWork)
	http.HandleFunc("/io/run", handleRun)
	http.Handle("/", http.FileServer(http.Dir(dir)))

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleRunWebSocket(w, r)

	default:
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleWork(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleWorkGet(w, r)

	case http.MethodPost:
		handleWorkPost(w, r)

	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleWorkNonblock(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleWorkGetNonblock(w, r)

	default:
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleRunWebSocket(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("%s: %v", r.URL, err)
		return
	}
	defer conn.Close()

	client := getClient(r.URL.RawQuery)

	read := make(chan struct{})
	writ := make(chan struct{})

	go func() {
		defer close(read)

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}

			client.r2w <- data
		}
	}()

	go func() {
		defer close(writ)

		for data := range client.w2r {
			if conn.WriteMessage(websocket.BinaryMessage, data) != nil {
				return
			}
		}
	}()

	<-read
	<-writ
}

func handleWorkGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	c := getClient(r.URL.RawQuery).r2w
	timer := time.NewTimer(40 * time.Second)

	select {
	case data := <-c:
		timer.Stop()

		if _, err := w.Write(data); err != nil {
			log.Printf("%s: %v", r.URL, err)
			return
		}

		for {
			select {
			case data := <-c:
				if _, err := w.Write(data); err != nil {
					log.Printf("%s: %v", r.URL, err)
					return
				}

			default:
				return
			}
		}

	case <-timer.C:
	}
}

func handleWorkGetNonblock(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	c := getClient(r.URL.RawQuery).r2w

	for {
		select {
		case data := <-c:
			if _, err := w.Write(data); err != nil {
				log.Printf("%s: %v", r.URL, err)
				return
			}

		default:
			return
		}
	}
}

func handleWorkPost(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("%s: %v", r.URL, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	getClient(r.URL.RawQuery).w2r <- data

	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusOK)
}
