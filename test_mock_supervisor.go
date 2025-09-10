//go:build linux

package svcmgr

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MockSupervisor creates a fake supervise directory structure for testing
// This allows tests to run without actual supervisor processes
type MockSupervisor struct {
	ServiceDir   string
	SuperviseDir string
	ControlFile  string
	StatusFile   string
	ServiceType  ServiceType // Track which supervision system we're mocking
}

// NewMockSupervisor creates a mock supervise directory for testing
func NewMockSupervisor(serviceDir string) (*MockSupervisor, error) {
	return NewMockSupervisorWithType(serviceDir, ServiceTypeRunit)
}

// NewMockSupervisorWithType creates a mock supervise directory for a specific supervision system
func NewMockSupervisorWithType(serviceDir string, serviceType ServiceType) (*MockSupervisor, error) {
	m := &MockSupervisor{
		ServiceDir:   serviceDir,
		SuperviseDir: filepath.Join(serviceDir, "supervise"),
		ControlFile:  filepath.Join(serviceDir, "supervise", "control"),
		StatusFile:   filepath.Join(serviceDir, "supervise", "status"),
		ServiceType:  serviceType,
	}

	// Create supervise directory
	if err := os.MkdirAll(m.SuperviseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating supervise dir: %w", err)
	}

	// Create named pipes (FIFOs)
	if err := os.MkdirAll(m.SuperviseDir, 0o755); err != nil {
		return nil, err
	}

	// Create a fake status file with initial "down" status
	// Format depends on the supervision system type
	var statusData []byte
	switch m.ServiceType {
	case ServiceTypeDaemontools:
		statusData = make([]byte, 18)
	case ServiceTypeS6:
		statusData = make([]byte, 35)
	default: // Runit
		statusData = make([]byte, 20)
	}

	// Set timestamp
	now := time.Now()
	tai64 := uint64(now.Unix()) + TAI64Offset // TAI64 epoch offset

	if m.ServiceType == ServiceTypeDaemontools {
		// Daemontools: TAI64 timestamp (8 bytes, big-endian)
		binary.BigEndian.PutUint64(statusData[DaemontoolsTAI64Start:DaemontoolsTAI64End], tai64)
	} else if m.ServiceType != ServiceTypeS6 {
		// Runit: TAI64 timestamp (8 bytes, big-endian)
		// S6 timestamp is handled later in its specific section
		binary.BigEndian.PutUint64(statusData[RunitTAI64Start:RunitTAI64End], tai64)
	}

	// Set initial values based on system type
	switch m.ServiceType {
	case ServiceTypeS6:
		// S6 format (35 bytes) - Pre-2.20.0 format
		// Initial state: all zeros except for want flag
		// PID (big-endian uint16)
		binary.BigEndian.PutUint16(statusData[S6PIDStartPre220:S6PIDEndPre220], 0)
		// Flags
		statusData[S6FlagsBytePre220] = 0 // All flags off initially
	case ServiceTypeDaemontools:
		// Daemontools format (18 bytes)
		// PID (little-endian)
		binary.LittleEndian.PutUint32(statusData[DaemontoolsPIDStart:DaemontoolsPIDEnd], 0)
		// Flags
		statusData[DaemontoolsStatusFlag] = 0 // reserved/status
		statusData[DaemontoolsWantFlag] = 'd' // want
	default:
		// Runit format (20 bytes)
		// PID (little-endian)
		binary.LittleEndian.PutUint32(statusData[RunitPIDStart:RunitPIDEnd], 0)
		// Flags
		statusData[RunitPausedFlag] = 0 // paused
		statusData[RunitWantFlag] = 'd' // want
		statusData[RunitTermFlag] = 0   // term
		statusData[RunitRunFlag] = 0    // run
	}

	// Initial flags are already set above based on type

	// Write status file
	if err := os.WriteFile(m.StatusFile, statusData, 0o644); err != nil {
		return nil, fmt.Errorf("creating status file: %w", err)
	}

	// Create control as a regular file (not a real FIFO, but enough for client creation)
	if err := os.WriteFile(m.ControlFile, []byte{}, 0o644); err != nil {
		return nil, fmt.Errorf("creating control file: %w", err)
	}

	return m, nil
}

