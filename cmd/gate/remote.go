// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	api "github.com/tsavola/gate/serverapi"
	"github.com/tsavola/gate/webapi"
	"github.com/tsavola/gate/webapi/authorization"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

var remoteCommands = map[string]command{
	"call": {
		usage:  "module [function]",
		detail: moduleUsage,
		do: func() {
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
			}

			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionCall},
			}
			if c.Function != "" {
				params.Set(webapi.ParamFunction, c.Function)
			}
			if c.DebugLog != "" {
				params.Set(webapi.ParamDebug, "true")
			}

			var status webapi.Status

			switch arg := flag.Arg(0); {
			case !(strings.Contains(arg, "/") || strings.Contains(arg, ".")):
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

			if status.State != webapi.StateTerminated || status.Cause != "" {
				log.Fatal(status)
			}
			os.Exit(status.Result)
		},
	},

	"debug": {
		usage: "instance [command [offset...]]",
		do: func() {
			debug(func(instID string, debug api.DebugRequest) (res api.DebugResponse) {
				debugJSON, err := json.Marshal(debug)
				check(err)

				params := url.Values{
					webapi.ParamAction: []string{webapi.ActionDebug},
				}

				req := &http.Request{
					Method: http.MethodPost,
					Header: http.Header{
						webapi.HeaderContentType: []string{webapi.ContentTypeJSON},
					},
					Body:          ioutil.NopCloser(bytes.NewReader(debugJSON)),
					ContentLength: int64(len(debugJSON)),
				}

				_, resp := doHTTP(req, webapi.PathInstances+instID, params)
				check(json.NewDecoder(resp.Body).Decode(&res))
				return
			})
		},
	},

	"delete": {
		usage: "instance",
		do: func() {
			commandInstance(webapi.ActionDelete)
		},
	},

	"export": {
		usage: "module [filename]",
		do: func() {
			var filename string
			if flag.NArg() > 1 {
				filename = flag.Arg(1)
			}

			exportRemote(flag.Arg(0), filename)
		},
	},

	"import": {
		usage: "filename",
		do: func() {
			data, hash := loadModule(flag.Arg(0))

			req := &http.Request{
				Method: http.MethodPut,
				Header: http.Header{
					webapi.HeaderContentType: []string{webapi.ContentTypeWebAssembly},
				},
				Body:          ioutil.NopCloser(data),
				ContentLength: int64(data.Len()),
			}
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionRef},
			}

			doHTTP(req, webapi.PathModuleRefs+hash, params)
			fmt.Println(hash)
		},
	},

	"instances": {
		do: func() {
			_, resp := doHTTP(nil, webapi.PathInstances, nil)

			var is webapi.Instances
			check(json.NewDecoder(resp.Body).Decode(&is))

			for _, inst := range is.Instances {
				fmt.Printf("%-36s %s\n", inst.Instance, inst.Status)
			}
		},
	},

	"io": {
		usage: "instance",
		do: func() {
			req := &http.Request{
				Method: http.MethodPost,
				Body:   os.Stdin,
			}
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionIO},
			}

			_, resp := doHTTP(req, webapi.PathInstances+flag.Arg(0), params)
			checkCopy(os.Stdout, resp.Body)
		},
	},

	"kill": {
		usage: "instance",
		do: func() {
			commandInstanceWaiter(webapi.ActionKill)
		},
	},

	"launch": {
		usage:  "module [function]",
		detail: moduleUsage,
		do: func() {
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
			}

			actions := []string{
				webapi.ActionLaunch,
			}
			if c.Suspend {
				actions = append(actions, webapi.ActionSuspend)
			}

			params := url.Values{
				webapi.ParamAction: actions,
			}
			if c.Function != "" {
				params.Set(webapi.ParamFunction, c.Function)
			}
			if c.Instance != "" {
				params.Set(webapi.ParamInstance, c.Instance)
			}
			if c.DebugLog != "" {
				params.Set(webapi.ParamDebug, "true")
			}

			var (
				req = new(http.Request)
				uri string
			)
			switch arg := flag.Arg(0); {
			case !(strings.Contains(arg, "/") || strings.Contains(arg, ".")):
				req.Method = http.MethodPost
				uri = webapi.PathModuleRefs + arg

			case strings.HasPrefix(arg, "/ipfs/"):
				req.Method = http.MethodPut
				uri = webapi.PathModule + arg

				if c.Ref {
					params.Add(webapi.ParamAction, webapi.ActionRef)
				}

			default:
				module, key := loadModule(arg)

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

			_, resp := doHTTP(req, uri, params)
			fmt.Println(resp.Header.Get(webapi.HeaderInstance))
		},
	},

	"modules": {
		do: func() {
			_, resp := doHTTP(nil, webapi.PathModuleRefs, nil)

			var refs webapi.ModuleRefs
			check(json.NewDecoder(resp.Body).Decode(&refs))

			for _, m := range refs.Modules {
				fmt.Println(m.Id)
			}
		},
	},

	"repl": {
		usage: "instance",
		do: func() {
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionIO},
			}

			var d websocket.Dialer
			conn, _, err := d.Dial(makeWebsocketURL(webapi.PathInstances+flag.Arg(0), params), nil)
			check(err)

			check(conn.WriteJSON(webapi.IO{Authorization: makeAuthorization()}))

			var reply webapi.IOConnection
			check(conn.ReadJSON(&reply))
			if !reply.Connected {
				log.Fatal("connection rejected")
			}

			replWebsocket(conn)
		},
	},

	"resume": {
		usage: "instance",
		do: func() {
			req := &http.Request{
				Method: http.MethodPost,
			}

			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionResume},
			}
			if c.DebugLog != "" {
				params.Set(webapi.ParamDebug, "true")
			}

			doHTTP(req, webapi.PathInstances+flag.Arg(0), params)
		},
	},

	"snapshot": {
		usage: "instance [filename]",
		do: func() {
			req := &http.Request{
				Method: http.MethodPost,
			}
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionSnapshot},
			}

			_, resp := doHTTP(req, webapi.PathInstances+flag.Arg(0), params)

			location := resp.Header.Get(webapi.HeaderLocation)
			if location == "" {
				log.Fatal("no Location header in response")
			}
			progID := path.Base(location)

			if flag.NArg() == 1 {
				fmt.Println(progID)
			} else {
				fmt.Fprintln(terminalOr(ioutil.Discard), progID)
				exportRemote(progID, flag.Arg(1))
			}
		},
	},

	"status": {
		usage: "instance",
		do: func() {
			fmt.Println(commandInstance(webapi.ActionStatus))
		},
	},

	"suspend": {
		usage: "instance",
		do: func() {
			commandInstanceWaiter(webapi.ActionSuspend)
		},
	},

	"unref": {
		usage: "module",
		do: func() {
			req := &http.Request{
				Method: http.MethodPost,
			}
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionUnref},
			}

			doHTTP(req, webapi.PathModuleRefs+flag.Arg(0), params)
		},
	},

	"wait": {
		usage: "instance",
		do: func() {
			fmt.Println(commandInstance(webapi.ActionWait))
		},
	},
}

