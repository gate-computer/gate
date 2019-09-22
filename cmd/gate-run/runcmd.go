// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"context"
	"debug/dwarf"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"

	"github.com/tsavola/confi"
	"github.com/tsavola/gate/entry"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/principal"
	"github.com/tsavola/gate/internal/system"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/catalog"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/service/plugin"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/object/debug"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/object/stack/stacktrace"
	"github.com/tsavola/wag/section"
	"github.com/tsavola/wag/trap"
	"github.com/tsavola/wag/wa"
)

const (
	DefaultMaxProcesses = 100
	DefaultStackSize    = wa.PageSize
)

type ProgramConfig struct {
	StackSize int
}

type timing struct {
	loading time.Duration
	running time.Duration
	overall time.Duration
}

var processPolicy = runtime.ProcessPolicy{
	TimeResolution: 1, // Best resolution.
	Debug:          os.Stderr,
}

func init() {
	log.SetFlags(0)
}

type Config struct {
	Runtime runtime.Config

	Principal struct {
		ID string
	}

	Scope struct {
		System bool
	}

	Plugin struct {
		LibDir string
	}

	Service map[string]interface{}

	Program ProgramConfig

	Function string

	Benchmark struct {
		Repeat int
		Timing bool
	}

	Dump string
}

var c = new(Config)

func parseConfig(flags *flag.FlagSet) {
	flags.Var(confi.FileReader(c), "f", "read TOML configuration file")
	flags.Var(confi.Assigner(c), "c", "set a configuration key (path.to.key=value)")
	flags.Parse(os.Args[1:])
}

