// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/tsavola/wag/callmap"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/section"
	"github.com/tsavola/wag/trap"
	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/internal/memfd"
	"github.com/tsavola/gate/internal/publicerror"
)

var (
	pageSize = os.Getpagesize()
)

func roundToPage(size int) uint32 {
	mask := uint32(pageSize) - 1
	return (uint32(size) + mask) &^ mask
}

// checkCurrentGid makes sure that this process belongs to gid.
func checkCurrentGid(gid uint) (err error) {
	currentGroups, err := syscall.Getgroups()
	if err != nil {
		return
	}

	currentGroups = append(currentGroups, syscall.Getgid())

	for _, currentGid := range currentGroups {
		if uint(currentGid) == gid {
			return
		}
	}

	err = fmt.Errorf("this process does not belong to group %d", gid)
	return
}

func randAddrs() (textAddr, heapAddr, stackAddr uint64) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	textAddr = randAddr(minTextAddr, maxTextAddr, b[0:4])
	heapAddr = randAddr(minHeapAddr, maxHeapAddr, b[4:8])
	stackAddr = randAddr(minStackAddr, maxStackAddr, b[8:12])
	return
}

func randAddr(minAddr, maxAddr uint64, b []byte) uint64 {
	minPage := minAddr / uint64(pageSize)
	maxPage := maxAddr / uint64(pageSize)
	page := minPage + uint64(endian.Uint32(b))%(maxPage-minPage)
	return page * uint64(pageSize)
}

// imageInfo is like the info object in loader.c
type imageInfo struct {
	TextAddr       uint64
	HeapAddr       uint64
	StackAddr      uint64
	PageSize       uint32
	RODataSize     uint32
	TextSize       uint32
	GlobalsSize    uint32
	InitMemorySize uint32
	GrowMemorySize uint32
	StackSize      uint32
	MagicNumber    uint32
	Arg            int32
}

type Image struct {
	maps *os.File
	info imageInfo
}

func (image *Image) Init(ctx context.Context, rt *Runtime) (err error) {
	numFiles := 1
	err = rt.acquireFiles(ctx, numFiles)
	if err != nil {
		return
	}
	defer func() {
		rt.releaseFiles(numFiles)
	}()

	err = image.init(ctx, rt)
	if err != nil {
		return
	}

	numFiles = 0
	return
}

func (image *Image) init(ctx context.Context, rt *Runtime) (err error) {
	mapsFd, err := memfd.Create("maps", memfd.CLOEXEC|memfd.ALLOW_SEALING)
	if err != nil {
		return
	}

	image.maps = os.NewFile(uintptr(mapsFd), "maps")
	return
}

func (image *Image) Release(rt *Runtime) (err error) {
	if image.maps == nil {
		return
	}

	err = image.maps.Close()
	image.maps = nil

	rt.releaseFiles(1)
	return
}

func (image *Image) Populate(m *compile.Module, growMemorySize wasm.MemorySize, stackSize int32,
) (err error) {
	initMemorySize, _ := m.MemoryLimits()

	if initMemorySize > growMemorySize {
		err = publicerror.Errorf("initial memory size %d exceeds maximum memory size %d", initMemorySize, growMemorySize)
		return
	}

	roData := m.ROData()
	text := m.Text()
	data, globalsDataSize := m.Data()

	_, err = image.maps.Write(roData)
	if err != nil {
		return
	}

	roDataSize := roundToPage(len(roData))

	_, err = image.maps.WriteAt(text, int64(roDataSize))
	if err != nil {
		return
	}

	textSize := roundToPage(len(text))
	globalsMapSize := roundToPage(globalsDataSize)
	globalsDataOffset := int(globalsMapSize) - globalsDataSize

	_, err = image.maps.WriteAt(data, int64(roDataSize)+int64(textSize)+int64(globalsDataOffset))
	if err != nil {
		return
	}

	totalSize := int64(roDataSize) + int64(textSize) + int64(globalsMapSize) + int64(growMemorySize) + int64(stackSize)

	err = image.maps.Truncate(totalSize)
	if err != nil {
		return
	}

	_, err = memfd.Fcntl(int(image.maps.Fd()), memfd.F_ADD_SEALS, memfd.F_SEAL_SHRINK|memfd.F_SEAL_GROW)
	if err != nil {
		return
	}

	textAddr, heapAddr, stackAddr := randAddrs()

	image.info = imageInfo{
		TextAddr:       textAddr,
		HeapAddr:       heapAddr,
		StackAddr:      stackAddr,
		PageSize:       uint32(pageSize),
		RODataSize:     roDataSize,
		TextSize:       textSize,
		GlobalsSize:    uint32(globalsMapSize),
		InitMemorySize: uint32(initMemorySize),
		GrowMemorySize: uint32(growMemorySize),
		StackSize:      uint32(stackSize),
		MagicNumber:    magicNumber,
		Arg:            image.info.Arg, // in case SetArg was called before this
	}
	return
}

