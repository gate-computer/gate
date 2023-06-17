// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.29.1
// 	protoc        (unknown)
// source: internal/manifest/manifest.proto

package manifest

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

type ByteRange struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Start int64  `protobuf:"varint,1,opt,name=start,proto3" json:"start,omitempty"`
	Size  uint32 `protobuf:"varint,2,opt,name=size,proto3" json:"size,omitempty"`
}

func (x *ByteRange) Reset() {
	*x = ByteRange{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_manifest_manifest_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ByteRange) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ByteRange) ProtoMessage() {}

func (x *ByteRange) ProtoReflect() protoreflect.Message {
	mi := &file_internal_manifest_manifest_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
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
	return file_internal_manifest_manifest_proto_rawDescGZIP(), []int{0}
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

type Program struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	LibraryChecksum         uint64            `protobuf:"fixed64,1,opt,name=library_checksum,json=libraryChecksum,proto3" json:"library_checksum,omitempty"`
	TextRevision            int32             `protobuf:"varint,2,opt,name=text_revision,json=textRevision,proto3" json:"text_revision,omitempty"`
	TextAddr                uint64            `protobuf:"varint,3,opt,name=text_addr,json=textAddr,proto3" json:"text_addr,omitempty"`
	TextSize                uint32            `protobuf:"varint,4,opt,name=text_size,json=textSize,proto3" json:"text_size,omitempty"`
	StackUsage              uint32            `protobuf:"varint,5,opt,name=stack_usage,json=stackUsage,proto3" json:"stack_usage,omitempty"`
	GlobalsSize             uint32            `protobuf:"varint,6,opt,name=globals_size,json=globalsSize,proto3" json:"globals_size,omitempty"`
	MemorySize              uint32            `protobuf:"varint,7,opt,name=memory_size,json=memorySize,proto3" json:"memory_size,omitempty"`
	MemorySizeLimit         int64             `protobuf:"zigzag64,8,opt,name=memory_size_limit,json=memorySizeLimit,proto3" json:"memory_size_limit,omitempty"`
	MemoryDataSize          uint32            `protobuf:"varint,9,opt,name=memory_data_size,json=memoryDataSize,proto3" json:"memory_data_size,omitempty"`
	ModuleSize              int64             `protobuf:"varint,10,opt,name=module_size,json=moduleSize,proto3" json:"module_size,omitempty"`
	Sections                []*ByteRange      `protobuf:"bytes,11,rep,name=sections,proto3" json:"sections,omitempty"`
	SnapshotSection         *ByteRange        `protobuf:"bytes,12,opt,name=snapshot_section,json=snapshotSection,proto3" json:"snapshot_section,omitempty"`
	ExportSectionWrap       *ByteRange        `protobuf:"bytes,13,opt,name=export_section_wrap,json=exportSectionWrap,proto3" json:"export_section_wrap,omitempty"`
	BufferSection           *ByteRange        `protobuf:"bytes,14,opt,name=buffer_section,json=bufferSection,proto3" json:"buffer_section,omitempty"`
	BufferSectionHeaderSize uint32            `protobuf:"varint,15,opt,name=buffer_section_header_size,json=bufferSectionHeaderSize,proto3" json:"buffer_section_header_size,omitempty"`
	StackSection            *ByteRange        `protobuf:"bytes,16,opt,name=stack_section,json=stackSection,proto3" json:"stack_section,omitempty"`
	GlobalTypes             []byte            `protobuf:"bytes,17,opt,name=global_types,json=globalTypes,proto3" json:"global_types,omitempty"` // Limited by wag's maxGlobals check.
	StartFunc               *Function         `protobuf:"bytes,18,opt,name=start_func,json=startFunc,proto3" json:"start_func,omitempty"`
	EntryIndexes            map[string]uint32 `protobuf:"bytes,19,rep,name=entry_indexes,json=entryIndexes,proto3" json:"entry_indexes,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"varint,2,opt,name=value,proto3"` // Limited by func name len and wag's maxExports check.
	EntryAddrs              map[uint32]uint32 `protobuf:"bytes,20,rep,name=entry_addrs,json=entryAddrs,proto3" json:"entry_addrs,omitempty" protobuf_key:"varint,1,opt,name=key,proto3" protobuf_val:"varint,2,opt,name=value,proto3"`
	CallSitesSize           uint32            `protobuf:"varint,21,opt,name=call_sites_size,json=callSitesSize,proto3" json:"call_sites_size,omitempty"`
	FuncAddrsSize           uint32            `protobuf:"varint,22,opt,name=func_addrs_size,json=funcAddrsSize,proto3" json:"func_addrs_size,omitempty"`
	Random                  bool              `protobuf:"varint,23,opt,name=random,proto3" json:"random,omitempty"`
	Snapshot                *Snapshot         `protobuf:"bytes,24,opt,name=snapshot,proto3" json:"snapshot,omitempty"`
}

