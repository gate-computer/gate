// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/service"
	"savo.la/gate/localhost/flat"
)

const testMaxSendSize = 65536

var testCode = packet.Code(time.Now().UnixNano() & 0x7fff)

func TestHTTPRequest(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "bogus" {
			t.Error(r.Host)
		}
		if r.Method != http.MethodGet {
			t.Error(r.Method)
		}
		if r.URL.Path != "/test" {
			t.Error(r.URL)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, "hellocalhost")
	}))
	defer s.Close()

	u, err := url.Parse(s.URL)
	if err != nil {
		panic(err)
	}

	i := newInstance(&localhost{u.Scheme, u.Host, s.Client()}, service.InstanceConfig{
		Service: packet.Service{
			MaxSendSize: testMaxSendSize,
			Code:        testCode,
		},
	})

	b := flatbuffers.NewBuilder(0)
	method := b.CreateString(http.MethodGet)
	uri := b.CreateString("//bogus/test")
	flat.HTTPRequestStart(b)
	flat.HTTPRequestAddMethod(b, method)
	flat.HTTPRequestAddUri(b, uri)
	request := flat.HTTPRequestEnd(b)
	flat.CallStart(b)
	flat.CallAddFunctionType(b, flat.FunctionHTTPRequest)
	flat.CallAddFunction(b, request)
	b.Finish(flat.CallEnd(b))

	p := packet.Make(testCode, packet.DomainCall, packet.HeaderSize+len(b.FinishedBytes()))
	copy(p.Content(), b.FinishedBytes())

	if !packet.IsValidCall(p, testCode) {
		t.Error(p)
	}

	c := make(chan packet.Buf, 1)
	i.Handle(context.Background(), c, p)
	p = <-c

	if !packet.IsValidCall(p, testCode) {
		t.Error(p)
	}

	r := flat.GetRootAsHTTPResponse(p, packet.HeaderSize)
	if r.StatusCode() != http.StatusCreated {
		t.Error(r.StatusCode())
	}
	if string(r.ContentType()) != "text/plain" {
		t.Errorf("%q", r.ContentType())
	}
	if r.ContentLength() != 13 {
		t.Error(r.ContentLength())
	}
	if r.BodyLength() != 13 {
		t.Error(r.BodyLength())
	}
	if !bytes.Equal(r.BodyBytes(), []byte("hellocalhost\n")) {
		t.Errorf("%q", r.BodyBytes())
	}
	if r.BodyStreamId() != -1 {
		t.Error(r.BodyStreamId())
	}
}
