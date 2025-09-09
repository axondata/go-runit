//go:build linux

package runit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/renameio/v2"
)

// BuilderSystemd extends ServiceBuilder to generate systemd unit files
type BuilderSystemd struct {
	*ServiceBuilder
	// UseSudo indicates whether to use sudo for privileged operations
	UseSudo bool
	// SudoCommand is the sudo command to use (default: "sudo")
	SudoCommand string
	// UnitDir is the directory where unit files are written (default: /etc/systemd/system)
	UnitDir string
	// SystemctlPath is the path to systemctl binary
	SystemctlPath string
}

// NewBuilderSystemd creates a new BuilderSystemd from a ServiceBuilder
func NewBuilderSystemd(sb *ServiceBuilder) *BuilderSystemd {
	return &BuilderSystemd{
		ServiceBuilder: sb,
		UseSudo:        os.Geteuid() != 0, // Auto-detect if we need sudo
		SudoCommand:    "sudo",
		UnitDir:        "/etc/systemd/system",
		SystemctlPath:  "systemctl",
	}
}

// WithSudo configures sudo usage
func (b *BuilderSystemd) WithSudo(use bool, command string) *BuilderSystemd {
	b.UseSudo = use
	if command != "" {
		b.SudoCommand = command
	}
	return b
}

// WithUnitDir sets the systemd unit directory
func (b *BuilderSystemd) WithUnitDir(dir string) *BuilderSystemd {
	b.UnitDir = dir
	return b
}

// BuildSystemdUnit generates the systemd unit file content
func (b *BuilderSystemd) BuildSystemdUnit() (string, error) {
	c := b.ServiceBuilder.config
	if len(c.Cmd) == 0 {
		return "", fmt.Errorf("command not specified")
	}

	var unit strings.Builder

	// [Unit] section
	unit.WriteString("[Unit]\n")
	unit.WriteString(fmt.Sprintf("Description=%s service\n", c.Name))
	unit.WriteString("After=network.target\n")

	// Add documentation link if available
	unit.WriteString("# Managed by go-runit systemd adapter\n")
	unit.WriteString("\n")

	// [Service] section
	unit.WriteString("[Service]\n")
	unit.WriteString("Type=simple\n")
	unit.WriteString("Restart=always\n")
	unit.WriteString("RestartSec=1\n")
	unit.WriteString("KillMode=mixed\n")
	unit.WriteString("KillSignal=SIGTERM\n")
	unit.WriteString("TimeoutStopSec=10\n")

	// Map ChpstConfig fields to systemd directives
	if c.Chpst != nil {
		if c.Chpst.User != "" {
			unit.WriteString(fmt.Sprintf("User=%s\n", c.Chpst.User))
		}
		if c.Chpst.Group != "" {
			unit.WriteString(fmt.Sprintf("Group=%s\n", c.Chpst.Group))
		}
		if c.Chpst.Nice != 0 {
			unit.WriteString(fmt.Sprintf("Nice=%d\n", c.Chpst.Nice))
		}
		if c.Chpst.IONice != 0 {
			// Map IONice to IOSchedulingClass and IOSchedulingPriority
			// IONice 1-3 = best-effort, 4-7 = idle
			if c.Chpst.IONice <= 3 {
				unit.WriteString("IOSchedulingClass=2\n") // best-effort
				unit.WriteString(fmt.Sprintf("IOSchedulingPriority=%d\n", c.Chpst.IONice))
			} else {
				unit.WriteString("IOSchedulingClass=3\n") // idle
			}
		}
		if c.Chpst.LimitMem > 0 {
			unit.WriteString(fmt.Sprintf("MemoryLimit=%d\n", c.Chpst.LimitMem))
		}
		if c.Chpst.LimitFiles > 0 {
			unit.WriteString(fmt.Sprintf("LimitNOFILE=%d\n", c.Chpst.LimitFiles))
		}
		if c.Chpst.LimitProcs > 0 {
			unit.WriteString(fmt.Sprintf("LimitNPROC=%d\n", c.Chpst.LimitProcs))
		}
		if c.Chpst.LimitCPU > 0 {
			unit.WriteString(fmt.Sprintf("LimitCPU=%d\n", c.Chpst.LimitCPU))
		}
		if c.Chpst.Root != "" {
			unit.WriteString(fmt.Sprintf("RootDirectory=%s\n", c.Chpst.Root))
		}
	}

	// Working directory
	if c.Cwd != "" {
		unit.WriteString(fmt.Sprintf("WorkingDirectory=%s\n", c.Cwd))
	}

	// Umask
	if c.Umask != 0 {
		unit.WriteString(fmt.Sprintf("UMask=%04o\n", c.Umask))
	}

	// Environment variables
	for key, value := range c.Env {
		// Escape quotes in values
		escapedValue := strings.ReplaceAll(value, `"`, `\"`)
		unit.WriteString(fmt.Sprintf("Environment=\"%s=%s\"\n", key, escapedValue))
	}

	// Command - properly quote arguments
	execStart := c.Cmd[0]
	for i := 1; i < len(c.Cmd); i++ {
		arg := c.Cmd[i]
		// Quote args with spaces or special characters
		if strings.ContainsAny(arg, " \t\n\"'\\$") {
			arg = fmt.Sprintf("%q", arg)
		}
		execStart += " " + arg
	}
	unit.WriteString(fmt.Sprintf("ExecStart=%s\n", execStart))

	// Finish command maps to ExecStopPost
	if len(c.Finish) > 0 {
		execStop := c.Finish[0]
		for i := 1; i < len(c.Finish); i++ {
			arg := c.Finish[i]
			if strings.ContainsAny(arg, " \t\n\"'\\$") {
				arg = fmt.Sprintf("%q", arg)
			}
			execStop += " " + arg
		}
		unit.WriteString(fmt.Sprintf("ExecStopPost=%s\n", execStop))
	}

	// Logging configuration
	if c.Svlogd != nil {
		unit.WriteString("StandardOutput=journal\n")
		unit.WriteString("StandardError=journal\n")

		// Map svlogd settings to journald where possible
		if c.Svlogd.Prefix != "" {
			unit.WriteString(fmt.Sprintf("SyslogIdentifier=%s\n", c.Svlogd.Prefix))
		}

		// Note: svlogd size/rotation settings don't map directly
		// journald handles its own rotation
	} else if c.StderrPath != "" {
		// Custom stderr redirection
		unit.WriteString("StandardOutput=journal\n")
		unit.WriteString(fmt.Sprintf("StandardError=file:%s\n", c.StderrPath))
	} else {
		// Default logging
		unit.WriteString("StandardOutput=journal\n")
		unit.WriteString("StandardError=journal\n")
	}

	unit.WriteString("\n")
	unit.WriteString("[Install]\n")
	unit.WriteString("WantedBy=multi-user.target\n")

	return unit.String(), nil
}

