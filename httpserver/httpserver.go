package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/traps"
	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/run"
)

const (
	memorySizeLimit = 16 * wasm.Page
	stackSize       = 16 * 4096
)

var env *run.Environment

func main() {
	var err error

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	env, err = run.NewEnvironment(path.Join(dir, "bin/executor"), path.Join(dir, "bin/loader"))
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/execute", execute)

	log.Fatal(http.ListenAndServe(":8888", nil))
}

func execute(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	log.Printf("%s begin", r.RemoteAddr)
	defer log.Printf("%s end", r.RemoteAddr)

	wasm := bufio.NewReader(r.Body)

	var m wag.Module

	err := m.Load(wasm, env, nil, nil, run.RODataAddr, nil)
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err)
		return
	}

	_, memorySize := m.MemoryLimits()
	if memorySize > memorySizeLimit {
		memorySize = memorySizeLimit
	}

	payload, err := run.NewPayload(&m, memorySize, stackSize)
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err)
		return
	}

	output, err := run.Run(env, payload)
	if err != nil {
		if trap, ok := err.(traps.Id); ok {
			w.Header().Set("X-Gate-Trap", trap.String())
		} else {
			log.Printf("%s error: %v", r.RemoteAddr, err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, err)
			return
		}
	}

	var data []byte

	if len(output) >= 8 {
		size := binary.LittleEndian.Uint32(output)
		if size >= 8 && size <= uint32(len(output)) {
			data = output[8:size]
		}
	}

	if data == nil {
		log.Printf("%s error: invalid output: %v", r.RemoteAddr, output)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(data); err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
	}
	return
}
