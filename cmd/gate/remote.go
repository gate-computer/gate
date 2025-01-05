// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	pb "gate.computer/gate/pb/server"
	"gate.computer/gate/web"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/encoding/protojson"

	. "import.name/pan/mustcheck"
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
				web.ParamAction: []string{web.ActionCall},
			}
			if c.Function != "" {
				params.Set(web.ParamFunction, c.Function)
			}
			for _, t := range c.InstanceTags {
				params.Add(web.ParamInstanceTag, t)
			}
			if c.DebugLog != "" {
				params.Set(web.ParamLog, "*")
			}

			var status web.Status

			switch arg := flag.Arg(0); {
			case !(strings.Contains(arg, "/") || strings.Contains(arg, ".")):
				status = callPost(web.PathKnownModules+arg, params)

			case strings.HasPrefix(arg, "/ipfs/"):
				if c.Pin {
					params.Add(web.ParamAction, web.ActionPin)
					for _, t := range c.ModuleTags {
						params.Add(web.ParamModuleTag, t)
					}
				}
				status = callPost(web.PathModule+arg, params)

			default:
				if c.Pin {
					params.Add(web.ParamAction, web.ActionPin)
					for _, t := range c.ModuleTags {
						params.Add(web.ParamModuleTag, t)
					}
				}
				status = callWebsocket(arg, params)
			}

			if status.State != web.StateTerminated || status.Cause != "" {
				fatal(status)
			}
			os.Exit(status.Result)
		},
	},

	"debug": {
		usage: "instance [command [offset...]]",
		do: func() {
			debug(func(instID string, debug *pb.DebugRequest) *pb.DebugResponse {
				debugJSON := Must(protojson.Marshal(debug))

				params := url.Values{
					web.ParamAction: []string{web.ActionDebug},
				}

				req := &http.Request{
					Method: http.MethodPost,
					Header: http.Header{
						web.HeaderContentType: []string{web.ContentTypeJSON},
					},
					Body:          io.NopCloser(bytes.NewReader(debugJSON)),
					ContentLength: int64(len(debugJSON)),
				}

				_, resp := doHTTP(req, web.PathInstances+instID, params)

				res := new(pb.DebugResponse)
				Check(protojson.Unmarshal(Must(io.ReadAll(resp.Body)), res))
				return res
			})
		},
	},

	"delete": {
		usage: "instance",
		do: func() {
			commandInstance(web.ActionDelete)
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
					web.HeaderContentType: []string{web.ContentTypeWebAssembly},
				},
				Body:          io.NopCloser(data),
				ContentLength: int64(data.Len()),
			}
			params := url.Values{
				web.ParamAction: []string{web.ActionPin},
			}
			for _, t := range c.ModuleTags {
				params.Add(web.ParamModuleTag, t)
			}

			doHTTP(req, web.PathKnownModules+hash, params)
			fmt.Println(hash)
		},
	},

	"instances": {
		do: func() {
			req := &http.Request{Method: http.MethodPost}
			_, resp := doHTTP(req, web.PathInstances, nil)

			var is web.Instances
			Check(json.NewDecoder(resp.Body).Decode(&is))

			sort.SliceStable(is.Instances, func(i, j int) bool {
				return is.Instances[i].Instance < is.Instances[j].Instance
			})

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
				web.ParamAction: []string{web.ActionIO},
			}

			_, resp := doHTTP(req, web.PathInstances+flag.Arg(0), params)
			Must(io.Copy(os.Stdout, resp.Body))
		},
	},

	"kill": {
		usage: "instance",
		do: func() {
			commandInstanceWaiter(web.ActionKill)
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
				web.ActionLaunch,
			}
			if c.Suspend {
				actions = append(actions, web.ActionSuspend)
			}

			params := url.Values{
				web.ParamAction: actions,
			}
			if c.Function != "" {
				params.Set(web.ParamFunction, c.Function)
			}
			if c.Instance != "" {
				params.Set(web.ParamInstance, c.Instance)
			}
			for _, t := range c.InstanceTags {
				params.Add(web.ParamInstanceTag, t)
			}
			if c.DebugLog != "" {
				params.Set(web.ParamLog, "*")
			}

			var (
				req = new(http.Request)
				uri string
			)
			switch arg := flag.Arg(0); {
			case !(strings.Contains(arg, "/") || strings.Contains(arg, ".")):
				req.Method = http.MethodPost
				uri = web.PathKnownModules + arg

			case strings.HasPrefix(arg, "/ipfs/"):
				req.Method = http.MethodPost
				uri = web.PathModule + arg

				if c.Pin {
					params.Add(web.ParamAction, web.ActionPin)
					for _, t := range c.ModuleTags {
						params.Add(web.ParamModuleTag, t)
					}
				}

			default:
				module, key := loadModule(arg)

				req.Method = http.MethodPost
				uri = web.PathKnownModules + key

				if c.Pin {
					params.Add(web.ParamAction, web.ActionPin)
					for _, t := range c.ModuleTags {
						params.Add(web.ParamModuleTag, t)
					}
				}

				req.Header = http.Header{
					web.HeaderContentType: []string{web.ContentTypeWebAssembly},
				}
				req.Body = io.NopCloser(module)
				req.ContentLength = int64(module.Len())
			}

			_, resp := doHTTP(req, uri, params)
			fmt.Println(resp.Header.Get(web.HeaderInstance))
		},
	},

	"modules": {
		do: func() {
			req := &http.Request{Method: http.MethodPost}
			_, resp := doHTTP(req, web.PathKnownModules, nil)

			var refs web.Modules
			Check(json.NewDecoder(resp.Body).Decode(&refs))

			sort.SliceStable(refs.Modules, func(i, j int) bool {
				return refs.Modules[i].ID < refs.Modules[j].ID
			})

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
				web.ParamAction: []string{web.ActionPin},
			}
			for _, t := range c.ModuleTags {
				params.Add(web.ParamModuleTag, t)
			}

			doHTTP(req, web.PathKnownModules+flag.Arg(0), params)
		},
	},

	"repl": {
		usage: "instance",
		do: func() {
			params := url.Values{
				web.ParamAction: []string{web.ActionIO},
			}

			var d websocket.Dialer
			conn, _, err := d.Dial(makeWebsocketURL(web.PathInstances+flag.Arg(0), params), nil)
			Check(err)

			Check(conn.WriteJSON(web.IO{Authorization: makeAuthorization()}))

			var reply web.IOConnection
			Check(conn.ReadJSON(&reply))
			if !reply.Connected {
				fatal("connection rejected")
			}

			replWebsocket(conn)
		},
	},

	"resume": {
		usage: "instance",
		do: func() {
			req := &http.Request{Method: http.MethodPost}
			params := url.Values{
				web.ParamAction: []string{web.ActionResume},
			}
			if c.DebugLog != "" {
				params.Set(web.ParamLog, "*")
			}

			doHTTP(req, web.PathInstances+flag.Arg(0), params)
		},
	},

	"show": {
		usage: "module",
		do: func() {
			req := &http.Request{Method: http.MethodPost}
			_, resp := doHTTP(req, web.PathKnownModules+flag.Arg(0), nil)

			var info web.ModuleInfo
			Check(json.NewDecoder(resp.Body).Decode(&info))

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
				web.ParamAction: []string{web.ActionSnapshot},
			}
			for _, t := range c.ModuleTags {
				params.Add(web.ParamModuleTag, t)
			}

			_, resp := doHTTP(req, web.PathInstances+flag.Arg(0), params)

			location := resp.Header.Get(web.HeaderLocation)
			if location == "" {
				fatal("no Location header in response")
			}

			fmt.Println(path.Base(location))
		},
	},

	"status": {
		usage: "instance",
		do: func() {
			req := &http.Request{
				Method: http.MethodPost,
				Header: http.Header{web.HeaderAccept: []string{web.ContentTypeJSON}},
			}

			_, resp := doHTTP(req, web.PathInstances+flag.Arg(0), nil)

			info := new(web.InstanceInfo)
			Check(json.NewDecoder(resp.Body).Decode(info))

			fmt.Printf("%s %s\n", info.Status, info.Tags)
		},
	},

	"suspend": {
		usage: "instance",
		do: func() {
			commandInstanceWaiter(web.ActionSuspend)
		},
	},

	"unpin": {
		usage: "module",
		do: func() {
			req := &http.Request{Method: http.MethodPost}
			params := url.Values{
				web.ParamAction: []string{web.ActionUnpin},
			}

			doHTTP(req, web.PathKnownModules+flag.Arg(0), params)
		},
	},

	"update": {
		usage: "instance [instancetag...]",
		do: func() {
			if tail := flag.Args()[1:]; len(tail) != 0 {
				c.InstanceTags = tail
			}

			params := url.Values{
				web.ParamAction: []string{web.ActionUpdate},
			}
			update := web.InstanceUpdate{
				Persist: true,
				Tags:    c.InstanceTags,
			}
			if len(update.Tags) == 0 {
				fatal("no tags")
			}

			updateJSON := Must(json.Marshal(update))

			req := &http.Request{
				Method: http.MethodPost,
				Header: http.Header{
					web.HeaderContentType: []string{web.ContentTypeJSON},
				},
				Body:          io.NopCloser(bytes.NewReader(updateJSON)),
				ContentLength: int64(len(updateJSON)),
			}

			doHTTP(req, web.PathInstances+flag.Arg(0), params)
		},
	},

	"wait": {
		usage: "instance",
		do: func() {
			fmt.Println(commandInstance(web.ActionWait))
		},
	},
}

