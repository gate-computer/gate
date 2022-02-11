// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bufio"
	"debug/dwarf"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gate.computer/gate/runtime/abi"
	"gate.computer/gate/server/api"
	webapi "gate.computer/gate/server/web/api"
	"gate.computer/wag/binding"
	"gate.computer/wag/compile"
	objectdebug "gate.computer/wag/object/debug"
	"gate.computer/wag/object/stack"
	"gate.computer/wag/object/stack/stacktrace"
	"gate.computer/wag/section"
	"gate.computer/wag/wa"
	dbus "github.com/godbus/dbus/v5"
	"import.name/pan"
)

type location struct {
	file string
	line int
}

type lineAddr struct {
	line   int
	addr   uint64
	column int
}

type lineAddrs []lineAddr

func (a lineAddrs) Len() int      { return len(a) }
func (a lineAddrs) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a lineAddrs) Less(i, j int) bool {
	if a[i].line == a[j].line {
		return a[i].addr < a[j].addr
	}
	return a[i].line < a[j].line
}

type debugCallFunc func(instID string, req *api.DebugRequest) *api.DebugResponse

func debug(call debugCallFunc) {
	req := new(api.DebugRequest)

	if flag.NArg() > 1 {
		switch flag.Arg(1) {
		case "break":
			req.Op = api.DebugOpConfigUnion
			if req.Config == nil {
				req.Config = new(api.DebugConfig)
			}

		case "delete":
			req.Op = api.DebugOpConfigComplement

		case "detach":
			req.Op = api.DebugOpConfigSet
			if flag.NArg() > 2 {
				fatal("detach command does not support offsets")
			}

		case "bt", "backtrace":
			req.Op = api.DebugOpReadStack
			if flag.NArg() > 2 {
				fatal("stacktrace command does not support offsets")
			}

		case "dumptext":
			if flag.NArg() > 2 {
				fatal("dumptext command does not support offsets")
			}

		default:
			fatalf("unknown debug op: %s", flag.Arg(1))
		}

		if flag.NArg() > 2 {
			req.Config.Breakpoints = parseBreakpoints(flag.Args()[2:], call, flag.Arg(0))
		}
	}

	res := call(flag.Arg(0), req)

	switch flag.Arg(1) {
	case "bt", "backtrace":
		debugBacktrace(res)

	case "dumptext":
		_, text, codeMap, names, _ := build(res)
		check(dumpText(text, codeMap.FuncAddrs, &names))

	default:
		modkey := res.Module
		if x := strings.SplitN(res.Module, "/", 2); len(x) == 2 && x[0] == api.KnownModuleSource {
			modkey = x[1]
		}
		fmt.Printf("Module:         %s\n", modkey)
		fmt.Printf("Status:         %s\n", statusString(res.Status))
		fmt.Printf("Breakpoints:")
		sep := "    "
		for _, offset := range res.Config.Breakpoints {
			fmt.Printf("%s0x%x", sep, offset)
			sep = " "
		}
		fmt.Println()
	}
}

