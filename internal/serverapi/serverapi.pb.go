// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: internal/serverapi/serverapi.proto

package serverapi // import "github.com/tsavola/gate/internal/serverapi"

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"

import io "io"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

type State int32

const (
	State_NONEXISTENT State = 0
	State_RUNNING     State = 1
	State_SUSPENDED   State = 2
	State_HALTED      State = 3
	State_TERMINATED  State = 4
	State_KILLED      State = 5
)

var State_name = map[int32]string{
	0: "NONEXISTENT",
	1: "RUNNING",
	2: "SUSPENDED",
	3: "HALTED",
	4: "TERMINATED",
	5: "KILLED",
}
var State_value = map[string]int32{
	"NONEXISTENT": 0,
	"RUNNING":     1,
	"SUSPENDED":   2,
	"HALTED":      3,
	"TERMINATED":  4,
	"KILLED":      5,
}

func (x State) String() string {
	return proto.EnumName(State_name, int32(x))
}
func (State) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_3caa1f8a6e1a38f3, []int{0}
}

type Cause int32

const (
	Cause_NORMAL                            Cause = 0
	Cause_UNREACHABLE                       Cause = 3
	Cause_CALL_STACK_EXHAUSTED              Cause = 4
	Cause_MEMORY_ACCESS_OUT_OF_BOUNDS       Cause = 5
	Cause_INDIRECT_CALL_INDEX_OUT_OF_BOUNDS Cause = 6
	Cause_INDIRECT_CALL_SIGNATURE_MISMATCH  Cause = 7
	Cause_INTEGER_DIVIDE_BY_ZERO            Cause = 8
	Cause_INTEGER_OVERFLOW                  Cause = 9
	Cause_ABI_DEFICIENCY                    Cause = 26
	Cause_ABI_VIOLATION                     Cause = 27
	Cause_INTERNAL                          Cause = 29
)

var Cause_name = map[int32]string{
	0:  "NORMAL",
	3:  "UNREACHABLE",
	4:  "CALL_STACK_EXHAUSTED",
	5:  "MEMORY_ACCESS_OUT_OF_BOUNDS",
	6:  "INDIRECT_CALL_INDEX_OUT_OF_BOUNDS",
	7:  "INDIRECT_CALL_SIGNATURE_MISMATCH",
	8:  "INTEGER_DIVIDE_BY_ZERO",
	9:  "INTEGER_OVERFLOW",
	26: "ABI_DEFICIENCY",
	27: "ABI_VIOLATION",
	29: "INTERNAL",
}
var Cause_value = map[string]int32{
	"NORMAL":                            0,
	"UNREACHABLE":                       3,
	"CALL_STACK_EXHAUSTED":              4,
	"MEMORY_ACCESS_OUT_OF_BOUNDS":       5,
	"INDIRECT_CALL_INDEX_OUT_OF_BOUNDS": 6,
	"INDIRECT_CALL_SIGNATURE_MISMATCH":  7,
	"INTEGER_DIVIDE_BY_ZERO":            8,
	"INTEGER_OVERFLOW":                  9,
	"ABI_DEFICIENCY":                    26,
	"ABI_VIOLATION":                     27,
	"INTERNAL":                          29,
}

func (x Cause) String() string {
	return proto.EnumName(Cause_name, int32(x))
}
func (Cause) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_3caa1f8a6e1a38f3, []int{1}
}

type ModuleRef struct {
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
}

func (m *ModuleRef) Reset()         { *m = ModuleRef{} }
func (m *ModuleRef) String() string { return proto.CompactTextString(m) }
func (*ModuleRef) ProtoMessage()    {}
func (*ModuleRef) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_3caa1f8a6e1a38f3, []int{0}
}
func (m *ModuleRef) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *ModuleRef) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_ModuleRef.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *ModuleRef) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ModuleRef.Merge(dst, src)
}
func (m *ModuleRef) XXX_Size() int {
	return m.Size()
}
func (m *ModuleRef) XXX_DiscardUnknown() {
	xxx_messageInfo_ModuleRef.DiscardUnknown(m)
}