func discoverRemoteScope(w io.Writer) {
	fmt.Fprintln(w)

	params := url.Values{
		web.ParamFeature: []string{web.FeatureScope},
	}

	req := &http.Request{Method: http.MethodGet}
	_, resp := doHTTP(req, web.Path, params)

	var f web.Features
	Check(json.NewDecoder(resp.Body).Decode(&f))

	printScope(w, f.Scope)
}

func exportRemote(module, filename string) {
	download(filename, func() (io.Reader, int64) {
		_, resp := doHTTP(nil, web.PathKnownModules+module, nil)
		return resp.Body, resp.ContentLength
	})
}

func callPost(uri string, params url.Values) web.Status {
	req := &http.Request{
		Method: http.MethodPost,
		Header: http.Header{web.HeaderTE: []string{web.TETrailers}},
		Body:   os.Stdin,
	}

	_, resp := doHTTP(req, uri, params)
	Must(io.Copy(os.Stdout, resp.Body))
	return unmarshalStatus(resp.Trailer.Get(web.HeaderStatus))
}

func callWebsocket(filename string, params url.Values) web.Status {
	module, key := loadModule(filename)

	url := makeWebsocketURL(web.PathKnownModules+key, params)

	conn, _, err := new(websocket.Dialer).Dial(url, nil)
	Check(err)
	defer conn.Close()

	Check(conn.WriteJSON(web.Call{
		Authorization: makeAuthorization(),
		ContentType:   web.ContentTypeWebAssembly,
		ContentLength: int64(module.Len()),
	}))
	Check(conn.WriteMessage(websocket.BinaryMessage, module.Bytes()))
	Check(conn.ReadJSON(new(web.CallConnection)))

	// TODO: input

	for {
		msgType, data, err := conn.ReadMessage()
		Check(err)

		switch msgType {
		case websocket.BinaryMessage:
			os.Stdout.Write(data)

		case websocket.TextMessage:
			var status web.ConnectionStatus
			Check(json.Unmarshal(data, &status))
			return status.Status
		}
	}
}

