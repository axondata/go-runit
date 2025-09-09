//go:build linux

package runit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// SkipIfShort skips the test if running in short mode
func SkipIfShort(t *testing.T, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skipf("Skipping in short mode: %s", reason)
	}
}

// SupervisorProcess manages a supervisor process for testing
type SupervisorProcess struct {
	Type       ServiceType
	ServiceDir string
	Cmd        *exec.Cmd
	Started    bool
}

// StartSupervisor starts the appropriate supervisor process for the service type
func StartSupervisor(ctx context.Context, serviceType ServiceType, serviceDir string) (*SupervisorProcess, error) {
	sp := &SupervisorProcess{
		Type:       serviceType,
		ServiceDir: serviceDir,
	}

	var supervisorCmd string
	var args []string

	switch serviceType {
	case ServiceTypeRunit:
		supervisorCmd = "runsv"
		args = []string{serviceDir}
	case ServiceTypeDaemontools:
		supervisorCmd = "supervise"
		args = []string{serviceDir}
	case ServiceTypeS6:
		supervisorCmd = "s6-supervise"
		args = []string{serviceDir}
	default:
		return nil, fmt.Errorf("unsupported service type for supervisor: %v", serviceType)
	}

	// Check if supervisor binary exists
	if _, err := exec.LookPath(supervisorCmd); err != nil {
		return nil, fmt.Errorf("supervisor binary %s not found: %w", supervisorCmd, err)
	}

	// Start the supervisor process
	sp.Cmd = exec.CommandContext(ctx, supervisorCmd, args...)
	sp.Cmd.Dir = filepath.Dir(serviceDir)

	// Set up process group so we can kill the entire tree
	sp.Cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := sp.Cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", supervisorCmd, err)
	}

	sp.Started = true

	// Wait for supervise directory to be created
	superviseDir := filepath.Join(serviceDir, "supervise")
	statusFile := filepath.Join(superviseDir, "status")
	for i := 0; i < 50; i++ { // Wait up to 5 seconds
		if _, err := os.Stat(superviseDir); err == nil {
			// Supervise directory exists, now wait for status file
			// Get expected size based on service type
			var expectedSize int64
			switch serviceType {
			case ServiceTypeRunit:
				expectedSize = 20
			case ServiceTypeDaemontools:
				expectedSize = 18
			case ServiceTypeS6:
				expectedSize = 35
			default:
				expectedSize = 20 // Default to runit size
			}

			for j := 0; j < 20; j++ { // Wait up to 2 seconds for status file
				if info, err := os.Stat(statusFile); err == nil {
					size := info.Size()
					if size == expectedSize {
						// Status file exists and has correct size
						// Give it a tiny bit more time to ensure it's fully written
						time.Sleep(50 * time.Millisecond)
						return sp, nil
					} else if size > 0 {
						if debugEnv := os.Getenv("DEBUG_RUNIT"); debugEnv != "" {
							fmt.Fprintf(os.Stderr, "[DEBUG] Status file %s has size %d bytes (waiting for %d)\n", statusFile, size, expectedSize)
						}
					}
				}
				time.Sleep(100 * time.Millisecond)
			}
			// Supervise dir exists but no valid status file yet
			if debugEnv := os.Getenv("DEBUG_RUNIT"); debugEnv != "" {
				fmt.Fprintf(os.Stderr, "[DEBUG] Warning: supervise dir exists but status file not ready for %s\n", serviceDir)
			}
			return sp, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// If we got here, supervise directory wasn't created
	sp.Stop()
	return nil, fmt.Errorf("supervisor started but supervise directory not created after 5 seconds")
}

// Stop stops the supervisor process
func (sp *SupervisorProcess) Stop() error {
	if !sp.Started || sp.Cmd == nil || sp.Cmd.Process == nil {
		return nil
	}

	// Kill the process group
	if sp.Cmd.SysProcAttr != nil && sp.Cmd.SysProcAttr.Setpgid {
		// Kill the entire process group
		_ = syscall.Kill(-sp.Cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(100 * time.Millisecond)
		_ = syscall.Kill(-sp.Cmd.Process.Pid, syscall.SIGKILL)
	} else {
		// Just kill the process
		_ = sp.Cmd.Process.Kill()
	}

	// Wait for it to exit
	_ = sp.Cmd.Wait()
	sp.Started = false

	// Clean up supervise directory
	superviseDir := filepath.Join(sp.ServiceDir, "supervise")
	_ = os.RemoveAll(superviseDir)

	return nil
}

// WaitForSupervise waits for the supervise directory to be created
func WaitForSupervise(serviceDir string, timeout time.Duration) error {
	superviseDir := filepath.Join(serviceDir, "supervise")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(superviseDir); err == nil {
			// Give it a bit more time to fully initialize
			time.Sleep(100 * time.Millisecond)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("supervise directory not created after %v", timeout)
}

// CreateTestServiceWithSupervisor creates a service and starts its supervisor
func CreateTestServiceWithSupervisor(ctx context.Context, sys SupervisionSystem, serviceName string, logger *TestLogger) (client interface{}, supervisor *SupervisorProcess, cleanup func() error, err error) {
	serviceDir := fmt.Sprintf("/tmp/test-services/%s", serviceName)

	// Build the service
	builder := NewServiceBuilderWithConfig(serviceName, filepath.Dir(serviceDir), sys.Config)
	builder.WithCmd([]string{"/bin/sh", "-c", "while true; do echo 'Test service running'; sleep 2; done"})

	if err := builder.Build(); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to build service: %w", err)
	}

	// Start the supervisor
	supervisor, err = StartSupervisor(ctx, sys.Type, serviceDir)
	if err != nil {
		os.RemoveAll(serviceDir)
		return nil, nil, nil, fmt.Errorf("failed to start supervisor: %w", err)
	}

	// Create the client based on type
	var c ServiceClient
	switch sys.Config.Type {
	case ServiceTypeRunit:
		c, err = NewClientRunit(serviceDir)
	case ServiceTypeDaemontools:
		c, err = NewClientDaemontools(serviceDir)
	case ServiceTypeS6:
		c, err = NewClientS6(serviceDir)
	case ServiceTypeSystemd:
		c, err = NewClientSystemd(serviceName), nil
	default:
		supervisor.Stop()
		os.RemoveAll(serviceDir)
		return nil, nil, nil, fmt.Errorf("unsupported service type: %v", sys.Config.Type)
	}
	if err != nil {
		supervisor.Stop()
		os.RemoveAll(serviceDir)
		return nil, nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Create cleanup function
	cleanup = func() error {
		if supervisor != nil {
			supervisor.Stop()
		}
		return os.RemoveAll(serviceDir)
	}

	return c, supervisor, cleanup, nil
}
