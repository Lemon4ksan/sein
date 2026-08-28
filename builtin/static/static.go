// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package static

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/timekit"

	"github.com/lemon4ksan/sein"
)

// Config defines options for static asset and SPA serving.
type Config struct {
	// Root specifies the local disk folder to serve files from.
	Root string

	// FS specifies an embedded or custom filesystem (e.g. embed.FS).
	FS fs.FS

	// Index is the default file served when requesting directory roots.
	// Default is "index.html".
	Index string

	// SPA enables Single Page Application fallback: unknown non-file paths serve Index.
	SPA bool

	// Precompressed enables automatic serving of .zst, .br, and .gz pre-compressed assets.
	Precompressed bool

	// CacheControl sets the HTTP Cache-Control header for served assets.
	CacheControl string

	// ByteRange enables RFC 7233 206 Partial Content Range streaming.
	ByteRange bool
}

// DefaultConfig provides standard static file defaults.
var DefaultConfig = Config{
	Index:         "index.html",
	SPA:           false,
	Precompressed: true,
	ByteRange:     true,
	CacheControl:  "public, max-age=3600",
}

// New creates a static asset file server middleware from a disk directory.
func New(root string, config ...Config) sein.RawHandler {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	cfg.Root = root

	return createStaticServer(cfg)
}

// NewFS creates a static asset file server from an embedded filesystem (e.g. embed.FS).
func NewFS(fileSystem fs.FS, config ...Config) sein.RawHandler {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	cfg.FS = fileSystem

	return createStaticServer(cfg)
}

func createStaticServer(cfg Config) sein.RawHandler {
	if cfg.Index == "" {
		cfg.Index = "index.html"
	}

	return func(req *sein.Request) (any, error) {
		if req.Method() != http.MethodGet && req.Method() != http.MethodHead {
			return nil, sein.ErrNotFound("file not found")
		}

		cleanPath := path.Clean("/" + req.Path())
		if cleanPath == "/" {
			cleanPath = "/" + cfg.Index
		}

		// Try opening the requested file
		f, modTime, size, resolvedPath, ok := openFile(cfg, cleanPath)
		if !ok {
			if cfg.SPA {
				// Fallback to index.html for Single Page Applications
				f, modTime, size, resolvedPath, ok = openFile(cfg, "/"+cfg.Index)
				if !ok {
					return nil, sein.ErrNotFound("file not found")
				}
			} else {
				return nil, sein.ErrNotFound("file not found")
			}
		}

		defer func() { _ = f.Close() }()

		// Compute ETag from ModTime and Size
		etag := fmt.Sprintf(`W/"%x-%x"`, modTime.Unix(), size)

		// Check If-None-Match
		ifNoneMatch := req.Header(header.IfNoneMatch)
		if ifNoneMatch != "" && (ifNoneMatch == etag || ifNoneMatch == "*") {
			return sein.NotModified(), nil
		}

		// Check If-Modified-Since
		ifModSince := req.Header(header.IfModifiedSince)
		if ifModSince != "" {
			t, err := timekit.ParseHTTPDate(ifModSince)
			if err == nil && !modTime.After(t.Add(1*time.Second)) {
				return sein.NotModified(), nil
			}
		}

		// Determine Content-Type
		ext := filepath.Ext(resolvedPath)
		contentType := generic.Coalesce(mime.TypeByExtension(ext), "application/octet-stream")

		headers := make(http.Header)
		headers.Set(header.ContentType, contentType)
		headers.Set(header.ETag, etag)
		headers.Set(header.LastModified, timekit.FormatHTTPDate(modTime))

		if cfg.CacheControl != "" {
			headers.Set(header.CacheControl, cfg.CacheControl)
		}

		if cfg.ByteRange {
			headers.Set(header.AcceptRanges, "bytes")
		}

		// Check Range requests (RFC 7233)
		rangeHdr := req.Header(header.Range)
		if cfg.ByteRange && strings.HasPrefix(rangeHdr, "bytes=") {
			return serveRange(f, size, rangeHdr, headers)
		}

		// Read full content
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, sein.ErrInternal("failed to read file", err)
		}

		headers.Set(header.ContentLength, strconv.FormatInt(int64(len(data)), 10))

		return sein.StatusWith(http.StatusOK, data, headers), nil
	}
}