// Build creates and installs the systemd unit file
func (b *BuilderSystemd) Build() error {
	return b.BuildWithContext(context.Background())
}

// BuildWithContext creates and installs the systemd unit file with context
func (b *BuilderSystemd) BuildWithContext(ctx context.Context) error {
	// Generate unit content
	unitContent, err := b.BuildSystemdUnit()
	if err != nil {
		return fmt.Errorf("generating unit file: %w", err)
	}

	// Determine unit file path
	unitName := fmt.Sprintf("%s.service", b.ServiceBuilder.config.Name)
	unitPath := filepath.Join(b.UnitDir, unitName)

	// Write unit file
	if err := b.writeUnitFile(ctx, unitPath, unitContent); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	// Reload systemd
	if err := b.reloadSystemd(ctx); err != nil {
		return fmt.Errorf("reloading systemd: %w", err)
	}

	return nil
}

// writeUnitFile writes the unit file, using sudo if necessary
func (b *BuilderSystemd) writeUnitFile(ctx context.Context, path string, content string) error {
	if !b.UseSudo {
		// Direct write if we have permissions
		return renameio.WriteFile(path, []byte(content), 0o644)
	}

	// Use sudo tee to write the file
	// Equivalent to: echo "content" | sudo tee /path/to/file
	cmd := exec.CommandContext(ctx, b.SudoCommand, "tee", path)
	cmd.Stdin = strings.NewReader(content)

	// Capture output to avoid printing to stdout
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo tee failed: %w (output: %s)", err, out.String())
	}

	return nil
}

// reloadSystemd runs systemctl daemon-reload
func (b *BuilderSystemd) reloadSystemd(ctx context.Context) error {
	var cmd *exec.Cmd

	if b.UseSudo {
		cmd = exec.CommandContext(ctx, b.SudoCommand, b.SystemctlPath, "daemon-reload")
	} else {
		cmd = exec.CommandContext(ctx, b.SystemctlPath, "daemon-reload")
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("daemon-reload failed: %w (output: %s)", err, out.String())
	}

	return nil
}

// Enable enables the systemd service to start on boot
func (b *BuilderSystemd) Enable(ctx context.Context) error {
	serviceName := fmt.Sprintf("%s.service", b.ServiceBuilder.config.Name)

	var cmd *exec.Cmd
	if b.UseSudo {
		cmd = exec.CommandContext(ctx, b.SudoCommand, b.SystemctlPath, "enable", serviceName)
	} else {
		cmd = exec.CommandContext(ctx, b.SystemctlPath, "enable", serviceName)
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("enable failed: %w (output: %s)", err, out.String())
	}

	return nil
}

// Remove removes the systemd unit file
func (b *BuilderSystemd) Remove(ctx context.Context) error {
	serviceName := fmt.Sprintf("%s.service", b.ServiceBuilder.config.Name)
	unitPath := filepath.Join(b.UnitDir, serviceName)

	// Stop and disable the service first
	client := &ClientSystemd{
		ServiceName:   b.ServiceBuilder.config.Name,
		UseSudo:       b.UseSudo,
		SudoCommand:   b.SudoCommand,
		SystemctlPath: b.SystemctlPath,
	}

	// Stop the service (ignore errors if it's not running)
	_ = client.Stop(ctx)

	// Disable the service (ignore errors if it's not enabled)
	var cmd *exec.Cmd
	if b.UseSudo {
		cmd = exec.CommandContext(ctx, b.SudoCommand, b.SystemctlPath, "disable", serviceName)
	} else {
		cmd = exec.CommandContext(ctx, b.SystemctlPath, "disable", serviceName)
	}
	_ = cmd.Run()

	// Remove the unit file
	if b.UseSudo {
		cmd = exec.CommandContext(ctx, b.SudoCommand, "rm", "-f", unitPath)
	} else {
		cmd = exec.CommandContext(ctx, "rm", "-f", unitPath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("removing unit file: %w", err)
	}

	// Reload systemd
	return b.reloadSystemd(ctx)
}
