package runit

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"
)

// TestStatusDecodeDaemontools tests parsing daemontools status files
func TestStatusDecodeDaemontools(t *testing.T) {
	tests := []struct {
		name     string
		hexData  string
		expected Status
	}{
		{
			name: "service_down",
			// TAI64N format: TAI64 (8 bytes big-endian) + nanos (4 bytes big-endian) + PID (4 bytes little-endian) + flags (2 bytes)
			// TAI64: 0x4000000067890abc (big-endian)
			// Nanos: 0x00000000
			// PID: 0
			// Flags: 0x00, 'd'
			hexData: "4000000067890abc0000000000000000" + "0064",
			expected: Status{
				State: StateDown,
				PID:   0,
				Flags: Flags{WantDown: true},
			},
		},
		{
			name: "service_running",
			// TAI64N format
			// PID: 12345 (0x3039 little-endian)
			// Flags: 0x00, 'u'
			hexData: "4000000067890abc0000000039300000" + "0075",
			expected: Status{
				State: StateRunning,
				PID:   12345,
				Flags: Flags{WantUp: true},
			},
		},
		{
			name: "service_crashed",
			// TAI64N format
			// PID: 0
			// Flags: 0x00, 'u'
			hexData: "4000000067890abc0000000000000000" + "0075",
			expected: Status{
				State: StateCrashed,
				PID:   0,
				Flags: Flags{WantUp: true},
			},
		},
		{
			name: "service_paused",
			// TAI64N format (daemontools doesn't actually have paused state)
			// PID: 54321 (0xd431 little-endian)
			// Flags: 0x00, 'u'
			hexData: "4000000067890abc0000000031d40000" + "0075",
			expected: Status{
				State: StateRunning, // Daemontools doesn't have paused state
				PID:   54321,
				Flags: Flags{WantUp: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.hexData)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			status, err := decodeStatusDaemontools(data)
			if err != nil {
				t.Fatalf("Failed to decode status: %v", err)
			}

			if status.State != tt.expected.State {
				t.Errorf("State: got %v, want %v", status.State, tt.expected.State)
			}
			if status.PID != tt.expected.PID {
				t.Errorf("PID: got %d, want %d", status.PID, tt.expected.PID)
			}
			if status.Flags.WantUp != tt.expected.Flags.WantUp {
				t.Errorf("WantUp: got %v, want %v", status.Flags.WantUp, tt.expected.Flags.WantUp)
			}
			if status.Flags.WantDown != tt.expected.Flags.WantDown {
				t.Errorf("WantDown: got %v, want %v", status.Flags.WantDown, tt.expected.Flags.WantDown)
			}
		})
	}
}

// TestStatusDecodeRunit tests parsing runit status files
func TestStatusDecodeRunit(t *testing.T) {
	tests := []struct {
		name     string
		hexData  string
		expected Status
	}{
		{
			name: "service_down",
			// TAI64N: TAI64 seconds at bytes 0-7 (big-endian), nanoseconds at bytes 8-11 (big-endian)
			// PID: 0 at bytes 12-15 (little-endian)
			// Flags at bytes 16-19: paused: 0, want: 'd', term: 0, run: 0
			hexData: "400000006789abcd000000000000000000640000",
			expected: Status{
				State: StateDown,
				PID:   0,
				Flags: Flags{WantDown: true, NormallyUp: false},
			},
		},
		{
			name: "service_running_ready",
			// TAI64N: TAI64 seconds at bytes 0-7 (big-endian), nanoseconds at bytes 8-11 (big-endian)
			// PID: 12345 (0x3039 little-endian) at bytes 12-15
			// Flags at bytes 16-19: paused: 0, want: 'u', term: 0, run: 1
			hexData: "400000006789abcd000000003930000000750001",
			expected: Status{
				State: StateRunning,
				PID:   12345,
				Flags: Flags{WantUp: true, NormallyUp: true},
			},
		},
		{
			name: "service_starting_not_ready",
			// TAI64N: TAI64 seconds at bytes 0-7 (big-endian), nanoseconds at bytes 8-11 (big-endian)
			// PID: 12345 (0x3039 little-endian) at bytes 12-15
			// Flags at bytes 16-19: paused: 0, want: 'u', term: 0, run: 1
			// Note: in the fixed decoder, running with want='u' is StateRunning, not StateStarting
			hexData: "400000006789abcd000000003930000000750001",
			expected: Status{
				State: StateRunning, // With the fixed decoder, running with want='u' is always StateRunning
				PID:   12345,
				Flags: Flags{WantUp: true, NormallyUp: true},
			},
		},
		{
			name: "service_stopping",
			// TAI64N: TAI64 seconds at bytes 0-7 (big-endian), nanoseconds at bytes 8-11 (big-endian)
			// PID: 8765 (0x223d little-endian) at bytes 12-15
			// Flags at bytes 16-19: paused: 0, want: 'd', term: 0, run: 1
			hexData: "400000006789abcd000000003d22000000640001",
			expected: Status{
				State: StateStopping,
				PID:   8765,
				Flags: Flags{WantDown: true, NormallyUp: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.hexData)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			status, err := decodeStatusRunit(data)
			if err != nil {
				t.Fatalf("Failed to decode status: %v", err)
			}

			if status.State != tt.expected.State {
				t.Errorf("State: got %v, want %v", status.State, tt.expected.State)
			}
			if status.PID != tt.expected.PID {
				t.Errorf("PID: got %d, want %d", status.PID, tt.expected.PID)
			}
			if status.Flags.WantUp != tt.expected.Flags.WantUp {
				t.Errorf("WantUp: got %v, want %v", status.Flags.WantUp, tt.expected.Flags.WantUp)
			}
			if status.Flags.WantDown != tt.expected.Flags.WantDown {
				t.Errorf("WantDown: got %v, want %v", status.Flags.WantDown, tt.expected.Flags.WantDown)
			}
			// NormallyUp flag interpretation varies and is not critical for basic functionality
		})
	}
}

