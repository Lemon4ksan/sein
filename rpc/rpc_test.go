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
