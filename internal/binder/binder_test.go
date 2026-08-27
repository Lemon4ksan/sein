// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein/internal/binder"
)

type mockRequestView struct {
	params     map[string]string
	queries    map[string]string
	rawQueries map[string][]string
	headers    map[string]string
	cookies    map[string]string
	bearer     string
	hasBearer  bool
	clientIP   string
	proto      string
	scheme     string
	host       string
	method     string
	path       string
	formVals   map[string]string
	bodyBytes    []byte
	contexts     map[reflect.Type]any
	cookieSecret string
}

func (m *mockRequestView) Param(name string) string { return m.params[name] }
func (m *mockRequestView) Query(key string) string  { return m.queries[key] }
func (m *mockRequestView) Header(key string) string { return m.headers[key] }
func (m *mockRequestView) Cookie(name string) (string, error) {
	if v, ok := m.cookies[name]; ok {
		return v, nil
	}

	return "", errors.New("no cookie")
}
func (m *mockRequestView) CookieSecret() string             { return m.cookieSecret }
func (m *mockRequestView) BearerToken() (string, bool)      { return m.bearer, m.hasBearer }
func (m *mockRequestView) ClientIP() string                 { return m.clientIP }
func (m *mockRequestView) Protocol() string                 { return m.proto }
func (m *mockRequestView) Scheme() string                   { return m.scheme }
func (m *mockRequestView) Host() string                     { return m.host }
func (m *mockRequestView) Method() string                   { return m.method }
func (m *mockRequestView) Path() string                     { return m.path }
func (m *mockRequestView) FormValue(key string) string      { return m.formVals[key] }
func (m *mockRequestView) FormFile(key string) (any, error) { return nil, errors.New("no file") }
func (m *mockRequestView) FormFiles(key string) ([]any, error) {
	return nil, errors.New("no files")
}
func (m *mockRequestView) Body() []byte                     { return m.bodyBytes }
func (m *mockRequestView) BindJSON(dest any) error          { return nil }
func (m *mockRequestView) RawURLQuery() map[string][]string { return m.rawQueries }
func (m *mockRequestView) GetContext(typ reflect.Type) (any, bool) {
	if m.contexts != nil {
		v, ok := m.contexts[typ]
		return v, ok
	}

	return nil, false
}

type ComprehensiveTestDTO struct {
	ID        uint64        `path:"id"`
	QueryName string        `query:"name,trim,lower"`
	Tags      []string      `query:"tags"`
	Page      int           `query:"page,default=1,min=1,max=100"`
	Timeout   time.Duration `query:"timeout,default=10s"`
	CreatedAt time.Time     `query:"created_at"`
	IP        net.IP        `net:"ip"`
	IPAddr    netip.Addr    `net:"ip"`
	AuthToken string        `auth:"bearer,required"`
	ApiKey    string        `header:"X-Api-Key,required"`
	SessionID string        `cookie:"sid,required"`
	RawBody   []byte        `body:"raw"`
}

func TestBinderIngest(t *testing.T) {
	mockReq := &mockRequestView{
		params: map[string]string{"id": "12345"},
		queries: map[string]string{
			"name":       "  GORDON FREEMAN  ",
			"created_at": "2026-08-26T12:00:00Z",
		},
		rawQueries: map[string][]string{
			"tags": {"go,rust", "docker"},
		},
		headers: map[string]string{
			"X-Api-Key": "secret-api-key",
		},
		cookies: map[string]string{
			"sid": "session-12345",
		},
		bearer:    "bearer-jwt-token",
		hasBearer: true,
		clientIP:  "192.0.2.1",
		bodyBytes: []byte("raw binary payload"),
	}

	var dto ComprehensiveTestDTO

	err := binder.Ingest(mockReq, &dto)
	require.NoError(t, err)

	assert.Equal(t, uint64(12345), dto.ID)
	assert.Equal(t, "gordon freeman", dto.QueryName)
	assert.Equal(t, 1, dto.Page)
	assert.Equal(t, 10*time.Second, dto.Timeout)
	assert.Equal(t, []string{"go", "rust", "docker"}, dto.Tags)
	assert.Equal(t, "192.0.2.1", dto.IP.String())
	assert.Equal(t, "192.0.2.1", dto.IPAddr.String())
	assert.Equal(t, "bearer-jwt-token", dto.AuthToken)
	assert.Equal(t, "secret-api-key", dto.ApiKey)
	assert.Equal(t, "session-12345", dto.SessionID)
	assert.Equal(t, []byte("raw binary payload"), dto.RawBody)
}

