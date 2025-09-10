package svcmgr_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/renameio/v2"

	"github.com/axondata/go-svcmgr"
)

// TestIntegrationSingleService tests a single runit service lifecycle
func TestIntegrationSingleService(t *testing.T) {
	svcmgr.RequireNotShort(t)
	svcmgr.RequireRunit(t)
	svcmgr.RequireTool(t, "runsvdir")

	// Create temporary directory for test
	tmpDir := t.TempDir()
	serviceDir := filepath.Join(tmpDir, "test-service")

	// Create service directory structure
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("failed to create service dir: %v", err)
	}

	// Create a simple run script that sleeps and exits
	runScript := `#!/bin/sh
exec 2>&1
echo "Service starting"
sleep 2
echo "Service running"
sleep 10
echo "Service stopping"
exit 0
`
	runFile := filepath.Join(serviceDir, "run")
	if err := renameio.WriteFile(runFile, []byte(runScript), 0o755); err != nil {
		t.Fatalf("failed to write run script: %v", err)
	}

	// Start runsv for this service
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "runsv", serviceDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start runsv: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Wait for supervise directory to be created
	superviseDir := filepath.Join(serviceDir, "supervise")
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(superviseDir); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Create client for the service
	client, err := svcmgr.NewClientRunit(serviceDir)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Test service operations
	t.Run("Status", func(t *testing.T) {
		status, err := client.Status(context.Background())
		if err != nil {
			t.Errorf("failed to get status: %v", err)
		}
		t.Logf("Initial status: state=%v pid=%d", status.State, status.PID)
	})

	t.Run("Start", func(t *testing.T) {
		if err := client.Up(context.Background()); err != nil {
			t.Errorf("failed to start service: %v", err)
		}

		// Wait for service to start
		time.Sleep(500 * time.Millisecond)

		status, err := client.Status(context.Background())
		if err != nil {
			t.Errorf("failed to get status: %v", err)
		}

		if status.PID == 0 {
			t.Error("service should be running but PID is 0")
		}
		if status.State != svcmgr.StateRunning {
			t.Errorf("expected state %v, got %v", svcmgr.StateRunning, status.State)
		}
		t.Logf("Service started: pid=%d state=%v", status.PID, status.State)
	})

	t.Run("Signals", func(t *testing.T) {
		// Send HUP signal
		if err := client.HUP(context.Background()); err != nil {
			t.Errorf("failed to send HUP: %v", err)
		}

		// Send TERM signal
		if err := client.Term(context.Background()); err != nil {
			t.Errorf("failed to send TERM: %v", err)
		}

		time.Sleep(500 * time.Millisecond)

		status, err := client.Status(context.Background())
		if err != nil {
			t.Errorf("failed to get status after TERM: %v", err)
		}
		t.Logf("After TERM: pid=%d state=%v", status.PID, status.State)
	})

	t.Run("Stop", func(t *testing.T) {
		if err := client.Down(context.Background()); err != nil {
			t.Errorf("failed to stop service: %v", err)
		}

		// Wait for service to stop
		time.Sleep(1 * time.Second)

		status, err := client.Status(context.Background())
		if err != nil {
			t.Errorf("failed to get status: %v", err)
		}

		if status.PID != 0 {
			t.Errorf("service should be stopped but PID is %d", status.PID)
		}
		if status.State != svcmgr.StateDown {
			t.Errorf("expected state %v, got %v", svcmgr.StateDown, status.State)
		}
		t.Logf("Service stopped: state=%v", status.State)
	})
}

// TestIntegrationServiceWithExitCodes tests services with different exit codes
func TestIntegrationServiceWithExitCodes(t *testing.T) {
	svcmgr.RequireNotShort(t)
	svcmgr.RequireRunit(t)

	testCases := []struct {
		name     string
		script   string
		wantUp   bool
		expected svcmgr.State
	}{
		{
			name: "exit_0",
			script: `#!/bin/sh
exec 2>&1
echo "Exiting with 0"
exit 0`,
			wantUp:   true,
			expected: svcmgr.StateCrashed, // want up but process exits
		},
		{
			name: "exit_1",
			script: `#!/bin/sh
exec 2>&1
echo "Exiting with 1"
exit 1`,
			wantUp:   true,
			expected: svcmgr.StateCrashed,
		},
		{
			name: "long_running",
			script: `#!/bin/sh
exec 2>&1
echo "Long running service"
exec sleep 300`,
			wantUp:   true,
			expected: svcmgr.StateRunning,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			serviceDir := filepath.Join(tmpDir, tc.name)

			if err := os.MkdirAll(serviceDir, 0o755); err != nil {
				t.Fatalf("failed to create service dir: %v", err)
			}

			runFile := filepath.Join(serviceDir, "run")
			if err := renameio.WriteFile(runFile, []byte(tc.script), 0o755); err != nil {
				t.Fatalf("failed to write run script: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "runsv", serviceDir)
			if err := cmd.Start(); err != nil {
				t.Fatalf("failed to start runsv: %v", err)
			}
			defer func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}()

			// Wait for supervise directory
			superviseDir := filepath.Join(serviceDir, "supervise")
			for i := 0; i < 50; i++ {
				if _, err := os.Stat(superviseDir); err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			client, err := svcmgr.NewClientRunit(serviceDir)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			if tc.wantUp {
				if err := client.Up(context.Background()); err != nil {
					t.Errorf("failed to start service: %v", err)
				}
			}

			// Wait for service to stabilize
			time.Sleep(2 * time.Second)

			status, err := client.Status(context.Background())
			if err != nil {
				t.Errorf("failed to get status: %v", err)
			}

			if status.State != tc.expected {
				t.Errorf("expected state %v, got %v", tc.expected, status.State)
			}
			t.Logf("Service %s: state=%v pid=%d", tc.name, status.State, status.PID)
		})
	}
}

// TestIntegrationServiceBuilder tests the ServiceBuilder functionality
func TestIntegrationServiceBuilder(t *testing.T) {
	svcmgr.RequireNotShort(t)
	svcmgr.RequireRunit(t)

	tmpDir := t.TempDir()

	// Build a service using ServiceBuilder
	builder := svcmgr.NewServiceBuilder("test-builder", tmpDir)
	builder.
		WithCmd([]string{"/bin/sh", "-c", "while true; do echo 'Running'; sleep 1; done"}).
		WithEnv("TEST_VAR", "test_value").
		WithUmask(0o022)

	if err := builder.Build(); err != nil {
		t.Fatalf("failed to build service: %v", err)
	}

	serviceDir := filepath.Join(tmpDir, "test-builder")

	// Verify files were created
	if _, err := os.Stat(filepath.Join(serviceDir, "run")); err != nil {
		t.Errorf("run script not created: %v", err)
	}

	if _, err := os.Stat(filepath.Join(serviceDir, "env", "TEST_VAR")); err != nil {
		t.Errorf("env file not created: %v", err)
	}

	// Start runsv for the built service
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "runsv", serviceDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start runsv: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Wait for supervise directory
	superviseDir := filepath.Join(serviceDir, "supervise")
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(superviseDir); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	client, err := svcmgr.NewClientRunit(serviceDir)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Up(context.Background()); err != nil {
		t.Errorf("failed to start built service: %v", err)
	}

	time.Sleep(1 * time.Second)

	status, err := client.Status(context.Background())
	if err != nil {
		t.Errorf("failed to get status: %v", err)
	}

	if status.State != svcmgr.StateRunning {
		t.Errorf("built service not running: state=%v", status.State)
	}
	t.Logf("Built service running: pid=%d", status.PID)
}
