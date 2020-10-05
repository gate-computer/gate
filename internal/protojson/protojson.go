// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protojson

import (
	"io"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func Marshal(m proto.Message) ([]byte, error) {
	return protojson.Marshal(m)
}

func MustMarshal(m proto.Message) []byte {
	b, err := Marshal(m)
	if err != nil {
		panic(err)
	}

	return b
}

func Write(w io.Writer, m proto.Message) error {
	b, err := Marshal(m)
	if err != nil {
		return err
	}

	_, err = w.Write(b)
	return err
}
