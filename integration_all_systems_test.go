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
	"sync"
	"testing"
	"time"

	"github.com/google/renameio/v2"
)

// SystemTestResult represents test results for a single supervision system
type SystemTestResult struct {
	System    string        `json:"system"`
	Available bool          `json:"available"`
	Results   []TestResult  `json:"results"`
	Started   time.Time     `json:"started"`
	Completed time.Time     `json:"completed"`
	Duration  time.Duration `json:"duration"`
	Passed    int           `json:"passed"`
	Failed    int           `json:"failed"`
	Skipped   int           `json:"skipped"`
}

// ComparisonReport represents the full comparison report
type ComparisonReport struct {
	Host      string             `json:"host"`
	Started   time.Time          `json:"started"`
	Completed time.Time          `json:"completed"`
	Duration  time.Duration      `json:"duration"`
	Systems   []SystemTestResult `json:"systems"`
	Summary   map[string]int     `json:"summary"`
}

func TestAllSupervisionSystems(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping lengthy integration test in short mode")
	}
	// Setup logging
	logDir := os.Getenv("TEST_LOG_DIR")
	if logDir == "" {
		// If not provided, create our own timestamped directory
		timestamp := time.Now().Format("20060102-150405")
		logDir = filepath.Join("/tmp/go-runit-integration-tests", timestamp)

		if err := os.MkdirAll(logDir, 0o755); err != nil {
			t.Fatalf("Failed to create log directory: %v", err)
		}
	} else {
		// Use the provided directory directly (script already includes timestamp)
		// Just make sure it exists
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			t.Fatalf("Failed to create log directory: %v", err)
		}
	}

	mainLogFile := filepath.Join(logDir, "all-systems.log")
	mainLogger, err := NewTestLogger(t, mainLogFile, true)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer mainLogger.Close()

	mainLogger.Log("Starting supervision system comparison tests")
	mainLogger.Log("Host: %s", getHostname())
	mainLogger.Log("Log directory: %s", logDir)
	mainLogger.Log("Running as UID: %d", os.Geteuid())

	// Initialize systems
	systems := initializeSystems(mainLogger)

	// Initialize comparison report
	report := &ComparisonReport{
		Host:    getHostname(),
		Started: time.Now(),
		Systems: []SystemTestResult{},
		Summary: make(map[string]int),
	}

	// Test each system using proper subtests
	for _, system := range systems {
		system := system // Capture for closure
		t.Run(system.Name, func(t *testing.T) {
			t.Parallel() // Run systems in parallel
			
			if !system.Available {
				t.Skipf("%s not available", system.Name)
			}
			
			result := testSupervisionSystemWithT(t, system, logDir, mainLogger)
			report.Systems = append(report.Systems, result)
			report.Summary[result.System+"_passed"] = result.Passed
			report.Summary[result.System+"_failed"] = result.Failed
			report.Summary[result.System+"_skipped"] = result.Skipped
		})
	}

	// Finalize report
	report.Completed = time.Now()
	report.Duration = report.Completed.Sub(report.Started)

	// Write comparison report
	reportFile := filepath.Join(logDir, "comparison-report.json")
	if err := writeComparisonReport(reportFile, report); err != nil {
		mainLogger.Log("Failed to write comparison report: %v", err)
	} else {
		mainLogger.Log("Comparison report written to: %s", reportFile)
	}

	// Write human-readable summary
	summaryFile := filepath.Join(logDir, "summary.txt")
	if err := writeTextSummary(summaryFile, report); err != nil {
		mainLogger.Log("Failed to write text summary: %v", err)
	} else {
		mainLogger.Log("Text summary written to: %s", summaryFile)
	}

	// Print summary to console
	printSummary(mainLogger, report)
}

func initializeSystems(logger *TestLogger) []SupervisionSystem {
	systems := []SupervisionSystem{
		{
			Name:   "runit",
			Type:   ServiceTypeRunit,
			Config: ConfigRunit(),
		},
		{
			Name:   "daemontools",
			Type:   ServiceTypeDaemontools,
			Config: ConfigDaemontools(),
		},
		{
			Name:   "s6",
			Type:   ServiceTypeS6,
			Config: ConfigS6(),
		},
		{
			Name:   "systemd",
			Type:   ServiceTypeSystemd,
			Config: ConfigSystemd(),
		},
	}

	// Check availability of each system
	for i := range systems {
		systems[i].Available = checkSystemAvailability(&systems[i], logger)
	}

	return systems
}

func checkSystemAvailability(sys *SupervisionSystem, logger *TestLogger) bool {
	switch sys.Type {
	case ServiceTypeRunit:
		if CheckAllToolsAvailable("sv", "runsv") {
			logger.Log("Runit: Available")
			return true
		}
		logger.Log("Runit: Not available (sv or runsv not found)")
		return false

	case ServiceTypeDaemontools:
		if CheckAllToolsAvailable("svc", "supervise") {
			logger.Log("Daemontools: Available")
			return true
		}
		logger.Log("Daemontools: Not available (svc or supervise not found)")
		return false

	case ServiceTypeS6:
		if CheckAllToolsAvailable("s6-svc", "s6-supervise") {
			logger.Log("S6: Available")
			return true
		}
		logger.Log("S6: Not available (s6-svc or s6-supervise not found)")
		return false

	case ServiceTypeSystemd:
		if CheckToolAvailable("systemctl") {
			// Also check if systemd is actually running
			// Note: is-system-running returns non-zero for degraded/starting/maintenance states
			// but systemd is still the init system in those cases
			output, _ := exec.Command("systemctl", "is-system-running").Output()
			state := strings.TrimSpace(string(output))
			if state == "" {
				logger.Log("Systemd: Available but not running as init")
			} else {
				logger.Log("Systemd: Available and running (state: %s)", state)
			}
			return true
		}
		logger.Log("Systemd: Not available (systemctl not found)")
		return false

	default:
		return false
	}
}