func (image *Image) SetArg(arg int32) {
	image.info.Arg = arg
}

func (image *Image) DumpGlobalsMemoryStack(w io.Writer) (err error) {
	fd := int(image.maps.Fd())

	dataMapOffset := int64(image.info.RODataSize) + int64(image.info.TextSize)
	dataSize := int(image.info.GlobalsSize) + int(image.info.GrowMemorySize) + int(image.info.StackSize)

	data, err := syscall.Mmap(fd, dataMapOffset, dataSize, syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return
	}
	defer syscall.Munmap(data)

	buf := data[:image.info.GlobalsSize]
	fmt.Fprintf(w, "--- GLOBALS (%d kB) ---\n", len(buf)/1024)
	for i := 0; len(buf) > 0; i += 8 {
		fmt.Fprintf(w, "%08x: %016x\n", i, endian.Uint64(buf[0:8]))
		buf = buf[8:]
	}

	buf = data[image.info.GlobalsSize : image.info.GlobalsSize+image.info.GrowMemorySize]
	fmt.Fprintf(w, "--- MEMORY (%d kB) ---\n", len(buf)/1024)
	for i := 0; len(buf) > 0; i += 32 {
		fmt.Fprintf(w, "%08x: %016x %016x %016x %016x\n", i, endian.Uint64(buf[0:8]), endian.Uint64(buf[8:16]), endian.Uint64(buf[16:24]), endian.Uint64(buf[24:32]))
		buf = buf[32:]
	}

	buf = data[image.info.GlobalsSize+image.info.GrowMemorySize:]
	fmt.Fprintf(w, "--- STACK (%d kB) ---\n", len(buf)/1024)
	for i := 0; len(buf) > 0; i += 32 {
		fmt.Fprintf(w, "%08x: %016x %016x %016x %016x\n", i, endian.Uint64(buf[0:8]), endian.Uint64(buf[8:16]), endian.Uint64(buf[16:24]), endian.Uint64(buf[24:32]))
		buf = buf[32:]
	}

	fmt.Fprintf(w, "---\n")
	return
}

func (image *Image) DumpStacktrace(w io.Writer, m *compile.Module, mapping *callmap.Map, ns *section.NameSection,
) (err error) {
	fd := int(image.maps.Fd())

	offset := int64(image.info.RODataSize) + int64(image.info.TextSize) + int64(image.info.GlobalsSize) + int64(image.info.GrowMemorySize)
	size := int(image.info.StackSize)

	stack, err := syscall.Mmap(fd, offset, size, syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return
	}
	defer syscall.Munmap(stack)

	return writeStacktraceTo(w, image.info.TextAddr, stack, m, mapping.FuncAddrs, mapping.CallSites, ns)
}

type Process struct {
	process
	writer *os.File
	reader *os.File
}

func (p *Process) Init(ctx context.Context, rt *Runtime, image *Image, debug io.Writer,
) (err error) {
	numFiles := 4
	if debug != nil {
		numFiles += 2
	}

	err = rt.acquireFiles(ctx, numFiles)
	if err != nil {
		return
	}
	defer func() {
		rt.releaseFiles(numFiles)
	}()

	err = p.init(ctx, rt, image, debug)
	if err != nil {
		return
	}

	numFiles = 0
	return
}

func (p *Process) init(ctx context.Context, rt *Runtime, image *Image, debug io.Writer,
) (err error) {
	var (
		inputR  *os.File
		inputW  *os.File
		outputR *os.File
		outputW *os.File
		debugR  *os.File
		debugW  *os.File
	)

	defer func() {
		if inputR != nil {
			inputR.Close()
		}
		if inputW != nil {
			inputW.Close()
		}
		if outputR != nil {
			outputR.Close()
		}
		if outputW != nil {
			outputW.Close()
		}
		if debugR != nil {
			debugR.Close()
		}
		if debugW != nil {
			debugW.Close()
		}
	}()

	inputR, inputW, err = os.Pipe()
	if err != nil {
		return
	}

	outputR, outputW, err = os.Pipe()
	if err != nil {
		return
	}

	if debug != nil {
		debugR, debugW, err = os.Pipe()
		if err != nil {
			return
		}
	}

	err = rt.executor.execute(ctx, &p.process, &execFiles{inputR, outputW, image.maps, debugW})
	if err != nil {
		return
	}

	if debug != nil {
		go copyCloseRelease(rt, debug, debugR)
	}

	p.writer = inputW
	p.reader = outputR

	inputR = nil
	inputW = nil
	outputR = nil
	outputW = nil
	debugR = nil
	debugW = nil
	return
}