var xxx_messageInfo_ModuleRef proto.InternalMessageInfo

type ModuleRefs struct {
	Modules []ModuleRef `protobuf:"bytes,1,rep,name=modules" json:"modules"`
}

func (m *ModuleRefs) Reset()         { *m = ModuleRefs{} }
func (m *ModuleRefs) String() string { return proto.CompactTextString(m) }
func (*ModuleRefs) ProtoMessage()    {}
func (*ModuleRefs) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_3caa1f8a6e1a38f3, []int{1}
}
func (m *ModuleRefs) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *ModuleRefs) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_ModuleRefs.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *ModuleRefs) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ModuleRefs.Merge(dst, src)
}
func (m *ModuleRefs) XXX_Size() int {
	return m.Size()
}
func (m *ModuleRefs) XXX_DiscardUnknown() {
	xxx_messageInfo_ModuleRefs.DiscardUnknown(m)
}

var xxx_messageInfo_ModuleRefs proto.InternalMessageInfo

type Status struct {
	State  State  `protobuf:"varint,1,opt,name=state,proto3,enum=server.State" json:"state,omitempty"`
	Cause  Cause  `protobuf:"varint,2,opt,name=cause,proto3,enum=server.Cause" json:"cause,omitempty"`
	Result int32  `protobuf:"varint,3,opt,name=result,proto3" json:"result,omitempty"`
	Error  string `protobuf:"bytes,4,opt,name=error,proto3" json:"error,omitempty"`
	Debug  string `protobuf:"bytes,5,opt,name=debug,proto3" json:"debug,omitempty"`
}

func (m *Status) Reset()         { *m = Status{} }
func (m *Status) String() string { return proto.CompactTextString(m) }
func (*Status) ProtoMessage()    {}
func (*Status) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_3caa1f8a6e1a38f3, []int{2}
}
func (m *Status) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Status) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Status.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *Status) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Status.Merge(dst, src)
}
func (m *Status) XXX_Size() int {
	return m.Size()
}
func (m *Status) XXX_DiscardUnknown() {
	xxx_messageInfo_Status.DiscardUnknown(m)
}

var xxx_messageInfo_Status proto.InternalMessageInfo

type InstanceStatus struct {
	Instance  string `protobuf:"bytes,1,opt,name=instance,proto3" json:"instance,omitempty"`
	Status    Status `protobuf:"bytes,2,opt,name=status" json:"status"`
	Transient bool   `protobuf:"varint,3,opt,name=transient,proto3" json:"transient,omitempty"`
}

func (m *InstanceStatus) Reset()         { *m = InstanceStatus{} }
func (m *InstanceStatus) String() string { return proto.CompactTextString(m) }
func (*InstanceStatus) ProtoMessage()    {}
func (*InstanceStatus) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_3caa1f8a6e1a38f3, []int{3}
}
func (m *InstanceStatus) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *InstanceStatus) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_InstanceStatus.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *InstanceStatus) XXX_Merge(src proto.Message) {
	xxx_messageInfo_InstanceStatus.Merge(dst, src)
}
func (m *InstanceStatus) XXX_Size() int {
	return m.Size()
}
func (m *InstanceStatus) XXX_DiscardUnknown() {
	xxx_messageInfo_InstanceStatus.DiscardUnknown(m)
}

var xxx_messageInfo_InstanceStatus proto.InternalMessageInfo

type Instances struct {
	Instances []InstanceStatus `protobuf:"bytes,1,rep,name=instances" json:"instances"`
}

func (m *Instances) Reset()         { *m = Instances{} }
func (m *Instances) String() string { return proto.CompactTextString(m) }
func (*Instances) ProtoMessage()    {}
func (*Instances) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_3caa1f8a6e1a38f3, []int{4}
}
func (m *Instances) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Instances) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Instances.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *Instances) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Instances.Merge(dst, src)
}
func (m *Instances) XXX_Size() int {
	return m.Size()
}
func (m *Instances) XXX_DiscardUnknown() {
	xxx_messageInfo_Instances.DiscardUnknown(m)
}

