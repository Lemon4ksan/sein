// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/lemon4ksan/foundation/types/uuid"

	"github.com/lemon4ksan/sein"
)

// UserRequestDTO demonstrates a complete contract binding path, query, header, and body.
type UserRequestDTO struct {
	UserID   uuid.UUID           `path:"id,uuid"`
	Query    string              `query:"q,default=active,trim,lower"`
	Limit    int                 `query:"limit,default=25,positive,multiple_of=5"`
	TraceID  string              `header:"X-Trace-ID,required"`
	ClientIP net.IP              `net:"ip"`
	Password sein.Secret[string] `json:"password" validate:"min=8"`
}

func Example() {
	app := sein.New()

	app.Post("/users/:id", func(ctx context.Context, req UserRequestDTO) (map[string]any, error) {
		return map[string]any{
			"user_id":  req.UserID.String(),
			"query":    req.Query,
			"limit":    req.Limit,
			"trace_id": req.TraceID,
			"password": req.Password.String(), // Returns masked "******"
		}, nil
	})

	httpReq := httptest.NewRequest(
		http.MethodPost,
		"/users/a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11?q=+ADMIN+&limit=50",
		strings.NewReader(`{"password":"my-secret-password"}`),
	)
	httpReq.Header.Set("X-Trace-ID", "trace-98765")
	httpReq.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httpReq)

	fmt.Println(rec.Body.String())
	// Output:
	// {"limit":50,"password":"******","query":"admin","trace_id":"trace-98765","user_id":"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"}
}
