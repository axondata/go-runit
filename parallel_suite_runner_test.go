// +build linux

package svcmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestSuite manages scanning supervisors for the entire test suite
type TestSuite struct {
	mu         sync.RWMutex
	baseDir    string
	supervisors map[ServiceType]*SuiteSupervision
	counter    int64
}

// SuiteSupervision represents a running scanning supervisor for a service type
type SuiteSupervision struct {
	Type       ServiceType
	Dir        string
	Process    *exec.Cmd
	Available  bool
	RefCount   int32  // Atomic counter of active services
	Services   map[string]*ServiceRef // Track active services
	mu         sync.Mutex
}

// ServiceRef tracks a service with reference counting
type ServiceRef struct {
	Dir       string
	Client    ServiceClient
	RefCount  int32
	CreatedAt time.Time
}

// NewTestSuite creates a new test suite (scanning supervisors are started lazily)
func NewTestSuite(t *testing.T) (*TestSuite, error) {
	baseDir := t.TempDir()
	ts := &TestSuite{
		baseDir:     baseDir,
		supervisors: make(map[ServiceType]*SuiteSupervision),
	}

	// Initialize supervisor entries but don't start processes yet
	for _, st := range []ServiceType{ServiceTypeRunit, ServiceTypeDaemontools, ServiceTypeS6} {
		if !checkSupervisionAvailable(st) {
			continue
		}

		serviceDir := filepath.Join(baseDir, st.String())
		if err := os.MkdirAll(serviceDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create service dir for %s: %w", st, err)
		}

		supervisor := &SuiteSupervision{
			Type:      st,
			Dir:       serviceDir,
			Available: true,
			Services:  make(map[string]*ServiceRef),
			Process:   nil, // Will be started lazily when first service is created
		}

		ts.supervisors[st] = supervisor
	}

	return ts, nil
}

// startSupervisor starts a scanning supervisor if not already running
func (ts *TestSuite) startSupervisor(supervisor *SuiteSupervision) error {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()

	// Already running
	if supervisor.Process != nil {
		return nil
	}

	ctx := context.Background()
	var cmd *exec.Cmd

	switch supervisor.Type {
	case ServiceTypeRunit:
		cmd = exec.CommandContext(ctx, "runsvdir", "-P", supervisor.Dir)
	case ServiceTypeDaemontools:
		cmd = exec.CommandContext(ctx, "svscan", supervisor.Dir)
	case ServiceTypeS6:
		cmd = exec.CommandContext(ctx, "s6-svscan", "-t", "500", supervisor.Dir)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s scanner: %w", supervisor.Type, err)
	}

	supervisor.Process = cmd

	// Give the scanner time to initialize
	time.Sleep(500 * time.Millisecond)

	return nil
}

// stopSupervisor stops a scanning supervisor if running and no services active
func (ts *TestSuite) stopSupervisor(supervisor *SuiteSupervision) error {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()

	// Check if we should stop (no services and process is running)
	if atomic.LoadInt32(&supervisor.RefCount) > 0 || supervisor.Process == nil {
		return nil
	}

	// Stop the scanning supervisor
	if err := syscall.Kill(-supervisor.Process.Process.Pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to kill %s scanner: %w", supervisor.Type, err)
	}

	// Wait for it to exit
	supervisor.Process.Wait()
	supervisor.Process = nil

	return nil
}

