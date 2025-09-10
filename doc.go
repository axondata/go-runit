// Package svcmgr provides a native Go library for controlling process supervisors
// including runit, s6, daemontools, and systemd without shelling out to external commands.
//
// The core functionality centers around the Client type, which provides
// direct control and status operations for a single runit service:
//
//	client, err := svcmgr.New("/etc/service/myapp")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Start the service
//	err = client.Up(context.Background())
//
//	// Get status
//	status, err := client.Status(context.Background())
//	fmt.Printf("Service state: %v, PID: %d\n", status.State, status.PID)
//
// # Manager for Bulk Operations
//
// The Manager type is provided as a convenience for applications that need
// to control multiple services concurrently. It's particularly useful for:
//
//   - System initialization/shutdown sequences
//   - Health monitoring dashboards
//   - Service orchestration tools
//   - Testing frameworks that manage multiple services
//
// If your application already has its own concurrency framework or only
// manages single services, you may not need the Manager. It's designed to
// be optional - the Client type provides all core functionality.
//
//	manager := svcmgr.NewManager(
//	    svcmgr.WithConcurrency(5),
//	    svcmgr.WithTimeout(10 * time.Second),
//	)
//
//	// Start multiple services concurrently
//	err = manager.Up(ctx, "/etc/service/web", "/etc/service/db", "/etc/service/cache")
//
// # Design Philosophy
//
// This library prioritizes:
//
//   - Zero external process spawning (no exec of sv/runsv)
//   - Direct communication with supervise control/status endpoints
//   - Zero allocations on hot paths (status decode, state strings)
//   - Context-aware operations with proper timeouts
//   - Type safety (no string-based operation codes)
//
// The Manager is included because many supervisor deployments involve coordinating
// multiple services, and having a tested, concurrent implementation prevents
// users from reimplementing the same patterns. However, it remains optional -
// all its functionality can be replicated using Client instances directly.
package svcmgr
