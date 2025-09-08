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
	// Name is the service name
	Name string
	// Dir is the base directory where the service will be created
	Dir string
	// Cmd is the command and arguments to execute
	Cmd []string
	// Cwd is the working directory for the service
	Cwd string
	// Umask sets the file mode creation mask
	Umask fs.FileMode
	// Env contains environment variables for the service
	Env map[string]string
	// Chpst configures process limits and user context
	Chpst *ChpstBuilder
	// Svlogd configures logging
	Svlogd *SvlogdBuilder
	// Finish is the command to run when the service stops
	Finish []string
	// StderrPath is an optional path to redirect stderr (if different from stdout)
	StderrPath string
	// ChpstPath is the path to the chpst binary
	ChpstPath string
	// SvlogdPath is the path to the svlogd binary
	SvlogdPath string
}

// ChpstBuilder configures chpst options for process control
type ChpstBuilder struct {
	// User to run the process as
	User string
	// Group to run the process as
	Group string
	// Nice value for process priority
	Nice int
	// IONice value for I/O priority
	IONice int
	// LimitMem sets memory limit in bytes
	LimitMem int64
	// LimitFiles sets maximum number of open files
	LimitFiles int
	// LimitProcs sets maximum number of processes
	LimitProcs int
	// LimitCPU sets CPU time limit in seconds
	LimitCPU int
	// Root changes the root directory
	Root string
}

// SvlogdBuilder configures svlogd logging options
type SvlogdBuilder struct {
	// Size is the maximum size of current log file in bytes
	Size int64
	// Num is the number of old log files to keep
	Num int
	// Timeout is the maximum age of current log file in seconds
	Timeout int
	// Processor is an optional processor script for log files
	Processor string
	// Config contains additional svlogd configuration lines
	Config []string
	// Timestamp adds timestamps to log lines
	Timestamp bool
	// Replace replaces non-printable characters
	Replace bool
	// Prefix adds a prefix to each log line
	Prefix string
}

// NewServiceBuilder creates a new ServiceBuilder with default settings
func NewServiceBuilder(name, dir string) *ServiceBuilder {
	return &ServiceBuilder{
		Name:       name,
		Dir:        dir,
		Env:        make(map[string]string),
		Umask:      DefaultUmask,
		ChpstPath:  DefaultChpstPath,
		SvlogdPath: DefaultSvlogdPath,
	}
}

// WithCmd sets the command to execute
func (b *ServiceBuilder) WithCmd(cmd []string) *ServiceBuilder {
	b.Cmd = cmd
	return b
}

// WithCwd sets the working directory
func (b *ServiceBuilder) WithCwd(cwd string) *ServiceBuilder {
	b.Cwd = cwd
	return b
}

// WithUmask sets the file mode creation mask
func (b *ServiceBuilder) WithUmask(umask fs.FileMode) *ServiceBuilder {
	b.Umask = umask
	return b
}

// WithEnv adds an environment variable
func (b *ServiceBuilder) WithEnv(key, value string) *ServiceBuilder {
	b.Env[key] = value
	return b
}

// WithChpst configures process control settings
func (b *ServiceBuilder) WithChpst(fn func(*ChpstBuilder)) *ServiceBuilder {
	if b.Chpst == nil {
		b.Chpst = &ChpstBuilder{}
	}
	fn(b.Chpst)
	return b
}

// WithChpstPath sets the path to the chpst binary
func (b *ServiceBuilder) WithChpstPath(path string) *ServiceBuilder {
	b.ChpstPath = path
	return b
}

// WithSvlogd configures logging settings
func (b *ServiceBuilder) WithSvlogd(fn func(*SvlogdBuilder)) *ServiceBuilder {
	if b.Svlogd == nil {
		b.Svlogd = &SvlogdBuilder{
			Size:      1000000,
			Num:       10,
			Timestamp: true,
		}
	}
	fn(b.Svlogd)
	return b
}

