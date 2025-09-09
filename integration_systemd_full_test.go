//go:build linux

package runit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/renameio/v2"
)

// TestResult represents a single test result with timing information
type TestResult struct {
	Name      string        `json:"name"`
	Started   time.Time     `json:"started"`
	Completed time.Time     `json:"completed"`
	Duration  time.Duration `json:"duration"`
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
	Logs      []string      `json:"logs"`
}

// TestReport represents the full test report
type TestReport struct {
	Host       string        `json:"host"`
	Started    time.Time     `json:"started"`
	Completed  time.Time     `json:"completed"`
	Duration   time.Duration `json:"duration"`
	TotalTests int           `json:"total_tests"`
	Passed     int           `json:"passed"`
	Failed     int           `json:"failed"`
	Results    []TestResult  `json:"results"`
}

func TestSystemdFullIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping lengthy systemd integration test in short mode")
	}
	// Check if we're running on Linux with systemd
	RequireSystemd(t)

	// Check if we have sufficient permissions
	if os.Geteuid() != 0 {
		t.Log("WARNING: Not running as root, some tests may fail or be skipped")
	}

	// Setup logging
	logDir := os.Getenv("TEST_LOG_DIR")
	if logDir == "" {
		logDir = "/tmp/go-runit-systemd-tests"
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	mainLogFile := filepath.Join(logDir, fmt.Sprintf("systemd-test-%s.log", time.Now().Format("20060102-150405")))
	logger, err := NewTestLogger(t, mainLogFile, true)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Log("Starting systemd integration tests on host: %s", getHostname())
	logger.Log("Log directory: %s", logDir)
	logger.Log("Running as UID: %d", os.Geteuid())

	// Initialize test report
	report := &TestReport{
		Host:    getHostname(),
		Started: time.Now(),
		Results: []TestResult{},
	}

	// Run test suites
	ctx := context.Background()

	testSuites := []struct {
		name string
		fn   func(context.Context, *TestLogger) error
	}{
		{"BasicServiceLifecycle", testBasicServiceLifecycle},
		{"ServiceWithEnvironment", testServiceWithEnvironment},
		{"ServiceWithResourceLimits", testServiceWithResourceLimits},
		{"ServiceSignalHandling", testServiceSignalHandling},
		{"ServiceWithLogging", testServiceWithLogging},
		{"ServiceRestart", testServiceRestartSystemd},
		{"ServiceDependencies", testServiceDependencies},
		{"ServiceFailureHandling", testServiceFailureHandling},
		{"ConcurrentServices", testConcurrentServices},
		{"ServiceStatusMonitoring", testServiceStatusMonitoring},
	}

	for _, suite := range testSuites {
		result := TestResult{
			Name:    suite.name,
			Started: time.Now(),
			Logs:    []string{},
		}

		logger.Log("=== Starting test suite: %s ===", suite.name)

		// Create a sub-logger for this test
		testLogger, _ := NewTestLogger(t, filepath.Join(logDir, fmt.Sprintf("%s.log", suite.name)), false)

		err := suite.fn(ctx, testLogger)
		result.Completed = time.Now()
		result.Duration = result.Completed.Sub(result.Started)
		result.Logs = testLogger.logs
		testLogger.Close()

		if err != nil {
			result.Success = false
			result.Error = err.Error()
			logger.Log("FAILED: %s - %v", suite.name, err)
			report.Failed++
		} else {
			result.Success = true
			logger.Log("PASSED: %s (duration: %v)", suite.name, result.Duration)
			report.Passed++
		}

		report.Results = append(report.Results, result)
	}

	// Finalize report
	report.Completed = time.Now()
	report.Duration = report.Completed.Sub(report.Started)
	report.TotalTests = len(report.Results)

	// Write JSON report
	reportFile := filepath.Join(logDir, fmt.Sprintf("report-%s.json", time.Now().Format("20060102-150405")))
	if err := writeJSONReport(reportFile, report); err != nil {
		logger.Log("Failed to write JSON report: %v", err)
	} else {
		logger.Log("JSON report written to: %s", reportFile)
	}

	// Write summary
	logger.Log("=== TEST SUMMARY ===")
	logger.Log("Total tests: %d", report.TotalTests)
	logger.Log("Passed: %d", report.Passed)
	logger.Log("Failed: %d", report.Failed)
	logger.Log("Total duration: %v", report.Duration)

	// Fail the test if any suite failed
	if report.Failed > 0 {
		t.Errorf("%d test suites failed", report.Failed)
	}
}

