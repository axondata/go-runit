// Package runit_test provides integration tests for daemontools compatibility.
// These tests require daemontools to be installed (svscan, supervise, etc.).
// Tests will automatically skip if daemontools tools are not available.
package runit_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/axondata/go-runit"
)

func TestIntegrationDaemontools(t *testing.T) {
	runit.RequireDaemontools(t)

	t.Run("basic_operations", func(t *testing.T) {
		// Create a test service directory
		tmpDir := t.TempDir()
		serviceDir := filepath.Join(tmpDir, "test-service")

		// Note: This is a placeholder test
		// Real daemontools integration would need:
		// 1. Create service directory structure
		// 2. Start svscan
		// 3. Test operations that daemontools supports
		// 4. Verify that unsupported operations (Once, Quit) fail appropriately

		client, err := runit.NewClient(serviceDir, runit.ServiceTypeDaemontools)
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

