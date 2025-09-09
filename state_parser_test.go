package runit

import (
	"encoding/binary"
	"encoding/hex"
	"runtime"
	"testing"
	"time"
)

// TestCase represents a test case for state file parsing
type StateParserTestCase struct {
	Name         string
	HexData      string
	Parser       StateParser
	ExpectedPID  int
	ExpectedState State
	Architecture string // e.g., "amd64", "arm64"
	OS           string // e.g., "linux", "darwin"
	Description  string // Additional context about the test
}

func TestRunitStateParser(t *testing.T) {
	parser := &RunitStateParser{}

	testCases := []StateParserTestCase{
		{
			Name:         "runit_running_linux_amd64",
			Architecture: "amd64",
			OS:           "linux",
			Description:  "Running service with PID 1234 on Linux AMD64",
			Parser:       parser,
			// TAI64 timestamp (8 bytes) + nanoseconds (4 bytes) + PID 1234 (4 bytes, little-endian) + flags: paused=0, want='u', term=0, run=1
			HexData:      "4000000067890abc00000000d204000000750001",
			ExpectedPID:  1234,
			ExpectedState: StateRunning,
		},
		{
			Name:         "runit_down_linux_amd64",
			Architecture: "amd64",
			OS:           "linux",
			Description:  "Down service (PID 0) on Linux AMD64",
			Parser:       parser,
			// TAI64 timestamp + nanoseconds + PID 0 + flags (want='d')
			HexData:      "4000000067890abc000000000000000000640000",
			ExpectedPID:  0,
			ExpectedState: StateDown,
		},
		{
			Name:         "runit_paused_linux_arm64",
			Architecture: "arm64",
			OS:           "linux",
			Description:  "Paused service with PID 5678 on Linux ARM64",
			Parser:       parser,
			// TAI64 timestamp + nanoseconds + PID 5678 (little-endian) + paused=1, want='u', term=0, run=1
			HexData:      "4000000067890abc000000002e16000001750001",
			ExpectedPID:  5678,
			ExpectedState: StatePaused,
		},
		{
			Name:         "runit_finishing_linux_amd64",
			Architecture: "amd64",
			OS:           "linux",
			Description:  "Service finishing (terminating) with PID 9999",
			Parser:       parser,
			// TAI64 timestamp + nanoseconds + PID 9999 + flags (term=1)
			HexData:      "4000000067890abc000000000f27000000750101",
			ExpectedPID:  9999,
			ExpectedState: StateFinishing,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Logf("Test: %s", tc.Description)
			t.Logf("Architecture: %s, OS: %s", tc.Architecture, tc.OS)
			t.Logf("Current system: %s/%s", runtime.GOOS, runtime.GOARCH)

			data, err := hex.DecodeString(tc.HexData)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			if !tc.Parser.ValidateSize(len(data)) {
				t.Fatalf("Invalid data size: %d bytes", len(data))
			}

			status, err := tc.Parser.Parse(data)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if status.PID != tc.ExpectedPID {
				t.Errorf("PID mismatch: expected %d, got %d", tc.ExpectedPID, status.PID)
			}

			if status.State != tc.ExpectedState {
				t.Errorf("State mismatch: expected %v, got %v", tc.ExpectedState, status.State)
			}

			t.Logf("Hexdump of test data:\n%s", hex.Dump(data))
		})
	}
}

func TestDaemontoolsStateParser(t *testing.T) {
	parser := &DaemontoolsStateParser{}

	testCases := []StateParserTestCase{
		{
			Name:         "daemontools_running_linux_amd64",
			Architecture: "amd64",
			OS:           "linux",
			Description:  "Running service with PID 1234 on Linux AMD64",
			Parser:       parser,
			// TAI64 (8) + nano (4) + PID 1234 (little-endian) + status + want='u'
			HexData:      "4000000067890abc00000000d20400000075",
			ExpectedPID:  1234,
			ExpectedState: StateRunning,
		},
		{
			Name:         "daemontools_down_linux_arm64",
			Architecture: "arm64",
			OS:           "linux",
			Description:  "Down service on Linux ARM64",
			Parser:       parser,
			// TAI64 + nano + PID 0 + status + want='d'
			HexData:      "4000000067890abc00000000000000000064",
			ExpectedPID:  0,
			ExpectedState: StateDown,
		},
		{
			Name:         "daemontools_stopping_freebsd_amd64",
			Architecture: "amd64",
			OS:           "freebsd",
			Description:  "Service stopping (PID exists but want='d')",
			Parser:       parser,
			// TAI64 + nano + PID 4321 + status + want='d'
			HexData:      "4000000067890abc00000000e11000000064",
			ExpectedPID:  4321,
			ExpectedState: StateStopping,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Logf("Test: %s", tc.Description)
			t.Logf("Architecture: %s, OS: %s", tc.Architecture, tc.OS)

			data, err := hex.DecodeString(tc.HexData)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			if !tc.Parser.ValidateSize(len(data)) {
				t.Fatalf("Invalid data size: %d bytes", len(data))
			}

			status, err := tc.Parser.Parse(data)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if status.PID != tc.ExpectedPID {
				t.Errorf("PID mismatch: expected %d, got %d", tc.ExpectedPID, status.PID)
			}

			if status.State != tc.ExpectedState {
				t.Errorf("State mismatch: expected %v, got %v", tc.ExpectedState, status.State)
			}

			t.Logf("Hexdump of test data:\n%s", hex.Dump(data))
		})
	}
}

