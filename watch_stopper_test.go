package svcmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWatchWithStopper verifies that the go-stopper integration works correctly
func TestWatchWithStopper(t *testing.T) {
	// Skip if not on a supported platform
	if os.Getenv("CI") == "" {
		t.Skip("Skipping integration test outside of CI")
	}

	tmpDir := t.TempDir()
	serviceDir := filepath.Join(tmpDir, "test-service")
	superviseDir := filepath.Join(serviceDir, "supervise")
	statusFile := filepath.Join(superviseDir, "status")

	// Create the directory structure
	if err := os.MkdirAll(superviseDir, 0o755); err != nil {
		t.Fatalf("Failed to create supervise dir: %v", err)
	}

	// Create a mock status file
	statusData := make([]byte, StatusFileSize)
	if err := os.WriteFile(statusFile, statusData, 0o644); err != nil {
		t.Fatalf("Failed to create status file: %v", err)
	}

	// Create a runit client
	client := &ClientRunit{
		ServiceDir: serviceDir,
	}

	// Test 1: Normal watch and cleanup
	t.Run("NormalOperation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		events, cleanup, err := client.Watch(ctx)
		if err != nil {
			t.Fatalf("Watch failed: %v", err)
		}

		// Should receive initial event
		select {
		case event := <-events:
			if event.Err != nil {
				t.Errorf("Unexpected error in event: %v", event.Err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Timeout waiting for initial event")
		}

		// Cleanup should work without hanging
		done := make(chan error, 1)
		go func() {
			done <- cleanup()
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Cleanup failed: %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Error("Cleanup took too long")
		}
	})

	// Test 2: Context cancellation
	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		events, cleanup, err := client.Watch(ctx)
		if err != nil {
			t.Fatalf("Watch failed: %v", err)
		}
		defer func() { _ = cleanup() }()

		// Cancel context
		cancel()

		// Events channel should close eventually
		timeout := time.After(500 * time.Millisecond)
		for {
			select {
			case _, ok := <-events:
				if !ok {
					// Channel closed as expected
					return
				}
			case <-timeout:
				t.Error("Events channel didn't close after context cancellation")
				return
			}
		}
	})

	// Test 3: Multiple cleanups (should be idempotent)
	t.Run("IdempotentCleanup", func(t *testing.T) {
		ctx := context.Background()

		_, cleanup, err := client.Watch(ctx)
		if err != nil {
			t.Fatalf("Watch failed: %v", err)
		}

		// First cleanup
		if err := cleanup(); err != nil {
			t.Errorf("First cleanup failed: %v", err)
		}

		// Second cleanup should not error or hang
		done := make(chan error, 1)
		go func() {
			done <- cleanup()
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Second cleanup failed: %v", err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Second cleanup took too long")
		}
	})
}
