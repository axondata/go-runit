//go:build devtree_cmd

package runit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// DevTree manages a development runit service tree for unprivileged operation.
// It creates a scoped directory structure suitable for running services without root.
type DevTree struct {
	// Base is the root directory of the development tree
	Base string
	// RunsvdirPath is the path to the runsvdir binary
	RunsvdirPath string
}

// DevTreeOption configures a DevTree
type DevTreeOption func(*DevTree)

// WithRunsvdirPath sets the path to the runsvdir binary
func WithRunsvdirPath(path string) DevTreeOption {
	return func(d *DevTree) {
		d.RunsvdirPath = path
	}
}

// NewDevTree creates a new DevTree with default settings
func NewDevTree(base string, opts ...DevTreeOption) (*DevTree, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return nil, fmt.Errorf("resolving base path: %w", err)
	}

	d := &DevTree{
		Base:         absBase,
		RunsvdirPath: DefaultRunsvdirPath,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d, nil
}

// ServicesDir returns the path to the services directory
func (d *DevTree) ServicesDir() string {
	return filepath.Join(d.Base, "services")
}

// EnabledDir returns the path to the enabled services directory
func (d *DevTree) EnabledDir() string {
	return filepath.Join(d.Base, "enabled")
}

// LogDir returns the path to the log directory
func (d *DevTree) LogDir() string {
	return filepath.Join(d.Base, "log")
}

// PIDFile returns the path to the runsvdir PID file
func (d *DevTree) PIDFile() string {
	return filepath.Join(d.Base, "runsvdir.pid")
}

// Ensure creates the development tree directory structure if it doesn't exist
func (d *DevTree) Ensure() error {
	dirs := []string{
		d.ServicesDir(),
		d.EnabledDir(),
		d.LogDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, DirMode); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	return nil
}

// EnsureRunsvdir starts runsvdir if not already running.
// It checks for an existing process and starts a new one if needed.
func (d *DevTree) EnsureRunsvdir() error {
	pidFile := d.PIDFile()

	// Check if runsvdir is already running
	if data, err := os.ReadFile(pidFile); err == nil {
		if pid, err := strconv.Atoi(string(data)); err == nil {
			if proc, err := os.FindProcess(pid); err == nil {
				// Signal(0) checks if process exists
				if err := proc.Signal(nil); err == nil {
					return nil
				}
			}
		}
	}

	if err := d.Ensure(); err != nil {
		return err
	}

	cmd := exec.Command(d.RunsvdirPath, "-P", d.EnabledDir())
	cmd.Dir = d.Base
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting runsvdir: %w", err)
	}

	pidData := []byte(strconv.Itoa(cmd.Process.Pid))
	if err := os.WriteFile(pidFile, pidData, FileMode); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("writing pid file: %w", err)
	}

	return nil
}

// EnableService creates a symlink in the enabled directory to activate a service
func (d *DevTree) EnableService(name string) error {
	source := filepath.Join(d.ServicesDir(), name)
	target := filepath.Join(d.EnabledDir(), name)

	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("service %s not found: %w", name, err)
	}

	// Remove existing symlink if present
	os.Remove(target)

	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("creating symlink: %w", err)
	}

	return nil
}

// DisableService removes the symlink from the enabled directory to deactivate a service
func (d *DevTree) DisableService(name string) error {
	target := filepath.Join(d.EnabledDir(), name)

	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing symlink: %w", err)
	}

	return nil
}

// StopRunsvdir stops the runsvdir process if it's running
func (d *DevTree) StopRunsvdir() error {
	pidFile := d.PIDFile()

	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading pid file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return fmt.Errorf("parsing pid: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding process: %w", err)
	}

	// Try graceful shutdown first, then force kill
	if err := proc.Signal(os.Interrupt); err != nil {
		if err := proc.Kill(); err != nil {
			return fmt.Errorf("killing process: %w", err)
		}
	}

	os.Remove(pidFile)
	return nil
}
