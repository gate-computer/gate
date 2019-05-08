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

type Status_State int32

const (
	Status_nonexistent Status_State = 0
	Status_running     Status_State = 1
	Status_suspended   Status_State = 2
	Status_terminated  Status_State = 3
	Status_killed      Status_State = 4
)

var Status_State_name = map[int32]string{
	0: "nonexistent",
	1: "running",
	2: "suspended",
	3: "terminated",
	4: "killed",
}
var Status_State_value = map[string]int32{
	"nonexistent": 0,
	"running":     1,
	"suspended":   2,
	"terminated":  3,
	"killed":      4,
}

func (x Status_State) String() string {
	return proto.EnumName(Status_State_name, int32(x))
}
func (Status_State) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_5ce57fe6f7e08012, []int{2, 0}
}

type Status_Cause int32

const (
	Status_DEFAULT                           Status_Cause = 0
	Status_no_function                       Status_Cause = 1
	Status_unreachable                       Status_Cause = 3
	Status_call_stack_exhausted              Status_Cause = 4
	Status_memory_access_out_of_bounds       Status_Cause = 5
	Status_indirect_call_index_out_of_bounds Status_Cause = 6
	Status_indirect_call_signature_mismatch  Status_Cause = 7
	Status_integer_divide_by_zero            Status_Cause = 8
	Status_integer_overflow                  Status_Cause = 9
	Status_abi_violation                     Status_Cause = 65
)

var Status_Cause_name = map[int32]string{
	0:  "DEFAULT",
	1:  "no_function",
	3:  "unreachable",
	4:  "call_stack_exhausted",
	5:  "memory_access_out_of_bounds",
	6:  "indirect_call_index_out_of_bounds",
	7:  "indirect_call_signature_mismatch",
	8:  "integer_divide_by_zero",
	9:  "integer_overflow",
	65: "abi_violation",
}
var Status_Cause_value = map[string]int32{
	"DEFAULT":                           0,
	"no_function":                       1,
	"unreachable":                       3,
	"call_stack_exhausted":              4,
	"memory_access_out_of_bounds":       5,
	"indirect_call_index_out_of_bounds": 6,
	"indirect_call_signature_mismatch":  7,
	"integer_divide_by_zero":            8,
	"integer_overflow":                  9,
	"abi_violation":                     65,
}

func (x Status_Cause) String() string {
	return proto.EnumName(Status_Cause_name, int32(x))
}
func (Status_Cause) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_5ce57fe6f7e08012, []int{2, 1}
}