func main() {
	c.Runtime.MaxProcesses = DefaultMaxProcesses
	c.Runtime.Cgroup.Title = runtime.DefaultCgroupTitle
	c.Program.StackSize = DefaultStackSize
	c.Benchmark.Repeat = 1

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	parseConfig(flags)

	suspend := make(chan struct{})
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGQUIT)
	go func() {
		<-signals
		close(suspend)
		for range signals {
		}
	}()

	if c.Runtime.LibDir == "" || c.Plugin.LibDir == "" {
		filename, err := os.Executable()
		if err != nil {
			log.Fatalf("%s: %v", os.Args[0], err)
		}
		bindir := path.Dir(filename)
		libdir := path.Join(bindir, "..", "lib", "gate")
		if c.Runtime.LibDir == "" {
			c.Runtime.LibDir = path.Join(libdir, "runtime")
		}
		if c.Plugin.LibDir == "" {
			c.Plugin.LibDir = path.Join(libdir, "plugin")
		}
	}

	plugins, err := plugin.OpenAll(c.Plugin.LibDir)
	if err != nil {
		log.Fatal(err)
	}

	c.Service = plugins.ServiceConfig

	originConfig := origin.Config{MaxConns: 1}
	c.Service["origin"] = &originConfig

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] wasmfile...\n\nOptions:\n", flag.CommandLine.Name())
		flag.PrintDefaults()
	}
	flag.Usage = confi.FlagUsage(nil, c)
	parseConfig(flag.CommandLine)

	filenames := flag.Args()
	if len(filenames) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	principalID, err := principal.ParseID(c.Principal.ID)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	ctx = principal.ContextWithID(ctx, principalID)
	if c.Scope.System {
		ctx = system.ContextWithUserID(ctx, strconv.Itoa(os.Getuid()))
	}

	serviceRegistry := new(service.Registry)

	if err := plugins.InitServices(serviceRegistry); err != nil {
		log.Fatal(err)
	}

	var execClosed bool

	executor, err := runtime.NewExecutor(c.Runtime)
	if err != nil {
		log.Fatalf("runtime: %v", err)
	}
	defer func() {
		execClosed = true
		executor.Close()
	}()

	go func() {
		<-executor.Dead()
		if !execClosed {
			log.Fatal("executor died")
		}
	}()

	timings := make([]timing, len(filenames))
	exitCode := 0

	for round := 0; round < c.Benchmark.Repeat; round++ {
		var (
			execDone = make(chan int, len(filenames))
			ioDone   = make(chan struct{}, len(filenames))
		)

		for i, filename := range filenames {
			i := i
			filename := filename

			connector := origin.New(originConfig)

			var input io.Reader = os.Stdin
			if i > 0 {
				input = bytes.NewReader(nil)
			}

			go func() {
				defer func() { ioDone <- struct{}{} }()

				conn := connector.Connect(ctx)
				if conn == nil {
					return
				}

				if err := conn(ctx, input, os.Stdout); err != nil {
					log.Print(err)
				}
			}()

			r := serviceRegistry.Clone()
			r.Register(connector)
			r.Register(catalog.New(r))

			go func() {
				defer connector.Close()
				execute(ctx, executor, filename, r, &timings[i], suspend, execDone)
			}()
		}

		for range filenames {
			if n := <-execDone; n > exitCode {
				exitCode = n
			}
			<-ioDone
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

func execute(ctx context.Context, executor *runtime.Executor, filename string, services runtime.ServiceRegistry, timing *timing, suspend <-chan struct{}, done chan<- int) {
	var exit int

	defer func() {
		done <- exit
	}()

	tBegin := time.Now()

	proc, err := executor.NewProcess(ctx)
	if err != nil {
		log.Fatalf("process: %v", err)
	}
	defer proc.Kill()

	tLoadBegin := tBegin

	var im debug.InsnMap
	var ns = new(section.NameSection)
	var cs = new(section.CustomSections)

	funcSigs, prog, inst, err := load(filename, &im, ns, cs)
	if err != nil {
		log.Fatalf("load: %v", err)
	}
	defer prog.Close()
	defer inst.Close()

	tLoadEnd := time.Now()
	tRunBegin := tLoadEnd

	err = proc.Start(prog, inst, processPolicy)
	if err != nil {
		log.Fatalf("execute: %v", err)
	}

	go func() {
		select {
		case <-suspend:
			proc.Suspend()

		case <-ctx.Done():
			return
		}
	}()

	var buffers snapshot.Buffers

	exit, trapID, err := proc.Serve(ctx, services, &buffers)

	tRunEnd := time.Now()
	tEnd := tRunEnd

	switch {
	case err != nil:
		defer os.Exit(1)
		log.Printf("serve: %v", err)

	case trapID != 0:
		log.Printf("%v", trapID)
		if trapID == trap.Suspended {
			if !dump(prog, inst, buffers, true) {
				exit = 4
			}
		} else {
			exit = 3
		}

	default:
		if exit != 0 {
			log.Printf("exit: %d", exit)
		}
	}

	timing.loading += tLoadEnd.Sub(tLoadBegin)
	timing.running += tRunEnd.Sub(tRunBegin)
	timing.overall += tEnd.Sub(tBegin)

	var trace []stack.Frame

	if trapID != 0 || err != nil {
		trace, err = inst.Stacktrace(im, funcSigs)
		if err != nil {
			log.Fatalf("stacktrace: %v", err)
		}
	}

	debugInfo, err := newDWARF(cs.Sections)
	if err != nil {
		log.Printf("dwarf: %v", err) // Not fatal
	}

	if len(trace) > 0 {
		stacktrace.Fprint(os.Stderr, trace, funcSigs, ns, debugInfo)
	}
}

func load(filename string, codeMap *debug.InsnMap, ns *section.NameSection, cs *section.CustomSections,
) (funcSigs []wa.FuncType, prog *image.Program, inst *image.Instance, err error) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return
	}

	build, err := image.NewBuild(image.Memory, int(info.Size()), compile.DefaultMaxTextSize, &codeMap.CallMap, true)
	if err != nil {
		return
	}
	defer build.Close()

	r := codeMap.Reader(bufio.NewReader(io.TeeReader(f, build.ModuleWriter())))

	var loadConfig = compile.Config{
		CustomSectionLoader: section.CustomLoaders{
			".debug_abbrev":   cs.Load,
			".debug_info":     cs.Load,
			".debug_line":     cs.Load,
			".debug_pubnames": cs.Load,
			".debug_ranges":   cs.Load,
			".debug_str":      cs.Load,
			"name":            ns.Load,
		}.Load,
	}

	mod, err := compile.LoadInitialSections(&compile.ModuleConfig{Config: loadConfig}, r)
	if err != nil {
		return
	}

	err = binding.BindImports(&mod, build.ImportResolver())
	if err != nil {
		return
	}

	text := build.TextBuffer()

	var codeConfig = &compile.CodeConfig{
		Text:   text,
		Mapper: codeMap,
		Config: loadConfig,
	}

	err = compile.LoadCodeSection(codeConfig, r, mod)
	if err != nil {
		return
	}

	// textCopy := make([]byte, len(text.Bytes()))
	// copy(textCopy, text.Bytes())

	var entryIndex uint32
	var entryAddr uint32

	if c.Function != "" {
		entryIndex, err = entry.ModuleFuncIndex(mod, c.Function)
		if err != nil {
			return
		}

		entryAddr = codeMap.FuncAddrs[entryIndex]
	}

	err = build.FinishText(c.Program.StackSize, 0, mod.GlobalsSize(), mod.InitialMemorySize(), mod.MemorySizeLimit())
	if err != nil {
		return
	}

	var dataConfig = &compile.DataConfig{
		GlobalsMemory:   build.GlobalsMemoryBuffer(),
		MemoryAlignment: build.MemoryAlignment(),
		Config:          loadConfig,
	}

	err = compile.LoadDataSection(dataConfig, r, mod)
	if err != nil {
		return
	}

	// if f, err := os.Create("/tmp/datadump.txt"); err != nil {
	// 	log.Fatal(err)
	// } else {
	// 	defer f.Close()
	// 	if _, err := f.Write(dataConfig.GlobalsMemory.Bytes()); err != nil {
	// 		log.Fatal(err)
	// 	}
	// }

	err = compile.LoadCustomSections(&loadConfig, r)
	if err != nil {
		return
	}

	// if f, err := os.Create("/tmp/textdump.txt"); err != nil {
	// 	log.Fatal(err)
	// } else {
	// 	defer f.Close()
	// 	if err := dump.Text(f, textCopy, 0, codeMap.FuncAddrs, ns); err != nil {
	// 		log.Fatal(err)
	// 	}
	// }

	prog, err = build.FinishProgram(image.SectionMap{}, nil, nil, nil)
	if err != nil {
		return
	}

	inst, err = build.FinishInstance(entryIndex, entryAddr)
	if err != nil {
		return
	}

	funcSigs = mod.FuncTypes()
	return
}

