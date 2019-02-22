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

	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/wa"
)

// exportStack from native source buffer to portable target buffer.
func exportStack(portable, native []byte, textAddr uint64, codeMap object.CallMap) (err error) {
	if n := len(native); n == 0 || n&7 != 0 {
		err = fmt.Errorf("invalid stack size %d", n)
		return
	}

	var level int
	if false {
		log.Printf("exportStack: textAddr=0x%x", textAddr)
	}

	var initialStackOffset int32

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

		_, callIndex, _, stackOffset, initial, ok := codeMap.FindAddr(uint32(retAddr))
		if !ok {
			err = fmt.Errorf("call instruction not found for return address 0x%x", retAddr)
			return
		}

		binary.LittleEndian.PutUint64(portable, uint64(callIndex))
		portable = portable[8:]

		if false {
			log.Printf("exportStack: level=%d callIndex=%d", level, callIndex)
			level++
		}

		if initial {
			initialStackOffset = stackOffset
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

	switch initialStackOffset {
	case 16:
		// Stack contains entry function address.  (This precedes entry
		// function call, i.e. this is the start function return site.)
		funcAddr := binary.LittleEndian.Uint32(native)
		native = native[8:]

		if false {
			log.Printf("exportStack: level=%d entry funcAddr=0x%x", level, funcAddr)
		}

		i := sort.Search(len(codeMap.FuncAddrs), func(i int) bool {
			return codeMap.FuncAddrs[i] >= funcAddr
		})
		if i == len(codeMap.FuncAddrs) || codeMap.FuncAddrs[i] != funcAddr {
			err = fmt.Errorf("entry function address 0x%x is unknown", funcAddr)
			return
		}

		binary.LittleEndian.PutUint32(portable, uint32(i)) // Entry function index.
		portable = portable[8:]

	case 8:
		// Entry function address has been popped off stack.  (This
		// follows start function call, i.e. this is the entry function
		// return site.)

	default:
		err = fmt.Errorf("initial function call site has inconsistent stack offset %d", initialStackOffset)
		return
	}

	if n := len(native); n != 0 {
		err = fmt.Errorf("%d bytes of excess data at start of stack", n)
		return
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

		callIndex := binary.LittleEndian.Uint64(buf)
		if callIndex >= uint64(len(codeMap.CallSites)) {
			err = fmt.Errorf("function call site index %d is unknown", callIndex)
			return
		}
		call = codeMap.CallSites[callIndex]

		if call.StackOffset == 0 || call.StackOffset&7 != 0 {
			err = fmt.Errorf("invalid stack offset %d", call.StackOffset)
			return
		}

		binary.LittleEndian.PutUint64(buf, textAddr+uint64(call.RetAddr))
		buf = buf[8:]

		if false {
			log.Printf("importStack: level=%d callIndex=%d absRetAddr=0x%x", level, callIndex, textAddr+uint64(call.RetAddr))
			level++
		}

		if len(codeMap.FuncAddrs) == 0 || call.RetAddr <= codeMap.FuncAddrs[0] {
			break
		}

		if int(call.StackOffset-8) < minVars*8 {
			err = fmt.Errorf("inconsistent call stack")
			return
		}

		buf = buf[call.StackOffset-8:]

		funcIndex, callIndexAgain, _, stackOffsetAgain, initial, ok := codeMap.FindAddr(call.RetAddr)
		if !ok || uint64(callIndexAgain) != callIndex || stackOffsetAgain != call.StackOffset || initial {
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

	// See the comments in exportStack's switch statement.
	switch call.StackOffset {
	case 16:
		funcIndex := binary.LittleEndian.Uint32(buf)
		if funcIndex >= uint32(len(codeMap.FuncAddrs)) {
			err = fmt.Errorf("entry function index %d is unknown", funcIndex)
			return
		}
		funcAddr := codeMap.FuncAddrs[funcIndex]

		sigIndex := funcTypeIndexes[funcIndex]
		sig := types[sigIndex]
		if len(sig.Params) != 0 || sig.Result != wa.Void {
			err = fmt.Errorf("entry function %d has invalid signature: %s", funcIndex, sig)
			return
		}

		binary.LittleEndian.PutUint64(buf, uint64(funcAddr))
		buf = buf[8:]

	case 8:

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
