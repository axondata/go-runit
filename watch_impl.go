//go:build linux || darwin

package svcmgr

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"vawter.tech/stopper"
)

// watchClient is an interface for client-specific Watch operations
type watchClient interface {
	ServiceClient
	getServiceDir() string
	getStatusFileSize() int
}

// watchState manages the state of a watch operation
type watchState struct {
	mu              sync.Mutex
	lastRaw         []byte
	debouncer       *time.Timer
	spinStartTime   time.Time
	spinCount       int
	backoffInterval time.Duration
}

// watchImpl provides a common implementation for Watch across all client types
//
//nolint:gocyclo // Complex state management required for robust watch functionality
func watchImpl(ctx context.Context, client watchClient) (<-chan WatchEvent, WatchCleanupFunc, error) {
	superviseDir := filepath.Join(client.getServiceDir(), SuperviseDir)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, &OpError{Op: OpStatus, Path: superviseDir, Err: err}
	}

	if err := watcher.Add(superviseDir); err != nil {
		_ = watcher.Close()
		return nil, nil, &OpError{Op: OpStatus, Path: superviseDir, Err: err}
	}

	ch := make(chan WatchEvent, 10)

	// Create stopper context for managing goroutine lifecycle
	sctx := stopper.WithContext(ctx)

	// Register watcher cleanup with stopper
	sctx.Defer(func() {
		_ = watcher.Close()
		close(ch)
	})

	state := &watchState{
		lastRaw: make([]byte, client.getStatusFileSize()),
	}

	// Create cleanup function using stopper
	cleanup := func() error {
		sctx.Stop(100 * time.Millisecond) // Graceful stop with 100ms grace period
		return sctx.Wait()
	}

	// Read and send status updates
	readAndSend := func() {
		// Check if we're stopping
		if sctx.IsStopping() {
			return
		}

		// Use the original context for the Status call
		status, err := client.Status(ctx)
		if err != nil {
			if !sctx.IsStopping() {
				select {
				case ch <- WatchEvent{Err: err}:
				case <-sctx.Stopping():
				}
			}
			return
		}

		// Check if status changed
		currentRaw := make([]byte, len(state.lastRaw))
		copy(currentRaw, status.Raw[:])

		state.mu.Lock()
		defer state.mu.Unlock()

		changed := false
		if len(currentRaw) != len(state.lastRaw) {
			changed = true
		} else {
			for i := range currentRaw {
				if currentRaw[i] != state.lastRaw[i] {
					changed = true
					break
				}
			}
		}

		if changed {
			state.lastRaw = currentRaw

			// Reset spin detection on successful change
			state.spinCount = 0
			state.spinStartTime = time.Time{}
			state.backoffInterval = 0

			if !sctx.IsStopping() {
				select {
				case ch <- WatchEvent{Status: status}:
				case <-sctx.Stopping():
				}
			}
		} else {
			// Track spinning behavior
			now := time.Now()
			if state.spinStartTime.IsZero() {
				state.spinStartTime = now
				state.spinCount = 1
			} else {
				state.spinCount++

				// If we've been spinning for >= 5 seconds, enter backoff mode
				if now.Sub(state.spinStartTime) >= 5*time.Second && state.backoffInterval == 0 {
					state.backoffInterval = time.Second
				}
			}
		}
	}

	// Initial read
	readAndSend()

	// Launch watcher goroutine using stopper
	sctx.Go(func(sctx *stopper.Context) error {
		// Register debouncer cleanup
		sctx.Defer(func() {
			state.mu.Lock()
			if state.debouncer != nil {
				state.debouncer.Stop()
			}
			state.mu.Unlock()
		})

		for !sctx.IsStopping() {
			select {
			case <-sctx.Stopping():
				return nil

			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}

				if filepath.Base(event.Name) == StatusFile {
					state.mu.Lock()

					// If in backoff mode, use longer debounce
					debounceTime := 10 * time.Millisecond
					if state.backoffInterval > 0 {
						debounceTime = state.backoffInterval
					}

					// Cancel existing debouncer
					if state.debouncer != nil {
						state.debouncer.Stop()
					}
					state.debouncer = time.AfterFunc(debounceTime, readAndSend)
					state.mu.Unlock()
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
				if err != nil && !sctx.IsStopping() {
					select {
					case ch <- WatchEvent{Err: err}:
					case <-sctx.Stopping():
						return nil
					}
				}
			}
		}
		return nil
	})

	return ch, cleanup, nil
}

// Adapter implementations for each client type

func (c *ClientRunit) getServiceDir() string {
	return c.ServiceDir
}

func (c *ClientRunit) getStatusFileSize() int {
	return StatusFileSize
}

func (c *ClientDaemontools) getServiceDir() string {
	return c.ServiceDir
}

func (c *ClientDaemontools) getStatusFileSize() int {
	return DaemontoolsStatusSize
}

func (c *ClientS6) getServiceDir() string {
	return c.ServiceDir
}

func (c *ClientS6) getStatusFileSize() int {
	return S6MaxStatusSize
}
