package svcmgr

import (
	"context"
)

// ServiceClient is the main interface all supervision clients implement.
// It provides a unified API for controlling services across different
// supervision systems (runit, daemontools, s6, systemd).
type ServiceClient interface {
	// Basic operations
	Up(ctx context.Context) error
	Down(ctx context.Context) error
	Status(ctx context.Context) (Status, error)

	// Signal operations
	Term(ctx context.Context) error
	Kill(ctx context.Context) error
	HUP(ctx context.Context) error
	Alarm(ctx context.Context) error
	Interrupt(ctx context.Context) error
	Quit(ctx context.Context) error
	USR1(ctx context.Context) error
	USR2(ctx context.Context) error

	// Control operations
	Once(ctx context.Context) error
	Pause(ctx context.Context) error
	Continue(ctx context.Context) error

	// Aliases
	Start(ctx context.Context) error // Alias for Up
	Stop(ctx context.Context) error  // Alias for Down
	Restart(ctx context.Context) error

	// Supervision control
	ExitSupervise(ctx context.Context) error

	// Watch monitors the service's status for changes
	// Returns a channel of events and a stop function
	Watch(ctx context.Context) (<-chan WatchEvent, WatchCleanupFunc, error)

	// Wait blocks until the service reaches one of the specified states
	// If states is nil or empty, waits for any status change
	Wait(ctx context.Context, states []State) (Status, error)
}
