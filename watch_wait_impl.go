//go:build linux || darwin

package svcmgr

import (
	"context"
)

// waitImpl provides a common implementation for Wait across all client types
func waitImpl(ctx context.Context, client ServiceClient, states []State) (Status, error) {
	// If states is empty, wait for any change
	if len(states) == 0 {
		events, cleanup, err := client.Watch(ctx)
		if err != nil {
			return Status{}, err
		}
		defer func() { _ = cleanup() }()

		select {
		case event := <-events:
			if event.Err != nil {
				return Status{}, event.Err
			}
			return event.Status, nil
		case <-ctx.Done():
			return Status{}, ctx.Err()
		}
	}

	// First check current state
	status, err := client.Status(ctx)
	if err != nil {
		return Status{}, err
	}
	for _, targetState := range states {
		if status.State == targetState {
			return status, nil
		}
	}

	// Watch for changes
	events, cleanup, err := client.Watch(ctx)
	if err != nil {
		return Status{}, err
	}
	defer func() { _ = cleanup() }()

	for {
		select {
		case event := <-events:
			if event.Err != nil {
				return Status{}, event.Err
			}
			for _, targetState := range states {
				if event.Status.State == targetState {
					return event.Status, nil
				}
			}
		case <-ctx.Done():
			return Status{}, ctx.Err()
		}
	}
}
