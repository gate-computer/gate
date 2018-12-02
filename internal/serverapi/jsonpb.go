// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package serverapi

import (
	"bytes"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
)

var JSONMarshaler = jsonpb.Marshaler{
	OrigName: true,
}

func MarshalJSON(x proto.Message) (b []byte, err error) {
	var buf bytes.Buffer

	err = JSONMarshaler.Marshal(&buf, x)
	if err != nil {
		return
	}

	b = buf.Bytes()
	return
}
