// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule represents a parsed recurrence schedule that can calculate the next execution time.
type Schedule interface {
	Next(from time.Time) time.Time
}

// EverySchedule represents a fixed-interval recurring schedule.
type EverySchedule struct {
	Interval time.Duration
}

// Next returns the next tick time after from.
func (s EverySchedule) Next(from time.Time) time.Time {
	return from.Add(s.Interval)
}

// CronSchedule represents a parsed standard 5-field cron expression (minute hour day-of-month month day-of-week).
type CronSchedule struct {
	Minutes     uint64 // bits 0-59
	Hours       uint64 // bits 0-23
	DaysOfMonth uint64 // bits 1-31
	Months      uint64 // bits 1-12
	DaysOfWeek  uint64 // bits 0-6 (0 = Sunday)
}

// Parse parses a cron expression (standard 5 fields or @descriptors) into a Schedule.
func Parse(spec string) (Schedule, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty cron spec")
	}

	// Descriptor shortcuts
	if strings.HasPrefix(spec, "@") {
		lower := strings.ToLower(spec)
		if strings.HasPrefix(lower, "@every ") {
			durationStr := strings.TrimSpace(spec[7:])
			d, err := time.ParseDuration(durationStr)
			if err != nil {
				return nil, fmt.Errorf("invalid @every duration %q: %w", durationStr, err)
			}
			if d <= 0 {
				return nil, fmt.Errorf("duration must be positive: %v", d)
			}
			return EverySchedule{Interval: d}, nil
		}

		switch lower {
		case "@hourly":
			return Parse("0 * * * *")
		case "@daily", "@midnight":
			return Parse("0 0 * * *")
		case "@weekly":
			return Parse("0 0 * * 0")
		case "@monthly":
			return Parse("0 0 1 * *")
		case "@yearly", "@annually":
			return Parse("0 0 1 1 *")
		default:
			return nil, fmt.Errorf("unrecognized cron descriptor: %s", spec)
		}
	}

	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 cron fields (min hour dom mon dow), got %d in %q", len(fields), spec)
	}

	minutes, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}

	hours, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}

	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day of month field: %w", err)
	}

	months, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}

	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("invalid day of week field: %w", err)
	}

	return &CronSchedule{
		Minutes:     minutes,
		Hours:       hours,
		DaysOfMonth: dom,
		Months:      months,
		DaysOfWeek:  dow,
	}, nil
}

func parseField(field string, min, max int) (uint64, error) {
	var bits uint64

	for _, expr := range strings.Split(field, ",") {
		expr = strings.TrimSpace(expr)
		if expr == "" {
			continue
		}

		step := 1
		rangeStr := expr

		if parts := strings.Split(expr, "/"); len(parts) == 2 {
			rangeStr = parts[0]
			s, err := strconv.Atoi(parts[1])
			if err != nil || s <= 0 {
				return 0, fmt.Errorf("invalid step: %s", parts[1])
			}
			step = s
		}

		var start, end int
		if rangeStr == "*" {
			start = min
			end = max
		} else if parts := strings.Split(rangeStr, "-"); len(parts) == 2 {
			s, err1 := strconv.Atoi(parts[0])
			e, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || s > e || s < min || e > max {
				return 0, fmt.Errorf("invalid range: %s", rangeStr)
			}
			start = s
			end = e
		} else {
			val, err := strconv.Atoi(rangeStr)
			if err != nil || val < min || val > max {
				return 0, fmt.Errorf("invalid value %q (must be between %d and %d)", rangeStr, min, max)
			}
			start = val
			end = val
		}

		for i := start; i <= end; i += step {
			bits |= 1 << i
		}
	}

	return bits, nil
}

// Next finds the next point in time that satisfies the cron schedule.
func (s *CronSchedule) Next(from time.Time) time.Time {
	// Start looking at the next whole second
	t := from.Truncate(time.Minute).Add(time.Minute)

	// Scan up to 5 years into the future
	limit := t.AddDate(5, 0, 0)

	for t.Before(limit) {
		if (s.Months & (1 << t.Month())) == 0 {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		domMatch := (s.DaysOfMonth & (1 << t.Day())) != 0
		dowMatch := (s.DaysOfWeek & (1 << t.Weekday())) != 0

		if !domMatch || !dowMatch {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		if (s.Hours & (1 << t.Hour())) == 0 {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}

		if (s.Minutes & (1 << t.Minute())) == 0 {
			t = t.Add(time.Minute)
			continue
		}

		return t
	}

	return time.Time{}
}
