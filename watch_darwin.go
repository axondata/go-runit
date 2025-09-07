//go:build darwin

package runit

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch monitors the service's status file for changes and sends events.
// It uses fsnotify to detect writes to the status file and applies debouncing
// to coalesce rapid changes. The returned channel receives status updates,
// and the stop function cleanly terminates the watcher.
func (c *Client) Watch(ctx context.Context) (<-chan WatchEvent, func() error, error) {
	superviseDir := filepath.Join(c.ServiceDir, SuperviseDir)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, &OpError{Op: OpStatus, Path: superviseDir, Err: err}
	}

	if err := watcher.Add(superviseDir); err != nil {
		_ = watcher.Close()
		return nil, nil, &OpError{Op: OpStatus, Path: superviseDir, Err: err}
	}

	ch := make(chan WatchEvent, 10)

	var (
		mu        sync.Mutex
		closed    bool
		lastRaw   [StatusFileSize]byte
		debouncer *time.Timer
	)

	stop := func() error {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return nil
		}
		closed = true
		if debouncer != nil {
			debouncer.Stop()
		}
		err := watcher.Close()
		close(ch)
		return err
	}

	readAndSend := func() {
		mu.Lock()
		if closed {
			mu.Unlock()
			return
		}
		mu.Unlock()

		status, err := c.Status(context.Background())
		if err != nil {
			mu.Lock()
			if closed {
				mu.Unlock()
				return
			}
			// Keep the lock while sending to prevent races with stop()
			select {
			case ch <- WatchEvent{Err: err}:
				mu.Unlock()
			case <-ctx.Done():
				mu.Unlock()
			}
			return
		}

		if len(status.Raw) == StatusFileSize {
			var currentRaw [StatusFileSize]byte
			copy(currentRaw[:], status.Raw)

			mu.Lock()
			if closed {
				mu.Unlock()
				return
			}
			if currentRaw != lastRaw {
				lastRaw = currentRaw
				// Keep the lock while sending to prevent races with stop()
				select {
				case ch <- WatchEvent{Status: status}:
					mu.Unlock()
				case <-ctx.Done():
					mu.Unlock()
				}
			} else {
				mu.Unlock()
			}
		}
	}

	readAndSend()

	go func() {
		defer func() { _ = stop() }()

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if filepath.Base(event.Name) == StatusFile && (event.Op&fsnotify.Write != 0 || event.Op&fsnotify.Create != 0) {
					mu.Lock()
					if debouncer != nil {
						debouncer.Stop()
					}
					debouncer = time.AfterFunc(c.WatchDebounce, readAndSend)
					mu.Unlock()
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				select {
				case ch <- WatchEvent{Err: err}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, stop, nil
}
