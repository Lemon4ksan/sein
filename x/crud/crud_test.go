// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crud_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/x/crud"
)

type Product struct {
	ID    int64   `json:"id"`
	Title string  `json:"title"`
	Price float64 `json:"price"`
}

type mockProductRepository struct {
	mu       sync.RWMutex
	items    map[int64]*Product
	nextID   int64
}

func newMockProductRepo() *mockProductRepository {
	return &mockProductRepository{
		items:  make(map[int64]*Product),
		nextID: 1,
	}
}

func (m *mockProductRepository) FindByID(ctx context.Context, id any) (*Product, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var key int64
	switch v := id.(type) {
	case int64:
		key = v
	case int:
		key = int64(v)
	default:
		return nil, errors.New("invalid id type")
	}

	p, ok := m.items[key]
	if !ok {
		return nil, errors.New("orm: record not found")
	}
	return p, nil
}

func (m *mockProductRepository) FindAll(ctx context.Context) ([]*Product, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]*Product, 0, len(m.items))
	for _, p := range m.items {
		res = append(res, p)
	}
	return res, nil
}

func (m *mockProductRepository) Create(ctx context.Context, entity *Product) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entity.ID == 0 {
		entity.ID = m.nextID
		m.nextID++
	}
	m.items[entity.ID] = entity
	return nil
}

func (m *mockProductRepository) Update(ctx context.Context, entity *Product) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.items[entity.ID]; !ok {
		return errors.New("orm: record not found")
	}
	m.items[entity.ID] = entity
	return nil
}

func (m *mockProductRepository) Delete(ctx context.Context, id any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var key int64
	switch v := id.(type) {
	case int64:
		key = v
	case int:
		key = int64(v)
	default:
		return errors.New("invalid id type")
	}

	if _, ok := m.items[key]; !ok {
		return errors.New("orm: record not found")
	}
	delete(m.items, key)
	return nil
}

func TestCRUD_Lifecycle(t *testing.T) {
	app := sein.New()
	repo := newMockProductRepo()

	crud.Mount(app.Group("/products"), repo)

	// 1. GET /products -> empty list
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/products/", nil)
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.JSONEq(t, `[]`, rec1.Body.String())

	// 2. POST /products -> Create
	p1 := Product{Title: "Laptop", Price: 999.99}
	body, _ := json.Marshal(p1)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/products/", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusCreated, rec2.Code)

	var created Product
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &created))
	assert.Equal(t, int64(1), created.ID)
	assert.Equal(t, "Laptop", created.Title)

	// 3. GET /products/1 -> Found
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/products/1", nil)
	app.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusOK, rec3.Code)

	var fetched Product
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &fetched))
	assert.Equal(t, int64(1), fetched.ID)
	assert.Equal(t, "Laptop", fetched.Title)

	// 4. GET /products/999 -> 404 Not Found
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodGet, "/products/999", nil)
	app.ServeHTTP(rec4, req4)
	assert.Equal(t, http.StatusNotFound, rec4.Code)

	// 5. PUT /products/1 -> Update
	updated := Product{ID: 1, Title: "Gaming Laptop", Price: 1299.99}
	upBody, _ := json.Marshal(updated)
	rec5 := httptest.NewRecorder()
	req5 := httptest.NewRequest(http.MethodPut, "/products/1", bytes.NewReader(upBody))
	req5.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(rec5, req5)
	assert.Equal(t, http.StatusOK, rec5.Code)

	// 6. DELETE /products/1 -> 204 No Content
	rec6 := httptest.NewRecorder()
	req6 := httptest.NewRequest(http.MethodDelete, "/products/1", nil)
	app.ServeHTTP(rec6, req6)
	assert.Equal(t, http.StatusNoContent, rec6.Code)

	// 7. GET /products/1 -> 404
	rec7 := httptest.NewRecorder()
	req7 := httptest.NewRequest(http.MethodGet, "/products/1", nil)
	app.ServeHTTP(rec7, req7)
	assert.Equal(t, http.StatusNotFound, rec7.Code)
}

func TestCRUD_ReadOnly(t *testing.T) {
	app := sein.New()
	repo := newMockProductRepo()
	_ = repo.Create(context.Background(), &Product{ID: 42, Title: "Read Only", Price: 10})

	crud.Mount(app.Group("/catalog"), repo, crud.WithReadOnly())

	// GET is allowed
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/catalog/42", nil)
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// POST is not registered (404/405)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/catalog/", bytes.NewReader([]byte(`{}`)))
	app.ServeHTTP(rec2, req2)
	assert.NotEqual(t, http.StatusCreated, rec2.Code)
}

func TestCRUD_CustomIDParam(t *testing.T) {
	app := sein.New()
	repo := newMockProductRepo()
	_ = repo.Create(context.Background(), &Product{ID: 7, Title: "Item 7", Price: 77})

	crud.Mount(app.Group("/items"), repo, crud.WithIDParam("item_id"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/items/7", nil)
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
