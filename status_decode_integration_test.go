//go:build linux

package svcmgr

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// SupervisionTestSuite provides common test infrastructure for supervision systems
type SupervisionTestSuite struct {
	suite.Suite
	tempDir string
}

func (s *SupervisionTestSuite) SetupSuite() {
	// Create a temporary directory for all tests
	var err error
	s.tempDir, err = os.MkdirTemp("", "go-runit-test-*")
	require.NoError(s.T(), err, "Failed to create temp directory")
}

func (s *SupervisionTestSuite) TearDownSuite() {
	// Clean up temp directory
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
	}
}


// TestStatusDecodeIntegration tests status decoding against real supervisors
func TestStatusDecodeIntegration(t *testing.T) {
	suite.Run(t, new(StatusDecodeTestSuite))
}

type StatusDecodeTestSuite struct {
	SupervisionTestSuite
}

func (s *StatusDecodeTestSuite) TestRunitStatusDecode() {
	// Skip if runit tools are not available
	RequireRunit(s.T())

	// Create a test service directory
	serviceDir := filepath.Join(s.tempDir, "test-runit-service")
	require.NoError(s.T(), os.MkdirAll(serviceDir, 0755))

	// Create a simple run script
	runScript := filepath.Join(serviceDir, "run")
	runContent := "#!/bin/sh\nexec sleep 1000\n"
	require.NoError(s.T(), os.WriteFile(runScript, []byte(runContent), 0755))

	// Start runsv
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "runsv", serviceDir)
	require.NoError(s.T(), cmd.Start())
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for supervise directory to be created
	superviseDir := filepath.Join(serviceDir, "supervise")
	statusFile := filepath.Join(superviseDir, "status")

	require.Eventually(s.T(), func() bool {
		_, err := os.Stat(statusFile)
		return err == nil
	}, 5*time.Second, 100*time.Millisecond, "Status file was not created")

	// Now test our decoder against the real status file
	statusData, err := os.ReadFile(statusFile)
	require.NoError(s.T(), err)
	require.Len(s.T(), statusData, RunitStatusSize, "Runit status file should be %d bytes", RunitStatusSize)

	// Decode the status
	status, err := decodeStatusRunit(statusData)
	require.NoError(s.T(), err)

	// The service should be running
	require.Equal(s.T(), StateRunning, status.State, "Service should be running")
	require.Greater(s.T(), status.PID, 0, "PID should be positive")
	require.True(s.T(), status.Flags.WantUp, "Service should want to be up")

	// Now stop the service using sv command if available
	// Note: Using exec.LookPath directly here because sv is optional - we don't want to skip the test
	// if sv is not available, we just won't use it for the stop operation
	if _, err := exec.LookPath("sv"); err == nil { //nolint:gosec // Optional tool check
		stopCmd := exec.Command("sv", "stop", serviceDir)
		err := stopCmd.Run()
		// sv might fail if it's not configured properly, that's ok
		if err == nil {
			// Wait a bit for status to update
			time.Sleep(500 * time.Millisecond)

			// Read status again
			statusData, err = os.ReadFile(statusFile)
			require.NoError(s.T(), err)

			status, err = decodeStatusRunit(statusData)
			require.NoError(s.T(), err)

			// Service should be down or stopping
			require.Contains(s.T(), []State{StateDown, StateStopping}, status.State)
		}
	}
}

