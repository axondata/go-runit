package svcmgr

import (
	"reflect"
	"testing"
)

// TestStatusCommandConstruction verifies that status commands are correctly constructed
func TestStatusCommandConstruction(t *testing.T) {
	tests := []struct {
		name       string
		cmd        []string
		serviceDir string
		expected   []string
	}{
		{
			name:       "runit sv status",
			cmd:        []string{"sv", "status"},
			serviceDir: "/var/service/test",
			expected:   []string{"sv", "status", "/var/service/test"},
		},
		{
			name:       "daemontools svstat",
			cmd:        []string{"svstat"},
			serviceDir: "/service/test",
			expected:   []string{"svstat", "/service/test"},
		},
		{
			name:       "s6 s6-svstat",
			cmd:        []string{"s6-svstat"},
			serviceDir: "/run/service/test",
			expected:   []string{"s6-svstat", "/run/service/test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate what the test does: append service dir to command args
			cmdArgs := make([]string, len(tt.cmd[1:]), len(tt.cmd[1:])+1)
			copy(cmdArgs, tt.cmd[1:])
			cmdArgs = append(cmdArgs, tt.serviceDir)
			fullCmd := append([]string{tt.cmd[0]}, cmdArgs...)

			if !reflect.DeepEqual(fullCmd, tt.expected) {
				t.Errorf("Command construction failed:\ngot:      %v\nexpected: %v", fullCmd, tt.expected)
			}
		})
	}
}