func commandInstance(actions ...string) web.Status {
	req := &http.Request{Method: http.MethodPost}
	params := url.Values{
		web.ParamAction: actions,
	}

	status, _ := doHTTP(req, web.PathInstances+flag.Arg(0), params)
	return status
}

func commandInstanceWaiter(action string) {
	actions := []string{action}
	if c.Wait {
		actions = append(actions, web.ActionWait)
	}

	status := commandInstance(actions...)
	if c.Wait {
		fmt.Println(status)
	}
}

func loadModule(filename string) (b *bytes.Buffer, key string) {
	f := Must(os.Open(filename))
	defer f.Close()

	b = new(bytes.Buffer)
	h := web.KnownModuleHash.New()
	Must(io.Copy(h, io.TeeReader(f, b)))
	key = web.EncodeKnownModule(h.Sum(nil))
	return
}

func doHTTP(req *http.Request, uri string, params url.Values) (status web.Status, resp *http.Response) {
	if req == nil {
		req = new(http.Request)
	}
	req.URL = makeURL(uri, params, req.Body != nil)
	req.Close = true
	req.Host = req.URL.Host

	auth := makeAuthorization()
	if auth != "" {
		if req.Header == nil {
			req.Header = make(http.Header, 1)
		}
		req.Header.Set(web.HeaderAuthorization, auth)
	}

	resp = Must(http.DefaultClient.Do(req))

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusCreated:
		// ok

	default:
		msg := resp.Status
		if x := strings.SplitN(resp.Header.Get(web.HeaderContentType), ";", 2); x[0] == "text/plain" {
			if text, _ := io.ReadAll(resp.Body); len(text) > 0 {
				msg = string(text)
			}
		}
		fatal(msg)
	}

	if serialized := resp.Header.Get(web.HeaderStatus); serialized != "" {
		status = unmarshalStatus(serialized)
	}
	return
}

