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

// ClientRunit provides control and status operations for a runit service.
// It communicates directly with the service's supervise process through
// control sockets/FIFOs and status files, without shelling out to sv.
type ClientRunit struct {
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

// NewClientRunit creates a new ClientRunit for the specified service directory.
// It verifies the service has a supervise directory.
func NewClientRunit(serviceDir string) (*ClientRunit, error) {
	absPath, err := filepath.Abs(serviceDir)
	if err != nil {
		return nil, fmt.Errorf("resolving service dir: %w", err)
	}

	rc := &ClientRunit{
		ServiceDir:    absPath,
		DialTimeout:   DefaultDialTimeout,
		WriteTimeout:  DefaultWriteTimeout,
		ReadTimeout:   DefaultReadTimeout,
		BackoffMin:    DefaultBackoffMin,
		BackoffMax:    DefaultBackoffMax,
		MaxAttempts:   DefaultMaxAttempts,
		WatchDebounce: DefaultWatchDebounce,
	}

	superviseDir := filepath.Join(rc.ServiceDir, SuperviseDir)
	if _, err := os.Stat(superviseDir); os.IsNotExist(err) {
		return nil, &OpError{Op: OpUnknown, Path: superviseDir, Err: ErrNotSupervised}
	}

	return rc, nil
}

// send writes a single control byte to the service's control socket/FIFO.
// It implements exponential backoff and retries for transient failures.
func (rc *ClientRunit) send(ctx context.Context, op Operation) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Runit supports all operations
	controlPath := filepath.Join(rc.ServiceDir, SuperviseDir, ControlFile)
	cmd := op.Byte()

	var lastErr error
	backoff := rc.BackoffMin

	for attempt := 0; attempt < rc.MaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			backoff *= 2
			if backoff > rc.BackoffMax {
				backoff = rc.BackoffMax
			}
		}

		conn, err := net.DialTimeout("unix", controlPath, rc.DialTimeout)
		if err == nil {
			defer func() { _ = conn.Close() }()

			if rc.WriteTimeout > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(rc.WriteTimeout))
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
func (rc *ClientRunit) Up(ctx context.Context) error {
	return rc.send(ctx, OpUp)
}

// Once starts the service once (does not restart if it exits)
func (rc *ClientRunit) Once(ctx context.Context) error {
	return rc.send(ctx, OpOnce)
}

// Down stops the service (sets want down)
func (rc *ClientRunit) Down(ctx context.Context) error {
	return rc.send(ctx, OpDown)
}

// Term sends SIGTERM to the service process
func (rc *ClientRunit) Term(ctx context.Context) error {
	return rc.send(ctx, OpTerm)
}

// Interrupt sends SIGINT to the service process
func (rc *ClientRunit) Interrupt(ctx context.Context) error {
	return rc.send(ctx, OpInterrupt)
}

// HUP sends SIGHUP to the service process
func (rc *ClientRunit) HUP(ctx context.Context) error {
	return rc.send(ctx, OpHUP)
}

// Alarm sends SIGALRM to the service process
func (rc *ClientRunit) Alarm(ctx context.Context) error {
	return rc.send(ctx, OpAlarm)
}

// Quit sends SIGQUIT to the service process
func (rc *ClientRunit) Quit(ctx context.Context) error {
	return rc.send(ctx, OpQuit)
}

// Kill sends SIGKILL to the service process
func (rc *ClientRunit) Kill(ctx context.Context) error {
	return rc.send(ctx, OpKill)
}

// Pause sends SIGSTOP to the service process
func (rc *ClientRunit) Pause(ctx context.Context) error {
	return rc.send(ctx, OpPause)
}

// Continue sends SIGCONT to the service process
func (rc *ClientRunit) Continue(ctx context.Context) error {
	return rc.send(ctx, OpCont)
}

// USR1 sends SIGUSR1 to the service process
func (rc *ClientRunit) USR1(ctx context.Context) error {
	return rc.send(ctx, OpUSR1)
}

// USR2 sends SIGUSR2 to the service process
func (rc *ClientRunit) USR2(ctx context.Context) error {
	return rc.send(ctx, OpUSR2)
}

// Restart restarts the service by sending Down then Up
func (rc *ClientRunit) Restart(ctx context.Context) error {
	if err := rc.Down(ctx); err != nil {
		return err
	}
	// Small delay to ensure the service has stopped
	time.Sleep(100 * time.Millisecond)
	return rc.Up(ctx)
}

// Start is an alias for Up
func (rc *ClientRunit) Start(ctx context.Context) error {
	return rc.Up(ctx)
}

// Stop is an alias for Down
func (rc *ClientRunit) Stop(ctx context.Context) error {
	return rc.Down(ctx)
}

// ExitSupervise terminates the supervise process for this service
func (rc *ClientRunit) ExitSupervise(ctx context.Context) error {
	return rc.send(ctx, OpExit)
}

// Status reads and decodes the service's binary status file.
// It returns typed Status information without shelling out to sv.
func (rc *ClientRunit) Status(_ context.Context) (Status, error) {
	statusPath := filepath.Join(rc.ServiceDir, SuperviseDir, StatusFile)

	file, err := os.Open(statusPath)
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}
	defer func() { _ = file.Close() }()

	// Runit status files are exactly 20 bytes
	buf := make([]byte, StatusFileSize)
	n, err := io.ReadFull(file, buf)
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}
	if n != StatusFileSize {
		return Status{}, &OpError{
			Op:   OpStatus,
			Path: statusPath,
			Err:  fmt.Errorf("invalid status file size: %d bytes (expected %d)", n, StatusFileSize),
		}
	}

	// Decode using runit-specific decoder
	status, err := decodeStatusRunit(buf)
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}

	return status, nil
}

// Ensure ClientRunit implements ServiceClient
var _ ServiceClient = (*ClientRunit)(nil)