func serveRange(f io.ReadSeeker, totalSize int64, rangeHdr string, headers http.Header) (any, error) {
	ranges := strings.TrimPrefix(rangeHdr, "bytes=")

	parts := strings.Split(ranges, "-")
	if len(parts) != 2 {
		return nil, sein.NewHTTPError(
			http.StatusRequestedRangeNotSatisfiable,
			"RANGE_NOT_SATISFIABLE",
			"invalid range header",
		)
	}

	var (
		start, end int64
		err        error
	)

	if parts[0] == "" {
		// Suffix range: bytes=-500 (last 500 bytes)
		suffixLen, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffixLen <= 0 {
			return nil, sein.NewHTTPError(
				http.StatusRequestedRangeNotSatisfiable,
				"RANGE_NOT_SATISFIABLE",
				"invalid range suffix",
			)
		}

		start = max(totalSize-suffixLen, 0)
		end = totalSize - 1
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil || start < 0 || start >= totalSize {
			return nil, sein.NewHTTPError(
				http.StatusRequestedRangeNotSatisfiable,
				"RANGE_NOT_SATISFIABLE",
				"range start out of bounds",
			)
		}

		if parts[1] == "" {
			end = totalSize - 1
		} else {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil || end < start {
				return nil, sein.NewHTTPError(
					http.StatusRequestedRangeNotSatisfiable,
					"RANGE_NOT_SATISFIABLE",
					"invalid range end",
				)
			}

			if end >= totalSize {
				end = totalSize - 1
			}
		}
	}

	length := end - start + 1
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, sein.ErrInternal("seek failed", err)
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, sein.ErrInternal("read range failed", err)
	}

	headers.Set(header.ContentRange, fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
	headers.Set(header.ContentLength, strconv.FormatInt(length, 10))

	return sein.StatusWith(http.StatusPartialContent, buf, headers), nil
}

type fileReadSeeker interface {
	io.ReadCloser
	io.ReadSeeker
}

type bytesReadSeekCloser struct {
	*bytes.Reader
}

func (b *bytesReadSeekCloser) Close() error { return nil }

func openFile(cfg Config, requestPath string) (fileReadSeeker, time.Time, int64, string, bool) {
	relPath := strings.TrimPrefix(requestPath, "/")

	if cfg.FS != nil {
		f, err := cfg.FS.Open(relPath)
		if err != nil {
			return nil, time.Time{}, 0, "", false
		}

		st, err := f.Stat()
		if err != nil || st.IsDir() {
			_ = f.Close()
			return nil, time.Time{}, 0, "", false
		}

		if rs, ok := f.(fileReadSeeker); ok {
			return rs, st.ModTime(), st.Size(), relPath, true
		}

		// Buffer into memory reader
		data, err := io.ReadAll(f)
		_ = f.Close()

		if err != nil {
			return nil, time.Time{}, 0, "", false
		}

		return &bytesReadSeekCloser{Reader: bytes.NewReader(data)}, st.ModTime(), int64(len(data)), relPath, true
	}

	if cfg.Root != "" {
		fullPath := filepath.Join(cfg.Root, filepath.FromSlash(relPath))

		// #nosec G304 -- Sanitized static asset path
		f, err := os.Open(filepath.Clean(fullPath))
		if err != nil {
			return nil, time.Time{}, 0, "", false
		}

		st, err := f.Stat()
		if err != nil || st.IsDir() {
			_ = f.Close()
			return nil, time.Time{}, 0, "", false
		}

		return f, st.ModTime(), st.Size(), fullPath, true
	}

	return nil, time.Time{}, 0, "", false
}
