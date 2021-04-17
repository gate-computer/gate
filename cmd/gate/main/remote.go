// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"gate.computer/gate/server/api"
	webapi "gate.computer/gate/server/web/api"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/proto"
)

var remoteCommands = map[string]command{
	"call": {
		usage:    "module [function]",
		detail:   moduleUsage,
		discover: discoverRemoteScope,
		parse:    parseLaunchFlags,
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
			for _, t := range c.InstanceTags {
				params.Add(webapi.ParamInstanceTag, t)
			}
			if c.DebugLog != "" {
				params.Set(webapi.ParamLog, "*")
			}

			var status webapi.Status

			switch arg := flag.Arg(0); {
			case !(strings.Contains(arg, "/") || strings.Contains(arg, ".")):
				status = callPost(webapi.PathKnownModules+arg, params)

			case strings.HasPrefix(arg, "/ipfs/"):
				if c.Pin {
					params.Add(webapi.ParamAction, webapi.ActionPin)
					for _, t := range c.ModuleTags {
						params.Add(webapi.ParamModuleTag, t)
					}
				}
				status = callPost(webapi.PathModule+arg, params)

			default:
				if c.Pin {
					params.Add(webapi.ParamAction, webapi.ActionPin)
					for _, t := range c.ModuleTags {
						params.Add(webapi.ParamModuleTag, t)
					}
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
			debug(func(instID string, debug *api.DebugRequest) *api.DebugResponse {
				debugJSON, err := proto.Marshal(debug)
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

				res := new(api.DebugResponse)
				decodeProto(resp.Body, res)
				return res
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
		usage: "filename [moduletag...]",
		do: func() {
			if tail := flag.Args()[1:]; len(tail) != 0 {
				c.ModuleTags = tail
			}

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
				webapi.ParamAction: []string{webapi.ActionPin},
			}
			for _, t := range c.ModuleTags {
				params.Add(webapi.ParamModuleTag, t)
			}

			doHTTP(req, webapi.PathKnownModules+hash, params)
			fmt.Println(hash)
		},
	},

	"instances": {
		do: func() {
			req := &http.Request{Method: http.MethodPost}
			_, resp := doHTTP(req, webapi.PathInstances, nil)

			var is webapi.Instances
			check(json.NewDecoder(resp.Body).Decode(&is))

			for _, inst := range is.Instances {
				fmt.Printf("%-36s %s %s\n", inst.Instance, inst.Status, inst.Tags)
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
		usage:    "module [function [instancetag...]]",
		detail:   moduleUsage,
		discover: discoverRemoteScope,
		parse:    parseLaunchFlags,
		do: func() {
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
				if tail := flag.Args()[2:]; len(tail) != 0 {
					c.InstanceTags = tail
				}
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
			for _, t := range c.InstanceTags {
				params.Add(webapi.ParamInstanceTag, t)
			}
			if c.DebugLog != "" {
				params.Set(webapi.ParamLog, "*")
			}

			var (
				req = new(http.Request)
				uri string
			)
			switch arg := flag.Arg(0); {
			case !(strings.Contains(arg, "/") || strings.Contains(arg, ".")):
				req.Method = http.MethodPost
				uri = webapi.PathKnownModules + arg

			case strings.HasPrefix(arg, "/ipfs/"):
				req.Method = http.MethodPut
				uri = webapi.PathModule + arg

				if c.Pin {
					params.Add(webapi.ParamAction, webapi.ActionPin)
					for _, t := range c.ModuleTags {
						params.Add(webapi.ParamModuleTag, t)
					}
				}

			default:
				module, key := loadModule(arg)

				req.Method = http.MethodPut
				uri = webapi.PathKnownModules + key

				if c.Pin {
					params.Add(webapi.ParamAction, webapi.ActionPin)
					for _, t := range c.ModuleTags {
						params.Add(webapi.ParamModuleTag, t)
					}
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
			req := &http.Request{Method: http.MethodPost}
			_, resp := doHTTP(req, webapi.PathKnownModules, nil)

			var refs webapi.Modules
			check(json.NewDecoder(resp.Body).Decode(&refs))

			for _, m := range refs.Modules {
				fmt.Println(m.ID, m.Tags)
			}
		},
	},

	"pin": {
		usage: "module [moduletag...]",
		do: func() {
			if tail := flag.Args()[1:]; len(tail) != 0 {
				c.ModuleTags = tail
			}

			req := &http.Request{Method: http.MethodPost}
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionPin},
			}
			for _, t := range c.ModuleTags {
				params.Add(webapi.ParamModuleTag, t)
			}

			doHTTP(req, webapi.PathKnownModules+flag.Arg(0), params)
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
			req := &http.Request{Method: http.MethodPost}
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionResume},
			}
			if c.DebugLog != "" {
				params.Set(webapi.ParamLog, "*")
			}

			doHTTP(req, webapi.PathInstances+flag.Arg(0), params)
		},
	},

	"show": {
		usage: "module",
		do: func() {
			req := &http.Request{Method: http.MethodPost}
			_, resp := doHTTP(req, webapi.PathKnownModules+flag.Arg(0), nil)

			var info webapi.ModuleInfo
			check(json.NewDecoder(resp.Body).Decode(&info))

			fmt.Println(info.Tags)
		},
	},

	"snapshot": {
		usage: "instance [moduletag...]",
		do: func() {
			if tail := flag.Args()[1:]; len(tail) != 0 {
				c.ModuleTags = tail
			}

			req := &http.Request{Method: http.MethodPost}
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionSnapshot},
			}
			for _, t := range c.ModuleTags {
				params.Add(webapi.ParamModuleTag, t)
			}

			_, resp := doHTTP(req, webapi.PathInstances+flag.Arg(0), params)

			location := resp.Header.Get(webapi.HeaderLocation)
			if location == "" {
				log.Fatal("no Location header in response")
			}

			fmt.Println(path.Base(location))
		},
	},

	"status": {
		usage: "instance",
		do: func() {
			req := &http.Request{
				Method: http.MethodPost,
				Header: http.Header{webapi.HeaderAccept: []string{webapi.ContentTypeJSON}},
			}

			_, resp := doHTTP(req, webapi.PathInstances+flag.Arg(0), nil)

			info := new(webapi.InstanceInfo)
			check(json.NewDecoder(resp.Body).Decode(info))

			fmt.Printf("%s %s\n", info.Status, info.Tags)
		},
	},

	"suspend": {
		usage: "instance",
		do: func() {
			commandInstanceWaiter(webapi.ActionSuspend)
		},
	},

	"unpin": {
		usage: "module",
		do: func() {
			req := &http.Request{Method: http.MethodPost}
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionUnpin},
			}

			doHTTP(req, webapi.PathKnownModules+flag.Arg(0), params)
		},
	},

	"update": {
		usage: "instance [instancetag...]",
		do: func() {
			if tail := flag.Args()[1:]; len(tail) != 0 {
				c.InstanceTags = tail
			}

			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionUpdate},
			}
			update := webapi.InstanceUpdate{
				Persist: true,
				Tags:    c.InstanceTags,
			}
			if len(update.Tags) == 0 {
				log.Fatal("no tags")
			}

			updateJSON, err := json.Marshal(update)
			check(err)

			req := &http.Request{
				Method: http.MethodPost,
				Header: http.Header{
					webapi.HeaderContentType: []string{webapi.ContentTypeJSON},
				},
				Body:          ioutil.NopCloser(bytes.NewReader(updateJSON)),
				ContentLength: int64(len(updateJSON)),
			}

			doHTTP(req, webapi.PathInstances+flag.Arg(0), params)
		},
	},

	"wait": {
		usage: "instance",
		do: func() {
			fmt.Println(commandInstance(webapi.ActionWait))
		},
	},
}

