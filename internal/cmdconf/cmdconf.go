// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmdconf

import (
	"flag"
	"fmt"
	"os"

	"import.name/confi"
)

var xdgEnvInited bool

func initEnvXDG() {
	if xdgEnvInited {
		return
	}

	ensure := func(name, value string) {
		if os.Getenv(name) == "" {
			os.Setenv(name, os.ExpandEnv(value))
		}
	}

	ensure("XDG_CACHE_HOME", "${HOME}/.cache")
	ensure("XDG_CONFIG_HOME", "${HOME}/.config")
	ensure("XDG_DATA_HOME", "${HOME}/.local/share")
	ensure("XDG_STATE_HOME", "${HOME}/.local/state")

	xdgEnvInited = true
}

// ExpandEnv ensures that HOME and XDG_*_HOME variables are available, and then
// calls [os.ExpandEnv].
func ExpandEnv(s string) string {
	initEnvXDG()
	return os.ExpandEnv(s)
}

// Parse command-line flags into the configuration object.  The default
// filenames are expanded with ExpandEnv.
func Parse(config any, flags *flag.FlagSet, lenient bool, defaults ...string) {
	var expanded []string
	for _, p := range defaults {
		expanded = append(expanded, ExpandEnv(p))
	}

	b := confi.NewBuffer(expanded...)

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