func (p *Process) Kill(rt *Runtime) {
	if p.writer == nil {
		return
	}

	p.process.kill()
	p.writer.Close()
	p.reader.Close()

	p.writer = nil
	p.reader = nil

	rt.releaseFiles(2)
	return
}

type execFiles struct {
	input  *os.File
	output *os.File
	maps   *os.File // Borrowed
	debug  *os.File // Optional
}

func (files *execFiles) fds() (fds []int) {
	if files.debug == nil {
		fds = make([]int, 3)
	} else {
		fds = make([]int, 4)
	}

	fds[0] = int(files.input.Fd())
	fds[1] = int(files.output.Fd())
	fds[2] = int(files.maps.Fd())

	if files.debug != nil {
		fds[3] = int(files.debug.Fd())
	}
	return
}

func (files *execFiles) release(limiter FileLimiter) {
	numFiles := 2
	files.input.Close()
	files.output.Close()

	// don't close maps

	if files.debug != nil {
		numFiles++
		files.debug.Close()
	}

	limiter.release(numFiles)
}

func copyCloseRelease(rt *Runtime, w io.Writer, r *os.File) {
	defer rt.releaseFiles(1)
	defer r.Close()

	io.Copy(w, r)
}

// InitImageAndProcess is otherwise same as Image.Init() + Process.Init(), but
// avoids deadlocks by allocating all required file descriptors in a single
// step.
func InitImageAndProcess(ctx context.Context, rt *Runtime, image *Image, proc *Process, debug io.Writer,
) (err error) {
	numFiles := 5
	if debug != nil {
		numFiles += 2
	}

	err = rt.acquireFiles(ctx, numFiles)
	if err != nil {
		return
	}
	defer func() {
		rt.releaseFiles(numFiles)
	}()

	err = image.init(ctx, rt)
	if err != nil {
		return
	}

	err = proc.init(ctx, rt, image, debug)
	if err != nil {
		return
	}

	numFiles = 0
	return
}

func Load(m *compile.Module, r compile.Reader, rt *Runtime, textBuf compile.TextBuffer, roDataBuf compile.DataBuffer, mapper compile.Mapper,
) error {
	m.EntrySymbol = EntrySymbol
	return m.Load(r, rt.Env(), textBuf, roDataBuf, RODataAddr, nil, mapper)
}

func Run(ctx context.Context, rt *Runtime, proc *Process, image *Image, services ServiceRegistry,
) (exit int, trapId trap.Id, err error) {
	if services == nil {
		services = noServices{}
	}

	err = binary.Write(proc.writer, endian, &image.info)
	if err != nil {
		return
	}

	err = ioLoop(ctx, services, proc)
	if err != nil {
		return
	}

	status, err := proc.killWait()
	if err != nil {
		return
	}

	switch {
	case status.Exited():
		code := status.ExitStatus()

		switch code {
		case 0, 1:
			exit = code
			return
		}

		if n := code - 100; n >= 0 && n < int(trap.NumTraps) {
			trapId = trap.Id(n)
			return
		}

		err = fmt.Errorf("process exit code: %d", code)
		return

	case status.Signaled():
		err = fmt.Errorf("process termination signal: %d", status.Signal())
		return

	default:
		err = fmt.Errorf("unknown process status: %d", status)
		return
	}
}

type Instance struct {
	Image

	proc Process
}

func (inst *Instance) Init(ctx context.Context, rt *Runtime, debug io.Writer,
) error {
	return InitImageAndProcess(ctx, rt, &inst.Image, &inst.proc, debug)
}

func (inst *Instance) Kill(rt *Runtime) (err error) {
	err = inst.Image.Release(rt)
	inst.proc.Kill(rt)
	return
}

func (inst *Instance) Run(ctx context.Context, rt *Runtime, services ServiceRegistry,
) (exit int, trapId trap.Id, err error) {
	return Run(ctx, rt, &inst.proc, &inst.Image, services)
}
