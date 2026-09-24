// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package loadbalance provides high-performance, generic load balancing algorithms
// with dynamic health tracking and concurrency metrics for distributed backends.
//
// # Architecture
//
// The core orchestrator is [Balancer], parameterized over any endpoint type T. Backends are wrapped
// in [Target], which maintains atomic metrics including active in-flight requests, failure counters,
// and health cooldown timestamps. The package provides five standard distribution strategies ([Strategy]):
//   - [RoundRobin]: Sequential cyclic distribution utilizing lock-free atomic counters.
//   - [Random]: Uniform pseudo-random distribution.
//   - [Weighted]: Proportional traffic distribution according to assigned target weights.
//   - [LeastConn]: Dispatches requests to the target with the fewest active in-flight connections.
//   - [PeakEWMA]: Two-choice randomized selection (Power of Two Choices) biased by in-flight load.
//
// # Dynamic Health Checks & Failure Isolation
//
// When a backend encounters errors, callers invoke [Target.RecordFailure]. Upon reaching a threshold,
// the target transitions to an unhealthy state and is temporarily excised from routing candidate sets.
// After the configured cooldown window elapses, the target becomes eligible for trial requests.
// Successful executions ([Target.RecordSuccess]) reset failure counters and restore healthy status.
//
// # Concurrency Guarantees
//
//   - [Balancer] protects target membership using a [sync.RWMutex]. Selecting targets via [Balancer.Select]
//     acquires a shared read lock and is fully safe for high-frequency concurrent execution.
//   - Dynamic target registration ([Balancer.AddTarget], [Balancer.RemoveTarget]) acquires an exclusive lock.
//   - Target metrics ([Target.Acquire], [Target.Release], [Target.RecordSuccess], [Target.RecordFailure])
//     rely exclusively on atomic operations, ensuring zero mutex contention during hotpath request processing.
//
// # Example
//
//	package main
//
//	import (
//		"fmt"
//		"time"
//
//		"github.com/lemon4ksan/sein/async/loadbalance"
//	)
//
//	func main() {
//		t1 := loadbalance.NewTarget("http://srv-1.internal", 3)
//		t2 := loadbalance.NewTarget("http://srv-2.internal", 1)
//
//		balancer, err := loadbalance.New(loadbalance.RoundRobin, 5*time.Second, t1, t2)
//		if err != nil {
//			panic(err)
//		}
//
//		target, err := balancer.Select()
//		if err != nil {
//			panic(err)
//		}
//
//		target.Acquire()
//		defer target.Release()
//
//		fmt.Printf("Selected backend: %s\n", target.Value)
//		target.RecordSuccess()
//	}
package loadbalance
