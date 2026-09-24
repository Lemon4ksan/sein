// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package loadbalance

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/silicon/randkit"
)

var (
	// ErrNoTargets is returned when a load balancer has zero registered targets.
	ErrNoTargets = errors.New("loadbalance: no targets registered")

	// ErrNoHealthyTargets is returned when all registered targets are unhealthy or in cooldown.
	ErrNoHealthyTargets = errors.New("loadbalance: no healthy targets available")
)

// Strategy defines the target selection algorithm.
type Strategy uint8

const (
	// RoundRobin selects targets sequentially in cyclic order.
	RoundRobin Strategy = iota
	// Random selects targets pseudo-randomly.
	Random
	// Weighted selects targets proportionally according to assigned integer weights.
	Weighted
	// LeastConn selects targets with the fewest active in-flight requests.
	LeastConn
	// PeakEWMA selects using Power of Two Choices (P2C) biased by latency and active connections.
	PeakEWMA
)

// Target wraps an arbitrary endpoint T with health and load metrics.
type Target[T any] struct {
	Value       T
	Weight      int
	activeConns atomic.Int64
	failCount   atomic.Uint32
	unhealthy   atomic.Bool
	lastFailed  atomic.Int64 // UnixNano
}

// NewTarget constructs a new [Target] wrapper with weight >= 1.
func NewTarget[T any](val T, weight int) *Target[T] {
	if weight < 1 {
		weight = 1
	}

	return &Target[T]{
		Value:  val,
		Weight: weight,
	}
}

// IsHealthy reports whether target is available and not in cooldown.
func (t *Target[T]) IsHealthy(cooldown time.Duration) bool {
	if !t.unhealthy.Load() {
		return true
	}

	if cooldown <= 0 {
		return false
	}

	last := t.lastFailed.Load()
	if last == 0 {
		return false
	}

	return time.Since(time.Unix(0, last)) >= cooldown
}

// RecordSuccess records a successful execution on target.
func (t *Target[T]) RecordSuccess() {
	t.failCount.Store(0)
	t.unhealthy.Store(false)
}

// RecordFailure records a failure and marks target unhealthy if threshold is exceeded.
func (t *Target[T]) RecordFailure(maxFails uint32) {
	if maxFails == 0 {
		maxFails = 3
	}

	fails := t.failCount.Add(1)
	if fails >= maxFails {
		t.unhealthy.Store(true)
		t.lastFailed.Store(time.Now().UnixNano())
	}
}

// ActiveConns returns current in-flight concurrent requests.
func (t *Target[T]) ActiveConns() int64 {
	return t.activeConns.Load()
}

// Acquire increments in-flight connection counter.
func (t *Target[T]) Acquire() {
	t.activeConns.Add(1)
}

// Release decrements in-flight connection counter.
func (t *Target[T]) Release() {
	t.activeConns.Add(-1)
}

// Balancer distributes work items across a collection of generic [Target]s.
type Balancer[T any] struct {
	mu       sync.RWMutex
	targets  []*Target[T]
	strategy Strategy
	cooldown time.Duration
	counter  atomic.Uint64
}

// New creates a new generic [Balancer].
func New[T any](strategy Strategy, cooldown time.Duration, targets ...*Target[T]) (*Balancer[T], error) {
	if len(targets) == 0 {
		return nil, ErrNoTargets
	}

	if cooldown <= 0 {
		cooldown = 10 * time.Second
	}

	return &Balancer[T]{
		targets:  targets,
		strategy: strategy,
		cooldown: cooldown,
	}, nil
}

// Select picks the best available [Target] according to configured strategy.
func (b *Balancer[T]) Select() (*Target[T], error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	n := len(b.targets)
	if n == 0 {
		return nil, ErrNoTargets
	}

	healthy := make([]*Target[T], 0, n)
	for _, target := range b.targets {
		if target.IsHealthy(b.cooldown) {
			healthy = append(healthy, target)
		}
	}

	if len(healthy) == 0 {
		return nil, ErrNoHealthyTargets
	}

	switch b.strategy {
	case Random:
		return healthy[randkit.Intn(len(healthy))], nil

	case Weighted:
		totalWeight := 0
		for _, target := range healthy {
			totalWeight += target.Weight
		}

		if totalWeight <= 0 {
			return healthy[0], nil
		}

		r := randkit.Intn(totalWeight)
		acc := 0
		for _, target := range healthy {
			acc += target.Weight
			if r < acc {
				return target, nil
			}
		}

		return healthy[0], nil

	case LeastConn:
		best := healthy[0]
		minConns := best.ActiveConns()

		for _, target := range healthy[1:] {
			conns := target.ActiveConns()
			if conns < minConns {
				minConns = conns
				best = target
			}
		}

		return best, nil

	case PeakEWMA:
		if len(healthy) == 1 {
			return healthy[0], nil
		}

		// Power of Two Choices (P2C)
		i := randkit.Intn(len(healthy))
		j := randkit.Intn(len(healthy) - 1)
		if j >= i {
			j++
		}

		t1 := healthy[i]
		t2 := healthy[j]

		score1 := t1.ActiveConns() + 1
		score2 := t2.ActiveConns() + 1

		if score1 <= score2 {
			return t1, nil
		}

		return t2, nil

	default: // RoundRobin
		idx := b.counter.Add(1) - 1
		return healthy[idx%uint64(len(healthy))], nil
	}
}

// Add adds a new target to the balancer.
func (b *Balancer[T]) Add(target *Target[T]) {
	if target == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.targets = append(b.targets, target)
}

// Remove removes target from balancer if found.
func (b *Balancer[T]) Remove(val T, equals func(a, b T) bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.targets = slices.DeleteFunc(b.targets, func(t *Target[T]) bool {
		return equals(t.Value, val)
	})
}
