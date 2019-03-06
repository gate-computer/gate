// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: server/detail/detail.proto

package detail // import "github.com/tsavola/gate/server/detail"

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

type Iface int32

const (
	Iface_DEFAULT Iface = 0
)

var Iface_name = map[int32]string{
	0: "DEFAULT",
}
var Iface_value = map[string]int32{
	"DEFAULT": 0,
}

func (x Iface) String() string {
	return proto.EnumName(Iface_name, int32(x))
}
func (Iface) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_detail_db0a3ca9618f460d, []int{0}
}

type Op int32

const (
	Op_Unknown          Op = 0
	Op_ModuleList       Op = 1
	Op_ModuleUpload     Op = 2
	Op_ModuleDownload   Op = 3
	Op_ModuleUnref      Op = 4
	Op_CallRef          Op = 5
	Op_CallUpload       Op = 6
	Op_CallSource       Op = 7
	Op_LaunchRef        Op = 8
	Op_LaunchUpload     Op = 9
	Op_LaunchSource     Op = 10
	Op_InstanceList     Op = 11
	Op_InstanceConnect  Op = 12
	Op_InstanceStatus   Op = 13
	Op_InstanceWait     Op = 14
	Op_InstanceSuspend  Op = 15
	Op_InstanceResume   Op = 16
	Op_InstanceSnapshot Op = 17
)

var Op_name = map[int32]string{
	0:  "Unknown",
	1:  "ModuleList",
	2:  "ModuleUpload",
	3:  "ModuleDownload",
	4:  "ModuleUnref",
	5:  "CallRef",
	6:  "CallUpload",
	7:  "CallSource",
	8:  "LaunchRef",
	9:  "LaunchUpload",
	10: "LaunchSource",
	11: "InstanceList",
	12: "InstanceConnect",
	13: "InstanceStatus",
	14: "InstanceWait",
	15: "InstanceSuspend",
	16: "InstanceResume",
	17: "InstanceSnapshot",
}
var Op_value = map[string]int32{
	"Unknown":          0,
	"ModuleList":       1,
	"ModuleUpload":     2,
	"ModuleDownload":   3,
	"ModuleUnref":      4,
	"CallRef":          5,
	"CallUpload":       6,
	"CallSource":       7,
	"LaunchRef":        8,
	"LaunchUpload":     9,
	"LaunchSource":     10,
	"InstanceList":     11,
	"InstanceConnect":  12,
	"InstanceStatus":   13,
	"InstanceWait":     14,
	"InstanceSuspend":  15,
	"InstanceResume":   16,
	"InstanceSnapshot": 17,
}

func (x Op) String() string {
	return proto.EnumName(Op_name, int32(x))
}
func (Op) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_detail_db0a3ca9618f460d, []int{1}
}

