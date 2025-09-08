package runit

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/renameio/v2"
)

// ServiceBuilder provides a fluent interface for creating runit service directories
// with run scripts, environment variables, logging, and process control settings.
type ServiceBuilder struct {
	config *ServiceBuilderConfig
}

// NewServiceBuilder creates a new ServiceBuilder with default settings
func NewServiceBuilder(name, dir string) *ServiceBuilder {
	return &ServiceBuilder{
		config: &ServiceBuilderConfig{
			Name:       name,
			Dir:        dir,
			Env:        make(map[string]string),
			Umask:      DefaultUmask,
			ChpstPath:  DefaultChpstPath,
			SvlogdPath: DefaultSvlogdPath,
		},
	}
}

// Config returns a copy of the current configuration
func (b *ServiceBuilder) Config() *ServiceBuilderConfig {
	return b.config.Clone()
}

// WithCmd sets the command to execute
func (b *ServiceBuilder) WithCmd(cmd []string) *ServiceBuilder {
	b.config.Cmd = cmd
	return b
}

// WithCwd sets the working directory
func (b *ServiceBuilder) WithCwd(cwd string) *ServiceBuilder {
	b.config.Cwd = cwd
	return b
}

// WithUmask sets the file mode creation mask
func (b *ServiceBuilder) WithUmask(umask fs.FileMode) *ServiceBuilder {
	b.config.Umask = umask
	return b
}

// WithEnv adds an environment variable
func (b *ServiceBuilder) WithEnv(key, value string) *ServiceBuilder {
	b.config.Env[key] = value
	return b
}

// WithChpst configures process control settings
func (b *ServiceBuilder) WithChpst(fn func(*ChpstConfig)) *ServiceBuilder {
	if b.config.Chpst == nil {
		b.config.Chpst = &ChpstConfig{}
	}
	fn(b.config.Chpst)
	return b
}

// WithChpstPath sets the path to the chpst binary
func (b *ServiceBuilder) WithChpstPath(path string) *ServiceBuilder {
	b.config.ChpstPath = path
	return b
}

// WithSvlogd configures logging settings
func (b *ServiceBuilder) WithSvlogd(fn func(*ConfigSvlogd)) *ServiceBuilder {
	if b.config.Svlogd == nil {
		b.config.Svlogd = &ConfigSvlogd{
			Size:      1000000,
			Num:       10,
			Timestamp: true,
		}
	}
	fn(b.config.Svlogd)
	return b
}

// WithSvlogdPath sets the path to the svlogd binary
func (b *ServiceBuilder) WithSvlogdPath(path string) *ServiceBuilder {
	b.config.SvlogdPath = path
	return b
}

// WithFinish sets the command to run when the service stops
func (b *ServiceBuilder) WithFinish(cmd []string) *ServiceBuilder {
	b.config.Finish = cmd
	return b
}

// WithStderrPath sets a separate path for stderr output
func (b *ServiceBuilder) WithStderrPath(path string) *ServiceBuilder {
	b.config.StderrPath = path
	return b
}

// buildArgs constructs the command-line arguments for chpst
func (c *ChpstConfig) buildArgs() []string {
	var args []string

	if c.User != "" {
		args = append(args, "-u", c.User)
	}
	if c.Group != "" {
		args = append(args, "-U", c.Group)
	}
	if c.Nice != 0 {
		args = append(args, "-n", fmt.Sprintf("%d", c.Nice))
	}
	if c.LimitMem > 0 {
		args = append(args, "-m", fmt.Sprintf("%d", c.LimitMem))
	}
	if c.LimitFiles > 0 {
		args = append(args, "-o", fmt.Sprintf("%d", c.LimitFiles))
	}
	if c.LimitProcs > 0 {
		args = append(args, "-p", fmt.Sprintf("%d", c.LimitProcs))
	}
	if c.LimitCPU > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", c.LimitCPU))
	}
	if c.Root != "" {
		args = append(args, "-/", c.Root)
	}

	return args
}

// buildArgs constructs the command-line arguments for svlogd
func (s *ConfigSvlogd) buildArgs() []string {
	var args []string

	if s.Size > 0 {
		args = append(args, fmt.Sprintf("s%d", s.Size))
	}
	if s.Num > 0 {
		args = append(args, fmt.Sprintf("n%d", s.Num))
	}
	if s.Timeout > 0 {
		args = append(args, fmt.Sprintf("t%d", s.Timeout))
	}
	if s.Processor != "" {
		args = append(args, fmt.Sprintf("!%s", s.Processor))
	}
	if s.Prefix != "" {
		args = append(args, fmt.Sprintf("p%s", s.Prefix))
	}

	args = append(args, s.Config...)
	args = append(args, ".")

	return args
}