func exportRemote(module, filename string) {
	download(filename, func() (io.Reader, int64) {
		_, resp := doHTTP(nil, webapi.PathModuleRefs+module, nil)
		return resp.Body, resp.ContentLength
	})
}

func callPost(uri string, params url.Values) webapi.Status {
	req := &http.Request{
		Method: http.MethodPost,
		Body:   os.Stdin,
	}

	_, resp := doHTTP(req, uri, params)
	checkCopy(os.Stdout, resp.Body)
	return unmarshalStatus(resp.Trailer.Get(webapi.HeaderStatus))
}

func callWebsocket(filename string, params url.Values) webapi.Status {
	module, key := loadModule(filename)

	url := makeWebsocketURL(webapi.PathModuleRefs+key, params)

	conn, _, err := new(websocket.Dialer).Dial(url, nil)
	check(err)
	defer conn.Close()

	check(conn.WriteJSON(webapi.Call{
		Authorization: makeAuthorization(),
		ContentType:   webapi.ContentTypeWebAssembly,
		ContentLength: int64(module.Len()),
	}))
	check(conn.WriteMessage(websocket.BinaryMessage, module.Bytes()))
	check(conn.ReadJSON(new(webapi.CallConnection)))

	// TODO: input

	for {
		msgType, data, err := conn.ReadMessage()
		check(err)

		switch msgType {
		case websocket.BinaryMessage:
			os.Stdout.Write(data)

		case websocket.TextMessage:
			var status webapi.ConnectionStatus
			check(json.Unmarshal(data, &status))
			return status.Status
		}
	}
}

