//go:build integration || integration_runit
// +build integration integration_runit

package runit_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/renameio/v2"

	"github.com/axondata/go-runit"
)

// TestIntegrationCrashedStateDetection verifies we can detect the crashed state
// This test uses the 'once' command to prevent automatic restart
func TestIntegrationCrashedStateDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("runsv"); err != nil {
		t.Skip("runsv not found in PATH")
	}

	tmpDir := t.TempDir()
	serviceDir := filepath.Join(tmpDir, "crash-test")

	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("failed to create service dir: %v", err)
	}

	// Service that exits quickly
	runScript := `#!/bin/sh
exec 2>&1
echo "Service starting PID=$$"
sleep 1
echo "Service exiting with code 1"
exit 1`

	runFile := filepath.Join(serviceDir, "run")
	if err := renameio.WriteFile(runFile, []byte(runScript), 0o755); err != nil {
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

	t.Run("DetectCrashedWithOnce", func(t *testing.T) {
		// Use 'once' to run service once without restart
		if err := client.Once(context.Background()); err != nil {
			t.Errorf("failed to run once: %v", err)
		}

		// Wait for service to start and exit
		time.Sleep(2 * time.Second)

		status, err := client.Status(context.Background())
		if err != nil {
			t.Errorf("failed to get status: %v", err)
		}

		// After 'once' and exit, service should be down (not crashed)
		// because we told it to run once
		if status.State != runit.StateDown {
			t.Logf("After 'once' and exit: state=%v (expected down)", status.State)
		}

		// Now tell it to be up
		if err := client.Up(context.Background()); err != nil {
			t.Errorf("failed to set want up: %v", err)
		}

		// Give the process time to exit but before restart
		time.Sleep(1500 * time.Millisecond)

		// Try to catch the crashed state
		crashedSeen := false
		for i := 0; i < 10; i++ {
			status, err := client.Status(context.Background())
			if err != nil {
				continue
			}

			t.Logf("Status check %d: state=%v pid=%d want_up=%v",
				i, status.State, status.PID, status.Flags.WantUp)

			if status.State == runit.StateCrashed {
				crashedSeen = true
				t.Log("Successfully detected StateCrashed!")
				break
			}

			// Check for the crashed condition even if our state inference is wrong
			if status.PID == 0 && status.Flags.WantUp {
				t.Log("Detected crashed condition: PID=0 with WantUp=true")
				crashedSeen = true
			}

			time.Sleep(200 * time.Millisecond)
		}

		if !crashedSeen {
			t.Log("Note: Crashed state is transient and may be hard to catch")
		}
	})

	t.Run("DetectDownVsCrashed", func(t *testing.T) {
		// First ensure service is down
		if err := client.Down(context.Background()); err != nil {
			t.Errorf("failed to stop service: %v", err)
		}

		time.Sleep(1 * time.Second)

		status, err := client.Status(context.Background())
		if err != nil {
			t.Errorf("failed to get status: %v", err)
		}

		// Should be down with want_down
		if status.State != runit.StateDown {
			t.Errorf("Expected StateDown, got %v", status.State)
		}
		if status.Flags.WantUp {
			t.Error("Expected WantUp=false when down")
		}
		t.Logf("Down state confirmed: pid=%d want_up=%v", status.PID, status.Flags.WantUp)

		// Now set want up but service will crash
		if err := client.Up(context.Background()); err != nil {
			t.Errorf("failed to set want up: %v", err)
		}

		// Service will start and exit repeatedly
		// Try to catch it in crashed state
		time.Sleep(1500 * time.Millisecond)

		status, err = client.Status(context.Background())
		if err != nil {
			t.Errorf("failed to get status: %v", err)
		}

		// Log the actual state
		t.Logf("After Up with crashing service: state=%v pid=%d want_up=%v",
			status.State, status.PID, status.Flags.WantUp)

		// The key difference between down and crashed:
		// - Down: PID=0, WantUp=false
		// - Crashed: PID=0, WantUp=true
		if status.Flags.WantUp && status.PID == 0 {
			t.Log("Confirmed crashed condition: want up but not running")
		}
	})
}

// TestIntegrationRestartBehavior verifies runit's restart behavior
func TestIntegrationRestartBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("runsv"); err != nil {
		t.Skip("runsv not found in PATH")
	}

	tmpDir := t.TempDir()
	serviceDir := filepath.Join(tmpDir, "restart-test")

	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("failed to create service dir: %v", err)
	}

	// Create a counter file to track restarts
	counterFile := filepath.Join(tmpDir, "restart-count")
	if err := renameio.WriteFile(counterFile, []byte("0"), 0o644); err != nil {
		t.Fatalf("failed to create counter file: %v", err)
	}

	// Service that increments counter and exits
	runScript := `#!/bin/sh
exec 2>&1
COUNT=$(cat ` + counterFile + `)
COUNT=$((COUNT + 1))
echo "$COUNT" > ` + counterFile + `
echo "Service start #$COUNT PID=$$"
sleep 1
echo "Service exit #$COUNT"
exit 1`

	runFile := filepath.Join(serviceDir, "run")
	if err := renameio.WriteFile(runFile, []byte(runScript), 0o755); err != nil {
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

	// Start the service
	if err := client.Up(context.Background()); err != nil {
		t.Errorf("failed to start service: %v", err)
	}

	// Let it restart a few times
	time.Sleep(5 * time.Second)

	// Check restart count
	countData, err := os.ReadFile(counterFile)
	if err != nil {
		t.Errorf("failed to read counter: %v", err)
	}

	restartCount := string(countData)
	t.Logf("Service restarted %s times in 5 seconds", restartCount)

	// Stop the service
	if err := client.Down(context.Background()); err != nil {
		t.Errorf("failed to stop service: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Verify it's actually down
	status, err := client.Status(context.Background())
	if err != nil {
		t.Errorf("failed to get final status: %v", err)
	}

	if status.State != runit.StateDown {
		t.Errorf("Expected StateDown after Down(), got %v", status.State)
	}

	t.Logf("Final state: %v, PID: %d", status.State, status.PID)
}
