// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"

	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/wa"
)

const initStackSize = 24

func putInitStack(portable []byte, startFuncIndex, entryFuncIndex int64) {
	if n := len(portable); n != initStackSize {
		panic(n)
	}

	const callIndex = 0    // Virtual call site at beginning of enter routine.
	const stackOffset = 16 // The function address are on the stack.

	binary.LittleEndian.PutUint64(portable[0:], stackOffset<<32|callIndex)
	binary.LittleEndian.PutUint64(portable[8:], uint64(startFuncIndex))
	binary.LittleEndian.PutUint64(portable[16:], uint64(entryFuncIndex))
}

// exportStack from native source buffer to portable target buffer.
func exportStack(portable, native []byte, textAddr uint64, codeMap object.CallMap) (err error) {
	if n := len(native); n == 0 || n&7 != 0 {
		err = fmt.Errorf("invalid stack size %d", n)
		return
	}
	if n := len(portable); n != len(native) {
		panic(n)
	}

	var level int
	if false {
		log.Printf("exportStack: textAddr=0x%x", textAddr)
	}

	var initStackOffset int32

	for {
		if len(native) == 0 {
			err = errors.New("ran out of stack before initial call")
			return
		}

		absRetAddr := binary.LittleEndian.Uint64(native)
		native = native[8:]

		if false {
			log.Printf("exportStack: level=%d absRetAddr=0x%x", level, absRetAddr)
		}

		retAddr := absRetAddr - textAddr
		if retAddr > math.MaxUint32 {
			err = fmt.Errorf("return address 0x%x is not in text section", absRetAddr)
			return
		}

		init, _, callIndex, stackOffset, _ := codeMap.FindAddr(uint32(retAddr))
		if callIndex < 0 {
			err = fmt.Errorf("call instruction not found for return address 0x%x", retAddr)
			return
		}

		binary.LittleEndian.PutUint64(portable, uint64(stackOffset)<<32|uint64(callIndex))
		portable = portable[8:]

		if false {
			log.Printf("exportStack: level=%d callIndex=%d stackOffset=%d", level, callIndex, stackOffset)
			level++
		}

		if init {
			initStackOffset = stackOffset
			break
		}

		if stackOffset == 0 || stackOffset&7 != 0 {
			err = fmt.Errorf("invalid stack offset %d", stackOffset)
			return
		}

		copy(portable[:stackOffset-8], native[:stackOffset-8])
		native = native[stackOffset-8:]
		portable = portable[stackOffset-8:]
	}

	switch initStackOffset {
	case 8:
		// Stack still contains entry function address: this call site precedes
		// entry function call; this is the start function return site.
		funcAddr := binary.LittleEndian.Uint64(native)
		native = native[8:]

		if false {
			log.Printf("exportStack: level=%d entry funcAddr=0x%x", level, funcAddr)
		}

		funcIndex := uint32(math.MaxUint32) // No entry function.

		if funcAddr != 0 {
			i := sort.Search(len(codeMap.FuncAddrs), func(i int) bool {
				return uint64(codeMap.FuncAddrs[i]) >= funcAddr
			})
			if i == len(codeMap.FuncAddrs) || uint64(codeMap.FuncAddrs[i]) != funcAddr {
				err = fmt.Errorf("entry function address 0x%x is unknown", funcAddr)
				return
			}
			funcIndex = uint32(i)
		}

		binary.LittleEndian.PutUint64(portable, uint64(funcIndex))
		portable = portable[8:]

	case 0:
		// Entry function address has been popped off the stack: this call site
		// follows start function call; this is the entry function return site.

	default:
		err = fmt.Errorf("initial function call site has inconsistent stack offset %d", initStackOffset)
		return
	}

	if n := len(native); n != 0 {
		err = fmt.Errorf("%d bytes of excess data at start of stack", n)
		return
	}
	if n := len(portable); n != len(native) {
		panic(n)
	}
	return
}

// importStack converts buffer from portable to native representation in-place.
func importStack(buf []byte, textAddr uint64, codeMap object.CallMap, types []wa.FuncType, funcTypeIndexes []uint32,
) (err error) {
	if n := len(buf); n == 0 || n&7 != 0 {
		err = fmt.Errorf("invalid stack size %d", n)
		return
	}

	var level int

	var minVars int
	var call object.CallSite

	for {
		if len(buf) == 0 {
			err = errors.New("ran out of stack before initial call")
			return
		}

		pair := binary.LittleEndian.Uint64(buf)

		callIndex := uint32(pair)
		if callIndex >= uint32(len(codeMap.CallSites)) {
			err = fmt.Errorf("function call site index %d is unknown", callIndex)
			return
		}
		call = codeMap.CallSites[callIndex]

		if off := int32(pair >> 32); off != call.StackOffset {
			err = fmt.Errorf("encoded stack offset %d of call site %d does not match offset %d in map", off, callIndex, call.StackOffset)
			return
		}

		binary.LittleEndian.PutUint64(buf, textAddr+uint64(call.RetAddr))
		buf = buf[8:]

		if false {
			log.Printf("importStack: level=%d callIndex=%d call.RetAddr=0x%x call.StackOffset=%d", level, callIndex, call.RetAddr, call.StackOffset)
			level++
		}

		if len(codeMap.FuncAddrs) == 0 || call.RetAddr < codeMap.FuncAddrs[0] {
			break
		}

		if call.StackOffset&7 != 0 {
			err = fmt.Errorf("invalid stack offset %d", call.StackOffset)
			return
		}
		if int(call.StackOffset-8) < minVars*8 {
			err = fmt.Errorf("inconsistent call stack")
			return
		}

		buf = buf[call.StackOffset-8:]

		init, funcIndex, callIndexAgain, stackOffsetAgain, _ := codeMap.FindAddr(call.RetAddr)
		if init || callIndexAgain != int(callIndex) || stackOffsetAgain != call.StackOffset {
			err = fmt.Errorf("call instruction not found for return address 0x%x", call.RetAddr)
			return
		}

		sigIndex := funcTypeIndexes[funcIndex]
		sig := types[sigIndex]
		minVars = len(sig.Params)
	}

	if minVars > 0 {
		err = fmt.Errorf("inconsistent call stack")
		return
	}

	switch call.StackOffset {
	case 8:
		// See the comment in exportStack.
		var funcAddr uint32

		if funcIndex := binary.LittleEndian.Uint32(buf); funcIndex != math.MaxUint32 {
			if funcIndex >= uint32(len(codeMap.FuncAddrs)) {
				err = fmt.Errorf("entry function index %d is unknown", funcIndex)
				return
			}
			funcAddr = codeMap.FuncAddrs[funcIndex]

			sigIndex := funcTypeIndexes[funcIndex]
			sig := types[sigIndex]
			if !binding.IsEntryFuncType(sig) {
				err = fmt.Errorf("entry function %d has invalid signature: %s", funcIndex, sig)
				return
			}
		}

		binary.LittleEndian.PutUint64(buf, uint64(funcAddr))
		buf = buf[8:]

	case 0:
		// See the comment in exportStack.

	default:
		err = fmt.Errorf("initial function call site 0x%x has inconsistent stack offset %d", call.RetAddr, call.StackOffset)
		return
	}

	if n := len(buf); n != 0 {
		err = fmt.Errorf("%d bytes of excess data at start of stack", n)
		return
	}
	return
}