func parseBreakpoints(args []string, call debugCallFunc, instID string) (breakOffs []uint64) {
	var (
		breakLocs  []location
		breakFuncs []string
	)

	for _, s := range args {
		switch tokens := strings.SplitN(s, ":", 2); len(tokens) {
		case 1:
			var prefix rune
			for _, r := range s {
				prefix = r
				break
			}

			if prefix != 0 {
				if unicode.IsNumber(prefix) {
					if offset, err := strconv.ParseUint(s, 0, 64); err == nil {
						breakOffs = append(breakOffs, offset)
						continue
					}
				} else {
					breakFuncs = append(breakFuncs, s)
					continue
				}
			}

		case 2:
			file := tokens[0]
			line, err := strconv.Atoi(tokens[1])
			if err == nil {
				breakLocs = append(breakLocs, location{file, line})
				continue
			}
		}

		fatalf("invalid breakpoint expression: %q", s)
	}

	if len(breakLocs) == 0 && len(breakFuncs) == 0 {
		return
	}

	_, _, codeMap, _, info := build(call(instID, &api.DebugRequest{Op: api.DebugOpConfigGet}))
	if info == nil {
		fatal("module contains no debug information")
	}

	var (
		locAddrs    = make(map[string]lineAddrs)
		funcEntries []dwarf.Entry
	)

	for r := info.Reader(); ; {
		e, err := r.Next()
		check(err)
		if e == nil {
			break
		}

		switch e.Tag {
		case dwarf.TagCompileUnit:
			if e.Children {
				lr, err := info.LineReader(e)
				check(err)

				if lr != nil {
					for {
						var le dwarf.LineEntry

						if err := lr.Next(&le); err != nil {
							if err == io.EOF {
								break
							}
							check(err)
						}

						locAddrs[le.File.Name] = append(locAddrs[le.File.Name], lineAddr{le.Line, le.Address, le.Column})
					}
				}
			}

		case dwarf.TagSubprogram:
			funcEntries = append(funcEntries, *e)
			r.SkipChildren()

		default:
			r.SkipChildren()
		}
	}

	for _, lines := range locAddrs {
		sort.Sort(lines)
	}

	funcAddrs := make(map[string][]uint64)

	for _, e := range funcEntries {
		x := e.Val(dwarf.AttrLowpc)
		if x == nil {
			continue
		}
		off := asUint64(x)
		if off == 0 {
			continue
		}

		x = e.Val(dwarf.AttrName)
		if x == nil {
			continue
		}
		name := x.(string)
		if name == "" {
			continue
		}

		funcAddrs[name] = append(funcAddrs[name], off)
	}

	for _, br := range breakLocs {
		ok := false

		for file, lines := range locAddrs {
			if strings.HasSuffix(path.Join("/", file), path.Join("/", br.file)) {
				for _, x := range lines {
					if x.line == br.line {
						fmt.Printf("%s:%d:%d: setting breakpoint at offset 0x%x\n", file, x.line, x.column, x.addr)
						breakOffs = append(breakOffs, x.addr)
						ok = true
						break
					}
				}
			}
		}

		if !ok {
			fatalf("%s:%d: source location not found", br.file, br.line)
		}
	}

	var insnOffs []uint32

	for _, insn := range codeMap.Insns {
		if insn.SourceOffset != 0 {
			insnOffs = append(insnOffs, insn.SourceOffset)
		}
	}

	for _, name := range breakFuncs {
		ok := false

		for _, lowOff := range funcAddrs[name] {
			i := sort.Search(len(insnOffs), func(i int) bool { return uint64(insnOffs[i]) >= lowOff })
			if i == len(insnOffs) {
				continue
			}

			off := uint64(insnOffs[i])
			fmt.Printf("%s: setting breakpoint at offset 0x%x\n", name, off)
			breakOffs = append(breakOffs, off)
			ok = true
		}

		if !ok {
			fatalf("%s: function not found", name)
		}
	}

	return
}

func debugBacktrace(res *api.DebugResponse) {
	if len(res.Data) == 0 {
		fatal("no stack")
	}

	mod, _, codeMap, names, info := build(res)
	frames := traceStack(res.Data, codeMap, mod.FuncTypes())
	check(stacktrace.Fprint(os.Stdout, frames, mod.FuncTypes(), &names, info))
}

