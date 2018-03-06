// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/ianlancetaylor/demangle"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/sections"
)

type callSite struct {
	index       uint64
	stackOffset int
}

func findCaller(funcMap []byte, retAddr uint32) (num int, funcAddr uint32, initial, ok bool) {
	count := len(funcMap) / 4
	if count == 0 {
		return
	}

	firstFuncAddr := endian.Uint32(funcMap[:4])
	if retAddr > 0 && retAddr < firstFuncAddr {
		initial = true
		ok = true
		return
	}

	num = sort.Search(count, func(i int) bool {
		i++
		if i == count {
			return true
		} else {
			return retAddr <= endian.Uint32(funcMap[i*4:(i+1)*4])
		}
	})

	if num < count {
		funcAddr = endian.Uint32(funcMap[num*4 : (num+1)*4])
		ok = true
	}
	return
}

func getCallSites(callMap []byte) (callSites map[int]callSite) {
	callSites = make(map[int]callSite)

	for i := 0; len(callMap) > 0; i++ {
		entry := endian.Uint64(callMap[:8])
		callMap = callMap[8:]

		addr := int(uint32(entry))
		stackOffset := int(entry >> 32)

		callSites[addr] = callSite{uint64(i), stackOffset}
	}

	return
}

func writeStacktraceTo(w io.Writer, textAddr uint64, stack []byte, m *wag.Module, ns *sections.NameSection,
) (err error) {
	funcMap := m.FunctionMap()
	callMap := m.CallMap()
	funcSigs := m.FunctionSignatures()

	stack = stack[signalStackReserve:]

	unused := endian.Uint64(stack)
	if unused == 0 {
		err = errors.New("no stack")
		return
	}
	if unused > uint64(len(stack)) || (unused&7) != 0 {
		err = errors.New("corrupted stack")
		return
	}
	stack = stack[unused:]

	callSites := getCallSites(callMap)

	depth := 1

	for ; len(stack) > 0; depth++ {
		absoluteRetAddr := endian.Uint64(stack[:8])

		retAddr := absoluteRetAddr - textAddr
		if retAddr > 0x7ffffffe {
			fmt.Fprintf(w, "#%d  <absolute return address 0x%x is not in text section>\n", depth, absoluteRetAddr)
			return
		}

		funcNum, funcAddr, start, ok := findCaller(funcMap, uint32(retAddr))
		if !ok {
			fmt.Fprintf(w, "#%d  <function not found for return address 0x%x>\n", depth, retAddr)
			return
		}

		site, found := callSites[int(retAddr)]
		if !found {
			fmt.Fprintf(w, "#%d  <unknown return address 0x%x>\n", depth, retAddr)
			return
		}

		if start {
			if site.stackOffset != 0 {
				fmt.Fprintf(w, "#%d  <start function call site stack offset is not zero>\n", depth)
			}
			if len(stack) != 8 {
				fmt.Fprintf(w, "#%d  <start function return address is not stored at start of stack>\n", depth)
			}
			return
		}

		if site.stackOffset < 8 || (site.stackOffset&7) != 0 {
			fmt.Fprintf(w, "#%d  <invalid stack offset %d>\n", depth, site.stackOffset)
			return
		}

		var name string
		var localNames []string

		if ns != nil && funcNum < len(ns.FunctionNames) {
			name = ns.FunctionNames[funcNum].FunName
			localNames = ns.FunctionNames[funcNum].LocalNames
		} else {
			name = fmt.Sprintf("func-%d", funcNum)
		}

		prettyName, err := demangle.ToString(name)
		if err != nil {
			prettyName = name
			if funcNum < len(funcSigs) {
				prettyName += funcSigs[funcNum].StringWithNames(localNames)
			}
		}

		fmt.Fprintf(w, "#%d  %s  +0x%x\n", depth, prettyName, uint32(retAddr)-funcAddr)

		stack = stack[site.stackOffset:]
	}

	if len(stack) != 0 {
		fmt.Fprintf(w, "#%d  <%d bytes of untraced stack>\n", depth, len(stack))
	}
	return
}
