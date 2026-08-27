// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cron provides a zero-allocation, in-memory background cron task scheduler for sein.
package cron

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/sein"
)

// Job is a background task executed on a recurring schedule.
type Job func(ctx context.Context) error

// EntryID represents a unique identifier for a registered cron entry.
type EntryID uint64

// Entry describes a scheduled cron job.
type Entry struct {
	ID       EntryID
	Schedule Schedule
	Job      Job
	Next     time.Time
	Prev     time.Time
}

// Scheduler coordinates and runs scheduled cron jobs in the background.
type Scheduler struct {
	entries []*Entry
	running atomic.Bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
	mu      sync.RWMutex
	nextID  atomic.Uint64
}

// New creates an unstarted [Scheduler] instance.
func New() *Scheduler {
	return &Scheduler{
		entries: make([]*Entry, 0, 8),
		stopCh:  make(chan struct{}),
	}
}

// Schedule registers a cron job with a cron expression or @descriptor string (e.g. "*/5 * * * *", "@every 30s").
func (s *Scheduler) Schedule(spec string, job Job) (EntryID, error) {
	sched, err := Parse(spec)
	if err != nil {
		return 0, err
	}

	return s.ScheduleWith(sched, job), nil
}

// Every registers a recurring task to execute at a fixed time interval.
func (s *Scheduler) Every(interval time.Duration, job Job) EntryID {
	return s.ScheduleWith(EverySchedule{Interval: interval}, job)
}

// ScheduleWith registers a job with an arbitrary [Schedule] implementation.
func (s *Scheduler) ScheduleWith(sched Schedule, job Job) EntryID {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := EntryID(s.nextID.Add(1))
	now := time.Now()

	entry := &Entry{
		ID:       id,
		Schedule: sched,
		Job:      job,
		Next:     sched.Next(now),
		Prev:     time.Time{},
	}

	s.entries = append(s.entries, entry)

	return id
}

// Start launches the background scheduler event loop in a dedicated goroutine.
func (s *Scheduler) Start(ctx context.Context) {
	if s.running.Swap(true) {
		return // Already running
	}

	s.wg.Add(1)
	go s.run(ctx)
}

// Stop gracefully terminates the scheduler and waits for in-flight jobs to complete.
func (s *Scheduler) Stop() {
	if !s.running.Swap(false) {
		return // Not running
	}

	close(s.stopCh)
	s.wg.Wait()
}

// Attach registers graceful start and shutdown hooks on the provided sein Server.
func (s *Scheduler) Attach(srv *sein.Server) {
	s.Start(context.Background())
}

func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	for {
		s.mu.Lock()
		now := time.Now()
		var earliest time.Time

		for _, entry := range s.entries {
			if !entry.Next.IsZero() && (now.Equal(entry.Next) || now.After(entry.Next)) {
				entry.Prev = now
				entry.Next = entry.Schedule.Next(now)

				// Dispatch job in non-blocking worker goroutine
				s.wg.Add(1)
				go func(e *Entry, execTime time.Time) {
					defer s.wg.Done()
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[cron] panic in job #%d: %v\n", e.ID, r)
						}
					}()

					_ = e.Job(ctx)
				}(entry, now)
			}

			if !entry.Next.IsZero() {
				if earliest.IsZero() || entry.Next.Before(earliest) {
					earliest = entry.Next
				}
			}
		}
		s.mu.Unlock()

		sleepDur := 100 * time.Millisecond
		if !earliest.IsZero() {
			d := time.Until(earliest)
			if d > 0 {
				if d > 1*time.Second {
					sleepDur = 1 * time.Second
				} else {
					sleepDur = d
				}
			} else {
				sleepDur = 5 * time.Millisecond
			}
		}

		timer := time.NewTimer(sleepDur)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.stopCh:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// Entries returns a point-in-time snapshot of all registered cron entries.
func (s *Scheduler) Entries() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Entry, len(s.entries))
	for i, e := range s.entries {
		result[i] = *e
	}

	return result
}