func (x *Program) Reset() {
	*x = Program{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_manifest_manifest_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Program) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Program) ProtoMessage() {}

func (x *Program) ProtoReflect() protoreflect.Message {
	mi := &file_internal_manifest_manifest_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Program.ProtoReflect.Descriptor instead.
func (*Program) Descriptor() ([]byte, []int) {
	return file_internal_manifest_manifest_proto_rawDescGZIP(), []int{1}
}

func (x *Program) GetLibraryChecksum() uint64 {
	if x != nil {
		return x.LibraryChecksum
	}
	return 0
}

func (x *Program) GetTextRevision() int32 {
	if x != nil {
		return x.TextRevision
	}
	return 0
}

func (x *Program) GetTextAddr() uint64 {
	if x != nil {
		return x.TextAddr
	}
	return 0
}

func (x *Program) GetTextSize() uint32 {
	if x != nil {
		return x.TextSize
	}
	return 0
}

func (x *Program) GetStackUsage() uint32 {
	if x != nil {
		return x.StackUsage
	}
	return 0
}

func (x *Program) GetGlobalsSize() uint32 {
	if x != nil {
		return x.GlobalsSize
	}
	return 0
}

func (x *Program) GetMemorySize() uint32 {
	if x != nil {
		return x.MemorySize
	}
	return 0
}

func (x *Program) GetMemorySizeLimit() int64 {
	if x != nil {
		return x.MemorySizeLimit
	}
	return 0
}

func (x *Program) GetMemoryDataSize() uint32 {
	if x != nil {
		return x.MemoryDataSize
	}
	return 0
}

func (x *Program) GetModuleSize() int64 {
	if x != nil {
		return x.ModuleSize
	}
	return 0
}

func (x *Program) GetSections() []*ByteRange {
	if x != nil {
		return x.Sections
	}
	return nil
}

func (x *Program) GetSnapshotSection() *ByteRange {
	if x != nil {
		return x.SnapshotSection
	}
	return nil
}

func (x *Program) GetExportSectionWrap() *ByteRange {
	if x != nil {
		return x.ExportSectionWrap
	}
	return nil
}

func (x *Program) GetBufferSection() *ByteRange {
	if x != nil {
		return x.BufferSection
	}
	return nil
}

func (x *Program) GetBufferSectionHeaderSize() uint32 {
	if x != nil {
		return x.BufferSectionHeaderSize
	}
	return 0
}

func (x *Program) GetStackSection() *ByteRange {
	if x != nil {
		return x.StackSection
	}
	return nil
}

func (x *Program) GetGlobalTypes() []byte {
	if x != nil {
		return x.GlobalTypes
	}
	return nil
}

func (x *Program) GetStartFunc() *Function {
	if x != nil {
		return x.StartFunc
	}
	return nil
}

func (x *Program) GetEntryIndexes() map[string]uint32 {
	if x != nil {
		return x.EntryIndexes
	}
	return nil
}

func (x *Program) GetEntryAddrs() map[uint32]uint32 {
	if x != nil {
		return x.EntryAddrs
	}
	return nil
}

func (x *Program) GetCallSitesSize() uint32 {
	if x != nil {
		return x.CallSitesSize
	}
	return 0
}

func (x *Program) GetFuncAddrsSize() uint32 {
	if x != nil {
		return x.FuncAddrsSize
	}
	return 0
}

func (x *Program) GetRandom() bool {
	if x != nil {
		return x.Random
	}
	return false
}

func (x *Program) GetSnapshot() *Snapshot {
	if x != nil {
		return x.Snapshot
	}
	return nil
}

type Instance struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	TextAddr      uint64    `protobuf:"varint,1,opt,name=text_addr,json=textAddr,proto3" json:"text_addr,omitempty"`
	StackSize     uint32    `protobuf:"varint,2,opt,name=stack_size,json=stackSize,proto3" json:"stack_size,omitempty"`
	StackUsage    uint32    `protobuf:"varint,3,opt,name=stack_usage,json=stackUsage,proto3" json:"stack_usage,omitempty"`
	GlobalsSize   uint32    `protobuf:"varint,4,opt,name=globals_size,json=globalsSize,proto3" json:"globals_size,omitempty"`
	MemorySize    uint32    `protobuf:"varint,5,opt,name=memory_size,json=memorySize,proto3" json:"memory_size,omitempty"`
	MaxMemorySize uint32    `protobuf:"varint,6,opt,name=max_memory_size,json=maxMemorySize,proto3" json:"max_memory_size,omitempty"`
	StartFunc     *Function `protobuf:"bytes,7,opt,name=start_func,json=startFunc,proto3" json:"start_func,omitempty"`
	EntryFunc     *Function `protobuf:"bytes,8,opt,name=entry_func,json=entryFunc,proto3" json:"entry_func,omitempty"`
	Snapshot      *Snapshot `protobuf:"bytes,9,opt,name=snapshot,proto3" json:"snapshot,omitempty"`
}

