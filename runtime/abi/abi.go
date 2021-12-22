// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package abi

//go:generate go run ../../cmd/gate-librarian -v -go=abi library.go -- library/compile.sh -o /dev/stdout

import (
	"bytes"
	"errors"
	"strconv"
	"strings"

	"gate.computer/gate/internal/error/badprogram"
	"gate.computer/wag/compile"
	"gate.computer/wag/wa"
)

const (
	maxPacketSize = 65536
)

// Mirrors the vector initialization in runtime/loader/loader.c
var rtFunctions = map[string]map[string]int{
	"env": {
		"rt_write8": -19, // Public ABI.
		"rt_read8":  -18, // Public ABI.
		"rt_trap":   -17, // Public ABI.
		"rt_debug":  -16, // Public ABI.
		"rt_write":  -15,
		"rt_read":   -14,
		"rt_poll":   -13,
		"rt_time":   -12,
		"rt_random": -6,
	},
}

func LibraryChecksum() uint64 {
	return libraryChecksum
}

func Library() compile.Library {
	return library
}

var library = func() compile.Library {
	r := bytes.NewReader(libraryWASM[:])

	mod, err := compile.LoadInitialSections(nil, r)
	if err != nil {
		panic(err)
	}

	lib, err := mod.AsLibrary()
	if err != nil {
		panic(err)
	}

	for i := 0; i < lib.NumImportFuncs(); i++ {
		module, field, _ := lib.ImportFunc(i)
		index := rtFunctions[module][field]
		if index == 0 {
			panic(index)
		}
		lib.SetImportFunc(i, index)
	}

	if err := lib.LoadSections(r); err != nil {
		panic(err)
	}

	return lib
}()

type libraryFunction struct {
	Index uint32
	Type  wa.FuncType
}

var gateFunctions = map[string]libraryFunction{
	"fd": library_fd,
	"io": library_io,
}

var wasiFunctions = map[string]libraryFunction{
	"args_get":                library_args_get,
	"args_sizes_get":          library_args_sizes_get,
	"clock_res_get":           library_clock_res_get,
	"clock_time_get":          library_clock_time_get,
	"environ_get":             library_environ_get,
	"environ_sizes_get":       library_environ_sizes_get,
	"fd_advise":               library_stub_fd_i64_i64_i32,
	"fd_allocate":             library_stub_fd_i64_i64,
	"fd_close":                library_fd_close,
	"fd_datasync":             library_stub_fd,
	"fd_fdstat_get":           library_fd_fdstat_get,
	"fd_fdstat_set_flags":     library_stub_fd_i32,
	"fd_fdstat_set_rights":    library_fd_fdstat_set_rights,
	"fd_filestat_get":         library_stub_fd_i32,
	"fd_filestat_set_size":    library_stub_fd_i64,
	"fd_filestat_set_times":   library_stub_fd_i64_i64_i32,
	"fd_pread":                library_stub_fd_i32_i32_i64_i32,
	"fd_prestat_dir_name":     library_fd_prestat_dir_name,
	"fd_prestat_get":          library_stub_fd_i32,
	"fd_pwrite":               library_stub_fd_i32_i32_i64_i32,
	"fd_read":                 library_fd_read,
	"fd_readdir":              library_stub_fd_i32_i32_i64_i32,
	"fd_renumber":             library_fd_renumber,
	"fd_seek":                 library_stub_fd_i64_i32_i32,
	"fd_sync":                 library_stub_fd,
	"fd_tell":                 library_stub_fd_i32,
	"fd_write":                library_fd_write,
	"path_create_directory":   library_stub_fd_i32_i32,
	"path_filestat_get":       library_stub_fd_i32_i32_i32_i32,
	"path_filestat_set_times": library_stub_fd_i32_i32_i32_i64_i64_i32,
	"path_link":               library_stub_fd_i32_i32_i32_fd_i32_i32,
	"path_open":               library_stub_fd_i32_i32_i32_i32_i64_i64_i32_i32,
	"path_readlink":           library_stub_fd_i32_i32_i32_i32_i32,
	"path_remove_directory":   library_stub_fd_i32_i32,
	"path_rename":             library_stub_fd_i32_i32_fd_i32_i32,
	"path_symlink":            library_stub_i32_i32_fd_i32_i32,
	"path_unlink_file":        library_stub_fd_i32_i32,
	"poll_oneoff":             library_poll_oneoff,
	"proc_exit":               library_proc_exit,
	"proc_raise":              library_proc_raise,
	"random_get":              library_random_get,
	"sched_yield":             library_sched_yield,
	"sock_recv":               library_sock_recv,
	"sock_send":               library_sock_send,
	"sock_shutdown":           library_stub_fd_i32,
}

type ImportResolver struct {
	Random bool
}

func (ir *ImportResolver) ResolveFunc(module, field string, sig wa.FuncType) (index uint32, err error) {
	m := module
	f := field

	if m == "env" {
		switch {
		case strings.HasPrefix(f, "__gate_"):
			m = "gate"
			f = f[7:]

		case strings.HasPrefix(f, "__wasi_"):
			m = "wasi_snapshot_preview1"
			f = f[7:]
		}
	}

	var (
		libfn libraryFunction
		found bool
	)

	switch m {
	case "gate":
		i := strings.LastIndexByte(f, '_')
		if i <= 0 {
			break
		}

		prefix := f[:i]
		suffix := f[i+1:]

		libfn, found = gateFunctions[prefix]
		if found {
			if strings.HasPrefix(suffix, "0") {
				return 0, badprogram.Errorf("invalid size suffix in symbol: %s", f)
			}
			if n, e := strconv.ParseUint(suffix, 10, 32); e != nil {
				if !errors.Is(e, strconv.ErrRange) || !allDigits(suffix) {
					return 0, badprogram.Errorf("invalid size suffix in symbol: %s", f)
				}
			} else if n < maxPacketSize {
				return 0, badprogram.Errorf("value of symbol size suffix is too small: %s", f)
			}
			// Max receive size is just validated and thrown away.
		}

	case "wasi_snapshot_preview1", "wasi_unstable":
		libfn, found = wasiFunctions[f]
	}

	if !found {
		err = badprogram.Errorf("import function not supported: %q %q", module, field)
		return
	}

	if !sig.Equal(libfn.Type) {
		err = badprogram.Errorf("function %s.%s %s imported with wrong signature %s", module, field, libfn.Type, sig)
		return
	}

	if libfn.Index == library_random_get.Index {
		ir.Random = true
	}

	index = libfn.Index
	return
}

func (*ImportResolver) ResolveGlobal(module, field string, t wa.Type) (value uint64, err error) {
	err = badprogram.Errorf("import global not supported: %q %q", module, field)
	return
}

func allDigits(s string) bool {
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
