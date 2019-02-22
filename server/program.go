// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"bufio"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"log"
	goruntime "runtime"

	"github.com/tsavola/gate/entry"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/image/manifest"
	"github.com/tsavola/gate/image/wasm"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	"github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/runtime/abi"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/section"
	"github.com/tsavola/wag/wa"
)

var (
	newHash      = sha512.New384
	hashEncoding = base64.URLEncoding
)

func validateHashBytes(hash1 string, digest2 []byte) (err error) {
	digest1, err := hashEncoding.DecodeString(hash1)
	if err != nil {
		return
	}

	if subtle.ConstantTimeCompare(digest1, digest2) != 1 {
		err = failrequest.New(event.FailRequest_ModuleHashMismatch, "module hash does not match content")
		return
	}

	return
}

func validateHashContent(hash1 string, r io.Reader) (err error) {
	hash2 := newHash()

	_, err = io.Copy(hash2, r)
	if err != nil {
		err = wrapContentError(err)
		return
	}

	return validateHashBytes(hash1, hash2.Sum(nil))
}

type program struct {
	hash    string
	man     manifest.Archive
	archive image.LocalArchive
	codeMap object.CallMap

	// Protected by Server.lock:
	refCount int
}

// buildProgram returns Executable if ExecutableRef is provided.
// InstancePolicy must be provided with ExecutableRef.  Entry name can be
// provided only with ExecutableRef.
func buildProgram(refBack image.BackingStore, ref image.ExecutableRef, instPolicy *InstancePolicy, progPolicy *ProgramPolicy, arcBack image.LocalStorage, allegedHash string, content io.ReadCloser, contentSize int, entryName string,
) (exe *image.Executable, prog *program, err error) {
	defer func() {
		if content != nil {
			content.Close()
		}
	}()

	var codeMap object.CallMap

	build, err := image.NewBuild(arcBack, refBack, ref, contentSize, progPolicy.MaxTextSize, &codeMap)
	if err != nil {
		return
	}
	defer build.Close()

	var actualHash = newHash()
	var r = bufio.NewReader(io.TeeReader(io.TeeReader(content, build.ModuleWriter()), actualHash))

	var sectionMap image.SectionMap
	var sectionLoaders = make(section.CustomLoaders)
	var sectionConfig = compile.Config{
		SectionMapper:       sectionMap.Mapper(),
		CustomSectionLoader: sectionLoaders.Load,
	}

	sectionLoaders[wasm.StackSectionName] = func(string, section.Reader, uint32) error {
		return failrequest.New(event.FailRequest_ModuleError, "stack section appears too early in wasm module")
	}

	module, err := compile.LoadInitialSections(&compile.ModuleConfig{Config: sectionConfig}, r)
	if err != nil {
		err = failrequest.Tag(event.FailRequest_ModuleError, err)
		return
	}

	var stackSize int
	var memorySize = module.InitialMemorySize()
	var maxMemorySize int

	if instPolicy == nil {
		stackSize = progPolicy.MaxStackSize

		maxMemorySize = memorySize
	} else {
		stackSize = instPolicy.StackSize

		maxMemorySize = module.MemorySizeLimit()
		if maxMemorySize > instPolicy.MaxMemorySize {
			maxMemorySize = alignMemorySize(instPolicy.MaxMemorySize)
		}

		if memorySize > maxMemorySize {
			err = resourcelimit.New("initial program memory size exceeds instance memory size limit")
			return
		}
	}

	err = binding.BindImports(&module, abi.Imports)
	if err != nil {
		return
	}

	var entryIndex uint32

	if entryName != "" {
		entryIndex, err = entry.ModuleFuncIndex(module, entryName)
		if err != nil {
			return
		}
	}

	var codeConfig = &compile.CodeConfig{
		Text:   build.TextBuffer(),
		Mapper: &codeMap,
		Config: sectionConfig,
	}

	err = compile.LoadCodeSection(codeConfig, r, module)
	if err != nil {
		err = failrequest.Tag(event.FailRequest_ModuleError, err)
		return
	}

	// textCopy := make([]byte, len(build.TextBuffer().Bytes()))
	// copy(textCopy, build.TextBuffer().Bytes())

	sectionLoaders[wasm.StackSectionName] = func(_ string, r section.Reader, length uint32) (err error) {
		sectionLoaders[wasm.StackSectionName] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "multiple stack sections in wasm module")
		}

		if entryName != "" {
			err = failrequest.New(event.FailRequest_FunctionNotFound, "entry function specified with suspended program")
			return
		}

		if length > uint32(stackSize)-executable.StackLimitOffset {
			err = failrequest.New(event.FailRequest_ModuleError, "stack section is too large")
			return
		}

		sectionMap.Stack = sectionMap.Sections[section.Custom] // The section currently being loaded.

		err = build.FinishText(int(length), stackSize, module.GlobalsSize(), memorySize, maxMemorySize)
		if err != nil {
			return
		}

		err = build.ReadSuspendedStack(r, int(length), module.Types(), module.FuncTypeIndexes())
		if err != nil {
			return
		}

		return
	}

	err = compile.LoadCustomSections(&sectionConfig, r)
	if err != nil {
		err = failrequest.Tag(event.FailRequest_ModuleError, err)
		return
	}

	sectionLoaders[wasm.StackSectionName] = func(string, section.Reader, uint32) error {
		return failrequest.New(event.FailRequest_ModuleError, "stack section appears too late in wasm module")
	}

	var entryAddr uint32

	if sectionMap.Stack.Offset == 0 {
		err = build.FinishText(0, stackSize, module.GlobalsSize(), memorySize, maxMemorySize)
		if err != nil {
			return
		}

		if instPolicy != nil {
			if entryName != "" {
				entryAddr = codeMap.FuncAddrs[entryIndex]
			}
			build.SetupEntryStackFrame(entryIndex, entryAddr)
		}
	}

	var dataConfig = &compile.DataConfig{
		GlobalsMemory:   build.GlobalsMemoryBuffer(),
		MemoryAlignment: build.MemoryAlignment(),
		Config:          sectionConfig,
	}

	err = compile.LoadDataSection(dataConfig, r, module)
	if err != nil {
		err = failrequest.Tag(event.FailRequest_ModuleError, err)
		return
	}

	err = compile.LoadCustomSections(&sectionConfig, r)
	if err != nil {
		err = failrequest.Tag(event.FailRequest_ModuleError, err)
		return
	}

	err = content.Close()
	content = nil
	if err != nil {
		err = wrapContentError(err)
		return
	}

	var actualDigest = actualHash.Sum(nil)

	if allegedHash != "" {
		err = validateHashBytes(allegedHash, actualDigest)
		if err != nil {
			return
		}
	}

	var hash = hashEncoding.EncodeToString(actualDigest)

	// if f, err := os.Create("/tmp/text-" + hash + ".txt"); err != nil {
	// 	log.Fatal(err)
	// } else {
	// 	defer f.Close()
	// 	if err := dump.Text(f, textCopy, 0, codeMap.FuncAddrs, nil); err != nil {
	// 		log.Fatal(err)
	// 	}
	// }

	entryIndexes, entryAddrs := entry.Maps(module, codeMap.FuncAddrs)

	archive, exe, err := build.FinishArchiveExecutable(hash, sectionMap, module.GlobalTypes(), entryIndexes, entryAddrs)
	if err != nil {
		return
	}

	prog = newProgram(hash, archive, codeMap)
	return
}

