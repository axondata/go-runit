//go:build linux

package runit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// parseRunitSv parses output from runit's sv status command
// Example: "run: /service/test: (pid 12345) 5s"
func parseRunitSv(output string) (pid int, state string, err error) {
	output = strings.TrimSpace(output)

	// Parse state and PID
	if strings.HasPrefix(output, "run:") {
		state = "run"
		re := regexp.MustCompile(`\(pid (\d+)\)`)
		if matches := re.FindStringSubmatch(output); len(matches) > 1 {
			pid, _ = strconv.Atoi(matches[1])
		}
	} else if strings.HasPrefix(output, "down:") {
		state = "down"
	} else if strings.HasPrefix(output, "pause:") {
		state = "pause"
	} else if strings.HasPrefix(output, "finish:") {
		state = "finish"
	} else {
		state = strings.SplitN(output, ":", 2)[0]
	}

	return pid, state, nil
}

// parseDaemontoolsSvstat parses output from daemontools' svstat command
// Example: "/service/test: up (pid 12345) 5 seconds"
func parseDaemontoolsSvstat(output string) (pid int, state string, err error) {
	output = strings.TrimSpace(output)

	if strings.Contains(output, ": up") {
		state = "up"
		re := regexp.MustCompile(`\(pid (\d+)\)`)
		if matches := re.FindStringSubmatch(output); len(matches) > 1 {
			pid, _ = strconv.Atoi(matches[1])
		}
	} else if strings.Contains(output, ": down") {
		state = "down"
	} else if strings.Contains(output, ": paused") {
		state = "paused"
	}

	return pid, state, nil
}

// parseS6Svstat parses output from s6's s6-svstat command
// Example: "up (pid 12345) 5 seconds"
func parseS6Svstat(output string) (pid int, state string, err error) {
	output = strings.TrimSpace(output)

	// s6-svstat output is simpler
	parts := strings.Fields(output)
	if len(parts) > 0 {
		state = parts[0]
	}

	re := regexp.MustCompile(`\(pid (\d+)\)`)
	if matches := re.FindStringSubmatch(output); len(matches) > 1 {
		pid, _ = strconv.Atoi(matches[1])
	}

	return pid, state, nil
}

