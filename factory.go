package svcmgr

import (
	"fmt"
	"path/filepath"
)

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
	// ServiceTypeSystemd represents systemd supervision
	ServiceTypeSystemd
)

// ServiceType string constants
const (
	serviceTypeUnknownStr     = "unknown"
	serviceTypeRunitStr       = "runit"
	serviceTypeDaemontoolsStr = "daemontools"
	serviceTypeS6Str          = "s6"
	serviceTypeSystemdStr     = "systemd"
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
		OpUSR1:      {},
		OpUSR2:      {},
		OpKill:      {},
		OpPause:     {},
		OpCont:      {},
		OpExit:      {},
		OpStatus:    {},
	}
}

// NewClient creates a ServiceClient based on the detected or specified supervision system
func NewClient(serviceDir string, serviceType ServiceType) (ServiceClient, error) {
	switch serviceType {
	case ServiceTypeRunit:
		return NewClientRunit(serviceDir)
	case ServiceTypeDaemontools:
		return NewClientDaemontools(serviceDir)
	case ServiceTypeS6:
		return NewClientS6(serviceDir)
	case ServiceTypeSystemd:
		// Systemd uses service names, not directories
		// Extract service name from path
		serviceName := filepath.Base(serviceDir)
		return NewClientSystemd(serviceName), nil
	default:
		return nil, fmt.Errorf("unsupported service type: %v", serviceType)
	}
}

// NewServiceBuilderWithConfig creates a service builder for the specified supervision system
func NewServiceBuilderWithConfig(name, dir string, config *ServiceConfig) *ServiceBuilder {
	builder := NewServiceBuilder(name, dir)

	// Set paths based on config
	if config.ChpstPath != "" {
		builder.config.ChpstPath = config.ChpstPath
	}
	if config.LoggerPath != "" {
		builder.config.SvlogdPath = config.LoggerPath
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

// String returns the string representation of ServiceType
func (st ServiceType) String() string {
	switch st {
	case ServiceTypeRunit:
		return serviceTypeRunitStr
	case ServiceTypeDaemontools:
		return serviceTypeDaemontoolsStr
	case ServiceTypeS6:
		return serviceTypeS6Str
	case ServiceTypeSystemd:
		return serviceTypeSystemdStr
	case ServiceTypeUnknown:
		fallthrough
	default:
		return serviceTypeUnknownStr
	}
}
