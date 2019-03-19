// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tsavola/confi"
	"github.com/tsavola/gate/webapi"
	"github.com/tsavola/gate/webapi/authorization"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

const (
	DefaultRef      = true
	DefaultFunction = "main"
	DefaultTLS      = true
)

type Config struct {
	IdentityFile string
	Ref          bool
	Function     string
	Instance     string
	Debug        string
	TLS          bool
	address      string
}

var c = new(Config)

const mainUsage = `Usage: %s [options] <address> <command> [arguments]

Commands:
  call      execute a wasm module with I/O
  download  download a wasm module
  instances list instances
  io        connect to a running instance
  launch    create an instance from a wasm module
  modules   list wasm module references
  resume    resume a suspended instance
  snapshot  create a wasm snapshot of an instance
  status    query current status of an instance
  suspend   suspend a running instance
  unref     remove a wasm module reference
  upload    upload a wasm module
  wait      wait until an instance is suspended or terminated

Options:
`

func parseConfig(flags *flag.FlagSet, c *Config) {
	flags.Var(confi.FileReader(c), "f", "read TOML configuration file")
	flags.Var(confi.Assigner(c), "c", "set a configuration key (path.to.key=value)")
	flags.Parse(os.Args[1:])
}

func main() {
	log.SetFlags(0)

	if home := os.Getenv("HOME"); home != "" {
		c.IdentityFile = path.Join(home, ".ssh/id_ed25519")
	}

	c.Ref = DefaultRef
	c.Function = DefaultFunction
	c.TLS = DefaultTLS

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	parseConfig(flags, c)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), mainUsage, flag.CommandLine.Name())
		flag.PrintDefaults()
	}
	flag.Usage = confi.FlagUsage(nil, c)
	parseConfig(flag.CommandLine, c)

	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(2)
	}

	c.address = flag.Arg(0)
	os.Args = flag.Args()[1:]

	progname := flag.CommandLine.Name()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.CommandLine.ErrorHandling())

	command, ok := commands[flag.CommandLine.Name()]
	if !ok {
		flag.Usage()
		os.Exit(2)
	}

	flag.Usage = func() {
		var options bool
		flag.VisitAll(func(*flag.Flag) { options = true })

		usageFmt := "Usage: %s %s %s"

		if options {
			usageFmt += " [options]"
		}

		usageFmt += "%s\n"

		if strings.Contains(command.usage, "<module>") {
			usageFmt += "\nThe module can be a local wasm file, a reference which exists on the server,\nor a source supported by the server.\n"
		}

		if options {
			usageFmt += "\nOptions:\n"
		}

		fmt.Fprintf(flag.CommandLine.Output(), usageFmt, progname, c.address, flag.CommandLine.Name(), command.usage)
		flag.PrintDefaults()
	}

	command.do()
}