// Build creates the service directory structure and scripts
func (b *ServiceBuilder) Build() error {
	if b.config.Dir == "" {
		return fmt.Errorf("service directory not specified")
	}
	if len(b.config.Cmd) == 0 {
		return fmt.Errorf("command not specified")
	}

	serviceDir := filepath.Join(b.config.Dir, b.config.Name)
	if err := os.MkdirAll(serviceDir, DirMode); err != nil {
		return fmt.Errorf("creating service directory: %w", err)
	}

	if len(b.config.Env) > 0 {
		envDir := filepath.Join(serviceDir, "env")
		if err := os.MkdirAll(envDir, DirMode); err != nil {
			return fmt.Errorf("creating env directory: %w", err)
		}

		for key, value := range b.config.Env {
			envFile := filepath.Join(envDir, key)
			if err := renameio.WriteFile(envFile, []byte(value), FileMode); err != nil {
				return fmt.Errorf("writing env file %s: %w", key, err)
			}
		}
	}

	runScript := b.buildRunScript()
	runFile := filepath.Join(serviceDir, "run")
	if err := renameio.WriteFile(runFile, []byte(runScript), ExecMode); err != nil {
		return fmt.Errorf("writing run script: %w", err)
	}

	if len(b.config.Finish) > 0 {
		finishScript := b.buildFinishScript()
		finishFile := filepath.Join(serviceDir, "finish")
		if err := renameio.WriteFile(finishFile, []byte(finishScript), ExecMode); err != nil {
			return fmt.Errorf("writing finish script: %w", err)
		}
	}

	if b.config.Svlogd != nil {
		logDir := filepath.Join(serviceDir, "log")
		if err := os.MkdirAll(logDir, DirMode); err != nil {
			return fmt.Errorf("creating log directory: %w", err)
		}

		logRunScript := b.buildLogRunScript()
		logRunFile := filepath.Join(logDir, "run")
		if err := renameio.WriteFile(logRunFile, []byte(logRunScript), ExecMode); err != nil {
			return fmt.Errorf("writing log/run script: %w", err)
		}

		mainDir := filepath.Join(logDir, "main")
		if err := os.MkdirAll(mainDir, DirMode); err != nil {
			return fmt.Errorf("creating log/main directory: %w", err)
		}
	}

	return nil
}

// buildRunScript generates the run script for the service
func (b *ServiceBuilder) buildRunScript() string {
	var lines []string
	lines = append(lines, "#!/bin/sh")

	// Handle stderr redirection
	if b.config.StderrPath != "" {
		lines = append(lines, fmt.Sprintf("exec 2>%s", shellQuote(b.config.StderrPath)))
	} else {
		lines = append(lines, "exec 2>&1")
	}

	if b.config.Umask != 0 {
		lines = append(lines, fmt.Sprintf("umask %04o", b.config.Umask))
	}

	if b.config.Cwd != "" {
		lines = append(lines, fmt.Sprintf("cd %s", shellQuote(b.config.Cwd)))
	}

	// Calculate capacity needed
	capacity := len(b.config.Cmd)
	if len(b.config.Env) > 0 {
		capacity += 3 // chpst -e ./env
	}
	if b.config.Chpst != nil {
		capacity += 1 + len(b.config.Chpst.buildArgs())
	}

	cmdParts := make([]string, 0, capacity)

	if len(b.config.Env) > 0 {
		cmdParts = append(cmdParts, b.config.ChpstPath, "-e", "./env")
	}

	if b.config.Chpst != nil {
		cmdParts = append(cmdParts, b.config.ChpstPath)
		cmdParts = append(cmdParts, b.config.Chpst.buildArgs()...)
	}

	for _, part := range b.config.Cmd {
		cmdParts = append(cmdParts, shellQuote(part))
	}

	lines = append(lines, "exec "+strings.Join(cmdParts, " "))

	return strings.Join(lines, "\n") + "\n"
}

// buildFinishScript generates the finish script for the service
func (b *ServiceBuilder) buildFinishScript() string {
	var lines []string
	lines = append(lines, "#!/bin/sh")

	cmdParts := make([]string, 0, len(b.config.Finish))
	for _, part := range b.config.Finish {
		cmdParts = append(cmdParts, shellQuote(part))
	}

	lines = append(lines, "exec "+strings.Join(cmdParts, " "))

	return strings.Join(lines, "\n") + "\n"
}

// buildLogRunScript generates the log/run script for svlogd
func (b *ServiceBuilder) buildLogRunScript() string {
	var lines []string
	lines = append(lines, "#!/bin/sh")

	cmdParts := []string{b.config.SvlogdPath}
	if b.config.Svlogd.Timestamp {
		cmdParts = append(cmdParts, "-tt")
	}
	if b.config.Svlogd.Replace {
		cmdParts = append(cmdParts, "-r")
	}
	cmdParts = append(cmdParts, b.config.Svlogd.buildArgs()...)

	lines = append(lines, "exec "+strings.Join(cmdParts, " "))

	return strings.Join(lines, "\n") + "\n"
}

// shellQuote escapes a string for safe use in shell scripts
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}

	if !needsShellQuoting(s) {
		return s
	}

	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// needsShellQuoting checks if a string contains characters that require shell quoting
func needsShellQuoting(s string) bool {
	// Characters that require quoting in shell
	const specialChars = " \t\n'\"\\$`!*?[](){}<>|&;~"

	for _, r := range s {
		if strings.ContainsRune(specialChars, r) {
			return true
		}
	}
	return false
}
