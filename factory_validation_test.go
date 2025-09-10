package svcmgr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/renameio/v2"
)

func TestOperationValidation(t *testing.T) {
	// Create a temporary service directory
	tmpDir, err := os.MkdirTemp("", "runit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create supervise directory
	superviseDir := filepath.Join(tmpDir, "supervise")
	if err := os.MkdirAll(superviseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a control FIFO
	controlPath := filepath.Join(superviseDir, "control")
	if err := renameio.WriteFile(controlPath, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		config    *ServiceConfig
		operation Operation
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "runit allows Once",
			config:    ConfigRunit(),
			operation: OpOnce,
			wantErr:   false,
		},
		{
			name:      "daemontools blocks Once",
			config:    ConfigDaemontools(),
			operation: OpOnce,
			wantErr:   true,
			errMsg:    "not supported by daemontools",
		},
		{
			name:      "daemontools blocks Quit",
			config:    ConfigDaemontools(),
			operation: OpQuit,
			wantErr:   true,
			errMsg:    "not supported by daemontools",
		},
		{
			name:      "s6 allows Once",
			config:    ConfigS6(),
			operation: OpOnce,
			wantErr:   false,
		},
		{
			name:      "s6 allows Quit",
			config:    ConfigS6(),
			operation: OpQuit,
			wantErr:   false,
		},
		{
			name:      "all systems allow Up",
			config:    ConfigDaemontools(),
			operation: OpUp,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client based on service type
			var client ServiceClient
			switch tt.config.Type {
			case ServiceTypeRunit:
				client, err = NewClientRunit(tmpDir)
			case ServiceTypeDaemontools:
				client, err = NewClientDaemontools(tmpDir)
			case ServiceTypeS6:
				client, err = NewClientS6(tmpDir)
			default:
				t.Fatalf("Unknown service type: %v", tt.config.Type)
			}
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			// Try to perform the operation
			ctx := context.Background()
			switch tt.operation {
			case OpOnce:
				err = client.Once(ctx)
			case OpQuit:
				err = client.Quit(ctx)
			case OpPause:
				err = client.Pause(ctx)
			case OpCont:
				err = client.Continue(ctx)
			default:
				err = client.Up(ctx) // Default operation
			}

			if tt.wantErr {
				// Should get validation error
				if err == nil {
					t.Fatal("Expected validation error, got nil")
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, err)
				}
			} else if err != nil && strings.Contains(err.Error(), "not supported") {
				// Should either succeed or fail for non-validation reasons
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

func TestClientWithoutConfig(t *testing.T) {
	// Create a temporary service directory
	tmpDir, err := os.MkdirTemp("", "runit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create supervise directory
	superviseDir := filepath.Join(tmpDir, "supervise")
	if err := os.MkdirAll(superviseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create client using default runit client
	client, err := NewClientRunit(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Should have service directory set
	if client.ServiceDir != tmpDir {
		t.Errorf("Expected service directory %s, got %s", tmpDir, client.ServiceDir)
	}

	// Runit client should support all operations
	// Test that Once operation is available (runit supports it)
	ctx := context.Background()
	err = client.Once(ctx)
	// Will fail due to no supervise process, but that's ok - we're just checking it's callable
	if err != nil && strings.Contains(err.Error(), "not supported") {
		t.Errorf("Once operation should be supported by runit: %v", err)
	}
}
