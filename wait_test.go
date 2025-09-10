//go:build linux

package svcmgr

import (
	"context"
	"testing"
	"time"
)

// TestWaitNilStates verifies that Wait properly handles nil states
func TestWaitNilStates(t *testing.T) {
	// Create a mock service
	serviceDir, mock, cleanup, err := CreateMockService("test-wait-nil", ConfigRunit())
	if err != nil {
		t.Fatalf("Failed to create mock service: %v", err)
	}
	defer cleanup()

	// Create client
	client, err := NewClient(serviceDir, ServiceTypeRunit)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Update mock to running state
	if err := mock.UpdateStatus(true, 12345); err != nil {
		t.Fatalf("Failed to update mock status: %v", err)
	}

	// Test with nil states (should wait for any change)
	// Note: Watch sends an initial status, so Wait will return immediately with current status
	ctx1, cancel1 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel1()

	status1, err1 := client.Wait(ctx1, nil)
	if err1 != nil {
		t.Errorf("Unexpected error from Wait with nil states: %v", err1)
	}
	// Should get the current status (running)
	if status1.State != StateRunning {
		t.Errorf("Expected StateRunning from Wait(nil), got %v", status1.State)
	}

	// Test with empty slice (should behave the same as nil)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()

	status2, err2 := client.Wait(ctx2, []State{})
	if err2 != nil {
		t.Errorf("Unexpected error from Wait with empty states: %v", err2)
	}
	// Should get the current status (running)
	if status2.State != StateRunning {
		t.Errorf("Expected StateRunning from Wait([]), got %v", status2.State)
	}

	// Test with specific states
	ctx3, cancel3 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel3()

	// We're already running, so this should return immediately
	status3, err3 := client.Wait(ctx3, []State{StateRunning})
	if err3 != nil {
		t.Errorf("Unexpected error waiting for running state: %v", err3)
	}
	if status3.State != StateRunning {
		t.Errorf("Expected StateRunning, got %v", status3.State)
	}
	if status3.PID != 12345 {
		t.Errorf("Expected PID 12345, got %d", status3.PID)
	}
}

// TestWaitNilSafety verifies that nil doesn't cause a panic
func TestWaitNilSafety(t *testing.T) {
	// Create a mock service
	serviceDir, _, cleanup, err := CreateMockService("test-wait-safety", ConfigRunit())
	if err != nil {
		t.Fatalf("Failed to create mock service: %v", err)
	}
	defer cleanup()

	// Create client
	client, err := NewClient(serviceDir, ServiceTypeRunit)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// This should not panic
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	var nilStates []State
	_, _ = client.Wait(ctx, nilStates)

	// Test passed if we didn't panic
}