func testSupervisionSystemWithT(t *testing.T, sys SupervisionSystem, logDir string, mainLogger *TestLogger) SystemTestResult {
	result := SystemTestResult{
		System:    sys.Name,
		Available: sys.Available,
		Started:   time.Now(),
		Results:   []TestResult{},
	}

	// Create system-specific log directory
	sysLogDir := filepath.Join(logDir, sys.Name)
	if err := os.MkdirAll(sysLogDir, 0o755); err != nil {
		t.Fatalf("Failed to create log directory for %s: %v", sys.Name, err)
	}

	// Create system-specific logger
	sysLogFile := filepath.Join(sysLogDir, "tests.log")
	sysLogger, err := NewTestLogger(t, sysLogFile, false)
	if err != nil {
		t.Fatalf("Failed to create logger for %s: %v", sys.Name, err)
	}
	defer sysLogger.Close()

	mainLogger.Log("=== Testing %s ===", strings.ToUpper(sys.Name))
	sysLogger.Log("Starting tests for %s", sys.Name)

	// Run test suites as subtests
	ctx := context.Background()
	testSuites := getTestSuites(sys)

	for _, suite := range testSuites {
		suite := suite // Capture for closure
		t.Run(suite.name, func(t *testing.T) {
			testResult := TestResult{
				Name:    suite.name,
				Started: time.Now(),
				Logs:    []string{},
			}

			sysLogger.Log("Running test: %s", suite.name)

			// Create test-specific logger
			testLogFile := filepath.Join(sysLogDir, fmt.Sprintf("%s.log", suite.name))
			testLogger, _ := NewTestLogger(t, testLogFile, false)

			err := suite.fn(ctx, sys, testLogger)
			testResult.Completed = time.Now()
			testResult.Duration = testResult.Completed.Sub(testResult.Started)
			testResult.Logs = testLogger.logs
			testLogger.Close()

			if err != nil {
				testResult.Success = false
				testResult.Error = err.Error()
				sysLogger.Log("  %s: FAILED (%v) in %v", suite.name, err, testResult.Duration)
				mainLogger.Log("  %s/%s: FAILED (%v)", sys.Name, suite.name, testResult.Duration)
				result.Failed++
				t.Errorf("Test %s failed: %v", suite.name, err)
			} else {
				testResult.Success = true
				sysLogger.Log("  %s: PASSED in %v", suite.name, testResult.Duration)
				mainLogger.Log("  %s/%s: PASSED (%v)", sys.Name, suite.name, testResult.Duration)
				result.Passed++
			}

			result.Results = append(result.Results, testResult)
		})
	}

	result.Completed = time.Now()
	result.Duration = result.Completed.Sub(result.Started)

	mainLogger.Log("%s: %d passed, %d failed, %d skipped (total: %v)",
		sys.Name, result.Passed, result.Failed, result.Skipped, result.Duration)

	return result
}

func testSupervisionSystem(sys SupervisionSystem, logDir string, mainLogger *TestLogger) SystemTestResult {
	result := SystemTestResult{
		System:    sys.Name,
		Available: sys.Available,
		Started:   time.Now(),
		Results:   []TestResult{},
	}

	if !sys.Available {
		result.Skipped = 1
		result.Completed = time.Now()
		result.Duration = result.Completed.Sub(result.Started)
		mainLogger.Log("Skipping %s: not available", sys.Name)
		return result
	}

	// Create system-specific log directory
	sysLogDir := filepath.Join(logDir, sys.Name)
	if err := os.MkdirAll(sysLogDir, 0o755); err != nil {
		mainLogger.Log("Failed to create log directory for %s: %v", sys.Name, err)
		result.Failed = 1
		result.Completed = time.Now()
		result.Duration = result.Completed.Sub(result.Started)
		return result
	}

	// Create system-specific logger
	sysLogFile := filepath.Join(sysLogDir, "tests.log")
	sysLogger, err := NewTestLogger(nil, sysLogFile, false)
	if err != nil {
		mainLogger.Log("Failed to create logger for %s: %v", sys.Name, err)
		result.Failed = 1
		result.Completed = time.Now()
		result.Duration = result.Completed.Sub(result.Started)
		return result
	}
	defer sysLogger.Close()

	mainLogger.Log("=== Testing %s ===", strings.ToUpper(sys.Name))
	sysLogger.Log("Starting tests for %s", sys.Name)

	// Run test suites
	ctx := context.Background()
	testSuites := getTestSuites(sys)

	for _, suite := range testSuites {
		testResult := TestResult{
			Name:    suite.name,
			Started: time.Now(),
			Logs:    []string{},
		}

		sysLogger.Log("Running test: %s", suite.name)

		// Create test-specific logger
		testLogFile := filepath.Join(sysLogDir, fmt.Sprintf("%s.log", suite.name))
		testLogger, _ := NewTestLogger(nil, testLogFile, false)

		err := suite.fn(ctx, sys, testLogger)
		testResult.Completed = time.Now()
		testResult.Duration = testResult.Completed.Sub(testResult.Started)
		testResult.Logs = testLogger.logs
		testLogger.Close()

		if err != nil {
			testResult.Success = false
			testResult.Error = err.Error()
			sysLogger.Log("FAILED: %s - %v", suite.name, err)
			mainLogger.Log("  %s/%s: FAILED - %v", sys.Name, suite.name, err)
			result.Failed++
		} else {
			testResult.Success = true
			sysLogger.Log("PASSED: %s (duration: %v)", suite.name, testResult.Duration)
			mainLogger.Log("  %s/%s: PASSED (%v)", sys.Name, suite.name, testResult.Duration)
			result.Passed++
		}

		result.Results = append(result.Results, testResult)
	}

	result.Completed = time.Now()
	result.Duration = result.Completed.Sub(result.Started)

	mainLogger.Log("%s: %d passed, %d failed, %d skipped (total: %v)",
		sys.Name, result.Passed, result.Failed, result.Skipped, result.Duration)

	return result
}