func (s *StatusDecodeTestSuite) TestDaemontoolsStatusDecode() {
	// Skip if daemontools is not available
	RequireDaemontools(s.T())

	// Create a test service directory
	serviceDir := filepath.Join(s.tempDir, "test-daemontools-service")
	require.NoError(s.T(), os.MkdirAll(serviceDir, 0755))

	// Create a simple run script
	runScript := filepath.Join(serviceDir, "run")
	runContent := "#!/bin/sh\nexec sleep 1000\n"
	require.NoError(s.T(), os.WriteFile(runScript, []byte(runContent), 0755))

	// Start supervise
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "supervise", serviceDir)
	require.NoError(s.T(), cmd.Start())
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for supervise directory to be created
	superviseDir := filepath.Join(serviceDir, "supervise")
	statusFile := filepath.Join(superviseDir, "status")

	require.Eventually(s.T(), func() bool {
		info, err := os.Stat(statusFile)
		return err == nil && info.Size() == DaemontoolsStatusSize
	}, 5*time.Second, 100*time.Millisecond, "Status file was not created")

	// Test our decoder
	statusData, err := os.ReadFile(statusFile)
	require.NoError(s.T(), err)
	require.Len(s.T(), statusData, DaemontoolsStatusSize)

	status, err := decodeStatusDaemontools(statusData)
	require.NoError(s.T(), err)

	// Verify the decoded status makes sense
	// Daemontools might show various states depending on timing and if the service starts
	require.Contains(s.T(), []State{StateRunning, StateDown, StateStarting, StateCrashed}, status.State)
}

func (s *StatusDecodeTestSuite) TestS6StatusDecode() {
	// Skip if s6 is not available
	RequireS6(s.T())

	// Create a test service directory
	serviceDir := filepath.Join(s.tempDir, "test-s6-service")
	require.NoError(s.T(), os.MkdirAll(serviceDir, 0755))

	// Create a simple run script
	runScript := filepath.Join(serviceDir, "run")
	runContent := "#!/bin/sh\nexec sleep 1000\n"
	require.NoError(s.T(), os.WriteFile(runScript, []byte(runContent), 0755))

	// Start s6-supervise
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "s6-supervise", serviceDir)
	require.NoError(s.T(), cmd.Start())
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for supervise directory to be created
	superviseDir := filepath.Join(serviceDir, "supervise")
	statusFile := filepath.Join(superviseDir, "status")

	require.Eventually(s.T(), func() bool {
		info, err := os.Stat(statusFile)
		if err != nil {
			return false
		}
		// S6 can have two different status file sizes
		return info.Size() == S6StatusSizePre220 || info.Size() == S6StatusSizeCurrent
	}, 5*time.Second, 100*time.Millisecond, "Status file was not created")

	// Test our decoder
	statusData, err := os.ReadFile(statusFile)
	require.NoError(s.T(), err)

	status, err := decodeStatusS6(statusData)
	require.NoError(s.T(), err)

	// Verify format was detected
	require.Contains(s.T(), []S6FormatVersion{S6FormatPre220, S6FormatCurrent}, status.S6Format)

	// Verify the decoded status makes sense
	require.Contains(s.T(), []State{StateRunning, StateDown, StateStarting}, status.State)
}

// TestMockSupervisorValidation tests that our mock supervisors create valid status files
func TestMockSupervisorValidation(t *testing.T) {
	// This test validates our mock implementation, not against real tools
	testCases := []struct {
		name   string
		config *ServiceConfig
		decode func([]byte) (Status, error)
	}{
		{
			name:   "runit",
			config: ConfigRunit(),
			decode: decodeStatusRunit,
		},
		{
			name:   "daemontools",
			config: ConfigDaemontools(),
			decode: decodeStatusDaemontools,
		},
		{
			name:   "s6",
			config: ConfigS6(),
			decode: decodeStatusS6,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock service
			serviceDir, mock, cleanup, err := CreateMockService("test-"+tc.name, tc.config)
			require.NoError(t, err)
			defer cleanup()

			// Test different states
			states := []struct {
				running bool
				pid     int
			}{
				{false, 0},
				{true, 12345},
			}

			for _, state := range states {
				// Update mock status
				err := mock.UpdateStatus(state.running, state.pid)
				require.NoError(t, err)

				// Read and decode status file
				statusFile := filepath.Join(serviceDir, "supervise", "status")
				statusData, err := os.ReadFile(statusFile)
				require.NoError(t, err)

				status, err := tc.decode(statusData)
				require.NoError(t, err)

				// Verify decoded values
				require.Equal(t, state.pid, status.PID)
				if state.running {
					require.Equal(t, StateRunning, status.State)
				} else {
					require.Equal(t, StateDown, status.State)
				}
			}
		})
	}
}