func build(res *api.DebugResponse) (mod compile.Module, text []byte, codeMap objectdebug.InsnMap, names section.NameSection, debugInfo *dwarf.Data) {
	var modkey string
	if x := strings.SplitN(res.Module, "/", 2); len(x) == 2 && x[0] == webapi.KnownModuleSource {
		modkey = x[1]
	} else {
		fatal("unsupported module specification:", res.Module)
	}

	r, w, err := os.Pipe()
	check(err)

	wFD := dbus.UnixFD(w.Fd())
	call := daemonCall("DownloadModule", wFD, modkey)
	closeFiles(w)

	var moduleLen int64
	check(call.Store(&moduleLen))

	var reader = bufio.NewReader(r)
	var custom section.CustomSections
	var config = compile.Config{
		CustomSectionLoader: section.CustomLoader(map[string]section.CustomContentLoader{
			"name":            names.Load,
			".debug_abbrev":   custom.Load,
			".debug_info":     custom.Load,
			".debug_line":     custom.Load,
			".debug_pubnames": custom.Load,
			".debug_ranges":   custom.Load,
			".debug_str":      custom.Load,
		}),
	}

	mod, err = compile.LoadInitialSections(&compile.ModuleConfig{Config: config}, reader)
	check(err)

	err = binding.BindImports(&mod, new(abi.ImportResolver))
	if err != nil {
		return
	}

	var codeConfig = &compile.CodeConfig{
		Mapper:      &codeMap,
		Breakpoints: make(map[uint32]compile.Breakpoint),
		Config:      config,
	}

	for _, offset := range res.Config.Breakpoints {
		codeConfig.Breakpoints[uint32(offset)] = compile.Breakpoint{}
	}

	err = compile.LoadCodeSection(codeConfig, reader, mod, abi.Library())
	if err != nil {
		return
	}

	text = codeConfig.Text.Bytes()

	_, err = section.CopyStandardSection(ioutil.Discard, reader, section.Data, config.CustomSectionLoader)
	if err == nil {
		err = compile.LoadCustomSections(&config, reader)
	}
	if err != nil && err != io.EOF {
		log.Print(err)
	}

	var (
		abbrev   = custom.Sections[".debug_abbrev"]
		info     = custom.Sections[".debug_info"]
		line     = custom.Sections[".debug_line"]
		pubnames = custom.Sections[".debug_pubnames"]
		ranges   = custom.Sections[".debug_ranges"]
		str      = custom.Sections[".debug_str"]
	)
	if info != nil {
		debugInfo, err = dwarf.New(abbrev, nil, nil, info, line, pubnames, ranges, str)
		if err != nil {
			log.Print(err)
		}
	}

	return
}

func traceStack(buf []byte, textMap objectdebug.InsnMap, funcTypes []wa.FuncType) []stack.Frame {
	if n := len(buf); n == 0 || n&7 != 0 {
		check(fmt.Errorf("invalid stack size %d", n))
	}

	var frames []stack.Frame

	for len(buf) > 0 {
		pair := binary.LittleEndian.Uint64(buf)

		callIndex := uint32(pair)
		if callIndex >= uint32(len(textMap.CallSites)) {
			check(fmt.Errorf("function call site index %d is unknown", callIndex))
		}
		call := textMap.CallSites[callIndex]

		if off := int32(pair >> 32); off != call.StackOffset {
			check(fmt.Errorf("encoded stack offset %d of call site %d does not match offset %d in map", off, callIndex, call.StackOffset))
		}

		if len(textMap.FuncAddrs) == 0 || call.RetAddr < textMap.FuncAddrs[0] {
			return frames
		}

		if call.StackOffset&7 != 0 {
			check(fmt.Errorf("invalid stack offset %d", call.StackOffset))
		}
		if call.StackOffset == 0 {
			check(errors.New("inconsistent call stack"))
		}

		init, funcIndex, callIndexAgain, stackOffset, retOff := textMap.FindCall(call.RetAddr)
		if init || callIndexAgain != int(callIndex) || stackOffset != call.StackOffset {
			check(fmt.Errorf("call instruction not found for return address 0x%x", call.RetAddr))
		}

		numParams := len(funcTypes[funcIndex].Params)
		numOthers := int(stackOffset/8) - 1
		numLocals := numParams + numOthers
		locals := make([]uint64, numLocals)

		for i := 0; i < numParams; i++ {
			locals[i] = binary.LittleEndian.Uint64(buf[(numLocals-i+1)*8:])
		}

		for i := 0; i < numOthers; i++ {
			locals[numParams+i] = binary.LittleEndian.Uint64(buf[(numOthers-i)*8:])
		}

		frames = append(frames, stack.Frame{
			FuncIndex: funcIndex,
			RetOffset: int(retOff),
			Locals:    locals,
		})

		buf = buf[stackOffset:]
	}

	panic(pan.Wrap(errors.New("ran out of stack before initial call")))
}

func asUint64(x interface{}) uint64 {
	switch v := reflect.ValueOf(x); v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(v.Int())
	default:
		return v.Uint()
	}
}
