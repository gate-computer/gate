// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.4
// 	protoc        (unknown)
// source: internal/pb/server/inventory.proto

package server

import (
	server "gate.computer/gate/pb/server"
	snapshot "gate.computer/gate/pb/snapshot"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
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

type Module struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Tags          []string               `protobuf:"bytes,1,rep,name=tags,proto3" json:"tags,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Module) Reset() {
	*x = Module{}
	mi := &file_internal_pb_server_inventory_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Module) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Module) ProtoMessage() {}

func (x *Module) ProtoReflect() protoreflect.Message {
	mi := &file_internal_pb_server_inventory_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Module.ProtoReflect.Descriptor instead.
func (*Module) Descriptor() ([]byte, []int) {
	return file_internal_pb_server_inventory_proto_rawDescGZIP(), []int{0}
}

func (x *Module) GetTags() []string {
	if x != nil {
		return x.Tags
	}
	return nil
}

type Instance struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	Exists         bool                   `protobuf:"varint,1,opt,name=exists,proto3" json:"exists,omitempty"`
	Transient      bool                   `protobuf:"varint,2,opt,name=transient,proto3" json:"transient,omitempty"`
	Status         *server.Status         `protobuf:"bytes,3,opt,name=status,proto3" json:"status,omitempty"`
	Buffers        *snapshot.Buffers      `protobuf:"bytes,4,opt,name=buffers,proto3" json:"buffers,omitempty"`
	TimeResolution *durationpb.Duration   `protobuf:"bytes,5,opt,name=time_resolution,json=timeResolution,proto3" json:"time_resolution,omitempty"`
	Tags           []string               `protobuf:"bytes,6,rep,name=tags,proto3" json:"tags,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *Instance) Reset() {
	*x = Instance{}
	mi := &file_internal_pb_server_inventory_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Instance) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Instance) ProtoMessage() {}