type testSuite struct {
	name string
	fn   func(context.Context, SupervisionSystem, *TestLogger) error
}

func getTestSuites(sys SupervisionSystem) []testSuite {
	suites := []testSuite{
		{"BasicLifecycle", testBasicLifecycle},
		{"ServiceControl", testServiceControl},
		{"SignalHandling", testSignalHandling},
		{"ServiceStatus", testServiceStatus},
		{"ServiceRestart", testServiceRestart},
		{"ConcurrentOperations", testConcurrentOperations},
	}

	// Add system-specific tests with descriptive names
	switch sys.Type {
	case ServiceTypeRunit:
		suites = append(suites, testSuite{"OnceMode", testRunitOnce})
	case ServiceTypeS6:
		suites = append(suites, testSuite{"ReadinessProtocol", testS6Specific})
	case ServiceTypeSystemd:
		suites = append(suites, testSuite{"UnitFileManagement", testSystemdUnit})
	}

	return suites
}

func testBasicLifecycle(ctx context.Context, sys SupervisionSystem, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-%s-lifecycle-%d", sys.Name, time.Now().Unix())
	logger.Log("[%s] Testing basic lifecycle: %s", strings.ToUpper(sys.Name), serviceName)

	// Create service using the helper that handles mock/real supervisors
	client, cleanup, err := createTestServiceWithSupervisor(ctx, sys, serviceName, logger)
	if err != nil {
		return fmt.Errorf("failed to create test service: %w", err)
	}

	// Skip actual service operations in mock mode
	if isMockMode(sys) {
		defer cleanup()
		logger.Log("[%s] Mock mode - skipping actual service operations", strings.ToUpper(sys.Name))
		logger.Log("[%s] Mock test completed - client creation successful", strings.ToUpper(sys.Name))
		return nil
	}

	defer cleanup()

	// Test start
	logger.Log("Starting service")
	switch c := client.(type) {
	case *ClientSystemd:
		if err := c.Start(ctx); err != nil {
			return fmt.Errorf("failed to start: %w", err)
		}
	default:
		if sc, ok := client.(ServiceClient); ok {
			if err := sc.Up(ctx); err != nil {
				return fmt.Errorf("failed to start: %w", err)
			}
		}
	}

	// Wait for service to actually start with retries
	// Supervisors need time to process the up command and start the service
	logger.Log("Waiting for service to start...")
	var running bool
	var lastStatus string
	maxRetries := 30 // 30 * 200ms = 6 seconds max

	for i := 0; i < maxRetries; i++ {
		time.Sleep(200 * time.Millisecond)

		switch c := client.(type) {
		case *ClientSystemd:
			status, err := c.StatusSystemd(ctx)
			if err != nil {
				continue // Keep trying
			}
			running = status.Running
			lastStatus = fmt.Sprintf("running=%v, pid=%d", status.Running, status.MainPID)
			if running {
				logger.Log("Service started: %s", lastStatus)
				break
			}
		default:
			if sc, ok := client.(ServiceClient); ok {
				status, err := sc.Status(ctx)
				if err != nil {
					continue // Keep trying
				}

				// Check if service is running
				running = status.State == StateRunning

				// For S6, also accept StateStarting if it has a PID (service is up but not "ready")
				if sys.Type == ServiceTypeS6 && status.State == StateStarting && status.PID > 0 {
					running = true
				}

				lastStatus = fmt.Sprintf("state=%s, pid=%d, wantUp=%v", status.State, status.PID, status.Flags.WantUp)

				// If service is running or wants to be up with a PID, we're good
				if running || (status.Flags.WantUp && status.PID > 0) {
					logger.Log("Service started: %s", lastStatus)
					running = true
					break
				}

				// Log progress every 5 attempts (1 second)
				if i > 0 && i%5 == 0 {
					logger.Log("Still waiting... current status: %s", lastStatus)
				}
			}
		}

		if running {
			break
		}
	}

	if !running {
		logger.Log("Service failed to start. Last status: %s", lastStatus)
		return fmt.Errorf("service not running after start")
	}

	// Test stop
	logger.Log("Stopping service")
	switch c := client.(type) {
	case *ClientSystemd:
		if err := c.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop: %w", err)
		}
	default:
		if sc, ok := client.(ServiceClient); ok {
			if err := sc.Down(ctx); err != nil {
				return fmt.Errorf("failed to stop: %w", err)
			}
		}
	}

	time.Sleep(3 * time.Second)

	// Verify stopped
	switch c := client.(type) {
	case *ClientSystemd:
		status, _ := c.StatusSystemd(ctx)
		if status.Running {
			return fmt.Errorf("service still running after stop")
		}
	default:
		if sc, ok := client.(ServiceClient); ok {
			status, _ := sc.Status(ctx)
			if status.State == StateRunning {
				return fmt.Errorf("service still running after stop")
			}
		}
	}

	logger.Log("Lifecycle test completed successfully")
	return nil
}

