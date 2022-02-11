// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.25.0
// 	protoc        v3.6.1
// source: server/api/pb/meta.proto

package pb

import (
	proto "github.com/golang/protobuf/proto"
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

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

type Iface int32

const (
	Iface_DEFAULT Iface = 0
)

// Enum value maps for Iface.
var (
	Iface_name = map[int32]string{
		0: "DEFAULT",
	}
	Iface_value = map[string]int32{
		"DEFAULT": 0,
	}
)

func (x Iface) Enum() *Iface {
	p := new(Iface)
	*p = x
	return p
}

func (x Iface) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Iface) Descriptor() protoreflect.EnumDescriptor {
	return file_server_api_pb_meta_proto_enumTypes[0].Descriptor()
}

func (Iface) Type() protoreflect.EnumType {
	return &file_server_api_pb_meta_proto_enumTypes[0]
}

func (x Iface) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Iface.Descriptor instead.
func (Iface) EnumDescriptor() ([]byte, []int) {
	return file_server_api_pb_meta_proto_rawDescGZIP(), []int{0}
}

type Op int32

const (
	Op_UNKNOWN           Op = 0
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
		0:  "UNKNOWN",
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
		"UNKNOWN":           0,
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
	return file_server_api_pb_meta_proto_enumTypes[1].Descriptor()
}

func (Op) Type() protoreflect.EnumType {
	return &file_server_api_pb_meta_proto_enumTypes[1]
}

func (x Op) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Op.Descriptor instead.
func (Op) EnumDescriptor() ([]byte, []int) {
	return file_server_api_pb_meta_proto_rawDescGZIP(), []int{1}
}

type Meta struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Iface     Iface  `protobuf:"varint,1,opt,name=iface,proto3,enum=gate.server.Iface" json:"iface,omitempty"`
	Req       uint64 `protobuf:"varint,2,opt,name=req,proto3" json:"req,omitempty"`
	Addr      string `protobuf:"bytes,3,opt,name=addr,proto3" json:"addr,omitempty"`
	Op        Op     `protobuf:"varint,4,opt,name=op,proto3,enum=gate.server.Op" json:"op,omitempty"`
	Principal string `protobuf:"bytes,5,opt,name=principal,proto3" json:"principal,omitempty"`
}

func (x *Meta) Reset() {
	*x = Meta{}
	if protoimpl.UnsafeEnabled {
		mi := &file_server_api_pb_meta_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Meta) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Meta) ProtoMessage() {}