var commands = map[string]struct {
	usage string
	do    func()
}{
	"call": {
		usage: " <module> [function]",
		do: func() {
			flag.Parse()
			switch flag.NArg() {
			case 1:

			case 2:
				c.Function = flag.Arg(1)

			default:
				flag.Usage()
				os.Exit(2)
			}

			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionCall},
			}
			if c.Function != "" {
				params.Set(webapi.ParamFunction, c.Function)
			}
			if c.Debug != "" {
				params.Set(webapi.ParamDebug, c.Debug)
			}

			var status webapi.Status

			switch arg := flag.Arg(0); {
			case !strings.Contains(arg, "/"):
				status = callPost(webapi.PathModuleRefs+arg, params)

			case strings.HasPrefix(arg, "/ipfs/"):
				if c.Ref {
					params.Add(webapi.ParamAction, webapi.ActionRef)
				}

				status = callPost(webapi.PathModule+arg, params)

			default:
				if c.Ref {
					params.Add(webapi.ParamAction, webapi.ActionRef)
				}

				status = callWebsocket(arg, params)
			}

			if status.State != "terminated" || status.Cause != "" {
				log.Fatal(status)
			}
			os.Exit(status.Result)
		},
	},

	"download": {
		usage: " <module>",
		do: func() {
			// TODO: output file option

			flag.Parse()
			if flag.NArg() != 1 {
				flag.Usage()
				os.Exit(2)
			}

			req := &http.Request{Method: http.MethodGet}

			_, resp, err := doHTTP(req, webapi.PathModuleRefs+flag.Arg(0), nil)
			if err != nil {
				log.Fatal(err)
			}

			_, err = io.Copy(os.Stdout, resp.Body)
			if err != nil {
				log.Fatal(err)
			}
		},
	},

	"instances": {
		do: func() {
			flag.Parse()
			if flag.NArg() != 0 {
				flag.Usage()
				os.Exit(2)
			}

			req := &http.Request{Method: http.MethodGet}

			_, resp, err := doHTTP(req, webapi.PathInstances, nil)
			if err != nil {
				log.Fatal(err)
			}

			var is webapi.Instances

			err = json.NewDecoder(resp.Body).Decode(&is)
			if err != nil {
				log.Fatal(err)
			}

			for _, inst := range is.Instances {
				fmt.Printf("%-36s %s\n", inst.Instance, inst.Status)
			}
		},
	},

	"io": {
		usage: " <instance>",
		do: func() {
			flag.Parse()
			if flag.NArg() != 1 {
				flag.Usage()
				os.Exit(2)
			}

			req := &http.Request{
				Method: http.MethodPost,
				Body:   os.Stdin,
			}

			params := url.Values{webapi.ParamAction: []string{webapi.ActionIO}}

			_, resp, err := doHTTP(req, webapi.PathInstances+flag.Arg(0), params)
			if err != nil {
				log.Fatal(err)
			}

			_, err = io.Copy(os.Stdout, resp.Body)
			if err != nil {
				log.Fatal(err)
			}
		},
	},

	"launch": {
		usage: " <module> [function]",
		do: func() {
			flag.Parse()
			switch flag.NArg() {
			case 1:

			case 2:
				c.Function = flag.Arg(1)

			default:
				flag.Usage()
				os.Exit(2)
			}

			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionLaunch},
			}
			if c.Function != "" {
				params.Set(webapi.ParamFunction, c.Function)
			}
			if c.Instance != "" {
				params.Set(webapi.ParamInstance, c.Instance)
			}
			if c.Debug != "" {
				params.Set(webapi.ParamDebug, c.Debug)
			}

			var req = new(http.Request)
			var uri string

			switch arg := flag.Arg(0); {
			case !strings.Contains(arg, "/"):
				req.Method = http.MethodPost
				uri = webapi.PathModuleRefs + arg

			case strings.HasPrefix(arg, "/ipfs/"):
				req.Method = http.MethodPut
				uri = webapi.PathModule + arg

				if c.Ref {
					params.Add(webapi.ParamAction, webapi.ActionRef)
				}

			default:
				module, key, err := loadModule(arg)
				if err != nil {
					log.Fatal(err)
				}

				req.Method = http.MethodPut
				uri = webapi.PathModuleRefs + key

				if c.Ref {
					params.Add(webapi.ParamAction, webapi.ActionRef)
				}

				req.Header = http.Header{
					webapi.HeaderContentType: []string{webapi.ContentTypeWebAssembly},
				}
				req.Body = ioutil.NopCloser(module)
				req.ContentLength = int64(module.Len())
			}

			_, resp, err := doHTTP(req, uri, params)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println(resp.Header.Get(webapi.HeaderInstance))
		},
	},

	"modules": {
		do: func() {
			flag.Parse()
			if flag.NArg() != 0 {
				flag.Usage()
				os.Exit(2)
			}

			req := &http.Request{Method: http.MethodGet}

			_, resp, err := doHTTP(req, webapi.PathModuleRefs, nil)
			if err != nil {
				log.Fatal(err)
			}

			var refs webapi.ModuleRefs

			err = json.NewDecoder(resp.Body).Decode(&refs)
			if err != nil {
				log.Fatal(err)
			}

			for _, m := range refs.Modules {
				if m.Suspended {
					fmt.Println(m.Key, "suspended")
				} else {
					fmt.Println(m.Key)
				}
			}
		},
	},

	"resume": {
		usage: " <instance>",
		do: func() {
			flag.Parse()
			if flag.NArg() != 1 {
				flag.Usage()
				os.Exit(2)
			}

			req := &http.Request{Method: http.MethodPost}

			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionResume},
			}
			if c.Debug != "" {
				params.Set(webapi.ParamDebug, c.Debug)
			}

			_, _, err := doHTTP(req, webapi.PathInstances+flag.Arg(0), params)
			if err != nil {
				log.Fatal(err)
			}
		},
	},

	"snapshot": {
		usage: " <instance>",
		do: func() {
			flag.Parse()
			if flag.NArg() != 1 {
				flag.Usage()
				os.Exit(2)
			}

			req := &http.Request{Method: http.MethodPost}
			params := url.Values{webapi.ParamAction: []string{webapi.ActionSnapshot}}

			_, resp, err := doHTTP(req, webapi.PathInstances+flag.Arg(0), params)
			if err != nil {
				log.Fatal(err)
			}

			location := resp.Header.Get(webapi.HeaderLocation)
			if location == "" {
				log.Fatal("no Location header in response")
			}

			fmt.Println(path.Base(location))
		},
	},

	"status": {
		usage: " <instance>",
		do: func() {
			commandInstance(webapi.ActionStatus)
		},
	},

	"suspend": {
		usage: " <instance>",
		do: func() {
			commandInstance(webapi.ActionSuspend)
		},
	},

	"unref": {
		usage: " <module>",
		do: func() {
			flag.Parse()
			if flag.NArg() != 1 {
				flag.Usage()
				os.Exit(2)
			}

			req := &http.Request{Method: http.MethodPost}
			params := url.Values{webapi.ParamAction: []string{webapi.ActionUnref}}

			_, _, err := doHTTP(req, webapi.PathModuleRefs+flag.Arg(0), params)
			if err != nil {
				log.Fatal(err)
			}
		},
	},

	"upload": {
		usage: " <module>",
		do: func() {
			flag.Parse()
			if flag.NArg() != 1 {
				flag.Usage()
				os.Exit(2)
			}

			module, key, err := loadModule(flag.Arg(0))
			if err != nil {
				log.Fatal(err)
			}

			req := &http.Request{
				Method: http.MethodPut,
				Header: http.Header{
					webapi.HeaderContentType: []string{webapi.ContentTypeWebAssembly},
				},
				Body:          ioutil.NopCloser(module),
				ContentLength: int64(module.Len()),
			}

			var params url.Values
			if c.Ref {
				params = url.Values{webapi.ParamAction: []string{webapi.ActionRef}}
			}

			_, _, err = doHTTP(req, webapi.PathModuleRefs+key, params)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println(key)
		},
	},

	"wait": {
		usage: " <instance>",
		do: func() {
			commandInstance(webapi.ActionWait)
		},
	},
}

