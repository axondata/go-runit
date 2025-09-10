package svcmgr

import (
	"errors"
	"fmt"
)

// Common errors returned by runit operations
var (
	// ErrNotSupervised indicates the service directory lacks a supervise subdirectory
	ErrNotSupervised = errors.New("runit: supervise dir missing")

	// ErrControlNotReady indicates the control socket/FIFO is not accepting connections
	ErrControlNotReady = errors.New("runit: control not accepting connections")

	// ErrTimeout indicates an operation exceeded its timeout
	ErrTimeout = errors.New("runit: timeout")

	// ErrDecode indicates the status file could not be decoded
	ErrDecode = errors.New("runit: status decode")
)

// OpError represents an error from a runit operation
type OpError struct {
	// Op is the operation that failed
	Op Operation
	// Path is the file path involved in the operation
	Path string
	// Err is the underlying error
	Err error
}

// Error returns a formatted error message
func (e *OpError) Error() string {
	return fmt.Sprintf("runit %s %q: %v", e.Op.String(), e.Path, e.Err)
}

// Unwrap returns the underlying error for error chain inspection
func (e *OpError) Unwrap() error {
	return e.Err
}

// MultiError aggregates multiple errors from bulk operations
type MultiError struct {
	// Errors contains all accumulated errors
	Errors []error
}

// Error returns a summary of the accumulated errors
func (m *MultiError) Error() string {
	if len(m.Errors) == 0 {
		return "no errors"
	}
	if len(m.Errors) == 1 {
		return m.Errors[0].Error()
	}
	return fmt.Sprintf("%d errors occurred", len(m.Errors))
}

// Add appends an error to the collection if it's not nil
func (m *MultiError) Add(err error) {
	if err != nil {
		m.Errors = append(m.Errors, err)
	}
}

// Err returns nil if no errors occurred, otherwise returns the MultiError itself
func (m *MultiError) Err() error {
	if len(m.Errors) == 0 {
		return nil
	}
	return m
}
