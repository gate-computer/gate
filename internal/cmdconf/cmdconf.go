// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmdconf

import (
	"flag"
	"fmt"
	"os"
	"path"

	"import.name/confi"
)

var (
	homeDir string
	homeErr error
)

func init() {
	homeDir, homeErr = os.UserHomeDir()
}

func JoinHome(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	if path.IsAbs(dir) {
		return dir, nil
	}
	if homeErr != nil {
		return "", homeErr
	}
	return path.Join(homeDir, dir), nil
}

func JoinHomeFallback(dir, alternative string) string {
	if s, err := JoinHome(dir); err == nil {
		return s
	}
	return alternative
}

// Parse command-line flags into the configuration object.  The default
// filename patterns can be absolute, or relative to home directory.
func Parse(config any, flags *flag.FlagSet, lenient bool, defaults ...string) {
	var absDefaults []string
	for _, p := range defaults {
		if p, err := JoinHome(p); err == nil {
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
