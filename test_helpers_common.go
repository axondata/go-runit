package svcmgr

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// toolAvailabilityCache caches the results of tool availability checks
// to avoid repeated exec.LookPath calls during test execution
var (
	toolAvailabilityCache = make(map[string]bool)
	toolAvailabilityMu    sync.RWMutex

	// Cached checks for common tool sets
	runitAvailable       bool
	runitOnce            sync.Once
	daemontoolsAvailable bool
	daemontoolsOnce      sync.Once
	s6Available          bool
	s6Once               sync.Once
	systemdAvailable     bool
	systemdOnce          sync.Once
)

// checkToolCached returns whether a tool is available, using cache
func checkToolCached(toolName string) bool {
	// Check cache first
	toolAvailabilityMu.RLock()
	if available, ok := toolAvailabilityCache[toolName]; ok {
		toolAvailabilityMu.RUnlock()
		return available
	}
	toolAvailabilityMu.RUnlock()

	// Not in cache, check and store result
	toolAvailabilityMu.Lock()
	defer toolAvailabilityMu.Unlock()

	// Double-check after acquiring write lock
	if available, ok := toolAvailabilityCache[toolName]; ok {
		return available
	}

	_, err := exec.LookPath(toolName)
	available := err == nil
	toolAvailabilityCache[toolName] = available
	return available
}

// RequireTool skips the test if the tool is not available in PATH.
// This should be used for any test that depends on external binaries.
func RequireTool(t *testing.T, toolName string) {
	t.Helper()
	if !checkToolCached(toolName) {
		t.Skipf("%s not found in PATH, skipping test (install it to run this test)", toolName)
	}
}

// RequireTools skips the test if any of the tools are not available in PATH.
// This is useful for tests that need multiple tools (e.g., both runsv and sv).
func RequireTools(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		RequireTool(t, tool)
	}
}

// RequireRoot skips the test if not running as root.
// Use this for tests that need system-level privileges.
func RequireRoot(t *testing.T) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("test requires root privileges (run with sudo to enable)")
	}
}

// RequireLinux skips the test if not running on Linux.
// Use this for Linux-specific functionality like systemd.
func RequireLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("test requires Linux")
	}
}

// RequireNotShort skips the test if running in short mode.
// Use this for integration tests that take longer to run.
func RequireNotShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

// RequireRunit ensures all runit tools are available
func RequireRunit(t *testing.T) {
	t.Helper()
	runitOnce.Do(func() {
		runitAvailable = checkToolCached("runsv") && checkToolCached("sv")
	})
	if !runitAvailable {
		t.Skip("runit tools (runsv, sv) not found in PATH, skipping test")
	}
}

// RequireDaemontools ensures all daemontools tools are available
func RequireDaemontools(t *testing.T) {
	t.Helper()
	daemontoolsOnce.Do(func() {
		daemontoolsAvailable = checkToolCached("supervise") &&
			checkToolCached("svstat") &&
			checkToolCached("svc")
	})
	if !daemontoolsAvailable {
		t.Skip("daemontools tools (supervise, svstat, svc) not found in PATH, skipping test")
	}
}

// RequireS6 ensures all s6 tools are available
func RequireS6(t *testing.T) {
	t.Helper()
	s6Once.Do(func() {
		s6Available = checkToolCached("s6-supervise") &&
			checkToolCached("s6-svstat") &&
			checkToolCached("s6-svc")
	})
	if !s6Available {
		t.Skip("s6 tools (s6-supervise, s6-svstat, s6-svc) not found in PATH, skipping test")
	}
}

// RequireSystemd ensures systemd is available (Linux only)
func RequireSystemd(t *testing.T) {
	t.Helper()
	RequireLinux(t)
	systemdOnce.Do(func() {
		systemdAvailable = checkToolCached("systemctl")
	})
	if !systemdAvailable {
		t.Skip("systemd (systemctl) not found in PATH, skipping test")
	}
}

// CheckToolAvailable returns true if a tool is available in PATH.
// This is a non-skipping version for conditional logic.
func CheckToolAvailable(tool string) bool {
	return checkToolCached(tool)
}

