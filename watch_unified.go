//go:build linux || darwin

package runit

import (
	"context"
)

// Watch implementations using the common watchImpl

// Watch for ClientRunit monitors the service's status file for changes
func (c *ClientRunit) Watch(ctx context.Context) (<-chan WatchEvent, WatchCleanupFunc, error) {
	return watchImpl(ctx, c)
}

// Watch for ClientDaemontools monitors the service's status file for changes
func (c *ClientDaemontools) Watch(ctx context.Context) (<-chan WatchEvent, WatchCleanupFunc, error) {
	return watchImpl(ctx, c)
}

// Watch for ClientS6 monitors the service's status file for changes
func (c *ClientS6) Watch(ctx context.Context) (<-chan WatchEvent, WatchCleanupFunc, error) {
	return watchImpl(ctx, c)
}