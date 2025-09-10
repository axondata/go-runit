//go:build linux

package svcmgr

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

const systemdUnitDir = "/run/systemd/system"

// shouldUseMocks determines if we should use mock supervisors
func shouldUseMocks() bool {
	return os.Getenv("USE_MOCK_SUPERVISORS") == "1" || os.Getenv("TEST_MODE") == "mock"
}

// createTestServiceWithSupervisor creates a test service with either mock or real supervisor
func createTestServiceWithSupervisor(ctx context.Context, sys SupervisionSystem, serviceName string, logger *TestLogger) (client interface{}, cleanup func() error, err error) {
	return createTestServiceWithSupervisorAndCmd(ctx, sys, serviceName, logger, []string{"/bin/sh", "-c", "while true; do echo 'Running'; sleep 2; done"}, nil)
}

// createTestServiceWithSupervisorAndCmd creates a test service with custom command and environment
func createTestServiceWithSupervisorAndCmd(ctx context.Context, sys SupervisionSystem, serviceName string, logger *TestLogger, cmd []string, env map[string]string) (client interface{}, cleanup func() error, err error) {
	if sys.Type == ServiceTypeSystemd {
		// Systemd doesn't need mocks, it manages its own processes
		builder := NewServiceBuilder(serviceName, "")
		builder.WithCmd(cmd)
		if env != nil {
			builder.WithEnvMap(env)
		}

		systemdBuilder := NewBuilderSystemd(builder)
		if os.Geteuid() == 0 {
			systemdBuilder.UnitDir = systemdUnitDir
			systemdBuilder.UseSudo = false
		}

		if err := systemdBuilder.BuildWithContext(ctx); err != nil {
			return nil, nil, fmt.Errorf("failed to build systemd service: %w", err)
		}

		cleanup = func() error {
			return systemdBuilder.Remove(ctx)
		}

		c := NewClientSystemd(serviceName)
		if os.Geteuid() == 0 {
			c.UseSudo = false
		}
		return c, cleanup, nil
	}

	// For runit/daemontools/s6
	if shouldUseMocks() {
		logger.Log("[%s] Using mock supervisor", strings.ToUpper(sys.Name))
		serviceDir, _, cleanupFn, err := CreateMockService(serviceName, sys.Config)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create mock service: %w", err)
		}

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
			cleanupFn()
			return nil, nil, fmt.Errorf("unsupported service type: %v", sys.Config.Type)
		}
		if err != nil {
			cleanupFn()
			return nil, nil, fmt.Errorf("failed to create client: %w", err)
		}

		return c, func() error { cleanupFn(); return nil }, nil
	}

	// Try real supervisor
	serviceDir := fmt.Sprintf("/tmp/test-services/%s", serviceName)

	builder := NewServiceBuilderWithConfig(serviceName, "/tmp/test-services", sys.Config)
	builder.WithCmd(cmd)
	if env != nil {
		builder.WithEnvMap(env)
	}

	if err := builder.Build(); err != nil {
		return nil, nil, fmt.Errorf("failed to build service: %w", err)
	}

	// Start the supervisor process
	supervisor, err := StartSupervisor(ctx, sys.Type, serviceDir)
	if err != nil {
		// Fall back to mock supervisor when real one isn't available
		logger.Log("[%s] Real supervisor not available: %v", strings.ToUpper(sys.Name), err)
		logger.Log("[%s] Falling back to mock supervisor", strings.ToUpper(sys.Name))

		// Clean up the service directory created for real supervisor
		_ = os.RemoveAll(serviceDir)

		// Create mock service instead
		mockServiceDir, mock, cleanupFn, mockErr := CreateMockService(serviceName, sys.Config)
		if mockErr != nil {
			return nil, nil, fmt.Errorf("failed to create mock supervisor: %w", mockErr)
		}

		// Create client based on type
		var c ServiceClient
		var clientErr error
		switch sys.Config.Type {
		case ServiceTypeRunit:
			c, clientErr = NewClientRunit(mockServiceDir)
		case ServiceTypeDaemontools:
			c, clientErr = NewClientDaemontools(mockServiceDir)
		case ServiceTypeS6:
			c, clientErr = NewClientS6(mockServiceDir)
		case ServiceTypeSystemd:
			c, clientErr = NewClientSystemd(serviceName), nil
		default:
			cleanupFn()
			return nil, nil, fmt.Errorf("unsupported service type: %v", sys.Config.Type)
		}
		if clientErr != nil {
			cleanupFn()
			return nil, nil, fmt.Errorf("failed to create client: %w", clientErr)
		}

		// Simulate service being ready for mock
		_ = mock.UpdateStatus(true, os.Getpid())

		cleanup := func() error {
			cleanupFn()
			return nil
		}

		return c, cleanup, nil
	}

	cleanup = func() error {
		if supervisor != nil {
			_ = supervisor.Stop()
		}
		return os.RemoveAll(serviceDir)
	}

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
		_ = cleanup()
		return nil, nil, fmt.Errorf("unsupported service type: %v", sys.Config.Type)
	}
	if err != nil {
		_ = cleanup()
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Wait for supervisor to be ready to accept commands
	// The supervisor needs time to:
	// 1. Set up the control FIFO
	// 2. Start listening for commands
	// 3. Write initial status
	logger.Log("[%s] Waiting for supervisor to be ready...", strings.ToUpper(sys.Name))

	// Try to read status repeatedly until we get a valid response
	// This ensures the supervisor is fully initialized
	for i := 0; i < 20; i++ { // 2 seconds max
		status, err := c.Status(context.Background())
		if err == nil {
			logger.Log("[%s] Supervisor ready, initial state: %s", strings.ToUpper(sys.Name), status.State)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Additional small delay to ensure control FIFO is ready
	time.Sleep(200 * time.Millisecond)

	return c, cleanup, nil
}

// isMockMode returns true if we're in mock mode for non-systemd services
func isMockMode(sys SupervisionSystem) bool {
	return sys.Type != ServiceTypeSystemd && shouldUseMocks()
}
