// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package proto

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// TxnPoolOperatorClient is the client API for TxnPoolOperator service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type TxnPoolOperatorClient interface {
	// Status returns the current status of the pool
	Status(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*TxnPoolStatusResp, error)
	// AddTxn adds a local transaction to the pool
	AddTxn(ctx context.Context, in *AddTxnReq, opts ...grpc.CallOption) (*empty.Empty, error)
	// Subscribe subscribes for new events in the txpool
	Subscribe(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (TxnPoolOperator_SubscribeClient, error)
}

type txnPoolOperatorClient struct {
	cc grpc.ClientConnInterface
}

func NewTxnPoolOperatorClient(cc grpc.ClientConnInterface) TxnPoolOperatorClient {
	return &txnPoolOperatorClient{cc}
}

func (c *txnPoolOperatorClient) Status(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*TxnPoolStatusResp, error) {
	out := new(TxnPoolStatusResp)
	err := c.cc.Invoke(ctx, "/v1.TxnPoolOperator/Status", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *txnPoolOperatorClient) AddTxn(ctx context.Context, in *AddTxnReq, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/v1.TxnPoolOperator/AddTxn", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *txnPoolOperatorClient) Subscribe(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (TxnPoolOperator_SubscribeClient, error) {
	stream, err := c.cc.NewStream(ctx, &TxnPoolOperator_ServiceDesc.Streams[0], "/v1.TxnPoolOperator/Subscribe", opts...)
	if err != nil {
		return nil, err
	}
	x := &txnPoolOperatorSubscribeClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type TxnPoolOperator_SubscribeClient interface {
	Recv() (*TxPoolEvent, error)
	grpc.ClientStream
}

type txnPoolOperatorSubscribeClient struct {
	grpc.ClientStream
}

func (x *txnPoolOperatorSubscribeClient) Recv() (*TxPoolEvent, error) {
	m := new(TxPoolEvent)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// TxnPoolOperatorServer is the server API for TxnPoolOperator service.
// All implementations must embed UnimplementedTxnPoolOperatorServer
// for forward compatibility
type TxnPoolOperatorServer interface {
	// Status returns the current status of the pool
	Status(context.Context, *empty.Empty) (*TxnPoolStatusResp, error)
	// AddTxn adds a local transaction to the pool
	AddTxn(context.Context, *AddTxnReq) (*empty.Empty, error)
	// Subscribe subscribes for new events in the txpool
	Subscribe(*empty.Empty, TxnPoolOperator_SubscribeServer) error
	mustEmbedUnimplementedTxnPoolOperatorServer()
}

// UnimplementedTxnPoolOperatorServer must be embedded to have forward compatible implementations.
type UnimplementedTxnPoolOperatorServer struct {
}

func (UnimplementedTxnPoolOperatorServer) Status(context.Context, *empty.Empty) (*TxnPoolStatusResp, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedTxnPoolOperatorServer) AddTxn(context.Context, *AddTxnReq) (*empty.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddTxn not implemented")
}
func (UnimplementedTxnPoolOperatorServer) Subscribe(*empty.Empty, TxnPoolOperator_SubscribeServer) error {
	return status.Errorf(codes.Unimplemented, "method Subscribe not implemented")
}
func (UnimplementedTxnPoolOperatorServer) mustEmbedUnimplementedTxnPoolOperatorServer() {}

// UnsafeTxnPoolOperatorServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to TxnPoolOperatorServer will
// result in compilation errors.
type UnsafeTxnPoolOperatorServer interface {
	mustEmbedUnimplementedTxnPoolOperatorServer()
}

func RegisterTxnPoolOperatorServer(s grpc.ServiceRegistrar, srv TxnPoolOperatorServer) {
	s.RegisterService(&TxnPoolOperator_ServiceDesc, srv)
}

func _TxnPoolOperator_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(empty.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TxnPoolOperatorServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.TxnPoolOperator/Status",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TxnPoolOperatorServer).Status(ctx, req.(*empty.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _TxnPoolOperator_AddTxn_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddTxnReq)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TxnPoolOperatorServer).AddTxn(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.TxnPoolOperator/AddTxn",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TxnPoolOperatorServer).AddTxn(ctx, req.(*AddTxnReq))
	}
	return interceptor(ctx, in, info, handler)
}

func _TxnPoolOperator_Subscribe_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(empty.Empty)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(TxnPoolOperatorServer).Subscribe(m, &txnPoolOperatorSubscribeServer{stream})
}

type TxnPoolOperator_SubscribeServer interface {
	Send(*TxPoolEvent) error
	grpc.ServerStream
}

type txnPoolOperatorSubscribeServer struct {
	grpc.ServerStream
}

func (x *txnPoolOperatorSubscribeServer) Send(m *TxPoolEvent) error {
	return x.ServerStream.SendMsg(m)
}

// TxnPoolOperator_ServiceDesc is the grpc.ServiceDesc for TxnPoolOperator service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var TxnPoolOperator_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "v1.TxnPoolOperator",
	HandlerType: (*TxnPoolOperatorServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Status",
			Handler:    _TxnPoolOperator_Status_Handler,
		},
		{
			MethodName: "AddTxn",
			Handler:    _TxnPoolOperator_AddTxn_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Subscribe",
			Handler:       _TxnPoolOperator_Subscribe_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "txpool/proto/operator.proto",
}