// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"github.com/tsavola/confi"
)

const (
	DefaultRef = true
)

type Config struct {
	Address      string
	IdentityFile string
	Ref          bool
	Function     string
	Instance     string
	Debug        string
	REPL         REPLConfig
}

var home = os.Getenv("HOME")
var c = new(Config)

const mainUsageHead = `Usage: %s [options] [address] command [arguments]

Commands:
  call      execute a wasm module with I/O
  delete    delete an instance
  download  download a wasm module
  instances list instances
  io        connect to a running instance
  launch    create an instance from a wasm module
  modules   list wasm module references
  repl      connect to a running instance in interactive mode
  resume    resume a suspended instance
  snapshot  create a wasm snapshot of an instance
  status    query current status of an instance
  suspend   suspend a running instance
  unref     remove a wasm module reference
  upload    upload a wasm module
  wait      wait until an instance is suspended, halted or terminated

Address examples:
  example.net           (scheme defaults to https)
  https://internal      (scheme needed with unqualified hostname)
  http://localhost:8080

Options:
`

const moduleUsage = `
Module can be a local wasm file, a reference, or a supported source:
  ./wasm-file
  I4hOg1lxclcr20elFIIjlrWw4H7Twp2eMTGU1KrfX_np05M6WZ0DpcTIvSajbE9d
  /ipfs/QmQugy6674g1rJumFQ5gAtuJf8uJobxSi23GUqUaewoPLc
`

const mainUsageTail = `
Default configuration is read from ~/.config/gate/gate.toml if it exists.
It will be ignored if the -F option is used.
`

type command struct {
	usage string
	do    func()
}

func parseConfig(flags *flag.FlagSet, c *Config) {
	var defaults string
	if home != "" {
		defaults = path.Join(home, ".config/gate/gate.toml")
	}

	b := confi.NewBuffer(defaults)

	flags.Var(b.FileReplacer(), "F", "replace previous configuration with this file")
	flags.Var(b.FileReader(), "f", "read an additional configuration file")
	flags.Var(b.Assigner(), "o", "set a configuration option (path.to.key=value)")
	flags.Parse(os.Args[1:])

	if err := b.Apply(c); err != nil {
		fmt.Fprintf(flags.Output(), "%s: %v\n", flags.Name(), err)
		os.Exit(2)
	}
}

func main() {
	log.SetFlags(0)

	if home != "" {
		c.IdentityFile = path.Join(home, ".ssh/id_ed25519")
	}

	c.Ref = DefaultRef

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	parseConfig(flags, c)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), mainUsageHead, flag.CommandLine.Name())
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), mainUsageTail)
	}
	flag.Usage = confi.FlagUsage(nil, c)
	parseConfig(flag.CommandLine, c)

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	if s := flag.Arg(0); strings.Contains(s, ".") || strings.Contains(s, "://") {
		if flag.NArg() < 2 || flag.Arg(1) == "-h" || flag.Arg(1) == "-help" || flag.Arg(1) == "--help" {
			flag.Usage()
			os.Exit(2)
		}
		c.Address = s
		os.Args = flag.Args()[1:]
	} else {
		os.Args = flag.Args()
	}

	progname := flag.CommandLine.Name()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.CommandLine.ErrorHandling())

	commands := localCommands
	otherCommands := remoteCommands
	if c.Address != "" {
		commands, otherCommands = otherCommands, commands
	}

	command, ok := commands[flag.CommandLine.Name()]
	if !ok {
		if _, exist := otherCommands[flag.CommandLine.Name()]; exist {
			fmt.Fprintln(flag.CommandLine.Output(), "Command not supported for specified address.")
		} else {
			flag.Usage()
		}
		os.Exit(2)
	}

	flag.Usage = func() {
		var options bool
		flag.VisitAll(func(*flag.Flag) { options = true })

		usageFmt := "Usage: %s"
		if c.Address != "" {
			usageFmt += " "
		}
		usageFmt += "%s %s"
		if options {
			usageFmt += " [options]"
		}
		if command.usage != "" {
			usageFmt += " "
		}
		usageFmt += "%s\n"
		if strings.Contains(command.usage, "module") {
			usageFmt += moduleUsage
		}
		if options {
			usageFmt += "\nOptions:\n"
		}

		fmt.Fprintf(flag.CommandLine.Output(), usageFmt, progname, c.Address, flag.CommandLine.Name(), command.usage)
		flag.PrintDefaults()
	}
	flag.CommandLine.Usage = flag.Usage
	flag.Parse()

	req := command.usage
	if i := strings.Index(req, "["); i >= 0 {
		req = req[:i]
	}
	if flag.NArg() < len(strings.Fields(strings.TrimSpace(req))) {
		flag.Usage()
		os.Exit(2)
	}
	if !strings.Contains(command.usage, "...") {
		if flag.NArg() > len(strings.Fields(strings.TrimSpace(command.usage))) {
			flag.Usage()
			os.Exit(2)
		}
	}

	command.do()
	os.Exit(0)
}
