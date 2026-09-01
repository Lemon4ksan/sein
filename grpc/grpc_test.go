// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/grpc"
	"github.com/lemon4ksan/sein/grpc/codes"
	"github.com/lemon4ksan/sein/grpc/metadata"
	"github.com/lemon4ksan/sein/grpc/status"
)

type HelloRequest struct {
	Name string `json:"name"`
}

type HelloReply struct {
	Message string `json:"message"`
}

// Mock service definition matching protoc-gen-go-grpc layout
var greeterServiceDesc = grpc.ServiceDesc{
	ServiceName: "helloworld.Greeter",
	HandlerType: (*any)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "SayHello",
			Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
				in := new(HelloRequest)
				if err := dec(in); err != nil {
					return nil, err
				}
				if interceptor == nil {
					return srv.(GreeterServer).SayHello(ctx, in)
				}
				info := &grpc.UnaryServerInfo{
					Server:     srv,
					FullMethod: "/helloworld.Greeter/SayHello",
				}
				handler := func(c context.Context, req any) (any, error) {
					return srv.(GreeterServer).SayHello(c, req.(*HelloRequest))
				}
				return interceptor(ctx, in, info, handler)
			},
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName: "StreamHello",
			Handler: func(srv any, stream grpc.ServerStream) error {
				return srv.(GreeterServer).StreamHello(stream)
			},
			ServerStreams: true,
			ClientStreams: true,
		},
	},
}

type GreeterServer interface {
	SayHello(context.Context, *HelloRequest) (*HelloReply, error)
	StreamHello(grpc.ServerStream) error
}

type greeterImpl struct {
	sayHelloFn    func(context.Context, *HelloRequest) (*HelloReply, error)
	streamHelloFn func(grpc.ServerStream) error
}

func (g *greeterImpl) SayHello(ctx context.Context, req *HelloRequest) (*HelloReply, error) {
	if g.sayHelloFn != nil {
		return g.sayHelloFn(ctx, req)
	}
	return &HelloReply{Message: "Hello " + req.Name}, nil
}

func (g *greeterImpl) StreamHello(stream grpc.ServerStream) error {
	if g.streamHelloFn != nil {
		return g.streamHelloFn(stream)
	}
	return nil
}

func encodeGRPCFrame(t *testing.T, msg any) []byte {
	t.Helper()
	var buf bytes.Buffer
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	err = grpc.WriteMsg(&buf, data, false)
	require.NoError(t, err)
	return buf.Bytes()
}

func decodeGRPCFrame(t *testing.T, r io.Reader, target any) {
	t.Helper()
	data, _, err := grpc.ReadMsg(r, grpc.DefaultMaxReceiveMsgSize)
	require.NoError(t, err)
	err = json.Unmarshal(data, target)
	require.NoError(t, err)
}

func TestGRPC_UnarySuccess(t *testing.T) {
	srv := grpc.NewServer()
	srv.RegisterService(&greeterServiceDesc, &greeterImpl{})

	body := encodeGRPCFrame(t, &HelloRequest{Name: "G-MAN"})
	req := httptest.NewRequest(http.MethodPost, "/helloworld.Greeter/SayHello", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("X-Custom-Header", "Valhalla")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "application/grpc", res.Header.Get("Content-Type"))
	assert.Equal(t, "0", res.Trailer.Get("Grpc-Status"))

	var reply HelloReply
	decodeGRPCFrame(t, res.Body, &reply)
	assert.Equal(t, "Hello G-MAN", reply.Message)
}

func TestGRPC_UnaryError(t *testing.T) {
	srv := grpc.NewServer()
	srv.RegisterService(&greeterServiceDesc, &greeterImpl{
		sayHelloFn: func(ctx context.Context, req *HelloRequest) (*HelloReply, error) {
			return nil, status.Error(codes.NotFound, "entity not found")
		},
	})

	body := encodeGRPCFrame(t, &HelloRequest{Name: "Missing"})
	req := httptest.NewRequest(http.MethodPost, "/helloworld.Greeter/SayHello", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/grpc")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, strconv.Itoa(int(codes.NotFound)), res.Header.Get("Grpc-Status"))
	assert.Equal(t, "entity+not+found", res.Header.Get("Grpc-Message"))
}

func TestGRPC_UnaryInterceptorAndMetadata(t *testing.T) {
	var intercepted bool
	interceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		intercepted = true
		assert.Equal(t, "/helloworld.Greeter/SayHello", info.FullMethod)

		md, ok := metadata.FromIncomingContext(ctx)
		assert.True(t, ok)
		assert.Equal(t, []string{"SecretToken"}, md.Get("Authorization"))

		err := metadata.SetHeader(ctx, metadata.Pairs("X-Server-Time", "12345"))
		assert.NoError(t, err)

		return handler(ctx, req)
	}

	srv := grpc.NewServer(grpc.UnaryInterceptor(interceptor))
	srv.RegisterService(&greeterServiceDesc, &greeterImpl{})

	body := encodeGRPCFrame(t, &HelloRequest{Name: "Interception"})
	req := httptest.NewRequest(http.MethodPost, "/helloworld.Greeter/SayHello", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Authorization", "SecretToken")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	assert.True(t, intercepted)
	assert.Equal(t, "12345", res.Header.Get("X-Server-Time"))
	assert.Equal(t, "0", res.Trailer.Get("Grpc-Status"))
}

func TestGRPC_StreamingRPC(t *testing.T) {
	srv := grpc.NewServer()
	srv.RegisterService(&greeterServiceDesc, &greeterImpl{
		streamHelloFn: func(ss grpc.ServerStream) error {
			for i := 0; i < 3; i++ {
				var req HelloRequest
				if err := ss.RecvMsg(&req); err != nil {
					return err
				}
				if err := ss.SendMsg(&HelloReply{Message: "Echo " + req.Name}); err != nil {
					return err
				}
			}
			return nil
		},
	})

	pipeReader, pipeWriter := io.Pipe()
	go func() {
		for i := 0; i < 3; i++ {
			body := encodeGRPCFrame(t, &HelloRequest{Name: "Msg" + strconv.Itoa(i)})
			_, _ = pipeWriter.Write(body)
		}
		_ = pipeWriter.Close()
	}()

	req := httptest.NewRequest(http.MethodPost, "/helloworld.Greeter/StreamHello", pipeReader)
	req.Header.Set("Content-Type", "application/grpc")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)

	for i := 0; i < 3; i++ {
		var reply HelloReply
		decodeGRPCFrame(t, res.Body, &reply)
		assert.Equal(t, "Echo Msg"+strconv.Itoa(i), reply.Message)
	}

	assert.Equal(t, "0", res.Trailer.Get("Grpc-Status"))
}

