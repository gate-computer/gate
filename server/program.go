// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"bufio"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	goruntime "runtime"

	"github.com/tsavola/gate/entry"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	"github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/runtime/abi"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/gate/snapshot/wasm"
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/section"
	"github.com/tsavola/wag/wa"
)

var (
	newHash      = sha512.New384
	hashEncoding = base64.RawURLEncoding
)

var errModuleSizeMismatch = failrequest.Wrap(event.FailRequest_ModuleError, errors.New("content length does not match existing module size"), "invalid module content")

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
	key   string
	image *image.Program

	// Protected by Server.lock:
	refCount int
}

// buildProgram returns an instance if instance policy is defined.  Entry name
// can be provided only when building an instance.
func buildProgram(storage image.Storage, progPolicy *ProgramPolicy, instPolicy *InstancePolicy, allegedHash string, content io.ReadCloser, contentSize int, entryName string,
) (prog *program, inst *image.Instance, err error) {
	defer func() {
		if content != nil {
			content.Close()
		}
	}()

	var codeMap object.CallMap

	build, err := image.NewBuild(storage, contentSize, progPolicy.MaxTextSize, &codeMap, instPolicy != nil)
	if err != nil {
		return
	}
	defer build.Close()

	var actualHash = newHash()
	var r = bufio.NewReader(io.TeeReader(io.TeeReader(content, build.ModuleWriter()), actualHash))

	var sectionMap image.SectionMap
	var loaders = make(section.CustomLoaders)
	var sectionConfig = compile.Config{
		SectionMapper:       sectionMap.Mapper(),
		CustomSectionLoader: loaders.Load,
	}

	var buffers snapshot.Buffers
	var serviceBuf []byte

	loaders[wasm.ServiceSection] = func(_ string, r section.Reader, length uint32) (err error) {
		return failrequest.New(event.FailRequest_ModuleError, "service section appears too early in wasm module")
	}
	loaders[wasm.IOSection] = func(_ string, r section.Reader, length uint32) (err error) {
		return failrequest.New(event.FailRequest_ModuleError, "io section appears too early in wasm module")
	}
	loaders[wasm.BufferSection] = func(_ string, r section.Reader, length uint32) (err error) {
		return failrequest.New(event.FailRequest_ModuleError, "buffer section appears too early in wasm module")
	}
	loaders[wasm.StackSection] = func(string, section.Reader, uint32) error {
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

	var entryAddr uint32

	if entryName != "" {
		entryAddr = codeMap.FuncAddrs[entryIndex]
	}

	loaders[wasm.ServiceSection] = func(_ string, r section.Reader, length uint32) (err error) {
		loaders[wasm.ServiceSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "multiple service sections in wasm module")
		}

		sectionMap.Service = sectionMap.Sections[section.Custom] // The section currently being loaded.

		buffers.Services, serviceBuf, err = readServiceSection(r, length)
		return
	}

	loaders[wasm.IOSection] = func(_ string, r section.Reader, length uint32) (err error) {
		loaders[wasm.ServiceSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "service section must appear before io section in wasm module")
		}
		loaders[wasm.IOSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "multiple io sections in wasm module")
		}

		sectionMap.IO = sectionMap.Sections[section.Custom] // The section currently being loaded.

		buffers.Input, buffers.Output, err = readIOSection(r, length)
		return
	}

	loaders[wasm.BufferSection] = func(_ string, r section.Reader, length uint32) (err error) {
		loaders[wasm.ServiceSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "service section must appear before buffer section in wasm module")
		}
		loaders[wasm.IOSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "io section must appear before buffer section in wasm module")
		}
		loaders[wasm.BufferSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "multiple buffer sections in wasm module")
		}

		if uint64(length) != uint64(len(serviceBuf)+len(buffers.Input)+len(buffers.Output)) {
			err = failrequest.New(event.FailRequest_ModuleError, "unexpected buffer section length in wasm module")
		}

		sectionMap.Buffer = sectionMap.Sections[section.Custom] // The section currently being loaded.

		_, err = io.ReadFull(r, serviceBuf)
		if err != nil {
			return
		}

		_, err = io.ReadFull(r, buffers.Input)
		if err != nil {
			return
		}

		_, err = io.ReadFull(r, buffers.Output)
		if err != nil {
			return
		}

		return
	}

	loaders[wasm.StackSection] = func(_ string, r section.Reader, length uint32) (err error) {
		loaders[wasm.ServiceSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "service section must appear before stack section in wasm module")
		}
		loaders[wasm.IOSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "io section must appear before stack section in wasm module")
		}
		loaders[wasm.BufferSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "buffer section must appear before stack section in wasm module")
		}
		loaders[wasm.StackSection] = func(string, section.Reader, uint32) error {
			return failrequest.New(event.FailRequest_ModuleError, "multiple stack sections in wasm module")
		}

		if entryAddr != 0 {
			err = notfound.ErrSuspended
			return
		}

		if length > uint32(stackSize)-executable.StackLimitOffset {
			err = failrequest.New(event.FailRequest_ModuleError, "stack section is too large")
			return
		}

		sectionMap.Stack = sectionMap.Sections[section.Custom] // The section currently being loaded.

		err = build.FinishText(stackSize, int(length), module.GlobalsSize(), memorySize, maxMemorySize)
		if err != nil {
			return
		}

		err = build.ReadStack(r, module.Types(), module.FuncTypeIndexes())
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

	if sectionMap.Stack.Offset == 0 {
		err = build.FinishText(stackSize, 0, module.GlobalsSize(), memorySize, maxMemorySize)
		if err != nil {
			return
		}
	}

	loaders[wasm.ServiceSection] = func(string, section.Reader, uint32) error {
		return failrequest.New(event.FailRequest_ModuleError, "service section appears too late in wasm module")
	}
	loaders[wasm.IOSection] = func(string, section.Reader, uint32) error {
		return failrequest.New(event.FailRequest_ModuleError, "io section appears too late in wasm module")
	}
	loaders[wasm.BufferSection] = func(string, section.Reader, uint32) error {
		return failrequest.New(event.FailRequest_ModuleError, "buffer section appears too late in wasm module")
	}
	loaders[wasm.StackSection] = func(string, section.Reader, uint32) error {
		return failrequest.New(event.FailRequest_ModuleError, "stack section appears too late in wasm module")
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

	var key = hashEncoding.EncodeToString(actualDigest)

	// if f, err := os.Create("/tmp/text-" + hash + ".txt"); err != nil {
	// 	log.Fatal(err)
	// } else {
	// 	defer f.Close()
	// 	if err := dump.Text(f, textCopy, 0, codeMap.FuncAddrs, nil); err != nil {
	// 		log.Fatal(err)
	// 	}
	// }

	entryIndexes, entryAddrs := entry.Maps(module, codeMap.FuncAddrs)

	progImage, err := build.FinishProgram(sectionMap, module.GlobalTypes(), entryIndexes, entryAddrs)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			progImage.Close()
		}
	}()

	err = progImage.Store(key)
	if err != nil {
		return
	}

	inst, err = build.FinishInstance(entryIndex, entryAddr)
	if err != nil {
		return
	}

	prog = newProgram(key, progImage)
	return
}

