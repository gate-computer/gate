// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package librarian

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"gate.computer/wag/compile"
	"gate.computer/wag/section"
	"gate.computer/wag/wa"
)

func Build(output, ld, objdump, gopkg string, verbose bool, commands [][]string) error {
	var objects []string

	if len(commands) == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		filename, remove, err := writeTempFile(b)
		if err != nil {
			return err
		}
		defer remove()

		objects = append(objects, filename)
	}

	for _, command := range commands {
		if verbose {
			fmt.Println(strings.Join(command, " "))
		}

		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stderr = os.Stderr
		b, err := cmd.Output()
		if err != nil {
			return err
		}

		filename, remove, err := writeTempFile(b)
		if err != nil {
			return err
		}
		defer remove()

		objects = append(objects, filename)
	}

	return Link(output, ld, objdump, gopkg, verbose, objects...)
}

func Link(output, ld, objdump, gopkg string, verbose bool, objects ...string) error {
	var linked string

	if len(objects) == 1 && ld == "" {
		linked = objects[0]
	} else {
		args := append([]string{"--allow-undefined", "--export-dynamic", "--no-entry", "-o", "/dev/stdout"}, objects...)

		if verbose {
			fmt.Println(ld, strings.Join(args, " "))
		}

		cmd := exec.Command(ld, args...)
		cmd.Stderr = os.Stderr
		b, err := cmd.Output()
		if err != nil {
			return err
		}

		filename, remove, err := writeTempFile(b)
		if err != nil {
			return err
		}
		defer remove()

		linked = filename
	}

	if verbose {
		fmt.Println()
	}

	cmd := exec.Command(objdump, "-d", linked)
	cmd.Stderr = os.Stderr
	dump, err := cmd.Output()
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(dump), "\n") {
		matched, err := regexp.MatchString(`(^\w+ func|\scall \d+ <(env\.)?rt_\w+>)`, line)
		if err != nil {
			return err
		}
		if matched {
			if verbose {
				fmt.Println("", line)
			}
			continue
		}

		matched, err = regexp.MatchString(`(^Data|\scall|\sglobal|\smemory\.grow)`, line)
		if err != nil {
			return err
		}
		if matched {
			if verbose {
				fmt.Println(line)
			}
			return errors.New("WebAssembly module is not a suitable library")
		}
	}

	if verbose {
		fmt.Println()
	}

	data, err := os.ReadFile(linked)
	if err != nil {
		return err
	}

	sections := new(section.Map)
	config := compile.ModuleConfig{
		Config: compile.Config{
			ModuleMapper: sections,
		},
	}
	module, err := compile.LoadInitialSections(&config, compile.NewLoader(bytes.NewReader(data)))
	if err != nil {
		return err
	}

	b := new(bytes.Buffer)
	r := compile.NewLoader(bytes.NewReader(data))

	if _, err := io.CopyN(b, r, sections.Sections[section.Type].Start); err != nil {
		return err
	}
	librarySections := map[section.ID]bool{
		section.Type:     true,
		section.Import:   true,
		section.Function: true,
		section.Memory:   true,
		section.Export:   true,
		section.Code:     true,
	}
	for id := section.Type; id <= section.Code; id++ {
		w := io.Discard
		if librarySections[id] {
			w = b
		}
		if _, err := section.CopyStandardSection(w, r, id, nil); err != nil {
			return err
		}
	}

	data = b.Bytes()

	if gopkg != "" {
		var funcs []string
		for name := range module.ExportFuncs() {
			funcs = append(funcs, name)
		}
		sort.Strings(funcs)

		b := bytes.NewBuffer(nil)
		checksum := crc64.Checksum(data, crc64.MakeTable(crc64.ECMA))

		fmt.Fprintln(b, "// Code generated by gate-librarian, DO NOT EDIT!")
		fmt.Fprintln(b)
		fmt.Fprintf(b, "package %s\n", gopkg)
		fmt.Fprintln(b)
		fmt.Fprintln(b, `import "gate.computer/wag/wa"`)
		fmt.Fprintln(b)
		fmt.Fprintf(b, "const libraryChecksum uint64 = 0x%016x\n", checksum)
		fmt.Fprintln(b)
		fmt.Fprintln(b, "var (")

		for i, name := range funcs {
			index, sig, found := module.ExportFunc(name)
			if !found {
				panic(name)
			}

			if i > 0 {
				fmt.Fprintln(b)
			}
			fmt.Fprintf(b, "\tlibrary_%s = libraryFunction{\n", name)
			fmt.Fprintf(b, "\t\tIndex: %d,\n", index)
			fmt.Fprintf(b, "\t\tType: %s}\n", reprFuncType(sig))
		}

		fmt.Fprintln(b, ")")
		fmt.Fprintln(b)
		fmt.Fprint(b, "var libraryWASM = [...]byte{")

		for i, n := range data {
			if i%12 == 0 {
				fmt.Fprintf(b, "\n\t")
			} else {
				fmt.Fprintf(b, " ")
			}
			fmt.Fprintf(b, "0x%02x,", n)
		}

		fmt.Fprintln(b, "\n}")

		data = b.Bytes()
	}

	if err := os.WriteFile(output, data, 0o644); err != nil {
		return err
	}

	return nil
}

func reprFuncType(f wa.FuncType) string {
	s := "wa.FuncType{"

	if len(f.Params) == 0 && len(f.Results) == 0 {
		return " " + s + "}"
	}

	s += "\n"

	indent := " "
	if len(f.Results) > 0 {
		indent = "  "
	}

	if len(f.Params) > 0 {
		s += fmt.Sprintf("\t\t\tParams:%s[]wa.Type{", indent)

		for i, p := range f.Params {
			if i > 0 {
				s += ", "
			}
			s += "wa." + strings.ToUpper(p.String())
		}

		s += "},\n"
	}

	if len(f.Results) > 0 {
		s += "\t\t\tResults: []wa.Type{"

		for i, p := range f.Results {
			if i > 0 {
				s += ", "
			}
			s += "wa." + strings.ToUpper(p.String())
		}

		s += "},\n"
	}

	return s + "\t\t}"
}

func writeTempFile(b []byte) (string, func(), error) {
	var ok bool

	f, err := os.CreateTemp("", "*.wasm")
	if err != nil {
		return "", nil, err
	}

	name := f.Name()

	remove := func() {
		os.Remove(name)
	}

	defer func() {
		if !ok {
			remove()
		}
	}()

	if _, err := f.Write(b); err != nil {
		return "", nil, err
	}

	if err := f.Close(); err != nil {
		return "", nil, err
	}

	ok = true
	return name, remove, nil
}
