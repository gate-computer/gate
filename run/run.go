package run

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/types"
	"github.com/tsavola/wag/wasm"
)

var (
	pageSize = uint32(os.Getpagesize())
)

type envFunc struct {
	addr uint64
	sig  types.Function
}

type Environment struct {
	executorBin string
	loaderBin   string

	funcs map[string]envFunc
}

func NewEnvironment(executorBin, loaderBin string) (env *Environment, err error) {
	env = &Environment{
		executorBin: executorBin,
		loaderBin:   loaderBin,
		funcs:       make(map[string]envFunc),
	}

	f, err := elf.Open(loaderBin)
	if err != nil {
		return
	}
	defer f.Close()

	symbols, err := f.Symbols()
	if err != nil {
		return
	}

	for _, s := range symbols {
		switch s.Name {
		case "__gate_get_abi_version", "__gate_get_max_packet_size":
			env.funcs[s.Name] = envFunc{s.Value, types.Function{
				Result: types.I32,
			}}

		case "__gate_func_ptr":
			env.funcs[s.Name] = envFunc{s.Value, types.Function{
				Args:   []types.T{types.I32},
				Result: types.I32,
			}}

		case "__gate_exit":
			env.funcs[s.Name] = envFunc{s.Value, types.Function{
				Args: []types.T{types.I32},
			}}

		case "__gate_recv_full":
			env.funcs[s.Name] = envFunc{s.Value, types.Function{
				Args:   []types.T{types.I32, types.I32},
				Result: types.I32,
			}}

		case "__gate_send_full":
			env.funcs[s.Name] = envFunc{s.Value, types.Function{
				Args: []types.T{types.I32, types.I32},
			}}
		}
	}

	return
}

func (env *Environment) ImportFunction(module, field string, sig types.Function) (variadic bool, addr uint64, err error) {
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

func (env *Environment) ImportGlobal(module, field string, t types.T) (value uint64, err error) {
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

type Payload struct {
	info  []uint32
	parts [][]byte
}

func NewPayload(m *wag.Module, growMemorySize wasm.MemorySize, stackSize int32) (payload *Payload, err error) {
	initMemorySize, _ := m.MemoryLimits()

	if initMemorySize > growMemorySize {
		err = fmt.Errorf("initial memory size %d exceeds maximum memory size %d", initMemorySize, growMemorySize)
		return
	}

	roData := m.ROData()
	text := m.Text()
	data, memoryOffset := m.Data()

	memory := data[memoryOffset:]
	binary.LittleEndian.PutUint32(data[4:], uint32(len(memory))) // stack ptr?

	payload = &Payload{
		info: []uint32{
			pageSize,
			uint32(len(roData)),
			uint32(len(text)),
			uint32(len(data)),
			uint32(memoryOffset),
			uint32(initMemorySize),
			uint32(growMemorySize),
			uint32(stackSize),
		},
		parts: [][]byte{
			roData,
			text,
			data,
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

func Run(env *Environment, payload *Payload) (output []byte, err error) {
	cmd := exec.Cmd{
		Path: env.executorBin,
		Args: []string{env.executorBin, env.loaderBin},
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

	output, _ = ioutil.ReadAll(stdout)

	err = cmd.Wait()
	if err != nil {
		return
	}

	if !cmd.ProcessState.Success() {
		err = errors.New(cmd.ProcessState.String())
	}
	return
}
