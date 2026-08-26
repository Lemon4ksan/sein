// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/sein/internal/h1"
)

func TestRequestParsing_BasicGET(t *testing.T) {
	rawReq := "GET /api/v1/users?page=2&limit=50 HTTP/1.1\r\n" +
		"Host: api.vlhl.tf\r\n" +
		"User-Agent: test-agent\r\n" +
		"Accept: application/json\r\n" +
		"\r\n"

	br := bufio.NewReader(stringsReader(rawReq))
	var req h1.Request

	err := req.ReadRequest(br, nil, 1024*1024)
	if err != nil {
		t.Fatalf("unexpected error reading request: %v", err)
	}

	if req.Method != "GET" {
		t.Errorf("expected Method GET, got %q", req.Method)
	}
	if req.Path != "/api/v1/users" {
		t.Errorf("expected Path /api/v1/users, got %q", req.Path)
	}
	if req.Query != "page=2&limit=50" {
		t.Errorf("expected Query page=2&limit=50, got %q", req.Query)
	}
	if req.Proto != "HTTP/1.1" {
		t.Errorf("expected Proto HTTP/1.1, got %q", req.Proto)
	}
	if req.Host != "api.vlhl.tf" {
		t.Errorf("expected Host api.vlhl.tf, got %q", req.Host)
	}
	if req.Headers.Get("User-Agent") != "test-agent" {
		t.Errorf("expected User-Agent test-agent, got %q", req.Headers.Get("User-Agent"))
	}
}

