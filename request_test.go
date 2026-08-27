// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"net/http"
	"net/netip"
	"testing"

	"github.com/lemon4ksan/sein"
)

func TestRequest_ClientIP_Direct(t *testing.T) {
	httpReq, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	httpReq.RemoteAddr = "203.0.113.195:45678"

	req := sein.NewRequest(httpReq, nil)
	if ip := req.ClientIP(); ip != "203.0.113.195" {
		t.Fatalf("expected 203.0.113.195, got %q", ip)
	}

	if ip := req.IP(); ip != "203.0.113.195" {
		t.Fatalf("expected 203.0.113.195 from IP(), got %q", ip)
	}
}

func TestRequest_ClientIP_TrustedProxyTraversal(t *testing.T) {
	httpReq, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	httpReq.RemoteAddr = "127.0.0.1:45678"
	// Client -> Proxy1 (Public) -> Cloudflare / Internal LB -> sein
	httpReq.Header.Set("X-Forwarded-For", "198.51.100.42, 203.0.113.50, 10.0.0.1, 127.0.0.1")

	req := sein.NewRequest(httpReq, nil)

	// Right-to-left traversal should skip 127.0.0.1 and 10.0.0.1 (in DefaultTrustedProxies)
	// and pick 203.0.113.50 as the rightmost untrusted client IP.
	expected := "203.0.113.50"
	if ip := req.ClientIP(); ip != expected {
		t.Fatalf("expected %q, got %q", expected, ip)
	}

	// Test IPs list
	ips := req.IPs()
	if len(ips) != 4 || ips[0] != "198.51.100.42" || ips[1] != "203.0.113.50" {
		t.Fatalf("unexpected IPs slice: %v", ips)
	}
}

func TestRequest_ClientIP_CustomTrust(t *testing.T) {
	httpReq, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	httpReq.Header.Set("X-Forwarded-For", "198.51.100.42, 203.0.113.50")

	req := sein.NewRequest(httpReq, nil)

	customTrust := []netip.Prefix{
		netip.MustParsePrefix("203.0.113.0/24"),
	}

	if ip := req.ClientIPWithTrust(customTrust); ip != "198.51.100.42" {
		t.Fatalf("expected 198.51.100.42, got %q", ip)
	}
}

func TestRequest_Scheme_Sanitization(t *testing.T) {
	tests := []struct {
		name      string
		fwdProto  string
		forwarded string
		expected  string
	}{
		{
			name:     "Valid HTTPS",
			fwdProto: "https",
			expected: "https",
		},
		{
			name:     "Valid HTTP",
			fwdProto: "http",
			expected: "http",
		},
		{
			name:     "Case Insensitive HTTPS",
			fwdProto: "HTTPS",
			expected: "https",
		},
		{
			name:     "Malicious Javascript scheme",
			fwdProto: "javascript:alert(1)",
			expected: "http",
		},
		{
			name:     "Malicious Data scheme",
			fwdProto: "data:text/html,<script>",
			expected: "http",
		},
		{
			name:      "Forwarded RFC 7239 header",
			forwarded: "proto=https;for=192.0.2.60",
			expected:  "https",
		},
		{
			name:      "Forwarded Quoted proto",
			forwarded: `for=192.0.2.60;proto="https"`,
			expected:  "https",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			httpReq, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
			if tc.fwdProto != "" {
				httpReq.Header.Set("X-Forwarded-Proto", tc.fwdProto)
			}

			if tc.forwarded != "" {
				httpReq.Header.Set("Forwarded", tc.forwarded)
			}

			req := sein.NewRequest(httpReq, nil)
			if scheme := req.Scheme(); scheme != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, scheme)
			}
		})
	}
}

func TestRequest_ArenaAllocation(t *testing.T) {
	httpReq, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req := sein.NewRequest(httpReq, nil)
	defer req.Release()

	// 1. Arena scope
	arena := req.Arena()
	if arena == nil {
		t.Fatal("expected non-nil arena")
	}

	// 2. AllocBytes
	b := req.AllocBytes(64)
	if len(b) != 64 {
		t.Fatalf("expected length 64, got %d", len(b))
	}
	copy(b, "zero-gc arena bytes")

	// 3. AllocString
	str := req.AllocString("hello from sein bump allocator")
	if str != "hello from sein bump allocator" {
		t.Fatalf("unexpected string: %s", str)
	}
}
