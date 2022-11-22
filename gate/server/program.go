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
	"gate.computer/gate/runtime/abi"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal/error/failrequest"
	"gate.computer/gate/snapshot"
	"gate.computer/internal/error/badmodule"
	"gate.computer/wag/compile"
	"gate.computer/wag/object"
)

var errModuleSizeMismatch = &badmodule.Dual{
	Private: "content length does not match existing module size",
	Public:  "invalid module content",
}

func validateHashBytes(pan icky, hash1 string, digest2 []byte) {
	digest1, err := hex.DecodeString(hash1)
	pan.check(err)

	if subtle.ConstantTimeCompare(digest1, digest2) != 1 {
		pan.check(failrequest.Error(event.FailModuleHashMismatch, "module hash does not match content"))
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
func buildProgram(pan icky, storage image.Storage, progPolicy *ProgramPolicy, instPolicy *InstancePolicy, mod *api.ModuleUpload, entryName string) (*program, *image.Instance) {
	content := mod.TakeStream()
	defer func() {
		if content != nil {
			content.Close()
		}
	}()

	var codeMap object.CallMap

	b, err := build.New(storage, int(mod.Length), progPolicy.MaxTextSize, &codeMap, instPolicy != nil)
	pan.check(err)
	defer b.Close()

	hasher := api.KnownModuleHash.New()
	r := compile.NewLoader(bufio.NewReader(io.TeeReader(io.TeeReader(content, b.Image.ModuleWriter()), hasher)))

	b.InstallEarlySnapshotLoaders()

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), r)
	pan.check(err)

	b.StackSize = progPolicy.MaxStackSize
	if instPolicy != nil {
		if b.StackSize > instPolicy.StackSize {
			b.StackSize = instPolicy.StackSize
		}
		pan.check(b.SetMaxMemorySize(instPolicy.MaxMemorySize))
	}

	pan.check(b.BindFunctions(entryName))

	pan.check(compile.LoadCodeSection(b.CodeConfig(&codeMap), r, b.Module, abi.Library()))

	pan.check(b.VerifyBreakpoints())

	b.InstallSnapshotDataLoaders()

	pan.check(compile.LoadCustomSections(&b.Config, r))

	pan.check(b.FinishImageText())

	b.InstallLateSnapshotLoaders()

	pan.check(compile.LoadDataSection(b.DataConfig(), r, b.Module))
	pan.check(compile.LoadCustomSections(&b.Config, r))

	err = content.Close()
	content = nil
	if err != nil {
		pan.check(wrapContentError(err))
	}

	actualHash := hasher.Sum(nil)

	if mod.Hash != "" {
		validateHashBytes(pan, mod.Hash, actualHash)
	}

	progImage, err := b.FinishProgramImage()
	pan.check(err)
	defer closeProgramImage(&progImage)

	var inst *image.Instance

	if instPolicy != nil {
		inst, err = b.FinishInstanceImage(progImage)
		pan.check(err)
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

func (prog *program) ensureStorage(pan icky) {
	prog.storeMu.Lock()
	defer prog.storeMu.Unlock()

	if prog.stored {
		return
	}

	pan.check(prog.image.Store(prog.id))
	prog.stored = true
}

func rebuildProgramImage(pan icky, storage image.Storage, progPolicy *ProgramPolicy, content io.Reader, breakpoints []uint64) (*image.Program, *object.CallMap) {
	callMap := new(object.CallMap)

	b, err := build.New(storage, 0, progPolicy.MaxTextSize, callMap, false)
	pan.check(err)
	defer b.Close()

	r := compile.NewLoader(bufio.NewReader(content))

	b.InstallEarlySnapshotLoaders()

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), r)
	pan.check(err)

	b.StackSize = progPolicy.MaxStackSize

	pan.check(b.BindFunctions(""))

	if b.Snapshot == nil {
		b.Snapshot = new(snapshot.Snapshot)
	}
	b.Snapshot.Breakpoints = append([]uint64(nil), breakpoints...)

	pan.check(compile.LoadCodeSection(b.CodeConfig(callMap), r, b.Module, abi.Library()))

	pan.check(b.VerifyBreakpoints())

	b.InstallSnapshotDataLoaders()

	pan.check(compile.LoadCustomSections(&b.Config, r))

	pan.check(b.FinishImageText())

	b.InstallLateSnapshotLoaders()

	pan.check(compile.LoadDataSection(b.DataConfig(), r, b.Module))

	progImage, err := b.FinishProgramImage()
	pan.check(err)

	return progImage, callMap
}
