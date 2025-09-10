// Example demonstrating the Wait() interface for blocking until status changes
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/axondata/go-svcmgr"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <service-dir>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s /var/service/nginx\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s /etc/service/sshd\n", os.Args[0])
		os.Exit(1)
	}

	serviceDir := os.Args[1]

	// Create a client (using runit as example - you could also use ServiceTypeDaemontools, ServiceTypeS6, etc)
	client, err := svcmgr.NewClient(serviceDir, svcmgr.ServiceTypeRunit)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Get initial status
	status, err := client.Status(context.Background())
	if err != nil {
		log.Fatalf("Failed to get status: %v", err)
	}
	fmt.Printf("Current state: %s (PID: %d)\n", status.State, status.PID)

	// Example 1: Wait for any change
	fmt.Println("\nWaiting for any status change (timeout: 30s)...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Wait for any change (empty states slice)
	newStatus, err := client.Wait(ctx, nil)
	if err != nil {
		if err == context.DeadlineExceeded {
			fmt.Println("No changes detected within timeout")
		} else {
			log.Printf("Error waiting for change: %v", err)
		}
	} else {
		fmt.Printf("Status changed! New state: %s (PID: %d)\n", newStatus.State, newStatus.PID)
		if newStatus.State != status.State {
			fmt.Printf("State transition: %s -> %s\n", status.State, newStatus.State)
		}
	}

	// Example 2: Wait for specific state
	targetState := svcmgr.StateRunning
	fmt.Printf("\nWaiting for service to reach %s state (timeout: 30s)...\n", targetState)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()

	// Wait for specific state
	finalStatus, err := client.Wait(ctx2, []svcmgr.State{targetState})
	if err != nil {
		if err == context.DeadlineExceeded {
			fmt.Printf("Service did not reach %s state within timeout\n", targetState)
		} else {
			log.Printf("Error waiting for state: %v", err)
		}
	} else {
		fmt.Printf("Service reached %s state! PID: %d\n", targetState, finalStatus.PID)
		if finalStatus.Ready {
			fmt.Printf("Service is ready (readiness signaled at %s)\n", finalStatus.ReadySince)
		}
	}
}