func callPost(uri string, params url.Values) webapi.Status {
	req := &http.Request{
		Method: http.MethodPost,
		Body:   os.Stdin,
	}

	_, resp, err := doHTTP(req, uri, params)
	if err != nil {
		log.Fatal(err)
	}

	_, err = io.Copy(os.Stdout, resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	status, err := unmarshalStatus(resp.Trailer.Get(webapi.HeaderStatus))
	if err != nil {
		log.Fatal(err)
	}

	return status
}

func callWebsocket(filename string, params url.Values) webapi.Status {
	module, key, err := loadModule(filename)
	if err != nil {
		log.Fatal(err)
	}

	u, err := makeURL("ws", webapi.PathModuleRefs+key, params)
	if err != nil {
		log.Fatal(err)
	}

	conn, _, err := new(websocket.Dialer).Dial(u.String(), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	call := &webapi.Call{
		ContentType:   webapi.ContentTypeWebAssembly,
		ContentLength: int64(module.Len()),
	}

	call.Authorization, err = makeAuthorization()
	if err != nil {
		log.Fatal(err)
	}

	if err := conn.WriteJSON(call); err != nil {
		log.Fatal(err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, module.Bytes()); err != nil {
		log.Fatal(err)
	}

	if err := conn.ReadJSON(new(webapi.CallConnection)); err != nil {
		log.Fatal(err)
	}

	// TODO: input

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			log.Fatal(err)
		}

		switch msgType {
		case websocket.BinaryMessage:
			os.Stdout.Write(data) // TODO: handle error?

		case websocket.TextMessage:
			// TODO: receive ConnectionStatus
			var status webapi.Status

			if err := json.Unmarshal(data, &status); err != nil {
				log.Fatal(err)
			}

			return status
		}
	}
}

func commandInstance(action string) {
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	req := &http.Request{Method: http.MethodPost}
	params := url.Values{webapi.ParamAction: []string{action}}

	status, _, err := doHTTP(req, webapi.PathInstances+flag.Arg(0), params)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(status)
}

func loadModule(filename string) (b *bytes.Buffer, key string, err error) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	b = new(bytes.Buffer)
	h := webapi.ModuleRefHash.New()

	_, err = io.Copy(h, io.TeeReader(f, b))
	if err != nil {
		return
	}

	key = base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return
}

