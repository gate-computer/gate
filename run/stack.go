// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/meta"
	"github.com/tsavola/wag/section"
)

type callSite struct {
	index       uint64
	stackOffset int
}

func findCaller(funcMap []meta.TextAddr, retAddr uint32) (num int, funcAddr uint32, initial, ok bool) {
	if len(funcMap) == 0 {
		return
	}

	firstFuncAddr := funcMap[0]
	if retAddr > 0 && retAddr < uint32(firstFuncAddr) {
		initial = true
		ok = true
		return
	}

	num = sort.Search(len(funcMap), func(i int) bool {
		i++
		if i == len(funcMap) {
			return true
		} else {
			return retAddr <= uint32(funcMap[i])
		}
	})

	if num < len(funcMap) {
		ok = true
	}
	return
}

func getCallSites(callMap []meta.CallSite) (callSites map[int]callSite) {
	callSites = make(map[int]callSite)

	for i, site := range callMap {
		callSites[int(site.ReturnAddr)] = callSite{
			uint64(i),
			int(site.StackOffset),
		}
	}

	return
}

func writeStacktraceTo(w io.Writer, textAddr uint64, stack []byte, m *compile.Module, funcAddrs []meta.TextAddr, callSiteList []meta.CallSite, ns *section.NameSection,
) (err error) {
	funcSigs := m.FuncSigs()

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

	callSites := getCallSites(callSiteList)

	depth := 1

	for ; len(stack) > 0; depth++ {
		absoluteRetAddr := endian.Uint64(stack[:8])

		retAddr := absoluteRetAddr - textAddr
		if retAddr > 0x7ffffffe {
			fmt.Fprintf(w, "#%d  <absolute return address 0x%x is not in text section>\n", depth, absoluteRetAddr)
			return
		}

		funcNum, funcAddr, start, ok := findCaller(funcAddrs, uint32(retAddr))
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

		if ns != nil && funcNum < len(ns.FuncNames) {
			name = ns.FuncNames[funcNum].FunName
		} else {
			name = fmt.Sprintf("func-%d", funcNum)
		}

		if !strings.Contains(name, "(") && funcNum < len(funcSigs) {
			name += funcSigs[funcNum].String() // TODO: parameter names
		}

		fmt.Fprintf(w, "#%d  %s  +0x%x\n", depth, name, uint32(retAddr)-funcAddr)

		stack = stack[site.stackOffset:]
	}

	if len(stack) != 0 {
		fmt.Fprintf(w, "#%d  <%d bytes of untraced stack>\n", depth, len(stack))
	}
	return
}