var xxx_messageInfo_Instances proto.InternalMessageInfo

type IOConnection struct {
	Connected bool   `protobuf:"varint,1,opt,name=connected,proto3" json:"connected,omitempty"`
	Status    Status `protobuf:"bytes,2,opt,name=status" json:"status"`
}

func (m *IOConnection) Reset()         { *m = IOConnection{} }
func (m *IOConnection) String() string { return proto.CompactTextString(m) }
func (*IOConnection) ProtoMessage()    {}
func (*IOConnection) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_3caa1f8a6e1a38f3, []int{5}
}
func (m *IOConnection) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *IOConnection) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_IOConnection.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *IOConnection) XXX_Merge(src proto.Message) {
	xxx_messageInfo_IOConnection.Merge(dst, src)
}
func (m *IOConnection) XXX_Size() int {
	return m.Size()
}
func (m *IOConnection) XXX_DiscardUnknown() {
	xxx_messageInfo_IOConnection.DiscardUnknown(m)
}

var xxx_messageInfo_IOConnection proto.InternalMessageInfo

type ConnectionStatus struct {
	Status Status `protobuf:"bytes,1,opt,name=status" json:"status"`
}

func (m *ConnectionStatus) Reset()         { *m = ConnectionStatus{} }
func (m *ConnectionStatus) String() string { return proto.CompactTextString(m) }
func (*ConnectionStatus) ProtoMessage()    {}
func (*ConnectionStatus) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_3caa1f8a6e1a38f3, []int{6}
}
func (m *ConnectionStatus) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *ConnectionStatus) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_ConnectionStatus.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *ConnectionStatus) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ConnectionStatus.Merge(dst, src)
}
func (m *ConnectionStatus) XXX_Size() int {
	return m.Size()
}
func (m *ConnectionStatus) XXX_DiscardUnknown() {
	xxx_messageInfo_ConnectionStatus.DiscardUnknown(m)
}

var xxx_messageInfo_ConnectionStatus proto.InternalMessageInfo

func init() {
	proto.RegisterType((*ModuleRef)(nil), "server.ModuleRef")
	proto.RegisterType((*ModuleRefs)(nil), "server.ModuleRefs")
	proto.RegisterType((*Status)(nil), "server.Status")
	proto.RegisterType((*InstanceStatus)(nil), "server.InstanceStatus")
	proto.RegisterType((*Instances)(nil), "server.Instances")
	proto.RegisterType((*IOConnection)(nil), "server.IOConnection")
	proto.RegisterType((*ConnectionStatus)(nil), "server.ConnectionStatus")
	proto.RegisterEnum("server.State", State_name, State_value)
	proto.RegisterEnum("server.Cause", Cause_name, Cause_value)
}
func (m *ModuleRef) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *ModuleRef) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Id) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintServerapi(dAtA, i, uint64(len(m.Id)))
		i += copy(dAtA[i:], m.Id)
	}
	return i, nil
}

func (m *ModuleRefs) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *ModuleRefs) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Modules) > 0 {
		for _, msg := range m.Modules {
			dAtA[i] = 0xa
			i++
			i = encodeVarintServerapi(dAtA, i, uint64(msg.Size()))
			n, err := msg.MarshalTo(dAtA[i:])
			if err != nil {
				return 0, err
			}
			i += n
		}
	}
	return i, nil
}

func (m *Status) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Status) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.State != 0 {
		dAtA[i] = 0x8
		i++
		i = encodeVarintServerapi(dAtA, i, uint64(m.State))
	}
	if m.Cause != 0 {
		dAtA[i] = 0x10
		i++
		i = encodeVarintServerapi(dAtA, i, uint64(m.Cause))
	}
	if m.Result != 0 {
		dAtA[i] = 0x18
		i++
		i = encodeVarintServerapi(dAtA, i, uint64(m.Result))
	}
	if len(m.Error) > 0 {
		dAtA[i] = 0x22
		i++
		i = encodeVarintServerapi(dAtA, i, uint64(len(m.Error)))
		i += copy(dAtA[i:], m.Error)
	}
	if len(m.Debug) > 0 {
		dAtA[i] = 0x2a
		i++
		i = encodeVarintServerapi(dAtA, i, uint64(len(m.Debug)))
		i += copy(dAtA[i:], m.Debug)
	}
	return i, nil
}