func dump(prog *image.Program, inst *image.Instance, buffers snapshot.Buffers, suspended bool) (ok bool) {
	if c.Dump == "" {
		return
	}

	prog2, err := image.Snapshot(prog, inst, buffers, suspended)
	if err != nil {
		log.Printf("snapshot: %v", err)
		return
	}
	defer prog2.Close()

	f, err := os.Create(c.Dump)
	if err != nil {
		log.Print(err)
		return
	}
	defer func() {
		if f != nil {
			f.Close()
		}
		if err != nil {
			os.Remove(c.Dump)
		}
	}()

	_, err = io.Copy(f, prog2.NewModuleReader())
	if err != nil {
		log.Printf("dump: %v", err)
		return
	}

	err = f.Close()
	f = nil
	if err != nil {
		log.Print(err)
		return
	}

	ok = true
	log.Printf("snapshot: %s", c.Dump)
	return
}

func newDWARF(sections map[string][]byte) (data *dwarf.Data, err error) {
	var (
		abbrev   = sections[".debug_abbrev"]
		info     = sections[".debug_info"]
		line     = sections[".debug_line"]
		pubnames = sections[".debug_pubnames"]
		ranges   = sections[".debug_ranges"]
		str      = sections[".debug_str"]
	)

	if info != nil {
		data, err = dwarf.New(abbrev, nil, nil, info, line, pubnames, ranges, str)
	}
	return
}
