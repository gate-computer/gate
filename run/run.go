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
	"path"
	"syscall"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/reader"
	"github.com/tsavola/wag/sections"
	"github.com/tsavola/wag/traps"
	"github.com/tsavola/wag/types"
	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/internal/cred"
	"github.com/tsavola/gate/internal/memfd"
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

type runtimeFunc struct {
	addr uint64
	sig  types.Function
}

type runtimeEnv struct {
	funcs map[string]runtimeFunc
}

func (env *runtimeEnv) init(config *Config) (err error) {
	mapPath := path.Join(config.LibDir, "runtime.map")
	mapFile, err := os.Open(mapPath)
	if err != nil {
		return
	}
	defer mapFile.Close()

	env.funcs = make(map[string]runtimeFunc)

	for {
		var (
			name string
			addr uint64
			n    int
		)

		n, err = fmt.Fscanf(mapFile, "%x T %s\n", &addr, &name)
		if err != nil {
			if err == io.EOF && n == 0 {
				err = nil
				break
			}
			return
		}
		if n != 2 {
			err = fmt.Errorf("%s: parse error", mapPath)
			return
		}

		switch name {
		case "__gate_get_abi_version", "__gate_get_arg", "__gate_get_max_packet_size":
			env.funcs[name] = runtimeFunc{addr, types.Function{
				Result: types.I32,
			}}

		case "__gate_func_ptr":
			env.funcs[name] = runtimeFunc{addr, types.Function{
				Args:   []types.T{types.I32},
				Result: types.I32,
			}}

		case "__gate_exit":
			env.funcs[name] = runtimeFunc{addr, types.Function{
				Args: []types.T{types.I32},
			}}

		case "__gate_recv":
			env.funcs[name] = runtimeFunc{addr, types.Function{
				Args:   []types.T{types.I32, types.I32, types.I32},
				Result: types.I32,
			}}

		case "__gate_send", "__gate_debug_write":
			env.funcs[name] = runtimeFunc{addr, types.Function{
				Args: []types.T{types.I32, types.I32},
			}}
		}
	}

	return
}

func (env *runtimeEnv) ImportFunction(module, field string, sig types.Function) (variadic bool, addr uint64, err error) {
	if module == "env" {
		if f, found := env.funcs[field]; found {
			if !f.sig.Equal(sig) {
				err = fmt.Errorf("function %s %s imported with wrong signature: %s", field, f.sig, sig)
				return
			}

			addr = f.addr
			return
		}
	}

	err = fmt.Errorf("imported function not found: %s %s %s", module, field, sig)
	return
}

func (env *runtimeEnv) ImportGlobal(module, field string, t types.T) (value uint64, err error) {
	if module == "env" {
		switch field {
		case "__gate_abi_version":
			value = abiVersion
			return

		case "__gate_max_packet_size":
			value = maxPacketSize
			return
		}
	}

	err = fmt.Errorf("imported global not found: %s %s %s", module, field, t)
	return
}

type Runtime struct {
	env       runtimeEnv
	executor  executor
	commonGid uint
}

func NewRuntime(config *Config) (rt *Runtime, err error) {
	err = cred.ValidateId("group", config.CommonGid)
	if err != nil {
		return
	}

	err = checkCurrentGid(config.CommonGid)
	if err != nil {
		return
	}

	rt = &Runtime{
		commonGid: config.CommonGid,
	}

	err = rt.env.init(config)
	if err != nil {
		return
	}

	err = rt.executor.init(config)
	return
}

func (rt *Runtime) Close() error {
	return rt.executor.close()
}

// Done channel will be closed when the executor process dies.  If that wasn't
// requested by calling Close, this indicates an internal error.
func (rt *Runtime) Done() <-chan struct{} {
	return rt.executor.doneReceiving
}

func (rt *Runtime) Environment() wag.Environment {
	return &rt.env
}