// CheckAnyToolAvailable returns true if any of the tools are available
func CheckAnyToolAvailable(tools ...string) bool {
	for _, tool := range tools {
		if checkToolCached(tool) {
			return true
		}
	}
	return false
}

// CheckAllToolsAvailable returns true only if all tools are available
func CheckAllToolsAvailable(tools ...string) bool {
	for _, tool := range tools {
		if !checkToolCached(tool) {
			return false
		}
	}
	return true
}

// WaitForRunning waits for a service to reach the running state
func WaitForRunning(t *testing.T, client ServiceClient, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := client.Status(context.Background())
		if err == nil && status.State == StateRunning {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Get final status for error message
	status, err := client.Status(context.Background())
	if err != nil {
		return fmt.Errorf("service did not reach running state within %v (status error: %v)", timeout, err)
	}
	// Check one more time in case of race condition at deadline
	if status.State == StateRunning {
		return nil
	}
	return fmt.Errorf("service did not reach running state within %v (current state: %v)", timeout, status.State)
}

// WaitForState waits for a service to reach a specific state
func WaitForState(t *testing.T, client ServiceClient, expectedState State, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := client.Status(context.Background())
		if err == nil && status.State == expectedState {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Get final status for error message
	status, err := client.Status(context.Background())
	if err != nil {
		return fmt.Errorf("service did not reach running state within %v (status error: %v)", timeout, err)
	}
	// Check one more time in case of race condition at deadline
	if status.State == StateRunning {
		return nil
	}
	return fmt.Errorf("service did not reach state %v within %v (current state: %v)", expectedState, timeout, status.State)
}

// WaitForStatusFile waits for a valid status file to be created
func WaitForStatusFile(serviceDir string, serviceType ServiceType, timeout time.Duration) error {
	statusFile := filepath.Join(serviceDir, "supervise", "status")
	deadline := time.Now().Add(timeout)

	// Get expected size based on service type
	var expectedSize int64
	switch serviceType {
	case ServiceTypeRunit:
		expectedSize = RunitStatusSize
	case ServiceTypeDaemontools:
		expectedSize = DaemontoolsStatusSize
	case ServiceTypeS6:
		// S6 can have two different sizes
		expectedSize = S6StatusSizeCurrent // We'll check for both sizes
	default:
		expectedSize = 20 // Default to runit size
	}

	for time.Now().Before(deadline) {
		if info, err := os.Stat(statusFile); err == nil {
			size := info.Size()
			// For S6, check both possible sizes
			if serviceType == ServiceTypeS6 {
				if size == S6StatusSizePre220 || size == S6StatusSizeCurrent {
					// Give it a bit more time to ensure it's fully written
					time.Sleep(50 * time.Millisecond)
					return nil
				}
			} else if size == expectedSize {
				// Give it a bit more time to ensure it's fully written
				time.Sleep(50 * time.Millisecond)
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Check what's actually there for better error message
	if info, err := os.Stat(statusFile); err == nil {
		return fmt.Errorf("status file exists but has wrong size: got %d, expected %d", info.Size(), expectedSize)
	}
	return fmt.Errorf("status file not created within %v", timeout)
}

// DiagnosticInfo contains detailed diagnostic information about a service failure
type DiagnosticInfo struct {
	ServiceDir   string
	ServiceType  ServiceType
	StatusFile   string
	RunScript    string
	LogScript    string
	StatusHex    string
	StatusError  error
	RunContent   string
	LogContent   string
	ProcessInfo  string
	LastLogLines []string
}

// CollectServiceDiagnostics gathers comprehensive diagnostic information for debugging
func CollectServiceDiagnostics(serviceDir string, serviceType ServiceType) (*DiagnosticInfo, error) {
	diag := &DiagnosticInfo{
		ServiceDir:  serviceDir,
		ServiceType: serviceType,
	}

	// Read status file
	statusFile := filepath.Join(serviceDir, "supervise", "status")
	diag.StatusFile = statusFile

	if data, err := os.ReadFile(statusFile); err == nil {
		diag.StatusHex = hex.Dump(data)
	} else {
		diag.StatusError = err
	}

	// Read run script
	runScript := filepath.Join(serviceDir, "run")
	if content, err := os.ReadFile(runScript); err == nil {
		diag.RunContent = string(content)
	}
	diag.RunScript = runScript

	// Read log/run script if exists
	logScript := filepath.Join(serviceDir, "log", "run")
	if content, err := os.ReadFile(logScript); err == nil {
		diag.LogContent = string(content)
	}
	diag.LogScript = logScript

	// Try to get process information
	switch serviceType {
	case ServiceTypeRunit:
		if out, err := exec.Command("sv", "status", serviceDir).CombinedOutput(); err == nil {
			diag.ProcessInfo = string(out)
		}
	case ServiceTypeDaemontools:
		if out, err := exec.Command("svstat", serviceDir).CombinedOutput(); err == nil {
			diag.ProcessInfo = string(out)
		}
	case ServiceTypeS6:
		if out, err := exec.Command("s6-svstat", serviceDir).CombinedOutput(); err == nil {
			diag.ProcessInfo = string(out)
		}
	}

	// Try to read last log lines if log exists
	logFile := filepath.Join(serviceDir, "log", "current")
	if content, err := os.ReadFile(logFile); err == nil {
		lines := strings.Split(string(content), "\n")
		if len(lines) > 20 {
			diag.LastLogLines = lines[len(lines)-20:]
		} else {
			diag.LastLogLines = lines
		}
	}

	return diag, nil
}

// FormatDiagnostics formats diagnostic information for display
func FormatDiagnostics(diag *DiagnosticInfo) string {
	var b strings.Builder

	b.WriteString("\n=== SERVICE DIAGNOSTIC INFORMATION ===\n")
	b.WriteString(fmt.Sprintf("Service Directory: %s\n", diag.ServiceDir))
	b.WriteString(fmt.Sprintf("Service Type: %s\n", diag.ServiceType))

	b.WriteString("\n--- Status File ---\n")
	b.WriteString(fmt.Sprintf("Path: %s\n", diag.StatusFile))
	if diag.StatusError != nil {
		b.WriteString(fmt.Sprintf("Error reading status: %v\n", diag.StatusError))
	} else if diag.StatusHex != "" {
		b.WriteString("Hexdump:\n")
		b.WriteString(diag.StatusHex)
	}

	b.WriteString("\n--- Run Script ---\n")
	b.WriteString(fmt.Sprintf("Path: %s\n", diag.RunScript))
	if diag.RunContent != "" {
		b.WriteString("Content:\n")
		b.WriteString(diag.RunContent)
		b.WriteString("\n")
	} else {
		b.WriteString("(not found or empty)\n")
	}

	if diag.LogContent != "" {
		b.WriteString("\n--- Log Script ---\n")
		b.WriteString(fmt.Sprintf("Path: %s\n", diag.LogScript))
		b.WriteString("Content:\n")
		b.WriteString(diag.LogContent)
		b.WriteString("\n")
	}

	if diag.ProcessInfo != "" {
		b.WriteString("\n--- Process Status ---\n")
		b.WriteString(diag.ProcessInfo)
	}

	if len(diag.LastLogLines) > 0 {
		b.WriteString("\n--- Last Log Lines ---\n")
		for _, line := range diag.LastLogLines {
			if line != "" {
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("=== END DIAGNOSTIC INFORMATION ===\n")
	return b.String()
}

// WaitForRunningWithDiagnostics is an enhanced version that provides detailed diagnostics on failure
func WaitForRunningWithDiagnostics(t *testing.T, client ServiceClient, serviceDir string, serviceType ServiceType, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := client.Status(context.Background())
		if err == nil && status.State == StateRunning {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Get final status for error message
	status, err := client.Status(context.Background())

	// Collect diagnostics before returning error
	diag, _ := CollectServiceDiagnostics(serviceDir, serviceType)
	diagStr := ""
	if diag != nil {
		diagStr = FormatDiagnostics(diag)
	}

	if err != nil {
		return fmt.Errorf("service did not reach running state within %v (status error: %v)%s", timeout, err, diagStr)
	}
	// Check one more time in case of race condition at deadline
	if status.State == StateRunning {
		return nil
	}
	return fmt.Errorf("service did not reach running state within %v (current state: %v)%s", timeout, status.State, diagStr)
}
