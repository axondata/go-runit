//go:build !devtree_cmd

package svcmgr

import "errors"

// DevTree represents a development runit service tree (stub implementation)
type DevTree struct {
	Base string
}

// NewDevTree creates a new DevTree (stub implementation)
func NewDevTree(_ string) (*DevTree, error) {
	return nil, errors.New("devtree not supported: built without devtree_cmd tag")
}

// ServicesDir returns the services directory path
func (d *DevTree) ServicesDir() string { return "" }

// EnabledDir returns the enabled services directory path
func (d *DevTree) EnabledDir() string { return "" }

// LogDir returns the log directory path
func (d *DevTree) LogDir() string { return "" }

// PIDFile returns the PID file path
func (d *DevTree) PIDFile() string { return "" }

// Ensure ensures the dev tree structure exists
func (d *DevTree) Ensure() error { return errors.New("not supported") }

// EnsureRunsvdir ensures runsvdir is running for the dev tree
func (d *DevTree) EnsureRunsvdir() error { return errors.New("not supported") }

// EnableService enables a service in the dev tree
func (d *DevTree) EnableService(_ string) error { return errors.New("not supported") }

// DisableService disables a service in the dev tree
func (d *DevTree) DisableService(_ string) error { return errors.New("not supported") }

// StopRunsvdir stops the runsvdir process for the dev tree
func (d *DevTree) StopRunsvdir() error { return errors.New("not supported") }
