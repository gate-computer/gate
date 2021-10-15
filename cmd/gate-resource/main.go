// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program gate-resource dumps C header file contents.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"

	"gate.computer/gate/include"
)

var resources = map[string]interface {
	fs.ReadDirFS
	fs.ReadFileFS
}{
	"include": include.FS,
}

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: %s [options] filename...\n", flag.CommandLine.Name())

		fmt.Fprintf(out, "\nFilenames:\n")

		for resdir, resfs := range resources {
			list, err := resfs.ReadDir(".")
			if err != nil {
				panic(err)
			}

			for _, f := range list {
				fmt.Fprintf(out, "  %s\n", path.Join(resdir, f.Name()))
			}
		}

		fmt.Fprintf(out, "\nOptions:\n")
		flag.PrintDefaults()
	}

	dir := flag.String("d", "", "write files to directory instead of stdout")
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	if err := Main(*dir, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func Main(outdir string, filenames []string) error {
	for _, filename := range filenames {
		resdir, name := path.Split(filename)
		resdir = path.Join(resdir, ".") // Remove separator at end.

		resfs := resources[resdir]
		if resfs == nil {
			return fmt.Errorf("%s: %w", filename, fs.ErrNotExist)
		}

		data, err := resfs.ReadFile(name)
		if err != nil {
			return fmt.Errorf("%s: %w", resdir, err)
		}

		if outdir != "" {
			os.MkdirAll(outdir, 0755)

			outname := path.Join(outdir, name)
			if err := ioutil.WriteFile(outname, data, 0644); err != nil {
				os.Remove(outname)
				return err
			}
		} else {
			if _, err := os.Stdout.Write(data); err != nil {
				return err
			}
		}
	}

	return nil
}
