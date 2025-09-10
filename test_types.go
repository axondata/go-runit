//go:build linux

package svcmgr

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// SupervisionSystem represents a supervision system to test
type SupervisionSystem struct {
	Name      string
	Type      ServiceType
	Available bool
	Config    *ServiceConfig
	Setup     func() error
	Teardown  func() error
}

// TestLogger handles logging with timestamps
type TestLogger struct {
	t       *testing.T
	mu      sync.Mutex
	logs    []string
	verbose bool
	file    *os.File
}

func NewTestLogger(t *testing.T, logFile string, verbose bool) (*TestLogger, error) {
	var file *os.File
	var err error

	if logFile != "" {
		file, err = os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("opening log file: %w", err)
		}
	}

	return &TestLogger{
		t:       t,
		verbose: verbose,
		file:    file,
	}, nil
}

func (l *TestLogger) Log(format string, args ...interface{}) {
	if l == nil {
		return
	}

	timestamp := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	logLine := fmt.Sprintf("[%s] %s", timestamp, msg)

	l.mu.Lock()
	l.logs = append(l.logs, logLine)
	l.mu.Unlock()

	if l.verbose {
		l.t.Log(logLine)
	}

	if l.file != nil {
		l.mu.Lock()
		fmt.Fprintln(l.file, logLine)
		l.mu.Unlock()
	}
}

func (l *TestLogger) Close() {
	if l != nil && l.file != nil {
		l.file.Close()
	}
}

func (l *TestLogger) GetLogs() []string {
	if l == nil {
		return nil
	}
	return l.logs
}
