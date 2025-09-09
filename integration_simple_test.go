//go:build linux
// +build linux

package runit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSimpleServiceCreation tests that we can create service directories correctly
func TestSimpleServiceCreation(t *testing.T) {
	systems := []struct {
		name   string
		config *ServiceConfig
	}{
		{"runit", ConfigRunit()},
		{"daemontools", ConfigDaemontools()},
		{"s6", ConfigS6()},
	}

	for _, sys := range systems {
		t.Run(sys.name, func(t *testing.T) {
			serviceName := fmt.Sprintf("test-%s-%d", sys.name, time.Now().Unix())
			serviceDir := filepath.Join("/tmp/test-services", serviceName)

			// Clean up after test
			defer os.RemoveAll(serviceDir)

			// Create service
			builder := NewServiceBuilderWithConfig(serviceName, "/tmp/test-services", sys.config)
			builder.WithCmd([]string{"/bin/sh", "-c", "echo 'Hello from service'; sleep 10"})
			builder.WithEnvMap(map[string]string{
				"TEST_VAR": "test_value",
				"SYSTEM":   sys.name,
			})

			if err := builder.Build(); err != nil {
				t.Fatalf("Failed to build service: %v", err)
			}

			// Verify directory structure
			if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
				t.Errorf("Service directory not created: %s", serviceDir)
			}

			runScript := filepath.Join(serviceDir, "run")
			if _, err := os.Stat(runScript); os.IsNotExist(err) {
				t.Errorf("Run script not created: %s", runScript)
			}

			// Check run script is executable
			info, err := os.Stat(runScript)
			if err == nil && info.Mode()&0111 == 0 {
				t.Errorf("Run script is not executable")
			}

			// Check env directory
			envDir := filepath.Join(serviceDir, "env")
			if _, err := os.Stat(envDir); os.IsNotExist(err) {
				t.Errorf("Env directory not created: %s", envDir)
			}

			// Check env variables
			testVarFile := filepath.Join(envDir, "TEST_VAR")
			if content, err := os.ReadFile(testVarFile); err != nil {
				t.Errorf("Failed to read TEST_VAR: %v", err)
			} else if string(content) != "test_value" {
				t.Errorf("TEST_VAR has wrong value: got %q, want %q", string(content), "test_value")
			}

			t.Logf("✓ Service %s created successfully with correct structure", serviceName)
		})
	}
}

// TestSimpleSystemdCreation tests systemd unit file creation
func TestSimpleSystemdCreation(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("Systemd tests require root privileges")
	}

	serviceName := fmt.Sprintf("test-systemd-%d", time.Now().Unix())

	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/sh", "-c", "echo 'Hello from systemd'; sleep 10"})
	builder.WithEnvMap(map[string]string{
		"TEST_VAR": "test_value",
		"SYSTEM":   "systemd",
	})

	systemdBuilder := NewBuilderSystemd(builder)
	systemdBuilder.UnitDir = "/tmp/test-systemd-units"
	systemdBuilder.UseSudo = false

	// Create unit directory
	if err := os.MkdirAll(systemdBuilder.UnitDir, 0755); err != nil {
		t.Fatalf("Failed to create unit directory: %v", err)
	}
	defer os.RemoveAll(systemdBuilder.UnitDir)

	// Generate unit content
	unitContent, err := systemdBuilder.BuildSystemdUnit()
	if err != nil {
		t.Fatalf("Failed to generate unit content: %v", err)
	}

	// Verify unit content
	if !strings.Contains(unitContent, "[Unit]") {
		t.Error("Unit content missing [Unit] section")
	}
	if !strings.Contains(unitContent, "[Service]") {
		t.Error("Unit content missing [Service] section")
	}
	if !strings.Contains(unitContent, "[Install]") {
		t.Error("Unit content missing [Install] section")
	}
	if !strings.Contains(unitContent, "TEST_VAR=test_value") {
		t.Error("Unit content missing environment variable")
	}

	t.Logf("✓ Systemd unit file generated successfully")
	t.Logf("Unit content:\n%s", unitContent)
}

// TestServiceWithRunningSupervision tests services with actual running supervision
// This test requires the supervision system to be installed and running
func TestServiceWithRunningSupervision(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping supervision test in short mode")
	}

	ctx := context.Background()

	// Test systemd if available and running
	t.Run("systemd", func(t *testing.T) {
		RequireSystemd(t) // This will skip if systemctl not available
		
		// Check if systemd is actually running as init
		if output, err := exec.Command("systemctl", "is-system-running").Output(); err == nil {
			status := strings.TrimSpace(string(output))
			if status != "running" && status != "degraded" {
				t.Skipf("Systemd present but not running as init: %s", status)
			}
		}
		
		testSystemdService(ctx, t)
	})

	// For runit/daemontools/s6, we would need a running supervision tree
	// which is complex to set up in tests, so we skip these for now
	t.Log("Note: Runit/Daemontools/S6 tests require a running supervision tree")
}

func testSystemdService(ctx context.Context, t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("Systemd service test requires root")
	}

	serviceName := fmt.Sprintf("test-systemd-svc-%d", time.Now().Unix())

	// Create service
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/sh", "-c", "while true; do date; sleep 5; done"})

	systemdBuilder := NewBuilderSystemd(builder)
	systemdBuilder.UnitDir = "/run/systemd/system" // Use runtime directory
	systemdBuilder.UseSudo = false

	if err := systemdBuilder.BuildWithContext(ctx); err != nil {
		t.Fatalf("Failed to build systemd service: %v", err)
	}

	// Clean up
	defer func() {
		client := NewClientSystemd(serviceName)
		client.UseSudo = false
		_ = client.Stop(ctx)
		_ = systemdBuilder.Remove(ctx)
	}()

	// Create client
	client := NewClientSystemd(serviceName)
	client.UseSudo = false

	// Start service
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}

	// Wait for service to be running
	time.Sleep(2 * time.Second)

	// Check status
	status, err := client.StatusSystemd(ctx)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if !status.Running {
		t.Errorf("Service not running: state=%s, substate=%s", status.ActiveState, status.SubState)
	}

	t.Logf("✓ Systemd service %s running with PID %d", serviceName, status.MainPID)

	// Stop service
	if err := client.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop service: %v", err)
	}

	t.Logf("✓ Systemd service %s stopped successfully", serviceName)
}
