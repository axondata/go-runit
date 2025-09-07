package runit

import (
	"context"
	"sync"
	"time"
)

// Manager handles operations on multiple runit services concurrently.
// It provides bulk operations with configurable concurrency and timeouts.
type Manager struct {
	// Concurrency is the maximum number of concurrent operations
	Concurrency int
	// Timeout is the per-operation timeout
	Timeout time.Duration
}

// ManagerOption configures a Manager
type ManagerOption func(*Manager)

// WithConcurrency sets the maximum number of concurrent operations
func WithConcurrency(n int) ManagerOption {
	return func(m *Manager) {
		m.Concurrency = n
	}
}

// WithTimeout sets the per-operation timeout
func WithTimeout(d time.Duration) ManagerOption {
	return func(m *Manager) {
		m.Timeout = d
	}
}

// NewManager creates a new Manager with default settings
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		Concurrency: 10,
		Timeout:     5 * time.Second,
	}

	for _, opt := range opts {
		opt(m)
	}

	if m.Concurrency < 1 {
		m.Concurrency = 1
	}

	return m
}

func (m *Manager) execute(ctx context.Context, services []string, op func(context.Context, *Client) error) error {
	if len(services) == 0 {
		return nil
	}

	sem := make(chan struct{}, m.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	merr := &MultiError{}

	for _, service := range services {
		wg.Add(1)
		go func(svc string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			client, err := New(svc)
			if err != nil {
				mu.Lock()
				merr.Add(&OpError{Op: OpUnknown, Path: svc, Err: err})
				mu.Unlock()
				return
			}

			opCtx := ctx
			if m.Timeout > 0 {
				var cancel context.CancelFunc
				opCtx, cancel = context.WithTimeout(ctx, m.Timeout)
				defer cancel()
			}

			if err := op(opCtx, client); err != nil {
				mu.Lock()
				merr.Add(err)
				mu.Unlock()
			}
		}(service)
	}

	wg.Wait()
	return merr.Err()
}

// Up starts the specified services
func (m *Manager) Up(ctx context.Context, services ...string) error {
	return m.execute(ctx, services, func(ctx context.Context, c *Client) error {
		return c.Up(ctx)
	})
}

// Down stops the specified services
func (m *Manager) Down(ctx context.Context, services ...string) error {
	return m.execute(ctx, services, func(ctx context.Context, c *Client) error {
		return c.Down(ctx)
	})
}

// Term sends SIGTERM to the specified services
func (m *Manager) Term(ctx context.Context, services ...string) error {
	return m.execute(ctx, services, func(ctx context.Context, c *Client) error {
		return c.Term(ctx)
	})
}

// Kill sends SIGKILL to the specified services
func (m *Manager) Kill(ctx context.Context, services ...string) error {
	return m.execute(ctx, services, func(ctx context.Context, c *Client) error {
		return c.Kill(ctx)
	})
}

// Status retrieves the status of the specified services
func (m *Manager) Status(ctx context.Context, services ...string) (map[string]Status, error) {
	if len(services) == 0 {
		return make(map[string]Status), nil
	}

	sem := make(chan struct{}, m.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[string]Status)
	merr := &MultiError{}

	for _, service := range services {
		wg.Add(1)
		go func(svc string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			client, err := New(svc)
			if err != nil {
				mu.Lock()
				merr.Add(&OpError{Op: OpStatus, Path: svc, Err: err})
				mu.Unlock()
				return
			}

			opCtx := ctx
			if m.Timeout > 0 {
				var cancel context.CancelFunc
				opCtx, cancel = context.WithTimeout(ctx, m.Timeout)
				defer cancel()
			}

			status, err := client.Status(opCtx)
			if err != nil {
				mu.Lock()
				merr.Add(err)
				mu.Unlock()
				return
			}

			mu.Lock()
			results[svc] = status
			mu.Unlock()
		}(service)
	}

	wg.Wait()
	return results, merr.Err()
}
