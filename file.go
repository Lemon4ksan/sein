// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
)

// File represents an uploaded multipart form file with zero-allocation streaming and direct disk-save helpers.
type File struct {
	Filename    string
	Size        int64
	ContentType string
	Header      textproto.MIMEHeader
	header      *multipart.FileHeader
	cachedBytes []byte
}

// NewFile constructs a File from a multipart.FileHeader.
func NewFile(fh *multipart.FileHeader) *File {
	var ct string
	if fh.Header != nil {
		ct = fh.Header.Get("Content-Type")
	}
	return &File{
		Filename:    fh.Filename,
		Size:        fh.Size,
		ContentType: ct,
		Header:      fh.Header,
		header:      fh,
	}
}

// Open opens the underlying uploaded file stream for reading.
func (f *File) Open() (io.ReadCloser, error) {
	if f.header == nil {
		return nil, ErrBadRequest("file stream is not available")
	}
	return f.header.Open()
}

// Bytes reads and caches the full uploaded file content into memory.
func (f *File) Bytes() ([]byte, error) {
	if f.cachedBytes != nil {
		return f.cachedBytes, nil
	}

	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	f.cachedBytes = data
	return data, nil
}

// SaveTo writes the uploaded file directly to the specified destination filesystem path.
func (f *File) SaveTo(dstPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}
