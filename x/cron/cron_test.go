// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/sein/x/cron"
)

func TestCron_ParseDescriptors(t *testing.T) {
	testCases := []struct {
		spec    string
		wantErr bool
	}{
		{"@every 100ms", false},
		{"@hourly", false},
		{"@daily", false},
		{"@weekly", false},
		{"@monthly", false},
		{"@yearly", false},
		{"*/5 * * * *", false},
		{"0 0 * * *", false},
		{"0 12 1 1 *", false},
		{"invalid spec", true},
		{"@every invalid", true},
		{"* * *", true},
	}

	for _, tc := range testCases {
		t.Run(tc.spec, func(t *testing.T) {
			sched, err := cron.Parse(tc.spec)
			if (err != nil) != tc.wantErr {
				t.Fatalf("spec %q: expected error=%v, got err=%v", tc.spec, tc.wantErr, err)
			}
			if err == nil && sched == nil {
				t.Fatalf("spec %q: expected non-nil schedule", tc.spec)
			}
		})
	}
}

func TestCron_Execution(t *testing.T) {
	sched := cron.New()

	var counter atomic.Int64

	sched.Every(50*time.Millisecond, func(ctx context.Context) error {
		counter.Add(1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched.Start(ctx)

	time.Sleep(250 * time.Millisecond)

	sched.Stop()

	count := counter.Load()
	if count < 2 {
		t.Errorf("expected at least 2 executions, got %d", count)
	}
}