func (x *Meta) ProtoReflect() protoreflect.Message {
	mi := &file_server_api_pb_meta_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Meta.ProtoReflect.Descriptor instead.
func (*Meta) Descriptor() ([]byte, []int) {
	return file_server_api_pb_meta_proto_rawDescGZIP(), []int{0}
}

func (x *Meta) GetIface() Iface {
	if x != nil {
		return x.Iface
	}
	return Iface_DEFAULT
}

func (x *Meta) GetReq() uint64 {
	if x != nil {
		return x.Req
	}
	return 0
}

func (x *Meta) GetAddr() string {
	if x != nil {
		return x.Addr
	}
	return ""
}

func (x *Meta) GetOp() Op {
	if x != nil {
		return x.Op
	}
	return Op_UNKNOWN
}

func (x *Meta) GetPrincipal() string {
	if x != nil {
		return x.Principal
	}
	return ""
}

var File_server_api_pb_meta_proto protoreflect.FileDescriptor

var file_server_api_pb_meta_proto_rawDesc = []byte{
	0x0a, 0x18, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x70, 0x62, 0x2f,
	0x6d, 0x65, 0x74, 0x61, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0b, 0x67, 0x61, 0x74, 0x65,
	0x2e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x22, 0x95, 0x01, 0x0a, 0x04, 0x4d, 0x65, 0x74, 0x61,
	0x12, 0x28, 0x0a, 0x05, 0x69, 0x66, 0x61, 0x63, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32,
	0x12, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x49, 0x66,
	0x61, 0x63, 0x65, 0x52, 0x05, 0x69, 0x66, 0x61, 0x63, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x72, 0x65,
	0x71, 0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x03, 0x72, 0x65, 0x71, 0x12, 0x12, 0x0a, 0x04,
	0x61, 0x64, 0x64, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x61, 0x64, 0x64, 0x72,
	0x12, 0x1f, 0x0a, 0x02, 0x6f, 0x70, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x0f, 0x2e, 0x67,
	0x61, 0x74, 0x65, 0x2e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x4f, 0x70, 0x52, 0x02, 0x6f,
	0x70, 0x12, 0x1c, 0x0a, 0x09, 0x70, 0x72, 0x69, 0x6e, 0x63, 0x69, 0x70, 0x61, 0x6c, 0x18, 0x05,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x70, 0x72, 0x69, 0x6e, 0x63, 0x69, 0x70, 0x61, 0x6c, 0x2a,
	0x14, 0x0a, 0x05, 0x49, 0x66, 0x61, 0x63, 0x65, 0x12, 0x0b, 0x0a, 0x07, 0x44, 0x45, 0x46, 0x41,
	0x55, 0x4c, 0x54, 0x10, 0x00, 0x2a, 0xde, 0x03, 0x0a, 0x02, 0x4f, 0x70, 0x12, 0x0b, 0x0a, 0x07,
	0x55, 0x4e, 0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x10, 0x00, 0x12, 0x0f, 0x0a, 0x0b, 0x4d, 0x4f, 0x44,
	0x55, 0x4c, 0x45, 0x5f, 0x4c, 0x49, 0x53, 0x54, 0x10, 0x01, 0x12, 0x0f, 0x0a, 0x0b, 0x4d, 0x4f,
	0x44, 0x55, 0x4c, 0x45, 0x5f, 0x49, 0x4e, 0x46, 0x4f, 0x10, 0x02, 0x12, 0x13, 0x0a, 0x0f, 0x4d,
	0x4f, 0x44, 0x55, 0x4c, 0x45, 0x5f, 0x44, 0x4f, 0x57, 0x4e, 0x4c, 0x4f, 0x41, 0x44, 0x10, 0x03,
	0x12, 0x11, 0x0a, 0x0d, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45, 0x5f, 0x55, 0x50, 0x4c, 0x4f, 0x41,
	0x44, 0x10, 0x04, 0x12, 0x11, 0x0a, 0x0d, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45, 0x5f, 0x53, 0x4f,
	0x55, 0x52, 0x43, 0x45, 0x10, 0x05, 0x12, 0x0e, 0x0a, 0x0a, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45,
	0x5f, 0x50, 0x49, 0x4e, 0x10, 0x06, 0x12, 0x10, 0x0a, 0x0c, 0x4d, 0x4f, 0x44, 0x55, 0x4c, 0x45,
	0x5f, 0x55, 0x4e, 0x50, 0x49, 0x4e, 0x10, 0x07, 0x12, 0x0f, 0x0a, 0x0b, 0x43, 0x41, 0x4c, 0x4c,
	0x5f, 0x45, 0x58, 0x54, 0x41, 0x4e, 0x54, 0x10, 0x08, 0x12, 0x0f, 0x0a, 0x0b, 0x43, 0x41, 0x4c,
	0x4c, 0x5f, 0x55, 0x50, 0x4c, 0x4f, 0x41, 0x44, 0x10, 0x09, 0x12, 0x0f, 0x0a, 0x0b, 0x43, 0x41,
	0x4c, 0x4c, 0x5f, 0x53, 0x4f, 0x55, 0x52, 0x43, 0x45, 0x10, 0x0a, 0x12, 0x11, 0x0a, 0x0d, 0x4c,
	0x41, 0x55, 0x4e, 0x43, 0x48, 0x5f, 0x45, 0x58, 0x54, 0x41, 0x4e, 0x54, 0x10, 0x0b, 0x12, 0x11,
	0x0a, 0x0d, 0x4c, 0x41, 0x55, 0x4e, 0x43, 0x48, 0x5f, 0x55, 0x50, 0x4c, 0x4f, 0x41, 0x44, 0x10,
	0x0c, 0x12, 0x11, 0x0a, 0x0d, 0x4c, 0x41, 0x55, 0x4e, 0x43, 0x48, 0x5f, 0x53, 0x4f, 0x55, 0x52,
	0x43, 0x45, 0x10, 0x0d, 0x12, 0x11, 0x0a, 0x0d, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45,
	0x5f, 0x4c, 0x49, 0x53, 0x54, 0x10, 0x0e, 0x12, 0x11, 0x0a, 0x0d, 0x49, 0x4e, 0x53, 0x54, 0x41,
	0x4e, 0x43, 0x45, 0x5f, 0x49, 0x4e, 0x46, 0x4f, 0x10, 0x0f, 0x12, 0x14, 0x0a, 0x10, 0x49, 0x4e,
	0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x43, 0x4f, 0x4e, 0x4e, 0x45, 0x43, 0x54, 0x10, 0x10,
	0x12, 0x11, 0x0a, 0x0d, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x57, 0x41, 0x49,
	0x54, 0x10, 0x11, 0x12, 0x11, 0x0a, 0x0d, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f,
	0x4b, 0x49, 0x4c, 0x4c, 0x10, 0x12, 0x12, 0x14, 0x0a, 0x10, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e,
	0x43, 0x45, 0x5f, 0x53, 0x55, 0x53, 0x50, 0x45, 0x4e, 0x44, 0x10, 0x13, 0x12, 0x13, 0x0a, 0x0f,
	0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x52, 0x45, 0x53, 0x55, 0x4d, 0x45, 0x10,
	0x14, 0x12, 0x15, 0x0a, 0x11, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x53, 0x4e,
	0x41, 0x50, 0x53, 0x48, 0x4f, 0x54, 0x10, 0x15, 0x12, 0x13, 0x0a, 0x0f, 0x49, 0x4e, 0x53, 0x54,
	0x41, 0x4e, 0x43, 0x45, 0x5f, 0x44, 0x45, 0x4c, 0x45, 0x54, 0x45, 0x10, 0x16, 0x12, 0x13, 0x0a,
	0x0f, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x55, 0x50, 0x44, 0x41, 0x54, 0x45,
	0x10, 0x17, 0x12, 0x12, 0x0a, 0x0e, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x44,
	0x45, 0x42, 0x55, 0x47, 0x10, 0x18, 0x42, 0x22, 0x5a, 0x20, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x63,
	0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65, 0x72, 0x2f, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x73, 0x65, 0x72,
	0x76, 0x65, 0x72, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
}

var (
	file_server_api_pb_meta_proto_rawDescOnce sync.Once
	file_server_api_pb_meta_proto_rawDescData = file_server_api_pb_meta_proto_rawDesc
)

func file_server_api_pb_meta_proto_rawDescGZIP() []byte {
	file_server_api_pb_meta_proto_rawDescOnce.Do(func() {
		file_server_api_pb_meta_proto_rawDescData = protoimpl.X.CompressGZIP(file_server_api_pb_meta_proto_rawDescData)
	})
	return file_server_api_pb_meta_proto_rawDescData
}

var file_server_api_pb_meta_proto_enumTypes = make([]protoimpl.EnumInfo, 2)
var file_server_api_pb_meta_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_server_api_pb_meta_proto_goTypes = []interface{}{
	(Iface)(0),   // 0: gate.server.Iface
	(Op)(0),      // 1: gate.server.Op
	(*Meta)(nil), // 2: gate.server.Meta
}
var file_server_api_pb_meta_proto_depIdxs = []int32{
	0, // 0: gate.server.Meta.iface:type_name -> gate.server.Iface
	1, // 1: gate.server.Meta.op:type_name -> gate.server.Op
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_server_api_pb_meta_proto_init() }
func file_server_api_pb_meta_proto_init() {
	if File_server_api_pb_meta_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_server_api_pb_meta_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Meta); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_server_api_pb_meta_proto_rawDesc,
			NumEnums:      2,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_server_api_pb_meta_proto_goTypes,
		DependencyIndexes: file_server_api_pb_meta_proto_depIdxs,
		EnumInfos:         file_server_api_pb_meta_proto_enumTypes,
		MessageInfos:      file_server_api_pb_meta_proto_msgTypes,
	}.Build()
	File_server_api_pb_meta_proto = out.File
	file_server_api_pb_meta_proto_rawDesc = nil
	file_server_api_pb_meta_proto_goTypes = nil
	file_server_api_pb_meta_proto_depIdxs = nil
}