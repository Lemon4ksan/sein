// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package loadbalance_test

import (
	"testing"
	"time"

	"github.com/lemon4ksan/sein/async/loadbalance"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

func TestBalancer_Constructors_And_TargetMetrics(t *testing.T) {
	t.Parallel()

	// 1. Target with weight < 1 defaults to 1
	t1 := loadbalance.NewTarget("srv1", -5)
	assert.Equal(t, 1, t1.Weight)
	assert.Equal(t, int64(0), t1.ActiveConns())

	// 2. Acquire and Release
	t1.Acquire()
	assert.Equal(t, int64(1), t1.ActiveConns())
	t1.Release()
	assert.Equal(t, int64(0), t1.ActiveConns())

	// 3. RecordSuccess & RecordFailure with default maxFails
	assert.True(t, t1.IsHealthy(time.Second))
	t1.RecordFailure(0) // 1 fail (maxFails defaults to 3)
	assert.True(t, t1.IsHealthy(time.Second))
	t1.RecordFailure(0) // 2 fails
	assert.True(t, t1.IsHealthy(time.Second))
	t1.RecordFailure(0) // 3 fails -> unhealthy
	assert.False(t, t1.IsHealthy(time.Second))
	assert.False(t, t1.IsHealthy(0)) // cooldown <= 0 returns false when unhealthy

	t1.RecordSuccess()
	assert.True(t, t1.IsHealthy(time.Second))

	// 4. Balancer New errors
	_, err := loadbalance.New[string](loadbalance.RoundRobin, time.Second)
	assert.ErrorIs(t, err, loadbalance.ErrNoTargets)

	// Balancer New with cooldown <= 0
	lbDefaultCool, err := loadbalance.New(loadbalance.RoundRobin, 0, t1)
	require.NoError(t, err)
	assert.NotNil(t, lbDefaultCool)
}

func TestBalancer_AllStrategies(t *testing.T) {
	t.Parallel()

	t1 := loadbalance.NewTarget("srv1", 10)
	t2 := loadbalance.NewTarget("srv2", 20)
	t3 := loadbalance.NewTarget("srv3", 30)

	// 1. Random
	lbRandom, err := loadbalance.New(loadbalance.Random, time.Second, t1, t2, t3)
	require.NoError(t, err)
	for range 10 {
		target, err := lbRandom.Select()
		require.NoError(t, err)
		assert.NotEmpty(t, target.Value)
	}

	// 2. Weighted
	lbWeighted, err := loadbalance.New(loadbalance.Weighted, time.Second, t1, t2, t3)
	require.NoError(t, err)
	for range 10 {
		target, err := lbWeighted.Select()
		require.NoError(t, err)
		assert.NotEmpty(t, target.Value)
	}

	// 3. LeastConn
	t1.Acquire()
	t1.Acquire() // 2 conns
	t2.Acquire() // 1 conn
	t3.Acquire()
	t3.Acquire()
	t3.Acquire() // 3 conns

	lbLeast, err := loadbalance.New(loadbalance.LeastConn, time.Second, t1, t2, t3)
	require.NoError(t, err)
	leastTarget, err := lbLeast.Select()
	require.NoError(t, err)
	assert.Equal(t, "srv2", leastTarget.Value)

	// 4. PeakEWMA
	lbPeak, err := loadbalance.New(loadbalance.PeakEWMA, time.Second, t1, t2)
	require.NoError(t, err)
	peakTarget, err := lbPeak.Select()
	require.NoError(t, err)
	assert.NotEmpty(t, peakTarget.Value)

	// PeakEWMA with single target
	lbSingle, err := loadbalance.New(loadbalance.PeakEWMA, time.Second, t1)
	require.NoError(t, err)
	singleTarget, err := lbSingle.Select()
	require.NoError(t, err)
	assert.Equal(t, "srv1", singleTarget.Value)

	// 5. RoundRobin
	lbRR, err := loadbalance.New(loadbalance.RoundRobin, time.Second, t1, t2)
	require.NoError(t, err)
	r1, _ := lbRR.Select()
	r2, _ := lbRR.Select()
	assert.Equal(t, "srv1", r1.Value)
	assert.Equal(t, "srv2", r2.Value)
}

func TestBalancer_Add_Remove_And_NoHealthy(t *testing.T) {
	t.Parallel()

	t1 := loadbalance.NewTarget("srv1", 1)
	t2 := loadbalance.NewTarget("srv2", 1)

	lb, err := loadbalance.New(loadbalance.RoundRobin, time.Second, t1)
	require.NoError(t, err)

	// Add nil and valid
	lb.Add(nil)
	lb.Add(t2)

	// Remove
	lb.Remove("srv1", func(a, b string) bool { return a == b })

	target, err := lb.Select()
	require.NoError(t, err)
	assert.Equal(t, "srv2", target.Value)

	// Make t2 unhealthy
	t2.RecordFailure(1)
	_, err = lb.Select()
	assert.ErrorIs(t, err, loadbalance.ErrNoHealthyTargets)

	// Remove all
	lb.Remove("srv2", func(a, b string) bool { return a == b })
	_, err = lb.Select()
	assert.ErrorIs(t, err, loadbalance.ErrNoTargets)
}
