// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
	"github.com/gogo/protobuf/vanity"
	"github.com/gogo/protobuf/vanity/command"
)

var filenamesWithoutXXX = map[string]struct{}{
	"server/api/server.proto": struct{}{},
}

func main() {
	req := command.Read()
	files := req.GetProtoFile()

	vanity.ForEachFile(files, vanity.TurnOffGoGettersAll)
	vanity.ForEachFile(files, vanity.TurnOffGoUnrecognizedAll)
	vanity.ForEachFile(files, vanity.TurnOnMarshalerAll)
	vanity.ForEachFile(files, vanity.TurnOnSizerAll)
	vanity.ForEachFile(files, vanity.TurnOnUnmarshalerAll)

	for _, file := range files {
		if _, found := filenamesWithoutXXX[*file.Name]; found {
			vanity.TurnOffGoSizecacheAll(file)
			vanity.TurnOffGoUnkeyedAll(file)
		}

		for _, msg := range file.MessageType {
			for _, field := range msg.Field {
				if *field.Type == descriptor.FieldDescriptorProto_TYPE_MESSAGE {
					vanity.TurnOffNullable(field)
				}
			}
		}
	}

	resp := command.Generate(req)
	command.Write(resp)
}
