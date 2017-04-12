package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/dewag"
	"github.com/tsavola/wag/sections"

	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/service"
	_ "github.com/tsavola/gate/service/defaults"
	"github.com/tsavola/gate/service/echo"
	"github.com/tsavola/gate/service/origin"
)

type readWriteCloser struct {
	io.Reader
	io.WriteCloser
}

func init() {
	echo.Default.Log = log.New(os.Stderr, "echo service: ", 0)
}

var (
	executor      string
	loader        string
	loaderSymbols string

	stackSize = 16 * 1024 * 1024
	dumpText  = false
	dumpStack = false
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	executor = path.Join(dir, "bin/executor")
	loader = path.Join(dir, "bin/loader")
	loaderSymbols = loader + ".symbols"

	var (
		addr string
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] wasm...\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.StringVar(&executor, "executor", executor, "filename")
	flag.StringVar(&loader, "loader", loader, "filename")
	flag.StringVar(&loaderSymbols, "loader-symbols", loaderSymbols, "filename")
	flag.IntVar(&stackSize, "stack-size", stackSize, "stack size")
	flag.BoolVar(&dumpText, "dump-text", dumpText, "disassemble before running")
	flag.BoolVar(&dumpStack, "dump-stack", dumpStack, "print stacktrace after running")
	flag.StringVar(&addr, "addr", addr, "I/O socket path (replaces stdio)")

	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	if addr != "" {
		os.Remove(addr)
		l, err := net.Listen("unix", addr)
		if err != nil {
			log.Fatal(err)
		}
		conn, err := l.Accept()
		l.Close()
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()

		origin.Default.R = conn
		origin.Default.W = conn
	} else {
		origin.Default.R = os.Stdin
		origin.Default.W = os.Stdout
	}

	done := make(chan struct{}, len(args))

	for _, arg := range args {
		go execute(arg, done)
	}

	for range args {
		<-done
	}
}

func execute(filename string, done chan<- struct{}) {
	defer func() {
		done <- struct{}{}
	}()

	wasm, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}

	env, err := run.NewEnvironment(executor, loader, loaderSymbols)
	if err != nil {
		log.Fatal(err)
	}

	var ns sections.NameSection

	m := wag.Module{
		MainSymbol:           "main",
		UnknownSectionLoader: sections.UnknownLoaders{"name": ns.Load}.Load,
	}

	err = m.Load(bufio.NewReader(wasm), env, new(bytes.Buffer), nil, run.RODataAddr, nil)
	if err != nil {
		log.Fatal(err)
	}

	_, memorySize := m.MemoryLimits()

	payload, err := run.NewPayload(&m, memorySize, int32(stackSize))
	if err != nil {
		log.Fatal(err)
	}
	defer payload.Close()

	if dumpText {
		dewag.PrintTo(os.Stderr, m.Text(), m.FunctionMap(), &ns)
	}

	exit, trap, err := run.Run(env, payload, service.Defaults, os.Stderr)
	if err != nil {
		log.Fatal(err)
	} else if trap != 0 {
		log.Printf("trap: %s", trap)
	} else if exit != 0 {
		log.Printf("exit: %d", exit)
	}

	if dumpStack {
		err := payload.DumpStacktrace(os.Stderr, m.FunctionMap(), m.CallMap(), m.FunctionSignatures(), &ns)
		if err != nil {
			log.Print(err)
		}
	}
}
