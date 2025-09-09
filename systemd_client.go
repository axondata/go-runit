//go:build linux

package runit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ClientSystemd provides control operations for systemd services
// It implements a similar interface to the runit Client but uses systemctl
type ClientSystemd struct {
	// ServiceName is the name of the systemd service (without .service suffix)
	ServiceName string

	// UseSudo indicates whether to use sudo for systemctl commands
	UseSudo bool

	// SudoCommand is the sudo command to use (default: "sudo")
	SudoCommand string

	// SystemctlPath is the path to systemctl binary
	SystemctlPath string

	// Timeout for systemctl operations
	Timeout time.Duration

	// WatchInterval is the polling interval for Watch when other methods unavailable
	WatchInterval time.Duration
}

// NewClientSystemd creates a new ClientSystemd for the specified service
func NewClientSystemd(serviceName string) *ClientSystemd {
	return &ClientSystemd{
		ServiceName:   serviceName,
		UseSudo:       os.Geteuid() != 0,
		SudoCommand:   "sudo",
		SystemctlPath: "systemctl",
		Timeout:       10 * time.Second,
		WatchInterval: 1 * time.Second,
	}
}

// WithSudo configures sudo usage
func (c *ClientSystemd) WithSudo(use bool, command string) *ClientSystemd {
	c.UseSudo = use
	if command != "" {
		c.SudoCommand = command
	}
	return c
}

// WithTimeout sets the timeout for operations
func (c *ClientSystemd) WithTimeout(d time.Duration) *ClientSystemd {
	c.Timeout = d
	return c
}

