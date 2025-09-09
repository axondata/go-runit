//go:build linux || darwin

package runit

import (
	"context"
)

// Wait blocks until the service reaches one of the specified states or context is cancelled.
// Returns the status when one of the states is reached, or an error if one occurred.
// If states is nil or empty, waits for any state change.
//
// Example:
//
//	// Wait for any change
//	status, err := client.Wait(ctx, nil)
//
//	// Wait for specific states
//	status, err := client.Wait(ctx, []State{StateRunning, StateDown})
func (c *ClientRunit) Wait(ctx context.Context, states []State) (Status, error) {
	return waitImpl(ctx, c, states)
}

// Wait for ClientDaemontools
func (c *ClientDaemontools) Wait(ctx context.Context, states []State) (Status, error) {
	return waitImpl(ctx, c, states)
}

// Wait for ClientS6
func (c *ClientS6) Wait(ctx context.Context, states []State) (Status, error) {
	return waitImpl(ctx, c, states)
}

// Wait for ClientSystemd
func (c *ClientSystemd) Wait(ctx context.Context, states []State) (Status, error) {
	return waitImpl(ctx, c, states)
}