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
	"log/slog"
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
	"import.name/pan"

	. "import.name/pan/mustcheck"
)

var errModuleSizeMismatch = &badmodule.Dual{
	Private: "content length does not match existing module size",
	Public:  "invalid module content",
}

func mustValidateHashBytes(hash1 string, digest2 []byte) {
	if subtle.ConstantTimeCompare(Must(hex.DecodeString(hash1)), digest2) == 1 {
		return
	}
	pan.Panic(failrequest.Error(event.FailModuleHashMismatch, "module hash does not match content"))
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

// mustBuildProgram returns an instance if instance policy is defined.  Entry
// name can be provided only when building an instance.
func mustBuildProgram(storage image.Storage, progPolicy *ProgramPolicy, instPolicy *InstancePolicy, mod *api.ModuleUpload, entryName string) (*program, *image.Instance) {
	content := mod.TakeStream()
	defer func() {
		if content != nil {
			content.Close()
		}
	}()

	var codeMap object.CallMap

	b := Must(build.New(storage, int(mod.Length), progPolicy.MaxTextSize, &codeMap, instPolicy != nil))
	defer b.Close()

	hasher := api.KnownModuleHash.New()
	r := compile.NewLoader(bufio.NewReader(io.TeeReader(io.TeeReader(content, b.Image.ModuleWriter()), hasher)))

	b.InstallEarlySnapshotLoaders()
	b.Module = Must(compile.LoadInitialSections(b.ModuleConfig(), r))

	b.StackSize = progPolicy.MaxStackSize
	if instPolicy != nil {
		if b.StackSize > instPolicy.StackSize {
			b.StackSize = instPolicy.StackSize
		}
		Check(b.SetMaxMemorySize(instPolicy.MaxMemorySize))
	}

	Check(b.BindFunctions(entryName))
	Check(compile.LoadCodeSection(b.CodeConfig(&codeMap), r, b.Module, abi.Library()))
	Check(b.VerifyBreakpoints())
	b.InstallSnapshotDataLoaders()
	Check(compile.LoadCustomSections(&b.Config, r))
	Check(b.FinishImageText())
	b.InstallLateSnapshotLoaders()
	Check(compile.LoadDataSection(b.DataConfig(), r, b.Module))
	Check(compile.LoadCustomSections(&b.Config, r))

	c := content
	content = nil
	if err := c.Close(); err != nil {
		pan.Panic(wrapContentError(err))
	}

	actualHash := hasher.Sum(nil)
	if mod.Hash != "" {
		mustValidateHashBytes(mod.Hash, actualHash)
	}

	progImage := Must(b.FinishProgramImage())
	defer closeProgramImage(&progImage)

	var inst *image.Instance
	if instPolicy != nil {
		inst = Must(b.FinishInstanceImage(progImage))
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
			slog.Error("server: closing unreachable program", "module", prog.id, "refcount", prog.refCount)
			prog.image.Close()
			prog.image = nil
		} else {
			slog.Error("server: unreachable program with negative reference count", "module", prog.id, "refcount", prog.refCount)
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

func (prog *program) mustEnsureStorage() {
	prog.storeMu.Lock()
	defer prog.storeMu.Unlock()

	if prog.stored {
		return
	}
	Check(prog.image.Store(prog.id))
	prog.stored = true
}

func mustRebuildProgramImage(storage image.Storage, progPolicy *ProgramPolicy, content io.Reader, breakpoints []uint64) (*image.Program, *object.CallMap) {
	callMap := new(object.CallMap)

	b := Must(build.New(storage, 0, progPolicy.MaxTextSize, callMap, false))
	defer b.Close()

	r := compile.NewLoader(bufio.NewReader(content))
	b.InstallEarlySnapshotLoaders()
	b.Module = Must(compile.LoadInitialSections(b.ModuleConfig(), r))
	b.StackSize = progPolicy.MaxStackSize
	Check(b.BindFunctions(""))

	if b.Snapshot == nil {
		b.Snapshot = new(snapshot.Snapshot)
	}
	b.Snapshot.Breakpoints = append([]uint64(nil), breakpoints...)

	Check(compile.LoadCodeSection(b.CodeConfig(callMap), r, b.Module, abi.Library()))
	Check(b.VerifyBreakpoints())
	b.InstallSnapshotDataLoaders()
	Check(compile.LoadCustomSections(&b.Config, r))
	Check(b.FinishImageText())
	b.InstallLateSnapshotLoaders()
	Check(compile.LoadDataSection(b.DataConfig(), r, b.Module))
	progImage := Must(b.FinishProgramImage())

	return progImage, callMap
}