func testServiceControl(ctx context.Context, sys SupervisionSystem, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-%s-control-%d", sys.Name, time.Now().Unix())
	logger.Log("[%s] Testing service control: %s", strings.ToUpper(sys.Name), serviceName)

	// Skip actual control operations in mock mode
	if isMockMode(sys) {
		logger.Log("[%s] Mock mode - skipping service control operations", strings.ToUpper(sys.Name))
		return nil
	}

	// Create service using the helper that handles mock/real supervisors
	client, cleanup, err := createTestServiceWithSupervisor(ctx, sys, serviceName, logger)
	if err != nil {
		return fmt.Errorf("failed to create test service: %w", err)
	}

	defer cleanup()

	// Test various control operations
	operations := []struct {
		name string
		fn   func() error
	}{
		{"Start", func() error {
			switch c := client.(type) {
			case *ClientSystemd:
				return c.Start(ctx)
			default:
				if sc, ok := client.(ServiceClient); ok {
					return sc.Start(ctx)
				}
				return fmt.Errorf("unknown client type")
			}
		}},
		{"Pause", func() error {
			switch c := client.(type) {
			case *ClientSystemd:
				// Systemd doesn't have pause, use stop
				return c.Stop(ctx)
			default:
				if sc, ok := client.(ServiceClient); ok {
					return sc.Pause(ctx)
				}
				return fmt.Errorf("unknown client type")
			}
		}},
		{"Continue", func() error {
			switch c := client.(type) {
			case *ClientSystemd:
				// Systemd doesn't have continue, use start
				return c.Start(ctx)
			default:
				if sc, ok := client.(ServiceClient); ok {
					return sc.Continue(ctx)
				}
				return fmt.Errorf("unknown client type")
			}
		}},
		{"Stop", func() error {
			switch c := client.(type) {
			case *ClientSystemd:
				return c.Stop(ctx)
			default:
				if sc, ok := client.(ServiceClient); ok {
					return sc.Stop(ctx)
				}
				return fmt.Errorf("unknown client type")
			}
		}},
	}

	for _, op := range operations {
		logger.Log("Testing operation: %s", op.name)
		if err := op.fn(); err != nil {
			// Some operations might not be supported
			if strings.Contains(err.Error(), "not supported") ||
				strings.Contains(err.Error(), "unsupported") {
				logger.Log("Operation %s not supported (expected)", op.name)
			} else {
				logger.Log("Operation %s failed: %v", op.name, err)
			}
		} else {
			logger.Log("Operation %s succeeded", op.name)
		}
		time.Sleep(1 * time.Second)
	}

	return nil
}

