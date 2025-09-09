//go:build go1.18
// +build go1.18

package runit

import (
	"testing"
)

// FuzzDecodeStatus tests the decodeStatusRunit function with random inputs
// to ensure it doesn't panic or cause unexpected behavior
func FuzzDecodeStatus(f *testing.F) {
	// Add seed corpus with valid status data
	f.Add(makeStatusData(1234, 'u', 0, 1))
	f.Add(makeStatusData(5678, 'd', 0, 0))
	f.Add(makeStatusData(9999, 'u', 1, 1))
	f.Add(makeStatusData(0, 'd', 0, 1, withTermFlag()))

	// Add edge cases
	emptyData := make([]byte, StatusFileSize)
	f.Add(emptyData)

	maxData := make([]byte, StatusFileSize)
	for i := range maxData {
		maxData[i] = 0xFF
	}
	f.Add(maxData)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Test that decodeStatusRunit doesn't panic
		status, err := decodeStatusRunit(data)

		// If successful, verify the status is reasonable
		if err == nil {
			// PID should be non-negative
			if status.PID < 0 {
				t.Errorf("negative PID: %d", status.PID)
			}

			// State should be valid
			switch status.State {
			case StateUnknown, StateDown, StateStarting, StateRunning, StatePaused,
				StateStopping, StateFinishing, StateCrashed, StateExited:
				// Valid states
			default:
				t.Errorf("invalid state: %v", status.State)
			}

			// Uptime should be non-negative
			if status.Uptime < 0 {
				t.Errorf("negative uptime: %v", status.Uptime)
			}
		}
	})
}

// FuzzMakeStatusData tests the makeStatusData helper function
func FuzzMakeStatusData(f *testing.F) {
	// Add seed corpus
	f.Add(1234, byte('u'), byte(0), byte(1))
	f.Add(5678, byte('d'), byte(0), byte(0))
	f.Add(0, byte('o'), byte(1), byte(1))
	f.Add(99999, byte('u'), byte(0), byte(1))

	f.Fuzz(func(t *testing.T, pid int, want byte, paused byte, running byte) {
		// Test that makeStatusData doesn't panic
		data := makeStatusData(pid, want, paused, running)

		// Verify the data is the correct size
		if len(data) != StatusFileSize {
			t.Errorf("wrong data size: got %d, want %d", len(data), StatusFileSize)
		}

		// Test round-trip: encode then decode
		status, err := decodeStatusRunit(data)
		if err != nil {
			// Some combinations might be invalid, that's ok
			return
		}

		// Verify PID matches (within valid range)
		if pid >= 0 && status.PID != pid {
			t.Errorf("PID mismatch: got %d, want %d", status.PID, pid)
		}
	})
}