func (m *InstanceStatus) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *InstanceStatus) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Instance) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintServerapi(dAtA, i, uint64(len(m.Instance)))
		i += copy(dAtA[i:], m.Instance)
	}
	dAtA[i] = 0x12
	i++
	i = encodeVarintServerapi(dAtA, i, uint64(m.Status.Size()))
	n1, err := m.Status.MarshalTo(dAtA[i:])
	if err != nil {
		return 0, err
	}
	i += n1
	if m.Transient {
		dAtA[i] = 0x18
		i++
		if m.Transient {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i++
	}
	return i, nil
}

func (m *Instances) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Instances) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Instances) > 0 {
		for _, msg := range m.Instances {
			dAtA[i] = 0xa
			i++
			i = encodeVarintServerapi(dAtA, i, uint64(msg.Size()))
			n, err := msg.MarshalTo(dAtA[i:])
			if err != nil {
				return 0, err
			}
			i += n
		}
	}
	return i, nil
}

func (m *IOConnection) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *IOConnection) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.Connected {
		dAtA[i] = 0x8
		i++
		if m.Connected {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i++
	}
	dAtA[i] = 0x12
	i++
	i = encodeVarintServerapi(dAtA, i, uint64(m.Status.Size()))
	n2, err := m.Status.MarshalTo(dAtA[i:])
	if err != nil {
		return 0, err
	}
	i += n2
	return i, nil
}

func (m *ConnectionStatus) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *ConnectionStatus) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	dAtA[i] = 0xa
	i++
	i = encodeVarintServerapi(dAtA, i, uint64(m.Status.Size()))
	n3, err := m.Status.MarshalTo(dAtA[i:])
	if err != nil {
		return 0, err
	}
	i += n3
	return i, nil
}

func encodeVarintServerapi(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *ModuleRef) Size() (n int) {
	var l int
	_ = l
	l = len(m.Id)
	if l > 0 {
		n += 1 + l + sovServerapi(uint64(l))
	}
	return n
}

func (m *ModuleRefs) Size() (n int) {
	var l int
	_ = l
	if len(m.Modules) > 0 {
		for _, e := range m.Modules {
			l = e.Size()
			n += 1 + l + sovServerapi(uint64(l))
		}
	}
	return n
}

func (m *Status) Size() (n int) {
	var l int
	_ = l
	if m.State != 0 {
		n += 1 + sovServerapi(uint64(m.State))
	}
	if m.Cause != 0 {
		n += 1 + sovServerapi(uint64(m.Cause))
	}
	if m.Result != 0 {
		n += 1 + sovServerapi(uint64(m.Result))
	}
	l = len(m.Error)
	if l > 0 {
		n += 1 + l + sovServerapi(uint64(l))
	}
	l = len(m.Debug)
	if l > 0 {
		n += 1 + l + sovServerapi(uint64(l))
	}
	return n
}

func (m *InstanceStatus) Size() (n int) {
	var l int
	_ = l
	l = len(m.Instance)
	if l > 0 {
		n += 1 + l + sovServerapi(uint64(l))
	}
	l = m.Status.Size()
	n += 1 + l + sovServerapi(uint64(l))
	if m.Transient {
		n += 2
	}
	return n
}

func (m *Instances) Size() (n int) {
	var l int
	_ = l
	if len(m.Instances) > 0 {
		for _, e := range m.Instances {
			l = e.Size()
			n += 1 + l + sovServerapi(uint64(l))
		}
	}
	return n
}

