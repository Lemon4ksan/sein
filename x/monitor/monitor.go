// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package monitor provides a lightweight, zero-dependency real-time server dashboard
// displaying CPU, memory, goroutines, GC pauses, and RPS metrics in the browser.
package monitor

import (
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// DefaultPath is the default monitoring dashboard URL.
const DefaultPath = "/monitor"

// StatsPayload represents the real-time server health metrics payload.
type StatsPayload struct {
	Timestamp      string  `json:"timestamp"`
	Goroutines     int     `json:"goroutines"`
	AllocMB        float64 `json:"alloc_mb"`
	TotalAllocMB   float64 `json:"total_alloc_mb"`
	SysMB          float64 `json:"sys_mb"`
	NumGC          uint32  `json:"num_gc"`
	GCCPUFraction  float64 `json:"gc_cpu_fraction"`
	NumCPU         int     `json:"num_cpu"`
	RequestsTotal  uint64  `json:"requests_total"`
}

// Config configures the Monitor dashboard.
type Config struct {
	// Path is the URL prefix for the dashboard. Default is "/monitor".
	Path string
	// Title is the browser page title. Default is "Sein Server Monitor".
	Title string
}

// Option configures Monitor settings.
type Option func(*Config)

// WithPath sets the dashboard route path.
func WithPath(p string) Option {
	return func(c *Config) {
		c.Path = p
	}
}

// WithTitle sets the page title.
func WithTitle(t string) Option {
	return func(c *Config) {
		c.Title = t
	}
}

func renderDashboardHTML(title, dataURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #f8fafc; margin: 0; padding: 24px; }
        h1 { margin-top: 0; font-size: 24px; color: #38bdf8; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; margin-top: 20px; }
        .card { background: #1e293b; border: 1px solid #334155; border-radius: 8px; padding: 16px; text-align: center; }
        .val { font-size: 28px; font-weight: bold; color: #4ade80; margin-top: 8px; }
        .lbl { font-size: 13px; color: #94a3b8; text-transform: uppercase; letter-spacing: 0.5px; }
    </style>
</head>
<body>
    <h1>⚡ %s</h1>
    <div class="grid">
        <div class="card"><div class="lbl">Goroutines</div><div class="val" id="goroutines">-</div></div>
        <div class="card"><div class="lbl">Allocated RAM</div><div class="val" id="alloc">-</div></div>
        <div class="card"><div class="lbl">System RAM</div><div class="val" id="sys">-</div></div>
        <div class="card"><div class="lbl">GC Cycles</div><div class="val" id="num_gc">-</div></div>
        <div class="card"><div class="lbl">CPU Cores</div><div class="val" id="num_cpu">%d</div></div>
        <div class="card"><div class="lbl">Total Requests</div><div class="val" id="req_total">-</div></div>
    </div>
    <script>
        async function update() {
            try {
                const res = await fetch("%s");
                const d = await res.json();
                document.getElementById("goroutines").textContent = d.goroutines;
                document.getElementById("alloc").textContent = d.alloc_mb + " MB";
                document.getElementById("sys").textContent = d.sys_mb + " MB";
                document.getElementById("num_gc").textContent = d.num_gc;
                document.getElementById("req_total").textContent = d.requests_total;
            } catch (e) {}
        }
        setInterval(update, 1000);
        update();
    </script>
</body>
</html>`, title, title, runtime.NumCPU(), dataURL)
}

// Register attaches the live monitoring web dashboard and metrics data endpoint to the sein server.
func Register(app *sein.Server, opts ...Option) {
	cfg := Config{
		Path:  DefaultPath,
		Title: "Sein Server Monitor",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	var reqCounter atomic.Uint64
	app.Use(func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			reqCounter.Add(1)
			return next(req)
		}
	})

	basePath := strings.TrimSuffix(cfg.Path, "/")
	dataURL := basePath + "/data"
	html := renderDashboardHTML(cfg.Title, dataURL)

	// HTML Dashboard page
	app.Get(basePath, func(_ *sein.Request) (any, error) {
		return sein.OK[any](html).
			WithHeader(header.ContentType, header.MIMETextHTMLCharsetUTF8), nil
	})

	app.Get(basePath+"/", func(_ *sein.Request) (any, error) {
		return sein.OK[any](html).
			WithHeader(header.ContentType, header.MIMETextHTMLCharsetUTF8), nil
	})

	// JSON Metrics endpoint
	app.Get(dataURL, func(_ *sein.Request) (any, error) {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		payload := StatsPayload{
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			Goroutines:    runtime.NumGoroutine(),
			AllocMB:       float64(mem.Alloc) / 1024 / 1024,
			TotalAllocMB:  float64(mem.TotalAlloc) / 1024 / 1024,
			SysMB:         float64(mem.Sys) / 1024 / 1024,
			NumGC:         mem.NumGC,
			GCCPUFraction: mem.GCCPUFraction,
			NumCPU:        runtime.NumCPU(),
			RequestsTotal: reqCounter.Load(),
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		return sein.OK[any](data).
			WithHeader(header.ContentType, header.MIMEApplicationJSONCharsetUTF8), nil
	})
}