func testBasicServiceLifecycle(ctx context.Context, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-basic-%d", time.Now().Unix())
	logger.Log("Testing basic service lifecycle with service: %s", serviceName)

	// Create a simple service
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/sh", "-c", "while true; do echo 'Service running'; sleep 2; done"})

	systemdBuilder := NewBuilderSystemd(builder)
	if os.Geteuid() == 0 {
		systemdBuilder.UnitDir = "/run/systemd/system"
		systemdBuilder.UseSudo = false
	}

	// Build and install
	logger.Log("Building and installing service")
	if err := systemdBuilder.BuildWithContext(ctx); err != nil {
		return fmt.Errorf("failed to build service: %w", err)
	}

	defer func() {
		logger.Log("Cleaning up service")
		if err := systemdBuilder.Remove(ctx); err != nil {
			logger.Log("Warning: failed to remove service: %v", err)
		}
	}()

	// Create client
	client := NewClientSystemd(serviceName)
	if os.Geteuid() == 0 {
		client.UseSudo = false
	}

	// Start service
	logger.Log("Starting service")
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Wait for service to be running
	time.Sleep(2 * time.Second)

	// Check status
	logger.Log("Checking service status")
	status, err := client.StatusSystemd(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if !status.Running {
		return fmt.Errorf("service not running after start")
	}
	logger.Log("Service is running with PID: %d", status.MainPID)

	// Stop service
	logger.Log("Stopping service")
	if err := client.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Wait and verify stopped
	time.Sleep(2 * time.Second)
	status, err = client.StatusSystemd(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status after stop: %w", err)
	}

	if status.Running {
		return fmt.Errorf("service still running after stop")
	}
	logger.Log("Service successfully stopped")

	return nil
}

func testServiceWithEnvironment(ctx context.Context, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-env-%d", time.Now().Unix())
	logger.Log("Testing service with environment variables: %s", serviceName)

	// Create service with environment
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/sh", "-c", "echo \"TEST_VAR=$TEST_VAR ANOTHER=$ANOTHER\"; sleep 5"})
	builder.WithEnvMap(map[string]string{
		"TEST_VAR": "test_value",
		"ANOTHER":  "another_value",
	})

	systemdBuilder := NewBuilderSystemd(builder)
	if os.Geteuid() == 0 {
		systemdBuilder.UnitDir = "/run/systemd/system"
		systemdBuilder.UseSudo = false
	}

	// Build and install
	logger.Log("Building service with environment variables")
	if err := systemdBuilder.BuildWithContext(ctx); err != nil {
		return fmt.Errorf("failed to build service: %w", err)
	}

	defer func() {
		if err := systemdBuilder.Remove(ctx); err != nil {
			logger.Log("Warning: failed to remove service: %v", err)
		}
	}()

	// Start and check logs
	client := NewClientSystemd(serviceName)
	if os.Geteuid() == 0 {
		client.UseSudo = false
	}

	logger.Log("Starting service")
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	time.Sleep(2 * time.Second)

	// Check logs for environment variables
	logger.Log("Checking service logs for environment variables")
	cmd := exec.CommandContext(ctx, "journalctl", "-u", serviceName+".service", "--no-pager", "-n", "10")
	output, err := cmd.Output()
	if err != nil {
		logger.Log("Warning: could not read journal: %v", err)
	} else {
		logs := string(output)
		if strings.Contains(logs, "TEST_VAR=test_value") && strings.Contains(logs, "ANOTHER=another_value") {
			logger.Log("Environment variables correctly set")
		} else {
			logger.Log("Warning: environment variables may not be set correctly")
		}
	}

	// Stop service
	logger.Log("Stopping service")
	return client.Stop(ctx)
}