func (x *Instance) Reset() {
	*x = Instance{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_manifest_manifest_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Instance) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Instance) ProtoMessage() {}

func (x *Instance) ProtoReflect() protoreflect.Message {
	mi := &file_internal_manifest_manifest_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
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
	return file_internal_manifest_manifest_proto_rawDescGZIP(), []int{2}
}

func (x *Instance) GetTextAddr() uint64 {
	if x != nil {
		return x.TextAddr
	}
	return 0
}

func (x *Instance) GetStackSize() uint32 {
	if x != nil {
		return x.StackSize
	}
	return 0
}

func (x *Instance) GetStackUsage() uint32 {
	if x != nil {
		return x.StackUsage
	}
	return 0
}

func (x *Instance) GetGlobalsSize() uint32 {
	if x != nil {
		return x.GlobalsSize
	}
	return 0
}

func (x *Instance) GetMemorySize() uint32 {
	if x != nil {
		return x.MemorySize
	}
	return 0
}

func (x *Instance) GetMaxMemorySize() uint32 {
	if x != nil {
		return x.MaxMemorySize
	}
	return 0
}

func (x *Instance) GetStartFunc() *Function {
	if x != nil {
		return x.StartFunc
	}
	return nil
}

func (x *Instance) GetEntryFunc() *Function {
	if x != nil {
		return x.EntryFunc
	}
	return nil
}

func (x *Instance) GetSnapshot() *Snapshot {
	if x != nil {
		return x.Snapshot
	}
	return nil
}

type Function struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Index uint32 `protobuf:"varint,1,opt,name=index,proto3" json:"index,omitempty"`
	Addr  uint32 `protobuf:"varint,2,opt,name=addr,proto3" json:"addr,omitempty"`
}

func (x *Function) Reset() {
	*x = Function{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_manifest_manifest_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Function) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Function) ProtoMessage() {}

