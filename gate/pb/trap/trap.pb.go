// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.1
// 	protoc        (unknown)
// source: gate/pb/trap/trap.proto

package trap

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type ID int32

const (
	// These correspond to constants in Go package gate.computer/wag/trap:
	ID_EXIT                              ID = 0
	ID_NO_FUNCTION                       ID = 1
	ID_SUSPENDED                         ID = 2
	ID_UNREACHABLE                       ID = 3
	ID_CALL_STACK_EXHAUSTED              ID = 4
	ID_MEMORY_ACCESS_OUT_OF_BOUNDS       ID = 5
	ID_INDIRECT_CALL_INDEX_OUT_OF_BOUNDS ID = 6
	ID_INDIRECT_CALL_SIGNATURE_MISMATCH  ID = 7
	ID_INTEGER_DIVIDE_BY_ZERO            ID = 8
	ID_INTEGER_OVERFLOW                  ID = 9
	ID_BREAKPOINT                        ID = 10
	// Gate-specific:
	ID_ABI_DEFICIENCY ID = 27
	ID_ABI_VIOLATION  ID = 28
	ID_INTERNAL_ERROR ID = 29
	ID_KILLED         ID = 30
)

// Enum value maps for ID.
var (
	ID_name = map[int32]string{
		0:  "EXIT",
		1:  "NO_FUNCTION",
		2:  "SUSPENDED",
		3:  "UNREACHABLE",
		4:  "CALL_STACK_EXHAUSTED",
		5:  "MEMORY_ACCESS_OUT_OF_BOUNDS",
		6:  "INDIRECT_CALL_INDEX_OUT_OF_BOUNDS",
		7:  "INDIRECT_CALL_SIGNATURE_MISMATCH",
		8:  "INTEGER_DIVIDE_BY_ZERO",
		9:  "INTEGER_OVERFLOW",
		10: "BREAKPOINT",
		27: "ABI_DEFICIENCY",
		28: "ABI_VIOLATION",
		29: "INTERNAL_ERROR",
		30: "KILLED",
	}
	ID_value = map[string]int32{
		"EXIT":                              0,
		"NO_FUNCTION":                       1,
		"SUSPENDED":                         2,
		"UNREACHABLE":                       3,
		"CALL_STACK_EXHAUSTED":              4,
		"MEMORY_ACCESS_OUT_OF_BOUNDS":       5,
		"INDIRECT_CALL_INDEX_OUT_OF_BOUNDS": 6,
		"INDIRECT_CALL_SIGNATURE_MISMATCH":  7,
		"INTEGER_DIVIDE_BY_ZERO":            8,
		"INTEGER_OVERFLOW":                  9,
		"BREAKPOINT":                        10,
		"ABI_DEFICIENCY":                    27,
		"ABI_VIOLATION":                     28,
		"INTERNAL_ERROR":                    29,
		"KILLED":                            30,
	}
)

func (x ID) Enum() *ID {
	p := new(ID)
	*p = x
	return p
}

func (x ID) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (ID) Descriptor() protoreflect.EnumDescriptor {
	return file_gate_pb_trap_trap_proto_enumTypes[0].Descriptor()
}

func (ID) Type() protoreflect.EnumType {
	return &file_gate_pb_trap_trap_proto_enumTypes[0]
}

func (x ID) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use ID.Descriptor instead.
func (ID) EnumDescriptor() ([]byte, []int) {
	return file_gate_pb_trap_trap_proto_rawDescGZIP(), []int{0}
}

var File_gate_pb_trap_trap_proto protoreflect.FileDescriptor