func (x *Instance) ProtoReflect() protoreflect.Message {
	mi := &file_internal_pb_server_inventory_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Instance.ProtoReflect.Descriptor instead.
func (*Instance) Descriptor() ([]byte, []int) {
	return file_internal_pb_server_inventory_proto_rawDescGZIP(), []int{1}
}

func (x *Instance) GetExists() bool {
	if x != nil {
		return x.Exists
	}
	return false
}

func (x *Instance) GetTransient() bool {
	if x != nil {
		return x.Transient
	}
	return false
}

func (x *Instance) GetStatus() *server.Status {
	if x != nil {
		return x.Status
	}
	return nil
}

func (x *Instance) GetBuffers() *snapshot.Buffers {
	if x != nil {
		return x.Buffers
	}
	return nil
}

func (x *Instance) GetTimeResolution() *durationpb.Duration {
	if x != nil {
		return x.TimeResolution
	}
	return nil
}

func (x *Instance) GetTags() []string {
	if x != nil {
		return x.Tags
	}
	return nil
}

var File_internal_pb_server_inventory_proto protoreflect.FileDescriptor

var file_internal_pb_server_inventory_proto_rawDesc = string([]byte{
	0x0a, 0x22, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x62, 0x2f, 0x73, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x2f, 0x69, 0x6e, 0x76, 0x65, 0x6e, 0x74, 0x6f, 0x72, 0x79, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x14, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72,
	0x6e, 0x61, 0x6c, 0x2e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x1a, 0x18, 0x67, 0x61, 0x74, 0x65,
	0x2f, 0x70, 0x62, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x61, 0x70, 0x69, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1e, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x70, 0x62, 0x2f, 0x73, 0x6e,
	0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x2f, 0x62, 0x75, 0x66, 0x66, 0x65, 0x72, 0x73, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x22, 0x1c, 0x0a, 0x06, 0x4d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x12, 0x12,
	0x0a, 0x04, 0x74, 0x61, 0x67, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x04, 0x74, 0x61,
	0x67, 0x73, 0x22, 0x81, 0x02, 0x0a, 0x08, 0x49, 0x6e, 0x73, 0x74, 0x61, 0x6e, 0x63, 0x65, 0x12,
	0x16, 0x0a, 0x06, 0x65, 0x78, 0x69, 0x73, 0x74, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52,
	0x06, 0x65, 0x78, 0x69, 0x73, 0x74, 0x73, 0x12, 0x1c, 0x0a, 0x09, 0x74, 0x72, 0x61, 0x6e, 0x73,
	0x69, 0x65, 0x6e, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x08, 0x52, 0x09, 0x74, 0x72, 0x61, 0x6e,
	0x73, 0x69, 0x65, 0x6e, 0x74, 0x12, 0x30, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x18, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x67, 0x61, 0x74,
	0x65, 0x2e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52,
	0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x35, 0x0a, 0x07, 0x62, 0x75, 0x66, 0x66, 0x65,
	0x72, 0x73, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e,
	0x67, 0x61, 0x74, 0x65, 0x2e, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x2e, 0x42, 0x75,
	0x66, 0x66, 0x65, 0x72, 0x73, 0x52, 0x07, 0x62, 0x75, 0x66, 0x66, 0x65, 0x72, 0x73, 0x12, 0x42,
	0x0a, 0x0f, 0x74, 0x69, 0x6d, 0x65, 0x5f, 0x72, 0x65, 0x73, 0x6f, 0x6c, 0x75, 0x74, 0x69, 0x6f,
	0x6e, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x52, 0x0e, 0x74, 0x69, 0x6d, 0x65, 0x52, 0x65, 0x73, 0x6f, 0x6c, 0x75, 0x74, 0x69,
	0x6f, 0x6e, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x61, 0x67, 0x73, 0x18, 0x06, 0x20, 0x03, 0x28, 0x09,
	0x52, 0x04, 0x74, 0x61, 0x67, 0x73, 0x42, 0x22, 0x5a, 0x20, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x63,
	0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65, 0x72, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c,
	0x2f, 0x70, 0x62, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
})

var (
	file_internal_pb_server_inventory_proto_rawDescOnce sync.Once
	file_internal_pb_server_inventory_proto_rawDescData []byte
)

func file_internal_pb_server_inventory_proto_rawDescGZIP() []byte {
	file_internal_pb_server_inventory_proto_rawDescOnce.Do(func() {
		file_internal_pb_server_inventory_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_internal_pb_server_inventory_proto_rawDesc), len(file_internal_pb_server_inventory_proto_rawDesc)))
	})
	return file_internal_pb_server_inventory_proto_rawDescData
}

var file_internal_pb_server_inventory_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_internal_pb_server_inventory_proto_goTypes = []any{
	(*Module)(nil),              // 0: gate.internal.server.Module
	(*Instance)(nil),            // 1: gate.internal.server.Instance
	(*server.Status)(nil),       // 2: gate.gate.server.Status
	(*snapshot.Buffers)(nil),    // 3: gate.gate.snapshot.Buffers
	(*durationpb.Duration)(nil), // 4: google.protobuf.Duration
}
var file_internal_pb_server_inventory_proto_depIdxs = []int32{
	2, // 0: gate.internal.server.Instance.status:type_name -> gate.gate.server.Status
	3, // 1: gate.internal.server.Instance.buffers:type_name -> gate.gate.snapshot.Buffers
	4, // 2: gate.internal.server.Instance.time_resolution:type_name -> google.protobuf.Duration
	3, // [3:3] is the sub-list for method output_type
	3, // [3:3] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_internal_pb_server_inventory_proto_init() }
func file_internal_pb_server_inventory_proto_init() {
	if File_internal_pb_server_inventory_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_internal_pb_server_inventory_proto_rawDesc), len(file_internal_pb_server_inventory_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_internal_pb_server_inventory_proto_goTypes,
		DependencyIndexes: file_internal_pb_server_inventory_proto_depIdxs,
		MessageInfos:      file_internal_pb_server_inventory_proto_msgTypes,
	}.Build()
	File_internal_pb_server_inventory_proto = out.File
	file_internal_pb_server_inventory_proto_goTypes = nil
	file_internal_pb_server_inventory_proto_depIdxs = nil
}