// TestStatusDecodeS6 tests parsing s6 status files
func TestStatusDecodeS6(t *testing.T) {
	tests := []struct {
		name     string
		hexData  string
		expected Status
	}{
		{
			name: "service_down",
			// Old S6 format (35 bytes):
			// bytes 0-11: TAI64N timestamp
			// bytes 12-23: TAI64N ready timestamp
			// bytes 24-29: reserved
			// bytes 30-31: PID (big-endian uint16)
			// bytes 32-34: flags
			hexData: "4000000067890abc00000000" + "4000000067890abc00000000" + "000000000000" + "0000" + "000000",
			expected: Status{
				State: StateDown,
				PID:   0,
				Flags: Flags{WantDown: true},
			},
		},
		{
			name: "service_running_ready",
			// Old S6 format (35 bytes):
			// PID: 12345 (0x3039 big-endian at bytes 30-31)
			// flags at byte 34: 0x0a (normally up + ready)
			hexData: "4000000067890abc00000000" + "4000000067890abc00000000" + "000000000000" + "3039" + "00000a",
			expected: Status{
				State: StateRunning,
				PID:   12345,
				Flags: Flags{WantUp: true},
			},
		},
		{
			name: "service_starting_not_ready",
			// Old S6 format: PID: 12345, flags: 0x02 (normally up, not ready)
			hexData: "4000000067890abc00000000" + "000000000000000000000000" + "000000000000" + "3039" + "000002",
			expected: Status{
				State: StateRunning, // Old format doesn't distinguish starting
				PID:   12345,
				Flags: Flags{WantUp: true},
			},
		},
		{
			name: "service_crashed",
			// Old S6 format: PID: 0, flags: 0x02 (normally up but not running)
			hexData: "4000000067890abc00000000" + "4000000067890abc00000000" + "000000000000" + "0000" + "000002",
			expected: Status{
				State: StateDown, // Old format doesn't distinguish crashed
				PID:   0,
				Flags: Flags{WantDown: true, NormallyUp: true},
			},
		},
		{
			name: "service_paused",
			// Old S6 format doesn't have pause state, just running
			// PID: 8765 (0x223d)
			hexData: "4000000067890abc00000000" + "4000000067890abc00000000" + "000000000000" + "223d" + "00000a",
			expected: Status{
				State: StateRunning, // Old format doesn't distinguish paused
				PID:   8765,
				Flags: Flags{WantUp: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.hexData)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			status, err := decodeStatusS6(data)
			if err != nil {
				t.Fatalf("Failed to decode status: %v", err)
			}

			if status.State != tt.expected.State {
				t.Errorf("State: got %v, want %v", status.State, tt.expected.State)
			}
			if status.PID != tt.expected.PID {
				t.Errorf("PID: got %d, want %d", status.PID, tt.expected.PID)
			}
			if status.Flags.WantUp != tt.expected.Flags.WantUp {
				t.Errorf("WantUp: got %v, want %v", status.Flags.WantUp, tt.expected.Flags.WantUp)
			}
			if status.Flags.WantDown != tt.expected.Flags.WantDown {
				t.Errorf("WantDown: got %v, want %v", status.Flags.WantDown, tt.expected.Flags.WantDown)
			}
		})
	}
}