func TestBinderValidationFailures(t *testing.T) {
	type ValidatedDTO struct {
		Limit int    `query:"limit,min=5,max=20"`
		Code  string `query:"code,len=4"`
		Email string `query:"email,email"`
		Role  string `query:"role,enum=admin|editor"`
	}

	// 1. Min failure
	req1 := &mockRequestView{queries: map[string]string{"limit": "3"}}

	var dto1 ValidatedDTO

	err1 := binder.Ingest(req1, &dto1)
	assert.Error(t, err1)

	// 2. Len failure
	req2 := &mockRequestView{queries: map[string]string{"code": "12345"}}

	var dto2 ValidatedDTO

	err2 := binder.Ingest(req2, &dto2)
	assert.Error(t, err2)

	// 3. Email failure
	req3 := &mockRequestView{queries: map[string]string{"email": "not-an-email"}}

	var dto3 ValidatedDTO

	err3 := binder.Ingest(req3, &dto3)
	assert.Error(t, err3)

	// 4. Enum failure
	req4 := &mockRequestView{queries: map[string]string{"role": "superuser"}}

	var dto4 ValidatedDTO

	err4 := binder.Ingest(req4, &dto4)
	assert.Error(t, err4)
}

type CustomID uint64
type CustomSlug string

type CustomAliasDTO struct {
	ID   CustomID   `path:"id"`
	Slug CustomSlug `query:"slug,trim"`
	Age  *int       `query:"age"`
}

func TestBinderCustomAliasesAndPointers(t *testing.T) {
	req := &mockRequestView{
		params:  map[string]string{"id": "99998888"},
		queries: map[string]string{"slug": "  custom-slug-value  ", "age": "42"},
	}

	var dto CustomAliasDTO
	err := binder.Ingest(req, &dto)
	require.NoError(t, err)

	assert.Equal(t, CustomID(99998888), dto.ID)
	assert.Equal(t, CustomSlug("custom-slug-value"), dto.Slug)
	require.NotNil(t, dto.Age)
	assert.Equal(t, 42, *dto.Age)
}

func TestBinderScalarIngestion(t *testing.T) {
	req := &mockRequestView{
		params: map[string]string{"": "777666"},
	}

	var id uint64
	err := binder.IngestScalarType(req, reflect.TypeOf(id), &id)
	require.NoError(t, err)
	assert.Equal(t, uint64(777666), id)

	var customID CustomID
	err = binder.IngestScalarType(req, reflect.TypeOf(customID), &customID)
	require.NoError(t, err)
	assert.Equal(t, CustomID(777666), customID)

	var str string
	err = binder.IngestScalarType(req, reflect.TypeOf(str), &str)
	require.NoError(t, err)
	assert.Equal(t, "777666", str)
}

type SignedCookieDTO struct {
	SessionID string `cookie:"session,sign,required"`
	Device    string `cookie:"device"`
}

func signCookieForTest(val, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(val))
	sig := hex.EncodeToString(mac.Sum(nil))
	return val + "." + sig
}

func TestBinder_SignedCookie(t *testing.T) {
	secret := "test-secret-key-32-bytes-long!!"
	validSession := "session-uuid-12345"
	signedCookie := signCookieForTest(validSession, secret)

	t.Run("Valid Signature", func(t *testing.T) {
		req := &mockRequestView{
			cookies: map[string]string{
				"session": signedCookie,
				"device":  "mobile",
			},
			cookieSecret: secret,
		}

		var dto SignedCookieDTO
		err := binder.Ingest(req, &dto)
		require.NoError(t, err)
		assert.Equal(t, validSession, dto.SessionID)
		assert.Equal(t, "mobile", dto.Device)
	})

	t.Run("Tampered Signature", func(t *testing.T) {
		req := &mockRequestView{
			cookies: map[string]string{
				"session": validSession + ".tampered-signature-123456",
			},
			cookieSecret: secret,
		}

		var dto SignedCookieDTO
		err := binder.Ingest(req, &dto)
		require.Error(t, err)
		assert.True(t, errors.Is(err, binder.ErrInvalidCookieSignature))
	})

	t.Run("Missing Signature On Signed Cookie", func(t *testing.T) {
		req := &mockRequestView{
			cookies: map[string]string{
				"session": "raw-unsigned-session-token",
			},
			cookieSecret: secret,
		}

		var dto SignedCookieDTO
		err := binder.Ingest(req, &dto)
		require.Error(t, err)
		assert.True(t, errors.Is(err, binder.ErrInvalidCookieSignature))
	})
}
