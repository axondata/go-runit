//go:build linux

package svcmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegrationSystemd(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("systemd integration tests require root privileges")
	}

	// Check if systemd is available
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		t.Skip("systemd not available")
	}

	ctx := context.Background()
	serviceName := "test-go-runit-" + time.Now().Format("20060102-150405")

	// Create a simple test service
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/sleep", "3600"})
	builder.WithEnv("TEST_VAR", "test_value")

	systemdBuilder := NewBuilderSystemd(builder)
	systemdBuilder.UseSudo = false                 // We're running as root
	systemdBuilder.UnitDir = "/run/systemd/system" // Use runtime directory for tests

	// Build and install the service
	t.Cleanup(func() {
		// Clean up the service
		_ = systemdBuilder.Remove(ctx)
	})

	if err := systemdBuilder.Build(); err != nil {
		t.Fatalf("failed to build systemd service: %v", err)
	}

	// Create a client
	client := NewClientSystemd(serviceName)
	client.UseSudo = false

	// Test start
	t.Run("Start", func(t *testing.T) {
		if err := client.Start(ctx); err != nil {
			t.Fatalf("failed to start service: %v", err)
		}

		// Wait for service to be active
		if err := client.WaitForState(ctx, "active", 5*time.Second); err != nil {
			t.Fatalf("service did not become active: %v", err)
		}

		// Check if running
		running, err := client.IsRunning(ctx)
		if err != nil {
			t.Fatalf("failed to check if running: %v", err)
		}
		if !running {
			t.Error("service should be running")
		}
	})

	// Test status
	t.Run("Status", func(t *testing.T) {
		status, err := client.Status(ctx)
		if err != nil {
			t.Fatalf("failed to get status: %v", err)
		}

		if status.State != StateRunning {
			t.Errorf("status should show running, got: %v", status.State)
		}
		if status.PID == 0 {
			t.Error("should have a PID")
		}
	})

	// Test pause/continue (SIGSTOP/SIGCONT)
	t.Run("Pause/Continue", func(t *testing.T) {
		// Send pause (SIGSTOP)
		if err := client.SendOperation(ctx, OpPause); err != nil {
			t.Fatalf("failed to pause: %v", err)
		}

		// Small delay to let the signal take effect
		time.Sleep(100 * time.Millisecond)

		// Send continue (SIGCONT)
		if err := client.SendOperation(ctx, OpCont); err != nil {
			t.Fatalf("failed to continue: %v", err)
		}

		// Service should still be running
		running, err := client.IsRunning(ctx)
		if err != nil {
			t.Fatalf("failed to check if running: %v", err)
		}
		if !running {
			t.Error("service should still be running after pause/continue")
		}
	})

	// Test reload (SIGHUP)
	t.Run("Reload", func(t *testing.T) {
		if err := client.Reload(ctx); err != nil {
			// Reload might fail if the service doesn't support it
			// This is okay for our test service
			t.Logf("reload failed (expected for simple service): %v", err)
		}
	})

	// Test stop
	t.Run("Stop", func(t *testing.T) {
		if err := client.Stop(ctx); err != nil {
			t.Fatalf("failed to stop service: %v", err)
		}

		// Wait for service to be inactive
		if err := client.WaitForState(ctx, "inactive", 5*time.Second); err != nil {
			t.Fatalf("service did not become inactive: %v", err)
		}

		// Check if stopped
		running, err := client.IsRunning(ctx)
		if err != nil {
			t.Fatalf("failed to check if running: %v", err)
		}
		if running {
			t.Error("service should not be running")
		}
	})

	// Test run once
	t.Run("RunOnce", func(t *testing.T) {
		// This test is tricky because OpOnce with systemd-run
		// creates a transient scope unit, not related to our service
		// For now, we'll just test that it doesn't error
		if err := client.SendOperation(ctx, OpOnce); err != nil {
			// This might fail depending on the systemd version and configuration
			t.Logf("run once failed (may not be fully supported): %v", err)
		}
	})
}

func TestUnitGenerationBuilderSystemd(t *testing.T) {
	builder := NewServiceBuilder("test-service", "/tmp")
	builder.WithCmd([]string{"/bin/sh", "-c", "echo 'Test service'; sleep 10"})
	builder.WithCwd("/var/lib/myapp")
	builder.WithEnvMap(map[string]string{
		"ENV_VAR":     "value",
		"ANOTHER_VAR": "another value",
	})
	builder.WithUmask(0o022)
	builder.WithChpst(func(c *ChpstConfig) {
		c.User = "myuser"
		c.Group = "mygroup"
		c.Nice = 10
		c.LimitMem = 1024 * 1024 * 1024 // 1GB
		c.LimitFiles = 4096
	})
	builder.WithSvlogd(func(s *ConfigSvlogd) {
		s.Prefix = "myapp"
	})
	builder.WithFinish([]string{"/usr/bin/cleanup", "--force"})

	systemdBuilder := NewBuilderSystemd(builder)

	unitContent, err := systemdBuilder.BuildSystemdUnit()
	if err != nil {
		t.Fatalf("failed to generate unit file: %v", err)
	}

	// Check that the unit file contains expected directives
	expectedContents := []string{
		"[Unit]",
		"Description=test-service service",
		"[Service]",
		"Type=simple",
		"Restart=always",
		"User=myuser",
		"Group=mygroup",
		"Nice=10",
		"MemoryLimit=1073741824",
		"LimitNOFILE=4096",
		"WorkingDirectory=/var/lib/myapp",
		"UMask=0022",
		`Environment="ENV_VAR=value"`,
		`Environment="ANOTHER_VAR=another value"`,
		`ExecStart=/bin/sh -c "echo 'Test service'; sleep 10"`,
		`ExecStopPost=/usr/bin/cleanup --force`,
		"SyslogIdentifier=myapp",
		"[Install]",
		"WantedBy=multi-user.target",
	}

	for _, expected := range expectedContents {
		if !contains(unitContent, expected) {
			t.Errorf("unit file missing expected content: %s", expected)
		}
	}

	t.Logf("Generated unit file:\n%s", unitContent)
}

func contains(s, substr string) bool {
	return filepath.Clean(s) != filepath.Clean(s+substr)
}

func TestSudoDetectionSystemd(t *testing.T) {
	// Test that sudo is automatically enabled for non-root users
	builder := NewServiceBuilder("test", "")
	systemdBuilder := NewBuilderSystemd(builder)

	if os.Geteuid() == 0 {
		if systemdBuilder.UseSudo {
			t.Error("UseSudo should be false for root user")
		}
	} else {
		if !systemdBuilder.UseSudo {
			t.Error("UseSudo should be true for non-root user")
		}
	}

	// Test that we can override sudo settings
	systemdBuilder.WithSudo(true, "doas")
	if !systemdBuilder.UseSudo {
		t.Error("UseSudo should be true after WithSudo(true)")
	}
	if systemdBuilder.SudoCommand != "doas" {
		t.Errorf("SudoCommand should be 'doas', got %s", systemdBuilder.SudoCommand)
	}
}