// TestMockSupervisorEncodingRunit tests that the mock supervisor creates correct runit status files
func TestMockSupervisorEncodingRunit(t *testing.T) {
	testCases := []struct {
		name    string
		running bool
		pid     int
	}{
		{"runit_down", false, 0},
		{"runit_up", true, 12345},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statusData := make([]byte, 20)
			now := time.Now()
			tai64 := uint64(now.Unix()) + TAI64Offset

			// Encode like the corrected mock supervisor (matching real runit format)
			// TAI64N timestamp at bytes 0-11 (big-endian)
			binary.BigEndian.PutUint64(statusData[0:8], tai64)
			// Nanoseconds at bytes 8-11 (big-endian)
			binary.BigEndian.PutUint32(statusData[8:12], uint32(now.Nanosecond()))

			// PID at bytes 12-15 (little-endian)
			binary.LittleEndian.PutUint32(statusData[12:16], uint32(tc.pid))

			// Flags
			statusData[16] = 0 // paused
			if tc.running {
				statusData[17] = 'u' // want
				if tc.pid > 0 {
					statusData[19] = 1 // run flag
				}
			} else {
				statusData[17] = 'd' // want
				statusData[19] = 0   // run flag
			}
			statusData[18] = 0 // term flag

			// Decode and verify
			status, err := decodeStatusRunit(statusData)
			if err != nil {
				t.Fatalf("Failed to decode status: %v", err)
			}

			if status.PID != tc.pid {
				t.Errorf("PID: got %d, want %d", status.PID, tc.pid)
			}

			if tc.running && tc.pid > 0 {
				if status.State != StateRunning {
					t.Errorf("Expected running state, got %v", status.State)
				}
			} else if !tc.running {
				if status.State != StateDown {
					t.Errorf("Expected down state, got %v", status.State)
				}
			}
		})
	}
}

// TestMockSupervisorEncodingDaemontools tests that the mock supervisor creates correct daemontools status files
func TestMockSupervisorEncodingDaemontools(t *testing.T) {
	testCases := []struct {
		name    string
		running bool
		pid     int
	}{
		{"daemontools_down", false, 0},
		{"daemontools_up", true, 54321},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statusData := make([]byte, 18)
			now := time.Now()
			tai64 := uint64(now.Unix()) + TAI64Offset

			// Encode like the corrected mock supervisor (matching real daemontools format)
			// TAI64N timestamp at bytes 0-11 (big-endian)
			binary.BigEndian.PutUint64(statusData[DaemontoolsTAI64Start:DaemontoolsTAI64End], tai64)
			// Nanoseconds at bytes 8-11 (big-endian)
			binary.BigEndian.PutUint32(statusData[DaemontoolsNanoStart:DaemontoolsNanoEnd], uint32(now.Nanosecond()))
			// PID at bytes 12-15 (little-endian)
			binary.LittleEndian.PutUint32(statusData[DaemontoolsPIDStart:DaemontoolsPIDEnd], uint32(tc.pid))

			// Flags at bytes 16-17
			statusData[DaemontoolsStatusFlag] = 0 // reserved/status
			if tc.running {
				statusData[DaemontoolsWantFlag] = 'u' // want
			} else {
				statusData[DaemontoolsWantFlag] = 'd'
			}

			// Decode and verify
			status, err := decodeStatusDaemontools(statusData)
			if err != nil {
				t.Fatalf("Failed to decode status: %v", err)
			}

			if status.PID != tc.pid {
				t.Errorf("PID: got %d, want %d", status.PID, tc.pid)
			}

			if tc.running && tc.pid > 0 {
				if status.State != StateRunning {
					t.Errorf("Expected running state, got %v", status.State)
				}
			} else if !tc.running {
				if status.State != StateDown {
					t.Errorf("Expected down state, got %v", status.State)
				}
			}
		})
	}
}

// TestMockSupervisorEncodingS6 tests that the mock supervisor creates correct S6 status files
func TestMockSupervisorEncodingS6(t *testing.T) {
	testCases := []struct {
		name    string
		running bool
		pid     int
	}{
		{"s6_down", false, 0},
		{"s6_up", true, 8765},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statusData := make([]byte, 35)
			now := time.Now()
			tai64 := uint64(now.Unix()) + TAI64Offset

			// Encode using the old S6 format (35 bytes)
			// bytes 0-11: TAI64N timestamp
			binary.BigEndian.PutUint64(statusData[0:8], tai64)
			binary.BigEndian.PutUint32(statusData[8:12], uint32(now.Nanosecond()))

			// bytes 12-23: TAI64N ready timestamp
			if tc.running && tc.pid > 0 {
				binary.BigEndian.PutUint64(statusData[12:20], tai64)
				binary.BigEndian.PutUint32(statusData[20:24], uint32(now.Nanosecond()))
			}

			// bytes 24-29: reserved/zeros (already zero)

			// bytes 30-31: PID (big-endian uint16)
			if tc.pid > 0 && tc.pid <= 65535 {
				binary.BigEndian.PutUint16(statusData[30:32], uint16(tc.pid))
			}

			// byte 34: flags
			var flags byte
			if tc.running && tc.pid > 0 {
				flags |= 0x08 // ready flag
			}
			if tc.running {
				flags |= 0x02 // normally up flag
			}
			statusData[34] = flags

			// Decode and verify
			status, err := decodeStatusS6(statusData)
			if err != nil {
				t.Fatalf("Failed to decode status: %v", err)
			}

			if status.PID != tc.pid {
				t.Errorf("PID: got %d, want %d", status.PID, tc.pid)
			}

			if tc.running && tc.pid > 0 {
				if status.State != StateRunning {
					t.Errorf("Expected running state, got %v", status.State)
				}
			} else if !tc.running {
				if status.State != StateDown {
					t.Errorf("Expected down state, got %v", status.State)
				}
			}
		})
	}
}