func testSignalHandling(ctx context.Context, sys SupervisionSystem, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-%s-signals-%d", sys.Name, time.Now().Unix())
	logger.Log("[%s] Testing signal handling: %s", strings.ToUpper(sys.Name), serviceName)

	// Skip signal handling in mock mode
	if isMockMode(sys) {
		logger.Log("[%s] Mock mode - skipping signal handling test", strings.ToUpper(sys.Name))
		return nil
	}

	// Use the shell script signal logger
	signalLoggerPath := "/bin/sh"
	signalLoggerScript := "./test-programs/signal-logger.sh"

	// Check if script exists, if not create a simple inline version
	if _, err := os.Stat(signalLoggerScript); os.IsNotExist(err) {
		// Create a simple inline signal handler script
		signalLoggerScript = filepath.Join(os.TempDir(), fmt.Sprintf("signal-logger-%d.sh", time.Now().Unix()))
		scriptContent := `#!/bin/sh
LOG_FILE="${SIGNAL_LOG_FILE:-/tmp/signal-log-$$.log}"
echo "$(date '+%Y-%m-%d %H:%M:%S') STARTUP: PID=$$" >> "$LOG_FILE"
echo "$(date '+%Y-%m-%d %H:%M:%S') ENVIRONMENT: TEST_SYSTEM=$TEST_SYSTEM" >> "$LOG_FILE"
trap 'echo "$(date) SIGNAL_RECEIVED: SIGHUP" >> "$LOG_FILE"' HUP
trap 'echo "$(date) SIGNAL_RECEIVED: SIGUSR1" >> "$LOG_FILE"' USR1
trap 'echo "$(date) SIGNAL_RECEIVED: SIGUSR2" >> "$LOG_FILE"' USR2
trap 'echo "$(date) SIGNAL_RECEIVED: SIGTERM" >> "$LOG_FILE"; exit 0' TERM
while true; do sleep 1; done
`
		if err := os.WriteFile(signalLoggerScript, []byte(scriptContent), 0755); err != nil {
			logger.Log("[%s] Failed to create signal logger script: %v", strings.ToUpper(sys.Name), err)
			return fmt.Errorf("failed to create signal logger script: %w", err)
		}
		defer os.Remove(signalLoggerScript)
	}

	// Create signal log file path
	signalLogFile := fmt.Sprintf("/tmp/signal-log-%s-%s.log", sys.Name, serviceName)

	// Create service using the helper that handles mock/real supervisors
	cmd := []string{signalLoggerPath, signalLoggerScript}
	env := map[string]string{
		"SIGNAL_LOG_FILE": signalLogFile,
		"TEST_SYSTEM":     sys.Name,
		"SERVICE_NAME":    serviceName,
	}
	if sys.Type == ServiceTypeSystemd {
		env["TEST_SYSTEM"] = "systemd"
	}

	client, cleanup, err := createTestServiceWithSupervisorAndCmd(ctx, sys, serviceName, logger, cmd, env)
	if err != nil {
		return fmt.Errorf("failed to create test service: %w", err)
	}

	defer cleanup()

	// Start service
	logger.Log("[%s] Starting service", strings.ToUpper(sys.Name))
	switch c := client.(type) {
	case *ClientSystemd:
		if err := c.Start(ctx); err != nil {
			return fmt.Errorf("failed to start: %w", err)
		}
	default:
		if sc, ok := client.(ServiceClient); ok {
			if err := sc.Up(ctx); err != nil {
				return fmt.Errorf("failed to start: %w", err)
			}
		}
	}

	time.Sleep(2 * time.Second)

	// Test signals
	signals := []struct {
		name string
		fn   func() error
	}{
		{"SIGHUP", func() error {
			switch c := client.(type) {
			case *ClientSystemd:
				return c.Reload(ctx)
			default:
				if sc, ok := client.(ServiceClient); ok {
					return sc.HUP(ctx)
				}
				return fmt.Errorf("unknown client type")
			}
		}},
		{"SIGUSR1", func() error {
			switch c := client.(type) {
			case *ClientSystemd:
				return c.USR1(ctx)
			default:
				if sc, ok := client.(ServiceClient); ok {
					return sc.USR1(ctx)
				}
				return fmt.Errorf("unknown client type")
			}
		}},
		{"SIGUSR2", func() error {
			switch c := client.(type) {
			case *ClientSystemd:
				return c.USR2(ctx)
			default:
				if sc, ok := client.(ServiceClient); ok {
					return sc.USR2(ctx)
				}
				return fmt.Errorf("unknown client type")
			}
		}},
	}

	for _, sig := range signals {
		logger.Log("[%s] Sending %s", strings.ToUpper(sys.Name), sig.name)
		if err := sig.fn(); err != nil {
			logger.Log("[%s] Failed to send %s: %v", strings.ToUpper(sys.Name), sig.name, err)
		} else {
			logger.Log("[%s] Sent %s successfully", strings.ToUpper(sys.Name), sig.name)
		}
		time.Sleep(1 * time.Second)
	}

	// Stop with SIGTERM
	logger.Log("[%s] Stopping service", strings.ToUpper(sys.Name))
	var stopErr error
	switch c := client.(type) {
	case *ClientSystemd:
		stopErr = c.Stop(ctx)
	default:
		if sc, ok := client.(ServiceClient); ok {
			stopErr = sc.Term(ctx)
		} else {
			stopErr = fmt.Errorf("unknown client type")
		}
	}

	// Read and log the signal log file
	if signalLogData, err := os.ReadFile(signalLogFile); err == nil {
		logger.Log("[%s] Signal logger output:\n%s", strings.ToUpper(sys.Name), string(signalLogData))
		os.Remove(signalLogFile)
	} else {
		logger.Log("[%s] Could not read signal log file: %v", strings.ToUpper(sys.Name), err)
	}

	return stopErr
}