// execSystemctl executes a systemctl command with optional sudo
func (c *ClientSystemd) execSystemctl(ctx context.Context, args ...string) (string, error) {
	var cmd *exec.Cmd

	serviceName := fmt.Sprintf("%s.service", c.ServiceName)
	fullArgs := append(args, serviceName)

	if c.UseSudo {
		sudoArgs := append([]string{c.SystemctlPath}, fullArgs...)
		cmd = exec.CommandContext(ctx, c.SudoCommand, sudoArgs...)
	} else {
		cmd = exec.CommandContext(ctx, c.SystemctlPath, fullArgs...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

// Up starts the service (sets want up)
func (c *ClientSystemd) Up(ctx context.Context) error {
	_, err := c.execSystemctl(ctx, "start")
	return err
}

// Start starts the service (alias for Up)
func (c *ClientSystemd) Start(ctx context.Context) error {
	return c.Up(ctx)
}

// Down stops the service (sets want down)
func (c *ClientSystemd) Down(ctx context.Context) error {
	_, err := c.execSystemctl(ctx, "stop")
	return err
}

// Stop stops the service (alias for Down)
func (c *ClientSystemd) Stop(ctx context.Context) error {
	return c.Down(ctx)
}

// Restart restarts the service
func (c *ClientSystemd) Restart(ctx context.Context) error {
	_, err := c.execSystemctl(ctx, "restart")
	return err
}

// Reload attempts to reload the service using systemctl reload.
// This uses the service's ExecReload= configuration if defined.
// If the service doesn't support reload, this will return an error.
// Note: This is NOT the same as sending SIGHUP - use HUP() for that.
func (c *ClientSystemd) Reload(ctx context.Context) error {
	_, err := c.execSystemctl(ctx, "reload")
	return err
}

// HUP sends SIGHUP directly to the service's main process.
// This bypasses systemd's reload mechanism and sends the signal directly.
// For services with ExecReload= configured, consider using Reload() instead.
func (c *ClientSystemd) HUP(ctx context.Context) error {
	return c.signalMainPID(ctx, "HUP")
}

// Kill sends SIGKILL to the service's main process
func (c *ClientSystemd) Kill(ctx context.Context) error {
	return c.signalMainPID(ctx, "KILL")
}

// Term sends SIGTERM to the service's main process
func (c *ClientSystemd) Term(ctx context.Context) error {
	return c.signalMainPID(ctx, "TERM")
}

// Signal sends a custom signal to the service's main process
func (c *ClientSystemd) Signal(ctx context.Context, sig string) error {
	// For most signals, we want to target the main PID specifically
	// rather than all processes in the service's cgroup
	return c.signalMainPID(ctx, sig)
}

// USR1 sends SIGUSR1 to the service
func (c *ClientSystemd) USR1(ctx context.Context) error {
	return c.Signal(ctx, "USR1")
}

// USR2 sends SIGUSR2 to the service
func (c *ClientSystemd) USR2(ctx context.Context) error {
	return c.Signal(ctx, "USR2")
}

// StatusSystemd returns the systemd-specific status of the service
func (c *ClientSystemd) StatusSystemd(ctx context.Context) (*StatusSystemd, error) {
	// Get basic status
	output, err := c.execSystemctl(ctx, "show", "--no-page")
	if err != nil {
		return nil, err
	}

	status := &StatusSystemd{
		Properties: make(map[string]string),
	}

	// Parse the key=value output
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			status.Properties[key] = value

			// Map common properties
			switch key {
			case "ActiveState":
				status.ActiveState = value
			case "SubState":
				status.SubState = value
			case "LoadState":
				status.LoadState = value
			case "MainPID":
				if pid, err := strconv.Atoi(value); err == nil && pid > 0 {
					status.MainPID = pid
				}
			case "ExecMainStartTimestampMonotonic":
				if usec, err := strconv.ParseInt(value, 10, 64); err == nil && usec > 0 {
					status.StartTime = time.Unix(0, usec*1000)
				}
			case "Result":
				status.Result = value
			}
		}
	}

	// Determine if service is running
	status.Running = status.ActiveState == "active" && status.SubState == "running"

	// Calculate uptime if running
	if status.Running && !status.StartTime.IsZero() {
		status.Uptime = time.Since(status.StartTime)
	}

	return status, nil
}

// IsRunning checks if the service is currently running
func (c *ClientSystemd) IsRunning(ctx context.Context) (bool, error) {
	output, err := c.execSystemctl(ctx, "is-active")
	if err != nil {
		// systemctl returns exit code 3 when service is not active
		// This is not really an error, just a status
		// Check if the error message contains "exit status 3"
		if strings.Contains(err.Error(), "exit status 3") {
			return false, nil
		}
		return false, err
	}

	return strings.TrimSpace(output) == "active", nil
}

// Enable enables the service to start on boot
func (c *ClientSystemd) Enable(ctx context.Context) error {
	_, err := c.execSystemctl(ctx, "enable")
	return err
}

// Disable disables the service from starting on boot
func (c *ClientSystemd) Disable(ctx context.Context) error {
	_, err := c.execSystemctl(ctx, "disable")
	return err
}

// StatusSystemd represents the status of a systemd service
type StatusSystemd struct {
	// ActiveState is the active state (active, inactive, failed, etc.)
	ActiveState string

	// SubState is the sub state (running, dead, exited, etc.)
	SubState string

	// LoadState is the load state (loaded, not-found, error, etc.)
	LoadState string

	// Running indicates if the service is currently running
	Running bool

	// MainPID is the main process ID (0 if not running)
	MainPID int

	// StartTime is when the service was started
	StartTime time.Time

	// Uptime is how long the service has been running
	Uptime time.Duration

	// Result is the result of the last run (success, exit-code, signal, etc.)
	Result string

	// Properties contains all properties returned by systemctl show
	Properties map[string]string
}

// String returns a human-readable status string
func (s *StatusSystemd) String() string {
	if s.Running {
		return fmt.Sprintf("running (pid %d) for %s", s.MainPID, s.Uptime.Round(time.Second))
	}
	return fmt.Sprintf("%s/%s", s.ActiveState, s.SubState)
}

// MapToStatus converts StatusSystemd to runit Status for compatibility
func (s *StatusSystemd) MapToStatus() *Status {
	status := &Status{
		PID: s.MainPID,
	}

	if s.Running && !s.StartTime.IsZero() {
		status.Since = s.StartTime
		status.Uptime = s.Uptime
	}

	// Map systemd states to runit-like states
	switch s.ActiveState {
	case "active":
		if s.SubState == "running" {
			status.State = StateRunning
		}
	case "inactive":
		status.State = StateDown
		status.Flags.WantDown = true
	case "failed":
		status.State = StateDown
	}

	// Set WantUp flag based on running state
	if s.Running {
		status.Flags.WantUp = true
	}

	return status
}

// SendOperation maps runit operations to systemd commands
func (c *ClientSystemd) SendOperation(ctx context.Context, op Operation) error {
	switch op {
	case OpUp: // OpStart is an alias for OpUp
		return c.Start(ctx)
	case OpDown: // OpStop is an alias for OpDown
		return c.Stop(ctx)
	case OpRestart:
		return c.Restart(ctx)
	case OpHUP:
		// Try reload first, fall back to sending SIGHUP to main PID
		if err := c.Reload(ctx); err != nil {
			return c.signalMainPID(ctx, "HUP")
		}
		return nil
	case OpTerm:
		return c.Term(ctx)
	case OpKill:
		return c.Kill(ctx)
	case OpInterrupt:
		return c.signalMainPID(ctx, "INT")
	case OpAlarm:
		return c.signalMainPID(ctx, "ALRM")
	case OpQuit:
		return c.signalMainPID(ctx, "QUIT")
	case OpUSR1:
		return c.signalMainPID(ctx, "USR1")
	case OpUSR2:
		return c.signalMainPID(ctx, "USR2")
	case OpPause:
		return c.signalMainPID(ctx, "STOP")
	case OpCont:
		return c.signalMainPID(ctx, "CONT")
	case OpOnce:
		// Run the service once using systemd-run
		return c.runOnce(ctx)
	case OpExit:
		// No direct equivalent - could stop and disable
		if err := c.Stop(ctx); err != nil {
			return err
		}
		return c.Disable(ctx)
	case OpStatus:
		// Status is a query, not an operation
		return nil
	default:
		return fmt.Errorf("unsupported operation: %v", op)
	}
}

// signalMainPID gets the MainPID and sends a signal directly to it
func (c *ClientSystemd) signalMainPID(ctx context.Context, signal string) error {
	// Get the MainPID
	serviceName := fmt.Sprintf("%s.service", c.ServiceName)

	var cmd *exec.Cmd
	if c.UseSudo {
		cmd = exec.CommandContext(ctx, c.SudoCommand, c.SystemctlPath, "show", "-p", "MainPID", "--value", serviceName)
	} else {
		cmd = exec.CommandContext(ctx, c.SystemctlPath, "show", "-p", "MainPID", "--value", serviceName)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("getting MainPID: %w (stderr: %s)", err, stderr.String())
	}

	pidStr := strings.TrimSpace(stdout.String())
	if pidStr == "" || pidStr == "0" {
		return fmt.Errorf("service has no MainPID (not running?)")
	}

	// Send the signal to the process
	if c.UseSudo {
		cmd = exec.CommandContext(ctx, c.SudoCommand, "kill", "-"+signal, pidStr)
	} else {
		cmd = exec.CommandContext(ctx, "kill", "-"+signal, pidStr)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sending signal %s to PID %s: %w", signal, pidStr, err)
	}

	return nil
}

// runOnce runs the service command once using systemd-run
func (c *ClientSystemd) runOnce(ctx context.Context) error {
	// First, we need to get the ExecStart command from the unit file
	serviceName := fmt.Sprintf("%s.service", c.ServiceName)

	var cmd *exec.Cmd
	if c.UseSudo {
		cmd = exec.CommandContext(ctx, c.SudoCommand, c.SystemctlPath, "show", "-p", "ExecStart", "--value", serviceName)
	} else {
		cmd = exec.CommandContext(ctx, c.SystemctlPath, "show", "-p", "ExecStart", "--value", serviceName)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("getting ExecStart: %w (stderr: %s)", err, stderr.String())
	}

	execStart := strings.TrimSpace(stdout.String())
	if execStart == "" {
		return fmt.Errorf("no ExecStart command found for service")
	}

	// Parse the ExecStart line (it's in a special format)
	// Format: { path=/usr/bin/command ; argv[]=/usr/bin/command arg1 arg2 ; ignore_errors=no }
	// We need to extract the actual command
	if strings.HasPrefix(execStart, "{") && strings.HasSuffix(execStart, "}") {
		// Parse systemd's structured format
		parts := strings.Split(execStart[1:len(execStart)-1], ";")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "argv[]=") {
				execStart = strings.TrimPrefix(part, "argv[]=")
				break
			} else if strings.HasPrefix(part, "path=") {
				// Fallback to just the path if argv[] is not found
				execStart = strings.TrimPrefix(part, "path=")
			}
		}
	}

	// Run the command once using systemd-run
	// --uid, --gid, --setenv can be extracted from the service if needed
	runArgs := []string{"systemd-run", "--no-block", "--uid=" + os.Getenv("USER")}

	// Add the command
	// Split execStart properly (this is simplified, may need shell parsing)
	cmdParts := strings.Fields(execStart)
	runArgs = append(runArgs, cmdParts...)

	if c.UseSudo {
		cmd = exec.CommandContext(ctx, c.SudoCommand, runArgs...)
	} else {
		cmd = exec.CommandContext(ctx, runArgs[0], runArgs[1:]...)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running service once: %w", err)
	}

	return nil
}