func commandInstance(actions ...string) webapi.Status {
	req := &http.Request{
		Method: http.MethodPost,
	}
	params := url.Values{
		webapi.ParamAction: actions,
	}

	status, _ := doHTTP(req, webapi.PathInstances+flag.Arg(0), params)
	return status
}

func commandInstanceWaiter(action string) {
	actions := []string{action}
	if c.Wait {
		actions = append(actions, webapi.ActionWait)
	}

	status := commandInstance(actions...)
	if c.Wait {
		fmt.Println(status)
	}
}

func loadModule(filename string) (b *bytes.Buffer, key string) {
	f := openFile(filename)
	defer f.Close()

	b = new(bytes.Buffer)
	h := webapi.ModuleRefHash.New()
	checkCopy(h, io.TeeReader(f, b))
	key = base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return
}

func doHTTP(req *http.Request, uri string, params url.Values,
) (status webapi.Status, resp *http.Response) {
	if req == nil {
		req = new(http.Request)
	}
	req.URL = makeURL(uri, params, req.Body != nil)
	req.Close = true
	req.Host = req.URL.Host

	auth := makeAuthorization()
	if auth != "" {
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		req.Header.Set(webapi.HeaderAuthorization, auth)
	}

	resp, err := http.DefaultClient.Do(req)
	check(err)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusCreated:
		// ok

	default:
		msg := resp.Status
		if x := strings.SplitN(resp.Header.Get(webapi.HeaderContentType), ";", 2); x[0] == "text/plain" {
			if text, _ := ioutil.ReadAll(resp.Body); len(text) > 0 {
				msg = string(text)
			}
		}
		log.Fatal(msg)
	}

	if serialized := resp.Header.Get(webapi.HeaderStatus); serialized != "" {
		status = unmarshalStatus(serialized)
	}
	return
}

func makeURL(uri string, params url.Values, prelocate bool) (u *url.URL) {
	addr := c.address
	if !strings.Contains(addr, "://") {
		addr = "https://" + addr
	}

	if prelocate {
		resp, err := http.Head(addr + webapi.Path)
		check(err)
		resp.Body.Close()

		u = resp.Request.URL
		u.Path = u.Path + strings.Replace(uri, webapi.Path, "", 1)
	} else {
		var err error

		u, err = url.Parse(addr + uri)
		if err != nil {
			return
		}
	}

	u.RawQuery = params.Encode()
	return
}

func makeWebsocketURL(uri string, params url.Values) string {
	u := makeURL(uri, params, true)

	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		log.Fatalf("address has unsupported scheme: %q", u.Scheme)
	}

	return u.String()
}

func makeAuthorization() string {
	if c.IdentityFile == "" {
		return ""
	}

	data, err := ioutil.ReadFile(c.IdentityFile)
	check(err)

	x, err := ssh.ParseRawPrivateKey(data)
	check(err)

	privateKey, ok := x.(*ed25519.PrivateKey)
	if !ok {
		log.Fatalf("%s: not an Ed25519 private key", c.IdentityFile)
	}

	publicJWK := webapi.PublicKeyEd25519(privateKey.Public().(ed25519.PublicKey))
	jwtHeader := webapi.TokenHeaderEdDSA(publicJWK)

	sort.Strings(c.Scope)
	scope := strings.Join(c.Scope, " ")

	claims := &webapi.Claims{
		Exp:   time.Now().Unix() + 60,
		Aud:   []string{"https://" + c.address + webapi.Path},
		Scope: scope,
	}

	auth, err := authorization.BearerEd25519(*privateKey, jwtHeader.MustEncode(), claims)
	check(err)

	return auth
}

func unmarshalStatus(serialized string) (status webapi.Status) {
	check(json.Unmarshal([]byte(serialized), &status))
	if status.Error != "" {
		log.Fatal(status.String())
	}
	return
}

func checkCopy(w io.Writer, r io.Reader) (n int64) {
	n, err := io.Copy(w, r)
	check(err)
	return
}
