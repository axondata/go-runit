//go:build !linux && !darwin

package runit

import (
	"context"
	"errors"
)

// Wait for ClientRunit - not supported on this platform
func (c *ClientRunit) Wait(ctx context.Context, states []State) (Status, error) {
	return Status{}, errors.New("wait not supported on this platform")
}

// Wait for ClientDaemontools - not supported on this platform
func (c *ClientDaemontools) Wait(ctx context.Context, states []State) (Status, error) {
	return Status{}, errors.New("wait not supported on this platform")
}

// Wait for ClientS6 - not supported on this platform
func (c *ClientS6) Wait(ctx context.Context, states []State) (Status, error) {
	return Status{}, errors.New("wait not supported on this platform")
}

// Wait for ClientSystemd - not supported on this platform
func (c *ClientSystemd) Wait(ctx context.Context, states []State) (Status, error) {
	return Status{}, errors.New("wait not supported on this platform")
}