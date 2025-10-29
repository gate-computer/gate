// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localhost

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gate.computer/gate/packet"
	"gate.computer/gate/service"
	"gate.computer/localhost/internal/flat"
	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/stretchr/testify/assert"

	. "import.name/testing/mustr"
)

const testMaxSendSize = 65536

var testCode = packet.Code(time.Now().UnixNano() & 0x7fff)

func TestRequest(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.Host, "bogus")
		assert.Equal(t, r.Method, http.MethodGet)
		assert.Equal(t, r.URL.Path, "/test")

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, "hellocalhost")
	}))
	defer s.Close()

	u := Must(t, R(url.Parse(s.URL)))

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

	p := append(packet.MakeCall(testCode, 0), b.FinishedBytes()...)

	c := make(chan packet.Thunk, 1)
	if err := inst.Start(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}

	p = Must(t, R(inst.Handle(context.Background(), c, p)))
	for len(p) == 0 {
		p = Must(t, R((<-c)()))
	}
	assert.True(t, packet.IsValidCall(p, testCode))

	r := flat.GetRootAsResponse(p, packet.HeaderSize)
	assert.Equal(t, int(r.StatusCode()), http.StatusCreated)
	assert.Equal(t, string(r.ContentType()), "text/plain")
	assert.Equal(t, r.BodyLength(), 13)
	assert.Equal(t, r.BodyBytes(), []byte("hellocalhost\n"))
}
