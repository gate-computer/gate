// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

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
	"strconv"
	"strings"

	dbus "github.com/godbus/dbus/v5"
	"github.com/tsavola/gate/runtime/abi"
	api "github.com/tsavola/gate/serverapi"
	"github.com/tsavola/gate/webapi"
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
	objectdebug "github.com/tsavola/wag/object/debug"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/object/stack/stacktrace"
	"github.com/tsavola/wag/section"
	"github.com/tsavola/wag/wa"
)

func debug(call func(instID string, req api.DebugRequest) api.DebugResponse) {
	var req api.DebugRequest

	if flag.NArg() > 1 {
		switch flag.Arg(1) {
		case "break":
			req.Op = api.DebugOpConfigUnion
			req.Config.DebugInfo = true

		case "delete":
			req.Op = api.DebugOpConfigComplement

		case "detach":
			req.Op = api.DebugOpConfigSet
			if flag.NArg() > 2 {
				log.Fatal("detach command does not support offset")
			}

		case "bt", "backtrace":
			req.Op = api.DebugOpReadStack
			if flag.NArg() > 2 {
				log.Fatal("stacktrace command does not support offset")
			}

		default:
			log.Fatalf("unknown debug op: %s", flag.Arg(1))
		}

		if flag.NArg() > 2 {
			offset, err := strconv.ParseUint(flag.Arg(2), 0, 64)
			check(err)
			req.Config.Breakpoints.Offsets = []uint64{offset}
		}
	}

	res := call(flag.Arg(0), req)

	switch flag.Arg(1) {
	case "bt", "backtrace":
		debugBacktrace(res)

	default:
		fmt.Printf("Status:         %s\n", statusString(res.Status))
		fmt.Printf("Debug info:     %v\n", res.Config.DebugInfo)
		fmt.Printf("Breakpoints:")
		sep := "    "
		for _, offset := range res.Config.Breakpoints.Offsets {
			fmt.Printf("%s0x%x", sep, offset)
			sep = " "
		}
		fmt.Println()
	}
}

func debugBacktrace(res api.DebugResponse) {
	if len(res.Data) == 0 {
		log.Fatal("no stack")
	}

	moduleSpec := strings.SplitN(res.Module, "/", 2)
	if len(moduleSpec) != 2 || moduleSpec[0] != webapi.ModuleRefSource {
		log.Fatal("unsupported module specification:", res.Module)
	}

	r, w, err := os.Pipe()
	check(err)

	wFD := dbus.UnixFD(w.Fd())
	call := daemonCall("Download", wFD, moduleSpec[1])
	closeFiles(w)

	var moduleLen int64
	check(call.Store(&moduleLen))

	var reader = bufio.NewReader(r)
	var names section.NameSection
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

	mod, err := compile.LoadInitialSections(&compile.ModuleConfig{Config: config}, reader)
	check(err)

	err = binding.BindImports(&mod, new(abi.ImportResolver))
	if err != nil {
		return
	}

	var codeReader = objectdebug.NewReadTeller(reader)
	var codeMap objectdebug.InsnMap
	var codeConfig = &compile.CodeConfig{
		Mapper:      codeMap.Mapper(codeReader),
		Breakpoints: make(map[uint32]compile.Breakpoint),
		Config:      config,
	}

	for _, offset := range res.Config.Breakpoints.Offsets {
		codeConfig.Breakpoints[uint32(offset)] = compile.Breakpoint{}
	}

	err = compile.LoadCodeSection(codeConfig, codeReader, mod, abi.Library())
	if err != nil {
		return
	}

	frames := traceStack(res.Data, codeMap, mod.FuncTypes())

	_, err = section.CopyStandardSection(ioutil.Discard, reader, section.Data, config.CustomSectionLoader)
	if err == nil {
		err = compile.LoadCustomSections(&config, reader)
	}
	if err != nil && err != io.EOF {
		log.Print(err)
	}

	var (
		abbrev    = custom.Sections[".debug_abbrev"]
		info      = custom.Sections[".debug_info"]
		line      = custom.Sections[".debug_line"]
		pubnames  = custom.Sections[".debug_pubnames"]
		ranges    = custom.Sections[".debug_ranges"]
		str       = custom.Sections[".debug_str"]
		debugInfo *dwarf.Data
	)
	if info != nil {
		debugInfo, err = dwarf.New(abbrev, nil, nil, info, line, pubnames, ranges, str)
		if err != nil {
			log.Print(err)
		}
	}

	check(stacktrace.Fprint(os.Stdout, frames, mod.FuncTypes(), &names, debugInfo))
}

func traceStack(buf []byte, textMap objectdebug.InsnMap, funcTypes []wa.FuncType,
) (frames []stack.Frame) {
	if n := len(buf); n == 0 || n&7 != 0 {
		panic(fmt.Errorf("invalid stack size %d", n))
	}

	for len(buf) > 0 {
		pair := binary.LittleEndian.Uint64(buf)

		callIndex := uint32(pair)
		if callIndex >= uint32(len(textMap.CallSites)) {
			panic(fmt.Errorf("function call site index %d is unknown", callIndex))
		}
		call := textMap.CallSites[callIndex]

		if off := int32(pair >> 32); off != call.StackOffset {
			panic(fmt.Errorf("encoded stack offset %d of call site %d does not match offset %d in map", off, callIndex, call.StackOffset))
		}

		if len(textMap.FuncAddrs) == 0 || call.RetAddr < textMap.FuncAddrs[0] {
			return
		}

		if call.StackOffset&7 != 0 {
			panic(fmt.Errorf("invalid stack offset %d", call.StackOffset))
		}
		if call.StackOffset == 0 {
			panic(errors.New("inconsistent call stack"))
		}

		init, funcIndex, callIndexAgain, stackOffset, retOff := textMap.FindCall(call.RetAddr)
		if init || callIndexAgain != int(callIndex) || stackOffset != call.StackOffset {
			panic(fmt.Errorf("call instruction not found for return address 0x%x", call.RetAddr))
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

	panic(errors.New("ran out of stack before initial call"))
}
