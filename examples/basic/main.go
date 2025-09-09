// Package main provides a basic example of using the go-runit library.
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
		service = flag.String("service", "/etc/service/example", "Service directory")
		action  = flag.String("action", "status", "Action: up, down, status, restart")
		timeout = flag.Duration("timeout", 5*time.Second, "Operation timeout")
	)
	flag.Parse()

	if err := run(*service, *action, *timeout); err != nil {
		log.Fatal(err)
	}
}

func run(service, action string, timeout time.Duration) error {
	// Default to runit for this basic example
	client, err := runit.NewClientRunit(service)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch action {
	case "up":
		if err := client.Up(ctx); err != nil {
			return fmt.Errorf("failed to start service: %w", err)
		}
		fmt.Println("Service started")

	case "down":
		if err := client.Down(ctx); err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}
		fmt.Println("Service stopped")

	case "restart":
		if err := client.Down(ctx); err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}
		time.Sleep(1 * time.Second)
		if err := client.Up(ctx); err != nil {
			return fmt.Errorf("failed to start service: %w", err)
		}
		fmt.Println("Service restarted")

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
			fmt.Printf("Since: %s\n", status.Since.Format(time.RFC3339))
		}
		fmt.Printf("Want up: %v\n", status.Flags.WantUp)
		fmt.Printf("Want down: %v\n", status.Flags.WantDown)

	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	return nil
}