func TestGRPC_MountOnSein(t *testing.T) {
	app := sein.New()

	// 1. Regular REST route
	app.Get("/api/ping", func(ctx context.Context) (string, error) {
		return "pong", nil
	})

	// 2. gRPC Server mounted on the exact same sein instance
	grpcSrv := grpc.NewServer()
	grpcSrv.RegisterService(&greeterServiceDesc, &greeterImpl{})
	grpcSrv.Mount(app)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Test REST endpoint
	respREST, err := client.Get("http://" + addr + "/api/ping")
	require.NoError(t, err)
	bodyREST, _ := io.ReadAll(respREST.Body)
	_ = respREST.Body.Close()
	assert.Equal(t, http.StatusOK, respREST.StatusCode)
	assert.Equal(t, "pong", string(bodyREST))

	// 2. Test gRPC endpoint on the same server!
	grpcBody := encodeGRPCFrame(t, &HelloRequest{Name: "Unified"})
	reqGRPC, err := http.NewRequest(http.MethodPost, "http://"+addr+"/helloworld.Greeter/SayHello", bytes.NewReader(grpcBody))
	require.NoError(t, err)
	reqGRPC.Header.Set("Content-Type", "application/grpc")

	respGRPC, err := client.Do(reqGRPC)
	require.NoError(t, err)
	defer respGRPC.Body.Close()

	assert.Equal(t, http.StatusOK, respGRPC.StatusCode)
	assert.Equal(t, "application/grpc", respGRPC.Header.Get("Content-Type"))

	var reply HelloReply
	decodeGRPCFrame(t, respGRPC.Body, &reply)
	assert.Equal(t, "Hello Unified", reply.Message)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestGRPC_Chains_And_Options(t *testing.T) {
	var unaryCalls []string
	interceptor1 := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		unaryCalls = append(unaryCalls, "int1")
		return handler(ctx, req)
	}
	interceptor2 := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		unaryCalls = append(unaryCalls, "int2")
		return handler(ctx, req)
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptor1, interceptor2),
		grpc.MaxRecvMsgSize(1024*1024),
		grpc.MaxSendMsgSize(1024*1024),
		grpc.CustomCodec(grpc.ProtoCodec{}),
	)
	srv.RegisterService(&greeterServiceDesc, &greeterImpl{})

	// 1. Valid request through chained interceptor
	body := encodeGRPCFrame(t, &HelloRequest{Name: "Chain"})
	req := httptest.NewRequest(http.MethodPost, "/helloworld.Greeter/SayHello", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/grpc")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, []string{"int1", "int2"}, unaryCalls)

	// 2. Unknown RPC Method -> Status Code 12 (UNIMPLEMENTED)
	reqUnk := httptest.NewRequest(http.MethodPost, "/helloworld.Greeter/NonExistent", bytes.NewReader(body))
	reqUnk.Header.Set("Content-Type", "application/grpc")
	recUnk := httptest.NewRecorder()
	srv.ServeHTTP(recUnk, reqUnk)
	assert.Equal(t, strconv.Itoa(int(codes.Unimplemented)), recUnk.Header().Get("Grpc-Status"))

	// 3. Non-POST / non-gRPC Content-Type -> 415 Status
	reqBad := httptest.NewRequest(http.MethodGet, "/helloworld.Greeter/SayHello", nil)
	recBad := httptest.NewRecorder()
	srv.ServeHTTP(recBad, reqBad)
	assert.Equal(t, http.StatusUnsupportedMediaType, recBad.Code)
}

func TestGRPC_Serve_H2C(t *testing.T) {
	srv := grpc.NewServer()
	srv.RegisterService(&greeterServiceDesc, &greeterImpl{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		_ = srv.Serve(ln)
	}()

	// Connect using HTTP/2 cleartext client
	client := &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
		},
		Timeout: 5 * time.Second,
	}

	body := encodeGRPCFrame(t, &HelloRequest{Name: "H2CTest"})
	req, err := http.NewRequest(http.MethodPost, "http://"+ln.Addr().String()+"/helloworld.Greeter/SayHello", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/grpc")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/grpc", resp.Header.Get("Content-Type"))

	var reply HelloReply
	decodeGRPCFrame(t, resp.Body, &reply)
	assert.Equal(t, "Hello H2CTest", reply.Message)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, srv.GracefulStop(ctx))
}