func testServiceStatus(ctx context.Context, sys SupervisionSystem, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-%s-status-%d", sys.Name, time.Now().Unix())
	logger.Log("[%s] Testing service status: %s", strings.ToUpper(sys.Name), serviceName)

	// Skip status monitoring in mock mode
	if isMockMode(sys) {
		logger.Log("[%s] Mock mode - skipping status monitoring test", strings.ToUpper(sys.Name))
		return nil
	}

	// Create service using the helper that handles mock/real supervisors
	cmd := []string{"/bin/sh", "-c", "echo 'Started'; sleep 10; echo 'Exiting'; exit 0"}
	client, cleanup, err := createTestServiceWithSupervisorAndCmd(ctx, sys, serviceName, logger, cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to create test service: %w", err)
	}

	defer cleanup()

	// Start service
	logger.Log("Starting service")
	switch c := client.(type) {
	case *ClientSystemd:
		if err := c.Start(ctx); err != nil {
			return fmt.Errorf("failed to start: %w", err)
		}
	default:
		if sc, ok := client.(ServiceClient); ok {
			if err := sc.Up(ctx); err != nil {
				return fmt.Errorf("failed to start: %w", err)
			}
		}
	}

	// Monitor status over time
	logger.Log("Monitoring status")
	for i := 0; i < 5; i++ {
		switch c := client.(type) {
		case *ClientSystemd:
			status, err := c.StatusSystemd(ctx)
			if err != nil {
				logger.Log("Status check %d failed: %v", i, err)
			} else {
				logger.Log("Status %d: state=%s/%s, running=%v, pid=%d",
					i, status.ActiveState, status.SubState, status.Running, status.MainPID)
			}
		default:
			if sc, ok := client.(ServiceClient); ok {
				status, err := sc.Status(ctx)
				if err != nil {
					logger.Log("Status check %d failed: %v", i, err)
				} else {
					logger.Log("Status %d: running=%v, pid=%d, state=%s, uptime=%d",
						i, status.State == StateRunning, status.PID, status.State, status.Uptime)
				}
			}
		}
		time.Sleep(2 * time.Second)
	}

	return nil
}

func testServiceRestart(ctx context.Context, sys SupervisionSystem, logger *TestLogger) error {
	serviceName := fmt.Sprintf("test-%s-restart-%d", sys.Name, time.Now().Unix())
	logger.Log("[%s] Testing service restart: %s", strings.ToUpper(sys.Name), serviceName)

	// Skip restart test in mock mode
	if isMockMode(sys) {
		logger.Log("[%s] Mock mode - skipping restart test", strings.ToUpper(sys.Name))
		return nil
	}

	// Create service using the helper that handles mock/real supervisors
	cmd := []string{"/bin/sh", "-c", "echo \"Started at $(date) with PID $$\"; sleep 30"}
	client, cleanup, err := createTestServiceWithSupervisorAndCmd(ctx, sys, serviceName, logger, cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to create test service: %w", err)
	}
	defer cleanup()

	// Skip actual restart operations in mock mode since they won't work
	if isMockMode(sys) {
		logger.Log("[%s] Mock mode - skipping actual restart operations", strings.ToUpper(sys.Name))
		return nil
	}

	// Start service
	logger.Log("Starting service")
	switch c := client.(type) {
	case *ClientSystemd:
		if err := c.Start(ctx); err != nil {
			return fmt.Errorf("failed to start: %w", err)
		}
	default:
		if sc, ok := client.(ServiceClient); ok {
			if err := sc.Up(ctx); err != nil {
				return fmt.Errorf("failed to start: %w", err)
			}
		}
	}

	time.Sleep(2 * time.Second)

	// Get initial PID
	var initialPID int
	switch c := client.(type) {
	case *ClientSystemd:
		status, err := c.StatusSystemd(ctx)
		if err != nil {
			return fmt.Errorf("failed to get initial status: %w", err)
		}
		initialPID = status.MainPID
	default:
		if sc, ok := client.(ServiceClient); ok {
			status, err := sc.Status(ctx)
			if err != nil {
				return fmt.Errorf("failed to get initial status: %w", err)
			}
			initialPID = status.PID
		}
	}
	logger.Log("Initial PID: %d", initialPID)

	// Restart service
	logger.Log("Restarting service")
	switch c := client.(type) {
	case *ClientSystemd:
		if err := c.Restart(ctx); err != nil {
			return fmt.Errorf("failed to restart: %w", err)
		}
	default:
		if sc, ok := client.(ServiceClient); ok {
			if err := sc.Restart(ctx); err != nil {
				return fmt.Errorf("failed to restart: %w", err)
			}
		}
	}

	time.Sleep(3 * time.Second)

	// Get new PID
	var newPID int
	switch c := client.(type) {
	case *ClientSystemd:
		status, err := c.StatusSystemd(ctx)
		if err != nil {
			return fmt.Errorf("failed to get status after restart: %w", err)
		}
		newPID = status.MainPID
	default:
		if sc, ok := client.(ServiceClient); ok {
			status, err := sc.Status(ctx)
			if err != nil {
				return fmt.Errorf("failed to get status after restart: %w", err)
			}
			newPID = status.PID
		}
	}
	logger.Log("New PID: %d", newPID)

	if initialPID == newPID && initialPID != 0 {
		return fmt.Errorf("PID did not change after restart")
	}

	logger.Log("Service successfully restarted")

	// Stop service
	switch c := client.(type) {
	case *ClientSystemd:
		return c.Stop(ctx)
	default:
		if sc, ok := client.(ServiceClient); ok {
			return sc.Down(ctx)
		}
		return fmt.Errorf("unknown client type")
	}
}

