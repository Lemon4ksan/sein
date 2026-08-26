// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package prefork provides high-throughput multi-process clustering utilizing SO_REUSEPORT.
// It eliminates Go runtime netpoller lock contention across multi-core systems by pinning
// dedicated event loops to individual CPU cores with zero inter-process mutex overhead.
package prefork

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/lemon4ksan/sein"
)

// EnvPreforkChild is the environment variable key indicating child worker execution.
const EnvPreforkChild = "SEIN_PREFORK_CHILD"

// Config configures multi-process prefork clustering.
type Config struct {
	// Workers is the number of child worker processes to spawn. Default is runtime.GOMAXPROCS(0).
	Workers int
	// RestartDelay is the backoff duration before restarting a failed child worker. Default is 250ms.
	RestartDelay time.Duration
	// GracefulTimeout is the maximum duration to wait for child workers to exit during shutdown. Default is 5s.
	GracefulTimeout time.Duration
}

// Option configures prefork settings.
type Option func(*Config)

// WithWorkers sets the number of spawned worker processes.
func WithWorkers(n int) Option {
	return func(c *Config) {
		c.Workers = n
	}
}

// WithRestartDelay sets the backoff duration before restarting a terminated child worker.
func WithRestartDelay(d time.Duration) Option {
	return func(c *Config) {
		c.RestartDelay = d
	}
}

// WithGracefulTimeout sets the maximum duration to wait for workers to shutdown cleanly.
func WithGracefulTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.GracefulTimeout = d
	}
}

// IsChild reports whether the current process is running as a preforked worker child.
func IsChild() bool {
	return os.Getenv(EnvPreforkChild) == "1"
}

// Run executes the server in multi-process prefork mode.
// If running as a child process, it opens a SO_REUSEPORT listener and serves incoming connections.
// If running as the master process, it spawns and supervises N worker child processes.
func Run(app *sein.Server, addr string, opts ...Option) error {
	cfg := Config{
		Workers:         runtime.GOMAXPROCS(0),
		RestartDelay:    250 * time.Millisecond,
		GracefulTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU()
	}

	if IsChild() {
		ln, err := Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("prefork child: failed to listen on %s: %w", addr, err)
		}

		return app.Serve(ln)
	}

	return runMaster(addr, cfg)
}

type workerProcess struct {
	cmd *exec.Cmd
	idx int
}

func runMaster(addr string, cfg Config) error {
	// First test that the address is bindable to fail early on invalid ports
	testLn, err := Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("prefork master: cannot bind to %s: %w", addr, err)
	}
	_ = testLn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu         sync.Mutex
		workers    = make(map[int]*workerProcess)
		shutdown   = false
		shutdownCh = make(chan struct{})
	)

	spawnWorker := func(idx int) *workerProcess {
		execPath, err := os.Executable()
		if err != nil {
			execPath = os.Args[0]
		}

		// #nosec G204,G702 -- prefork spawns worker copies of the current executable
		cmd := exec.CommandContext(ctx, execPath, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = append(os.Environ(), EnvPreforkChild+"=1")

		wp := &workerProcess{cmd: cmd, idx: idx}

		if err := cmd.Start(); err != nil {
			return nil
		}

		return wp
	}

	// Spawn initial worker pool
	mu.Lock()
	for i := 0; i < cfg.Workers; i++ {
		if wp := spawnWorker(i); wp != nil {
			workers[i] = wp
		}
	}
	mu.Unlock()

	// Watch and supervise child workers
	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		workerIdx := i

		wg.Go(func() {
			for {
				mu.Lock()
				wp := workers[workerIdx]
				mu.Unlock()

				if wp != nil && wp.cmd != nil && wp.cmd.Process != nil {
					_ = wp.cmd.Wait()
				}

				mu.Lock()
				isShuttingDown := shutdown
				mu.Unlock()

				if isShuttingDown {
					return
				}

				// Automatic worker revival
				time.Sleep(cfg.RestartDelay)

				mu.Lock()
				if !shutdown {
					if newWp := spawnWorker(workerIdx); newWp != nil {
						workers[workerIdx] = newWp
					}
				}
				mu.Unlock()
			}
		})
	}

	// Wait for OS termination signal
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	select {
	case <-sigCh:
	case <-shutdownCh:
	}

	mu.Lock()
	shutdown = true
	// Terminate all child processes
	for _, wp := range workers {
		if wp != nil && wp.cmd != nil && wp.cmd.Process != nil {
			_ = wp.cmd.Process.Signal(os.Interrupt)
		}
	}
	mu.Unlock()

	// Wait for workers to exit with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(cfg.GracefulTimeout):
		mu.Lock()
		for _, wp := range workers {
			if wp != nil && wp.cmd != nil && wp.cmd.Process != nil {
				_ = wp.cmd.Process.Kill()
			}
		}
		mu.Unlock()
	}

	return nil
}
