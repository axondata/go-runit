package svcmgr

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/axondata/go-svcmgr/internal/unix"
)

// ClientDaemontools provides control and status operations for a daemontools service.
// It communicates directly with the service's supervise process through
// control sockets/FIFOs and status files.
type ClientDaemontools struct {
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

// NewClientDaemontools creates a new ClientDaemontools for the specified service directory.
// It verifies the service has a supervise directory.
func NewClientDaemontools(serviceDir string) (*ClientDaemontools, error) {
	absPath, err := filepath.Abs(serviceDir)
	if err != nil {
		return nil, fmt.Errorf("resolving service dir: %w", err)
	}

	cd := &ClientDaemontools{
		ServiceDir:    absPath,
		DialTimeout:   DefaultDialTimeout,
		WriteTimeout:  DefaultWriteTimeout,
		ReadTimeout:   DefaultReadTimeout,
		BackoffMin:    DefaultBackoffMin,
		BackoffMax:    DefaultBackoffMax,
		MaxAttempts:   DefaultMaxAttempts,
		WatchDebounce: DefaultWatchDebounce,
	}

	superviseDir := filepath.Join(cd.ServiceDir, SuperviseDir)
	if _, err := os.Stat(superviseDir); os.IsNotExist(err) {
		return nil, &OpError{Op: OpUnknown, Path: superviseDir, Err: ErrNotSupervised}
	}

	return cd, nil
}

// send writes a single control byte to the service's control socket/FIFO.
// It implements exponential backoff and retries for transient failures.
func (cd *ClientDaemontools) send(ctx context.Context, op Operation) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	// Check if this operation is supported by daemontools
	config := ConfigDaemontools()
	if !config.IsOperationSupported(op) {
		return &OpError{
			Op:   op,
			Path: cd.ServiceDir,
			Err:  fmt.Errorf("operation %s not supported by daemontools", op),
		}
	}

	controlPath := filepath.Join(cd.ServiceDir, SuperviseDir, ControlFile)
	cmd := op.Byte()

	var lastErr error
	backoff := cd.BackoffMin

	for attempt := 0; attempt < cd.MaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			backoff *= 2
			if backoff > cd.BackoffMax {
				backoff = cd.BackoffMax
			}
		}

		conn, err := net.DialTimeout("unix", controlPath, cd.DialTimeout)
		if err == nil {
			defer func() { _ = conn.Close() }()

			if cd.WriteTimeout > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(cd.WriteTimeout))
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
func (cd *ClientDaemontools) Up(ctx context.Context) error {
	return cd.send(ctx, OpUp)
}

// Once starts the service once (does not restart if it exits)
func (cd *ClientDaemontools) Once(ctx context.Context) error {
	return cd.send(ctx, OpOnce)
}

// Down stops the service (sets want down)
func (cd *ClientDaemontools) Down(ctx context.Context) error {
	return cd.send(ctx, OpDown)
}

// Term sends SIGTERM to the service process
func (cd *ClientDaemontools) Term(ctx context.Context) error {
	return cd.send(ctx, OpTerm)
}

// Interrupt sends SIGINT to the service process
func (cd *ClientDaemontools) Interrupt(ctx context.Context) error {
	return cd.send(ctx, OpInterrupt)
}

// HUP sends SIGHUP to the service process
func (cd *ClientDaemontools) HUP(ctx context.Context) error {
	return cd.send(ctx, OpHUP)
}

// Alarm sends SIGALRM to the service process
func (cd *ClientDaemontools) Alarm(ctx context.Context) error {
	return cd.send(ctx, OpAlarm)
}

// Quit sends SIGQUIT to the service process
func (cd *ClientDaemontools) Quit(_ context.Context) error {
	// Daemontools doesn't support SIGQUIT
	return &OpError{
		Op:   OpQuit,
		Path: cd.ServiceDir,
		Err:  fmt.Errorf("SIGQUIT not supported by daemontools"),
	}
}

// Kill sends SIGKILL to the service process
func (cd *ClientDaemontools) Kill(ctx context.Context) error {
	return cd.send(ctx, OpKill)
}

// Pause sends SIGSTOP to the service process
func (cd *ClientDaemontools) Pause(ctx context.Context) error {
	return cd.send(ctx, OpPause)
}

// Continue sends SIGCONT to the service process
func (cd *ClientDaemontools) Continue(ctx context.Context) error {
	return cd.send(ctx, OpCont)
}

// USR1 sends SIGUSR1 to the service process
func (cd *ClientDaemontools) USR1(ctx context.Context) error {
	return cd.send(ctx, OpUSR1)
}

// USR2 sends SIGUSR2 to the service process
func (cd *ClientDaemontools) USR2(ctx context.Context) error {
	return cd.send(ctx, OpUSR2)
}

// Restart restarts the service by sending Down then Up
func (cd *ClientDaemontools) Restart(ctx context.Context) error {
	if err := cd.Down(ctx); err != nil {
		return err
	}
	// Small delay to ensure the service has stopped
	time.Sleep(100 * time.Millisecond)
	return cd.Up(ctx)
}

// Start is an alias for Up
func (cd *ClientDaemontools) Start(ctx context.Context) error {
	return cd.Up(ctx)
}

// Stop is an alias for Down
func (cd *ClientDaemontools) Stop(ctx context.Context) error {
	return cd.Down(ctx)
}

// ExitSupervise terminates the supervise process for this service
func (cd *ClientDaemontools) ExitSupervise(ctx context.Context) error {
	return cd.send(ctx, OpExit)
}

// Status reads and decodes the service's binary status file.
// It returns typed Status information.
func (cd *ClientDaemontools) Status(_ context.Context) (Status, error) {
	statusPath := filepath.Join(cd.ServiceDir, SuperviseDir, StatusFile)

	file, err := os.Open(statusPath)
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}
	defer func() { _ = file.Close() }()

	// Daemontools status files are exactly 18 bytes
	buf := make([]byte, DaemontoolsStatusSize)
	n, err := io.ReadFull(file, buf)
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}
	if n != DaemontoolsStatusSize {
		return Status{}, &OpError{
			Op:   OpStatus,
			Path: statusPath,
			Err:  fmt.Errorf("invalid status file size: %d bytes (expected %d)", n, DaemontoolsStatusSize),
		}
	}

	// Decode using daemontools-specific decoder
	status, err := decodeStatusDaemontools(buf)
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}

	return status, nil
}

// Ensure ClientDaemontools implements ServiceClient
var _ ServiceClient = (*ClientDaemontools)(nil)
