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

	"gate.computer/internal/manifest"
	"gate.computer/wag/binding"
	"gate.computer/wag/object"
	"gate.computer/wag/object/stack"
	"gate.computer/wag/wa"
)

const initStackSize = 24

func putInitStack(portable []byte, start, entry *manifest.Function) {
	if n := len(portable); n != initStackSize {
		panic(n)
	}

	var (
		startIndex uint64 = math.MaxUint64
		entryIndex uint64 = math.MaxUint64
	)
	if start != nil {
		startIndex = uint64(start.Index)
	}
	if entry != nil {
		entryIndex = uint64(entry.Index)
	}

	const callIndex = 0    // Virtual call site at beginning of enter routine.
	const stackOffset = 16 // The function address are on the stack.

	binary.LittleEndian.PutUint64(portable[0:], stackOffset<<32|callIndex)
	binary.LittleEndian.PutUint64(portable[8:], startIndex)
	binary.LittleEndian.PutUint64(portable[16:], entryIndex)
}

// exportStack from native source buffer to portable target buffer.
func exportStack(portable, native []byte, textAddr uint64, textMap stack.TextMap) (err error) {
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

		init, _, callIndex, stackOffset, _ := textMap.FindCall(uint32(retAddr))
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
			if i, found := textMap.FindFunc(uint32(funcAddr)); found {
				funcIndex = uint32(i)
			}
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
func importStack(buf []byte, textAddr uint64, codeMap object.CallMap, types []wa.FuncType, funcTypeIndexes []uint32) error {
	if n := len(buf); n == 0 || n&7 != 0 {
		return fmt.Errorf("invalid stack size %d", n)
	}

	var level int

	var minVars int
	var call object.CallSite

	for {
		if len(buf) == 0 {
			return errors.New("ran out of stack before initial call")
		}

		pair := binary.LittleEndian.Uint64(buf)

		callIndex := uint32(pair)
		if callIndex >= uint32(len(codeMap.CallSites)) {
			return fmt.Errorf("function call site index %d is unknown", callIndex)
		}
		call = codeMap.CallSites[callIndex]

		if off := int32(pair >> 32); off != call.StackOffset {
			return fmt.Errorf("encoded stack offset %d of call site %d does not match offset %d in map", off, callIndex, call.StackOffset)
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
			return fmt.Errorf("invalid stack offset %d", call.StackOffset)
		}
		if int(call.StackOffset-8) < minVars*8 {
			return errors.New("inconsistent call stack")
		}

		buf = buf[call.StackOffset-8:]

		init, funcIndex, callIndexAgain, stackOffsetAgain, _ := codeMap.FindCall(call.RetAddr)
		if init || callIndexAgain != int(callIndex) || stackOffsetAgain != call.StackOffset {
			return fmt.Errorf("call instruction not found for return address 0x%x", call.RetAddr)
		}

		sigIndex := funcTypeIndexes[funcIndex]
		sig := types[sigIndex]
		minVars = len(sig.Params)
	}

	if minVars > 0 {
		return errors.New("inconsistent call stack")
	}

	switch call.StackOffset {
	case 16:
		// Stack was synthesized by putInitStack.
		var startAddr uint32

		if funcIndex := binary.LittleEndian.Uint64(buf); funcIndex != math.MaxUint64 {
			if funcIndex >= uint64(len(codeMap.FuncAddrs)) {
				return fmt.Errorf("start function index %d is unknown", funcIndex)
			}
			startAddr = codeMap.FuncAddrs[funcIndex]

			sigIndex := funcTypeIndexes[funcIndex]
			sig := types[sigIndex]
			if !sig.Equal(wa.FuncType{}) {
				return fmt.Errorf("start function %d has invalid signature: %s", funcIndex, sig)
			}
		}

		binary.LittleEndian.PutUint64(buf, uint64(startAddr))
		buf = buf[8:]
		fallthrough

	case 8:
		// See the comment in exportStack.
		var entryAddr uint32

		if funcIndex := binary.LittleEndian.Uint64(buf); funcIndex != math.MaxUint64 {
			if funcIndex >= uint64(len(codeMap.FuncAddrs)) {
				return fmt.Errorf("entry function index %d is unknown", funcIndex)
			}
			entryAddr = codeMap.FuncAddrs[funcIndex]

			sigIndex := funcTypeIndexes[funcIndex]
			sig := types[sigIndex]
			if !binding.IsEntryFuncType(sig) {
				return fmt.Errorf("entry function %d has invalid signature: %s", funcIndex, sig)
			}
		}

		binary.LittleEndian.PutUint64(buf, uint64(entryAddr))
		buf = buf[8:]

	case 0:
		// See the comment in exportStack.

	default:
		return fmt.Errorf("initial function call site 0x%x has inconsistent stack offset %d", call.RetAddr, call.StackOffset)
	}

	if n := len(buf); n != 0 {
		return fmt.Errorf("%d bytes of excess data at start of stack", n)
	}

	return nil
}
