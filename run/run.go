package run

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"sort"
)

var (
	pageSize     uint64
	pageSizeMask uint64
)

func init() {
	pageSize = uint64(os.Getpagesize())
	pageSizeMask = pageSize - 1
}

func truncateToPage(addr uint64) uint64 {
	return addr &^ pageSizeMask
}

func roundToPage(size uint64) uint64 {
	return (size + pageSizeMask) &^ pageSizeMask
}

type section struct {
	addr uint64
	size uint64
}

func (s *section) init(es *elf.Section) {
	s.addr = es.Addr
	s.size = es.Size
}

type dataSection struct {
	section
	data []byte
}

func (ds *dataSection) init(es *elf.Section) (err error) {
	ds.section.init(es)
	ds.data, err = es.Data()
	return
}

type Payload struct {
	info  []uint64
	parts [][]byte
}

func NewPayload(elfFile *elf.File, memorySize int) (payload *Payload, err error) {
	var (
		textSect   dataSection
		rodataSect dataSection
		dataSect   dataSection
		bssSect    section
		tbssSize   uint64
		gateSect   dataSection
	)

	for _, s := range elfFile.Sections {
		switch s.Name {
		case ".text":
			err = textSect.init(s)
		case ".rodata":
			err = rodataSect.init(s)
		case ".gate":
			err = gateSect.init(s)
		case ".bss":
			bssSect.init(s)
		case ".tbss":
			tbssSize = s.Size
		case ".data":
			err = dataSect.init(s)
		}
		if err != nil {
			return
		}
	}

	var (
		unsafeStackPtrOffset  = uint64(0x10000)
		indirectCallCheckAddr = uint64(0x100000000)
		argsAddr              = uint64(0x100000000)
		startAddr             = uint64(0x100000000)
	)

	symbols, err := elfFile.Symbols()
	if err != nil {
		return
	}

	for _, s := range symbols {
		switch s.Name {
		case "__safestack_unsafe_stack_ptr":
			unsafeStackPtrOffset = s.Value
		case "__gate_indirect_call_check":
			indirectCallCheckAddr = s.Value
		case "__gate_args":
			argsAddr = s.Value
		case "_start":
			startAddr = s.Value
		}
	}

	// TODO: bounds checking
	if unsafeStackPtrOffset >= 0x10000 {
		panic("__safestack_unsafe_stack_ptr symbol not found")
	}
	if indirectCallCheckAddr >= 0xffffe000 {
		panic("__gate_indirect_call_check symbol not found")
	}
	if argsAddr >= 0xffffe000 {
		panic("__gate_args symbol not found")
	}
	if startAddr >= 0xffffe000 {
		panic("_start symbol not found")
	}

	if rodataSect.size == 0 {
		rodataSect.addr = textSect.addr + textSect.size
	}
	if dataSect.size == 0 {
		dataSect.addr = rodataSect.addr + rodataSect.size
	}
	if bssSect.size == 0 {
		bssSect.addr = dataSect.addr + dataSect.size
	}

	if !(textSect.addr <= rodataSect.addr && rodataSect.addr <= dataSect.addr && dataSect.addr <= bssSect.addr) {
		panic("unexpected section positioning")
	}

	alignedTextAddr := truncateToPage(textSect.addr)
	alignedTextSize := roundToPage(textSect.addr - alignedTextAddr + textSect.size)
	alignedRodataAddr := truncateToPage(rodataSect.addr)
	alignedRodataSize := roundToPage(rodataSect.addr - alignedRodataAddr + rodataSect.size)
	alignedDataAddr := truncateToPage(dataSect.addr)
	alignedProgramSize := roundToPage(bssSect.addr - alignedTextAddr + bssSect.size)
	alignedMemorySize := roundToPage(uint64(memorySize))

	if rodataSect.size == 0 {
		alignedRodataAddr = alignedTextAddr + alignedTextSize
		alignedRodataSize = 0
	}
	if dataSect.size == 0 {
		alignedDataAddr = alignedRodataAddr + alignedRodataSize
	}

	fmt.Fprintf(os.Stderr, "_start addr          0x%07x\n", startAddr)
	fmt.Fprintf(os.Stderr, ".text addr           0x%07x\n", textSect.addr)
	fmt.Fprintf(os.Stderr, ".text addr aligned   0x%07x\n", alignedTextAddr)
	fmt.Fprintf(os.Stderr, ".text end            0x%07x\n", textSect.addr+textSect.size)
	fmt.Fprintf(os.Stderr, ".text end aligned    0x%07x\n", alignedTextAddr+alignedTextSize)
	fmt.Fprintf(os.Stderr, ".rodata addr         0x%07x\n", rodataSect.addr)
	fmt.Fprintf(os.Stderr, ".rodata end          0x%07x\n", rodataSect.addr+rodataSect.size)
	fmt.Fprintf(os.Stderr, ".rodata end aligned  0x%07x\n", alignedRodataAddr+alignedRodataSize)
	fmt.Fprintf(os.Stderr, ".data addr           0x%07x\n", dataSect.addr)
	fmt.Fprintf(os.Stderr, ".data end            0x%07x\n", dataSect.addr+dataSect.size)
	fmt.Fprintf(os.Stderr, ".bss addr            0x%07x\n", bssSect.addr)
	fmt.Fprintf(os.Stderr, ".bss end             0x%07x\n", bssSect.addr+bssSect.size)
	fmt.Fprintf(os.Stderr, ".bss end aligned     0x%07x\n", dataSect.addr+alignedProgramSize)
	fmt.Fprintf(os.Stderr, ".gate addr           0x%07x\n", gateSect.addr)
	fmt.Fprintf(os.Stderr, ".text size           %9d\n", textSect.size)
	fmt.Fprintf(os.Stderr, ".text size aligned   %9d\n", alignedTextSize)
	fmt.Fprintf(os.Stderr, ".rodata size         %9d\n", rodataSect.size)
	fmt.Fprintf(os.Stderr, ".rodata size aligned %9d\n", alignedRodataSize)
	fmt.Fprintf(os.Stderr, ".data size           %9d\n", dataSect.size)
	fmt.Fprintf(os.Stderr, ".bss size            %9d\n", bssSect.size)
	fmt.Fprintf(os.Stderr, ".tbss size           %9d\n", tbssSize)
	fmt.Fprintf(os.Stderr, ".gate size           %9d\n", gateSect.size)
	fmt.Fprintf(os.Stderr, "program size aligned %9d\n", alignedProgramSize)

	if !(alignedTextAddr+alignedTextSize <= alignedRodataAddr && alignedRodataAddr+alignedRodataSize <= alignedDataAddr) {
		panic("overlapping section pages")
	}

	if alignedProgramSize > alignedMemorySize {
		err = errors.New("program exceeds memory size")
		return
	}

	// TODO: use the actual symbol
	indirectFuncArray := binaryUint64ToUint32Inplace(gateSect.data)
	sort.Sort(binaryUint32Order(indirectFuncArray))
	indirectFuncsSize := uint64(len(indirectFuncArray))

	alignedHeapSize := alignedMemorySize - alignedProgramSize

	fmt.Fprintf(os.Stderr, "heap size aligned    %9d\n", alignedHeapSize)
	fmt.Fprintf(os.Stderr, "indirect funcs count %5d + 3\n", indirectFuncsSize/4)

	payload = &Payload{
		info: []uint64{
			pageSize,
			textSect.addr,
			textSect.size,
			alignedTextAddr,
			alignedTextSize,
			rodataSect.addr,
			rodataSect.size,
			alignedRodataAddr,
			alignedRodataSize,
			dataSect.addr,
			dataSect.size,
			alignedProgramSize,
			indirectFuncsSize,
			roundToPage(indirectFuncsSize + 3*4),
			alignedHeapSize,
			tbssSize,
			unsafeStackPtrOffset,
			indirectCallCheckAddr,
			argsAddr,
			startAddr,
		},
		parts: [][]byte{
			textSect.data,
			rodataSect.data,
			dataSect.data,
			indirectFuncArray,
		},
	}
	return
}