var file_gate_pb_trap_trap_proto_rawDesc = []byte{
	0x0a, 0x17, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x70, 0x62, 0x2f, 0x74, 0x72, 0x61, 0x70, 0x2f, 0x74,
	0x72, 0x61, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x09, 0x67, 0x61, 0x74, 0x65, 0x2e,
	0x74, 0x72, 0x61, 0x70, 0x2a, 0xd0, 0x02, 0x0a, 0x02, 0x49, 0x44, 0x12, 0x08, 0x0a, 0x04, 0x45,
	0x58, 0x49, 0x54, 0x10, 0x00, 0x12, 0x0f, 0x0a, 0x0b, 0x4e, 0x4f, 0x5f, 0x46, 0x55, 0x4e, 0x43,
	0x54, 0x49, 0x4f, 0x4e, 0x10, 0x01, 0x12, 0x0d, 0x0a, 0x09, 0x53, 0x55, 0x53, 0x50, 0x45, 0x4e,
	0x44, 0x45, 0x44, 0x10, 0x02, 0x12, 0x0f, 0x0a, 0x0b, 0x55, 0x4e, 0x52, 0x45, 0x41, 0x43, 0x48,
	0x41, 0x42, 0x4c, 0x45, 0x10, 0x03, 0x12, 0x18, 0x0a, 0x14, 0x43, 0x41, 0x4c, 0x4c, 0x5f, 0x53,
	0x54, 0x41, 0x43, 0x4b, 0x5f, 0x45, 0x58, 0x48, 0x41, 0x55, 0x53, 0x54, 0x45, 0x44, 0x10, 0x04,
	0x12, 0x1f, 0x0a, 0x1b, 0x4d, 0x45, 0x4d, 0x4f, 0x52, 0x59, 0x5f, 0x41, 0x43, 0x43, 0x45, 0x53,
	0x53, 0x5f, 0x4f, 0x55, 0x54, 0x5f, 0x4f, 0x46, 0x5f, 0x42, 0x4f, 0x55, 0x4e, 0x44, 0x53, 0x10,
	0x05, 0x12, 0x25, 0x0a, 0x21, 0x49, 0x4e, 0x44, 0x49, 0x52, 0x45, 0x43, 0x54, 0x5f, 0x43, 0x41,
	0x4c, 0x4c, 0x5f, 0x49, 0x4e, 0x44, 0x45, 0x58, 0x5f, 0x4f, 0x55, 0x54, 0x5f, 0x4f, 0x46, 0x5f,
	0x42, 0x4f, 0x55, 0x4e, 0x44, 0x53, 0x10, 0x06, 0x12, 0x24, 0x0a, 0x20, 0x49, 0x4e, 0x44, 0x49,
	0x52, 0x45, 0x43, 0x54, 0x5f, 0x43, 0x41, 0x4c, 0x4c, 0x5f, 0x53, 0x49, 0x47, 0x4e, 0x41, 0x54,
	0x55, 0x52, 0x45, 0x5f, 0x4d, 0x49, 0x53, 0x4d, 0x41, 0x54, 0x43, 0x48, 0x10, 0x07, 0x12, 0x1a,
	0x0a, 0x16, 0x49, 0x4e, 0x54, 0x45, 0x47, 0x45, 0x52, 0x5f, 0x44, 0x49, 0x56, 0x49, 0x44, 0x45,
	0x5f, 0x42, 0x59, 0x5f, 0x5a, 0x45, 0x52, 0x4f, 0x10, 0x08, 0x12, 0x14, 0x0a, 0x10, 0x49, 0x4e,
	0x54, 0x45, 0x47, 0x45, 0x52, 0x5f, 0x4f, 0x56, 0x45, 0x52, 0x46, 0x4c, 0x4f, 0x57, 0x10, 0x09,
	0x12, 0x0e, 0x0a, 0x0a, 0x42, 0x52, 0x45, 0x41, 0x4b, 0x50, 0x4f, 0x49, 0x4e, 0x54, 0x10, 0x0a,
	0x12, 0x12, 0x0a, 0x0e, 0x41, 0x42, 0x49, 0x5f, 0x44, 0x45, 0x46, 0x49, 0x43, 0x49, 0x45, 0x4e,
	0x43, 0x59, 0x10, 0x1b, 0x12, 0x11, 0x0a, 0x0d, 0x41, 0x42, 0x49, 0x5f, 0x56, 0x49, 0x4f, 0x4c,
	0x41, 0x54, 0x49, 0x4f, 0x4e, 0x10, 0x1c, 0x12, 0x12, 0x0a, 0x0e, 0x49, 0x4e, 0x54, 0x45, 0x52,
	0x4e, 0x41, 0x4c, 0x5f, 0x45, 0x52, 0x52, 0x4f, 0x52, 0x10, 0x1d, 0x12, 0x0a, 0x0a, 0x06, 0x4b,
	0x49, 0x4c, 0x4c, 0x45, 0x44, 0x10, 0x1e, 0x42, 0x1c, 0x5a, 0x1a, 0x67, 0x61, 0x74, 0x65, 0x2e,
	0x63, 0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65, 0x72, 0x2f, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x70, 0x62,
	0x2f, 0x74, 0x72, 0x61, 0x70, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_gate_pb_trap_trap_proto_rawDescOnce sync.Once
	file_gate_pb_trap_trap_proto_rawDescData = file_gate_pb_trap_trap_proto_rawDesc
)

func file_gate_pb_trap_trap_proto_rawDescGZIP() []byte {
	file_gate_pb_trap_trap_proto_rawDescOnce.Do(func() {
		file_gate_pb_trap_trap_proto_rawDescData = protoimpl.X.CompressGZIP(file_gate_pb_trap_trap_proto_rawDescData)
	})
	return file_gate_pb_trap_trap_proto_rawDescData
}

var file_gate_pb_trap_trap_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_gate_pb_trap_trap_proto_goTypes = []any{
	(ID)(0), // 0: gate.trap.ID
}
var file_gate_pb_trap_trap_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_gate_pb_trap_trap_proto_init() }
func file_gate_pb_trap_trap_proto_init() {
	if File_gate_pb_trap_trap_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_gate_pb_trap_trap_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   0,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_gate_pb_trap_trap_proto_goTypes,
		DependencyIndexes: file_gate_pb_trap_trap_proto_depIdxs,
		EnumInfos:         file_gate_pb_trap_trap_proto_enumTypes,
	}.Build()
	File_gate_pb_trap_trap_proto = out.File
	file_gate_pb_trap_trap_proto_rawDesc = nil
	file_gate_pb_trap_trap_proto_goTypes = nil
	file_gate_pb_trap_trap_proto_depIdxs = nil
}
