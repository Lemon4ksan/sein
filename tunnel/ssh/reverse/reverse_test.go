// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reverse_test

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	golangssh "golang.org/x/crypto/ssh"

	"github.com/lemon4ksan/sein/tunnel/ssh/reverse"
)

// ============================================================================
// 1. REVERSE ROUTER TESTS
// ============================================================================

func TestRouter_RegistrationAndLookup(t *testing.T) {
	t.Parallel()

	dummyConn := &golangssh.ServerConn{}

	t.Run("auto_generated_subdomain_on_empty_host", func(t *testing.T) {
		router := reverse.NewRouter("sein.dev")
		tun, err := router.Register(dummyConn, "", 8080)
		require.NoError(t, err)
		require.NotNil(t, tun)

		assert.Contains(t, tun.Host, ".sein.dev")
		assert.Equal(t, uint32(8080), tun.Port)
	})

	t.Run("explicit_host_registration_and_lookup", func(t *testing.T) {
		router := reverse.NewRouter("sein.dev")
		tun, err := router.Register(dummyConn, "my-app.sein.dev", 9090)
		require.NoError(t, err)
		assert.Equal(t, "my-app.sein.dev", tun.Host)

		// Lookup by Host
		found, ok := router.Lookup("my-app.sein.dev")
		assert.True(t, ok)
		assert.Equal(t, tun, found)

		// Lookup by Port
		foundPort, okPort := router.LookupPort(9090)
		assert.True(t, okPort)
		assert.Equal(t, tun, foundPort)
	})

	t.Run("duplicate_host_registration_fails", func(t *testing.T) {
		router := reverse.NewRouter("sein.dev")
		_, err1 := router.Register(dummyConn, "duplicate.sein.dev", 3000)
		require.NoError(t, err1)

		_, err2 := router.Register(dummyConn, "duplicate.sein.dev", 3000)
		assert.ErrorIs(t, err2, reverse.ErrHostAlreadyBound)
	})

	t.Run("unregister_clears_route", func(t *testing.T) {
		router := reverse.NewRouter("sein.dev")
		tun, err := router.Register(dummyConn, "temp.sein.dev", 4000)
		require.NoError(t, err)

		router.Unregister("temp.sein.dev")

		_, ok := router.Lookup(tun.Host)
		assert.False(t, ok)

		_, okPort := router.LookupPort(4000)
		assert.False(t, okPort)
	})
}

// ============================================================================
// 2. HTTP REVERSE GATEWAY TESTS
// ============================================================================

func TestGateway_ServeHTTP_Errors(t *testing.T) {
	t.Parallel()

	t.Run("unregistered_host_returns_404", func(t *testing.T) {
		router := reverse.NewRouter("sein.dev")
		gateway := reverse.NewGateway(router)

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://unknown.sein.dev/", nil)
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		gateway.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "404 Tunnel Not Found")
	})

	t.Run("registered_host_with_closed_ssh_conn_returns_502", func(t *testing.T) {
		router := reverse.NewRouter("sein.dev")
		gateway := reverse.NewGateway(router)

		dummyConn := &golangssh.ServerConn{}
		_, err := router.Register(dummyConn, "bound.sein.dev", 80)
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://bound.sein.dev/", nil)
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		gateway.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadGateway, rec.Code)
		assert.Contains(t, rec.Body.String(), "502 Bad Gateway")
	})
}

func TestPeekSNI(t *testing.T) {
	t.Parallel()

	t.Run("invalid_non_tls_header_fails", func(t *testing.T) {
		c1, c2 := net.Pipe()
		t.Cleanup(func() {
			_ = c1.Close()
			_ = c2.Close()
		})

		go func() {
			_, _ = c1.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		}()

		_, conn, err := reverse.PeekSNI(c2)
		require.Error(t, err)
		assert.ErrorIs(t, err, reverse.ErrInvalidTLSHeader)

		// Read back bytes from connection bridge
		buf := make([]byte, 16)
		n, _ := conn.Read(buf)
		assert.Equal(t, "GET / HTTP/1.1\r\n", string(buf[:n]))
	})

	t.Run("valid_tls_client_hello_sni_extraction", func(t *testing.T) {
		rawClientHello := buildMockTLSClientHello("sni.example.com")
		require.NotEmpty(t, rawClientHello)

		c1, c2 := net.Pipe()
		t.Cleanup(func() {
			_ = c1.Close()
			_ = c2.Close()
		})

		go func() {
			_, _ = c1.Write(rawClientHello)
		}()

		sni, peekedConn, err := reverse.PeekSNI(c2)
		require.NoError(t, err)
		assert.Equal(t, "sni.example.com", sni)

		buf := make([]byte, len(rawClientHello))
		n, _ := io.ReadFull(peekedConn, buf)
		assert.Equal(t, len(rawClientHello), n)
	})
}

func buildMockTLSClientHello(serverName string) []byte {
	clientConn, serverConn := net.Pipe()

	tlsClient := tls.Client(clientConn, &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true,
	})

	ch := make(chan []byte, 1)

	go func() {
		buf := make([]byte, 4096)
		n, _ := serverConn.Read(buf)
		_ = serverConn.Close()
		_ = clientConn.Close()

		if n > 0 {
			res := make([]byte, n)
			copy(res, buf[:n])

			ch <- res
		} else {
			ch <- nil
		}
	}()

	go func() {
		_ = tlsClient.Handshake()
	}()

	select {
	case b := <-ch:
		return b
	case <-time.After(5 * time.Second):
		return nil
	}
}