func makeURL(uri string, params url.Values, prelocate bool) *url.URL {
	addr := c.address
	if !strings.Contains(addr, "://") {
		addr = "https://" + addr
	}

	var u *url.URL

	if prelocate {
		resp := Must(http.Head(addr + web.Path))
		resp.Body.Close()

		u = resp.Request.URL
		u.Path = u.Path + strings.Replace(uri, web.Path, "", 1)
	} else {
		u = Must(url.Parse(addr + uri))
	}

	if len(params) > 0 {
		u.RawQuery = params.Encode()
	}

	return u
}

func makeWebsocketURL(uri string, params url.Values) string {
	u := makeURL(uri, params, true)

	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		fatalf("address has unsupported scheme: %q", u.Scheme)
	}

	return u.String()
}

func makeAuthorization() string {
	if c.IdentityFile == "" {
		return ""
	}

	aud := Must(url.Parse(c.address))
	if aud.Scheme == "" {
		aud.Scheme = "https"
	}
	aud.Path += web.Path

	sort.Strings(c.Scope)
	scope := strings.Join(c.Scope, " ")

	claims := &web.Claims{
		Exp:   time.Now().Unix() + 60,
		Aud:   []string{aud.String()},
		Scope: scope,
	}

	identity := Must(os.ReadFile(c.IdentityFile))

	if len(identity) != 0 {
		x := Must(ssh.ParseRawPrivateKey(identity))
		privateKey, ok := x.(*ed25519.PrivateKey)
		if !ok {
			fatalf("%s: not an Ed25519 private key", c.IdentityFile)
		}

		publicJWK := web.PublicKeyEd25519(privateKey.Public().(ed25519.PublicKey))
		jwtHeader := web.TokenHeaderEdDSA(publicJWK)
		return Must(web.AuthorizationBearerEd25519(*privateKey, jwtHeader.MustEncode(), claims))
	} else {
		if aud.Scheme != "http" {
			fatalf("%s scheme with empty identity", aud.Scheme)
		}

		for _, ip := range Must(net.LookupIP(aud.Hostname())) {
			if !ip.IsLoopback() {
				fatalf("non-loopback host with empty identity: %s", ip)
			}
		}

		return Must(web.AuthorizationBearerLocal(claims))
	}
}

func unmarshalStatus(serialized string) (status web.Status) {
	Check(json.Unmarshal([]byte(serialized), &status))
	if status.Error != "" {
		fatal(status.String())
	}
	return
}
