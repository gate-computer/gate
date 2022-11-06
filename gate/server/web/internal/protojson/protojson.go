// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protojson

import (
	"io"
	"io/ioutil"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func Decode(r io.Reader, m proto.Message) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	return protojson.Unmarshal(b, m)
}

func MustMarshal(m proto.Message) []byte {
	b, err := protojson.Marshal(m)
	if err != nil {
		panic(err)
	}

	return b
}