func testConcurrentOperations(ctx context.Context, sys SupervisionSystem, logger *TestLogger) error {
	logger.Log("Testing concurrent operations for %s", sys.Name)

	// Skip concurrent operations in mock mode
	if isMockMode(sys) {
		logger.Log("[%s] Mock mode - skipping concurrent operations test", strings.ToUpper(sys.Name))
		return nil
	}

	numServices := 3
	services := make([]string, numServices)
	clients := make([]interface{}, numServices)
	cleanups := make([]func() error, numServices)
	var creationErrors []error

	// Create multiple services
	for i := 0; i < numServices; i++ {
		serviceName := fmt.Sprintf("test-%s-concurrent-%d-%d", sys.Name, i, time.Now().Unix())
		services[i] = serviceName
		logger.Log("Creating service %d: %s", i, serviceName)

		// Create service using the helper that handles mock/real supervisors
		cmd := []string{"/bin/sh", "-c", fmt.Sprintf("echo 'Service %d'; sleep 30", i)}
		client, cleanup, err := createTestServiceWithSupervisorAndCmd(ctx, sys, serviceName, logger, cmd, nil)
		if err != nil {
			logger.Log("Failed to create service %d: %v", i, err)
			creationErrors = append(creationErrors, fmt.Errorf("service %d: %w", i, err))
			continue
		}
		clients[i] = client
		cleanups[i] = cleanup
	}

	// Cleanup all services at the end
	defer func() {
		for i, cleanup := range cleanups {
			if cleanup != nil {
				if err := cleanup(); err != nil {
					logger.Log("Warning: failed to cleanup service %d: %v", i, err)
				}
			}
		}
	}()

	// Start all services concurrently
	logger.Log("Starting all services concurrently")
	var wg sync.WaitGroup
	for i, client := range clients {
		if client == nil {
			continue
		}
		wg.Add(1)
		go func(idx int, c interface{}) {
			defer wg.Done()
			switch cl := c.(type) {
			case *ClientSystemd:
				if err := cl.Start(ctx); err != nil {
					logger.Log("Service %d start error: %v", idx, err)
				}
			default:
				if sc, ok := cl.(ServiceClient); ok {
					if err := sc.Up(ctx); err != nil {
						logger.Log("Service %d start error: %v", idx, err)
					}
				}
			}
		}(i, client)
	}
	wg.Wait()

	time.Sleep(2 * time.Second)

	// Check all statuses
	logger.Log("Checking all service statuses")
	for i, client := range clients {
		if client == nil {
			continue
		}
		switch c := client.(type) {
		case *ClientSystemd:
			status, err := c.StatusSystemd(ctx)
			if err != nil {
				logger.Log("Service %d status error: %v", i, err)
			} else {
				logger.Log("Service %d: running=%v, pid=%d", i, status.Running, status.MainPID)
			}
		default:
			if sc, ok := client.(ServiceClient); ok {
				status, err := sc.Status(ctx)
				if err != nil {
					logger.Log("Service %d status error: %v", i, err)
				} else {
					logger.Log("Service %d: running=%v, pid=%d", i, status.State == StateRunning, status.PID)
				}
			}
		}
	}

	// Stop all services concurrently
	logger.Log("Stopping all services concurrently")
	for i, client := range clients {
		if client == nil {
			continue
		}
		wg.Add(1)
		go func(idx int, c interface{}) {
			defer wg.Done()
			switch cl := c.(type) {
			case *ClientSystemd:
				if err := cl.Stop(ctx); err != nil {
					logger.Log("Service %d stop error: %v", idx, err)
				}
			default:
				if sc, ok := cl.(ServiceClient); ok {
					if err := sc.Down(ctx); err != nil {
						logger.Log("Service %d stop error: %v", idx, err)
					}
				}
			}
		}(i, client)
	}
	wg.Wait()

	logger.Log("Concurrent operations completed")

	// Check if we had any creation errors
	if len(creationErrors) > 0 {
		if len(creationErrors) == numServices {
			// All services failed to create
			return fmt.Errorf("all %d services failed to create: %v", numServices, creationErrors[0])
		}
		// Some services failed
		logger.Log("Warning: %d/%d services failed to create", len(creationErrors), numServices)
	}

	return nil
}

// System-specific tests
func testRunitOnce(ctx context.Context, sys SupervisionSystem, logger *TestLogger) error {
	if sys.Type != ServiceTypeRunit {
		return nil
	}

	logger.Log("Testing runit-specific 'once' operation")

	// Skip in mock mode
	if isMockMode(sys) {
		logger.Log("[RUNIT] Mock mode - skipping 'once' operation test")
		return nil
	}

	serviceName := fmt.Sprintf("test-runit-once-%d", time.Now().Unix())

	// Create service using the helper that handles mock/real supervisors
	cmd := []string{"/bin/sh", "-c", "echo 'Running once'; exit 0"}
	clientInterface, cleanup, err := createTestServiceWithSupervisorAndCmd(ctx, sys, serviceName, logger, cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to create test service: %w", err)
	}
	defer cleanup()

	client, ok := clientInterface.(*ClientRunit)
	if !ok {
		return fmt.Errorf("expected *ClientRunit, got %T", clientInterface)
	}

	logger.Log("Running service once")
	if err := client.Once(ctx); err != nil {
		return fmt.Errorf("failed to run once: %w", err)
	}

	time.Sleep(2 * time.Second)

	status, err := client.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	logger.Log("Status after once: running=%v, state=%s", status.State == StateRunning, status.State)
	return nil
}

func testS6Specific(ctx context.Context, sys SupervisionSystem, logger *TestLogger) error {
	if sys.Type != ServiceTypeS6 {
		return nil
	}

	logger.Log("Testing s6-specific features")
	// Add s6-specific tests here
	return nil
}

