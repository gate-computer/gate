// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"bufio"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	goruntime "runtime"
	"sync"

	"gate.computer/gate/build"
	"gate.computer/gate/image"
	"gate.computer/gate/internal/error/badmodule"
	"gate.computer/gate/runtime/abi"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal/error/failrequest"
	"gate.computer/gate/snapshot"
	"gate.computer/wag/compile"
	"gate.computer/wag/object"
	"gate.computer/wag/object/debug"
	"gate.computer/wag/object/stack"
)

var errModuleSizeMismatch = &badmodule.Dual{
	Private: "content length does not match existing module size",
	Public:  "invalid module content",
}

func _validateHashBytes(hash1 string, digest2 []byte) {
	digest1, err := hex.DecodeString(hash1)
	_check(err)

	if subtle.ConstantTimeCompare(digest1, digest2) != 1 {
		panic(failrequest.New(event.FailModuleHashMismatch, "module hash does not match content"))
	}
}

type program struct {
	id      string
	image   *image.Program
	buffers snapshot.Buffers

	storeMu sync.Mutex
	stored  bool

	// Protected by server mutex:
	refCount int
}

// buildProgram returns an instance if instance policy is defined.  Entry name
// can be provided only when building an instance.
func _buildProgram(storage image.Storage, progPolicy *ProgramPolicy, instPolicy *InstancePolicy, mod *ModuleUpload, entryName string) (*program, *image.Instance) {
	content := mod.takeStream()
	defer func() {
		if content != nil {
			content.Close()
		}
	}()

	var codeMap object.CallMap

	b, err := build.New(storage, int(mod.Length), progPolicy.MaxTextSize, &codeMap, instPolicy != nil)
	_check(err)
	defer b.Close()

	hasher := api.KnownModuleHash.New()
	reader := bufio.NewReader(io.TeeReader(io.TeeReader(content, b.Image.ModuleWriter()), hasher))

	b.InstallEarlySnapshotLoaders()

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), reader)
	_check(err)

	b.StackSize = progPolicy.MaxStackSize
	if instPolicy != nil {
		if b.StackSize > instPolicy.StackSize {
			b.StackSize = instPolicy.StackSize
		}
		_check(b.SetMaxMemorySize(instPolicy.MaxMemorySize))
	}

	_check(b.BindFunctions(entryName))

	_check(compile.LoadCodeSection(b.CodeConfig(&codeMap), reader, b.Module, abi.Library()))

	_check(b.VerifyBreakpoints())

	b.InstallSnapshotDataLoaders()

	_check(compile.LoadCustomSections(&b.Config, reader))

	_check(b.FinishImageText())

	b.InstallLateSnapshotLoaders()

	_check(compile.LoadDataSection(b.DataConfig(), reader, b.Module))
	_check(compile.LoadCustomSections(&b.Config, reader))

	err = content.Close()
	content = nil
	if err != nil {
		panic(wrapContentError(err))
	}

	actualHash := hasher.Sum(nil)

	if mod.Hash != "" {
		_validateHashBytes(mod.Hash, actualHash)
	}

	progImage, err := b.FinishProgramImage()
	_check(err)
	defer closeProgramImage(&progImage)

	var inst *image.Instance

	if instPolicy != nil {
		inst, err = b.FinishInstanceImage(progImage)
		_check(err)
	}

	prog := newProgram(api.EncodeKnownModule(actualHash), progImage, b.Buffers, false)
	progImage = nil

	return prog, inst
}

func newProgram(id string, image *image.Program, buffers snapshot.Buffers, stored bool) *program {
	prog := &program{
		id:       id,
		image:    image,
		buffers:  buffers,
		stored:   stored,
		refCount: 1,
	}
	goruntime.SetFinalizer(prog, finalizeProgram)
	return prog
}

func finalizeProgram(prog *program) {
	if prog.refCount != 0 {
		if prog.refCount > 0 {
			log.Printf("closing unreachable program %q with reference count %d", prog.id, prog.refCount)
			prog.image.Close()
			prog.image = nil
		} else {
			log.Printf("unreachable program %q with reference count %d", prog.id, prog.refCount)
		}
	}
}

func (prog *program) ref(lock serverLock) *program {
	if prog.refCount <= 0 {
		panic(fmt.Sprintf("referencing program %q with reference count %d", prog.id, prog.refCount))
	}

	prog.refCount++
	return prog
}

func (prog *program) unref(lock serverLock) {
	if prog.refCount <= 0 {
		panic(fmt.Sprintf("unreferencing program %q with reference count %d", prog.id, prog.refCount))
	}

	prog.refCount--
	if prog.refCount == 0 {
		prog.image.Close()
		prog.image = nil
		goruntime.KeepAlive(prog)
	}
}

func (prog *program) _ensureStorage() {
	prog.storeMu.Lock()
	defer prog.storeMu.Unlock()

	if prog.stored {
		return
	}

	_check(prog.image.Store(prog.id))
	prog.stored = true
}

func _rebuildProgramImage(storage image.Storage, progPolicy *ProgramPolicy, content io.Reader, debugInfo bool, breakpoints []uint64) (*image.Program, stack.TextMap) {
	var (
		mapper  compile.ObjectMapper
		callMap *object.CallMap
		textMap stack.TextMap
	)
	if debugInfo {
		m := new(debug.TrapMap)
		mapper = m
		callMap = &m.CallMap
		textMap = m
	} else {
		m := new(object.CallMap)
		mapper = m
		callMap = m
		textMap = m
	}

	b, err := build.New(storage, 0, progPolicy.MaxTextSize, callMap, false)
	_check(err)
	defer b.Close()

	reader := bufio.NewReader(content)

	b.InstallEarlySnapshotLoaders()

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), reader)
	_check(err)

	b.StackSize = progPolicy.MaxStackSize

	_check(b.BindFunctions(""))

	if b.Snapshot == nil {
		b.Snapshot = new(snapshot.Snapshot)
	}
	b.Snapshot.Breakpoints = append([]uint64(nil), breakpoints...)

	codeReader := compile.Reader(reader)
	if len(breakpoints) > 0 {
		codeReader = debug.NewReadTeller(codeReader)
	}

	_check(compile.LoadCodeSection(b.CodeConfig(mapper), codeReader, b.Module, abi.Library()))

	_check(b.VerifyBreakpoints())

	b.InstallSnapshotDataLoaders()

	_check(compile.LoadCustomSections(&b.Config, reader))

	_check(b.FinishImageText())

	b.InstallLateSnapshotLoaders()

	_check(compile.LoadDataSection(b.DataConfig(), reader, b.Module))

	progImage, err := b.FinishProgramImage()
	_check(err)

	return progImage, textMap
}
