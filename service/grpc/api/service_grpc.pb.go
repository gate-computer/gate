// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package api

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	wrappers "github.com/golang/protobuf/ptypes/wrappers"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion7

// RootClient is the client API for Root service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type RootClient interface {
	Init(ctx context.Context, in *InitRequest, opts ...grpc.CallOption) (*InitResponse, error)
}

type rootClient struct {
	cc grpc.ClientConnInterface
}

func NewRootClient(cc grpc.ClientConnInterface) RootClient {
	return &rootClient{cc}
}

func (c *rootClient) Init(ctx context.Context, in *InitRequest, opts ...grpc.CallOption) (*InitResponse, error) {
	out := new(InitResponse)
	err := c.cc.Invoke(ctx, "/gate.service.grpc.api.Root/Init", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RootServer is the server API for Root service.
// All implementations must embed UnimplementedRootServer
// for forward compatibility
type RootServer interface {
	Init(context.Context, *InitRequest) (*InitResponse, error)
	mustEmbedUnimplementedRootServer()
}

// UnimplementedRootServer must be embedded to have forward compatible implementations.
type UnimplementedRootServer struct {
}

func (UnimplementedRootServer) Init(context.Context, *InitRequest) (*InitResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Init not implemented")
}
func (UnimplementedRootServer) mustEmbedUnimplementedRootServer() {}

// UnsafeRootServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RootServer will
// result in compilation errors.
type UnsafeRootServer interface {
	mustEmbedUnimplementedRootServer()
}

func RegisterRootServer(s *grpc.Server, srv RootServer) {
	s.RegisterService(&_Root_serviceDesc, srv)
}

func _Root_Init_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InitRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RootServer).Init(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gate.service.grpc.api.Root/Init",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RootServer).Init(ctx, req.(*InitRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Root_serviceDesc = grpc.ServiceDesc{
	ServiceName: "gate.service.grpc.api.Root",
	HandlerType: (*RootServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Init",
			Handler:    _Root_Init_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "service/grpc/api/service.proto",
}

// ServiceClient is the client API for Service service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ServiceClient interface {
	CreateInstance(ctx context.Context, in *CreateInstanceRequest, opts ...grpc.CallOption) (*CreateInstanceResponse, error)
	RestoreInstance(ctx context.Context, in *RestoreInstanceRequest, opts ...grpc.CallOption) (*RestoreInstanceResponse, error)
}

type serviceClient struct {
	cc grpc.ClientConnInterface
}

func NewServiceClient(cc grpc.ClientConnInterface) ServiceClient {
	return &serviceClient{cc}
}

func (c *serviceClient) CreateInstance(ctx context.Context, in *CreateInstanceRequest, opts ...grpc.CallOption) (*CreateInstanceResponse, error) {
	out := new(CreateInstanceResponse)
	err := c.cc.Invoke(ctx, "/gate.service.grpc.api.Service/CreateInstance", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *serviceClient) RestoreInstance(ctx context.Context, in *RestoreInstanceRequest, opts ...grpc.CallOption) (*RestoreInstanceResponse, error) {
	out := new(RestoreInstanceResponse)
	err := c.cc.Invoke(ctx, "/gate.service.grpc.api.Service/RestoreInstance", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ServiceServer is the server API for Service service.
// All implementations must embed UnimplementedServiceServer
// for forward compatibility
type ServiceServer interface {
	CreateInstance(context.Context, *CreateInstanceRequest) (*CreateInstanceResponse, error)
	RestoreInstance(context.Context, *RestoreInstanceRequest) (*RestoreInstanceResponse, error)
	mustEmbedUnimplementedServiceServer()
}

// UnimplementedServiceServer must be embedded to have forward compatible implementations.
type UnimplementedServiceServer struct {
}

func (UnimplementedServiceServer) CreateInstance(context.Context, *CreateInstanceRequest) (*CreateInstanceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateInstance not implemented")
}
func (UnimplementedServiceServer) RestoreInstance(context.Context, *RestoreInstanceRequest) (*RestoreInstanceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RestoreInstance not implemented")
}
func (UnimplementedServiceServer) mustEmbedUnimplementedServiceServer() {}

// UnsafeServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ServiceServer will
// result in compilation errors.
type UnsafeServiceServer interface {
	mustEmbedUnimplementedServiceServer()
}

func RegisterServiceServer(s *grpc.Server, srv ServiceServer) {
	s.RegisterService(&_Service_serviceDesc, srv)
}

func _Service_CreateInstance_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateInstanceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).CreateInstance(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gate.service.grpc.api.Service/CreateInstance",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).CreateInstance(ctx, req.(*CreateInstanceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Service_RestoreInstance_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RestoreInstanceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ServiceServer).RestoreInstance(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gate.service.grpc.api.Service/RestoreInstance",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ServiceServer).RestoreInstance(ctx, req.(*RestoreInstanceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Service_serviceDesc = grpc.ServiceDesc{
	ServiceName: "gate.service.grpc.api.Service",
	HandlerType: (*ServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreateInstance",
			Handler:    _Service_CreateInstance_Handler,
		},
		{
			MethodName: "RestoreInstance",
			Handler:    _Service_RestoreInstance_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "service/grpc/api/service.proto",
}

// InstanceClient is the client API for Instance service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type InstanceClient interface {
	Receive(ctx context.Context, in *ReceiveRequest, opts ...grpc.CallOption) (Instance_ReceiveClient, error)
	Handle(ctx context.Context, in *HandleRequest, opts ...grpc.CallOption) (*empty.Empty, error)
	Shutdown(ctx context.Context, in *ShutdownRequest, opts ...grpc.CallOption) (*empty.Empty, error)
	Suspend(ctx context.Context, in *SuspendRequest, opts ...grpc.CallOption) (*empty.Empty, error)
	Snapshot(ctx context.Context, in *SnapshotRequest, opts ...grpc.CallOption) (*wrappers.BytesValue, error)
}

type instanceClient struct {
	cc grpc.ClientConnInterface
}

func NewInstanceClient(cc grpc.ClientConnInterface) InstanceClient {
	return &instanceClient{cc}
}

func (c *instanceClient) Receive(ctx context.Context, in *ReceiveRequest, opts ...grpc.CallOption) (Instance_ReceiveClient, error) {
	stream, err := c.cc.NewStream(ctx, &_Instance_serviceDesc.Streams[0], "/gate.service.grpc.api.Instance/Receive", opts...)
	if err != nil {
		return nil, err
	}
	x := &instanceReceiveClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Instance_ReceiveClient interface {
	Recv() (*wrappers.BytesValue, error)
	grpc.ClientStream
}

type instanceReceiveClient struct {
	grpc.ClientStream
}

func (x *instanceReceiveClient) Recv() (*wrappers.BytesValue, error) {
	m := new(wrappers.BytesValue)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *instanceClient) Handle(ctx context.Context, in *HandleRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/gate.service.grpc.api.Instance/Handle", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *instanceClient) Shutdown(ctx context.Context, in *ShutdownRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/gate.service.grpc.api.Instance/Shutdown", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *instanceClient) Suspend(ctx context.Context, in *SuspendRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/gate.service.grpc.api.Instance/Suspend", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *instanceClient) Snapshot(ctx context.Context, in *SnapshotRequest, opts ...grpc.CallOption) (*wrappers.BytesValue, error) {
	out := new(wrappers.BytesValue)
	err := c.cc.Invoke(ctx, "/gate.service.grpc.api.Instance/Snapshot", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// InstanceServer is the server API for Instance service.
// All implementations must embed UnimplementedInstanceServer
// for forward compatibility
type InstanceServer interface {
	Receive(*ReceiveRequest, Instance_ReceiveServer) error
	Handle(context.Context, *HandleRequest) (*empty.Empty, error)
	Shutdown(context.Context, *ShutdownRequest) (*empty.Empty, error)
	Suspend(context.Context, *SuspendRequest) (*empty.Empty, error)
	Snapshot(context.Context, *SnapshotRequest) (*wrappers.BytesValue, error)
	mustEmbedUnimplementedInstanceServer()
}

// UnimplementedInstanceServer must be embedded to have forward compatible implementations.
type UnimplementedInstanceServer struct {
}

func (UnimplementedInstanceServer) Receive(*ReceiveRequest, Instance_ReceiveServer) error {
	return status.Errorf(codes.Unimplemented, "method Receive not implemented")
}
func (UnimplementedInstanceServer) Handle(context.Context, *HandleRequest) (*empty.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Handle not implemented")
}
func (UnimplementedInstanceServer) Shutdown(context.Context, *ShutdownRequest) (*empty.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Shutdown not implemented")
}
func (UnimplementedInstanceServer) Suspend(context.Context, *SuspendRequest) (*empty.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Suspend not implemented")
}
func (UnimplementedInstanceServer) Snapshot(context.Context, *SnapshotRequest) (*wrappers.BytesValue, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Snapshot not implemented")
}
func (UnimplementedInstanceServer) mustEmbedUnimplementedInstanceServer() {}

// UnsafeInstanceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to InstanceServer will
// result in compilation errors.
type UnsafeInstanceServer interface {
	mustEmbedUnimplementedInstanceServer()
}

func RegisterInstanceServer(s *grpc.Server, srv InstanceServer) {
	s.RegisterService(&_Instance_serviceDesc, srv)
}

func _Instance_Receive_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(ReceiveRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(InstanceServer).Receive(m, &instanceReceiveServer{stream})
}

type Instance_ReceiveServer interface {
	Send(*wrappers.BytesValue) error
	grpc.ServerStream
}

type instanceReceiveServer struct {
	grpc.ServerStream
}

func (x *instanceReceiveServer) Send(m *wrappers.BytesValue) error {
	return x.ServerStream.SendMsg(m)
}

func _Instance_Handle_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HandleRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InstanceServer).Handle(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gate.service.grpc.api.Instance/Handle",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InstanceServer).Handle(ctx, req.(*HandleRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Instance_Shutdown_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ShutdownRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InstanceServer).Shutdown(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gate.service.grpc.api.Instance/Shutdown",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InstanceServer).Shutdown(ctx, req.(*ShutdownRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Instance_Suspend_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SuspendRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InstanceServer).Suspend(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gate.service.grpc.api.Instance/Suspend",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InstanceServer).Suspend(ctx, req.(*SuspendRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Instance_Snapshot_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SnapshotRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InstanceServer).Snapshot(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gate.service.grpc.api.Instance/Snapshot",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InstanceServer).Snapshot(ctx, req.(*SnapshotRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Instance_serviceDesc = grpc.ServiceDesc{
	ServiceName: "gate.service.grpc.api.Instance",
	HandlerType: (*InstanceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Handle",
			Handler:    _Instance_Handle_Handler,
		},
		{
			MethodName: "Shutdown",
			Handler:    _Instance_Shutdown_Handler,
		},
		{
			MethodName: "Suspend",
			Handler:    _Instance_Suspend_Handler,
		},
		{
			MethodName: "Snapshot",
			Handler:    _Instance_Snapshot_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Receive",
			Handler:       _Instance_Receive_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "service/grpc/api/service.proto",
}