// WaitForState waits for the service to reach a specific state
func (c *ClientSystemd) WaitForState(ctx context.Context, targetState string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for state %s", targetState)
			}

			status, err := c.StatusSystemd(ctx)
			if err != nil {
				continue // Keep trying
			}

			if status.ActiveState == targetState {
				return nil
			}
		}
	}
}

// Status returns the status of the service in runit format for interface compatibility
func (c *ClientSystemd) Status(ctx context.Context) (Status, error) {
	systemdStatus, err := c.StatusSystemd(ctx)
	if err != nil {
		return Status{}, err
	}
	return *systemdStatus.MapToStatus(), nil
}

// Once starts the service once (does not restart if it exits)
func (c *ClientSystemd) Once(ctx context.Context) error {
	return c.runOnce(ctx)
}

// Pause sends SIGSTOP to the service process
func (c *ClientSystemd) Pause(ctx context.Context) error {
	return c.signalMainPID(ctx, "STOP")
}

// Continue sends SIGCONT to the service process
func (c *ClientSystemd) Continue(ctx context.Context) error {
	return c.signalMainPID(ctx, "CONT")
}

// Interrupt sends SIGINT to the service process
func (c *ClientSystemd) Interrupt(ctx context.Context) error {
	return c.signalMainPID(ctx, "INT")
}

