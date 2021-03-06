// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.25.0
// 	protoc        v3.6.1
// source: internal/webserverapi/webserverapi.proto

package webserverapi

import (
	api "gate.computer/gate/server/api"
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

type IOConnection struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Connected bool `protobuf:"varint,1,opt,name=connected,proto3" json:"connected,omitempty"`
}

func (x *IOConnection) Reset() {
	*x = IOConnection{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_webserverapi_webserverapi_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *IOConnection) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*IOConnection) ProtoMessage() {}

func (x *IOConnection) ProtoReflect() protoreflect.Message {
	mi := &file_internal_webserverapi_webserverapi_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use IOConnection.ProtoReflect.Descriptor instead.
func (*IOConnection) Descriptor() ([]byte, []int) {
	return file_internal_webserverapi_webserverapi_proto_rawDescGZIP(), []int{0}
}

func (x *IOConnection) GetConnected() bool {
	if x != nil {
		return x.Connected
	}
	return false
}

type ConnectionStatus struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Status *api.Status `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	Input  bool        `protobuf:"varint,2,opt,name=input,proto3" json:"input,omitempty"`
}

func (x *ConnectionStatus) Reset() {
	*x = ConnectionStatus{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_webserverapi_webserverapi_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ConnectionStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ConnectionStatus) ProtoMessage() {}

func (x *ConnectionStatus) ProtoReflect() protoreflect.Message {
	mi := &file_internal_webserverapi_webserverapi_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ConnectionStatus.ProtoReflect.Descriptor instead.
func (*ConnectionStatus) Descriptor() ([]byte, []int) {
	return file_internal_webserverapi_webserverapi_proto_rawDescGZIP(), []int{1}
}

func (x *ConnectionStatus) GetStatus() *api.Status {
	if x != nil {
		return x.Status
	}
	return nil
}

func (x *ConnectionStatus) GetInput() bool {
	if x != nil {
		return x.Input
	}
	return false
}

var File_internal_webserverapi_webserverapi_proto protoreflect.FileDescriptor

var file_internal_webserverapi_webserverapi_proto_rawDesc = []byte{
	0x0a, 0x28, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x77, 0x65, 0x62, 0x73, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x61, 0x70, 0x69, 0x2f, 0x77, 0x65, 0x62, 0x73, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x61, 0x70, 0x69, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x1a, 0x67, 0x61, 0x74, 0x65,
	0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x77, 0x65, 0x62, 0x73, 0x65, 0x72,
	0x76, 0x65, 0x72, 0x61, 0x70, 0x69, 0x1a, 0x17, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x61,
	0x70, 0x69, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22,
	0x2c, 0x0a, 0x0c, 0x49, 0x4f, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x12,
	0x1c, 0x0a, 0x09, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x65, 0x64, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x08, 0x52, 0x09, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x65, 0x64, 0x22, 0x59, 0x0a,
	0x10, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x12, 0x2f, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x17, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e,
	0x61, 0x70, 0x69, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x12, 0x14, 0x0a, 0x05, 0x69, 0x6e, 0x70, 0x75, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x05, 0x69, 0x6e, 0x70, 0x75, 0x74, 0x42, 0x2a, 0x5a, 0x28, 0x67, 0x61, 0x74, 0x65,
	0x2e, 0x63, 0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65, 0x72, 0x2f, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x69,
	0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x77, 0x65, 0x62, 0x73, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x61, 0x70, 0x69, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_internal_webserverapi_webserverapi_proto_rawDescOnce sync.Once
	file_internal_webserverapi_webserverapi_proto_rawDescData = file_internal_webserverapi_webserverapi_proto_rawDesc
)

func file_internal_webserverapi_webserverapi_proto_rawDescGZIP() []byte {
	file_internal_webserverapi_webserverapi_proto_rawDescOnce.Do(func() {
		file_internal_webserverapi_webserverapi_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_webserverapi_webserverapi_proto_rawDescData)
	})
	return file_internal_webserverapi_webserverapi_proto_rawDescData
}

var file_internal_webserverapi_webserverapi_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_internal_webserverapi_webserverapi_proto_goTypes = []interface{}{
	(*IOConnection)(nil),     // 0: gate.internal.webserverapi.IOConnection
	(*ConnectionStatus)(nil), // 1: gate.internal.webserverapi.ConnectionStatus
	(*api.Status)(nil),       // 2: gate.server.api.Status
}
var file_internal_webserverapi_webserverapi_proto_depIdxs = []int32{
	2, // 0: gate.internal.webserverapi.ConnectionStatus.status:type_name -> gate.server.api.Status
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_internal_webserverapi_webserverapi_proto_init() }
func file_internal_webserverapi_webserverapi_proto_init() {
	if File_internal_webserverapi_webserverapi_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_internal_webserverapi_webserverapi_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*IOConnection); i {
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
		file_internal_webserverapi_webserverapi_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ConnectionStatus); i {
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
			RawDescriptor: file_internal_webserverapi_webserverapi_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_internal_webserverapi_webserverapi_proto_goTypes,
		DependencyIndexes: file_internal_webserverapi_webserverapi_proto_depIdxs,
		MessageInfos:      file_internal_webserverapi_webserverapi_proto_msgTypes,
	}.Build()
	File_internal_webserverapi_webserverapi_proto = out.File
	file_internal_webserverapi_webserverapi_proto_rawDesc = nil
	file_internal_webserverapi_webserverapi_proto_goTypes = nil
	file_internal_webserverapi_webserverapi_proto_depIdxs = nil
}