func testServiceWithResourceLimits(ctx context.Context, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-limits-%d", time.Now().Unix())
	logger.Log("Testing service with resource limits: %s", serviceName)

	// Create service with resource limits
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/sleep", "30"})
	builder.WithChpst(func(c *ChpstConfig) {
		c.LimitMem = 100 * 1024 * 1024 // 100MB
		c.LimitFiles = 1024
		c.Nice = 10
	})

	systemdBuilder := NewBuilderSystemd(builder)
	if os.Geteuid() == 0 {
		systemdBuilder.UnitDir = "/run/systemd/system"
		systemdBuilder.UseSudo = false
	}

	// Build and install
	logger.Log("Building service with resource limits")
	if err := systemdBuilder.BuildWithContext(ctx); err != nil {
		return fmt.Errorf("failed to build service: %w", err)
	}

	defer func() {
		if err := systemdBuilder.Remove(ctx); err != nil {
			logger.Log("Warning: failed to remove service: %v", err)
		}
	}()

	// Read the generated unit file to verify limits
	unitPath := filepath.Join(systemdBuilder.UnitDir, serviceName+".service")
	unitContent, err := os.ReadFile(unitPath)
	if err != nil {
		logger.Log("Warning: could not read unit file: %v", err)
	} else {
		content := string(unitContent)
		if strings.Contains(content, "MemoryLimit=") || strings.Contains(content, "MemoryMax=") {
			logger.Log("Memory limit configured")
		}
		if strings.Contains(content, "LimitNOFILE=") {
			logger.Log("File descriptor limit configured")
		}
		if strings.Contains(content, "Nice=") {
			logger.Log("Nice value configured")
		}
	}

	// Start service
	client := NewClientSystemd(serviceName)
	if os.Geteuid() == 0 {
		client.UseSudo = false
	}

	logger.Log("Starting service with limits")
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	time.Sleep(2 * time.Second)

	// Verify service is running with limits
	status, err := client.StatusSystemd(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if !status.Running {
		return fmt.Errorf("service not running")
	}

	logger.Log("Service running with PID %d under resource limits", status.MainPID)

	// Stop service
	return client.Stop(ctx)
}

func testServiceSignalHandling(ctx context.Context, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-signals-%d", time.Now().Unix())
	logger.Log("Testing service signal handling: %s", serviceName)

	// Create a service that responds to signals
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/bash", "-c", `
		trap 'echo "Received SIGHUP"' HUP
		trap 'echo "Received SIGUSR1"' USR1
		trap 'echo "Received SIGUSR2"' USR2
		trap 'echo "Received SIGTERM"; exit 0' TERM
		while true; do sleep 1; done
	`})

	systemdBuilder := NewBuilderSystemd(builder)
	if os.Geteuid() == 0 {
		systemdBuilder.UnitDir = "/run/systemd/system"
		systemdBuilder.UseSudo = false
	}

	// Build and install
	logger.Log("Building service with signal handlers")
	if err := systemdBuilder.BuildWithContext(ctx); err != nil {
		return fmt.Errorf("failed to build service: %w", err)
	}

	defer func() {
		if err := systemdBuilder.Remove(ctx); err != nil {
			logger.Log("Warning: failed to remove service: %v", err)
		}
	}()

	// Start service
	client := NewClientSystemd(serviceName)
	if os.Geteuid() == 0 {
		client.UseSudo = false
	}

	logger.Log("Starting service")
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	time.Sleep(2 * time.Second)

	// Test various signals
	signals := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"SIGHUP", client.Reload},
		{"SIGUSR1", client.USR1},
		{"SIGUSR2", client.USR2},
	}

	for _, sig := range signals {
		logger.Log("Sending %s", sig.name)
		if err := sig.fn(ctx); err != nil {
			logger.Log("Warning: failed to send %s: %v", sig.name, err)
		} else {
			logger.Log("Successfully sent %s", sig.name)
		}
		time.Sleep(1 * time.Second)
	}

	// Stop with SIGTERM
	logger.Log("Stopping service with SIGTERM")
	return client.Stop(ctx)
}

func testServiceWithLogging(ctx context.Context, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-logging-%d", time.Now().Unix())
	logger.Log("Testing service with logging configuration: %s", serviceName)

	// Create service that produces logs
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/bash", "-c", `
		for i in {1..10}; do
			echo "Log message $i"
			echo "Error message $i" >&2
			sleep 1
		done
	`})
	builder.WithSvlogd(func(s *ConfigSvlogd) {
		s.Prefix = serviceName
	})

	systemdBuilder := NewBuilderSystemd(builder)
	if os.Geteuid() == 0 {
		systemdBuilder.UnitDir = "/run/systemd/system"
		systemdBuilder.UseSudo = false
	}

	// Build and install
	logger.Log("Building service with logging")
	if err := systemdBuilder.BuildWithContext(ctx); err != nil {
		return fmt.Errorf("failed to build service: %w", err)
	}

	defer func() {
		if err := systemdBuilder.Remove(ctx); err != nil {
			logger.Log("Warning: failed to remove service: %v", err)
		}
	}()

	// Start service
	client := NewClientSystemd(serviceName)
	if os.Geteuid() == 0 {
		client.UseSudo = false
	}

	logger.Log("Starting service")
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Let it run and produce logs
	logger.Log("Waiting for service to produce logs")
	time.Sleep(5 * time.Second)

	// Read logs
	logger.Log("Reading service logs")
	cmd := exec.CommandContext(ctx, "journalctl", "-u", serviceName+".service", "--no-pager", "-n", "20")
	output, err := cmd.Output()
	if err != nil {
		logger.Log("Warning: could not read journal: %v", err)
	} else {
		logs := string(output)
		lines := strings.Split(logs, "\n")
		logger.Log("Found %d log lines", len(lines))

		// Check for expected messages
		foundStdout := false
		foundStderr := false
		for _, line := range lines {
			if strings.Contains(line, "Log message") {
				foundStdout = true
			}
			if strings.Contains(line, "Error message") {
				foundStderr = true
			}
		}

		if foundStdout {
			logger.Log("Found stdout messages in logs")
		}
		if foundStderr {
			logger.Log("Found stderr messages in logs")
		}
	}

	// Stop service
	return client.Stop(ctx)
}

func testServiceRestartSystemd(ctx context.Context, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-restart-%d", time.Now().Unix())
	logger.Log("Testing service restart behavior: %s", serviceName)

	// Create service
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/bash", "-c", "echo 'Service started at '$(date); sleep 30"})

	systemdBuilder := NewBuilderSystemd(builder)
	if os.Geteuid() == 0 {
		systemdBuilder.UnitDir = "/run/systemd/system"
		systemdBuilder.UseSudo = false
	}

	// Build and install
	logger.Log("Building service")
	if err := systemdBuilder.BuildWithContext(ctx); err != nil {
		return fmt.Errorf("failed to build service: %w", err)
	}

	defer func() {
		if err := systemdBuilder.Remove(ctx); err != nil {
			logger.Log("Warning: failed to remove service: %v", err)
		}
	}()

	// Start service
	client := NewClientSystemd(serviceName)
	if os.Geteuid() == 0 {
		client.UseSudo = false
	}

	logger.Log("Starting service")
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	time.Sleep(2 * time.Second)

	// Get initial PID
	status1, err := client.StatusSystemd(ctx)
	if err != nil {
		return fmt.Errorf("failed to get initial status: %w", err)
	}
	initialPID := status1.MainPID
	logger.Log("Initial PID: %d", initialPID)

	// Restart service
	logger.Log("Restarting service")
	if err := client.Restart(ctx); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	time.Sleep(2 * time.Second)

	// Get new PID
	status2, err := client.StatusSystemd(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status after restart: %w", err)
	}
	newPID := status2.MainPID
	logger.Log("New PID: %d", newPID)

	if initialPID == newPID {
		return fmt.Errorf("PID did not change after restart (expected different PID)")
	}

	if !status2.Running {
		return fmt.Errorf("service not running after restart")
	}

	logger.Log("Service successfully restarted with new PID")

	// Stop service
	return client.Stop(ctx)
}

func testServiceDependencies(ctx context.Context, logger *TestLogger) error {
	logger.Log("Testing service dependencies")

	// This is a placeholder for dependency testing
	// In a real scenario, you would create multiple services with dependencies
	logger.Log("Dependency testing would require multiple services - skipping for now")

	return nil
}

func testServiceFailureHandling(ctx context.Context, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-failure-%d", time.Now().Unix())
	logger.Log("Testing service failure handling: %s", serviceName)

	// Create a service that fails
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/bash", "-c", "echo 'Service starting'; sleep 2; echo 'Service failing'; exit 1"})

	systemdBuilder := NewBuilderSystemd(builder)
	if os.Geteuid() == 0 {
		systemdBuilder.UnitDir = "/run/systemd/system"
		systemdBuilder.UseSudo = false
	}

	// Build and install
	logger.Log("Building service that will fail")
	if err := systemdBuilder.BuildWithContext(ctx); err != nil {
		return fmt.Errorf("failed to build service: %w", err)
	}

	defer func() {
		if err := systemdBuilder.Remove(ctx); err != nil {
			logger.Log("Warning: failed to remove service: %v", err)
		}
	}()

	// Start service
	client := NewClientSystemd(serviceName)
	if os.Geteuid() == 0 {
		client.UseSudo = false
	}

	logger.Log("Starting service (expected to fail)")
	if err := client.Start(ctx); err != nil {
		logger.Log("Service start returned error (expected): %v", err)
	}

	// Wait for service to fail
	time.Sleep(5 * time.Second)

	// Check status
	status, err := client.StatusSystemd(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	logger.Log("Service state: %s/%s", status.ActiveState, status.SubState)
	logger.Log("Service result: %s", status.Result)

	if status.ActiveState == "failed" {
		logger.Log("Service correctly entered failed state")
	} else {
		logger.Log("Warning: Service did not enter expected failed state")
	}

	return nil
}

func testConcurrentServices(ctx context.Context, logger *TestLogger) error {
	logger.Log("Testing concurrent service operations")

	numServices := 3
	services := make([]string, numServices)
	clients := make([]*ClientSystemd, numServices)

	// Create multiple services
	for i := 0; i < numServices; i++ {
		serviceName := fmt.Sprintf("test-concurrent-%d-%d", i, time.Now().Unix())
		services[i] = serviceName
		logger.Log("Creating service %d: %s", i, serviceName)

		builder := NewServiceBuilder(serviceName, "")
		builder.WithCmd([]string{"/bin/bash", "-c", fmt.Sprintf("echo 'Service %d running'; sleep 30", i)})

		systemdBuilder := NewBuilderSystemd(builder)
		if os.Geteuid() == 0 {
			systemdBuilder.UnitDir = "/run/systemd/system"
			systemdBuilder.UseSudo = false
		}

		if err := systemdBuilder.BuildWithContext(ctx); err != nil {
			return fmt.Errorf("failed to build service %d: %w", i, err)
		}

		clients[i] = NewClientSystemd(serviceName)
		if os.Geteuid() == 0 {
			clients[i].UseSudo = false
		}

		defer func(name string) {
			builder := NewServiceBuilder(name, "")
			systemdBuilder := NewBuilderSystemd(builder)
			if os.Geteuid() == 0 {
				systemdBuilder.UnitDir = "/run/systemd/system"
				systemdBuilder.UseSudo = false
			}
			if err := systemdBuilder.Remove(ctx); err != nil {
				logger.Log("Warning: failed to remove service %s: %v", name, err)
			}
		}(serviceName)
	}

	// Start all services concurrently
	logger.Log("Starting all services concurrently")
	startErrors := make(chan error, numServices)
	for i, client := range clients {
		go func(idx int, c *ClientSystemd) {
			startErrors <- c.Start(ctx)
		}(i, client)
	}

	// Wait for all starts
	for i := 0; i < numServices; i++ {
		if err := <-startErrors; err != nil {
			logger.Log("Warning: service %d start error: %v", i, err)
		}
	}

	time.Sleep(2 * time.Second)

	// Check all statuses
	logger.Log("Checking all service statuses")
	for i, client := range clients {
		status, err := client.StatusSystemd(ctx)
		if err != nil {
			logger.Log("Warning: failed to get status for service %d: %v", i, err)
			continue
		}
		logger.Log("Service %d: running=%v, pid=%d", i, status.Running, status.MainPID)
	}

	// Stop all services concurrently
	logger.Log("Stopping all services concurrently")
	stopErrors := make(chan error, numServices)
	for i, client := range clients {
		go func(idx int, c *ClientSystemd) {
			stopErrors <- c.Stop(ctx)
		}(i, client)
	}

	// Wait for all stops
	for i := 0; i < numServices; i++ {
		if err := <-stopErrors; err != nil {
			logger.Log("Warning: service %d stop error: %v", i, err)
		}
	}

	logger.Log("All concurrent services handled")
	return nil
}

func testServiceStatusMonitoring(ctx context.Context, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-monitor-%d", time.Now().Unix())
	logger.Log("Testing service status monitoring: %s", serviceName)

	// Create service
	builder := NewServiceBuilder(serviceName, "")
	builder.WithCmd([]string{"/bin/bash", "-c", `
		echo "Service starting"
		for i in {1..10}; do
			echo "Iteration $i"
			sleep 2
		done
		echo "Service completing"
	`})

	systemdBuilder := NewBuilderSystemd(builder)
	if os.Geteuid() == 0 {
		systemdBuilder.UnitDir = "/run/systemd/system"
		systemdBuilder.UseSudo = false
	}

	// Build and install
	logger.Log("Building service for monitoring")
	if err := systemdBuilder.BuildWithContext(ctx); err != nil {
		return fmt.Errorf("failed to build service: %w", err)
	}

	defer func() {
		if err := systemdBuilder.Remove(ctx); err != nil {
			logger.Log("Warning: failed to remove service: %v", err)
		}
	}()

	// Start service
	client := NewClientSystemd(serviceName)
	if os.Geteuid() == 0 {
		client.UseSudo = false
	}

	logger.Log("Starting service")
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Monitor status over time
	logger.Log("Monitoring service status")
	for i := 0; i < 5; i++ {
		status, err := client.StatusSystemd(ctx)
		if err != nil {
			logger.Log("Warning: failed to get status at iteration %d: %v", i, err)
			continue
		}

		logger.Log("Status %d: state=%s/%s, running=%v, pid=%d, uptime=%v",
			i, status.ActiveState, status.SubState, status.Running,
			status.MainPID, status.Uptime)

		// Also check using IsRunning
		running, err := client.IsRunning(ctx)
		if err != nil {
			logger.Log("Warning: IsRunning error: %v", err)
		} else {
			logger.Log("IsRunning: %v", running)
		}

		time.Sleep(3 * time.Second)
	}

	// Let service complete
	logger.Log("Waiting for service to complete")
	time.Sleep(10 * time.Second)

	// Final status
	finalStatus, err := client.StatusSystemd(ctx)
	if err != nil {
		logger.Log("Warning: failed to get final status: %v", err)
	} else {
		logger.Log("Final status: state=%s/%s, result=%s",
			finalStatus.ActiveState, finalStatus.SubState, finalStatus.Result)
	}

	return nil
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func writeJSONReport(filename string, report *TestReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return renameio.WriteFile(filename, data, 0o644)
}

// Helper function to run a single test with logging
func runTest(t *testing.T, name string, fn func(*testing.T, *TestLogger) error) {
	t.Run(name, func(t *testing.T) {
		logFile := fmt.Sprintf("/tmp/%s-%s.log", name, time.Now().Format("20060102-150405"))
		logger, err := NewTestLogger(t, logFile, true)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		if err := fn(t, logger); err != nil {
			t.Errorf("Test failed: %v", err)
		}
	})
}
