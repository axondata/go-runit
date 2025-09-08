package runit

import "io/fs"

// ServiceBuilderConfig represents the configuration for a service
// This struct contains all the settings that can be configured for a service
type ServiceBuilderConfig struct {
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
	Chpst *ChpstConfig
	// Svlogd configures logging
	Svlogd *ConfigSvlogd
	// Finish is the command to run when the service stops
	Finish []string
	// StderrPath is an optional path to redirect stderr (if different from stdout)
	StderrPath string
	// ChpstPath is the path to the chpst binary
	ChpstPath string
	// SvlogdPath is the path to the svlogd binary
	SvlogdPath string
}

// ChpstConfig configures chpst options for process control
type ChpstConfig struct {
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

// ConfigSvlogd configures svlogd logging options
type ConfigSvlogd struct {
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

// Clone creates a deep copy of the ServiceBuilderConfig
func (c *ServiceBuilderConfig) Clone() *ServiceBuilderConfig {
	if c == nil {
		return nil
	}

	clone := &ServiceBuilderConfig{
		Name:       c.Name,
		Dir:        c.Dir,
		Cwd:        c.Cwd,
		Umask:      c.Umask,
		Finish:     append([]string(nil), c.Finish...),
		StderrPath: c.StderrPath,
		ChpstPath:  c.ChpstPath,
		SvlogdPath: c.SvlogdPath,
	}

	// Deep copy Cmd
	if c.Cmd != nil {
		clone.Cmd = append([]string(nil), c.Cmd...)
	}

	// Deep copy Env
	if c.Env != nil {
		clone.Env = make(map[string]string, len(c.Env))
		for k, v := range c.Env {
			clone.Env[k] = v
		}
	}

	// Deep copy Chpst
	if c.Chpst != nil {
		clone.Chpst = &ChpstConfig{
			User:       c.Chpst.User,
			Group:      c.Chpst.Group,
			Nice:       c.Chpst.Nice,
			IONice:     c.Chpst.IONice,
			LimitMem:   c.Chpst.LimitMem,
			LimitFiles: c.Chpst.LimitFiles,
			LimitProcs: c.Chpst.LimitProcs,
			LimitCPU:   c.Chpst.LimitCPU,
			Root:       c.Chpst.Root,
		}
	}

	// Deep copy Svlogd
	if c.Svlogd != nil {
		clone.Svlogd = &ConfigSvlogd{
			Size:      c.Svlogd.Size,
			Num:       c.Svlogd.Num,
			Timeout:   c.Svlogd.Timeout,
			Processor: c.Svlogd.Processor,
			Config:    append([]string(nil), c.Svlogd.Config...),
			Timestamp: c.Svlogd.Timestamp,
			Replace:   c.Svlogd.Replace,
			Prefix:    c.Svlogd.Prefix,
		}
	}

	return clone
}