type Context struct {
	Iface                Iface    `protobuf:"varint,1,opt,name=iface,proto3,enum=detail.Iface" json:"iface,omitempty"`
	Req                  uint64   `protobuf:"varint,2,opt,name=req,proto3" json:"req,omitempty"`
	Addr                 string   `protobuf:"bytes,3,opt,name=addr,proto3" json:"addr,omitempty"`
	Op                   Op       `protobuf:"varint,4,opt,name=op,proto3,enum=detail.Op" json:"op,omitempty"`
	Principal            string   `protobuf:"bytes,5,opt,name=principal,proto3" json:"principal,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Context) Reset()         { *m = Context{} }
func (m *Context) String() string { return proto.CompactTextString(m) }
func (*Context) ProtoMessage()    {}
func (*Context) Descriptor() ([]byte, []int) {
	return fileDescriptor_detail_db0a3ca9618f460d, []int{0}
}
func (m *Context) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Context) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Context.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (dst *Context) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Context.Merge(dst, src)
}
func (m *Context) XXX_Size() int {
	return m.Size()
}
func (m *Context) XXX_DiscardUnknown() {
	xxx_messageInfo_Context.DiscardUnknown(m)
}

var xxx_messageInfo_Context proto.InternalMessageInfo

func init() {
	proto.RegisterType((*Context)(nil), "detail.Context")
	proto.RegisterEnum("detail.Iface", Iface_name, Iface_value)
	proto.RegisterEnum("detail.Op", Op_name, Op_value)
}
func (m *Context) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Context) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.Iface != 0 {
		dAtA[i] = 0x8
		i++
		i = encodeVarintDetail(dAtA, i, uint64(m.Iface))
	}
	if m.Req != 0 {
		dAtA[i] = 0x10
		i++
		i = encodeVarintDetail(dAtA, i, uint64(m.Req))
	}
	if len(m.Addr) > 0 {
		dAtA[i] = 0x1a
		i++
		i = encodeVarintDetail(dAtA, i, uint64(len(m.Addr)))
		i += copy(dAtA[i:], m.Addr)
	}
	if m.Op != 0 {
		dAtA[i] = 0x20
		i++
		i = encodeVarintDetail(dAtA, i, uint64(m.Op))
	}
	if len(m.Principal) > 0 {
		dAtA[i] = 0x2a
		i++
		i = encodeVarintDetail(dAtA, i, uint64(len(m.Principal)))
		i += copy(dAtA[i:], m.Principal)
	}
	return i, nil
}

func encodeVarintDetail(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *Context) Size() (n int) {
	var l int
	_ = l
	if m.Iface != 0 {
		n += 1 + sovDetail(uint64(m.Iface))
	}
	if m.Req != 0 {
		n += 1 + sovDetail(uint64(m.Req))
	}
	l = len(m.Addr)
	if l > 0 {
		n += 1 + l + sovDetail(uint64(l))
	}
	if m.Op != 0 {
		n += 1 + sovDetail(uint64(m.Op))
	}
	l = len(m.Principal)
	if l > 0 {
		n += 1 + l + sovDetail(uint64(l))
	}
	return n
}

func sovDetail(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozDetail(x uint64) (n int) {
	return sovDetail(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Context) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowDetail
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
			return fmt.Errorf("proto: Context: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Context: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Iface", wireType)
			}
			m.Iface = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowDetail
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Iface |= (Iface(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Req", wireType)
			}
			m.Req = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowDetail
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Req |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Addr", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowDetail
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
				return ErrInvalidLengthDetail
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Addr = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Op", wireType)
			}
			m.Op = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowDetail
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Op |= (Op(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Principal", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowDetail
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
				return ErrInvalidLengthDetail
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Principal = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipDetail(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthDetail
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
func skipDetail(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowDetail
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
					return 0, ErrIntOverflowDetail
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
					return 0, ErrIntOverflowDetail
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
				return 0, ErrInvalidLengthDetail
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowDetail
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
				next, err := skipDetail(dAtA[start:])
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
	ErrInvalidLengthDetail = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowDetail   = fmt.Errorf("proto: integer overflow")
)

func init() { proto.RegisterFile("server/detail/detail.proto", fileDescriptor_detail_db0a3ca9618f460d) }

var fileDescriptor_detail_db0a3ca9618f460d = []byte{
	// 413 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x54, 0x92, 0xcf, 0x6e, 0x13, 0x31,
	0x10, 0xc6, 0xe3, 0xcd, 0x3f, 0x32, 0x69, 0x92, 0x61, 0xe8, 0x61, 0x55, 0xa1, 0x55, 0x04, 0x42,
	0x8a, 0x7a, 0x48, 0x24, 0x38, 0x72, 0x82, 0x14, 0xa4, 0x4a, 0x41, 0x95, 0x36, 0x44, 0x48, 0xdc,
	0xdc, 0x5d, 0xa7, 0x59, 0xb1, 0xb5, 0x8d, 0xed, 0x6d, 0x79, 0x0a, 0xc4, 0x43, 0x71, 0xe8, 0x91,
	0x47, 0x80, 0xf0, 0x22, 0xc8, 0xfb, 0x87, 0xa6, 0x27, 0xcf, 0xf7, 0xf3, 0x7c, 0xdf, 0xcc, 0x61,
	0xe0, 0xc4, 0x0a, 0x73, 0x23, 0xcc, 0x22, 0x15, 0x8e, 0x67, 0x79, 0xfd, 0xcc, 0xb5, 0x51, 0x4e,
	0x51, 0xaf, 0x52, 0xcf, 0xbe, 0x33, 0xe8, 0x2f, 0x95, 0x74, 0xe2, 0x9b, 0xa3, 0xe7, 0xd0, 0xcd,
	0xb6, 0x3c, 0x11, 0x21, 0x9b, 0xb2, 0xd9, 0xf8, 0xe5, 0x68, 0x5e, 0x3b, 0xce, 0x3d, 0x8c, 0xab,
	0x3f, 0x42, 0x68, 0x1b, 0xf1, 0x35, 0x0c, 0xa6, 0x6c, 0xd6, 0x89, 0x7d, 0x49, 0x04, 0x1d, 0x9e,
	0xa6, 0x26, 0x6c, 0x4f, 0xd9, 0x6c, 0x10, 0x97, 0x35, 0x9d, 0x40, 0xa0, 0x74, 0xd8, 0x29, 0x73,
	0xa0, 0xc9, 0xb9, 0xd0, 0x71, 0xa0, 0x34, 0x3d, 0x85, 0x81, 0x36, 0x99, 0x4c, 0x32, 0xcd, 0xf3,
	0xb0, 0x5b, 0x9a, 0xee, 0xc1, 0xe9, 0x31, 0x74, 0xcb, 0x79, 0x34, 0x84, 0xfe, 0xd9, 0xbb, 0xf7,
	0x6f, 0x36, 0xab, 0x8f, 0xd8, 0x3a, 0xfd, 0x19, 0x40, 0x70, 0xa1, 0x3d, 0xdb, 0xc8, 0x2f, 0x52,
	0xdd, 0x4a, 0x6c, 0xd1, 0x18, 0xe0, 0x83, 0x4a, 0x8b, 0x5c, 0xac, 0x32, 0xeb, 0x90, 0x11, 0xc2,
	0x51, 0xa5, 0x37, 0x3a, 0x57, 0x3c, 0xc5, 0x80, 0x08, 0xc6, 0x15, 0x39, 0x53, 0xb7, 0xb2, 0x64,
	0x6d, 0x9a, 0xc0, 0xb0, 0xee, 0x92, 0x46, 0x6c, 0xb1, 0xe3, 0x33, 0x97, 0x3c, 0xcf, 0x63, 0xb1,
	0xc5, 0xae, 0xcf, 0xf4, 0xa2, 0x4e, 0xe8, 0x35, 0x7a, 0xad, 0x0a, 0x93, 0x08, 0xec, 0xd3, 0x08,
	0x06, 0x2b, 0x5e, 0xc8, 0x64, 0xe7, 0xdb, 0x1f, 0xf9, 0x91, 0x95, 0xac, 0x0d, 0x83, 0x7b, 0x52,
	0x5b, 0xc0, 0x93, 0x73, 0x69, 0x1d, 0x97, 0x49, 0xb5, 0xe8, 0x90, 0x9e, 0xc0, 0xa4, 0x21, 0x4b,
	0x25, 0xa5, 0x48, 0x1c, 0x1e, 0xf9, 0x5d, 0x1b, 0xb8, 0x76, 0xdc, 0x15, 0x16, 0x47, 0x87, 0xd6,
	0x4f, 0x3c, 0x73, 0x38, 0x3e, 0xb4, 0xae, 0x0b, 0xab, 0x85, 0x4c, 0x71, 0x72, 0x68, 0x8d, 0x85,
	0x2d, 0xae, 0x05, 0x22, 0x1d, 0x03, 0xfe, 0x6f, 0x94, 0x5c, 0xdb, 0x9d, 0x72, 0xf8, 0xf8, 0xed,
	0xeb, 0xbb, 0x3f, 0x51, 0xeb, 0x6e, 0x1f, 0xb1, 0x5f, 0xfb, 0x88, 0xfd, 0xde, 0x47, 0xec, 0xc7,
	0xdf, 0xa8, 0xf5, 0xf9, 0xc5, 0x55, 0xe6, 0x76, 0xc5, 0xe5, 0x3c, 0x51, 0xd7, 0x0b, 0x67, 0xf9,
	0x8d, 0xca, 0xf9, 0xe2, 0x8a, 0x3b, 0xb1, 0x78, 0x70, 0x3f, 0x97, 0xbd, 0xf2, 0x72, 0x5e, 0xfd,
	0x0b, 0x00, 0x00, 0xff, 0xff, 0x79, 0x7b, 0x4b, 0xf6, 0x57, 0x02, 0x00, 0x00,
}
