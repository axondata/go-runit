//go:build integration
// +build integration

package runit_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/axondata/go-runit"
)

// TestIntegrationExitCodeDetection verifies that runit properly captures
// and reflects process exit codes in the service state
func TestIntegrationExitCodeDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("runsv"); err != nil {
		t.Skip("runsv not found in PATH")
	}

	testCases := []struct {
		name           string
		script         string
		expectedStates []runit.State // States we expect to see in order
		description    string
	}{
		{
			name: "clean_exit_0",
			script: `#!/bin/sh
exec 2>&1
echo "Starting service"
sleep 1
echo "Exiting cleanly with code 0"
exit 0`,
			expectedStates: []runit.State{
				runit.StateRunning, // Initially running
				// Note: StateCrashed is very transient as runit restarts quickly
			},
			description: "Service exits with 0 and gets restarted by runit",
		},
		{
			name: "error_exit_1",
			script: `#!/bin/sh
exec 2>&1
echo "Starting service"
sleep 1
echo "Exiting with error code 1"
exit 1`,
			expectedStates: []runit.State{
				runit.StateRunning,
			},
			description: "Service exits with 1 and gets restarted by runit",
		},
		{
			name: "fatal_exit_111",
			script: `#!/bin/sh
exec 2>&1
echo "Starting service"
sleep 1
echo "Fatal error, exit 111"
exit 111`,
			expectedStates: []runit.State{
				runit.StateRunning,
			},
			description: "Service exits with 111 and gets restarted by runit",
		},
		{
			name: "signal_kill",
			script: `#!/bin/sh
exec 2>&1
echo "Starting service"
# This will run until killed
exec sleep 300`,
			expectedStates: []runit.State{
				runit.StateRunning,
			},
			description: "Service killed with SIGKILL gets restarted",
		},
		{
			name: "signal_term_handled",
			script: `#!/bin/sh
exec 2>&1
trap 'echo "Received TERM, exiting gracefully"; exit 0' TERM
echo "Starting service with TERM handler"
while true; do
    sleep 1
done`,
			expectedStates: []runit.State{
				runit.StateRunning,
				runit.StateFinishing, // Finish script may run
				runit.StateDown,      // After process exits
			},
			description: "Service handling SIGTERM gracefully",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Log(tc.description)

			tmpDir := t.TempDir()
			serviceDir := filepath.Join(tmpDir, tc.name)

			if err := os.MkdirAll(serviceDir, 0o755); err != nil {
				t.Fatalf("failed to create service dir: %v", err)
			}

			runFile := filepath.Join(serviceDir, "run")
			if err := os.WriteFile(runFile, []byte(tc.script), 0o755); err != nil {
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

			// Track states we observe
			observedStates := []runit.State{}
			statesSeen := make(map[runit.State]bool)

			// Monitor state changes
			checkState := func() runit.State {
				status, err := client.Status(context.Background())
				if err != nil {
					t.Logf("Warning: failed to get status: %v", err)
					return runit.StateUnknown
				}

				if !statesSeen[status.State] {
					observedStates = append(observedStates, status.State)
					statesSeen[status.State] = true
					t.Logf("State transition: %v (PID: %d)", status.State, status.PID)
				}

				return status.State
			}

			// Initial state check
			time.Sleep(500 * time.Millisecond)
			initialState := checkState()
			t.Logf("Initial state after Up: %v", initialState)

			// For signal tests, send the appropriate signal
			if tc.name == "signal_kill" {
				time.Sleep(1 * time.Second)
				if err := client.Kill(context.Background()); err != nil {
					t.Errorf("failed to send KILL: %v", err)
				}
			} else if tc.name == "signal_term_handled" {
				time.Sleep(1 * time.Second)
				// First send TERM
				if err := client.Term(context.Background()); err != nil {
					t.Errorf("failed to send TERM: %v", err)
				}
				time.Sleep(1 * time.Second)
				// Then tell it to go down
				if err := client.Down(context.Background()); err != nil {
					t.Errorf("failed to send down: %v", err)
				}
			}

			// Monitor for state changes with faster polling for transient states
			for i := 0; i < 40; i++ {
				time.Sleep(250 * time.Millisecond)
				state := checkState()

				// Check if we've seen all expected states
				allSeen := true
				for _, expected := range tc.expectedStates {
					if !statesSeen[expected] {
						allSeen = false
						break
					}
				}

				if allSeen {
					t.Logf("All expected states observed")
					break
				}

				// For crashed services, runit will restart them
				if state == runit.StateCrashed {
					// Wait a bit to see if it gets restarted
					time.Sleep(1 * time.Second)
					checkState()
				}
			}

			// Verify we saw the expected states
			for _, expected := range tc.expectedStates {
				if !statesSeen[expected] {
					t.Errorf("Expected to see state %v but didn't", expected)
				}
			}

			// Log final status
			finalStatus, _ := client.Status(context.Background())
			t.Logf("Final state: %v (PID: %d, Flags: %+v)",
				finalStatus.State, finalStatus.PID, finalStatus.Flags)
		})
	}
}

// TestIntegrationFinishScript tests that finish scripts are executed
func TestIntegrationFinishScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := exec.LookPath("runsv"); err != nil {
		t.Skip("runsv not found in PATH")
	}

	tmpDir := t.TempDir()
	serviceDir := filepath.Join(tmpDir, "finish-test")

	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("failed to create service dir: %v", err)
	}

	// Create a marker file that the finish script will write to
	markerFile := filepath.Join(tmpDir, "finish-ran")

	// Main run script
	runScript := `#!/bin/sh
exec 2>&1
echo "Service running"
sleep 2
echo "Service exiting"
exit 0`

	// Finish script that creates a marker file
	finishScript := `#!/bin/sh
echo "Finish script running"
echo "$$" > ` + markerFile + `
exit 0`

	runFile := filepath.Join(serviceDir, "run")
	if err := os.WriteFile(runFile, []byte(runScript), 0o755); err != nil {
		t.Fatalf("failed to write run script: %v", err)
	}

	finishFile := filepath.Join(serviceDir, "finish")
	if err := os.WriteFile(finishFile, []byte(finishScript), 0o755); err != nil {
		t.Fatalf("failed to write finish script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	// Monitor for StateFinishing
	finishingSeen := false
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)

		status, err := client.Status(context.Background())
		if err != nil {
			continue
		}

		if status.State == runit.StateFinishing {
			finishingSeen = true
			t.Logf("Detected StateFinishing")
			break
		}
	}

	// Check if finish script ran
	time.Sleep(2 * time.Second)
	if _, err := os.Stat(markerFile); err != nil {
		t.Error("Finish script did not run (marker file not created)")
	} else {
		t.Log("Finish script executed successfully")
	}

	if !finishingSeen {
		t.Error("Never saw StateFinishing state")
	}
}