func (x *Function) ProtoReflect() protoreflect.Message {
	mi := &file_internal_manifest_manifest_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
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
	return file_internal_manifest_manifest_proto_rawDescGZIP(), []int{3}
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

type Snapshot struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Flags         uint64   `protobuf:"varint,1,opt,name=flags,proto3" json:"flags,omitempty"`
	Trap          int32    `protobuf:"varint,2,opt,name=trap,proto3" json:"trap,omitempty"`
	Result        int32    `protobuf:"varint,3,opt,name=result,proto3" json:"result,omitempty"`
	MonotonicTime uint64   `protobuf:"varint,4,opt,name=monotonic_time,json=monotonicTime,proto3" json:"monotonic_time,omitempty"`
	Breakpoints   []uint64 `protobuf:"varint,5,rep,packed,name=breakpoints,proto3" json:"breakpoints,omitempty"`
}

func (x *Snapshot) Reset() {
	*x = Snapshot{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_manifest_manifest_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Snapshot) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Snapshot) ProtoMessage() {}

func (x *Snapshot) ProtoReflect() protoreflect.Message {
	mi := &file_internal_manifest_manifest_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Snapshot.ProtoReflect.Descriptor instead.
func (*Snapshot) Descriptor() ([]byte, []int) {
	return file_internal_manifest_manifest_proto_rawDescGZIP(), []int{4}
}

func (x *Snapshot) GetFlags() uint64 {
	if x != nil {
		return x.Flags
	}
	return 0
}

func (x *Snapshot) GetTrap() int32 {
	if x != nil {
		return x.Trap
	}
	return 0
}

func (x *Snapshot) GetResult() int32 {
	if x != nil {
		return x.Result
	}
	return 0
}

func (x *Snapshot) GetMonotonicTime() uint64 {
	if x != nil {
		return x.MonotonicTime
	}
	return 0
}

func (x *Snapshot) GetBreakpoints() []uint64 {
	if x != nil {
		return x.Breakpoints
	}
	return nil
}

var File_internal_manifest_manifest_proto protoreflect.FileDescriptor

var file_internal_manifest_manifest_proto_rawDesc = []byte{
	0x0a, 0x20, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x6d, 0x61, 0x6e, 0x69, 0x66,
	0x65, 0x73, 0x74, 0x2f, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x12, 0x13, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x6d,
	0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x22, 0x35, 0x0a, 0x09, 0x42, 0x79, 0x74, 0x65, 0x52,
	0x61, 0x6e, 0x67, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x72, 0x74, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x03, 0x52, 0x05, 0x73, 0x74, 0x61, 0x72, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x73, 0x69,
	0x7a, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x73, 0x69, 0x7a, 0x65, 0x22, 0xb7,
	0x0a, 0x0a, 0x07, 0x50, 0x72, 0x6f, 0x67, 0x72, 0x61, 0x6d, 0x12, 0x29, 0x0a, 0x10, 0x6c, 0x69,
	0x62, 0x72, 0x61, 0x72, 0x79, 0x5f, 0x63, 0x68, 0x65, 0x63, 0x6b, 0x73, 0x75, 0x6d, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x06, 0x52, 0x0f, 0x6c, 0x69, 0x62, 0x72, 0x61, 0x72, 0x79, 0x43, 0x68, 0x65,
	0x63, 0x6b, 0x73, 0x75, 0x6d, 0x12, 0x23, 0x0a, 0x0d, 0x74, 0x65, 0x78, 0x74, 0x5f, 0x72, 0x65,
	0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0c, 0x74, 0x65,
	0x78, 0x74, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x1b, 0x0a, 0x09, 0x74, 0x65,
	0x78, 0x74, 0x5f, 0x61, 0x64, 0x64, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x04, 0x52, 0x08, 0x74,
	0x65, 0x78, 0x74, 0x41, 0x64, 0x64, 0x72, 0x12, 0x1b, 0x0a, 0x09, 0x74, 0x65, 0x78, 0x74, 0x5f,
	0x73, 0x69, 0x7a, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x08, 0x74, 0x65, 0x78, 0x74,
	0x53, 0x69, 0x7a, 0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x5f, 0x75, 0x73,
	0x61, 0x67, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0a, 0x73, 0x74, 0x61, 0x63, 0x6b,
	0x55, 0x73, 0x61, 0x67, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x67, 0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x73,
	0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0b, 0x67, 0x6c, 0x6f,
	0x62, 0x61, 0x6c, 0x73, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x6d, 0x65, 0x6d, 0x6f,
	0x72, 0x79, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0a, 0x6d,
	0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x2a, 0x0a, 0x11, 0x6d, 0x65, 0x6d,
	0x6f, 0x72, 0x79, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x5f, 0x6c, 0x69, 0x6d, 0x69, 0x74, 0x18, 0x08,
	0x20, 0x01, 0x28, 0x12, 0x52, 0x0f, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x69, 0x7a, 0x65,
	0x4c, 0x69, 0x6d, 0x69, 0x74, 0x12, 0x28, 0x0a, 0x10, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x5f,
	0x64, 0x61, 0x74, 0x61, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x09, 0x20, 0x01, 0x28, 0x0d, 0x52,
	0x0e, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x44, 0x61, 0x74, 0x61, 0x53, 0x69, 0x7a, 0x65, 0x12,
	0x1f, 0x0a, 0x0b, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x0a,
	0x20, 0x01, 0x28, 0x03, 0x52, 0x0a, 0x6d, 0x6f, 0x64, 0x75, 0x6c, 0x65, 0x53, 0x69, 0x7a, 0x65,
	0x12, 0x3a, 0x0a, 0x08, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x0b, 0x20, 0x03,
	0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e,
	0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x42, 0x79, 0x74, 0x65, 0x52, 0x61, 0x6e,
	0x67, 0x65, 0x52, 0x08, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x49, 0x0a, 0x10,
	0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e,
	0x18, 0x0c, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d,
	0x61, 0x67, 0x65, 0x2e, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x42, 0x79, 0x74,
	0x65, 0x52, 0x61, 0x6e, 0x67, 0x65, 0x52, 0x0f, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74,
	0x53, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x4e, 0x0a, 0x13, 0x65, 0x78, 0x70, 0x6f, 0x72,
	0x74, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x77, 0x72, 0x61, 0x70, 0x18, 0x0d,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67,
	0x65, 0x2e, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x42, 0x79, 0x74, 0x65, 0x52,
	0x61, 0x6e, 0x67, 0x65, 0x52, 0x11, 0x65, 0x78, 0x70, 0x6f, 0x72, 0x74, 0x53, 0x65, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x57, 0x72, 0x61, 0x70, 0x12, 0x45, 0x0a, 0x0e, 0x62, 0x75, 0x66, 0x66, 0x65,
	0x72, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x0e, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x6d, 0x61, 0x6e,
	0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x42, 0x79, 0x74, 0x65, 0x52, 0x61, 0x6e, 0x67, 0x65, 0x52,
	0x0d, 0x62, 0x75, 0x66, 0x66, 0x65, 0x72, 0x53, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x3b,
	0x0a, 0x1a, 0x62, 0x75, 0x66, 0x66, 0x65, 0x72, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e,
	0x5f, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x0f, 0x20, 0x01,
	0x28, 0x0d, 0x52, 0x17, 0x62, 0x75, 0x66, 0x66, 0x65, 0x72, 0x53, 0x65, 0x63, 0x74, 0x69, 0x6f,
	0x6e, 0x48, 0x65, 0x61, 0x64, 0x65, 0x72, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x43, 0x0a, 0x0d, 0x73,
	0x74, 0x61, 0x63, 0x6b, 0x5f, 0x73, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x10, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e,
	0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x42, 0x79, 0x74, 0x65, 0x52, 0x61, 0x6e,
	0x67, 0x65, 0x52, 0x0c, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x53, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e,
	0x12, 0x21, 0x0a, 0x0c, 0x67, 0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x73,
	0x18, 0x11, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0b, 0x67, 0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x54, 0x79,
	0x70, 0x65, 0x73, 0x12, 0x3c, 0x0a, 0x0a, 0x73, 0x74, 0x61, 0x72, 0x74, 0x5f, 0x66, 0x75, 0x6e,
	0x63, 0x18, 0x12, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69,
	0x6d, 0x61, 0x67, 0x65, 0x2e, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x46, 0x75,
	0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x09, 0x73, 0x74, 0x61, 0x72, 0x74, 0x46, 0x75, 0x6e,
	0x63, 0x12, 0x53, 0x0a, 0x0d, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x5f, 0x69, 0x6e, 0x64, 0x65, 0x78,
	0x65, 0x73, 0x18, 0x13, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2e, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e,
	0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x50,
	0x72, 0x6f, 0x67, 0x72, 0x61, 0x6d, 0x2e, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x49, 0x6e, 0x64, 0x65,
	0x78, 0x65, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x0c, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x49,
	0x6e, 0x64, 0x65, 0x78, 0x65, 0x73, 0x12, 0x4d, 0x0a, 0x0b, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x5f,
	0x61, 0x64, 0x64, 0x72, 0x73, 0x18, 0x14, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2c, 0x2e, 0x67, 0x61,
	0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73,
	0x74, 0x2e, 0x50, 0x72, 0x6f, 0x67, 0x72, 0x61, 0x6d, 0x2e, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x41,
	0x64, 0x64, 0x72, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x0a, 0x65, 0x6e, 0x74, 0x72, 0x79,
	0x41, 0x64, 0x64, 0x72, 0x73, 0x12, 0x26, 0x0a, 0x0f, 0x63, 0x61, 0x6c, 0x6c, 0x5f, 0x73, 0x69,
	0x74, 0x65, 0x73, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x15, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0d,
	0x63, 0x61, 0x6c, 0x6c, 0x53, 0x69, 0x74, 0x65, 0x73, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x26, 0x0a,
	0x0f, 0x66, 0x75, 0x6e, 0x63, 0x5f, 0x61, 0x64, 0x64, 0x72, 0x73, 0x5f, 0x73, 0x69, 0x7a, 0x65,
	0x18, 0x16, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0d, 0x66, 0x75, 0x6e, 0x63, 0x41, 0x64, 0x64, 0x72,
	0x73, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x61, 0x6e, 0x64, 0x6f, 0x6d, 0x18,
	0x17, 0x20, 0x01, 0x28, 0x08, 0x52, 0x06, 0x72, 0x61, 0x6e, 0x64, 0x6f, 0x6d, 0x12, 0x39, 0x0a,
	0x08, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x18, 0x18, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x1d, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x6d, 0x61, 0x6e,
	0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x53, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x52, 0x08,
	0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x1a, 0x3f, 0x0a, 0x11, 0x45, 0x6e, 0x74, 0x72,
	0x79, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x65, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a,
	0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12,
	0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x05,
	0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x1a, 0x3d, 0x0a, 0x0f, 0x45, 0x6e, 0x74,
	0x72, 0x79, 0x41, 0x64, 0x64, 0x72, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03,
	0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14,
	0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x05, 0x76,
	0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22, 0x8a, 0x03, 0x0a, 0x08, 0x49, 0x6e, 0x73,
	0x74, 0x61, 0x6e, 0x63, 0x65, 0x12, 0x1b, 0x0a, 0x09, 0x74, 0x65, 0x78, 0x74, 0x5f, 0x61, 0x64,
	0x64, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x08, 0x74, 0x65, 0x78, 0x74, 0x41, 0x64,
	0x64, 0x72, 0x12, 0x1d, 0x0a, 0x0a, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x5f, 0x73, 0x69, 0x7a, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x09, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x53, 0x69, 0x7a,
	0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x5f, 0x75, 0x73, 0x61, 0x67, 0x65,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0a, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x55, 0x73, 0x61,
	0x67, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x67, 0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x73, 0x5f, 0x73, 0x69,
	0x7a, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0b, 0x67, 0x6c, 0x6f, 0x62, 0x61, 0x6c,
	0x73, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x5f,
	0x73, 0x69, 0x7a, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0a, 0x6d, 0x65, 0x6d, 0x6f,
	0x72, 0x79, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x26, 0x0a, 0x0f, 0x6d, 0x61, 0x78, 0x5f, 0x6d, 0x65,
	0x6d, 0x6f, 0x72, 0x79, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0d, 0x52,
	0x0d, 0x6d, 0x61, 0x78, 0x4d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x3c,
	0x0a, 0x0a, 0x73, 0x74, 0x61, 0x72, 0x74, 0x5f, 0x66, 0x75, 0x6e, 0x63, 0x18, 0x07, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e,
	0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f,
	0x6e, 0x52, 0x09, 0x73, 0x74, 0x61, 0x72, 0x74, 0x46, 0x75, 0x6e, 0x63, 0x12, 0x3c, 0x0a, 0x0a,
	0x65, 0x6e, 0x74, 0x72, 0x79, 0x5f, 0x66, 0x75, 0x6e, 0x63, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x1d, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x6d, 0x61,
	0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x2e, 0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x52,
	0x09, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x46, 0x75, 0x6e, 0x63, 0x12, 0x39, 0x0a, 0x08, 0x73, 0x6e,
	0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x18, 0x09, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x67,
	0x61, 0x74, 0x65, 0x2e, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x2e, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65,
	0x73, 0x74, 0x2e, 0x53, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x52, 0x08, 0x73, 0x6e, 0x61,
	0x70, 0x73, 0x68, 0x6f, 0x74, 0x22, 0x34, 0x0a, 0x08, 0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f,
	0x6e, 0x12, 0x14, 0x0a, 0x05, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d,
	0x52, 0x05, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x12, 0x12, 0x0a, 0x04, 0x61, 0x64, 0x64, 0x72, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x61, 0x64, 0x64, 0x72, 0x22, 0x95, 0x01, 0x0a, 0x08,
	0x53, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x66, 0x6c, 0x61, 0x67,
	0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x05, 0x66, 0x6c, 0x61, 0x67, 0x73, 0x12, 0x12,
	0x0a, 0x04, 0x74, 0x72, 0x61, 0x70, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x04, 0x74, 0x72,
	0x61, 0x70, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x18, 0x03, 0x20, 0x01,
	0x28, 0x05, 0x52, 0x06, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x25, 0x0a, 0x0e, 0x6d, 0x6f,
	0x6e, 0x6f, 0x74, 0x6f, 0x6e, 0x69, 0x63, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01,
	0x28, 0x04, 0x52, 0x0d, 0x6d, 0x6f, 0x6e, 0x6f, 0x74, 0x6f, 0x6e, 0x69, 0x63, 0x54, 0x69, 0x6d,
	0x65, 0x12, 0x20, 0x0a, 0x0b, 0x62, 0x72, 0x65, 0x61, 0x6b, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x73,
	0x18, 0x05, 0x20, 0x03, 0x28, 0x04, 0x52, 0x0b, 0x62, 0x72, 0x65, 0x61, 0x6b, 0x70, 0x6f, 0x69,
	0x6e, 0x74, 0x73, 0x42, 0x21, 0x5a, 0x1f, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x63, 0x6f, 0x6d, 0x70,
	0x75, 0x74, 0x65, 0x72, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x6d, 0x61,
	0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_internal_manifest_manifest_proto_rawDescOnce sync.Once
	file_internal_manifest_manifest_proto_rawDescData = file_internal_manifest_manifest_proto_rawDesc
)

func file_internal_manifest_manifest_proto_rawDescGZIP() []byte {
	file_internal_manifest_manifest_proto_rawDescOnce.Do(func() {
		file_internal_manifest_manifest_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_manifest_manifest_proto_rawDescData)
	})
	return file_internal_manifest_manifest_proto_rawDescData
}

