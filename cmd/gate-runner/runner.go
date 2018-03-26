// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/tsavola/config"
	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/service"
	_ "github.com/tsavola/gate/service/defaults"
	"github.com/tsavola/gate/service/echo"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/dewag"
	"github.com/tsavola/wag/sections"
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

type Config struct {
	Runtime run.Config

	Program struct {
		StackSize int32
		Arg       int32

		Dump struct {
			Text  bool
			Stack bool
		}
	}

	Origin struct {
		Net  string
		Addr string
	}

	Benchmark struct {
		Repeat int
		Timing bool
	}
}

var c = new(Config)

func main() {
	c.Runtime.MaxProcs = 100
	c.Runtime.LibDir = "lib"
	c.Runtime.CgroupTitle = run.DefaultCgroupTitle
	c.Program.StackSize = 65536
	c.Benchmark.Repeat = 1

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] wasm...\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Usage = config.FlagUsage(c)

	flag.Var(config.FileReader(c), "f", "read YAML configuration file")
	flag.Var(config.Assigner(c), "c", "set a configuration key (path.to.key=value)")
	flag.Parse()

	filenames := flag.Args()
	if len(filenames) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()

	originalDefaultOriginReader := origin.Default.R

	if c.Origin.Addr != "" {
		if c.Origin.Net == "unix" {
			os.Remove(c.Origin.Addr)
		}

		l, err := net.Listen(c.Origin.Net, c.Origin.Addr)
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

	var rtClosed bool

	rt, err := run.NewRuntime(ctx, &c.Runtime)
	if err != nil {
		log.Fatalf("runtime: %v", err)
	}
	defer func() {
		rtClosed = true
		rt.Close()
	}()

	go func() {
		<-rt.Done()
		if !rtClosed {
			log.Fatal("executor died")
		}
	}()

	timings := make([]timing, len(filenames))
	exitCode := 0

	for round := 0; round < c.Benchmark.Repeat; round++ {
		done := make(chan int, len(filenames))

		for i, filename := range filenames {
			r := service.Defaults

			if i > 0 {
				r = r.Clone()
				origin.New(originalDefaultOriginReader, os.Stdout).Register(r)
			}

			go execute(ctx, rt, filename, c.Program.Arg, r, &timings[i], done)
		}

		for range filenames {
			if n := <-done; n > exitCode {
				exitCode = n
			}
		}
	}

	if c.Benchmark.Timing {
		for i, filename := range filenames {
			output := func(title string, sum time.Duration) {
				avg := sum / time.Duration(c.Benchmark.Repeat)
				log.Printf("%s "+title+": %6d.%03dµs", filename, avg/time.Microsecond, avg%time.Microsecond)
			}

			output("loading time", timings[i].loading)
			output("running time", timings[i].running)
			output("overall time", timings[i].overall)
		}
	}

	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func execute(ctx context.Context, rt *run.Runtime, filename string, arg int32, services run.ServiceRegistry, timing *timing, done chan<- int) {
	exit := 0

	defer func() {
		done <- exit
	}()

	tBegin := time.Now()

	var (
		image run.Image
		proc  run.Process
	)

	err := run.InitImageAndProcess(ctx, rt, &image, &proc, os.Stderr)
	if err != nil {
		log.Fatalf("instance: %v", err)
	}
	defer image.Release(rt)
	defer proc.Kill(rt)

	tLoadBegin := tBegin

	var ns sections.NameSection

	m := wag.Module{
		UnknownSectionLoader: sections.UnknownLoaders{"name": ns.Load}.Load,
	}

	err = load(&m, filename, rt)
	if err != nil {
		log.Fatalf("module: %v", err)
	}

	tLoadEnd := time.Now()

	_, memorySize := m.MemoryLimits()

	err = image.Populate(&m, memorySize, c.Program.StackSize)
	if err != nil {
		log.Fatalf("image: %v", err)
	}

	image.SetArg(arg)

	if c.Program.Dump.Text {
		dewag.PrintTo(os.Stderr, m.Text(), m.FunctionMap(), &ns)
	}

	tRunBegin := time.Now()

	exit, trap, err := run.Run(ctx, rt, &proc, &image, services)
	if err != nil {
		log.Fatalf("run: %v", err)
	}

	tRunEnd := time.Now()
	tEnd := tRunEnd

	if trap != 0 {
		log.Printf("trap: %s", trap)
		exit = 3
	} else if exit != 0 {
		log.Printf("exit: %d", exit)
	}

	if c.Program.Dump.Stack {
		err := image.DumpStacktrace(os.Stderr, &m, &ns)
		if err != nil {
			log.Printf("stacktrace: %v", err)
		}
	}

	timing.loading += tLoadEnd.Sub(tLoadBegin)
	timing.running += tRunEnd.Sub(tRunBegin)
	timing.overall += tEnd.Sub(tBegin)
}

func load(m *wag.Module, filename string, rt *run.Runtime) (err error) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	err = run.Load(m, bufio.NewReader(f), rt, new(bytes.Buffer), nil, nil)
	return
}