func (m *IOConnection) Size() (n int) {
	var l int
	_ = l
	if m.Connected {
		n += 2
	}
	l = m.Status.Size()
	n += 1 + l + sovServerapi(uint64(l))
	return n
}

func (m *ConnectionStatus) Size() (n int) {
	var l int
	_ = l
	l = m.Status.Size()
	n += 1 + l + sovServerapi(uint64(l))
	return n
}

func sovServerapi(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozServerapi(x uint64) (n int) {
	return sovServerapi(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *ModuleRef) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowServerapi
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: ModuleRef: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: ModuleRef: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Id", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthServerapi
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Id = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipServerapi(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthServerapi
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *ModuleRefs) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowServerapi
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: ModuleRefs: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: ModuleRefs: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Modules", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthServerapi
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Modules = append(m.Modules, ModuleRef{})
			if err := m.Modules[len(m.Modules)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipServerapi(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthServerapi
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Status) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowServerapi
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Status: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Status: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field State", wireType)
			}
			m.State = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.State |= (State(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Cause", wireType)
			}
			m.Cause = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Cause |= (Cause(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Result", wireType)
			}
			m.Result = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Result |= (int32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Error", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthServerapi
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Error = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Debug", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthServerapi
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Debug = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipServerapi(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthServerapi
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *InstanceStatus) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowServerapi
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: InstanceStatus: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: InstanceStatus: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Instance", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthServerapi
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Instance = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Status", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthServerapi
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Status.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Transient", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.Transient = bool(v != 0)
		default:
			iNdEx = preIndex
			skippy, err := skipServerapi(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthServerapi
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Instances) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowServerapi
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Instances: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Instances: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Instances", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthServerapi
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Instances = append(m.Instances, InstanceStatus{})
			if err := m.Instances[len(m.Instances)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipServerapi(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthServerapi
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *IOConnection) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowServerapi
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: IOConnection: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: IOConnection: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Connected", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.Connected = bool(v != 0)
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Status", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthServerapi
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Status.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipServerapi(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthServerapi
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *ConnectionStatus) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowServerapi
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: ConnectionStatus: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: ConnectionStatus: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Status", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthServerapi
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Status.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipServerapi(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthServerapi
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipServerapi(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowServerapi
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowServerapi
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			iNdEx += length
			if length < 0 {
				return 0, ErrInvalidLengthServerapi
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowServerapi
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipServerapi(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthServerapi = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowServerapi   = fmt.Errorf("proto: integer overflow")
)

func init() {
	proto.RegisterFile("internal/serverapi/serverapi.proto", fileDescriptor_serverapi_3caa1f8a6e1a38f3)
}

var fileDescriptor_serverapi_3caa1f8a6e1a38f3 = []byte{
	// 665 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x94, 0x54, 0xdf, 0x4f, 0xd3, 0x40,
	0x1c, 0x5f, 0x37, 0x3a, 0xb6, 0xef, 0x60, 0x1e, 0x17, 0x42, 0x16, 0xd0, 0x31, 0xab, 0x26, 0x84,
	0x98, 0x2d, 0xe2, 0x9b, 0x2f, 0xda, 0xb5, 0xc7, 0x76, 0xa1, 0xbb, 0x9a, 0x6b, 0x87, 0x40, 0x4c,
	0x9a, 0xb2, 0x9d, 0xd8, 0x64, 0xb4, 0xa4, 0xed, 0x88, 0x7f, 0x86, 0x89, 0x89, 0x7f, 0x13, 0x8f,
	0x3c, 0xfa, 0x64, 0x14, 0xfe, 0x11, 0xd3, 0x5f, 0x4c, 0xf4, 0xc1, 0xf8, 0xd6, 0xcf, 0x8f, 0xfb,
	0x7c, 0x7f, 0x5c, 0x73, 0xa0, 0x78, 0x7e, 0x2c, 0x42, 0xdf, 0x9d, 0xf5, 0x22, 0x11, 0x5e, 0x8a,
	0xd0, 0xbd, 0xf0, 0x16, 0x5f, 0xdd, 0x8b, 0x30, 0x88, 0x03, 0x5c, 0xcd, 0x08, 0x65, 0x0b, 0xea,
	0xa3, 0x60, 0x3a, 0x9f, 0x09, 0x2e, 0x3e, 0xe0, 0x26, 0x94, 0xbd, 0x69, 0x4b, 0xea, 0x48, 0x3b,
	0x75, 0x5e, 0xf6, 0xa6, 0xca, 0x6b, 0x80, 0x3b, 0x31, 0xc2, 0x2f, 0x60, 0xf9, 0x3c, 0x45, 0x51,
	0x4b, 0xea, 0x54, 0x76, 0x1a, 0x7b, 0x6b, 0xdd, 0x2c, 0xa4, 0x7b, 0x67, 0xea, 0x2f, 0x5d, 0x7d,
	0xdf, 0x2e, 0xf1, 0xc2, 0xa7, 0x7c, 0x95, 0xa0, 0x6a, 0xc5, 0x6e, 0x3c, 0x8f, 0xf0, 0x13, 0x90,
	0xa3, 0xd8, 0x8d, 0x45, 0x1a, 0xdf, 0xdc, 0x5b, 0x2d, 0xce, 0x26, 0xb2, 0xe0, 0x99, 0x96, 0x98,
	0x26, 0xee, 0x3c, 0x12, 0xad, 0xf2, 0x7d, 0x93, 0x96, 0x90, 0x3c, 0xd3, 0xf0, 0x06, 0x54, 0x43,
	0x11, 0xcd, 0x67, 0x71, 0xab, 0xd2, 0x91, 0x76, 0x64, 0x9e, 0x23, 0xbc, 0x0e, 0xb2, 0x08, 0xc3,
	0x20, 0x6c, 0x2d, 0xa5, 0x03, 0x64, 0x20, 0x61, 0xa7, 0xe2, 0x74, 0x7e, 0xd6, 0x92, 0x33, 0x36,
	0x05, 0xca, 0x27, 0x68, 0x52, 0x3f, 0x8a, 0x5d, 0x7f, 0x22, 0xf2, 0xfe, 0x36, 0xa1, 0xe6, 0xe5,
	0x4c, 0xbe, 0x81, 0x3b, 0x8c, 0x9f, 0x43, 0x35, 0x4a, 0x5d, 0x69, 0x5f, 0x8d, 0xbd, 0xe6, 0xef,
	0xcd, 0xcf, 0xa3, 0x7c, 0xea, 0xdc, 0x83, 0x1f, 0x42, 0x3d, 0x0e, 0x5d, 0x3f, 0xf2, 0x84, 0x9f,
	0xb5, 0x58, 0xe3, 0x0b, 0x42, 0x19, 0x40, 0xbd, 0xa8, 0x1c, 0xe1, 0x57, 0x50, 0x2f, 0x8a, 0x14,
	0x4b, 0xdd, 0x28, 0xb2, 0xef, 0xf7, 0x97, 0xd7, 0x58, 0xd8, 0x95, 0x13, 0x58, 0xa1, 0xa6, 0x16,
	0xf8, 0xbe, 0x98, 0xc4, 0x5e, 0xe0, 0x27, 0x65, 0x27, 0x19, 0x12, 0xd9, 0x1d, 0xd6, 0xf8, 0x82,
	0xf8, 0xbf, 0x11, 0x94, 0x37, 0x80, 0x16, 0xc9, 0xf9, 0x82, 0x16, 0x09, 0xd2, 0xbf, 0x13, 0x76,
	0xdf, 0x83, 0x9c, 0xde, 0x2c, 0x7e, 0x00, 0x0d, 0x66, 0x32, 0x72, 0x44, 0x2d, 0x9b, 0x30, 0x1b,
	0x95, 0x70, 0x03, 0x96, 0xf9, 0x98, 0x31, 0xca, 0x06, 0x48, 0xc2, 0xab, 0x50, 0xb7, 0xc6, 0xd6,
	0x5b, 0xc2, 0x74, 0xa2, 0xa3, 0x32, 0x06, 0xa8, 0x0e, 0x55, 0xc3, 0x26, 0x3a, 0xaa, 0xe0, 0x26,
	0x80, 0x4d, 0xf8, 0x88, 0x32, 0x35, 0xc1, 0x4b, 0x89, 0x76, 0x40, 0x0d, 0x83, 0xe8, 0x48, 0xde,
	0xfd, 0x52, 0x06, 0x39, 0xfd, 0x27, 0x12, 0x96, 0x99, 0x7c, 0xa4, 0x1a, 0xa8, 0x94, 0x94, 0x1a,
	0x33, 0x4e, 0x54, 0x6d, 0xa8, 0xf6, 0x0d, 0x82, 0x2a, 0xb8, 0x05, 0xeb, 0x9a, 0x6a, 0x18, 0x8e,
	0x65, 0xab, 0xda, 0x81, 0x43, 0x8e, 0x86, 0xea, 0xd8, 0xca, 0xc2, 0xb6, 0x61, 0x6b, 0x44, 0x46,
	0x26, 0x3f, 0x76, 0x54, 0x4d, 0x23, 0x96, 0xe5, 0x98, 0x63, 0xdb, 0x31, 0xf7, 0x9d, 0xbe, 0x39,
	0x66, 0xba, 0x85, 0x64, 0xfc, 0x0c, 0x1e, 0x53, 0xa6, 0x53, 0x4e, 0x34, 0xdb, 0x49, 0x33, 0x28,
	0xd3, 0xc9, 0xd1, 0x1f, 0xb6, 0x2a, 0x7e, 0x0a, 0x9d, 0xfb, 0x36, 0x8b, 0x0e, 0x98, 0x6a, 0x8f,
	0x39, 0x71, 0x46, 0xd4, 0x1a, 0xa9, 0xb6, 0x36, 0x44, 0xcb, 0x78, 0x13, 0x36, 0x28, 0xb3, 0xc9,
	0x80, 0x70, 0x47, 0xa7, 0x87, 0x54, 0x27, 0x4e, 0xff, 0xd8, 0x39, 0x21, 0xdc, 0x44, 0x35, 0xbc,
	0x0e, 0xa8, 0xd0, 0xcc, 0x43, 0xc2, 0xf7, 0x0d, 0xf3, 0x1d, 0xaa, 0x63, 0x0c, 0x4d, 0xb5, 0x4f,
	0x1d, 0x9d, 0xec, 0x53, 0x8d, 0x12, 0xa6, 0x1d, 0xa3, 0x4d, 0xbc, 0x06, 0xab, 0x09, 0x77, 0x48,
	0x4d, 0x43, 0xb5, 0xa9, 0xc9, 0xd0, 0x16, 0x5e, 0x81, 0x5a, 0x72, 0x98, 0x33, 0xd5, 0x40, 0x8f,
	0xfa, 0xc3, 0xab, 0x9f, 0xed, 0xd2, 0xd5, 0x4d, 0x5b, 0xba, 0xbe, 0x69, 0x4b, 0x3f, 0x6e, 0xda,
	0xd2, 0xe7, 0xdb, 0x76, 0xe9, 0xfa, 0xb6, 0x5d, 0xfa, 0x76, 0xdb, 0x2e, 0x9d, 0xec, 0x9e, 0x79,
	0xf1, 0xc7, 0xf9, 0x69, 0x77, 0x12, 0x9c, 0xf7, 0xe2, 0xc8, 0xbd, 0x0c, 0x66, 0x6e, 0xef, 0xcc,
	0x8d, 0x45, 0xef, 0xef, 0xd7, 0xe2, 0xb4, 0x9a, 0x3e, 0x12, 0x2f, 0x7f, 0x05, 0x00, 0x00, 0xff,
	0xff, 0xc6, 0x96, 0x87, 0xfd, 0x4a, 0x04, 0x00, 0x00,
}
