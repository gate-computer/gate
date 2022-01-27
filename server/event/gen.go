// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"unicode"

	"gate.computer/gate/server/event"
	m "import.name/make"
)

func main() {
	var (
		gofmt  = os.Args[1]
		output = os.Args[2]
	)

	b := bytes.NewBuffer(nil)

	fmt.Fprintln(b, "// Code generated by gen.go, DO NOT EDIT!")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "package event")
	fmt.Fprintln(b)

	var names []string
	for _, name := range event.Type_name {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(b, "func (x *%s) EventName() string {", snake2title(name))
		fmt.Fprintf(b, " return \"%s\" }\n", name)
	}

	fmt.Fprintln(b)

	for _, name := range names {
		fmt.Fprintf(b, "func (*%s) EventType() int32 {", snake2title(name))
		fmt.Fprintf(b, " return int32(Type_%s) }\n", name)
	}

	data, err := m.RunIO(b, gofmt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := ioutil.WriteFile(output, data, 0666); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func snake2title(snake string) string {
	var (
		title string
		up    = true
	)

	for _, code := range snake {
		if code == '_' {
			up = true
		} else {
			r := rune(code)
			if up {
				r = unicode.ToUpper(r)
				up = false
			} else {
				r = unicode.ToLower(r)
			}
			title += string(r)
		}
	}

	return title
}