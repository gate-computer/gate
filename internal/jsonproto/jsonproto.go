// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonproto

import (
	"bytes"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
)

var Marshaler = jsonpb.Marshaler{
	OrigName: true,
}

func Marshal(x proto.Message) (b []byte, err error) {
	var buf bytes.Buffer

	err = Marshaler.Marshal(&buf, x)
	if err != nil {
		return
	}

	b = buf.Bytes()
	return
}

func MustMarshal(x proto.Message) (b []byte) {
	b, err := Marshal(x)
	if err != nil {
		panic(err)
	}
	return
}