// UpdateStatus updates the mock status file
func (m *MockSupervisor) UpdateStatus(running bool, pid int) error {
	// Create status data based on supervision system type
	var statusData []byte
	switch m.ServiceType {
	case ServiceTypeDaemontools:
		statusData = make([]byte, 18)
	case ServiceTypeS6:
		statusData = make([]byte, 35)
	default: // Runit
		statusData = make([]byte, 20)
	}

	// Set timestamp
	now := time.Now()
	tai64 := uint64(now.Unix()) + TAI64Offset // TAI64 epoch offset

	// Update based on system type
	switch m.ServiceType {
	case ServiceTypeS6:
		// Old S6 format (35 bytes, S6 2.12.x and earlier)
		// TAI64N timestamp
		binary.BigEndian.PutUint64(statusData[S6TimestampStartPre220:S6TimestampStartPre220+8], tai64)
		binary.BigEndian.PutUint32(statusData[S6TimestampStartPre220+8:S6TimestampEndPre220], uint32(now.Nanosecond()))

		// TAI64N ready timestamp
		if running && pid > 0 {
			binary.BigEndian.PutUint64(statusData[S6ReadyStartPre220:S6ReadyStartPre220+8], tai64)
			binary.BigEndian.PutUint32(statusData[S6ReadyStartPre220+8:S6ReadyEndPre220], uint32(now.Nanosecond()))
		}

		// bytes 24-27: reserved/zeros (already zero)

		// PID (big-endian uint32 at bytes 28-31)
		if pid > 0 {
			binary.BigEndian.PutUint32(statusData[S6PIDStartPre220:S6PIDEndPre220], uint32(pid))
		}

		// Flags/status
		var flags byte
		if running && pid > 0 {
			flags |= S6FlagReady // ready flag
		}
		if running {
			flags |= S6FlagNormallyUp // normally up flag
		}
		statusData[S6FlagsBytePre220] = flags
	case ServiceTypeDaemontools:
		// Daemontools format (18 bytes)
		// TAI64N timestamp (big-endian)
		binary.BigEndian.PutUint64(statusData[DaemontoolsTAI64Start:DaemontoolsTAI64End], tai64)
		// Nanoseconds (big-endian)
		binary.BigEndian.PutUint32(statusData[DaemontoolsNanoStart:DaemontoolsNanoEnd], uint32(now.Nanosecond()))

		// PID (little-endian)
		binary.LittleEndian.PutUint32(statusData[DaemontoolsPIDStart:DaemontoolsPIDEnd], uint32(pid))

		// Flags
		statusData[DaemontoolsStatusFlag] = 0 // reserved/status
		if running {
			statusData[DaemontoolsWantFlag] = 'u' // want
		} else {
			statusData[DaemontoolsWantFlag] = 'd' // want
		}
	default:
		// Runit format (20 bytes)
		// TAI64N timestamp (big-endian)
		binary.BigEndian.PutUint64(statusData[RunitTAI64Start:RunitTAI64End], tai64)
		// Nanoseconds (big-endian)
		binary.BigEndian.PutUint32(statusData[RunitNanoStart:RunitNanoEnd], uint32(now.Nanosecond()))

		// PID (little-endian)
		binary.LittleEndian.PutUint32(statusData[RunitPIDStart:RunitPIDEnd], uint32(pid))

		// Flags
		statusData[RunitPausedFlag] = 0 // paused
		if running {
			statusData[RunitWantFlag] = 'u' // want
			if pid > 0 {
				statusData[RunitRunFlag] = 1 // run flag (service has process)
			}
		} else {
			statusData[RunitWantFlag] = 'd' // want
			statusData[RunitRunFlag] = 0    // run flag
		}
		statusData[RunitTermFlag] = 0 // term flag
	}

	// All flags have been set above

	return os.WriteFile(m.StatusFile, statusData, 0o644)
}

// Cleanup removes the mock supervise directory
func (m *MockSupervisor) Cleanup() error {
	return os.RemoveAll(m.SuperviseDir)
}

// CreateMockService creates a service with a mock supervisor for testing
func CreateMockService(serviceName string, config *ServiceConfig) (serviceDir string, mock *MockSupervisor, cleanup func(), err error) {
	serviceDir = fmt.Sprintf("/tmp/test-services/%s", serviceName)

	// Build the service
	builder := NewServiceBuilderWithConfig(serviceName, filepath.Dir(serviceDir), config)
	builder.WithCmd([]string{"/bin/sh", "-c", "while true; do echo 'Mock service'; sleep 2; done"})

	if err := builder.Build(); err != nil {
		return "", nil, nil, fmt.Errorf("failed to build service: %w", err)
	}

	// Create mock supervisor with the correct type
	serviceType := ServiceTypeRunit
	if config != nil {
		serviceType = config.Type
	}
	mock, err = NewMockSupervisorWithType(serviceDir, serviceType)
	if err != nil {
		_ = os.RemoveAll(serviceDir)
		return "", nil, nil, fmt.Errorf("failed to create mock supervisor: %w", err)
	}

	cleanup = func() {
		if mock != nil {
			_ = mock.Cleanup()
		}
		_ = os.RemoveAll(serviceDir)
	}

	return serviceDir, mock, cleanup, nil
}
