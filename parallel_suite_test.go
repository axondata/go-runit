// +build linux

package svcmgr

import (
	"context"
	"testing"
	"time"
)

// TestParallelSuiteLazyInit tests that supervisors are started lazily and stopped when not needed
func TestParallelSuiteLazyInit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping parallel suite test in short mode")
	}

	suite, err := NewTestSuite(t)
	if err != nil {
		t.Fatalf("Failed to create test suite: %v", err)
	}
	defer suite.Cleanup()

	// Verify no supervisors are running initially
	for _, st := range []ServiceType{ServiceTypeRunit, ServiceTypeDaemontools, ServiceTypeS6} {
		if suite.IsSupervisorRunning(st) {
			t.Errorf("%s supervisor should not be running initially", st)
		}
		if count := suite.GetActiveServiceCount(st); count != 0 {
			t.Errorf("%s should have 0 services initially, got %d", st, count)
		}
	}

	// Test with daemontools (could be any type)
	if !checkSupervisionAvailable(ServiceTypeDaemontools) {
		t.Skip("Daemontools not available")
	}

	// Create first service - should start supervisor
	t.Log("Creating first service...")
	_, client1, release1, err := suite.CreateService(t, ServiceTypeDaemontools, "test1")
	if err != nil {
		t.Fatalf("Failed to create first service: %v", err)
	}

	// Verify supervisor is now running
	if !suite.IsSupervisorRunning(ServiceTypeDaemontools) {
		t.Error("Daemontools supervisor should be running after first service")
	}
	if count := suite.GetActiveServiceCount(ServiceTypeDaemontools); count != 1 {
		t.Errorf("Expected 1 active service, got %d", count)
	}

	// Test the service works
	ctx := context.Background()
	if err := client1.Start(ctx); err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}

	// Create second service - supervisor already running
	t.Log("Creating second service...")
	_, client2, release2, err := suite.CreateService(t, ServiceTypeDaemontools, "test2")
	if err != nil {
		t.Fatalf("Failed to create second service: %v", err)
	}

	if count := suite.GetActiveServiceCount(ServiceTypeDaemontools); count != 2 {
		t.Errorf("Expected 2 active services, got %d", count)
	}

	// Start second service
	if err := client2.Start(ctx); err != nil {
		t.Fatalf("Failed to start service 2: %v", err)
	}

	// Release first service - supervisor should still be running
	t.Log("Releasing first service...")
	release1()

	time.Sleep(500 * time.Millisecond) // Give time for cleanup

	if !suite.IsSupervisorRunning(ServiceTypeDaemontools) {
		t.Error("Supervisor should still be running with one service active")
	}
	if count := suite.GetActiveServiceCount(ServiceTypeDaemontools); count != 1 {
		t.Errorf("Expected 1 active service after releasing first, got %d", count)
	}

	// Release second service - supervisor should stop
	t.Log("Releasing second service...")
	release2()

	time.Sleep(1 * time.Second) // Give time for supervisor to stop

	if suite.IsSupervisorRunning(ServiceTypeDaemontools) {
		t.Error("Supervisor should have stopped after all services released")
	}
	if count := suite.GetActiveServiceCount(ServiceTypeDaemontools); count != 0 {
		t.Errorf("Expected 0 active services after releasing all, got %d", count)
	}

	// Verify other supervisors never started
	if suite.IsSupervisorRunning(ServiceTypeRunit) {
		t.Error("Runit supervisor should never have started")
	}
	if suite.IsSupervisorRunning(ServiceTypeS6) {
		t.Error("S6 supervisor should never have started")
	}

	// Cleanup verification
	t.Log("Waiting for all services to be cleaned up...")
	if err := suite.WaitForNoServices(5 * time.Second); err != nil {
		t.Errorf("Services not cleaned up: %v", err)
	}
}

