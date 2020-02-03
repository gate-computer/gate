// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmdconf

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/tsavola/confi"
)

var home = os.Getenv("HOME")

func JoinHome(dir string) string {
	if dir == "" {
		return ""
	}
	if path.IsAbs(dir) {
		return dir
	}
	if home != "" {
		return path.Join(home, dir)
	}
	return ""
}

// Parse command-line flags into the configuration object.  The default
// filename patterns can be absolute, or relative to home directory.
func Parse(config interface{}, flags *flag.FlagSet, lenient bool, defaults ...string) {
	var absDefaults []string
	for _, p := range defaults {
		p = JoinHome(p)
		if p != "" {
			absDefaults = append(absDefaults, p)
		}
	}

	b := confi.NewBuffer(absDefaults...)

	flags.Var(b.FileReplacer(), "F", "replace previous configuration with this file")
	flags.Var(b.FileReader(), "f", "read a configuration file")
	flags.Var(b.DirReader("*.toml"), "d", "read configuration files from a directory")
	flags.Var(b.Assigner(), "o", "set a configuration option (path.to.key=value)")
	flags.Parse(os.Args[1:])

	if err := b.Flush(config, lenient); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", flags.Name(), err)
		if !lenient {
			os.Exit(2)
		}
	}
}
