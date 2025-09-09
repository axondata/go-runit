//go:build linux || darwin

package runit

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
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
	closed          bool
	lastRaw         []byte
	debouncer       *time.Timer
	spinStartTime   time.Time
	spinCount       int
	backoffInterval time.Duration
}

// watchImpl provides a common implementation for Watch across all client types
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
	
	state := &watchState{
		lastRaw: make([]byte, client.getStatusFileSize()),
	}

	// Create cleanup function
	cleanup := func() error {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.closed {
			return nil
		}
		state.closed = true
		if state.debouncer != nil {
			state.debouncer.Stop()
		}
		err := watcher.Close()
		close(ch)
		return err
	}

	// Read and send status updates
	readAndSend := func() {
		state.mu.Lock()
		if state.closed {
			state.mu.Unlock()
			return
		}
		state.mu.Unlock()

		// Use the provided context, not a background context
		status, err := client.Status(ctx)
		if err != nil {
			state.mu.Lock()
			if state.closed {
				state.mu.Unlock()
				return
			}
			select {
			case ch <- WatchEvent{Err: err}:
				state.mu.Unlock()
			case <-ctx.Done():
				state.mu.Unlock()
			}
			return
		}

		// Check if status changed
		currentRaw := make([]byte, len(state.lastRaw))
		copy(currentRaw, status.Raw[:])

		state.mu.Lock()
		if state.closed {
			state.mu.Unlock()
			return
		}

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
			
			select {
			case ch <- WatchEvent{Status: status}:
				state.mu.Unlock()
			case <-ctx.Done():
				state.mu.Unlock()
				return
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
			state.mu.Unlock()
		}
	}

	// Initial read
	readAndSend()

	// Launch watcher goroutine
	go func() {
		defer func() { _ = cleanup() }()

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if filepath.Base(event.Name) == "status" {
					state.mu.Lock()
					if state.closed {
						state.mu.Unlock()
						return
					}

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
					return
				}
				if err != nil {
					state.mu.Lock()
					if state.closed {
						state.mu.Unlock()
						return
					}
					select {
					case ch <- WatchEvent{Err: err}:
						state.mu.Unlock()
					case <-ctx.Done():
						state.mu.Unlock()
						return
					}
				}
			}
		}
	}()

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