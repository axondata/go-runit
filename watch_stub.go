//go:build !linux && !darwin

package runit

import (
	"context"
	"errors"
)

func (c *Client) Watch(ctx context.Context) (<-chan WatchEvent, func() error, error) {
	return nil, nil, errors.New("watch not supported on this platform")
}
