// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package abi

//go:generate go run ../../internal/cmd/abi-library/generate.go

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
	"rt": {
		"trap":   -17,
		"debug":  -16,
		"write":  -15,
		"read":   -14,
		"poll":   -13,
		"time":   -12,
		"random": -6,
	},
}

var library compile.Library

func Library() compile.Library {
	return library
}

func init() {
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

	library = lib
}

type abiFunction struct {
	name   string
	random bool
}

var gateFunctions = map[string]abiFunction{
	"fd": {name: "fd"},
	"io": {name: "io"},
}

var wasiFunctions = map[string]abiFunction{
	"args_get":                {name: "args_get"},
	"args_sizes_get":          {name: "args_sizes_get"},
	"clock_res_get":           {name: "clock_res_get"},
	"clock_time_get":          {name: "clock_time_get"},
	"environ_get":             {name: "environ_get"},
	"environ_sizes_get":       {name: "environ_sizes_get"},
	"fd_advise":               {name: "fd_stub_i32i64i64i32"},
	"fd_allocate":             {name: "fs_stub_i64i64"},
	"fd_close":                {name: "fd_close"},
	"fd_datasync":             {name: "fd_stub_i32"},
	"fd_fdstat_get":           {name: "fd_fdstat_get"},
	"fd_fdstat_set_flags":     {name: "fd_fdstat_set_flags"},
	"fd_fdstat_set_rights":    {name: "fd_fdstat_set_rights"},
	"fd_filestat_get":         {name: "fd_filestat_get"},
	"fd_filestat_set_size":    {name: "fd_stub_i32i64"},
	"fd_filestat_set_times":   {name: "fd_stub_i32i64i64i32"},
	"fd_pread":                {name: "fd_stub_i32i32i32i64i32"},
	"fd_prestat_dir_name":     {name: "fd_stub_i32i32i32"},
	"fd_prestat_get":          {name: "fd_stub_i32i32"},
	"fd_pwrite":               {name: "fd_stub_i32i32i32i64i32"},
	"fd_read":                 {name: "fd_read"},
	"fd_readdir":              {name: "fd_stub_i32i32i32i64i32"},
	"fd_renumber":             {name: "fd_renumber"},
	"fd_seek":                 {name: "fd_stub_i32i64i32i32"},
	"fd_sync":                 {name: "fd_stub_i32"},
	"fd_tell":                 {name: "fd_stub_i32i32"},
	"fd_write":                {name: "fd_write"},
	"path_create_directory":   {name: "path_stub_i32i32i32"},
	"path_filestat_get":       {name: "path_stub_i32i32i32i32i32"},
	"path_filestat_set_times": {name: "path_stub_i32i32i32i32i64i64i32"},
	"path_link":               {name: "path_stub_i32i32i32i32i32i32i32"},
	"path_open":               {name: "path_stub_i32i32i32i32i32i64i64i32i32"},
	"path_readlink":           {name: "path_stub_i32i32i32i32i32i32"},
	"path_remove_directory":   {name: "path_stub_i32i32i32"},
	"path_rename":             {name: "path_stub_i32i32i32i32i32i32"},
	"path_symlink":            {name: "path_stub_i32i32i32i32i32"},
	"path_unlink_file":        {name: "path_stub_i32i32i32"},
	"poll_oneoff":             {name: "poll_oneoff"},
	"proc_exit":               {name: "proc_exit"},
	"proc_raise":              {name: "proc_raise"},
	"random_get":              {name: "random_get", random: true},
	"sched_yield":             {name: "sched_yield"},
	"sock_recv":               {name: "sock_recv"},
	"sock_send":               {name: "sock_send"},
	"sock_shutdown":           {name: "sock_shutdown"},
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

	var abi abiFunction

	switch m {
	case "gate":
		i := strings.LastIndexByte(f, '_')
		if i <= 0 {
			break
		}

		prefix := f[:i]
		suffix := f[i+1:]

		abi = gateFunctions[prefix]
		if abi.name != "" {
			if strings.HasPrefix(suffix, "0") {
				panic("TODO") // TODO: return nice error message (syntax error)
			}
			if n, e := strconv.ParseUint(suffix, 10, 32); e != nil {
				if !errors.Is(e, strconv.ErrRange) || !allDigits(suffix) {
					panic("TODO") // TODO: return nice error message (syntax error)
				}
			} else if n < maxPacketSize {
				panic("TODO") // TODO: return nice error message (value range)
			}
			// Max receive size is just validated and thrown away.
		}

	case "wasi_snapshot_preview1", "wasi_unstable":
		abi = wasiFunctions[f]
	}

	if abi.name == "" {
		err = badprogram.Errorf("import function not supported: %q %q", module, field)
		return
	}

	index, libsig, found := library.ExportFunc(abi.name)
	if !found {
		panic(abi.name)
	}

	if !sig.Equal(libsig) {
		err = badprogram.Errorf("function %s.%s %s imported with wrong signature %s", module, field, libsig, sig)
		return
	}

	if abi.random {
		ir.Random = true
	}
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
