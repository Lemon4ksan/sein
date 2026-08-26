// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2engine_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"golang.org/x/net/http2"

	"github.com/lemon4ksan/sein/internal/fast/h2engine"
)

func TestH2Server_EndToEnd(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer ln.Close()

	handler := func(req *h2engine.ServerRequest, res *h2engine.ServerResponse) error {
		switch req.Path {
		case "/hello":
			res.StatusCode = http.StatusOK
			res.Headers.Set("Content-Type", "text/plain")
			res.Body = []byte("Hello HTTP/2 World!")

			return nil

		case "/echo":
			res.StatusCode = http.StatusOK
			res.Headers.Set("Content-Type", "application/octet-stream")

			res.Body = append([]byte("Echo: "), req.Body...)

			return nil

		default:
			res.StatusCode = http.StatusNotFound
			res.Body = []byte("404 Not Found")
			return nil
		}
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				sc := h2engine.NewServerConn(c, handler)
				defer sc.Release()

				_ = sc.Serve()
			}(conn)
		}
	}()

	// Connect using Go's official HTTP/2 client transport over cleartext TCP
	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
		Timeout: 5 * time.Second,
	}

	addr := ln.Addr().String()

	// 1. Test GET /hello
	resp, err := client.Get("http://" + addr + "/hello")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "HTTP/2.0", resp.Proto)

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	require.NoError(t, err)
	assert.Equal(t, "Hello HTTP/2 World!", string(body))

	// 2. Test POST /echo
	postData := "Multiplexed Stream Payload 2026"
	respPost, err := client.Post("http://"+addr+"/echo", "text/plain", strings.NewReader(postData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respPost.StatusCode)

	postBody, err := io.ReadAll(respPost.Body)
	_ = respPost.Body.Close()

	require.NoError(t, err)
	assert.Equal(t, "Echo: "+postData, string(postBody))

	// 3. Test High-Concurrency Streams
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			r, err := client.Get("http://" + addr + "/hello")
			if err != nil {
				t.Errorf("stream %d failed: %v", id, err)
				return
			}

			b, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()

			if string(b) != "Hello HTTP/2 World!" {
				t.Errorf("unexpected body on stream %d: %s", id, string(b))
			}
		}(i)
	}

	wg.Wait()
}
