// Package main provides an example of managing multiple runit services.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/axondata/go-runit"
)

func main() {
	var (
		services    = flag.String("services", "", "Comma-separated service directories")
		action      = flag.String("action", "status", "Action: up, down, status, term, kill")
		concurrency = flag.Int("concurrency", 10, "Max concurrent operations")
		timeout     = flag.Duration("timeout", 5*time.Second, "Operation timeout")
	)
	flag.Parse()

	if *services == "" {
		log.Fatal("Please specify services with -services flag")
	}

	if err := run(*services, *action, *concurrency, *timeout); err != nil {
		log.Fatal(err)
	}
}

func run(services, action string, concurrency int, timeout time.Duration) error {
	serviceList := parseServices(services)

	mgr := runit.NewManager(
		runit.WithConcurrency(concurrency),
		runit.WithTimeout(timeout),
	)

	ctx := context.Background()

	switch action {
	case "up":
		return handleUp(ctx, mgr, serviceList)
	case "down":
		return handleDown(ctx, mgr, serviceList)
	case "term":
		return handleTerm(ctx, mgr, serviceList)
	case "kill":
		return handleKill(ctx, mgr, serviceList)
	case "status":
		return handleStatus(ctx, mgr, serviceList)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

func parseServices(services string) []string {
	serviceList := strings.Split(services, ",")
	for i, s := range serviceList {
		serviceList[i] = strings.TrimSpace(s)
	}
	return serviceList
}

func handleUp(ctx context.Context, mgr *runit.Manager, services []string) error {
	if err := mgr.Up(ctx, services...); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}
	fmt.Printf("Started %d services\n", len(services))
	return nil
}

func handleDown(ctx context.Context, mgr *runit.Manager, services []string) error {
	if err := mgr.Down(ctx, services...); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}
	fmt.Printf("Stopped %d services\n", len(services))
	return nil
}

func handleTerm(ctx context.Context, mgr *runit.Manager, services []string) error {
	if err := mgr.Term(ctx, services...); err != nil {
		return fmt.Errorf("failed to term services: %w", err)
	}
	fmt.Printf("Sent SIGTERM to %d services\n", len(services))
	return nil
}

func handleKill(ctx context.Context, mgr *runit.Manager, services []string) error {
	if err := mgr.Kill(ctx, services...); err != nil {
		return fmt.Errorf("failed to kill services: %w", err)
	}
	fmt.Printf("Sent SIGKILL to %d services\n", len(services))
	return nil
}

func handleStatus(ctx context.Context, mgr *runit.Manager, serviceList []string) error {
	statuses, err := mgr.Status(ctx, serviceList...)
	if err != nil {
		log.Printf("Warning: %v", err)
	}

	return printStatusTable(serviceList, statuses)
}

func printStatusTable(serviceList []string, statuses map[string]runit.Status) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Write header
	if _, err := fmt.Fprintln(w, "SERVICE\tSTATE\tPID\tUPTIME"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := fmt.Fprintln(w, "-------\t-----\t---\t------"); err != nil {
		return fmt.Errorf("failed to write separator: %w", err)
	}

	// Write service statuses
	for _, svc := range serviceList {
		if err := writeServiceStatus(w, svc, statuses); err != nil {
			log.Printf("Failed to write status for %s: %v", svc, err)
		}
	}

	// Flush output
	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %w", err)
	}

	return nil
}

func writeServiceStatus(w *tabwriter.Writer, svc string, statuses map[string]runit.Status) error {
	status, ok := statuses[svc]
	if !ok {
		_, err := fmt.Fprintf(w, "%s\tERROR\t-\t-\n", shortenPath(svc))
		return err
	}

	uptimeStr := "-"
	if status.PID > 0 {
		uptimeStr = formatDuration(status.Uptime)
	}

	_, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
		shortenPath(svc),
		status.State,
		status.PID,
		uptimeStr,
	)
	return err
}

func shortenPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}
