//go:build !linux && !darwin

package svcmgr

import (
	"context"
	"errors"
)

// Watch for ClientRunit - not supported on this platform
func (c *ClientRunit) Watch(ctx context.Context) (<-chan WatchEvent, WatchCleanupFunc, error) {
	return nil, nil, errors.New("watch not supported on this platform")
}

// Watch for ClientDaemontools - not supported on this platform
func (c *ClientDaemontools) Watch(ctx context.Context) (<-chan WatchEvent, WatchCleanupFunc, error) {
	return nil, nil, errors.New("watch not supported on this platform")
}

// Watch for ClientS6 - not supported on this platform
func (c *ClientS6) Watch(ctx context.Context) (<-chan WatchEvent, WatchCleanupFunc, error) {
	return nil, nil, errors.New("watch not supported on this platform")
}
