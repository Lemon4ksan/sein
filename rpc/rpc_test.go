// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rpc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/rpc"
)

type UserRequest struct {
	ID   int    `path:"id"`
	Role string `json:"role"`
}

type UserResponse struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
}

func TestRPC_Call(t *testing.T) {
	app := sein.New()

	app.Post("/users/:id/update", func(ctx context.Context, req UserRequest) (UserResponse, error) {
		return UserResponse{
			ID:    req.ID,
			Email: "user" + req.Role + "@example.com",
		}, nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	ctx := context.Background()
	res, err := rpc.Call[UserResponse](ctx, http.DefaultClient, http.MethodPost, srv.URL+"/users/:id/update", UserRequest{
		ID:   42,
		Role: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, 42, res.ID)
	assert.Equal(t, "useradmin@example.com", res.Email)
}

type OrderService struct{}

type CreateOrderReq struct {
	Item  string `json:"item"`
	Count int    `json:"count"`
}

type CreateOrderRes struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func (s *OrderService) CreateOrder(ctx context.Context, req CreateOrderReq) (CreateOrderRes, error) {
	return CreateOrderRes{
		OrderID: "ord-1234",
		Status:  "created",
	}, nil
}

func TestRPC_Mount(t *testing.T) {
	app := sein.New()
	rpc.Mount(app, "/rpc/orders", &OrderService{})

	srv := httptest.NewServer(app)
	defer srv.Close()

	ctx := context.Background()
	res, err := rpc.Call[CreateOrderRes](ctx, http.DefaultClient, http.MethodPost, srv.URL+"/rpc/orders/CreateOrder", CreateOrderReq{
		Item:  "TF2 Key",
		Count: 5,
	})

	require.NoError(t, err)
	assert.Equal(t, "ord-1234", res.OrderID)
	assert.Equal(t, "created", res.Status)
}

type PingRes struct {
	Reply string `json:"reply"`
}

type TimeRes struct {
	Date string `json:"date"`
}

type ItemRes struct {
	Item string `json:"item"`
}

type MultiSignatureService struct{}

func (s *MultiSignatureService) Ping() (PingRes, error) {
	return PingRes{Reply: "pong"}, nil
}

func (s *MultiSignatureService) GetTime(ctx context.Context) (TimeRes, error) {
	return TimeRes{Date: "2026-08-27"}, nil
}

func (s *MultiSignatureService) Echo(in CreateOrderReq) (ItemRes, error) {
	return ItemRes{Item: in.Item}, nil
}

func TestRPC_AllSignatures_And_Errors(t *testing.T) {
	app := sein.New()
	rpc.Mount(app, "/multi", &MultiSignatureService{})

	app.Get("/health", func(ctx context.Context) (PingRes, error) {
		return PingRes{Reply: "healthy"}, nil
	})

	app.Get("/error", func(ctx context.Context) (string, error) {
		return "", sein.ErrBadRequest("validation failed")
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	ctx := context.Background()

	// 1. func(s *Service) (Res, error)
	res1, err := rpc.Call[PingRes, any](ctx, http.DefaultClient, http.MethodPost, srv.URL+"/multi/Ping", nil)
	require.NoError(t, err)
	assert.Equal(t, "pong", res1.Reply)

	// 2. func(s *Service, ctx context.Context) (Res, error)
	res2, err := rpc.Call[TimeRes, any](ctx, http.DefaultClient, http.MethodPost, srv.URL+"/multi/GetTime", nil)
	require.NoError(t, err)
	assert.Equal(t, "2026-08-27", res2.Date)

	// 3. func(s *Service, in Req) (Res, error)
	res3, err := rpc.Call[ItemRes, CreateOrderReq](ctx, http.DefaultClient, http.MethodPost, srv.URL+"/multi/Echo", CreateOrderReq{Item: "Box"})
	require.NoError(t, err)
	assert.Equal(t, "Box", res3.Item)

	// 4. GET method call
	res4, err := rpc.Call[PingRes, any](ctx, http.DefaultClient, http.MethodGet, srv.URL+"/health", nil)
	require.NoError(t, err)
	assert.Equal(t, "healthy", res4.Reply)

	// 5. Server error handling
	_, err = rpc.Call[string, any](ctx, http.DefaultClient, http.MethodGet, srv.URL+"/error", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}
