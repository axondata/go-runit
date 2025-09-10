// Package svcmgr_test provides integration tests for s6 compatibility.
// These tests require s6 to be installed (s6-svscan, s6-supervise, etc.).
// Tests will automatically skip if s6 tools are not available.
package svcmgr_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/axondata/go-svcmgr"
)

func TestIntegrationS6(t *testing.T) {
	svcmgr.RequireS6(t)

	t.Run("basic_operations", func(t *testing.T) {
		// Create a test service directory
		tmpDir := t.TempDir()
		serviceDir := filepath.Join(tmpDir, "test-service")

		// Note: This is a placeholder test
		// Real s6 integration would need:
		// 1. Create service directory structure
		// 2. Start s6-svscan
		// 3. Test all operations (s6 supports everything)
		// 4. Verify s6-specific behavior

		client, err := svcmgr.NewClient(serviceDir, svcmgr.ServiceTypeS6)
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
