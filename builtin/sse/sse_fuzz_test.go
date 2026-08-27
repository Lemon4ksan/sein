// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sse_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/lemon4ksan/sein/builtin/sse"
)

func FuzzSSEFormatting(f *testing.F) {
	f.Add("greeting", "hello world", "evt-1", int64(3000))
	f.Add("update", "line1\nline2\nline3", "", int64(0))
	f.Add("", "", "id-99", int64(500))

	f.Fuzz(func(t *testing.T, event, data, id string, retryMs int64) {
		var buf bytes.Buffer
		w := sse.NewWriter(&buf)

		if event != "" {
			_ = w.SendEvent(event, data)
		} else {
			_ = w.Send(data)
		}

		if id != "" {
			_ = w.SendID(id, data)
		}

		if retryMs > 0 {
			_ = w.SendRetry(time.Duration(retryMs) * time.Millisecond)
		}
	})
}
