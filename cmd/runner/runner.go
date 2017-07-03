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
	"time"

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

type timing struct {
	loading time.Duration
	running time.Duration
	overall time.Duration
}

func init() {
	log.SetFlags(0)
	echo.Default.Log = log.New(os.Stderr, "echo service: ", 0)
}

var (
	executor      string
	loader        string
	loaderSymbols string

	stackSize = 16 * 1024 * 1024
	dumpTime  = false
	dumpText  = false
	dumpStack = false
	repeat    = 1
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
	flag.BoolVar(&dumpTime, "dump-time", dumpTime, "print average timings per program")
	flag.BoolVar(&dumpText, "dump-text", dumpText, "disassemble before running")
	flag.BoolVar(&dumpStack, "dump-stack", dumpStack, "print stacktrace after running")
	flag.IntVar(&repeat, "repeat", repeat, "repeat the program execution(s) multiple times")
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

	timings := make([]timing, len(args))

	for round := 0; round < repeat; round++ {
		done := make(chan struct{}, len(args))

		for i, arg := range args {
			var r run.ServiceRegistry

			if i == 0 {
				r = service.Defaults
			} else {
				r = origin.CloneRegistryWith(service.Defaults, nil, os.Stdout)
			}

			go execute(arg, r, &timings[i], done)
		}

		for range args {
			<-done
		}
	}

	if dumpTime {
		for i, arg := range args {
			output := func(title string, sum time.Duration) {
				avg := sum / time.Duration(repeat)
				log.Printf("%s "+title+": %6d.%03dµs", arg, avg/time.Microsecond, avg%time.Microsecond)
			}

			output("loading time", timings[i].loading)
			output("running time", timings[i].running)
			output("overall time", timings[i].overall)
		}
	}
}

func execute(filename string, services run.ServiceRegistry, timing *timing, done chan<- struct{}) {
	defer func() {
		done <- struct{}{}
	}()

	tBegin := time.Now()

	env, err := run.NewEnvironment(executor, loader, loaderSymbols)
	if err != nil {
		log.Fatalf("environment: %v", err)
	}

	tLoadBegin := time.Now()

	var ns sections.NameSection

	m := wag.Module{
		MainSymbol:           "main",
		UnknownSectionLoader: sections.UnknownLoaders{"name": ns.Load}.Load,
	}

	err = load(&m, filename, env)
	if err != nil {
		log.Fatalf("module: %v", err)
	}

	tLoadEnd := time.Now()

	_, memorySize := m.MemoryLimits()

	payload, err := run.NewPayload(&m, memorySize, int32(stackSize))
	if err != nil {
		log.Fatalf("payload: %v", err)
	}
	defer payload.Close()

	if dumpText {
		dewag.PrintTo(os.Stderr, m.Text(), m.FunctionMap(), &ns)
	}

	tRunBegin := time.Now()

	exit, trap, err := run.Run(env, payload, services, os.Stderr)
	if err != nil {
		log.Fatal(err)
	}

	tRunEnd := time.Now()
	tEnd := tRunEnd

	if trap != 0 {
		log.Printf("trap: %s", trap)
	} else if exit != 0 {
		log.Printf("exit: %d", exit)
	}

	if dumpStack {
		err := payload.DumpStacktrace(os.Stderr, m.FunctionMap(), m.CallMap(), m.FunctionSignatures(), &ns)
		if err != nil {
			log.Printf("stacktrace: %v", err)
		}
	}

	timing.loading += tLoadEnd.Sub(tLoadBegin)
	timing.running += tRunEnd.Sub(tRunBegin)
	timing.overall += tEnd.Sub(tBegin)
}

func load(m *wag.Module, filename string, env *run.Environment) (err error) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	err = m.Load(bufio.NewReader(f), env, new(bytes.Buffer), nil, run.RODataAddr, nil)
	return
}
