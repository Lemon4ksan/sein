// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dump provides zero-allocation HTTP request and response inspection middleware,
// generating detailed debug logs and runnable curl CLI commands.
package dump

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/lemon4ksan/foundation/async/logkit"
	"github.com/lemon4ksan/foundation/codec/json"

	"github.com/lemon4ksan/sein"
)

// Config configures the Dump middleware.
type Config struct {
	// DumpRequest logs the incoming HTTP request. Default is true.
	DumpRequest bool
	// DumpResponse logs the outgoing HTTP response. Default is true.
	DumpResponse bool
	// CurlCommand generates a runnable curl CLI command in the log. Default is true.
	CurlCommand bool
	// Logger is the structured logger instance.
	Logger logkit.Logger
	// Output is an optional custom dump callback.
	Output func(dumpStr string)
}

// Option configures Dump settings.
type Option func(*Config)

// WithDumpRequest toggles request dumping.
func WithDumpRequest(enabled bool) Option {
	return func(c *Config) {
		c.DumpRequest = enabled
	}
}

// WithDumpResponse toggles response dumping.
func WithDumpResponse(enabled bool) Option {
	return func(c *Config) {
		c.DumpResponse = enabled
	}
}

// WithCurlCommand toggles curl command generation.
func WithCurlCommand(enabled bool) Option {
	return func(c *Config) {
		c.CurlCommand = enabled
	}
}

// WithLogger sets a structured logger.
func WithLogger(l logkit.Logger) Option {
	return func(c *Config) {
		c.Logger = l
	}
}

// WithOutput sets a custom dump sink callback.
func WithOutput(fn func(dumpStr string)) Option {
	return func(c *Config) {
		c.Output = fn
	}
}

func buildCurl(req *sein.Request) string {
	var buf strings.Builder
	buf.WriteString("curl -X " + req.Method() + " ")

	url := req.Scheme() + "://" + req.Host() + req.Path()
	if q := req.Query(""); q != "" {
		url += "?" + string(q)
	}

	buf.WriteString("'" + url + "'")

	if raw := req.Raw(); raw != nil {
		for k, vv := range raw.Header {
			for _, v := range vv {
				fmt.Fprintf(&buf, " -H '%s: %s'", k, v)
			}
		}
	}

	if body := req.RawBody(); len(body) > 0 {
		fmt.Fprintf(&buf, " --data-raw '%s'", string(body))
	}

	return buf.String()
}

// New creates an HTTP dump and inspection middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		DumpRequest:  true,
		DumpResponse: true,
		CurlCommand:  true,
		Logger:       logkit.New(logkit.DefaultConfig(logkit.LevelDebug)),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			var sb bytes.Buffer

			if cfg.DumpRequest {
				sb.WriteString("\n========== HTTP REQUEST ==========\n")
				fmt.Fprintf(&sb, "%s %s %s\n", req.Method(), req.Path(), req.Proto())
				fmt.Fprintf(&sb, "Host: %s\n", req.Host())
				fmt.Fprintf(&sb, "Remote-IP: %s\n", req.ClientIP())

				if raw := req.Raw(); raw != nil {
					for k, vv := range raw.Header {
						for _, v := range vv {
							fmt.Fprintf(&sb, "%s: %s\n", k, v)
						}
					}
				}

				if body := req.RawBody(); len(body) > 0 {
					sb.WriteString("\n" + string(body) + "\n")
				}

				if cfg.CurlCommand {
					sb.WriteString("\n[cURL]: " + buildCurl(req) + "\n")
				}
			}

			res, err := next(req)

			if cfg.DumpResponse {
				sb.WriteString("\n========== HTTP RESPONSE ==========\n")
				statusCode := http.StatusOK
				if err != nil {
					if domainErr, ok := err.(sein.DomainError); ok {
						statusCode = domainErr.HTTPStatus()
					} else if httpErr, ok := sein.AsHTTPError(err); ok {
						statusCode = httpErr.HTTPStatus()
					} else {
						statusCode = http.StatusInternalServerError
					}
					fmt.Fprintf(&sb, "Status: %d\nError: %s\n", statusCode, err.Error())
				} else {
					if holder, ok := res.(sein.ResponseHolder); ok {
						if code := holder.StatusCode(); code != 0 {
							statusCode = code
						}
						fmt.Fprintf(&sb, "Status: %d\n", statusCode)
						for k, vv := range holder.ResponseHeaders() {
							for _, v := range vv {
								fmt.Fprintf(&sb, "%s: %s\n", k, v)
							}
						}
						if b, ok := holder.ResponseBody().([]byte); ok {
							sb.WriteString("\n" + string(b) + "\n")
						} else {
							data, _ := json.Marshal(holder.ResponseBody())
							sb.WriteString("\n" + string(data) + "\n")
						}
					} else {
						fmt.Fprintf(&sb, "Status: %d\n", statusCode)
						data, _ := json.Marshal(res)
						sb.WriteString("\n" + string(data) + "\n")
					}
				}
				sb.WriteString("====================================\n")
			}

			dumpStr := sb.String()
			if cfg.Output != nil {
				cfg.Output(dumpStr)
			} else if cfg.Logger != nil {
				cfg.Logger.Debug(dumpStr)
			}

			return res, err
		}
	}
}