func TestS6StateParserPre220(t *testing.T) {
	parser := &S6StateParserPre220{}

	// Helper to create pre-2.20 S6 status data
	createPre220Data := func(pid uint32, flags byte) []byte {
		data := make([]byte, S6StatusSizePre220)
		// TAI64N timestamp at bytes 0-11
		tai64 := uint64(time.Now().Unix()) + TAI64Offset
		binary.BigEndian.PutUint64(data[0:8], tai64)
		binary.BigEndian.PutUint32(data[8:12], 0) // nanoseconds

		// TAI64N ready timestamp at bytes 12-23
		if pid > 0 {
			binary.BigEndian.PutUint64(data[12:20], tai64)
			binary.BigEndian.PutUint32(data[20:24], 0)
		}

		// PID at bytes 28-31 (big-endian uint32)
		binary.BigEndian.PutUint32(data[28:32], pid)

		// Flags at byte 34
		data[34] = flags

		return data
	}

	testCases := []StateParserTestCase{
		{
			Name:         "s6_pre220_running_linux_amd64",
			Architecture: "amd64",
			OS:           "linux",
			Description:  "Running service with PID 12345 on Linux AMD64",
			Parser:       parser,
			HexData:      hex.EncodeToString(createPre220Data(12345, 0x0A)), // ready + normally up
			ExpectedPID:  12345,
			ExpectedState: StateRunning,
		},
		{
			Name:         "s6_pre220_down_linux_arm64",
			Architecture: "arm64",
			OS:           "linux",
			Description:  "Down service on Linux ARM64",
			Parser:       parser,
			HexData:      hex.EncodeToString(createPre220Data(0, 0x00)),
			ExpectedPID:  0,
			ExpectedState: StateDown,
		},
		{
			Name:         "s6_pre220_running_darwin_arm64",
			Architecture: "arm64",
			OS:           "darwin",
			Description:  "Running service with PID 65535 on macOS ARM64",
			Parser:       parser,
			HexData:      hex.EncodeToString(createPre220Data(65535, 0x0A)),
			ExpectedPID:  65535,
			ExpectedState: StateRunning,
		},
		{
			Name:         "s6_pre220_bigpid_linux_amd64",
			Architecture: "amd64",
			OS:           "linux",
			Description:  "Running service with large PID on Linux (testing 32-bit PID)",
			Parser:       parser,
			HexData:      hex.EncodeToString(createPre220Data(2147483647, 0x0A)), // Max 32-bit signed int
			ExpectedPID:  2147483647,
			ExpectedState: StateRunning,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Logf("Test: %s", tc.Description)
			t.Logf("Architecture: %s, OS: %s", tc.Architecture, tc.OS)

			data, err := hex.DecodeString(tc.HexData)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			if !tc.Parser.ValidateSize(len(data)) {
				t.Fatalf("Invalid data size: %d bytes", len(data))
			}

			status, err := tc.Parser.Parse(data)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if status.PID != tc.ExpectedPID {
				t.Errorf("PID mismatch: expected %d, got %d", tc.ExpectedPID, status.PID)
			}

			if status.State != tc.ExpectedState {
				t.Errorf("State mismatch: expected %v, got %v", tc.ExpectedState, status.State)
			}

			t.Logf("Hexdump of test data:\n%s", hex.Dump(data))
		})
	}
}

