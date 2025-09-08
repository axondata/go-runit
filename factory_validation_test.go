package runit

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
			// Create client with config
			client, err := NewClientWithConfig(tmpDir, tt.config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			// Try to send the operation
			ctx := context.Background()
			err = client.send(ctx, tt.operation)

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

	// Create client without config (standard New function)
	client, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Should have nil config
	if client.Config != nil {
		t.Error("Expected nil config for standard client")
	}

	// All operations should be allowed (no validation)
	ctx := context.Background()

	// This will fail due to no supervise process, but shouldn't have validation error
	err = client.send(ctx, OpOnce)
	if err != nil && strings.Contains(err.Error(), "not supported") {
		t.Errorf("Unexpected validation error without config: %v", err)
	}
}