// TestParallelSuiteComparison tests the same operation across all systems using the new suite
func TestParallelSuiteComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping parallel suite comparison test in short mode")
	}

	// Check if any supervision tools are available
	hasAny := false
	for _, st := range []ServiceType{ServiceTypeRunit, ServiceTypeDaemontools, ServiceTypeS6} {
		if checkSupervisionAvailable(st) {
			hasAny = true
			break
		}
	}
	if !hasAny {
		t.Skip("No supervision tools available")
	}

	suite, err := NewTestSuite(t)
	if err != nil {
		t.Fatalf("Failed to create test suite: %v", err)
	}
	defer suite.Cleanup()

	ctx := context.Background()

	// Test each available supervision system
	for _, st := range []ServiceType{ServiceTypeRunit, ServiceTypeDaemontools, ServiceTypeS6} {
		st := st // Capture for closure
		t.Run(st.String(), func(t *testing.T) {
			if !checkSupervisionAvailable(st) {
				t.Skipf("%s supervision tools not available", st)
			}
			// Create a service
			serviceDir, client, release, err := suite.CreateService(t, st, "test-service")
			if err != nil {
				t.Fatalf("Failed to create service: %v", err)
			}
			defer release()

			// Start the service
			startTime := time.Now()
			if err := client.Start(ctx); err != nil {
				t.Fatalf("Failed to start: %v", err)
			}
			startDuration := time.Since(startTime)

			// Wait for it to be running
			if err := WaitForRunning(t, client, 5*time.Second); err != nil {
				t.Fatalf("Service didn't start: %v", err)
			}

			// Check status
			status, err := client.Status(ctx)
			if err != nil {
				t.Fatalf("Failed to get status: %v", err)
			}
			if status.State != StateRunning {
				t.Errorf("Expected running, got %v", status.State)
			}

			// Stop the service
			stopTime := time.Now()
			if err := client.Stop(ctx); err != nil {
				t.Fatalf("Failed to stop: %v", err)
			}
			stopDuration := time.Since(stopTime)

			t.Logf("[%s] Service dir: %s, Start: %v, Stop: %v", st, serviceDir, startDuration, stopDuration)
		})
	}

	// Verify all supervisors are stopped after tests complete
	t.Log("Waiting for cleanup...")
	if err := suite.WaitForNoServices(10 * time.Second); err != nil {
		t.Errorf("Services not cleaned up: %v", err)
	}

	for _, st := range []ServiceType{ServiceTypeRunit, ServiceTypeDaemontools, ServiceTypeS6} {
		if suite.IsSupervisorRunning(st) {
			t.Errorf("%s supervisor still running after all tests", st)
		}
	}
}ubuntu@ip-172-31-62-134:~/src/go-runit$ cat parallel_suite_test.go ; echo
// +build linux

package svcmgr

import (
	"context"
	"testing"
	"time"
)

