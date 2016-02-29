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

func Run(executorPath, loaderPath string, program *elf.File, memorySize int) (err error) {
	var (
		textSect   dataSection
		rodataSect dataSection
		dataSect   dataSection
		bssSect    section
		tbssSize   uint64
		gateSect   dataSection
	)

	for _, s := range program.Sections {
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

	symbols, err := program.Symbols()
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

	fmt.Printf("_start addr          0x%07x\n", startAddr)
	fmt.Printf(".text addr           0x%07x\n", textSect.addr)
	fmt.Printf(".text addr aligned   0x%07x\n", alignedTextAddr)
	fmt.Printf(".text end            0x%07x\n", textSect.addr+textSect.size)
	fmt.Printf(".text end aligned    0x%07x\n", alignedTextAddr+alignedTextSize)
	fmt.Printf(".rodata addr         0x%07x\n", rodataSect.addr)
	fmt.Printf(".rodata end          0x%07x\n", rodataSect.addr+rodataSect.size)
	fmt.Printf(".rodata end aligned  0x%07x\n", alignedRodataAddr+alignedRodataSize)
	fmt.Printf(".data addr           0x%07x\n", dataSect.addr)
	fmt.Printf(".data end            0x%07x\n", dataSect.addr+dataSect.size)
	fmt.Printf(".bss addr            0x%07x\n", bssSect.addr)
	fmt.Printf(".bss end             0x%07x\n", bssSect.addr+bssSect.size)
	fmt.Printf(".bss end aligned     0x%07x\n", dataSect.addr+alignedProgramSize)
	fmt.Printf(".gate addr           0x%07x\n", gateSect.addr)
	fmt.Printf(".text size           %9d\n", textSect.size)
	fmt.Printf(".text size aligned   %9d\n", alignedTextSize)
	fmt.Printf(".rodata size         %9d\n", rodataSect.size)
	fmt.Printf(".rodata size aligned %9d\n", alignedRodataSize)
	fmt.Printf(".data size           %9d\n", dataSect.size)
	fmt.Printf(".bss size            %9d\n", bssSect.size)
	fmt.Printf(".tbss size           %9d\n", tbssSize)
	fmt.Printf(".gate size           %9d\n", gateSect.size)
	fmt.Printf("program size aligned %9d\n", alignedProgramSize)

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

	fmt.Printf("heap size aligned    %9d\n", alignedHeapSize)
	fmt.Printf("indirect funcs count %5d + 3\n", indirectFuncsSize / 4)

	info := []uint64{
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
	}

	cmd := exec.Cmd{
		Path: executorPath,
		Args: []string{executorPath, loaderPath},
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

	err = writeProgram(stdin, info, textSect.data, rodataSect.data, dataSect.data, indirectFuncArray)
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

func writeProgram(w io.Writer, info []uint64, sections ...[]byte) (err error) {
	err = binary.Write(w, nativeEndian, info)
	if err != nil {
		return
	}

	for _, s := range sections {
		_, err = w.Write(s)
		if err != nil {
			return
		}
	}

	return
}

func dumpOutput(r io.Reader) {
	data, _ := ioutil.ReadAll(r)
	fmt.Printf("%v\n", data)
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