func doHTTP(req *http.Request, uri string, params url.Values) (status webapi.Status, resp *http.Response, err error) {
	u, err := makeURL("http", uri, params)
	if err != nil {
		return
	}

	req.URL = u
	req.Close = true
	req.Host = u.Host

	auth, err := makeAuthorization()
	if err != nil {
		return
	}
	if auth != "" {
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		req.Header.Set(webapi.HeaderAuthorization, auth)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusCreated:

	default:
		if resp.Body == nil {
			err = errors.New(resp.Status)
		} else if text, e := ioutil.ReadAll(resp.Body); e != nil {
			err = fmt.Errorf("%s: %q", resp.Status, text)
		} else {
			err = errors.New(string(text))
		}
		return
	}

	if serialized := resp.Header.Get(webapi.HeaderStatus); serialized != "" {
		status, err = unmarshalStatus(serialized)
	}
	return
}

func makeURL(scheme, uri string, params url.Values) (u *url.URL, err error) {
	u, err = url.Parse(scheme + "s://" + c.address + uri)
	if err != nil {
		return
	}
	if !c.TLS {
		u.Scheme = scheme
	}
	u.RawQuery = params.Encode()
	return
}

func makeAuthorization() (auth string, err error) {
	if c.IdentityFile == "" {
		return
	}

	data, err := ioutil.ReadFile(c.IdentityFile)
	if err != nil {
		return
	}

	x, err := ssh.ParseRawPrivateKey(data)
	if err != nil {
		return
	}

	privateKey, ok := x.(*ed25519.PrivateKey)
	if !ok {
		err = fmt.Errorf("%s: not an ed25519 private key", c.IdentityFile)
		return
	}

	publicJWK := webapi.PublicKeyEd25519(privateKey.Public().(ed25519.PublicKey))
	jwtHeader := webapi.TokenHeaderEdDSA(publicJWK)

	claims := &webapi.Claims{
		Exp: time.Now().Unix() + 60,
		Aud: []string{"https://" + c.address + webapi.Path},
	}

	auth, err = authorization.BearerEd25519(*privateKey, jwtHeader.MustEncode(), claims)
	if err != nil {
		return
	}

	return
}

func unmarshalStatus(serialized string) (status webapi.Status, err error) {
	err = json.Unmarshal([]byte(serialized), &status)
	if err != nil {
		return
	}

	if status.State == "" {
		err = errors.New(status.Error)
		return
	}
	if status.Error != "" {
		err = errors.New(status.String())
		return
	}

	return
}