// Alarm sends SIGALRM to the service process
func (c *ClientSystemd) Alarm(ctx context.Context) error {
	return c.signalMainPID(ctx, "ALRM")
}

// Quit sends SIGQUIT to the service process
func (c *ClientSystemd) Quit(ctx context.Context) error {
	return c.signalMainPID(ctx, "QUIT")
}

// ExitSupervise stops and disables the service (no direct systemd equivalent)
func (c *ClientSystemd) ExitSupervise(ctx context.Context) error {
	if err := c.Stop(ctx); err != nil {
		return err
	}
	return c.Disable(ctx)
}

// Watch monitors the systemd service for state changes
func (c *ClientSystemd) Watch(ctx context.Context) (<-chan WatchEvent, WatchCleanupFunc, error) {
	// For systemd, we poll the status periodically since there's no file to watch
	ch := make(chan WatchEvent, 10)

	ticker := time.NewTicker(c.WatchInterval)
	done := make(chan struct{})

	var lastState string

	stop := func() error {
		close(done)
		ticker.Stop()
		close(ch)
		return nil
	}

	go func() {
		defer ticker.Stop()
		defer close(ch)

		// Get initial status
		if status, err := c.Status(ctx); err == nil {
			lastState = status.State.String()
			ch <- WatchEvent{Status: status}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				status, err := c.Status(ctx)
				if err != nil {
					select {
					case ch <- WatchEvent{Err: err}:
					case <-ctx.Done():
						return
					}
					continue
				}

				currentState := status.State.String()
				if currentState != lastState {
					lastState = currentState
					select {
					case ch <- WatchEvent{Status: status}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch, stop, nil
}

// Ensure ClientSystemd implements ServiceClient
var _ ServiceClient = (*ClientSystemd)(nil)
