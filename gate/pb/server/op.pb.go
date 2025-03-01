// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.4
// 	protoc        (unknown)
// source: gate/pb/server/op.proto

package server

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Op int32

const (
	Op_UNSPECIFIED       Op = 0
	Op_MODULE_LIST       Op = 1
	Op_MODULE_INFO       Op = 2
	Op_MODULE_DOWNLOAD   Op = 3
	Op_MODULE_UPLOAD     Op = 4
	Op_MODULE_SOURCE     Op = 5
	Op_MODULE_PIN        Op = 6
	Op_MODULE_UNPIN      Op = 7
	Op_CALL_EXTANT       Op = 8
	Op_CALL_UPLOAD       Op = 9
	Op_CALL_SOURCE       Op = 10
	Op_LAUNCH_EXTANT     Op = 11
	Op_LAUNCH_UPLOAD     Op = 12
	Op_LAUNCH_SOURCE     Op = 13
	Op_INSTANCE_LIST     Op = 14
	Op_INSTANCE_INFO     Op = 15
	Op_INSTANCE_CONNECT  Op = 16
	Op_INSTANCE_WAIT     Op = 17
	Op_INSTANCE_KILL     Op = 18
	Op_INSTANCE_SUSPEND  Op = 19
	Op_INSTANCE_RESUME   Op = 20
	Op_INSTANCE_SNAPSHOT Op = 21
	Op_INSTANCE_DELETE   Op = 22
	Op_INSTANCE_UPDATE   Op = 23
	Op_INSTANCE_DEBUG    Op = 24
)

// Enum value maps for Op.
var (
	Op_name = map[int32]string{
		0:  "UNSPECIFIED",
		1:  "MODULE_LIST",
		2:  "MODULE_INFO",
		3:  "MODULE_DOWNLOAD",
		4:  "MODULE_UPLOAD",
		5:  "MODULE_SOURCE",
		6:  "MODULE_PIN",
		7:  "MODULE_UNPIN",
		8:  "CALL_EXTANT",
		9:  "CALL_UPLOAD",
		10: "CALL_SOURCE",
		11: "LAUNCH_EXTANT",
		12: "LAUNCH_UPLOAD",
		13: "LAUNCH_SOURCE",
		14: "INSTANCE_LIST",
		15: "INSTANCE_INFO",
		16: "INSTANCE_CONNECT",
		17: "INSTANCE_WAIT",
		18: "INSTANCE_KILL",
		19: "INSTANCE_SUSPEND",
		20: "INSTANCE_RESUME",
		21: "INSTANCE_SNAPSHOT",
		22: "INSTANCE_DELETE",
		23: "INSTANCE_UPDATE",
		24: "INSTANCE_DEBUG",
	}
	Op_value = map[string]int32{
		"UNSPECIFIED":       0,
		"MODULE_LIST":       1,
		"MODULE_INFO":       2,
		"MODULE_DOWNLOAD":   3,
		"MODULE_UPLOAD":     4,
		"MODULE_SOURCE":     5,
		"MODULE_PIN":        6,
		"MODULE_UNPIN":      7,
		"CALL_EXTANT":       8,
		"CALL_UPLOAD":       9,
		"CALL_SOURCE":       10,
		"LAUNCH_EXTANT":     11,
		"LAUNCH_UPLOAD":     12,
		"LAUNCH_SOURCE":     13,
		"INSTANCE_LIST":     14,
		"INSTANCE_INFO":     15,
		"INSTANCE_CONNECT":  16,
		"INSTANCE_WAIT":     17,
		"INSTANCE_KILL":     18,
		"INSTANCE_SUSPEND":  19,
		"INSTANCE_RESUME":   20,
		"INSTANCE_SNAPSHOT": 21,
		"INSTANCE_DELETE":   22,
		"INSTANCE_UPDATE":   23,
		"INSTANCE_DEBUG":    24,
	}
)

func (x Op) Enum() *Op {
	p := new(Op)
	*p = x
	return p
}

func (x Op) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Op) Descriptor() protoreflect.EnumDescriptor {
	return file_gate_pb_server_op_proto_enumTypes[0].Descriptor()
}

func (Op) Type() protoreflect.EnumType {
	return &file_gate_pb_server_op_proto_enumTypes[0]
}

func (x Op) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Op.Descriptor instead.
func (Op) EnumDescriptor() ([]byte, []int) {
	return file_gate_pb_server_op_proto_rawDescGZIP(), []int{0}
}

var File_gate_pb_server_op_proto protoreflect.FileDescriptor