// imageInfo is like the info object in loader.c
type imageInfo struct {
	TextAddr       uint64
	HeapAddr       uint64
	StackAddr      uint64
	PageSize       uint32
	RODataSize     uint32
	TextSize       uint32
	MemoryOffset   uint32
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

func (image *Image) Init() (err error) {
	mapsFd, err := memfd.Create("maps", memfd.CLOEXEC|memfd.ALLOW_SEALING)
	if err != nil {
		return
	}

	image.maps = os.NewFile(uintptr(mapsFd), "maps")
	return
}

func (image *Image) Close() (err error) {
	if image.maps == nil {
		return
	}

	err = image.maps.Close()
	image.maps = nil
	return
}

func (image *Image) Populate(m *wag.Module, growMemorySize wasm.MemorySize, stackSize int32) (err error) {
	initMemorySize, _ := m.MemoryLimits()

	if initMemorySize > growMemorySize {
		err = fmt.Errorf("initial memory size %d exceeds maximum memory size %d", initMemorySize, growMemorySize)
		return
	}

	roData := m.ROData()
	text := m.Text()
	data, memoryOffset := m.Data()

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

	_, err = image.maps.WriteAt(data, int64(roDataSize)+int64(textSize))
	if err != nil {
		return
	}

	globalsMemorySize := roundToPage(memoryOffset + int(growMemorySize))
	totalSize := int64(roDataSize) + int64(textSize) + int64(globalsMemorySize) + int64(stackSize)

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
		MemoryOffset:   uint32(memoryOffset),
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

	globalsMemorySize := image.info.MemoryOffset + image.info.GrowMemorySize
	dataSize := int(globalsMemorySize) + int(image.info.StackSize)

	data, err := syscall.Mmap(fd, dataMapOffset, dataSize, syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		panic(err)
	}
	defer syscall.Munmap(data)

	buf := data[:image.info.MemoryOffset]
	fmt.Fprintf(w, "--- GLOBALS (%d kB) ---\n", len(buf)/1024)
	for i := 0; len(buf) > 0; i += 8 {
		fmt.Fprintf(w, "%08x: %x\n", i, buf[0:8])
		buf = buf[8:]
	}

	buf = data[image.info.MemoryOffset : image.info.MemoryOffset+globalsMemorySize]
	fmt.Fprintf(w, "--- MEMORY (%d kB) ---\n", len(buf)/1024)
	for i := 0; len(buf) > 0; i += 32 {
		fmt.Fprintf(w, "%08x: %x %x %x %x\n", i, buf[0:8], buf[8:16], buf[16:24], buf[24:32])
		buf = buf[32:]
	}

	buf = data[globalsMemorySize:]
	fmt.Fprintf(w, "--- STACK (%d kB) ---\n", len(buf)/1024)
	for i := 0; len(buf) > 0; i += 32 {
		fmt.Fprintf(w, "%08x: %x %x %x %x\n", i, buf[0:8], buf[8:16], buf[16:24], buf[24:32])
		buf = buf[32:]
	}

	fmt.Fprintf(w, "---\n")
	return
}

func (image *Image) DumpStacktrace(w io.Writer, funcMap, callMap []byte, funcSigs []types.Function, ns *sections.NameSection) (err error) {
	fd := int(image.maps.Fd())

	offset := int64(image.info.RODataSize) + int64(image.info.TextSize) + int64(image.info.MemoryOffset) + int64(image.info.GrowMemorySize)

	size := int(image.info.StackSize)

	stack, err := syscall.Mmap(fd, offset, size, syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return
	}
	defer syscall.Munmap(stack)

	return writeStacktraceTo(w, image.info.TextAddr, stack, funcMap, callMap, funcSigs, ns)
}

type Process struct {
	process
	stdin  *os.File // writer
	stdout *os.File // reader
}

func (p *Process) Init(ctx context.Context, rt *Runtime, image *Image, debug io.Writer) (err error) {
	var (
		stdinR  *os.File
		stdinW  *os.File
		stdoutR *os.File
		stdoutW *os.File
		debugR  *os.File
		debugW  *os.File
	)

	defer func() {
		if stdinR != nil {
			stdinR.Close()
		}
		if stdinW != nil {
			stdinW.Close()
		}
		if stdoutR != nil {
			stdoutR.Close()
		}
		if stdoutW != nil {
			stdoutW.Close()
		}
		if debugR != nil {
			debugR.Close()
		}
		if debugW != nil {
			debugW.Close()
		}
	}()

	stdinR, stdinW, err = os.Pipe()
	if err != nil {
		return
	}

	err = syscall.Fchown(int(stdinR.Fd()), -1, int(rt.commonGid))
	if err != nil {
		return
	}

	err = syscall.Fchmod(int(stdinR.Fd()), 0640)
	if err != nil {
		return
	}

	stdoutR, stdoutW, err = os.Pipe()
	if err != nil {
		return
	}

	execFiles := execFiles{stdinR, stdoutW, image.maps}

	if debug != nil {
		debugR, debugW, err = os.Pipe()
		if err != nil {
			return
		}

		execFiles = append(execFiles, debugW)
	}

	err = rt.executor.execute(ctx, &p.process, execFiles)
	if err != nil {
		return
	}

	if debugR != nil {
		go copyClose(debug, debugR)
	}

	p.stdin = stdinW
	p.stdout = stdoutR

	stdinR = nil
	stdinW = nil
	stdoutR = nil
	stdoutW = nil
	debugR = nil
	debugW = nil
	return
}

func (p *Process) Close() (err error) {
	if p.stdin == nil {
		return
	}

	p.process.kill()
	p.stdin.Close()
	p.stdout.Close()

	p.stdin = nil
	p.stdout = nil
	return
}

type execFiles []*os.File // stdin stdout maps [debug]

func (files execFiles) close() {
	files[0].Close() // stdin
	files[1].Close() // stdout

	// don't close maps

	if len(files) > 3 {
		files[3].Close() // debug
	}
}

func copyClose(w io.Writer, r *os.File) {
	defer r.Close()
	io.Copy(w, r)
}

func Load(m *wag.Module, r reader.Reader, rt *Runtime, textBuf wag.Buffer, roDataBuf []byte, startTrigger chan<- struct{}) error {
	m.MainSymbol = MainSymbol
	return m.Load(r, rt.Environment(), textBuf, roDataBuf, RODataAddr, startTrigger)
}

func Run(ctx context.Context, rt *Runtime, proc *Process, image *Image, services ServiceRegistry) (exit int, trap traps.Id, err error) {
	if services == nil {
		services = noServices{}
	}

	err = binary.Write(proc.stdin, endian, &image.info)
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

		if n := code - 100; n >= 0 && n < int(traps.NumTraps) {
			trap = traps.Id(n)
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

func (inst *Instance) Init(ctx context.Context, rt *Runtime, debug io.Writer) (err error) {
	err = inst.Image.Init()
	if err != nil {
		return
	}

	closeImage := true
	defer func() {
		if closeImage {
			inst.Image.Close()
		}
	}()

	err = inst.proc.Init(ctx, rt, &inst.Image, debug)
	if err != nil {
		return
	}

	closeImage = false
	return
}

func (inst *Instance) Close() {
	inst.Image.Close()
	inst.proc.Close()
}

func (inst *Instance) Run(ctx context.Context, rt *Runtime, services ServiceRegistry) (exit int, trap traps.Id, err error) {
	return Run(ctx, rt, &inst.proc, &inst.Image, services)
}
