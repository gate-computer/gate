// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localhost

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gate.computer/gate/packet"
	"gate.computer/gate/service"
	"gate.computer/localhost/flat"
	flatbuffers "github.com/google/flatbuffers/go"
)

const testMaxSendSize = 65536

var testCode = packet.Code(time.Now().UnixNano() & 0x7fff)

func TestRequest(t *testing.T) {
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

	inst := newInstance(&Localhost{u.Scheme, u.Host, s.Client()}, service.InstanceConfig{
		Service: packet.Service{
			MaxSendSize: testMaxSendSize,
			Code:        testCode,
		},
	})

	b := flatbuffers.NewBuilder(0)
	method := b.CreateString(http.MethodGet)
	uri := b.CreateString("//bogus/test")
	flat.RequestStart(b)
	flat.RequestAddMethod(b, method)
	flat.RequestAddUri(b, uri)
	request := flat.RequestEnd(b)
	flat.CallStart(b)
	flat.CallAddFunctionType(b, flat.FunctionRequest)
	flat.CallAddFunction(b, request)
	b.Finish(flat.CallEnd(b))

	p := packet.Make(testCode, packet.DomainCall, packet.HeaderSize+len(b.FinishedBytes()))
	copy(p.Content(), b.FinishedBytes())

	if !packet.IsValidCall(p, testCode) {
		t.Error(p)
	}

	c := make(chan packet.Thunk, 1)
	if err := inst.Start(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}

	p, err = inst.Handle(context.Background(), c, p)
	if err != nil {
		t.Fatal(err)
	}
	for len(p) == 0 {
		p = (<-c)()
	}

	if !packet.IsValidCall(p, testCode) {
		t.Error(p)
	}

	r := flat.GetRootAsResponse(p, packet.HeaderSize)
	if r.StatusCode() != http.StatusCreated {
		t.Error(r.StatusCode())
	}
	if string(r.ContentType()) != "text/plain" {
		t.Errorf("%q", r.ContentType())
	}
	if r.BodyLength() != 13 {
		t.Error(r.BodyLength())
	}
	if !bytes.Equal(r.BodyBytes(), []byte("hellocalhost\n")) {
		t.Errorf("%q", r.BodyBytes())
	}
}