func newProgram(hash string, archive image.LocalArchive, codeMap object.CallMap) *program {
	prog := &program{
		hash:     hash,
		man:      archive.Manifest(),
		archive:  archive,
		codeMap:  codeMap,
		refCount: 1,
	}
	goruntime.SetFinalizer(prog, finalizeProgram)
	return prog
}

func finalizeProgram(prog *program) {
	if prog.refCount != 0 {
		log.Printf("unreachable program with reference count %d", prog.refCount)
		if prog.refCount > 0 {
			prog.cleanup()
		}
	}
}

func (prog *program) cleanup() {
	prog.archive.Close()
}

// ref must be called with Server.lock held.
func (prog *program) ref() *program {
	prog.refCount++
	return prog
}

// unref must be called with Server.lock held.  Caller must invoke the cleanup
// method separately if the final reference was dropped.
func (prog *program) unref() (final bool) {
	prog.refCount--

	switch {
	case prog.refCount == 0:
		final = true

	case prog.refCount < 0:
		panic("program reference count is negative")
	}

	return
}

func (prog *program) resolveEntry(name string) (index, addr uint32, err error) {
	if name == "" {
		return
	}

	index, err = entry.MapFuncIndex(prog.man.EntryIndexes, name)
	if err != nil {
		return
	}

	addr = entry.MapFuncAddr(prog.man.EntryAddrs, index)
	return
}

func alignMemorySize(size int) int {
	mask := wa.PageSize - 1
	return (size + mask) &^ mask
}
