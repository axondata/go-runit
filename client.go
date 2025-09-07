package runit

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/axondata/go-runit/internal/unix"
)

// Client provides control and status operations for a single runit service.
// It communicates directly with the service's supervise process through
// control sockets/FIFOs and status files, without shelling out to sv.
type Client struct {
	// ServiceDir is the canonical path to the service directory
	ServiceDir string

	// DialTimeout is the timeout for establishing control socket connections
	DialTimeout time.Duration

	// WriteTimeout is the timeout for writing control commands
	WriteTimeout time.Duration

	// ReadTimeout is the timeout for reading status information
	ReadTimeout time.Duration

	// BackoffMin is the minimum duration between retry attempts
	BackoffMin time.Duration

	// BackoffMax is the maximum duration between retry attempts
	BackoffMax time.Duration

	// MaxAttempts is the maximum number of retry attempts for control operations
	MaxAttempts int

	// WatchDebounce is the debounce duration for watch events to coalesce rapid changes
	WatchDebounce time.Duration

	// mu protects concurrent access to send operations
	mu sync.Mutex
}

// Option configures a Client
type Option func(*Client)

// WithDialTimeout sets the timeout for control socket connections
func WithDialTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.DialTimeout = d
	}
}

// WithWriteTimeout sets the timeout for control write operations
func WithWriteTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.WriteTimeout = d
	}
}

// WithReadTimeout sets the timeout for status read operations
func WithReadTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.ReadTimeout = d
	}
}

// WithBackoff sets the minimum and maximum backoff durations for retries
func WithBackoff(minBackoff, maxBackoff time.Duration) Option {
	return func(c *Client) {
		c.BackoffMin = minBackoff
		c.BackoffMax = maxBackoff
	}
}

// WithMaxAttempts sets the maximum number of retry attempts
func WithMaxAttempts(n int) Option {
	return func(c *Client) {
		c.MaxAttempts = n
	}
}

// WithWatchDebounce sets the debounce duration for watch events
func WithWatchDebounce(d time.Duration) Option {
	return func(c *Client) {
		c.WatchDebounce = d
	}
}

// New creates a new Client for the specified service directory.
// It verifies the service has a supervise directory and applies any provided options.
func New(serviceDir string, opts ...Option) (*Client, error) {
	absPath, err := filepath.Abs(serviceDir)
	if err != nil {
		return nil, fmt.Errorf("resolving service dir: %w", err)
	}

	c := &Client{
		ServiceDir:    absPath,
		DialTimeout:   DefaultDialTimeout,
		WriteTimeout:  DefaultWriteTimeout,
		ReadTimeout:   DefaultReadTimeout,
		BackoffMin:    DefaultBackoffMin,
		BackoffMax:    DefaultBackoffMax,
		MaxAttempts:   DefaultMaxAttempts,
		WatchDebounce: DefaultWatchDebounce,
	}

	for _, opt := range opts {
		opt(c)
	}

	superviseDir := filepath.Join(c.ServiceDir, SuperviseDir)
	if _, err := os.Stat(superviseDir); os.IsNotExist(err) {
		return nil, &OpError{Op: OpUnknown, Path: superviseDir, Err: ErrNotSupervised}
	}

	return c, nil
}

// send writes a single control byte to the service's control socket/FIFO.
// It implements exponential backoff and retries for transient failures.
func (c *Client) send(ctx context.Context, op Operation) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	controlPath := filepath.Join(c.ServiceDir, SuperviseDir, ControlFile)
	cmd := op.Byte()

	var lastErr error
	backoff := c.BackoffMin

	for attempt := 0; attempt < c.MaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			backoff *= 2
			if backoff > c.BackoffMax {
				backoff = c.BackoffMax
			}
		}

		conn, err := net.DialTimeout("unix", controlPath, c.DialTimeout)
		if err == nil {
			defer func() { _ = conn.Close() }()

			if c.WriteTimeout > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout))
			}

			if _, err := conn.Write([]byte{cmd}); err == nil {
				return nil
			}
			lastErr = err
			continue
		}

		file, err := os.OpenFile(controlPath, os.O_WRONLY|unix.ONonblock, 0)
		if err == nil {
			defer func() { _ = file.Close() }()

			if _, err := file.Write([]byte{cmd}); err == nil {
				return nil
			}
			lastErr = err
			continue
		}

		lastErr = err
	}

	if lastErr != nil {
		return &OpError{Op: op, Path: controlPath, Err: lastErr}
	}
	return &OpError{Op: op, Path: controlPath, Err: ErrControlNotReady}
}

// Up starts the service (sets want up)
func (c *Client) Up(ctx context.Context) error {
	return c.send(ctx, OpUp)
}

// Once starts the service once (does not restart if it exits)
func (c *Client) Once(ctx context.Context) error {
	return c.send(ctx, OpOnce)
}

// Down stops the service (sets want down)
func (c *Client) Down(ctx context.Context) error {
	return c.send(ctx, OpDown)
}

// Term sends SIGTERM to the service process
func (c *Client) Term(ctx context.Context) error {
	return c.send(ctx, OpTerm)
}

// Interrupt sends SIGINT to the service process
func (c *Client) Interrupt(ctx context.Context) error {
	return c.send(ctx, OpInterrupt)
}

// HUP sends SIGHUP to the service process
func (c *Client) HUP(ctx context.Context) error {
	return c.send(ctx, OpHUP)
}

// Alarm sends SIGALRM to the service process
func (c *Client) Alarm(ctx context.Context) error {
	return c.send(ctx, OpAlarm)
}

// Quit sends SIGQUIT to the service process
func (c *Client) Quit(ctx context.Context) error {
	return c.send(ctx, OpQuit)
}

// Kill sends SIGKILL to the service process
func (c *Client) Kill(ctx context.Context) error {
	return c.send(ctx, OpKill)
}

// Pause sends SIGSTOP to the service process
func (c *Client) Pause(ctx context.Context) error {
	return c.send(ctx, OpPause)
}

// Cont sends SIGCONT to the service process
func (c *Client) Cont(ctx context.Context) error {
	return c.send(ctx, OpCont)
}

// ExitSupervise terminates the supervise process for this service
func (c *Client) ExitSupervise(ctx context.Context) error {
	return c.send(ctx, OpExit)
}

// Status reads and decodes the service's binary status file.
// It returns typed Status information without shelling out to sv.
func (c *Client) Status(_ context.Context) (Status, error) {
	statusPath := filepath.Join(c.ServiceDir, SuperviseDir, StatusFile)

	file, err := os.Open(statusPath)
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}
	defer func() { _ = file.Close() }()

	var buf [StatusFileSize]byte
	n, err := io.ReadFull(file, buf[:])
	if err != nil && err != io.ErrUnexpectedEOF {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}
	if n != StatusFileSize {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: ErrDecode}
	}

	status, err := decodeStatus(buf[:])
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}

	return status, nil
}
