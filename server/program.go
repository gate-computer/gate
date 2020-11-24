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

func validateHashBytes(hash1 string, digest2 []byte) (err error) {
	digest1, err := hex.DecodeString(hash1)
	if err != nil {
		return
	}

	if subtle.ConstantTimeCompare(digest1, digest2) != 1 {
		err = failrequest.New(event.FailModuleHashMismatch, "module hash does not match content")
		return
	}

	return
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
func buildProgram(storage image.Storage, progPolicy *ProgramPolicy, instPolicy *InstancePolicy, mod *ModuleUpload, entryName string) (prog *program, inst *image.Instance, err error) {
	content := mod.takeStream()
	defer func() {
		if content != nil {
			content.Close()
		}
	}()

	var codeMap object.CallMap

	b, err := build.New(storage, int(mod.Length), progPolicy.MaxTextSize, &codeMap, instPolicy != nil)
	if err != nil {
		return
	}
	defer b.Close()

	hasher := api.KnownModuleHash.New()
	reader := bufio.NewReader(io.TeeReader(io.TeeReader(content, b.Image.ModuleWriter()), hasher))

	b.InstallEarlySnapshotLoaders()

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), reader)
	if err != nil {
		return
	}

	b.StackSize = progPolicy.MaxStackSize
	if instPolicy != nil {
		if b.StackSize > instPolicy.StackSize {
			b.StackSize = instPolicy.StackSize
		}
		err = b.SetMaxMemorySize(instPolicy.MaxMemorySize)
		if err != nil {
			return
		}
	}

	err = b.BindFunctions(entryName)
	if err != nil {
		return
	}

	err = compile.LoadCodeSection(b.CodeConfig(&codeMap), reader, b.Module, abi.Library())
	if err != nil {
		return
	}

	err = b.VerifyBreakpoints()
	if err != nil {
		return
	}

	b.InstallSnapshotDataLoaders()

	err = compile.LoadCustomSections(&b.Config, reader)
	if err != nil {
		return
	}

	err = b.FinishImageText()
	if err != nil {
		return
	}

	b.InstallLateSnapshotLoaders()

	err = compile.LoadDataSection(b.DataConfig(), reader, b.Module)
	if err != nil {
		return
	}

	err = compile.LoadCustomSections(&b.Config, reader)
	if err != nil {
		return
	}

	err = content.Close()
	content = nil
	if err != nil {
		err = wrapContentError(err)
		return
	}

	actualHash := hasher.Sum(nil)

	if mod.Hash != "" {
		err = validateHashBytes(mod.Hash, actualHash)
		if err != nil {
			return
		}
	}

	progImage, err := b.FinishProgramImage()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			progImage.Close()
		}
	}()

	if instPolicy != nil {
		inst, err = b.FinishInstanceImage(progImage)
		if err != nil {
			return
		}
	}

	prog = newProgram(api.EncodeKnownModule(actualHash), progImage, b.Buffers, false)
	return
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

func (prog *program) ensureStorage() (err error) {
	prog.storeMu.Lock()
	defer prog.storeMu.Unlock()

	if prog.stored {
		return
	}

	err = prog.image.Store(prog.id)
	if err != nil {
		return
	}

	prog.stored = true
	return
}

func rebuildProgramImage(storage image.Storage, progPolicy *ProgramPolicy, content io.Reader, debugInfo bool, breakpoints []uint64) (progImage *image.Program, textMap stack.TextMap, err error) {
	var (
		mapper  compile.ObjectMapper
		callMap *object.CallMap
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
	if err != nil {
		return
	}
	defer b.Close()

	reader := bufio.NewReader(content)

	b.InstallEarlySnapshotLoaders()

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), reader)
	if err != nil {
		return
	}

	b.StackSize = progPolicy.MaxStackSize

	err = b.BindFunctions("")
	if err != nil {
		return
	}

	if b.Snapshot == nil {
		b.Snapshot = new(snapshot.Snapshot)
	}
	b.Snapshot.Breakpoints = append([]uint64(nil), breakpoints...)

	codeReader := compile.Reader(reader)
	if len(breakpoints) > 0 {
		codeReader = debug.NewReadTeller(codeReader)
	}

	err = compile.LoadCodeSection(b.CodeConfig(mapper), codeReader, b.Module, abi.Library())
	if err != nil {
		return
	}

	err = b.VerifyBreakpoints()
	if err != nil {
		return
	}

	b.InstallSnapshotDataLoaders()

	err = compile.LoadCustomSections(&b.Config, reader)
	if err != nil {
		return
	}

	err = b.FinishImageText()
	if err != nil {
		return
	}

	b.InstallLateSnapshotLoaders()

	err = compile.LoadDataSection(b.DataConfig(), reader, b.Module)
	if err != nil {
		return
	}

	progImage, err = b.FinishProgramImage()
	return
}