// TestParallelSuiteLazyInit tests that supervisors are started lazily and stopped when not needed
func TestParallelSuiteLazyInit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping parallel suite test in short mode")
	}

	suite, err := NewTestSuite(t)
	if err != nil {
		t.Fatalf("Failed to create test suite: %v", err)
	}
	defer suite.Cleanup()

	// Verify no supervisors are running initially
	for _, st := range []ServiceType{ServiceTypeRunit, ServiceTypeDaemontools, ServiceTypeS6} {
		if suite.IsSupervisorRunning(st) {
			t.Errorf("%s supervisor should not be running initially", st)
		}
		if count := suite.GetActiveServiceCount(st); count != 0 {
			t.Errorf("%s should have 0 services initially, got %d", st, count)
		}
	}

	// Test with daemontools (could be any type)
	if !checkSupervisionAvailable(ServiceTypeDaemontools) {
		t.Skip("Daemontools not available")
	}

	// Create first service - should start supervisor
	t.Log("Creating first service...")
	_, client1, release1, err := suite.CreateService(t, ServiceTypeDaemontools, "test1")
	if err != nil {
		t.Fatalf("Failed to create first service: %v", err)
	}

	// Verify supervisor is now running
	if !suite.IsSupervisorRunning(ServiceTypeDaemontools) {
		t.Error("Daemontools supervisor should be running after first service")
	}
	if count := suite.GetActiveServiceCount(ServiceTypeDaemontools); count != 1 {
		t.Errorf("Expected 1 active service, got %d", count)
	}

	// Test the service works
	ctx := context.Background()
	if err := client1.Start(ctx); err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}

	// Create second service - supervisor already running
	t.Log("Creating second service...")
	_, client2, release2, err := suite.CreateService(t, ServiceTypeDaemontools, "test2")
	if err != nil {
		t.Fatalf("Failed to create second service: %v", err)
	}

	if count := suite.GetActiveServiceCount(ServiceTypeDaemontools); count != 2 {
		t.Errorf("Expected 2 active services, got %d", count)
	}

	// Start second service
	if err := client2.Start(ctx); err != nil {
		t.Fatalf("Failed to start service 2: %v", err)
	}

	// Release first service - supervisor should still be running
	t.Log("Releasing first service...")
	release1()

	time.Sleep(500 * time.Millisecond) // Give time for cleanup

	if !suite.IsSupervisorRunning(ServiceTypeDaemontools) {
		t.Error("Supervisor should still be running with one service active")
	}
	if count := suite.GetActiveServiceCount(ServiceTypeDaemontools); count != 1 {
		t.Errorf("Expected 1 active service after releasing first, got %d", count)
	}

	// Release second service - supervisor should stop
	t.Log("Releasing second service...")
	release2()

	time.Sleep(1 * time.Second) // Give time for supervisor to stop

	if suite.IsSupervisorRunning(ServiceTypeDaemontools) {
		t.Error("Supervisor should have stopped after all services released")
	}
	if count := suite.GetActiveServiceCount(ServiceTypeDaemontools); count != 0 {
		t.Errorf("Expected 0 active services after releasing all, got %d", count)
	}

	// Verify other supervisors never started
	if suite.IsSupervisorRunning(ServiceTypeRunit) {
		t.Error("Runit supervisor should never have started")
	}
	if suite.IsSupervisorRunning(ServiceTypeS6) {
		t.Error("S6 supervisor should never have started")
	}

	// Cleanup verification
	t.Log("Waiting for all services to be cleaned up...")
	if err := suite.WaitForNoServices(5 * time.Second); err != nil {
		t.Errorf("Services not cleaned up: %v", err)
	}
}

// TestParallelSuiteComparison tests the same operation across all systems using the new suite
func TestParallelSuiteComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping parallel suite comparison test in short mode")
	}

	suite, err := NewTestSuite(t)
	if err != nil {
		t.Fatalf("Failed to create test suite: %v", err)
	}
	defer suite.Cleanup()

	ctx := context.Background()

	// Test each available supervision system
	for _, st := range []ServiceType{ServiceTypeRunit, ServiceTypeDaemontools, ServiceTypeS6} {
		if !checkSupervisionAvailable(st) {
			continue
		}

		t.Run(st.String(), func(t *testing.T) {
			// Create a service
			serviceDir, client, release, err := suite.CreateService(t, st, "test-service")
			if err != nil {
				t.Fatalf("Failed to create service: %v", err)
			}
			defer release()

			// Start the service
			startTime := time.Now()
			if err := client.Start(ctx); err != nil {
				t.Fatalf("Failed to start: %v", err)
			}
			startDuration := time.Since(startTime)

			// Wait for it to be running
			if err := WaitForRunning(t, client, 5*time.Second); err != nil {
				t.Fatalf("Service didn't start: %v", err)
			}

			// Check status
			status, err := client.Status(ctx)
			if err != nil {
				t.Fatalf("Failed to get status: %v", err)
			}
			if status.State != StateRunning {
				t.Errorf("Expected running, got %v", status.State)
			}

			// Stop the service
			stopTime := time.Now()
			if err := client.Stop(ctx); err != nil {
				t.Fatalf("Failed to stop: %v", err)
			}
			stopDuration := time.Since(stopTime)

			t.Logf("[%s] Service dir: %s, Start: %v, Stop: %v", st, serviceDir, startDuration, stopDuration)
		})
	}

	// Verify all supervisors are stopped after tests complete
	t.Log("Waiting for cleanup...")
	if err := suite.WaitForNoServices(10 * time.Second); err != nil {
		t.Errorf("Services not cleaned up: %v", err)
	}

	for _, st := range []ServiceType{ServiceTypeRunit, ServiceTypeDaemontools, ServiceTypeS6} {
		if suite.IsSupervisorRunning(st) {
			t.Errorf("%s supervisor still running after all tests", st)
		}
	}
}
