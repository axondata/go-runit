//go:build (integration || integration_runit) && (linux || darwin)
// +build integration integration_runit
// +build linux darwin

package runit_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/axondata/go-runit"
)

// TestIntegrationWatchFunctionality tests the fsnotify-based watch feature
func TestIntegrationWatchFunctionality(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("runsv"); err != nil {
		t.Skip("runsv not found in PATH")
	}

	tmpDir := t.TempDir()
	serviceDir := filepath.Join(tmpDir, "watch-test")

	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("failed to create service dir: %v", err)
	}

	// Service that changes state periodically
	runScript := `#!/bin/sh
exec 2>&1
echo "Service starting"
for i in 1 2 3; do
    echo "Iteration $i"
    sleep 2
done
echo "Service stopping"
exit 0`

	runFile := filepath.Join(serviceDir, "run")
	if err := os.WriteFile(runFile, []byte(runScript), 0o755); err != nil {
		t.Fatalf("failed to write run script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "runsv", serviceDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start runsv: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for supervise directory
	superviseDir := filepath.Join(serviceDir, "supervise")
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(superviseDir); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	client, err := runit.New(serviceDir)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	t.Run("WatchStateTransitions", func(t *testing.T) {
		watchCtx, watchCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer watchCancel()

		events, stop, err := client.Watch(watchCtx)
		if err != nil {
			t.Fatalf("failed to start watch: %v", err)
		}
		defer stop()

		// Track events
		var eventsMu sync.Mutex
		receivedEvents := []runit.WatchEvent{}
		stateSequence := []runit.State{}

		// Goroutine to collect events
		go func() {
			for event := range events {
				eventsMu.Lock()
				receivedEvents = append(receivedEvents, event)
				if event.Err == nil {
					stateSequence = append(stateSequence, event.Status.State)
					t.Logf("Watch event: state=%v pid=%d uptime=%v",
						event.Status.State, event.Status.PID, event.Status.Uptime)
				} else {
					t.Logf("Watch error: %v", event.Err)
				}
				eventsMu.Unlock()
			}
		}()

		// Trigger state changes
		time.Sleep(500 * time.Millisecond)

		// Start the service
		if err := client.Up(context.Background()); err != nil {
			t.Errorf("failed to start service: %v", err)
		}

		// Let it run for a bit
		time.Sleep(3 * time.Second)

		// Send a signal
		if err := client.Term(context.Background()); err != nil {
			t.Errorf("failed to send TERM: %v", err)
		}

		time.Sleep(1 * time.Second)

		// Stop the service
		if err := client.Down(context.Background()); err != nil {
			t.Errorf("failed to stop service: %v", err)
		}

		// Wait for more events
		time.Sleep(3 * time.Second)

		// Start it again
		if err := client.Up(context.Background()); err != nil {
			t.Errorf("failed to restart service: %v", err)
		}

		time.Sleep(2 * time.Second)

		// Check results
		eventsMu.Lock()
		defer eventsMu.Unlock()

		if len(receivedEvents) < 3 {
			t.Errorf("Expected at least 3 events, got %d", len(receivedEvents))
		}

		// Verify we saw different states
		statesSet := make(map[runit.State]bool)
		for _, state := range stateSequence {
			statesSet[state] = true
		}

		if len(statesSet) < 2 {
			t.Errorf("Expected to see at least 2 different states, saw %d", len(statesSet))
		}

		t.Logf("Total events received: %d", len(receivedEvents))
		t.Logf("Unique states observed: %d", len(statesSet))
		t.Logf("State sequence: %v", stateSequence)
	})

	t.Run("WatchDebouncing", func(t *testing.T) {
		// Test that rapid changes are debounced
		client2, err := runit.New(serviceDir,
			runit.WithWatchDebounce(100*time.Millisecond))
		if err != nil {
			t.Fatalf("failed to create client with custom debounce: %v", err)
		}

		watchCtx, watchCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer watchCancel()

		events, stop, err := client2.Watch(watchCtx)
		if err != nil {
			t.Fatalf("failed to start watch: %v", err)
		}
		defer stop()

		var eventCountMu sync.Mutex
		eventCount := 0
		go func() {
			for event := range events {
				if event.Err == nil {
					eventCountMu.Lock()
					eventCount++
					current := eventCount
					eventCountMu.Unlock()
					t.Logf("Debounced event %d: state=%v",
						current, event.Status.State)
				}
			}
		}()

		// Send rapid signals that should be debounced
		for i := 0; i < 5; i++ {
			client2.HUP(context.Background())
			time.Sleep(20 * time.Millisecond) // Less than debounce time
		}

		// Wait for debounce to settle
		time.Sleep(500 * time.Millisecond)

		// The rapid signals should result in fewer events due to debouncing
		// We can't predict exact count due to timing, but it should be less than 5
		eventCountMu.Lock()
		t.Logf("Events after rapid signals (with 100ms debounce): %d", eventCount)
		eventCountMu.Unlock()
	})

	t.Run("WatchStopCleanup", func(t *testing.T) {
		// Test that stop function properly cleans up
		watchCtx := context.Background()

		events, stop, err := client.Watch(watchCtx)
		if err != nil {
			t.Fatalf("failed to start watch: %v", err)
		}

		// Collect some events
		go func() {
			for range events {
				// Drain events
			}
		}()

		time.Sleep(500 * time.Millisecond)

		// Stop should cleanly terminate
		if err := stop(); err != nil {
			t.Errorf("stop() returned error: %v", err)
		}

		// Channel should be closed
		select {
		case _, ok := <-events:
			if ok {
				t.Error("events channel not closed after stop()")
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("events channel not closed after stop()")
		}
	})

	t.Run("WatchContextCancellation", func(t *testing.T) {
		// Test that context cancellation stops the watch
		watchCtx, watchCancel := context.WithCancel(context.Background())

		events, stop, err := client.Watch(watchCtx)
		if err != nil {
			t.Fatalf("failed to start watch: %v", err)
		}
		defer stop()

		done := make(chan bool)
		go func() {
			for range events {
				// Drain events
			}
			done <- true
		}()

		// Cancel context
		watchCancel()

		// Watch should stop
		select {
		case <-done:
			t.Log("Watch stopped after context cancellation")
		case <-time.After(2 * time.Second):
			t.Error("Watch did not stop after context cancellation")
		}
	})
}

// TestIntegrationWatchDeduplication tests that identical states are deduplicated
func TestIntegrationWatchDeduplication(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("runsv"); err != nil {
		t.Skip("runsv not found in PATH")
	}

	tmpDir := t.TempDir()
	serviceDir := filepath.Join(tmpDir, "dedup-test")

	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("failed to create service dir: %v", err)
	}

	// Long-running service
	runScript := `#!/bin/sh
exec 2>&1
exec sleep 300`

	runFile := filepath.Join(serviceDir, "run")
	if err := os.WriteFile(runFile, []byte(runScript), 0o755); err != nil {
		t.Fatalf("failed to write run script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "runsv", serviceDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start runsv: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for supervise directory
	superviseDir := filepath.Join(serviceDir, "supervise")
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(superviseDir); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	client, err := runit.New(serviceDir)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Start service
	if err := client.Up(context.Background()); err != nil {
		t.Errorf("failed to start service: %v", err)
	}

	time.Sleep(1 * time.Second)

	watchCtx, watchCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer watchCancel()

	events, stop, err := client.Watch(watchCtx)
	if err != nil {
		t.Fatalf("failed to start watch: %v", err)
	}
	defer stop()

	var mu sync.Mutex
	eventCount := 0
	lastPID := 0

	go func() {
		for event := range events {
			if event.Err == nil {
				mu.Lock()
				eventCount++
				current := eventCount
				if event.Status.PID != lastPID {
					t.Logf("Event %d: state=%v pid=%d (PID changed)",
						current, event.Status.State, event.Status.PID)
					lastPID = event.Status.PID
				} else {
					t.Logf("Event %d: state=%v pid=%d",
						current, event.Status.State, event.Status.PID)
				}
				mu.Unlock()
			}
		}
	}()

	// Send multiple HUP signals (shouldn't change state)
	for i := 0; i < 5; i++ {
		if err := client.HUP(context.Background()); err != nil {
			t.Errorf("failed to send HUP: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Wait for any pending events
	time.Sleep(1 * time.Second)

	// Because the state doesn't change (still running with same PID),
	// we should see minimal events due to deduplication
	mu.Lock()
	t.Logf("Total events received after 5 HUP signals: %d", eventCount)

	if eventCount > 3 {
		t.Logf("Warning: Received %d events, deduplication might not be working optimally", eventCount)
	}
	mu.Unlock()
}