type ModuleRef struct {
	Key string `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
}

func (m *ModuleRef) Reset()         { *m = ModuleRef{} }
func (m *ModuleRef) String() string { return proto.CompactTextString(m) }
func (*ModuleRef) ProtoMessage()    {}
func (*ModuleRef) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_5ce57fe6f7e08012, []int{0}
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
	return fileDescriptor_serverapi_5ce57fe6f7e08012, []int{1}
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
	State  Status_State `protobuf:"varint,1,opt,name=state,proto3,enum=server.Status_State" json:"state,omitempty"`
	Cause  Status_Cause `protobuf:"varint,2,opt,name=cause,proto3,enum=server.Status_Cause" json:"cause,omitempty"`
	Result int32        `protobuf:"varint,3,opt,name=result,proto3" json:"result,omitempty"`
	Error  string       `protobuf:"bytes,4,opt,name=error,proto3" json:"error,omitempty"`
	Debug  string       `protobuf:"bytes,5,opt,name=debug,proto3" json:"debug,omitempty"`
}

func (m *Status) Reset()         { *m = Status{} }
func (m *Status) String() string { return proto.CompactTextString(m) }
func (*Status) ProtoMessage()    {}
func (*Status) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_5ce57fe6f7e08012, []int{2}
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
	Instance string `protobuf:"bytes,1,opt,name=instance,proto3" json:"instance,omitempty"`
	Status   Status `protobuf:"bytes,2,opt,name=status" json:"status"`
}

func (m *InstanceStatus) Reset()         { *m = InstanceStatus{} }
func (m *InstanceStatus) String() string { return proto.CompactTextString(m) }
func (*InstanceStatus) ProtoMessage()    {}
func (*InstanceStatus) Descriptor() ([]byte, []int) {
	return fileDescriptor_serverapi_5ce57fe6f7e08012, []int{3}
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
	return fileDescriptor_serverapi_5ce57fe6f7e08012, []int{4}
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
	return fileDescriptor_serverapi_5ce57fe6f7e08012, []int{5}
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
	return fileDescriptor_serverapi_5ce57fe6f7e08012, []int{6}
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
	proto.RegisterEnum("server.Status_State", Status_State_name, Status_State_value)
	proto.RegisterEnum("server.Status_Cause", Status_Cause_name, Status_Cause_value)
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
	if len(m.Key) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintServerapi(dAtA, i, uint64(len(m.Key)))
		i += copy(dAtA[i:], m.Key)
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
	l = len(m.Key)
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
				return fmt.Errorf("proto: wrong wireType = %d for field Key", wireType)
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
			m.Key = string(dAtA[iNdEx:postIndex])
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
				m.State |= (Status_State(b) & 0x7F) << shift
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
				m.Cause |= (Status_Cause(b) & 0x7F) << shift
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
	proto.RegisterFile("internal/serverapi/serverapi.proto", fileDescriptor_serverapi_5ce57fe6f7e08012)
}

var fileDescriptor_serverapi_5ce57fe6f7e08012 = []byte{
	// 617 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x94, 0x94, 0xcb, 0x6e, 0xd3, 0x4e,
	0x14, 0xc6, 0xe3, 0xe6, 0xd2, 0xe6, 0xe4, 0xdf, 0xfc, 0xa7, 0xa3, 0xa8, 0x8a, 0x0a, 0xa4, 0xc1,
	0x02, 0xa9, 0xaa, 0x50, 0x22, 0xca, 0x8e, 0x0d, 0xb4, 0xe5, 0x56, 0x09, 0x04, 0x32, 0xb0, 0xe9,
	0xc6, 0x1a, 0xdb, 0x27, 0xc9, 0xa8, 0xf6, 0x4c, 0x35, 0x97, 0xd0, 0xb2, 0xe5, 0x05, 0x78, 0xac,
	0x2e, 0xbb, 0x64, 0x85, 0xa0, 0x7d, 0x0d, 0x16, 0xc8, 0x63, 0xbb, 0x51, 0x01, 0x09, 0xb1, 0xca,
	0x7c, 0xdf, 0xf9, 0xe6, 0x97, 0x33, 0x9e, 0x63, 0x83, 0xcf, 0x85, 0x41, 0x25, 0x58, 0x3a, 0xd6,
	0xa8, 0xe6, 0xa8, 0xd8, 0x31, 0x5f, 0xac, 0x46, 0xc7, 0x4a, 0x1a, 0x49, 0x5b, 0x85, 0xe1, 0xdf,
	0x82, 0xf6, 0x2b, 0x99, 0xd8, 0x14, 0x03, 0x9c, 0x50, 0x02, 0xf5, 0x23, 0x3c, 0xed, 0x7b, 0x43,
	0x6f, 0xab, 0x1d, 0xe4, 0x4b, 0xff, 0x11, 0xc0, 0x55, 0x59, 0xd3, 0xfb, 0xb0, 0x9c, 0x39, 0xa5,
	0xfb, 0xde, 0xb0, 0xbe, 0xd5, 0xd9, 0x59, 0x1b, 0x15, 0x98, 0xd1, 0x55, 0x68, 0xaf, 0x71, 0xf6,
	0x75, 0xb3, 0x16, 0x54, 0x39, 0xff, 0x47, 0x1d, 0x5a, 0x6f, 0x0d, 0x33, 0x56, 0xd3, 0x6d, 0x68,
	0x6a, 0xc3, 0x0c, 0x3a, 0x7e, 0x77, 0xa7, 0x57, 0xed, 0x2d, 0xca, 0xee, 0x07, 0x83, 0x22, 0x92,
	0x67, 0x63, 0x66, 0x35, 0xf6, 0x97, 0xfe, 0x98, 0xdd, 0xcf, 0x6b, 0x41, 0x11, 0xa1, 0xeb, 0xd0,
	0x52, 0xa8, 0x6d, 0x6a, 0xfa, 0xf5, 0xa1, 0xb7, 0xd5, 0x0c, 0x4a, 0x45, 0x7b, 0xd0, 0x44, 0xa5,
	0xa4, 0xea, 0x37, 0xdc, 0x79, 0x0a, 0x91, 0xbb, 0x09, 0x46, 0x76, 0xda, 0x6f, 0x16, 0xae, 0x13,
	0xfe, 0x1b, 0x68, 0xba, 0xff, 0xa7, 0xff, 0x43, 0x47, 0x48, 0x81, 0x27, 0x5c, 0x1b, 0x14, 0x86,
	0xd4, 0x68, 0x07, 0x96, 0x95, 0x15, 0x82, 0x8b, 0x29, 0xf1, 0xe8, 0x2a, 0xb4, 0xb5, 0xd5, 0xc7,
	0x28, 0x12, 0x4c, 0xc8, 0x12, 0xed, 0x02, 0x18, 0x54, 0x19, 0x17, 0xcc, 0x60, 0x42, 0xea, 0x14,
	0xa0, 0x75, 0xc4, 0xd3, 0x14, 0x13, 0xd2, 0xf0, 0x3f, 0x2d, 0x41, 0xd3, 0xb5, 0x99, 0x13, 0x9e,
	0x3c, 0x7d, 0xb6, 0xfb, 0xfe, 0xe5, 0x3b, 0x52, 0x2b, 0xf8, 0xe1, 0xc4, 0x8a, 0xd8, 0x70, 0x29,
	0x88, 0x97, 0x1b, 0x56, 0x28, 0x64, 0xf1, 0x8c, 0x45, 0x29, 0x92, 0x3a, 0xed, 0x43, 0x2f, 0x66,
	0x69, 0x1a, 0x6a, 0xc3, 0xe2, 0xa3, 0x10, 0x4f, 0x66, 0xcc, 0xea, 0x1c, 0xdf, 0xa0, 0x9b, 0x70,
	0x23, 0xc3, 0x4c, 0xaa, 0xd3, 0x90, 0xc5, 0x31, 0x6a, 0x1d, 0x4a, 0x6b, 0x42, 0x39, 0x09, 0x23,
	0x69, 0x45, 0xa2, 0x49, 0x93, 0xde, 0x85, 0xdb, 0x5c, 0x24, 0x5c, 0x61, 0x6c, 0x42, 0xc7, 0xe0,
	0x22, 0xc1, 0x93, 0x5f, 0x62, 0x2d, 0x7a, 0x07, 0x86, 0xd7, 0x63, 0x9a, 0x4f, 0x05, 0x33, 0x56,
	0x61, 0x98, 0x71, 0x9d, 0x31, 0x13, 0xcf, 0xc8, 0x32, 0xdd, 0x80, 0xf5, 0x7c, 0x8e, 0xa6, 0xa8,
	0xc2, 0x84, 0xcf, 0x79, 0x82, 0x61, 0x74, 0x1a, 0x7e, 0x44, 0x25, 0xc9, 0x0a, 0xed, 0x01, 0xa9,
	0x6a, 0x72, 0x8e, 0x6a, 0x92, 0xca, 0x0f, 0xa4, 0x4d, 0xd7, 0x60, 0x95, 0x45, 0x3c, 0x9c, 0x73,
	0x99, 0x32, 0x77, 0xba, 0x5d, 0xff, 0x10, 0xba, 0x07, 0x42, 0x1b, 0x26, 0x62, 0x2c, 0xa7, 0x60,
	0x03, 0x56, 0x78, 0xe9, 0x94, 0x83, 0x76, 0xa5, 0xe9, 0x3d, 0x68, 0x69, 0x97, 0x72, 0xd7, 0xde,
	0xd9, 0xe9, 0x5e, 0xbf, 0xf6, 0x72, 0xb6, 0xca, 0x8c, 0xff, 0x1c, 0xda, 0x15, 0x5b, 0xd3, 0x87,
	0xd0, 0xae, 0x30, 0xd5, 0x70, 0xae, 0x57, 0xbb, 0xaf, 0x77, 0x50, 0x52, 0x16, 0x71, 0xff, 0x10,
	0xfe, 0x3b, 0x78, 0xbd, 0x2f, 0x85, 0x40, 0x77, 0x29, 0xf4, 0x26, 0xb4, 0xe3, 0x42, 0x61, 0xe2,
	0x7a, 0x5c, 0x09, 0x16, 0xc6, 0x3f, 0x36, 0xf9, 0x18, 0xc8, 0x82, 0x5c, 0x3e, 0x82, 0x05, 0xc1,
	0xfb, 0x3b, 0x61, 0xef, 0xc5, 0xd9, 0xf7, 0x41, 0xed, 0xec, 0x62, 0xe0, 0x9d, 0x5f, 0x0c, 0xbc,
	0x6f, 0x17, 0x03, 0xef, 0xf3, 0xe5, 0xa0, 0x76, 0x7e, 0x39, 0xa8, 0x7d, 0xb9, 0x1c, 0xd4, 0x0e,
	0xb7, 0xa7, 0xdc, 0xcc, 0x6c, 0x34, 0x8a, 0x65, 0x36, 0x36, 0x9a, 0xcd, 0x65, 0xca, 0xc6, 0x53,
	0x66, 0x70, 0xfc, 0xfb, 0x37, 0x20, 0x6a, 0xb9, 0x57, 0xff, 0xc1, 0xcf, 0x00, 0x00, 0x00, 0xff,
	0xff, 0x4b, 0xd0, 0x09, 0x8b, 0x20, 0x04, 0x00, 0x00,
}
