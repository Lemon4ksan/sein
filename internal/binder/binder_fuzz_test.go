// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder_test

import (
	"testing"

	"github.com/lemon4ksan/sein/internal/binder"
)

type FuzzTargetDTO struct {
	Name  string `query:"name,trim,lower"`
	Age   int    `query:"age,gt=0,le=150"`
	Email string `query:"email,email"`
}

func FuzzBinderIngest(f *testing.F) {
	f.Add("Alice", "25", "alice@example.com")
	f.Add("  BOB  ", "-5", "not-an-email")
	f.Add("", "999999", "")

	f.Fuzz(func(t *testing.T, name, ageStr, email string) {
		mock := &mockRequestView{
			queries: map[string]string{
				"name":  name,
				"age":   ageStr,
				"email": email,
			},
		}

		var dest FuzzTargetDTO
		_ = binder.Ingest(mock, &dest)
	})
}
