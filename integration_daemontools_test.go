//go:build integration_daemontools
// +build integration_daemontools

// Package runit_test provides integration tests for daemontools compatibility.
// These tests require daemontools to be installed (svscan, supervise, etc.).
//
// Run with: go test -tags=integration_daemontools ./...
package runit_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/axondata/go-runit"
)

func TestDaemontoolsIntegration(t *testing.T) {
	if _, err := exec.LookPath("svscan"); err != nil {
		t.Skip("daemontools not installed, skipping integration tests")
	}

	t.Run("basic_operations", func(t *testing.T) {
		// Create a test service directory
		tmpDir := t.TempDir()
		serviceDir := filepath.Join(tmpDir, "test-service")

		// Create service with daemontools config
		config := runit.DaemontoolsConfig()

		// Note: This is a placeholder test
		// Real daemontools integration would need:
		// 1. Create service directory structure
		// 2. Start svscan
		// 3. Test operations that daemontools supports
		// 4. Verify that unsupported operations (Once, Quit) fail appropriately

		client, err := runit.NewClientWithConfig(serviceDir, config)
		if err == nil {
			// Test that Once operation is blocked
			ctx := context.Background()
			err = client.Once(ctx)
			if err == nil || err.Error() != "operation once not supported by daemontools" {
				t.Errorf("Expected Once to be blocked for daemontools")
			}
		}
	})
}

func skipIfDaemontoolsNotInstalled(t *testing.T) {
	if _, err := exec.LookPath("svscan"); err != nil {
		t.Skip("daemontools not installed")
	}
}
