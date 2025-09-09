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

// ClientS6 provides control and status operations for an s6 service.
// It communicates directly with the service's s6-supervise process through
// control sockets/FIFOs and status files.
type ClientS6 struct {
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

// NewClientS6 creates a new ClientS6 for the specified service directory.
// It verifies the service has a supervise directory.
func NewClientS6(serviceDir string) (*ClientS6, error) {
	absPath, err := filepath.Abs(serviceDir)
	if err != nil {
		return nil, fmt.Errorf("resolving service dir: %w", err)
	}

	cs := &ClientS6{
		ServiceDir:    absPath,
		DialTimeout:   DefaultDialTimeout,
		WriteTimeout:  DefaultWriteTimeout,
		ReadTimeout:   DefaultReadTimeout,
		BackoffMin:    DefaultBackoffMin,
		BackoffMax:    DefaultBackoffMax,
		MaxAttempts:   DefaultMaxAttempts,
		WatchDebounce: DefaultWatchDebounce,
	}

	superviseDir := filepath.Join(cs.ServiceDir, SuperviseDir)
	if _, err := os.Stat(superviseDir); os.IsNotExist(err) {
		return nil, &OpError{Op: OpUnknown, Path: superviseDir, Err: ErrNotSupervised}
	}

	return cs, nil
}

// send writes a single control byte to the service's control socket/FIFO.
// It implements exponential backoff and retries for transient failures.
func (cs *ClientS6) send(ctx context.Context, op Operation) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Check if this operation is supported by s6
	config := ConfigS6()
	if !config.IsOperationSupported(op) {
		return &OpError{
			Op:   op,
			Path: cs.ServiceDir,
			Err:  fmt.Errorf("operation %s not supported by s6", op),
		}
	}

	controlPath := filepath.Join(cs.ServiceDir, SuperviseDir, ControlFile)
	cmd := op.Byte()

	var lastErr error
	backoff := cs.BackoffMin

	for attempt := 0; attempt < cs.MaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			backoff *= 2
			if backoff > cs.BackoffMax {
				backoff = cs.BackoffMax
			}
		}

		conn, err := net.DialTimeout("unix", controlPath, cs.DialTimeout)
		if err == nil {
			defer func() { _ = conn.Close() }()

			if cs.WriteTimeout > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(cs.WriteTimeout))
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
func (cs *ClientS6) Up(ctx context.Context) error {
	return cs.send(ctx, OpUp)
}

// Once starts the service once (does not restart if it exits)
func (cs *ClientS6) Once(ctx context.Context) error {
	return cs.send(ctx, OpOnce)
}

// Down stops the service (sets want down)
func (cs *ClientS6) Down(ctx context.Context) error {
	return cs.send(ctx, OpDown)
}

// Term sends SIGTERM to the service process
func (cs *ClientS6) Term(ctx context.Context) error {
	return cs.send(ctx, OpTerm)
}

// Interrupt sends SIGINT to the service process
func (cs *ClientS6) Interrupt(ctx context.Context) error {
	return cs.send(ctx, OpInterrupt)
}

// HUP sends SIGHUP to the service process
func (cs *ClientS6) HUP(ctx context.Context) error {
	return cs.send(ctx, OpHUP)
}

// Alarm sends SIGALRM to the service process
func (cs *ClientS6) Alarm(ctx context.Context) error {
	return cs.send(ctx, OpAlarm)
}

// Quit sends SIGQUIT to the service process
func (cs *ClientS6) Quit(ctx context.Context) error {
	return cs.send(ctx, OpQuit)
}

// Kill sends SIGKILL to the service process
func (cs *ClientS6) Kill(ctx context.Context) error {
	return cs.send(ctx, OpKill)
}

// Pause sends SIGSTOP to the service process
func (cs *ClientS6) Pause(_ context.Context) error {
	// S6 doesn't support SIGSTOP
	return &OpError{
		Op:   OpPause,
		Path: cs.ServiceDir,
		Err:  fmt.Errorf("SIGSTOP not supported by s6"),
	}
}

// Continue sends SIGCONT to the service process
func (cs *ClientS6) Continue(_ context.Context) error {
	// S6 doesn't support SIGCONT
	return &OpError{
		Op:   OpCont,
		Path: cs.ServiceDir,
		Err:  fmt.Errorf("SIGCONT not supported by s6"),
	}
}

// USR1 sends SIGUSR1 to the service process
func (cs *ClientS6) USR1(ctx context.Context) error {
	return cs.send(ctx, OpUSR1)
}

// USR2 sends SIGUSR2 to the service process
func (cs *ClientS6) USR2(ctx context.Context) error {
	return cs.send(ctx, OpUSR2)
}

// Restart restarts the service by sending Down then Up
func (cs *ClientS6) Restart(ctx context.Context) error {
	if err := cs.Down(ctx); err != nil {
		return err
	}
	// Small delay to ensure the service has stopped
	time.Sleep(100 * time.Millisecond)
	return cs.Up(ctx)
}

// Start is an alias for Up
func (cs *ClientS6) Start(ctx context.Context) error {
	return cs.Up(ctx)
}

// Stop is an alias for Down
func (cs *ClientS6) Stop(ctx context.Context) error {
	return cs.Down(ctx)
}

// ExitSupervise terminates the supervise process for this service
func (cs *ClientS6) ExitSupervise(ctx context.Context) error {
	return cs.send(ctx, OpExit)
}

// Status reads and decodes the service's binary status file.
// It returns typed Status information.
func (cs *ClientS6) Status(_ context.Context) (Status, error) {
	statusPath := filepath.Join(cs.ServiceDir, SuperviseDir, StatusFile)

	file, err := os.Open(statusPath)
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}
	defer func() { _ = file.Close() }()

	// S6 status files can be either 35 or 43 bytes
	// Allocate for the maximum
	buf := make([]byte, S6MaxStatusSize)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}

	// Validate size
	if n != S6StatusSizePre220 && n != S6StatusSizeCurrent {
		return Status{}, &OpError{
			Op:   OpStatus,
			Path: statusPath,
			Err: fmt.Errorf("invalid S6 status file size: %d bytes (expected %d or %d)",
				n, S6StatusSizePre220, S6StatusSizeCurrent),
		}
	}

	// Decode using s6-specific decoder
	status, err := decodeStatusS6(buf[:n])
	if err != nil {
		return Status{}, &OpError{Op: OpStatus, Path: statusPath, Err: err}
	}

	return status, nil
}

// Ensure ClientS6 implements ServiceClient
var _ ServiceClient = (*ClientS6)(nil)
