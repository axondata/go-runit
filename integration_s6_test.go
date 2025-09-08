//go:build integration_s6
// +build integration_s6

// Package runit_test provides integration tests for s6 compatibility.
// These tests require s6 to be installed (s6-svscan, s6-supervise, etc.).
//
// Run with: go test -tags=integration_s6 ./...
package runit_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/axondata/go-runit"
)

func TestIntegrationS6(t *testing.T) {
	if _, err := exec.LookPath("s6-svscan"); err != nil {
		t.Skip("s6 not installed, skipping integration tests")
	}

	t.Run("basic_operations", func(t *testing.T) {
		// Create a test service directory
		tmpDir := t.TempDir()
		serviceDir := filepath.Join(tmpDir, "test-service")

		// Create service with s6 config
		config := runit.ConfigS6()

		// Note: This is a placeholder test
		// Real s6 integration would need:
		// 1. Create service directory structure
		// 2. Start s6-svscan
		// 3. Test all operations (s6 supports everything)
		// 4. Verify s6-specific behavior

		client, err := runit.NewClientWithConfig(serviceDir, config)
		if err == nil {
			// Test that all operations are allowed for s6
			ctx := context.Background()

			// These would normally fail due to no supervise,
			// but shouldn't have validation errors
			_ = client.Once(ctx) // Should be allowed
			_ = client.Quit(ctx) // Should be allowed
		}
	})
}

func skipIfS6NotInstalled(t *testing.T) {
	if _, err := exec.LookPath("s6-svscan"); err != nil {
		t.Skip("s6 not installed")
	}
}
