// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"context"
	"fmt"
	"hash/crc64"
	"io"
	"os"
	"path"
	"runtime"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/types"
)

const (
	wasmRuntimeModule = "env"
)

type runtimeFunc struct {
	addr uint64
	sig  types.Function
}

type runtimeEnv struct {
	funcs map[string]runtimeFunc
}

func (env *runtimeEnv) init(config *Config, checksum io.Writer) (err error) {
	mapPath := path.Join(config.LibDir, "runtime.map")
	mapFile, err := os.Open(mapPath)
	if err != nil {
		return
	}
	defer mapFile.Close()

	fmt.Fprintln(checksum, runtime.GOARCH)
	fmt.Fprintln(checksum, wasmRuntimeModule)
	mapReader := io.TeeReader(mapFile, checksum)

	env.funcs = make(map[string]runtimeFunc)

	for {
		var (
			name string
			addr uint64
			n    int
		)

		n, err = fmt.Fscanf(mapReader, "%x T %s\n", &addr, &name)
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

		case "__gate_recv", "__gate_send", "__gate_debug_write":
			env.funcs[name] = runtimeFunc{addr, types.Function{
				Args: []types.T{types.I32, types.I32},
			}}
		}
	}

	return
}

func (env *runtimeEnv) ImportFunction(module, field string, sig types.Function,
) (variadic bool, addr uint64, err error) {
	if module == wasmRuntimeModule {
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

func (env *runtimeEnv) ImportGlobal(module, field string, t types.T,
) (value uint64, err error) {
	err = fmt.Errorf("imported global not found: %s %s %s", module, field, t)
	return
}

type Runtime struct {
	// EnvironmentChecksum value is the same for compatible Runtime instances.
	// A program compiled with one may be executed with another.  The checksum
	// may be used to invalidate or choose a cache.
	EnvironmentChecksum uint64

	env      runtimeEnv
	executor executor
}

func NewRuntime(ctx context.Context, config *Config) (rt *Runtime, err error) {
	rt = new(Runtime)

	checksum := crc64.New(crc64.MakeTable(crc64.ECMA))

	err = rt.env.init(config, checksum)
	if err != nil {
		return
	}

	rt.EnvironmentChecksum = checksum.Sum64()

	err = rt.executor.init(ctx, config)
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

func (rt *Runtime) acquireFiles(ctx context.Context, n int) error {
	return rt.executor.limiter.acquire(ctx, n)
}

func (rt *Runtime) releaseFiles(n int) {
	rt.executor.limiter.release(n)
}
