// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1engine_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
)

func TestRequestParsing_BasicGET(t *testing.T) {
	rawReq := "GET /api/v1/users?page=2&limit=50 HTTP/1.1\r\n" +
		"Host: api.vlhl.tf\r\n" +
		"User-Agent: test-agent\r\n" +
		"Accept: application/json\r\n" +
		"\r\n"

	br := bufio.NewReader(stringsReader(rawReq))

	var req h1engine.Request

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

	var req h1engine.Request

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

	var req h1engine.Request

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
	var res h1engine.Response

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

	err := res.WriteTo(bw, true, true)
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

	handler := &h1engine.ConnHandler{
		MaxBodySize: 1024 * 1024,
		Handler: func(req *h1engine.Request, res *h1engine.Response) error {
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
	srv := h1engine.NewServer(func(req *h1engine.Request, res *h1engine.Response) error {
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

	handler := &h1engine.ConnHandler{
		MaxBodySize: 1024 * 1024,
		Handler: func(req *h1engine.Request, res *h1engine.Response) error {
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
		_, _ = clientConn.Write(
			[]byte("POST /upload HTTP/1.1\r\nHost: localhost\r\nExpect: 100-continue\r\nContent-Length: 12\r\n\r\n"),
		)
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

	handler := &h1engine.ConnHandler{
		MaxBodySize: 1024 * 1024,
		Handler: func(req *h1engine.Request, res *h1engine.Response) error {
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

func TestRequestParsing_RFC9112_LeadingCRLF(t *testing.T) {
	// RFC 9112 §2.2: Server SHOULD ignore empty lines before request-line
	rawReq := "\r\n\r\nGET /ping HTTP/1.1\r\nHost: localhost\r\n\r\n"
	br := bufio.NewReader(stringsReader(rawReq))

	var req h1engine.Request

	err := req.ReadRequest(br, nil, 1024)
	if err != nil {
		t.Fatalf("unexpected error parsing request with leading CRLFs: %v", err)
	}

	if req.Path != "/ping" {
		t.Errorf("expected Path /ping, got %q", req.Path)
	}
}

func TestRequestParsing_RFC9112_MissingHost(t *testing.T) {
	// RFC 9112 §3.2: HTTP/1.1 request MUST include Host header
	rawReq := "GET /ping HTTP/1.1\r\nUser-Agent: test\r\n\r\n"
	br := bufio.NewReader(stringsReader(rawReq))

	var req h1engine.Request

	err := req.ReadRequest(br, nil, 1024)
	if err == nil {
		t.Fatal("expected error for missing Host header in HTTP/1.1 request")
	}
}

func TestH1_HTTP_Pipelining_Batching(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	server := &h1engine.Server{
		Addr: ln.Addr().String(),
		Handler: func(req *h1engine.Request, res *h1engine.Response) error {
			res.StatusCode = http.StatusOK
			res.Body = []byte("Pipelined:" + req.Path)
			return nil
		},
	}

	go func() {
		_ = server.Serve(ln)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send 16 pipelined requests in a single TCP write
	const pipelineCount = 16
	var batch bytes.Buffer
	for i := 0; i < pipelineCount; i++ {
		batch.WriteString("GET /pipeline/" + string(rune('A'+i)) + " HTTP/1.1\r\nHost: localhost\r\n\r\n")
	}

	_, err = conn.Write(batch.Bytes())
	if err != nil {
		t.Fatalf("failed writing batch: %v", err)
	}

	// Read and parse 16 responses
	br := bufio.NewReader(conn)
	for i := 0; i < pipelineCount; i++ {
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			t.Fatalf("failed reading response #%d: %v", i, err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("failed reading body #%d: %v", i, err)
		}

		expected := "Pipelined:/pipeline/" + string(rune('A'+i))
		if string(body) != expected {
			t.Errorf("response #%d expected %q, got %q", i, expected, string(body))
		}
	}
}

func stringsReader(s string) io.Reader {
	return bytes.NewReader([]byte(s))
}

func TestChunkedWriter_And_FormatHex(t *testing.T) {
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	cw := h1engine.NewChunkedWriter(bw)

	// Test zero byte write
	n, err := cw.Write([]byte{})
	if err != nil || n != 0 {
		t.Fatalf("expected 0, nil on empty write, got %d, %v", n, err)
	}

	// Test writing data chunks
	chunk1 := []byte("Hello, ")
	chunk2 := []byte("Chunked ")
	chunk3 := []byte("World!")

	_, _ = cw.Write(chunk1)
	_, _ = cw.Write(chunk2)
	_, _ = cw.Write(chunk3)

	// Close to write terminal chunk
	if err := cw.Close(); err != nil {
		t.Fatalf("unexpected Close error: %v", err)
	}

	// Verify with ChunkedReader
	cr := h1engine.NewChunkedReader(bufio.NewReader(&buf))
	decoded, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("failed decoding chunked output: %v", err)
	}

	expected := "Hello, Chunked World!"
	if string(decoded) != expected {
		t.Fatalf("expected %q, got %q", expected, string(decoded))
	}

	// Test FormatHexUint zero and numbers
	var hexBuf [16]byte
	n0 := h1engine.FormatHexUint(&hexBuf, 0)
	if string(hexBuf[:n0]) != "0" {
		t.Fatalf("expected 0, got %s", string(hexBuf[:n0]))
	}

	n255 := h1engine.FormatHexUint(&hexBuf, 255)
	if string(hexBuf[:n255]) != "ff" {
		t.Fatalf("expected ff, got %s", string(hexBuf[:n255]))
	}
}

func TestHeaders_Comprehensive(t *testing.T) {
	h := h1engine.NewHeadersWithCapacity(8)
	h.Set("Content-Type", "application/json")
	h.Set("X-Custom", "initial")
	h.Set("X-Custom", "updated") // overwrite

	if h.Get("X-Custom") != "updated" {
		t.Fatalf("expected updated, got %q", h.Get("X-Custom"))
	}
	if !h.Has("Content-Type") {
		t.Fatal("expected Content-Type to exist")
	}

	h.Add("X-List", "item1")
	h.Add("X-List", "item2")

	httpH := make(http.Header)
	httpH.Add("Authorization", "Bearer secret")
	httpH.Add("X-Server", "sein")
	h.AddFromHTTP(httpH)

	if h.Get("Authorization") != "Bearer secret" {
		t.Fatalf("expected Bearer secret, got %q", h.Get("Authorization"))
	}

	h.Del("X-Custom")
	if h.Has("X-Custom") || h.Get("X-Custom") != "" {
		t.Fatal("expected X-Custom to be deleted")
	}
}

func TestRequest_ClientIP_And_EarlyHints_Hijack(t *testing.T) {
	req := &h1engine.Request{
		RemoteAddr: "192.0.2.1:12345",
		Headers:    h1engine.NewHeadersWithCapacity(4),
	}

	// 1. Direct RemoteAddr with port
	if ip := req.ClientIP(); ip != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %q", ip)
	}

	// 2. Direct RemoteAddr without port
	req.RemoteAddr = "192.0.2.2"
	if ip := req.ClientIP(); ip != "192.0.2.2" {
		t.Fatalf("expected 192.0.2.2, got %q", ip)
	}

	// 4. Early Hints
	var hintsHeader http.Header
	req.EarlyHintsFn = func(h http.Header) error {
		hintsHeader = h
		return nil
	}
	_ = req.WriteEarlyHints(http.Header{"Link": []string{"</app.css>; rel=preload"}})
	if len(hintsHeader["Link"]) == 0 {
		t.Fatal("expected EarlyHints to be dispatched")
	}

	// 5. Hijack errors
	_, _, err := req.Hijack()
	if err == nil {
		t.Fatal("expected error when HijackFn is nil")
	}
}

func TestResponse_StreamingWriteTo(t *testing.T) {
	res := &h1engine.Response{
		StatusCode: http.StatusOK,
		StreamWriter: func(w io.Writer) error {
			_, err := w.Write([]byte("stream-chunk-data"))
			return err
		},
	}

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	err := res.WriteTo(bw, true, true)
	if err != nil {
		t.Fatalf("unexpected WriteTo error: %v", err)
	}

	// Read and verify response
	resp, err := http.ReadResponse(bufio.NewReader(&buf), nil)
	if err != nil {
		t.Fatalf("failed to read streamed response: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "stream-chunk-data" {
		t.Fatalf("expected stream-chunk-data, got %q", string(body))
	}
}

func TestServer_ListenErrors_And_Shutdown(t *testing.T) {
	srv := h1engine.NewServer(func(req *h1engine.Request, res *h1engine.Response) error {
		return nil
	})

	// Invalid address for ListenAndServe
	srv.Addr = "invalid:::addr:99999"
	if err := srv.ListenAndServe(); err == nil {
		t.Fatal("expected error on invalid ListenAndServe address")
	}

	// Invalid TLS cert for ListenAndServeTLS
	if err := srv.ListenAndServeTLS("nonexistent.crt", "nonexistent.key"); err == nil {
		t.Fatal("expected error on nonexistent TLS cert")
	}

	// Double shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = srv.Shutdown(ctx)
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("expected nil on second shutdown, got %v", err)
	}
}

func TestHeaderLine_EdgeCases(t *testing.T) {
	var h h1engine.Headers

	// Header line with leading/trailing spaces
	line := []byte("   X-Custom-Header   :    custom-value   ")
	ok := h.ParseHeaderLine(line)
	if !ok {
		t.Fatal("expected ParseHeaderLine to succeed")
	}
	if h.Get("X-Custom-Header") != "custom-value" {
		t.Fatalf("unexpected header parsed: %q", h.Get("X-Custom-Header"))
	}

	// Missing colon
	if h.ParseHeaderLine([]byte("InvalidHeaderLineWithoutColon")) {
		t.Fatal("expected ParseHeaderLine to fail without colon")
	}

	// Empty key
	if h.ParseHeaderLine([]byte("   : value")) {
		t.Fatal("expected ParseHeaderLine to fail with empty key")
	}

	// IsKeepAlive checks
	h.Reset()
	if h.IsKeepAlive("HTTP/1.0") {
		t.Fatal("HTTP/1.0 without keep-alive should be false")
	}
	h.Set("Connection", "Keep-Alive")
	if !h.IsKeepAlive("HTTP/1.0") {
		t.Fatal("HTTP/1.0 with Connection: Keep-Alive should be true")
	}
	h.Set("Connection", "close")
	if h.IsKeepAlive("HTTP/1.1") {
		t.Fatal("HTTP/1.1 with Connection: close should be false")
	}
}
