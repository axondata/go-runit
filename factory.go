package runit

// ServiceType represents the type of service supervision system
type ServiceType int

const (
	// ServiceTypeUnknown represents an unknown supervision system
	ServiceTypeUnknown ServiceType = iota
	// ServiceTypeRunit represents runit supervision
	ServiceTypeRunit
	// ServiceTypeDaemontools represents daemontools supervision
	ServiceTypeDaemontools
	// ServiceTypeS6 represents s6 supervision
	ServiceTypeS6
)

// ServiceType string constants
const (
	serviceTypeUnknownStr     = "unknown"
	serviceTypeRunitStr       = "runit"
	serviceTypeDaemontoolsStr = "daemontools"
	serviceTypeS6Str          = "s6"
)

// ServiceConfig contains configuration for different supervision systems
type ServiceConfig struct {
	// Type specifies which supervision system this is for
	Type ServiceType
	// ServiceDir is the base service directory (e.g., /etc/service, /service, /run/service)
	ServiceDir string
	// ChpstPath is the path to the privilege/resource control tool
	ChpstPath string
	// LoggerPath is the path to the logger tool
	LoggerPath string
	// RunsvdirPath is the path to the service scanner
	RunsvdirPath string
	// SupportedOps contains the set of supported operations
	SupportedOps map[Operation]struct{}
}

// RunitConfig returns the default configuration for runit
//
//nolint:revive // Clear naming for multiple config types
func RunitConfig() *ServiceConfig {
	return &ServiceConfig{
		Type:         ServiceTypeRunit,
		ServiceDir:   "/etc/service",
		ChpstPath:    "chpst",
		LoggerPath:   "svlogd",
		RunsvdirPath: "runsvdir",
		SupportedOps: allOperations(),
	}
}

// DaemontoolsConfig returns the default configuration for daemontools
//
//nolint:revive // Clear naming for multiple config types
func DaemontoolsConfig() *ServiceConfig {
	config := &ServiceConfig{
		Type:         ServiceTypeDaemontools,
		ServiceDir:   "/service",
		ChpstPath:    "setuidgid", // or envuidgid
		LoggerPath:   "multilog",
		RunsvdirPath: "svscan",
		SupportedOps: allOperations(),
	}

	// Daemontools doesn't support these operations
	delete(config.SupportedOps, OpOnce) // No 'o' command
	delete(config.SupportedOps, OpQuit) // No 'q' command

	return config
}

// S6Config returns the default configuration for s6
//
//nolint:revive // Clear naming for multiple config types
func S6Config() *ServiceConfig {
	return &ServiceConfig{
		Type:         ServiceTypeS6,
		ServiceDir:   "/run/service", // Common s6 location
		ChpstPath:    "s6-setuidgid",
		LoggerPath:   "s6-log",
		RunsvdirPath: "s6-svscan",
		SupportedOps: allOperations(),
	}
}

// allOperations returns a set with all operations enabled
func allOperations() map[Operation]struct{} {
	return map[Operation]struct{}{
		OpUp:        {},
		OpOnce:      {},
		OpDown:      {},
		OpTerm:      {},
		OpInterrupt: {},
		OpHUP:       {},
		OpAlarm:     {},
		OpQuit:      {},
		OpKill:      {},
		OpPause:     {},
		OpCont:      {},
		OpExit:      {},
		OpStatus:    {},
	}
}

// NewClientWithConfig creates a new client with the specified service configuration
func NewClientWithConfig(serviceDir string, config *ServiceConfig, opts ...Option) (*Client, error) {
	// Validate that operations used are supported by this service type
	// This is primarily for documentation since the protocol is identical
	// The actual client doesn't need changes as daemontools/runit/s6 all use
	// the same supervise/control and supervise/status format

	// Create client with standard options
	client, err := New(serviceDir, opts...)
	if err != nil {
		return nil, err
	}

	// Set the config to enable operation validation
	client.Config = config

	return client, nil
}

// NewServiceBuilderWithConfig creates a service builder for the specified supervision system
func NewServiceBuilderWithConfig(name, dir string, config *ServiceConfig) *ServiceBuilder {
	builder := NewServiceBuilder(name, dir)

	// Set paths based on config
	if config.ChpstPath != "" {
		builder.ChpstPath = config.ChpstPath
	}
	if config.LoggerPath != "" {
		builder.SvlogdPath = config.LoggerPath
	}

	// For s6, we might need to adjust the service structure slightly
	if config.Type == ServiceTypeS6 {
		// s6 uses 's6-log' with a different config format
		// but the basic structure is compatible
		builder.WithSvlogdPath("s6-log")
	}

	return builder
}

// IsOperationSupported checks if an operation is supported by this service type
func (c *ServiceConfig) IsOperationSupported(op Operation) bool {
	_, ok := c.SupportedOps[op]
	return ok
}

// ServiceBuilderRunit creates a service builder configured for runit
func ServiceBuilderRunit(name, dir string) *ServiceBuilder {
	return NewServiceBuilderWithConfig(name, dir, RunitConfig())
}

// ServiceBuilderDaemontools creates a service builder configured for daemontools
func ServiceBuilderDaemontools(name, dir string) *ServiceBuilder {
	return NewServiceBuilderWithConfig(name, dir, DaemontoolsConfig())
}

// ServiceBuilderS6 creates a service builder configured for s6
func ServiceBuilderS6(name, dir string) *ServiceBuilder {
	return NewServiceBuilderWithConfig(name, dir, S6Config())
}

// String returns the string representation of ServiceType
func (st ServiceType) String() string {
	switch st {
	case ServiceTypeRunit:
		return serviceTypeRunitStr
	case ServiceTypeDaemontools:
		return serviceTypeDaemontoolsStr
	case ServiceTypeS6:
		return serviceTypeS6Str
	case ServiceTypeUnknown:
		fallthrough
	default:
		return serviceTypeUnknownStr
	}
}
