// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.4
// 	protoc        (unknown)
// source: internal/pb/image/manifest.proto

package image

import (
	snapshot "gate.computer/gate/pb/snapshot"
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

type ProgramManifest struct {
	state                   protoimpl.MessageState `protogen:"open.v1"`
	LibraryChecksum         uint64                 `protobuf:"fixed64,1,opt,name=library_checksum,json=libraryChecksum,proto3" json:"library_checksum,omitempty"`
	TextRevision            int32                  `protobuf:"varint,2,opt,name=text_revision,json=textRevision,proto3" json:"text_revision,omitempty"`
	TextAddr                uint64                 `protobuf:"varint,3,opt,name=text_addr,json=textAddr,proto3" json:"text_addr,omitempty"`
	TextSize                uint32                 `protobuf:"varint,4,opt,name=text_size,json=textSize,proto3" json:"text_size,omitempty"`
	StackUsage              uint32                 `protobuf:"varint,5,opt,name=stack_usage,json=stackUsage,proto3" json:"stack_usage,omitempty"`
	GlobalsSize             uint32                 `protobuf:"varint,6,opt,name=globals_size,json=globalsSize,proto3" json:"globals_size,omitempty"`
	MemorySize              uint32                 `protobuf:"varint,7,opt,name=memory_size,json=memorySize,proto3" json:"memory_size,omitempty"`
	MemorySizeLimit         int64                  `protobuf:"zigzag64,8,opt,name=memory_size_limit,json=memorySizeLimit,proto3" json:"memory_size_limit,omitempty"`
	MemoryDataSize          uint32                 `protobuf:"varint,9,opt,name=memory_data_size,json=memoryDataSize,proto3" json:"memory_data_size,omitempty"`
	ModuleSize              int64                  `protobuf:"varint,10,opt,name=module_size,json=moduleSize,proto3" json:"module_size,omitempty"`
	Sections                []*ByteRange           `protobuf:"bytes,11,rep,name=sections,proto3" json:"sections,omitempty"`
	SnapshotSection         *ByteRange             `protobuf:"bytes,12,opt,name=snapshot_section,json=snapshotSection,proto3" json:"snapshot_section,omitempty"`
	ExportSectionWrap       *ByteRange             `protobuf:"bytes,13,opt,name=export_section_wrap,json=exportSectionWrap,proto3" json:"export_section_wrap,omitempty"`
	BufferSection           *ByteRange             `protobuf:"bytes,14,opt,name=buffer_section,json=bufferSection,proto3" json:"buffer_section,omitempty"`
	BufferSectionHeaderSize uint32                 `protobuf:"varint,15,opt,name=buffer_section_header_size,json=bufferSectionHeaderSize,proto3" json:"buffer_section_header_size,omitempty"`
	StackSection            *ByteRange             `protobuf:"bytes,16,opt,name=stack_section,json=stackSection,proto3" json:"stack_section,omitempty"`
	GlobalTypes             []byte                 `protobuf:"bytes,17,opt,name=global_types,json=globalTypes,proto3" json:"global_types,omitempty"` // Limited by wag's maxGlobals check.
	StartFunc               *Function              `protobuf:"bytes,18,opt,name=start_func,json=startFunc,proto3" json:"start_func,omitempty"`
	EntryIndexes            map[string]uint32      `protobuf:"bytes,19,rep,name=entry_indexes,json=entryIndexes,proto3" json:"entry_indexes,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"varint,2,opt,name=value"` // Limited by func name len and wag's maxExports check.
	EntryAddrs              map[uint32]uint32      `protobuf:"bytes,20,rep,name=entry_addrs,json=entryAddrs,proto3" json:"entry_addrs,omitempty" protobuf_key:"varint,1,opt,name=key" protobuf_val:"varint,2,opt,name=value"`
	CallSitesSize           uint32                 `protobuf:"varint,21,opt,name=call_sites_size,json=callSitesSize,proto3" json:"call_sites_size,omitempty"`
	FuncAddrsSize           uint32                 `protobuf:"varint,22,opt,name=func_addrs_size,json=funcAddrsSize,proto3" json:"func_addrs_size,omitempty"`
	Random                  bool                   `protobuf:"varint,23,opt,name=random,proto3" json:"random,omitempty"`
	Snapshot                *snapshot.Snapshot     `protobuf:"bytes,24,opt,name=snapshot,proto3" json:"snapshot,omitempty"`
	unknownFields           protoimpl.UnknownFields
	sizeCache               protoimpl.SizeCache
}

func (x *ProgramManifest) Reset() {
	*x = ProgramManifest{}
	mi := &file_internal_pb_image_manifest_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ProgramManifest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ProgramManifest) ProtoMessage() {}

func (x *ProgramManifest) ProtoReflect() protoreflect.Message {
	mi := &file_internal_pb_image_manifest_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ProgramManifest.ProtoReflect.Descriptor instead.
func (*ProgramManifest) Descriptor() ([]byte, []int) {
	return file_internal_pb_image_manifest_proto_rawDescGZIP(), []int{0}
}

func (x *ProgramManifest) GetLibraryChecksum() uint64 {
	if x != nil {
		return x.LibraryChecksum
	}
	return 0
}

func (x *ProgramManifest) GetTextRevision() int32 {
	if x != nil {
		return x.TextRevision
	}
	return 0
}

func (x *ProgramManifest) GetTextAddr() uint64 {
	if x != nil {
		return x.TextAddr
	}
	return 0
}

func (x *ProgramManifest) GetTextSize() uint32 {
	if x != nil {
		return x.TextSize
	}
	return 0
}

func (x *ProgramManifest) GetStackUsage() uint32 {
	if x != nil {
		return x.StackUsage
	}
	return 0
}

func (x *ProgramManifest) GetGlobalsSize() uint32 {
	if x != nil {
		return x.GlobalsSize
	}
	return 0
}

func (x *ProgramManifest) GetMemorySize() uint32 {
	if x != nil {
		return x.MemorySize
	}
	return 0
}

func (x *ProgramManifest) GetMemorySizeLimit() int64 {
	if x != nil {
		return x.MemorySizeLimit
	}
	return 0
}

func (x *ProgramManifest) GetMemoryDataSize() uint32 {
	if x != nil {
		return x.MemoryDataSize
	}
	return 0
}

func (x *ProgramManifest) GetModuleSize() int64 {
	if x != nil {
		return x.ModuleSize
	}
	return 0
}

func (x *ProgramManifest) GetSections() []*ByteRange {
	if x != nil {
		return x.Sections
	}
	return nil
}

func (x *ProgramManifest) GetSnapshotSection() *ByteRange {
	if x != nil {
		return x.SnapshotSection
	}
	return nil
}

func (x *ProgramManifest) GetExportSectionWrap() *ByteRange {
	if x != nil {
		return x.ExportSectionWrap
	}
	return nil
}

func (x *ProgramManifest) GetBufferSection() *ByteRange {
	if x != nil {
		return x.BufferSection
	}
	return nil
}

func (x *ProgramManifest) GetBufferSectionHeaderSize() uint32 {
	if x != nil {
		return x.BufferSectionHeaderSize
	}
	return 0
}

func (x *ProgramManifest) GetStackSection() *ByteRange {
	if x != nil {
		return x.StackSection
	}
	return nil
}

func (x *ProgramManifest) GetGlobalTypes() []byte {
	if x != nil {
		return x.GlobalTypes
	}
	return nil
}

func (x *ProgramManifest) GetStartFunc() *Function {
	if x != nil {
		return x.StartFunc
	}
	return nil
}

func (x *ProgramManifest) GetEntryIndexes() map[string]uint32 {
	if x != nil {
		return x.EntryIndexes
	}
	return nil
}

func (x *ProgramManifest) GetEntryAddrs() map[uint32]uint32 {
	if x != nil {
		return x.EntryAddrs
	}
	return nil
}

func (x *ProgramManifest) GetCallSitesSize() uint32 {
	if x != nil {
		return x.CallSitesSize
	}
	return 0
}

func (x *ProgramManifest) GetFuncAddrsSize() uint32 {
	if x != nil {
		return x.FuncAddrsSize
	}
	return 0
}

func (x *ProgramManifest) GetRandom() bool {
	if x != nil {
		return x.Random
	}
	return false
}

func (x *ProgramManifest) GetSnapshot() *snapshot.Snapshot {
	if x != nil {
		return x.Snapshot
	}
	return nil
}

type InstanceManifest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	TextAddr      uint64                 `protobuf:"varint,1,opt,name=text_addr,json=textAddr,proto3" json:"text_addr,omitempty"`
	StackSize     uint32                 `protobuf:"varint,2,opt,name=stack_size,json=stackSize,proto3" json:"stack_size,omitempty"`
	StackUsage    uint32                 `protobuf:"varint,3,opt,name=stack_usage,json=stackUsage,proto3" json:"stack_usage,omitempty"`
	GlobalsSize   uint32                 `protobuf:"varint,4,opt,name=globals_size,json=globalsSize,proto3" json:"globals_size,omitempty"`
	MemorySize    uint32                 `protobuf:"varint,5,opt,name=memory_size,json=memorySize,proto3" json:"memory_size,omitempty"`
	MaxMemorySize uint32                 `protobuf:"varint,6,opt,name=max_memory_size,json=maxMemorySize,proto3" json:"max_memory_size,omitempty"`
	StartFunc     *Function              `protobuf:"bytes,7,opt,name=start_func,json=startFunc,proto3" json:"start_func,omitempty"`
	EntryFunc     *Function              `protobuf:"bytes,8,opt,name=entry_func,json=entryFunc,proto3" json:"entry_func,omitempty"`
	Snapshot      *snapshot.Snapshot     `protobuf:"bytes,9,opt,name=snapshot,proto3" json:"snapshot,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *InstanceManifest) Reset() {
	*x = InstanceManifest{}
	mi := &file_internal_pb_image_manifest_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *InstanceManifest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InstanceManifest) ProtoMessage() {}

func (x *InstanceManifest) ProtoReflect() protoreflect.Message {
	mi := &file_internal_pb_image_manifest_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InstanceManifest.ProtoReflect.Descriptor instead.
func (*InstanceManifest) Descriptor() ([]byte, []int) {
	return file_internal_pb_image_manifest_proto_rawDescGZIP(), []int{1}
}

func (x *InstanceManifest) GetTextAddr() uint64 {
	if x != nil {
		return x.TextAddr
	}
	return 0
}

func (x *InstanceManifest) GetStackSize() uint32 {
	if x != nil {
		return x.StackSize
	}
	return 0
}

func (x *InstanceManifest) GetStackUsage() uint32 {
	if x != nil {
		return x.StackUsage
	}
	return 0
}

func (x *InstanceManifest) GetGlobalsSize() uint32 {
	if x != nil {
		return x.GlobalsSize
	}
	return 0
}

func (x *InstanceManifest) GetMemorySize() uint32 {
	if x != nil {
		return x.MemorySize
	}
	return 0
}

func (x *InstanceManifest) GetMaxMemorySize() uint32 {
	if x != nil {
		return x.MaxMemorySize
	}
	return 0
}

func (x *InstanceManifest) GetStartFunc() *Function {
	if x != nil {
		return x.StartFunc
	}
	return nil
}

func (x *InstanceManifest) GetEntryFunc() *Function {
	if x != nil {
		return x.EntryFunc
	}
	return nil
}

func (x *InstanceManifest) GetSnapshot() *snapshot.Snapshot {
	if x != nil {
		return x.Snapshot
	}
	return nil
}

type Function struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Index         uint32                 `protobuf:"varint,1,opt,name=index,proto3" json:"index,omitempty"`
	Addr          uint32                 `protobuf:"varint,2,opt,name=addr,proto3" json:"addr,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Function) Reset() {
	*x = Function{}
	mi := &file_internal_pb_image_manifest_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Function) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Function) ProtoMessage() {}