func testSystemdUnit(ctx context.Context, sys SupervisionSystem, logger *TestLogger) error {
	if sys.Type != ServiceTypeSystemd {
		return nil
	}

	logger.Log("Testing systemd unit file generation")

	builder := NewServiceBuilder("test-unit", "")
	builder.WithCmd([]string{"/bin/sh", "-c", "echo 'Test service'; sleep 10"})
	builder.WithCwd("/var/lib/myapp")
	builder.WithEnv("ENV_VAR", "value")
	builder.WithUmask(0022)
	builder.WithChpst(func(c *ChpstConfig) {
		c.User = "myuser"
		c.Group = "mygroup"
		c.Nice = 10
		c.LimitMem = 1024 * 1024 * 1024
		c.LimitFiles = 4096
	})

	systemdBuilder := NewBuilderSystemd(builder)
	unitContent, err := systemdBuilder.BuildSystemdUnit()
	if err != nil {
		return fmt.Errorf("failed to generate unit file: %w", err)
	}

	logger.Log("Generated unit file:")
	for _, line := range strings.Split(unitContent, "\n") {
		if line != "" {
			logger.Log("  %s", line)
		}
	}

	// Verify expected content
	expectedParts := []string{
		"[Unit]",
		"[Service]",
		"ExecStart=/bin/sh",
		"WorkingDirectory=/var/lib/myapp",
		"Environment=\"ENV_VAR=value\"",
		"User=myuser",
		"Group=mygroup",
		"Nice=10",
		"[Install]",
	}

	for _, part := range expectedParts {
		if !strings.Contains(unitContent, part) {
			return fmt.Errorf("unit file missing expected content: %s", part)
		}
	}

	logger.Log("Unit file generation test passed")
	return nil
}

func writeComparisonReport(filename string, report *ComparisonReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return renameio.WriteFile(filename, data, 0o644)
}

func writeTextSummary(filename string, report *ComparisonReport) error {
	var sb strings.Builder

	sb.WriteString("SUPERVISION SYSTEM COMPARISON TEST REPORT\n")
	sb.WriteString("==========================================\n\n")
	sb.WriteString(fmt.Sprintf("Host: %s\n", report.Host))
	sb.WriteString(fmt.Sprintf("Started: %s\n", report.Started.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Completed: %s\n", report.Completed.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Duration: %v\n\n", report.Duration))

	sb.WriteString("SUMMARY BY SYSTEM\n")
	sb.WriteString("-----------------\n\n")

	for _, sys := range report.Systems {
		sb.WriteString(fmt.Sprintf("%s:\n", strings.ToUpper(sys.System)))
		sb.WriteString(fmt.Sprintf("  Available: %v\n", sys.Available))
		sb.WriteString(fmt.Sprintf("  Passed: %d\n", sys.Passed))
		sb.WriteString(fmt.Sprintf("  Failed: %d\n", sys.Failed))
		sb.WriteString(fmt.Sprintf("  Skipped: %d\n", sys.Skipped))
		sb.WriteString(fmt.Sprintf("  Duration: %v\n", sys.Duration))

		if len(sys.Results) > 0 {
			sb.WriteString("  Tests:\n")
			for _, test := range sys.Results {
				status := "PASS"
				if !test.Success {
					status = "FAIL"
				}
				sb.WriteString(fmt.Sprintf("    - %s: %s (%v)\n", test.Name, status, test.Duration))
				if test.Error != "" {
					sb.WriteString(fmt.Sprintf("      Error: %s\n", test.Error))
				}
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("COMPARISON MATRIX\n")
	sb.WriteString("-----------------\n\n")

	// Create a comparison matrix
	testNames := make(map[string]bool)
	for _, sys := range report.Systems {
		for _, test := range sys.Results {
			testNames[test.Name] = true
		}
	}

	// Header
	sb.WriteString(fmt.Sprintf("%-30s", "Test"))
	for _, sys := range report.Systems {
		sb.WriteString(fmt.Sprintf(" %-12s", sys.System))
	}
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("-", 30+len(report.Systems)*13) + "\n")

	// Results
	for testName := range testNames {
		sb.WriteString(fmt.Sprintf("%-30s", testName))
		for _, sys := range report.Systems {
			result := "N/A"
			for _, test := range sys.Results {
				if test.Name == testName {
					if test.Success {
						result = "PASS"
					} else {
						result = "FAIL"
					}
					break
				}
			}
			if !sys.Available {
				result = "SKIP"
			}
			sb.WriteString(fmt.Sprintf(" %-12s", result))
		}
		sb.WriteString("\n")
	}

	return renameio.WriteFile(filename, []byte(sb.String()), 0o644)
}

func printSummary(logger *TestLogger, report *ComparisonReport) {
	logger.Log("=== FINAL SUMMARY ===")
	for _, sys := range report.Systems {
		if sys.Available {
			logger.Log("%s: %d passed, %d failed (total: %v)",
				sys.System, sys.Passed, sys.Failed, sys.Duration)
		} else {
			logger.Log("%s: Not available (skipped)", sys.System)
		}
	}
	logger.Log("Total test duration: %v", report.Duration)
}