// TestStatusDecodeAgainstRealTools compares our decoders against real supervision tools
func TestStatusDecodeAgainstRealTools(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check which tools are available
	tools := []struct {
		name        string
		svstatCmd   []string // Command and subcommand as array
		serviceType ServiceType
		decoder     func([]byte) (Status, error)
		parser      func(string) (int, string, error)
	}{
		{
			name:        "runit",
			svstatCmd:   []string{"sv", "status"}, // sv needs 'status' subcommand
			serviceType: ServiceTypeRunit,
			decoder:     decodeStatusRunit,
			parser:      parseRunitSv,
		},
		{
			name:        "daemontools",
			svstatCmd:   []string{"svstat"}, // svstat takes directory directly
			serviceType: ServiceTypeDaemontools,
			decoder:     decodeStatusDaemontools,
			parser:      parseDaemontoolsSvstat,
		},
		{
			name:        "s6",
			svstatCmd:   []string{"s6-svstat"}, // s6-svstat takes directory directly
			serviceType: ServiceTypeS6,
			decoder:     decodeStatusS6,
			parser:      parseS6Svstat,
		},
	}

	for _, tool := range tools {
		t.Run(tool.name, func(t *testing.T) {
			// Check if tool is available (check the main command)
			RequireTool(t, tool.svstatCmd[0])

			// Also need the supervisor command
			var supervisorCmd string
			switch tool.serviceType {
			case ServiceTypeRunit:
				supervisorCmd = "runsv"
			case ServiceTypeDaemontools:
				supervisorCmd = "supervise"
			case ServiceTypeS6:
				supervisorCmd = "s6-supervise"
			}

			RequireTool(t, supervisorCmd)

			// Create a test service
			serviceDir := t.TempDir()

			// Create run script
			runScript := filepath.Join(serviceDir, "run")
			script := `#!/bin/sh
exec sleep 3600
`
			if err := os.WriteFile(runScript, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}

			// Start supervisor
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			supervisor := exec.CommandContext(ctx, supervisorCmd, serviceDir)
			if err := supervisor.Start(); err != nil {
				t.Fatalf("Failed to start %s: %v", supervisorCmd, err)
			}
			defer func() {
				supervisor.Process.Kill()
				supervisor.Wait()
			}()

			// Wait for supervisor to create status file
			statusFile := filepath.Join(serviceDir, "supervise", "status")
			for i := 0; i < 50; i++ {
				if _, err := os.Stat(statusFile); err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			// Give it a bit more time to stabilize
			time.Sleep(500 * time.Millisecond)

			// Now compare outputs for different states
			testStates := []struct {
				name    string
				setup   func() error
				cleanup func() error
			}{
				{
					name: "running",
					setup: func() error {
						// Send 'up' command
						controlFile := filepath.Join(serviceDir, "supervise", "control")
						return os.WriteFile(controlFile, []byte{'u'}, 0)
					},
				},
				{
					name: "down",
					setup: func() error {
						// Send 'down' command
						controlFile := filepath.Join(serviceDir, "supervise", "control")
						return os.WriteFile(controlFile, []byte{'d'}, 0)
					},
				},
			}

			for _, ts := range testStates {
				t.Run(ts.name, func(t *testing.T) {
					if ts.setup != nil {
						if err := ts.setup(); err != nil {
							t.Fatal(err)
						}
						// Wait for state change
						time.Sleep(500 * time.Millisecond)
					}

					// Read status file
					statusData, err := os.ReadFile(statusFile)
					if err != nil {
						t.Fatal(err)
					}

					// Decode with our decoder
					status, err := tool.decoder(statusData)
					if err != nil {
						t.Fatalf("Failed to decode status: %v", err)
					}

					// Get output from real tool
					// Append service directory to command args
					cmdArgs := append(tool.svstatCmd[1:], serviceDir)
					cmd := exec.Command(tool.svstatCmd[0], cmdArgs...)
					output, err := cmd.Output()
					if err != nil {
						// Since we already checked the tool exists, this is a real failure
						t.Fatalf("Failed to run %s: %v", tool.svstatCmd[0], err)
					}

					// Parse real tool output
					realPID, realState, err := tool.parser(string(output))
					if err != nil {
						t.Fatalf("Failed to parse %s output: %v", tool.svstatCmd[0], err)
					}

					// Compare PID
					if status.PID != realPID {
						t.Errorf("PID mismatch: our decoder=%d, %s=%d", status.PID, tool.svstatCmd[0], realPID)
						t.Logf("Status data (hex): %x", statusData)
					}

					// Compare state (map our states to tool states)
					ourState := status.State.String()
					switch tool.serviceType {
					case ServiceTypeRunit:
						// Map our states to runit svstat states
						switch status.State {
						case StateRunning:
							ourState = "run"
						case StateDown:
							ourState = "down"
						case StatePaused:
							ourState = "pause"
						case StateFinishing:
							ourState = "finish"
						}
					case ServiceTypeDaemontools, ServiceTypeS6:
						// Map our states to daemontools/s6 states
						switch status.State {
						case StateRunning:
							ourState = "up"
						case StateDown:
							ourState = "down"
						case StatePaused:
							ourState = "paused"
						}
					}

					if ourState != realState {
						t.Errorf("State mismatch: our decoder=%s, %s=%s", ourState, tool.svstatCmd[0], realState)
						t.Logf("Status data (hex): %x", statusData)
						t.Logf("Our full status: %+v", status)
					}

					if ts.cleanup != nil {
						ts.cleanup()
					}
				})
			}
		})
	}
}

// TestMockSupervisorStatusFiles tests that our mock creates valid status files
// This test validates the mock implementation itself, not against real tools
// (real tools expect a running supervisor daemon, not just status files)
func TestMockSupervisorStatusFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	configs := []struct {
		name   string
		config *ServiceConfig
		decode func([]byte) (Status, error)
	}{
		{
			name:   "runit",
			config: ConfigRunit(),
			decode: decodeStatusRunit,
		},
		{
			name:   "daemontools",
			config: ConfigDaemontools(),
			decode: decodeStatusDaemontools,
		},
		{
			name:   "s6",
			config: ConfigS6(),
			decode: decodeStatusS6,
		},
	}

	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			// Create mock service
			serviceDir, mock, cleanup, err := CreateMockService("test-"+cfg.name, cfg.config)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()

			// Test different states
			testCases := []struct {
				name    string
				running bool
				pid     int
			}{
				{"down", false, 0},
				{"running", true, 12345},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					// Update mock status
					if err := mock.UpdateStatus(tc.running, tc.pid); err != nil {
						t.Fatal(err)
					}

					// Read and validate the status file
					statusFile := filepath.Join(serviceDir, "supervise", "status")
					statusData, err := os.ReadFile(statusFile)
					if err != nil {
						t.Fatal(err)
					}

					// Decode with our decoder
					status, err := cfg.decode(statusData)
					if err != nil {
						t.Fatalf("Decoder failed: %v", err)
					}

					// Verify the decoded values match what we set
					if status.PID != tc.pid {
						t.Errorf("PID mismatch: expected %d, got %d", tc.pid, status.PID)
					}

					if tc.running && status.State != StateRunning {
						t.Errorf("State mismatch: expected running, got %v", status.State)
					} else if !tc.running && status.State != StateDown {
						t.Errorf("State mismatch: expected down, got %v", status.State)
					}

					// Verify the status file has the correct size for the type
					switch cfg.config.Type {
					case ServiceTypeRunit:
						if len(statusData) != RunitStatusSize {
							t.Errorf("Invalid status file size for runit: expected %d, got %d",
								RunitStatusSize, len(statusData))
						}
					case ServiceTypeDaemontools:
						if len(statusData) != DaemontoolsStatusSize {
							t.Errorf("Invalid status file size for daemontools: expected %d, got %d",
								DaemontoolsStatusSize, len(statusData))
						}
					case ServiceTypeS6:
						// S6 can have two different sizes
						validSize := len(statusData) == S6StatusSizePre220 || len(statusData) == S6StatusSizeCurrent
						if !validSize {
							t.Errorf("Invalid status file size for s6: got %d, expected %d or %d",
								len(statusData), S6StatusSizePre220, S6StatusSizeCurrent)
						}
					}
				})
			}
		})
	}
}
