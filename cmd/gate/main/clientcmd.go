// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"

	"gate.computer/gate/internal"
	"gate.computer/gate/internal/cmdconf"
	gatescope "gate.computer/gate/scope"
	"github.com/tsavola/confi"
	"golang.org/x/term"
	"import.name/pan"
)

const (
	DefaultIdentityFile = ".ssh/id_ed25519" // Relative to home directory.
	DefaultPin          = true
	DefaultWait         = true
	ShortcutDebugLog    = "/dev/stderr"
)

// Defaults are relative to home directory.
var Defaults = []string{
	".config/gate/client.toml",
	".config/gate/client.d/*.toml",
}

type Config struct {
	IdentityFile string
	Pin          bool
	ModuleTags   []string
	Wait         bool
	Function     string
	Instance     string
	InstanceTags []string
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
  modules   list known wasm modules
  resume    resume a suspended or halted instance
  snapshot  create a wasm module of an instance
  show      get information about a known module
  status    get current status and other instance information
  suspend   suspend a running instance
  pin       remember a wasm module or update its tags
  repl      connect to a running instance and present a line-oriented text UI
  unpin     forget a wasm module
  update    update instance's tags (and make it persistent if necessary)
  wait      wait until an instance is suspended, halted, terminated or killed

Local commands (no address before command):
  pull      copy a wasm module from a remote server to local storage
  push      copy a wasm module from local storage to a remote server

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
Default configuration is read from ~/.config/gate/client.toml and/or
~/.config/gate/client.d/*.toml.  They will be ignored if the -F option is used.
`

func registerRunFlags() {
	flag.Func("s", "extend scope (comma-separated; may be specified multiple times)", func(scope string) error {
		for _, s := range strings.Split(scope, ",") {
			c.Scope = append(c.Scope, strings.TrimSpace(s))
		}
		return nil
	})
}

func parseLaunchFlags() {
	registerRunFlags()
	flag.Parse()
}

type command struct {
	usage    string
	detail   string
	discover func(io.Writer)
	parse    func()
	do       func()
}

func Main() {
	log.SetFlags(0)

	if internal.CmdPanic == "" {
		defer func() {
			pan.Fatal(recover())
		}()
	}

	defaultIdentityFile, err := cmdconf.JoinHome(DefaultIdentityFile)
	check(err)

	c.IdentityFile = defaultIdentityFile
	c.Pin = DefaultPin
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

		if command.discover != nil {
			command.discover(flag.CommandLine.Output())
		}
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

func printScope(w io.Writer, scope []string) {
	if len(scope) == 0 {
		return
	}

	var (
		aliases = gatescope.ComputeAliases(scope)
		short   []string
		long    []string
	)

	for _, s := range scope {
		alias := gatescope.MatchAlias(s)
		if _, found := aliases[alias]; !found {
			alias = ""
		}

		if alias != "" {
			short = append(short, fmt.Sprintf("%s (%s)", alias, s))
		} else {
			long = append(long, s)
		}
	}

	sort.Strings(short)
	sort.Strings(long)

	fmt.Fprintln(w, "Scope values:")
	for _, s := range append(short, long...) {
		fmt.Fprintf(w, "  %s\n", s)
	}
}

func openFile(name string) *os.File {
	f, err := os.Open(name)
	check(err)
	return f
}

func terminalOr(fallback io.Writer) io.Writer {
	for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		if term.IsTerminal(int(f.Fd())) {
			return f
		}
	}
	return fallback
}

func fatal(x interface{}, args ...interface{}) {
	var (
		err error
		ok  bool
	)
	if len(args) == 0 {
		err, ok = x.(error)
	}
	if !ok {
		args = append([]interface{}{x}, args...)
		err = errors.New(fmt.Sprint(args...))
	}
	if err == nil {
		err = errors.New("nil")
	}
	check(err)
}

func fatalf(format string, args ...interface{}) {
	check(fmt.Errorf(format, args...))
}

func check(err error) {
	pan.Check(err)
}

func check_(_ interface{}, err error) {
	pan.Check(err)
}