var file_internal_manifest_manifest_proto_msgTypes = make([]protoimpl.MessageInfo, 7)
var file_internal_manifest_manifest_proto_goTypes = []interface{}{
	(*ByteRange)(nil), // 0: gate.image.manifest.ByteRange
	(*Program)(nil),   // 1: gate.image.manifest.Program
	(*Instance)(nil),  // 2: gate.image.manifest.Instance
	(*Function)(nil),  // 3: gate.image.manifest.Function
	(*Snapshot)(nil),  // 4: gate.image.manifest.Snapshot
	nil,               // 5: gate.image.manifest.Program.EntryIndexesEntry
	nil,               // 6: gate.image.manifest.Program.EntryAddrsEntry
}
var file_internal_manifest_manifest_proto_depIdxs = []int32{
	0,  // 0: gate.image.manifest.Program.sections:type_name -> gate.image.manifest.ByteRange
	0,  // 1: gate.image.manifest.Program.snapshot_section:type_name -> gate.image.manifest.ByteRange
	0,  // 2: gate.image.manifest.Program.export_section_wrap:type_name -> gate.image.manifest.ByteRange
	0,  // 3: gate.image.manifest.Program.buffer_section:type_name -> gate.image.manifest.ByteRange
	0,  // 4: gate.image.manifest.Program.stack_section:type_name -> gate.image.manifest.ByteRange
	3,  // 5: gate.image.manifest.Program.start_func:type_name -> gate.image.manifest.Function
	5,  // 6: gate.image.manifest.Program.entry_indexes:type_name -> gate.image.manifest.Program.EntryIndexesEntry
	6,  // 7: gate.image.manifest.Program.entry_addrs:type_name -> gate.image.manifest.Program.EntryAddrsEntry
	4,  // 8: gate.image.manifest.Program.snapshot:type_name -> gate.image.manifest.Snapshot
	3,  // 9: gate.image.manifest.Instance.start_func:type_name -> gate.image.manifest.Function
	3,  // 10: gate.image.manifest.Instance.entry_func:type_name -> gate.image.manifest.Function
	4,  // 11: gate.image.manifest.Instance.snapshot:type_name -> gate.image.manifest.Snapshot
	12, // [12:12] is the sub-list for method output_type
	12, // [12:12] is the sub-list for method input_type
	12, // [12:12] is the sub-list for extension type_name
	12, // [12:12] is the sub-list for extension extendee
	0,  // [0:12] is the sub-list for field type_name
}

func init() { file_internal_manifest_manifest_proto_init() }
func file_internal_manifest_manifest_proto_init() {
	if File_internal_manifest_manifest_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_internal_manifest_manifest_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ByteRange); i {
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
		file_internal_manifest_manifest_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Program); i {
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
		file_internal_manifest_manifest_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Instance); i {
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
		file_internal_manifest_manifest_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Function); i {
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
		file_internal_manifest_manifest_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Snapshot); i {
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
			RawDescriptor: file_internal_manifest_manifest_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   7,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_internal_manifest_manifest_proto_goTypes,
		DependencyIndexes: file_internal_manifest_manifest_proto_depIdxs,
		MessageInfos:      file_internal_manifest_manifest_proto_msgTypes,
	}.Build()
	File_internal_manifest_manifest_proto = out.File
	file_internal_manifest_manifest_proto_rawDesc = nil
	file_internal_manifest_manifest_proto_goTypes = nil
	file_internal_manifest_manifest_proto_depIdxs = nil
}