func discoverRemoteScope(w io.Writer) {
	fmt.Fprintln(w)

	params := url.Values{
		webapi.ParamFeature: []string{webapi.FeatureScope},
	}

	req := &http.Request{Method: http.MethodGet}
	_, resp := doHTTP(req, webapi.Path, params)

	var f webapi.Features
	check(json.NewDecoder(resp.Body).Decode(&f))

	printScope(w, f.Scope)
}

func exportRemote(module, filename string) {
	download(filename, func() (io.Reader, int64) {
		_, resp := doHTTP(nil, webapi.PathKnownModules+module, nil)
		return resp.Body, resp.ContentLength
	})
}

func callPost(uri string, params url.Values) webapi.Status {
	req := &http.Request{
		Method: http.MethodPost,
		Body:   os.Stdin,
	}
	req.Header.Set(webapi.HeaderTE, webapi.TETrailers)

	_, resp := doHTTP(req, uri, params)
	checkCopy(os.Stdout, resp.Body)
	return unmarshalStatus(resp.Trailer.Get(webapi.HeaderStatus))
}

func callWebsocket(filename string, params url.Values) webapi.Status {
	module, key := loadModule(filename)

	url := makeWebsocketURL(webapi.PathKnownModules+key, params)

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
	req := &http.Request{Method: http.MethodPost}
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
	h := webapi.KnownModuleHash.New()
	checkCopy(h, io.TeeReader(f, b))
	key = webapi.EncodeKnownModule(h.Sum(nil))
	return
}

