package svcmgr

import (
	"testing"
)

func TestServiceConfigs(t *testing.T) {
	tests := []struct {
		name   string
		config *ServiceConfig
		want   struct {
			serviceDir string
			chpst      string
			logger     string
			scanner    string
			hasOnce    bool
			hasQuit    bool
		}
	}{
		{
			name:   "runit",
			config: ConfigRunit(),
			want: struct {
				serviceDir string
				chpst      string
				logger     string
				scanner    string
				hasOnce    bool
				hasQuit    bool
			}{
				serviceDir: "/etc/service",
				chpst:      "chpst",
				logger:     "svlogd",
				scanner:    "runsvdir",
				hasOnce:    true,
				hasQuit:    true,
			},
		},
		{
			name:   "daemontools",
			config: ConfigDaemontools(),
			want: struct {
				serviceDir string
				chpst      string
				logger     string
				scanner    string
				hasOnce    bool
				hasQuit    bool
			}{
				serviceDir: "/service",
				chpst:      "setuidgid",
				logger:     "multilog",
				scanner:    "svscan",
				hasOnce:    false, // daemontools doesn't support 'once'
				hasQuit:    false, // daemontools doesn't support 'quit'
			},
		},
		{
			name:   "s6",
			config: ConfigS6(),
			want: struct {
				serviceDir string
				chpst      string
				logger     string
				scanner    string
				hasOnce    bool
				hasQuit    bool
			}{
				serviceDir: "/run/service",
				chpst:      "s6-setuidgid",
				logger:     "s6-log",
				scanner:    "s6-svscan",
				hasOnce:    true,
				hasQuit:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.ServiceDir != tt.want.serviceDir {
				t.Errorf("ServiceDir = %v, want %v", tt.config.ServiceDir, tt.want.serviceDir)
			}
			if tt.config.ChpstPath != tt.want.chpst {
				t.Errorf("ChpstPath = %v, want %v", tt.config.ChpstPath, tt.want.chpst)
			}
			if tt.config.LoggerPath != tt.want.logger {
				t.Errorf("LoggerPath = %v, want %v", tt.config.LoggerPath, tt.want.logger)
			}
			if tt.config.RunsvdirPath != tt.want.scanner {
				t.Errorf("RunsvdirPath = %v, want %v", tt.config.RunsvdirPath, tt.want.scanner)
			}
			if tt.config.IsOperationSupported(OpOnce) != tt.want.hasOnce {
				t.Errorf("OpOnce supported = %v, want %v", tt.config.IsOperationSupported(OpOnce), tt.want.hasOnce)
			}
			if tt.config.IsOperationSupported(OpQuit) != tt.want.hasQuit {
				t.Errorf("OpQuit supported = %v, want %v", tt.config.IsOperationSupported(OpQuit), tt.want.hasQuit)
			}
		})
	}
}

func TestServiceBuilderRunit(t *testing.T) {
	builder := ServiceBuilderRunit("test", "/tmp/services")
	config := builder.Config()

	if config.ChpstPath != "chpst" {
		t.Errorf("ChpstPath = %v, want chpst", config.ChpstPath)
	}
	if config.SvlogdPath != "svlogd" {
		t.Errorf("SvlogdPath = %v, want svlogd", config.SvlogdPath)
	}
}

func TestServiceBuilderDaemontools(t *testing.T) {
	builder := ServiceBuilderDaemontools("test", "/tmp/services")
	config := builder.Config()

	if config.ChpstPath != "setuidgid" {
		t.Errorf("ChpstPath = %v, want setuidgid", config.ChpstPath)
	}
	if config.SvlogdPath != "multilog" {
		t.Errorf("SvlogdPath = %v, want multilog", config.SvlogdPath)
	}
}

func TestServiceBuilderS6(t *testing.T) {
	builder := ServiceBuilderS6("test", "/tmp/services")
	config := builder.Config()

	if config.ChpstPath != "s6-setuidgid" {
		t.Errorf("ChpstPath = %v, want s6-setuidgid", config.ChpstPath)
	}
	if config.SvlogdPath != "s6-log" {
		t.Errorf("SvlogdPath = %v, want s6-log", config.SvlogdPath)
	}
}

func TestNewClientWithConfig(t *testing.T) {
	// Test that we can create clients with different configs
	configs := []*ServiceConfig{
		ConfigRunit(),
		ConfigDaemontools(),
		ConfigS6(),
	}

	for _, config := range configs {
		t.Run(config.Type.String(), func(t *testing.T) {
			// This will fail if the service doesn't exist, but that's expected
			// We're just testing that the factory works
			_, err := NewClient("/nonexistent", config.Type)
			if err == nil {
				t.Error("Expected error for nonexistent service")
			}
		})
	}
}