func newProgram(key string, image *image.Program) *program {
	prog := &program{
		key:      key,
		image:    image,
		refCount: 1,
	}
	goruntime.SetFinalizer(prog, finalizeProgram)
	return prog
}

func finalizeProgram(prog *program) {
	if prog.refCount != 0 {
		if prog.refCount > 0 {
			log.Printf("closing unreachable program %q with reference count %d", prog.key, prog.refCount)
			prog.image.Close()
			prog.image = nil
		} else {
			log.Printf("unreachable program %q with reference count %d", prog.key, prog.refCount)
		}
	}
}

// ref must be called with Server.lock held.
func (prog *program) ref() *program {
	if prog.refCount <= 0 {
		panic(fmt.Sprintf("referencing program %q with reference count %d", prog.key, prog.refCount))
	}

	prog.refCount++
	return prog
}

// unref must be called with Server.lock held.
func (prog *program) unref() {
	if prog.refCount <= 0 {
		panic(fmt.Sprintf("unreferencing program %q with reference count %d", prog.key, prog.refCount))
	}

	prog.refCount--
	if prog.refCount == 0 {
		prog.image.Close()
		prog.image = nil
		goruntime.KeepAlive(prog)
	}
	return
}

func (prog *program) resolveEntry(name string) (index, addr uint32, err error) {
	if name == "" {
		return
	}

	index, err = entry.MapFuncIndex(prog.image.Manifest().EntryIndexes, name)
	if err != nil {
		return
	}

	addr = entry.MapFuncAddr(prog.image.Manifest().EntryAddrs, index)
	return
}

func alignMemorySize(size int) int {
	mask := wa.PageSize - 1
	return (size + mask) &^ mask
}

// TODO: move these (and the custom section logic in buildProgram) to some other package

func readServiceSection(r section.Reader, length uint32,
) (services []snapshot.Service, buf []byte, err error) {
	var readLen int

	count, n, err := readVaruint32(r)
	if err != nil {
		return
	}
	readLen += n

	// TODO: validate count

	services = make([]snapshot.Service, count)
	sizes := make([]uint32, count)

	var totalSize uint64

	for i := range services {
		var nameLen byte

		nameLen, err = r.ReadByte()
		if err != nil {
			return
		}
		readLen++

		// TODO: validate nameLen

		b := make([]byte, nameLen)
		n, err = io.ReadFull(r, b)
		if err != nil {
			return
		}
		readLen += n
		services[i].Name = string(b)

		sizes[i], n, err = readVaruint32(r)
		if err != nil {
			return
		}
		readLen += n

		// TODO: validate size

		totalSize += uint64(sizes[i])
	}

	if uint64(readLen) != uint64(length) {
		err = failrequest.New(event.FailRequest_ModuleError, "invalid service section in wasm module")
		return
	}

	// TODO: validate totalSize

	buf = make([]byte, totalSize)
	return
}

func readIOSection(r section.Reader, length uint32) (inputBuf, outputBuf []byte, err error) {
	inputSize, n1, err := readVaruint32(r)
	if err != nil {
		return
	}

	outputSize, n2, err := readVaruint32(r)
	if err != nil {
		return
	}

	// TODO: validate sizes

	if uint64(n1+n2) != uint64(length) {
		err = failrequest.New(event.FailRequest_ModuleError, "invalid io section in wasm module")
		return
	}

	inputBuf = make([]byte, inputSize)
	outputBuf = make([]byte, outputSize)
	return
}

func readVaruint32(r section.Reader) (x uint32, n int, err error) {
	var shift uint
	for n = 1; ; n++ {
		var b byte
		b, err = r.ReadByte()
		if err != nil {
			return
		}
		if b < 0x80 {
			if n > 5 || n == 5 && b > 0xf {
				err = failrequest.New(event.FailRequest_ModuleError, "varuint32 is too large")
				return
			}
			x |= uint32(b) << shift
			return
		}
		x |= (uint32(b) & 0x7f) << shift
		shift += 7
	}
}