func doHTTP(req *http.Request, uri string, params url.Values) (status webapi.Status, resp *http.Response) {
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

	if len(params) > 0 {
		u.RawQuery = params.Encode()
	}

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

	aud, err := url.Parse(c.address)
	check(err)
	if aud.Scheme == "" {
		aud.Scheme = "https"
	}
	aud.Path += webapi.Path

	sort.Strings(c.Scope)
	scope := strings.Join(c.Scope, " ")

	claims := &webapi.Claims{
		Exp:   time.Now().Unix() + 60,
		Aud:   []string{aud.String()},
		Scope: scope,
	}

	identity, err := ioutil.ReadFile(c.IdentityFile)
	check(err)

	if len(identity) != 0 {
		x, err := ssh.ParseRawPrivateKey(identity)
		check(err)

		privateKey, ok := x.(*ed25519.PrivateKey)
		if !ok {
			log.Fatalf("%s: not an Ed25519 private key", c.IdentityFile)
		}

		publicJWK := webapi.PublicKeyEd25519(privateKey.Public().(ed25519.PublicKey))
		jwtHeader := webapi.TokenHeaderEdDSA(publicJWK)
		auth, err := webapi.AuthorizationBearerEd25519(*privateKey, jwtHeader.MustEncode(), claims)
		check(err)

		return auth
	} else {
		if aud.Scheme != "http" {
			log.Fatalf("%s scheme with empty identity", aud.Scheme)
		}

		ips, err := net.LookupIP(aud.Hostname())
		check(err)

		for _, ip := range ips {
			if !ip.IsLoopback() {
				log.Fatalf("non-loopback host with empty identity: %s", ip)
			}
		}

		auth, err := webapi.AuthorizationBearerLocal(claims)
		check(err)

		return auth
	}
}

func unmarshalStatus(serialized string) (status webapi.Status) {
	check(json.Unmarshal([]byte(serialized), &status))
	if status.Error != "" {
		log.Fatal(status.String())
	}
	return
}

func decodeProto(r io.Reader, m proto.Message) {
	b, err := ioutil.ReadAll(r)
	check(err)
	check(proto.Unmarshal(b, m))
}

func checkCopy(w io.Writer, r io.Reader) int64 {
	n, err := io.Copy(w, r)
	check(err)
	return n
}