var file_gate_pb_server_op_proto_rawDesc = string([]byte{
	0x0a, 0x17, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x70, 0x62, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x2f, 0x6f, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x10, 0x67, 0x61, 0x74, 0x65, 0x2e,
	0x67, 0x61, 0x74, 0x65, 0x2e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2a, 0xe2, 0x03, 0x0a, 0x02,
	0x4f, 0x70, 0x12, 0x0f, 0x0a, 0x0b, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45,
	0x44, 0x10, 0x00, 0x12, 0x0f, 0x0a, 0x0b, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45, 0x5f, 0x4c, 0x49,
	0x53, 0x54, 0x10, 0x01, 0x12, 0x0f, 0x0a, 0x0b, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45, 0x5f, 0x49,
	0x4e, 0x46, 0x4f, 0x10, 0x02, 0x12, 0x13, 0x0a, 0x0f, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45, 0x5f,
	0x44, 0x4f, 0x57, 0x4e, 0x4c, 0x4f, 0x41, 0x44, 0x10, 0x03, 0x12, 0x11, 0x0a, 0x0d, 0x4d, 0x4f,
	0x44, 0x55, 0x4c, 0x45, 0x5f, 0x55, 0x50, 0x4c, 0x4f, 0x41, 0x44, 0x10, 0x04, 0x12, 0x11, 0x0a,
	0x0d, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45, 0x5f, 0x53, 0x4f, 0x55, 0x52, 0x43, 0x45, 0x10, 0x05,
	0x12, 0x0e, 0x0a, 0x0a, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45, 0x5f, 0x50, 0x49, 0x4e, 0x10, 0x06,
	0x12, 0x10, 0x0a, 0x0c, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45, 0x5f, 0x55, 0x4e, 0x50, 0x49, 0x4e,
	0x10, 0x07, 0x12, 0x0f, 0x0a, 0x0b, 0x43, 0x41, 0x4c, 0x4c, 0x5f, 0x45, 0x58, 0x54, 0x41, 0x4e,
	0x54, 0x10, 0x08, 0x12, 0x0f, 0x0a, 0x0b, 0x43, 0x41, 0x4c, 0x4c, 0x5f, 0x55, 0x50, 0x4c, 0x4f,
	0x41, 0x44, 0x10, 0x09, 0x12, 0x0f, 0x0a, 0x0b, 0x43, 0x41, 0x4c, 0x4c, 0x5f, 0x53, 0x4f, 0x55,
	0x52, 0x43, 0x45, 0x10, 0x0a, 0x12, 0x11, 0x0a, 0x0d, 0x4c, 0x41, 0x55, 0x4e, 0x43, 0x48, 0x5f,
	0x45, 0x58, 0x54, 0x41, 0x4e, 0x54, 0x10, 0x0b, 0x12, 0x11, 0x0a, 0x0d, 0x4c, 0x41, 0x55, 0x4e,
	0x43, 0x48, 0x5f, 0x55, 0x50, 0x4c, 0x4f, 0x41, 0x44, 0x10, 0x0c, 0x12, 0x11, 0x0a, 0x0d, 0x4c,
	0x41, 0x55, 0x4e, 0x43, 0x48, 0x5f, 0x53, 0x4f, 0x55, 0x52, 0x43, 0x45, 0x10, 0x0d, 0x12, 0x11,
	0x0a, 0x0d, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x4c, 0x49, 0x53, 0x54, 0x10,
	0x0e, 0x12, 0x11, 0x0a, 0x0d, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x49, 0x4e,
	0x46, 0x4f, 0x10, 0x0f, 0x12, 0x14, 0x0a, 0x10, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45,
	0x5f, 0x43, 0x4f, 0x4e, 0x4e, 0x45, 0x43, 0x54, 0x10, 0x10, 0x12, 0x11, 0x0a, 0x0d, 0x49, 0x4e,
	0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x57, 0x41, 0x49, 0x54, 0x10, 0x11, 0x12, 0x11, 0x0a,
	0x0d, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x4b, 0x49, 0x4c, 0x4c, 0x10, 0x12,
	0x12, 0x14, 0x0a, 0x10, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x53, 0x55, 0x53,
	0x50, 0x45, 0x4e, 0x44, 0x10, 0x13, 0x12, 0x13, 0x0a, 0x0f, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e,
	0x43, 0x45, 0x5f, 0x52, 0x45, 0x53, 0x55, 0x4d, 0x45, 0x10, 0x14, 0x12, 0x15, 0x0a, 0x11, 0x49,
	0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x53, 0x4e, 0x41, 0x50, 0x53, 0x48, 0x4f, 0x54,
	0x10, 0x15, 0x12, 0x13, 0x0a, 0x0f, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x44,
	0x45, 0x4c, 0x45, 0x54, 0x45, 0x10, 0x16, 0x12, 0x13, 0x0a, 0x0f, 0x49, 0x4e, 0x53, 0x54, 0x41,
	0x4e, 0x43, 0x45, 0x5f, 0x55, 0x50, 0x44, 0x41, 0x54, 0x45, 0x10, 0x17, 0x12, 0x12, 0x0a, 0x0e,
	0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x44, 0x45, 0x42, 0x55, 0x47, 0x10, 0x18,
	0x42, 0x1e, 0x5a, 0x1c, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x63, 0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65,
	0x72, 0x2f, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x70, 0x62, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_gate_pb_server_op_proto_rawDescOnce sync.Once
	file_gate_pb_server_op_proto_rawDescData []byte
)

func file_gate_pb_server_op_proto_rawDescGZIP() []byte {
	file_gate_pb_server_op_proto_rawDescOnce.Do(func() {
		file_gate_pb_server_op_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_gate_pb_server_op_proto_rawDesc), len(file_gate_pb_server_op_proto_rawDesc)))
	})
	return file_gate_pb_server_op_proto_rawDescData
}

var file_gate_pb_server_op_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_gate_pb_server_op_proto_goTypes = []any{
	(Op)(0), // 0: gate.gate.server.Op
}
var file_gate_pb_server_op_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_gate_pb_server_op_proto_init() }
func file_gate_pb_server_op_proto_init() {
	if File_gate_pb_server_op_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_gate_pb_server_op_proto_rawDesc), len(file_gate_pb_server_op_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   0,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_gate_pb_server_op_proto_goTypes,
		DependencyIndexes: file_gate_pb_server_op_proto_depIdxs,
		EnumInfos:         file_gate_pb_server_op_proto_enumTypes,
	}.Build()
	File_gate_pb_server_op_proto = out.File
	file_gate_pb_server_op_proto_goTypes = nil
	file_gate_pb_server_op_proto_depIdxs = nil
}