// CreateService creates a new service directory for testing and returns a release function
func (ts *TestSuite) CreateService(t *testing.T, serviceType ServiceType, name string) (string, ServiceClient, func(), error) {
	ts.mu.RLock()
	supervisor, exists := ts.supervisors[serviceType]
	ts.mu.RUnlock()

	if !exists || !supervisor.Available {
		return "", nil, nil, fmt.Errorf("supervisor for %s not available", serviceType)
	}

	// Start the scanning supervisor if this is the first service
	if atomic.LoadInt32(&supervisor.RefCount) == 0 {
		if err := ts.startSupervisor(supervisor); err != nil {
			return "", nil, nil, fmt.Errorf("failed to start supervisor: %w", err)
		}
	}

	// Generate unique service name
	counter := atomic.AddInt64(&ts.counter, 1)
	serviceName := fmt.Sprintf("%s-%d-%d", name, counter, time.Now().UnixNano())
	serviceDir := filepath.Join(supervisor.Dir, serviceName)

	// Create the service
	config := getServiceConfig(serviceType)
	builder := NewServiceBuilderWithConfig(serviceName, supervisor.Dir, config)
	builder.WithCmd([]string{"/bin/sh", "-c", "exec sleep 3600"})

	if err := builder.Build(); err != nil {
		return "", nil, nil, fmt.Errorf("failed to build service: %w", err)
	}

	// Wait for the scanning supervisor to detect and start supervising
	superviseDir := filepath.Join(serviceDir, "supervise")
	statusFile := filepath.Join(superviseDir, "status")

	// Different scan intervals for different systems
	var maxWait time.Duration
	switch serviceType {
	case ServiceTypeRunit:
		maxWait = 2 * time.Second // runsvdir -P is fast
	case ServiceTypeS6:
		maxWait = 2 * time.Second // we set -t 500 (500ms)
	case ServiceTypeDaemontools:
		maxWait = 7 * time.Second // svscan default is 5 seconds
	}

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(statusFile); err == nil && info.Size() > 0 {
			// Status file exists, supervisor has picked it up
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Create the client
	var client ServiceClient
	var err error
	switch serviceType {
	case ServiceTypeRunit:
		client, err = NewClientRunit(serviceDir)
	case ServiceTypeDaemontools:
		client, err = NewClientDaemontools(serviceDir)
	case ServiceTypeS6:
		client, err = NewClientS6(serviceDir)
	}

	if err != nil {
		os.RemoveAll(serviceDir)
		return "", nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Track the service with reference counting
	supervisor.mu.Lock()
	serviceRef := &ServiceRef{
		Dir:       serviceDir,
		Client:    client,
		RefCount:  1,
		CreatedAt: time.Now(),
	}
	supervisor.Services[serviceName] = serviceRef
	atomic.AddInt32(&supervisor.RefCount, 1)
	supervisor.mu.Unlock()

	// Create release function that decrements refcount
	release := func() {
		supervisor.mu.Lock()

		if ref, exists := supervisor.Services[serviceName]; exists {
			newCount := atomic.AddInt32(&ref.RefCount, -1)
			if newCount <= 0 {
				// Remove service when refcount reaches 0
				delete(supervisor.Services, serviceName)
				newSupervisorCount := atomic.AddInt32(&supervisor.RefCount, -1)

				// Clean up the service directory
				os.RemoveAll(serviceDir)

				supervisor.mu.Unlock()

				// Stop the scanning supervisor if no more services
				if newSupervisorCount == 0 {
					ts.stopSupervisor(supervisor)
				}
				return
			}
		}
		supervisor.mu.Unlock()
	}

	return serviceDir, client, release, nil
}

// GetActiveServiceCount returns the number of active services for a supervisor type
func (ts *TestSuite) GetActiveServiceCount(serviceType ServiceType) int32 {
	ts.mu.RLock()
	supervisor, exists := ts.supervisors[serviceType]
	ts.mu.RUnlock()

	if !exists {
		return 0
	}

	return atomic.LoadInt32(&supervisor.RefCount)
}

// IsSupervisorRunning checks if a scanning supervisor is running
func (ts *TestSuite) IsSupervisorRunning(serviceType ServiceType) bool {
	ts.mu.RLock()
	supervisor, exists := ts.supervisors[serviceType]
	ts.mu.RUnlock()

	if !exists {
		return false
	}

	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()

	return supervisor.Process != nil
}

// GetServiceStats returns statistics about services
func (ts *TestSuite) GetServiceStats() map[ServiceType]int32 {
	stats := make(map[ServiceType]int32)

	ts.mu.RLock()
	defer ts.mu.RUnlock()

	for st, supervisor := range ts.supervisors {
		stats[st] = atomic.LoadInt32(&supervisor.RefCount)
	}

	return stats
}

// WaitForNoServices waits until all services are released (useful for cleanup verification)
func (ts *TestSuite) WaitForNoServices(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		allZero := true

		ts.mu.RLock()
		for _, supervisor := range ts.supervisors {
			if atomic.LoadInt32(&supervisor.RefCount) > 0 {
				allZero = false
				break
			}
		}
		ts.mu.RUnlock()

		if allZero {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Return error with current counts
	stats := ts.GetServiceStats()
	return fmt.Errorf("services still active after %v: %v", timeout, stats)
}

// Cleanup stops all scanning supervisors
func (ts *TestSuite) Cleanup() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	for _, supervisor := range ts.supervisors {
		if supervisor.Process != nil {
			// Kill the process group to stop scanner and all supervisors
			syscall.Kill(-supervisor.Process.Process.Pid, syscall.SIGTERM)
			supervisor.Process.Wait()
		}
	}
}

func checkSupervisionAvailable(st ServiceType) bool {
	switch st {
	case ServiceTypeRunit:
		return CheckToolAvailable("runsvdir") && CheckToolAvailable("runsv")
	case ServiceTypeDaemontools:
		return CheckToolAvailable("svscan") && CheckToolAvailable("supervise")
	case ServiceTypeS6:
		return CheckToolAvailable("s6-svscan") && CheckToolAvailable("s6-supervise")
	default:
		return false
	}
}

func getServiceConfig(serviceType ServiceType) *ServiceConfig {
	switch serviceType {
	case ServiceTypeRunit:
		return ConfigRunit()
	case ServiceTypeDaemontools:
		return ConfigDaemontools()
	case ServiceTypeS6:
		return ConfigS6()
	default:
		return ConfigRunit()
	}
}