func (payload *Payload) WriteTo(w io.Writer) (n int64, err error) {
	err = binary.Write(w, nativeEndian, payload.info)
	if err != nil {
		// n may be wrong
		return
	}

	n += 8 * int64(len(payload.info))

	for _, part := range payload.parts {
		var m int

		m, err = w.Write(part)
		if err != nil {
			return
		}

		n += int64(m)
	}

	return
}

func binaryUint64ToUint32Inplace(buf []byte) []byte {
	r := bytes.NewReader(buf)
	w := bytes.NewBuffer(buf[:0])

	for {
		var element uint64

		if err := binary.Read(r, nativeEndian, &element); err != nil {
			if err == io.EOF {
				return buf[:len(buf)/2]
			}

			panic(err)
		}

		binary.Write(w, nativeEndian, uint32(element))
	}
}

type binaryUint32Order []byte

func (b binaryUint32Order) Len() int {
	return len(b) / 4
}

func (b binaryUint32Order) Swap(i, j int) {
	var tmp [4]byte
	copy(tmp[:4], b[i*4:i*4+4])
	copy(b[i*4:i*4+4], b[j*4:j*4+4])
	copy(b[j*4:j*4+4], tmp[:4])
}

func (b binaryUint32Order) Less(i, j int) bool {
	l := nativeEndian.Uint32(b[i*4 : i*4+4])
	r := nativeEndian.Uint32(b[j*4 : j*4+4])
	return l < r
}

func Run(executorBin, loaderBin string, payload *Payload) (err error) {
	cmd := exec.Cmd{
		Path: executorBin,
		Args: []string{executorBin, loaderBin},
		Env:  []string{},
		Dir:  "/",
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return
	}

	err = cmd.Start()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return
	}

	_, err = payload.WriteTo(stdin)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return
	}

	dumpOutput(stdout)

	err = cmd.Wait()
	if err != nil {
		return
	}

	if !cmd.ProcessState.Success() {
		err = errors.New(cmd.ProcessState.String())
	}
	return
}

func dumpOutput(r io.Reader) {
	data, _ := ioutil.ReadAll(r)
	fmt.Fprintf(os.Stderr, "%v\n", data)
}
