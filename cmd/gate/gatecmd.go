// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"gate.computer/gate/internal/cmdconf"
	"github.com/tsavola/confi"
	"golang.org/x/sys/unix"
)

const (
	DefaultIdentityFile = ".ssh/id_ed25519" // Relative to home directory.
	DefaultRef          = true
	DefaultWait         = true
	ShortcutDebugLog    = "/dev/stderr"
)

// Defaults are relative to home directory.
var Defaults = []string{
	".config/gate/gate.toml",
	".config/gate/gate.d/*.toml",
}

type Config struct {
	IdentityFile string
	Ref          bool
	Wait         bool
	Function     string
	Instance     string
	Scope        []string
	Suspend      bool
	DebugLog     string
	REPL         REPLConfig

	address string
}

var c = new(Config)

const mainUsageHead = `Usage: %s [options] [address] command [arguments]

Common commands:
  call      execute a wasm module with I/O
  debug     instance debugger
  delete    delete an instance
  export    write a wasm module to a local file or standard output
  import    read a wasm module from a local file
  instances list instances
  io        connect to a running instance
  kill      kill a running instance
  launch    create an instance from a wasm module
  modules   list wasm module references
  resume    resume a suspended or halted instance
  snapshot  create a wasm module of a suspended or halted instance
  status    query current status of an instance
  suspend   suspend a running instance
  unref     remove a wasm module reference
  wait      wait until an instance is suspended, halted, terminated or killed

Local commands (no address before command):
  pull      copy a wasm module from a remote server to local storage
  push      copy a wasm module from local storage to a remote server

Remote commands (server address must be specified before command):
  repl      connect to a running instance in interactive mode

Address examples:
  example.net           (scheme defaults to https)
  https://internal      (scheme needed with unqualified hostname)
  http://localhost:8080

Options:
`

const altAddressUsage = `Usage: %s %s %s

For %s, the server address must be specified after the command.
`

const moduleUsage = `
Module can be a local wasm file, a reference, or a supported source:
  file.wasm
  I4hOg1lxclcr20elFIIjlrWw4H7Twp2eMTGU1KrfX_np05M6WZ0DpcTIvSajbE9d
  /ipfs/QmQugy6674g1rJumFQ5gAtuJf8uJobxSi23GUqUaewoPLc
`

const mainUsageTail = `
Default configuration is read from ~/.config/gate/gate.toml if it exists.
It will be ignored if the -F option is used.
`

func parseCallFlags() {
	debug := flag.Bool("d", c.DebugLog == ShortcutDebugLog, "write debug log to stderr")
	flag.Parse()
	if *debug {
		c.DebugLog = ShortcutDebugLog
	}
}

type command struct {
	usage  string
	detail string
	parse  func()
	do     func()
}

func main() {
	log.SetFlags(0)

	c.IdentityFile = cmdconf.JoinHome(DefaultIdentityFile)
	c.Ref = DefaultRef
	c.Wait = DefaultWait

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	cmdconf.Parse(c, flags, true, Defaults...)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), mainUsageHead, flag.CommandLine.Name())
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), mainUsageTail)
	}
	flag.Usage = confi.FlagUsage(nil, c)
	cmdconf.Parse(c, flag.CommandLine, false, Defaults...)

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	if s := flag.Arg(0); strings.Contains(s, ".") || strings.Contains(s, "://") {
		if flag.NArg() < 2 || flag.Arg(1) == "-h" || flag.Arg(1) == "-help" || flag.Arg(1) == "--help" {
			flag.Usage()
			os.Exit(2)
		}
		c.address = s
		os.Args = flag.Args()[1:]
	} else {
		os.Args = flag.Args()
	}

	progname := flag.CommandLine.Name()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.CommandLine.ErrorHandling())

	commands := localCommands
	otherCommands := remoteCommands
	if c.address != "" {
		commands, otherCommands = otherCommands, commands
	}

	command, ok := commands[flag.CommandLine.Name()]
	if !ok {
		if command, exist := otherCommands[flag.CommandLine.Name()]; exist {
			if strings.HasPrefix(command.usage, "address") {
				fmt.Fprintf(flag.CommandLine.Output(), altAddressUsage, progname, flag.CommandLine.Name(), command.usage, flag.CommandLine.Name())
			} else {
				fmt.Fprintln(flag.CommandLine.Output(), "Command not supported for specified address.")
			}
		} else {
			flag.Usage()
		}
		os.Exit(2)
	}

	flag.Usage = func() {
		var options bool
		flag.VisitAll(func(*flag.Flag) { options = true })

		usageFmt := "Usage: %s"
		if c.address != "" {
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
		if options {
			usageFmt += "\nOptions:\n"
		}

		fmt.Fprintf(flag.CommandLine.Output(), usageFmt, progname, c.address, flag.CommandLine.Name(), command.usage)
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), command.detail)
	}
	flag.CommandLine.Usage = flag.Usage
	if command.parse != nil {
		command.parse()
	} else {
		flag.Parse()
	}

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

func openFile(name string) *os.File {
	f, err := os.Open(name)
	check(err)
	return f
}

func terminalOr(fallback io.Writer) io.Writer {
	for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		if _, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS); err == nil {
			return f
		}
	}
	return fallback
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func check_(_ interface{}, err error) {
	check(err)
}
