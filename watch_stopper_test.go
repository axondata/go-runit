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

	// Create a valid mock status file with proper structure
	// The status file needs to have valid timestamps and structure
	statusData := make([]byte, StatusFileSize)
	// Set a valid TAI64N timestamp (first 12 bytes)
	now := time.Now()
	tai64 := uint64(now.Unix()) + 0x4000000000000000 // TAI64 epoch offset
	// Write TAI64 timestamp (8 bytes, big-endian)
	statusData[0] = byte(tai64 >> 56)
	statusData[1] = byte(tai64 >> 48)
	statusData[2] = byte(tai64 >> 40)
	statusData[3] = byte(tai64 >> 32)
	statusData[4] = byte(tai64 >> 24)
	statusData[5] = byte(tai64 >> 16)
	statusData[6] = byte(tai64 >> 8)
	statusData[7] = byte(tai64)
	// Nanoseconds (4 bytes, big-endian) - set to 0
	// PID (4 bytes, little-endian) - set to 0 (service down)
	// Remaining flags set to 0 (service down state)

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

		// Should receive initial event (allow more time in CI)
		timeout := 500 * time.Millisecond
		if os.Getenv("CI") != "" {
			timeout = 2 * time.Second // CI environments can be slower
		}

		select {
		case event := <-events:
			if event.Err != nil {
				t.Errorf("Unexpected error in event: %v", event.Err)
			}
		case <-time.After(timeout):
			t.Errorf("Timeout waiting for initial event after %v", timeout)
		}

		// Cleanup should work without hanging
		done := make(chan error, 1)
		go func() {
			done <- cleanup()
		}()

		cleanupTimeout := 1 * time.Second
		if os.Getenv("CI") != "" {
			cleanupTimeout = 3 * time.Second // CI environments can be slower
		}

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Cleanup failed: %v", err)
			}
		case <-time.After(cleanupTimeout):
			t.Errorf("Cleanup took too long (> %v)", cleanupTimeout)
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
		closeTimeout := 1 * time.Second
		if os.Getenv("CI") != "" {
			closeTimeout = 3 * time.Second // CI environments can be slower
		}
		timeout := time.After(closeTimeout)
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
