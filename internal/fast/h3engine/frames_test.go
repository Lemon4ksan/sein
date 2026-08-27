// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3engine

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/sein/internal/quic/quicvarint"
)

func TestSettingsEncodeAndParse(t *testing.T) {
	st := &Settings{
		MaxFieldSectionSize: 65536,
		QpackMaxTableCap:    4096,
		QpackBlockedStreams: 100,
		EnableDatagrams:     true,
		EnableConnect:       true,
		Other: map[uint64]uint64{
			0x1234: 5678,
		},
	}

	encoded := st.Encode()
	r := bytes.NewReader(encoded)

	frameType, payloadLen, err := ReadFrameHeader(r)
	if err != nil {
		t.Fatalf("failed to read frame header: %v", err)
	}

	if frameType != FrameTypeSettings {
		t.Fatalf("expected FrameTypeSettings (%d), got %d", FrameTypeSettings, frameType)
	}

	if int(payloadLen) != r.Len() {
		t.Fatalf("payload length mismatch: header specifies %d, remaining buffer is %d", payloadLen, r.Len())
	}

	parsed, err := DecodeSettings(r, payloadLen)
	if err != nil {
		t.Fatalf("DecodeSettings failed: %v", err)
	}

	if !parsed.EnableConnect {
		t.Errorf("expected EnableConnect to be true per RFC 9220 §3 (SETTINGS_ENABLE_CONNECT_PROTOCOL=0x08)")
	}
}

func TestReadFrameHeader(t *testing.T) {
	var buf []byte

	buf = quicvarint.Append(buf, FrameTypeHeaders)
	buf = quicvarint.Append(buf, 1024)

	r := bytes.NewReader(buf)

	fType, pLen, err := ReadFrameHeader(r)
	if err != nil {
		t.Fatalf("ReadFrameHeader failed: %v", err)
	}

	if fType != FrameTypeHeaders {
		t.Errorf("got frame type %d, want %d", fType, FrameTypeHeaders)
	}

	if pLen != 1024 {
		t.Errorf("got payload length %d, want 1024", pLen)
	}
}

func TestAppendFrameHeaders(t *testing.T) {
	hHeader := appendHeadersHeader(nil, 512)
	rH := bytes.NewReader(hHeader)

	fType, pLen, err := ReadFrameHeader(rH)
	if err != nil || fType != FrameTypeHeaders || pLen != 512 {
		t.Fatalf("appendHeadersHeader failed: type=%d, len=%d, err=%v", fType, pLen, err)
	}

	dHeader := appendDataHeader(nil, 2048)
	rD := bytes.NewReader(dHeader)

	fType, pLen, err = ReadFrameHeader(rD)
	if err != nil || fType != FrameTypeData || pLen != 2048 {
		t.Fatalf("appendDataHeader failed: type=%d, len=%d, err=%v", fType, pLen, err)
	}
}

func TestDecodeSettings_ReservedH2Settings_And_Errors(t *testing.T) {
	// Test reserved H2 settings IDs: 0x00, 0x02, 0x03, 0x04, 0x05
	for _, reservedID := range []uint64{0x00, 0x02, 0x03, 0x04, 0x05} {
		var payload []byte
		payload = quicvarint.Append(payload, reservedID)
		payload = quicvarint.Append(payload, 100)

		r := bytes.NewReader(payload)
		_, err := DecodeSettings(r, uint64(len(payload)))
		if err != ErrH3SettingsError {
			t.Fatalf("expected ErrH3SettingsError for reserved setting %d, got %v", reservedID, err)
		}
	}

	// Truncated payload
	var trunc []byte
	trunc = quicvarint.Append(trunc, SettingMaxFieldSectionSize)
	r := bytes.NewReader(trunc)
	_, err := DecodeSettings(r, uint64(len(trunc)))
	if err == nil {
		t.Fatal("expected error on truncated settings payload")
	}
}
