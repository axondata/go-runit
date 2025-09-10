//go:build !linux

package svcmgr

import (
	"context"
	"fmt"
)

// ClientSystemd provides control operations for systemd services (Linux only)
type ClientSystemd struct {
	ServiceName string
}

// NewClientSystemd creates a new ClientSystemd (stub for non-Linux)
func NewClientSystemd(serviceName string) *ClientSystemd {
	return &ClientSystemd{ServiceName: serviceName}
}

// Up starts the service (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Up(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Down stops the service (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Down(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Status returns the service status (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Status(_ context.Context) (Status, error) {
	return Status{}, fmt.Errorf("systemd is only supported on Linux")
}

// Term sends SIGTERM (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Term(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Kill sends SIGKILL (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Kill(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// HUP sends SIGHUP (stub - systemd is only supported on Linux)
func (c *ClientSystemd) HUP(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Alarm sends SIGALRM (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Alarm(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Interrupt sends SIGINT (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Interrupt(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Quit sends SIGQUIT (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Quit(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// USR1 sends SIGUSR1 (stub - systemd is only supported on Linux)
func (c *ClientSystemd) USR1(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// USR2 sends SIGUSR2 (stub - systemd is only supported on Linux)
func (c *ClientSystemd) USR2(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Once runs the service once (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Once(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Pause sends SIGSTOP (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Pause(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Continue sends SIGCONT (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Continue(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Start is an alias for Up (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Start(ctx context.Context) error {
	return c.Up(ctx)
}

// Stop is an alias for Down (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Stop(ctx context.Context) error {
	return c.Down(ctx)
}

// Restart restarts the service (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Restart(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// ExitSupervise stops and disables the service (stub - systemd is only supported on Linux)
func (c *ClientSystemd) ExitSupervise(_ context.Context) error {
	return fmt.Errorf("systemd is only supported on Linux")
}

// Watch monitors for service changes (stub - systemd is only supported on Linux)
func (c *ClientSystemd) Watch(_ context.Context) (<-chan WatchEvent, WatchCleanupFunc, error) {
	return nil, nil, fmt.Errorf("systemd is only supported on Linux")
}

// Ensure ClientSystemd implements ServiceClient
var _ ServiceClient = (*ClientSystemd)(nil)
