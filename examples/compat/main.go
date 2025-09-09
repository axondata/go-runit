// Package main demonstrates using go-runit with different supervision systems
// (runit, daemontools, s6) through the factory functions.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/axondata/go-runit"
)

func main() {
	var (
		system  = flag.String("system", "runit", "Supervision system: runit, daemontools, or s6")
		service = flag.String("service", "", "Service path (uses default for system if not specified)")
		action  = flag.String("action", "status", "Action: up, down, status")
		timeout = flag.Duration("timeout", 5*time.Second, "Operation timeout")
	)
	flag.Parse()

	if err := run(*system, *service, *action, *timeout); err != nil {
		log.Fatal(err)
	}
}

func run(system, service, action string, timeout time.Duration) error {
	// Get the appropriate configuration
	var config *runit.ServiceConfig
	switch system {
	case "runit":
		config = runit.ConfigRunit()
	case "daemontools":
		config = runit.ConfigDaemontools()
	case "s6":
		config = runit.ConfigS6()
	default:
		return fmt.Errorf("unknown system: %s", system)
	}

	// Use default service directory if not specified
	if service == "" {
		service = fmt.Sprintf("%s/example", config.ServiceDir)
	}

	fmt.Printf("Using %s configuration:\n", system)
	fmt.Printf("  Service directory: %s\n", config.ServiceDir)
	fmt.Printf("  Privilege tool: %s\n", config.ChpstPath)
	fmt.Printf("  Logger: %s\n", config.LoggerPath)
	fmt.Printf("  Scanner: %s\n", config.RunsvdirPath)
	fmt.Println()

	// Create client based on system type
	var client runit.ServiceClient
	var err error
	switch config.Type {
	case runit.ServiceTypeRunit:
		client, err = runit.NewClientRunit(service)
	case runit.ServiceTypeDaemontools:
		client, err = runit.NewClientDaemontools(service)
	case runit.ServiceTypeS6:
		client, err = runit.NewClientS6(service)
	default:
		return fmt.Errorf("unsupported service type: %v", config.Type)
	}
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch action {
	case "up":
		if !config.IsOperationSupported(runit.OpUp) {
			return fmt.Errorf("operation 'up' not supported by %s", system)
		}
		if err := client.Up(ctx); err != nil {
			return fmt.Errorf("failed to start service: %w", err)
		}
		fmt.Println("Service started")

	case "down":
		if !config.IsOperationSupported(runit.OpDown) {
			return fmt.Errorf("operation 'down' not supported by %s", system)
		}
		if err := client.Down(ctx); err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}
		fmt.Println("Service stopped")

	case "status":
		status, err := client.Status(ctx)
		if err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}

		fmt.Printf("Service: %s\n", service)
		fmt.Printf("State: %v\n", status.State)
		fmt.Printf("PID: %d\n", status.PID)
		if status.PID > 0 {
			fmt.Printf("Uptime: %s\n", status.Uptime.Round(time.Second))
		}

		// Show which operations are supported
		fmt.Printf("\nSupported operations for %s:\n", system)
		ops := []runit.Operation{
			runit.OpUp, runit.OpOnce, runit.OpDown,
			runit.OpTerm, runit.OpKill, runit.OpQuit,
		}
		for _, op := range ops {
			if config.IsOperationSupported(op) {
				fmt.Printf("  ✓ %s\n", op)
			} else {
				fmt.Printf("  ✗ %s (not supported)\n", op)
			}
		}

	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	return nil
}