func TestRequestParsing_POSTWithBody(t *testing.T) {
	body := `{"name":"alice","age":30}`
	rawReq := "POST /users HTTP/1.1\r\n" +
		"Host: api.vlhl.tf\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 25\r\n" +
		"\r\n" +
		body

	br := bufio.NewReader(stringsReader(rawReq))
	var req h1.Request

	err := req.ReadRequest(br, nil, 1024*1024)
	if err != nil {
		t.Fatalf("unexpected error reading request: %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("expected Method POST, got %q", req.Method)
	}
	if string(req.Body) != body {
		t.Errorf("expected body %q, got %q", body, string(req.Body))
	}
}

func TestRequestParsing_ChunkedBody(t *testing.T) {
	rawReq := "POST /upload HTTP/1.1\r\n" +
		"Host: api.vlhl.tf\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"4\r\nWiki\r\n" +
		"6\r\npedia \r\n" +
		"E\r\nin \r\n\r\nchunks.\r\n" +
		"0\r\n\r\n"

	br := bufio.NewReader(stringsReader(rawReq))
	var req h1.Request

	err := req.ReadRequest(br, nil, 1024*1024)
	if err != nil {
		t.Fatalf("unexpected error reading chunked request: %v", err)
	}

	expected := "Wikipedia in \r\n\r\nchunks."
	if string(req.Body) != expected {
		t.Errorf("expected chunked body %q, got %q", expected, string(req.Body))
	}
}

func TestResponseSerialization(t *testing.T) {
	var res h1.Response
	res.StatusCode = 201
	res.Headers.Set("Content-Type", "application/json")
	res.Cookies = append(res.Cookies, &http.Cookie{
		Name:  "token",
		Value: "secret123",
		Path:  "/",
	})
	res.Body = []byte(`{"created":true}`)

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)

	err := res.WriteTo(bw, true)
	if err != nil {
		t.Fatalf("unexpected error writing response: %v", err)
	}

	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("HTTP/1.1 201 Created\r\n")) {
		t.Errorf("missing status line in response: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("Connection: keep-alive\r\n")) {
		t.Errorf("missing keep-alive in response: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("Content-Length: 16\r\n")) {
		t.Errorf("missing content-length in response: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("Set-Cookie: token=secret123; Path=/\r\n")) {
		t.Errorf("missing set-cookie in response: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte(`{"created":true}`)) {
		t.Errorf("missing body in response: %s", out)
	}
}

func TestConnHandler_EndToEndKeepAlive(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	handler := &h1.ConnHandler{
		MaxBodySize: 1024 * 1024,
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = 200
			res.Headers.Set("Content-Type", "text/plain")
			res.Body = []byte("Hello, " + req.Path)
			return nil
		},
	}

	go func() {
		_ = handler.ServeConn(serverConn)
	}()

	// 1. Send first request
	_, _ = clientConn.Write([]byte("GET /first HTTP/1.1\r\nHost: localhost\r\n\r\n"))

	br := bufio.NewReader(clientConn)
	line1, err := br.ReadString('\n')
	if err != nil || line1 != "HTTP/1.1 200 OK\r\n" {
		t.Fatalf("expected 200 OK, got line: %q, err: %v", line1, err)
	}

	// Drain rest of headers and body
	for {
		l, _ := br.ReadString('\n')
		if l == "\r\n" {
			break
		}
	}
	body1 := make([]byte, len("Hello, /first"))
	_, _ = io.ReadFull(br, body1)
	if string(body1) != "Hello, /first" {
		t.Errorf("expected body 'Hello, /first', got %q", string(body1))
	}

	// 2. Send second request on SAME keep-alive connection
	_, _ = clientConn.Write([]byte("GET /second HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))

	line2, err := br.ReadString('\n')
	if err != nil || line2 != "HTTP/1.1 200 OK\r\n" {
		t.Fatalf("expected 200 OK on 2nd req, got line: %q, err: %v", line2, err)
	}
	for {
		l, _ := br.ReadString('\n')
		if l == "\r\n" {
			break
		}
	}
	body2 := make([]byte, len("Hello, /second"))
	_, _ = io.ReadFull(br, body2)
	if string(body2) != "Hello, /second" {
		t.Errorf("expected body 'Hello, /second', got %q", string(body2))
	}

	_ = clientConn.Close()
}

func TestServer_GracefulShutdown(t *testing.T) {
	srv := h1.NewServer(func(req *h1.Request, res *h1.Response) error {
		res.StatusCode = 200
		res.Body = []byte("OK")
		return nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	time.Sleep(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = srv.Shutdown(ctx)
	if err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestRequest_Expect100Continue(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	handler := &h1.ConnHandler{
		MaxBodySize: 1024 * 1024,
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = 200
			res.Body = []byte("Received: " + string(req.Body))
			return nil
		},
	}

	go func() {
		_ = handler.ServeConn(serverConn)
	}()

	// 1. Client sends headers with Expect: 100-continue
	go func() {
		_, _ = clientConn.Write([]byte("POST /upload HTTP/1.1\r\nHost: localhost\r\nExpect: 100-continue\r\nContent-Length: 12\r\n\r\n"))
	}()

	br := bufio.NewReader(clientConn)
	// Expect intermediate "HTTP/1.1 100 Continue\r\n\r\n"
	line1, err := br.ReadString('\n')
	if err != nil || line1 != "HTTP/1.1 100 Continue\r\n" {
		t.Fatalf("expected 100 Continue, got line: %q, err: %v", line1, err)
	}
	line2, err := br.ReadString('\n')
	if err != nil || line2 != "\r\n" {
		t.Fatalf("expected CRLF after 100 Continue, got: %q", line2)
	}

	// 2. Client sends body now
	_, _ = clientConn.Write([]byte("hello world!"))

	// 3. Client receives final 200 OK
	finalStatus, err := br.ReadString('\n')
	if err != nil || finalStatus != "HTTP/1.1 200 OK\r\n" {
		t.Fatalf("expected final 200 OK, got: %q, err: %v", finalStatus, err)
	}

	_ = clientConn.Close()
}

func TestRequest_Hijack(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	handler := &h1.ConnHandler{
		MaxBodySize: 1024 * 1024,
		Handler: func(req *h1.Request, res *h1.Response) error {
			conn, rw, err := req.Hijack()
			if err != nil {
				return err
			}
			go func() {
				defer conn.Close()
				_, _ = rw.WriteString("CUSTOM_BINARY_PROTOCOL_OK\n")
				_ = rw.Flush()
			}()
			return nil
		},
	}

	go func() {
		_ = handler.ServeConn(serverConn)
	}()

	_, _ = clientConn.Write([]byte("GET /upgrade HTTP/1.1\r\nHost: localhost\r\nUpgrade: custom\r\n\r\n"))

	br := bufio.NewReader(clientConn)
	msg, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("failed reading hijacked connection: %v", err)
	}

	if msg != "CUSTOM_BINARY_PROTOCOL_OK\n" {
		t.Errorf("expected 'CUSTOM_BINARY_PROTOCOL_OK\n', got %q", msg)
	}

	_ = clientConn.Close()
}

func stringsReader(s string) io.Reader {
	return bytes.NewReader([]byte(s))
}