func (x *Function) ProtoReflect() protoreflect.Message {
	mi := &file_internal_pb_image_manifest_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Function.ProtoReflect.Descriptor instead.
func (*Function) Descriptor() ([]byte, []int) {
	return file_internal_pb_image_manifest_proto_rawDescGZIP(), []int{2}
}

func (x *Function) GetIndex() uint32 {
	if x != nil {
		return x.Index
	}
	return 0
}

func (x *Function) GetAddr() uint32 {
	if x != nil {
		return x.Addr
	}
	return 0
}

type ByteRange struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Start         int64                  `protobuf:"varint,1,opt,name=start,proto3" json:"start,omitempty"`
	Size          uint32                 `protobuf:"varint,2,opt,name=size,proto3" json:"size,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ByteRange) Reset() {
	*x = ByteRange{}
	mi := &file_internal_pb_image_manifest_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ByteRange) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ByteRange) ProtoMessage() {}

func (x *ByteRange) ProtoReflect() protoreflect.Message {
	mi := &file_internal_pb_image_manifest_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ByteRange.ProtoReflect.Descriptor instead.
func (*ByteRange) Descriptor() ([]byte, []int) {
	return file_internal_pb_image_manifest_proto_rawDescGZIP(), []int{3}
}

func (x *ByteRange) GetStart() int64 {
	if x != nil {
		return x.Start
	}
	return 0
}

func (x *ByteRange) GetSize() uint32 {
	if x != nil {
		return x.Size
	}
	return 0
}

var File_internal_pb_image_manifest_proto protoreflect.FileDescriptor

var file_internal_pb_image_manifest_proto_rawDesc = string([]byte{
	0x0a, 0x20, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x62, 0x2f, 0x69, 0x6d,
	0x61, 0x67, 0x65, 0x2f, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x12, 0x13, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61,
	0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x1a, 0x1f, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x70, 0x62,
	0x2f, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x2f, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68,
	0x6f, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xce, 0x0a, 0x0a, 0x0f, 0x50, 0x72, 0x6f,
	0x67, 0x72, 0x61, 0x6d, 0x4d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x12, 0x29, 0x0a, 0x10,
	0x6c, 0x69, 0x62, 0x72, 0x61, 0x72, 0x79, 0x5f, 0x63, 0x68, 0x65, 0x63, 0x6b, 0x73, 0x75, 0x6d,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x06, 0x52, 0x0f, 0x6c, 0x69, 0x62, 0x72, 0x61, 0x72, 0x79, 0x43,
	0x68, 0x65, 0x63, 0x6b, 0x73, 0x75, 0x6d, 0x12, 0x23, 0x0a, 0x0d, 0x74, 0x65, 0x78, 0x74, 0x5f,
	0x72, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0c,
	0x74, 0x65, 0x78, 0x74, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x1b, 0x0a, 0x09,
	0x74, 0x65, 0x78, 0x74, 0x5f, 0x61, 0x64, 0x64, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x04, 0x52,
	0x08, 0x74, 0x65, 0x78, 0x74, 0x41, 0x64, 0x64, 0x72, 0x12, 0x1b, 0x0a, 0x09, 0x74, 0x65, 0x78,
	0x74, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x08, 0x74, 0x65,
	0x78, 0x74, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x5f,
	0x75, 0x73, 0x61, 0x67, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0a, 0x73, 0x74, 0x61,
	0x63, 0x6b, 0x55, 0x73, 0x61, 0x67, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x67, 0x6c, 0x6f, 0x62, 0x61,
	0x6c, 0x73, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0b, 0x67,
	0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x73, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x6d, 0x65,
	0x6d, 0x6f, 0x72, 0x79, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0d, 0x52,
	0x0a, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x2a, 0x0a, 0x11, 0x6d,
	0x65, 0x6d, 0x6f, 0x72, 0x79, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x5f, 0x6c, 0x69, 0x6d, 0x69, 0x74,
	0x18, 0x08, 0x20, 0x01, 0x28, 0x12, 0x52, 0x0f, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x69,
	0x7a, 0x65, 0x4c, 0x69, 0x6d, 0x69, 0x74, 0x12, 0x28, 0x0a, 0x10, 0x6d, 0x65, 0x6d, 0x6f, 0x72,
	0x79, 0x5f, 0x64, 0x61, 0x74, 0x61, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x09, 0x20, 0x01, 0x28,
	0x0d, 0x52, 0x0e, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x44, 0x61, 0x74, 0x61, 0x53, 0x69, 0x7a,
	0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x5f, 0x73, 0x69, 0x7a, 0x65,
	0x18, 0x0a, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0a, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x53, 0x69,
	0x7a, 0x65, 0x12, 0x3a, 0x0a, 0x08, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x0b,
	0x20, 0x03, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6e, 0x74, 0x65,
	0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x42, 0x79, 0x74, 0x65, 0x52,
	0x61, 0x6e, 0x67, 0x65, 0x52, 0x08, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x49,
	0x0a, 0x10, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69,
	0x6f, 0x6e, 0x18, 0x0c, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e,
	0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x42,
	0x79, 0x74, 0x65, 0x52, 0x61, 0x6e, 0x67, 0x65, 0x52, 0x0f, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68,
	0x6f, 0x74, 0x53, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x4e, 0x0a, 0x13, 0x65, 0x78, 0x70,
	0x6f, 0x72, 0x74, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x77, 0x72, 0x61, 0x70,
	0x18, 0x0d, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6e,
	0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x42, 0x79, 0x74,
	0x65, 0x52, 0x61, 0x6e, 0x67, 0x65, 0x52, 0x11, 0x65, 0x78, 0x70, 0x6f, 0x72, 0x74, 0x53, 0x65,
	0x63, 0x74, 0x69, 0x6f, 0x6e, 0x57, 0x72, 0x61, 0x70, 0x12, 0x45, 0x0a, 0x0e, 0x62, 0x75, 0x66,
	0x66, 0x65, 0x72, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x0e, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61,
	0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x42, 0x79, 0x74, 0x65, 0x52, 0x61, 0x6e, 0x67,
	0x65, 0x52, 0x0d, 0x62, 0x75, 0x66, 0x66, 0x65, 0x72, 0x53, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e,
	0x12, 0x3b, 0x0a, 0x1a, 0x62, 0x75, 0x66, 0x66, 0x65, 0x72, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69,
	0x6f, 0x6e, 0x5f, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x0f,
	0x20, 0x01, 0x28, 0x0d, 0x52, 0x17, 0x62, 0x75, 0x66, 0x66, 0x65, 0x72, 0x53, 0x65, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x48, 0x65, 0x61, 0x64, 0x65, 0x72, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x43, 0x0a,
	0x0d, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x10,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6e, 0x74, 0x65,
	0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x42, 0x79, 0x74, 0x65, 0x52,
	0x61, 0x6e, 0x67, 0x65, 0x52, 0x0c, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x53, 0x65, 0x63, 0x74, 0x69,
	0x6f, 0x6e, 0x12, 0x21, 0x0a, 0x0c, 0x67, 0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x5f, 0x74, 0x79, 0x70,
	0x65, 0x73, 0x18, 0x11, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0b, 0x67, 0x6c, 0x6f, 0x62, 0x61, 0x6c,
	0x54, 0x79, 0x70, 0x65, 0x73, 0x12, 0x3c, 0x0a, 0x0a, 0x73, 0x74, 0x61, 0x72, 0x74, 0x5f, 0x66,
	0x75, 0x6e, 0x63, 0x18, 0x12, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x67, 0x61, 0x74, 0x65,
	0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e,
	0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x09, 0x73, 0x74, 0x61, 0x72, 0x74, 0x46,
	0x75, 0x6e, 0x63, 0x12, 0x5b, 0x0a, 0x0d, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x5f, 0x69, 0x6e, 0x64,
	0x65, 0x78, 0x65, 0x73, 0x18, 0x13, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x36, 0x2e, 0x67, 0x61, 0x74,
	0x65, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65,
	0x2e, 0x50, 0x72, 0x6f, 0x67, 0x72, 0x61, 0x6d, 0x4d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74,
	0x2e, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x65, 0x73, 0x45, 0x6e, 0x74,
	0x72, 0x79, 0x52, 0x0c, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x65, 0x73,
	0x12, 0x55, 0x0a, 0x0b, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x5f, 0x61, 0x64, 0x64, 0x72, 0x73, 0x18,
	0x14, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x34, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6e, 0x74,
	0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x50, 0x72, 0x6f, 0x67,
	0x72, 0x61, 0x6d, 0x4d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x45, 0x6e, 0x74, 0x72,
	0x79, 0x41, 0x64, 0x64, 0x72, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x0a, 0x65, 0x6e, 0x74,
	0x72, 0x79, 0x41, 0x64, 0x64, 0x72, 0x73, 0x12, 0x26, 0x0a, 0x0f, 0x63, 0x61, 0x6c, 0x6c, 0x5f,
	0x73, 0x69, 0x74, 0x65, 0x73, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x15, 0x20, 0x01, 0x28, 0x0d,
	0x52, 0x0d, 0x63, 0x61, 0x6c, 0x6c, 0x53, 0x69, 0x74, 0x65, 0x73, 0x53, 0x69, 0x7a, 0x65, 0x12,
	0x26, 0x0a, 0x0f, 0x66, 0x75, 0x6e, 0x63, 0x5f, 0x61, 0x64, 0x64, 0x72, 0x73, 0x5f, 0x73, 0x69,
	0x7a, 0x65, 0x18, 0x16, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0d, 0x66, 0x75, 0x6e, 0x63, 0x41, 0x64,
	0x64, 0x72, 0x73, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x61, 0x6e, 0x64, 0x6f,
	0x6d, 0x18, 0x17, 0x20, 0x01, 0x28, 0x08, 0x52, 0x06, 0x72, 0x61, 0x6e, 0x64, 0x6f, 0x6d, 0x12,
	0x38, 0x0a, 0x08, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x18, 0x18, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x1c, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x73, 0x6e,
	0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x2e, 0x53, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x52,
	0x08, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x1a, 0x3f, 0x0a, 0x11, 0x45, 0x6e, 0x74,
	0x72, 0x79, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x65, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10,
	0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79,
	0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52,
	0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x1a, 0x3d, 0x0a, 0x0f, 0x45, 0x6e,
	0x74, 0x72, 0x79, 0x41, 0x64, 0x64, 0x72, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a,
	0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12,
	0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x05,
	0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22, 0x91, 0x03, 0x0a, 0x10, 0x49, 0x6e,
	0x73, 0x74, 0x61, 0x6e, 0x63, 0x65, 0x4d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x12, 0x1b,
	0x0a, 0x09, 0x74, 0x65, 0x78, 0x74, 0x5f, 0x61, 0x64, 0x64, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x04, 0x52, 0x08, 0x74, 0x65, 0x78, 0x74, 0x41, 0x64, 0x64, 0x72, 0x12, 0x1d, 0x0a, 0x0a, 0x73,
	0x74, 0x61, 0x63, 0x6b, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52,
	0x09, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x73, 0x74,
	0x61, 0x63, 0x6b, 0x5f, 0x75, 0x73, 0x61, 0x67, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52,
	0x0a, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x55, 0x73, 0x61, 0x67, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x67,
	0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x73, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x0d, 0x52, 0x0b, 0x67, 0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x73, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x1f,
	0x0a, 0x0b, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x0d, 0x52, 0x0a, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x69, 0x7a, 0x65, 0x12,
	0x26, 0x0a, 0x0f, 0x6d, 0x61, 0x78, 0x5f, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x5f, 0x73, 0x69,
	0x7a, 0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0d, 0x6d, 0x61, 0x78, 0x4d, 0x65, 0x6d,
	0x6f, 0x72, 0x79, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x3c, 0x0a, 0x0a, 0x73, 0x74, 0x61, 0x72, 0x74,
	0x5f, 0x66, 0x75, 0x6e, 0x63, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x67, 0x61,
	0x74, 0x65, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67,
	0x65, 0x2e, 0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x09, 0x73, 0x74, 0x61, 0x72,
	0x74, 0x46, 0x75, 0x6e, 0x63, 0x12, 0x3c, 0x0a, 0x0a, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x5f, 0x66,
	0x75, 0x6e, 0x63, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x67, 0x61, 0x74, 0x65,
	0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e,
	0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x09, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x46,
	0x75, 0x6e, 0x63, 0x12, 0x38, 0x0a, 0x08, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x18,
	0x09, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1c, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x67, 0x61, 0x74,
	0x65, 0x2e, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x2e, 0x53, 0x6e, 0x61, 0x70, 0x73,
	0x68, 0x6f, 0x74, 0x52, 0x08, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x22, 0x34, 0x0a,
	0x08, 0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x14, 0x0a, 0x05, 0x69, 0x6e, 0x64,
	0x65, 0x78, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x05, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x12,
	0x12, 0x0a, 0x04, 0x61, 0x64, 0x64, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x61,
	0x64, 0x64, 0x72, 0x22, 0x35, 0x0a, 0x09, 0x42, 0x79, 0x74, 0x65, 0x52, 0x61, 0x6e, 0x67, 0x65,
	0x12, 0x14, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x72, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52,
	0x05, 0x73, 0x74, 0x61, 0x72, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x73, 0x69, 0x7a, 0x65, 0x42, 0x21, 0x5a, 0x1f, 0x67, 0x61,
	0x74, 0x65, 0x2e, 0x63, 0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65, 0x72, 0x2f, 0x69, 0x6e, 0x74, 0x65,
	0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x62, 0x2f, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x62, 0x06, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_internal_pb_image_manifest_proto_rawDescOnce sync.Once
	file_internal_pb_image_manifest_proto_rawDescData []byte
)

func file_internal_pb_image_manifest_proto_rawDescGZIP() []byte {
	file_internal_pb_image_manifest_proto_rawDescOnce.Do(func() {
		file_internal_pb_image_manifest_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_internal_pb_image_manifest_proto_rawDesc), len(file_internal_pb_image_manifest_proto_rawDesc)))
	})
	return file_internal_pb_image_manifest_proto_rawDescData
}

var file_internal_pb_image_manifest_proto_msgTypes = make([]protoimpl.MessageInfo, 6)
var file_internal_pb_image_manifest_proto_goTypes = []any{
	(*ProgramManifest)(nil),   // 0: gate.internal.image.ProgramManifest
	(*InstanceManifest)(nil),  // 1: gate.internal.image.InstanceManifest
	(*Function)(nil),          // 2: gate.internal.image.Function
	(*ByteRange)(nil),         // 3: gate.internal.image.ByteRange
	nil,                       // 4: gate.internal.image.ProgramManifest.EntryIndexesEntry
	nil,                       // 5: gate.internal.image.ProgramManifest.EntryAddrsEntry
	(*snapshot.Snapshot)(nil), // 6: gate.gate.snapshot.Snapshot
}
var file_internal_pb_image_manifest_proto_depIdxs = []int32{
	3,  // 0: gate.internal.image.ProgramManifest.sections:type_name -> gate.internal.image.ByteRange
	3,  // 1: gate.internal.image.ProgramManifest.snapshot_section:type_name -> gate.internal.image.ByteRange
	3,  // 2: gate.internal.image.ProgramManifest.export_section_wrap:type_name -> gate.internal.image.ByteRange
	3,  // 3: gate.internal.image.ProgramManifest.buffer_section:type_name -> gate.internal.image.ByteRange
	3,  // 4: gate.internal.image.ProgramManifest.stack_section:type_name -> gate.internal.image.ByteRange
	2,  // 5: gate.internal.image.ProgramManifest.start_func:type_name -> gate.internal.image.Function
	4,  // 6: gate.internal.image.ProgramManifest.entry_indexes:type_name -> gate.internal.image.ProgramManifest.EntryIndexesEntry
	5,  // 7: gate.internal.image.ProgramManifest.entry_addrs:type_name -> gate.internal.image.ProgramManifest.EntryAddrsEntry
	6,  // 8: gate.internal.image.ProgramManifest.snapshot:type_name -> gate.gate.snapshot.Snapshot
	2,  // 9: gate.internal.image.InstanceManifest.start_func:type_name -> gate.internal.image.Function
	2,  // 10: gate.internal.image.InstanceManifest.entry_func:type_name -> gate.internal.image.Function
	6,  // 11: gate.internal.image.InstanceManifest.snapshot:type_name -> gate.gate.snapshot.Snapshot
	12, // [12:12] is the sub-list for method output_type
	12, // [12:12] is the sub-list for method input_type
	12, // [12:12] is the sub-list for extension type_name
	12, // [12:12] is the sub-list for extension extendee
	0,  // [0:12] is the sub-list for field type_name
}

func init() { file_internal_pb_image_manifest_proto_init() }
func file_internal_pb_image_manifest_proto_init() {
	if File_internal_pb_image_manifest_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_internal_pb_image_manifest_proto_rawDesc), len(file_internal_pb_image_manifest_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   6,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_internal_pb_image_manifest_proto_goTypes,
		DependencyIndexes: file_internal_pb_image_manifest_proto_depIdxs,
		MessageInfos:      file_internal_pb_image_manifest_proto_msgTypes,
	}.Build()
	File_internal_pb_image_manifest_proto = out.File
	file_internal_pb_image_manifest_proto_goTypes = nil
	file_internal_pb_image_manifest_proto_depIdxs = nil
}