func TestS6StateParserCurrent(t *testing.T) {
	parser := &S6StateParserCurrent{}

	// Helper to create current S6 status data (>= 2.20.0)
	createCurrentData := func(pid uint64, pgid uint64, flags byte) []byte {
		data := make([]byte, S6StatusSizeCurrent)
		// TAI64N timestamp at bytes 0-11
		tai64 := uint64(time.Now().Unix()) + TAI64Offset
		binary.BigEndian.PutUint64(data[0:8], tai64)
		binary.BigEndian.PutUint32(data[8:12], 0) // nanoseconds

		// TAI64N ready timestamp at bytes 12-23
		if pid > 0 {
			binary.BigEndian.PutUint64(data[12:20], tai64)
			binary.BigEndian.PutUint32(data[20:24], 0)
		}

		// PID at bytes 24-31 (big-endian uint64)
		binary.BigEndian.PutUint64(data[24:32], pid)

		// PGID at bytes 32-39 (big-endian uint64)
		binary.BigEndian.PutUint64(data[32:40], pgid)

		// wstat at bytes 40-41 (big-endian uint16)
		binary.BigEndian.PutUint16(data[40:42], 0)

		// Flags at byte 42
		data[42] = flags

		return data
	}

	testCases := []StateParserTestCase{
		{
			Name:         "s6_current_running_linux_amd64",
			Architecture: "amd64",
			OS:           "linux",
			Description:  "Running service with PID 12345 on Linux AMD64",
			Parser:       parser,
			HexData:      hex.EncodeToString(createCurrentData(12345, 12345, 0x0C)), // want up + ready
			ExpectedPID:  12345,
			ExpectedState: StateRunning,
		},
		{
			Name:         "s6_current_down_linux_arm64",
			Architecture: "arm64",
			OS:           "linux",
			Description:  "Down service on Linux ARM64",
			Parser:       parser,
			HexData:      hex.EncodeToString(createCurrentData(0, 0, 0x00)), // want down
			ExpectedPID:  0,
			ExpectedState: StateDown,
		},
		{
			Name:         "s6_current_paused_freebsd_amd64",
			Architecture: "amd64",
			OS:           "freebsd",
			Description:  "Paused service with PID 5678",
			Parser:       parser,
			HexData:      hex.EncodeToString(createCurrentData(5678, 5678, 0x05)), // paused + want up
			ExpectedPID:  5678,
			ExpectedState: StatePaused,
		},
		{
			Name:         "s6_current_finishing_linux_amd64",
			Architecture: "amd64",
			OS:           "linux",
			Description:  "Service finishing with PID 9999",
			Parser:       parser,
			HexData:      hex.EncodeToString(createCurrentData(9999, 9999, 0x02)), // finishing
			ExpectedPID:  9999,
			ExpectedState: StateFinishing,
		},
		{
			Name:         "s6_current_starting_darwin_arm64",
			Architecture: "arm64",
			OS:           "darwin",
			Description:  "Service starting (want up but no PID yet)",
			Parser:       parser,
			HexData:      hex.EncodeToString(createCurrentData(0, 0, 0x04)), // want up, no PID
			ExpectedPID:  0,
			ExpectedState: StateStarting,
		},
		{
			Name:         "s6_current_largepid_linux_amd64",
			Architecture: "amd64",
			OS:           "linux",
			Description:  "Running service with 64-bit PID (testing large PIDs)",
			Parser:       parser,
			HexData:      hex.EncodeToString(createCurrentData(4294967296, 4294967296, 0x0C)), // 2^32
			ExpectedPID:  4294967296,
			ExpectedState: StateRunning,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Logf("Test: %s", tc.Description)
			t.Logf("Architecture: %s, OS: %s", tc.Architecture, tc.OS)

			data, err := hex.DecodeString(tc.HexData)
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			if !tc.Parser.ValidateSize(len(data)) {
				t.Fatalf("Invalid data size: %d bytes", len(data))
			}

			status, err := tc.Parser.Parse(data)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if status.PID != tc.ExpectedPID {
				t.Errorf("PID mismatch: expected %d, got %d", tc.ExpectedPID, status.PID)
			}

			if status.State != tc.ExpectedState {
				t.Errorf("State mismatch: expected %v, got %v", tc.ExpectedState, status.State)
			}

			t.Logf("Hexdump of test data:\n%s", hex.Dump(data))
		})
	}
}

func TestGetStateParser(t *testing.T) {
	tests := []struct {
		name        string
		serviceType ServiceType
		dataSize    int
		expectError bool
		expectedParser string
	}{
		{
			name:        "runit_valid_size",
			serviceType: ServiceTypeRunit,
			dataSize:    RunitStatusSize,
			expectError: false,
			expectedParser: "runit",
		},
		{
			name:        "runit_invalid_size",
			serviceType: ServiceTypeRunit,
			dataSize:    19,
			expectError: true,
		},
		{
			name:        "daemontools_valid_size",
			serviceType: ServiceTypeDaemontools,
			dataSize:    DaemontoolsStatusSize,
			expectError: false,
			expectedParser: "daemontools",
		},
		{
			name:        "s6_pre220_size",
			serviceType: ServiceTypeS6,
			dataSize:    S6StatusSizePre220,
			expectError: false,
			expectedParser: "s6-pre-2.20.0",
		},
		{
			name:        "s6_current_size",
			serviceType: ServiceTypeS6,
			dataSize:    S6StatusSizeCurrent,
			expectError: false,
			expectedParser: "s6-current",
		},
		{
			name:        "s6_invalid_size",
			serviceType: ServiceTypeS6,
			dataSize:    40,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := GetStateParser(tt.serviceType, tt.dataSize)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if parser.Name() != tt.expectedParser {
				t.Errorf("Expected parser %s, got %s", tt.expectedParser, parser.Name())
			}
		})
	}
}
