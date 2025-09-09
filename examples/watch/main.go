// Package main provides an example of watching runit service status changes.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/axondata/go-runit"
)

func main() {
	service := flag.String("service", "/etc/service/example", "Service directory")
	flag.Parse()

	if err := run(*service); err != nil {
		log.Fatal(err)
	}
}

func run(service string) error {
	client, err := runit.NewClientRunit(service)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// Create context that cancels on interrupt signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	events, stop, err := client.Watch(ctx)
	if err != nil {
		return fmt.Errorf("failed to start watching: %w", err)
	}
	defer func() {
		if err := stop(); err != nil {
			log.Printf("Warning: failed to stop watching: %v", err)
		}
	}()

	fmt.Printf("Watching %s for status changes (Ctrl-C to stop)...\n", service)

	status, err := client.Status(context.Background())
	if err == nil {
		printStatus(status)
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nStopping...")
			return nil

		case event := <-events:
			if event.Err != nil {
				log.Printf("Watch error: %v", event.Err)
				continue
			}

			printStatus(event.Status)
		}
	}
}

func printStatus(status runit.Status) {
	timestamp := time.Now().Format("15:04:05")

	stateStr := fmt.Sprintf("%-10s", status.State.String())

	if status.PID > 0 {
		fmt.Printf("[%s] %s PID=%-6d uptime=%s\n",
			timestamp, stateStr, status.PID,
			status.Uptime.Round(time.Second))
	} else {
		fmt.Printf("[%s] %s\n", timestamp, stateStr)
	}
}