// WithSvlogdPath sets the path to the svlogd binary
func (b *ServiceBuilder) WithSvlogdPath(path string) *ServiceBuilder {
	b.SvlogdPath = path
	return b
}

// WithFinish sets the command to run when the service stops
func (b *ServiceBuilder) WithFinish(cmd []string) *ServiceBuilder {
	b.Finish = cmd
	return b
}

// WithStderrPath sets a separate path for stderr output
func (b *ServiceBuilder) WithStderrPath(path string) *ServiceBuilder {
	b.StderrPath = path
	return b
}

// buildArgs constructs the command-line arguments for chpst
func (c *ChpstBuilder) buildArgs() []string {
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
func (s *SvlogdBuilder) buildArgs() []string {
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
	if b.Dir == "" {
		return fmt.Errorf("service directory not specified")
	}
	if len(b.Cmd) == 0 {
		return fmt.Errorf("command not specified")
	}

	serviceDir := filepath.Join(b.Dir, b.Name)
	if err := os.MkdirAll(serviceDir, DirMode); err != nil {
		return fmt.Errorf("creating service directory: %w", err)
	}

	if len(b.Env) > 0 {
		envDir := filepath.Join(serviceDir, "env")
		if err := os.MkdirAll(envDir, DirMode); err != nil {
			return fmt.Errorf("creating env directory: %w", err)
		}

		for key, value := range b.Env {
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

	if len(b.Finish) > 0 {
		finishScript := b.buildFinishScript()
		finishFile := filepath.Join(serviceDir, "finish")
		if err := renameio.WriteFile(finishFile, []byte(finishScript), ExecMode); err != nil {
			return fmt.Errorf("writing finish script: %w", err)
		}
	}

	if b.Svlogd != nil {
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
	if b.StderrPath != "" {
		lines = append(lines, fmt.Sprintf("exec 2>%s", shellQuote(b.StderrPath)))
	} else {
		lines = append(lines, "exec 2>&1")
	}

	if b.Umask != 0 {
		lines = append(lines, fmt.Sprintf("umask %04o", b.Umask))
	}

	if b.Cwd != "" {
		lines = append(lines, fmt.Sprintf("cd %s", shellQuote(b.Cwd)))
	}

	// Calculate capacity needed
	capacity := len(b.Cmd)
	if len(b.Env) > 0 {
		capacity += 3 // chpst -e ./env
	}
	if b.Chpst != nil {
		capacity += 1 + len(b.Chpst.buildArgs())
	}

	cmdParts := make([]string, 0, capacity)

	if len(b.Env) > 0 {
		cmdParts = append(cmdParts, b.ChpstPath, "-e", "./env")
	}

	if b.Chpst != nil {
		cmdParts = append(cmdParts, b.ChpstPath)
		cmdParts = append(cmdParts, b.Chpst.buildArgs()...)
	}

	for _, part := range b.Cmd {
		cmdParts = append(cmdParts, shellQuote(part))
	}

	lines = append(lines, "exec "+strings.Join(cmdParts, " "))

	return strings.Join(lines, "\n") + "\n"
}

// buildFinishScript generates the finish script for the service
func (b *ServiceBuilder) buildFinishScript() string {
	var lines []string
	lines = append(lines, "#!/bin/sh")

	cmdParts := make([]string, 0, len(b.Finish))
	for _, part := range b.Finish {
		cmdParts = append(cmdParts, shellQuote(part))
	}

	lines = append(lines, "exec "+strings.Join(cmdParts, " "))

	return strings.Join(lines, "\n") + "\n"
}

// buildLogRunScript generates the log/run script for svlogd
func (b *ServiceBuilder) buildLogRunScript() string {
	var lines []string
	lines = append(lines, "#!/bin/sh")

	cmdParts := []string{b.SvlogdPath}
	if b.Svlogd.Timestamp {
		cmdParts = append(cmdParts, "-tt")
	}
	if b.Svlogd.Replace {
		cmdParts = append(cmdParts, "-r")
	}
	cmdParts = append(cmdParts, b.Svlogd.buildArgs()...)

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
