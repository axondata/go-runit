//go:build go1.18
// +build go1.18

package runit

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/renameio/v2"
)

// FuzzClientOperations tests various client operations with random inputs
func FuzzClientOperations(f *testing.F) {
	// Check if we can create unix sockets before starting parallel tests
	tmpDir := f.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	listener, err := net.Listen("unix", testSocketPath)
	if err != nil {
		f.Skip("Cannot create unix sockets on this platform")
	}
	listener.Close()
	os.Remove(testSocketPath)

	// Add seed corpus with valid operations
	ops := []byte{'u', 'd', 'o', 't', 'k', 'h', 'i', 'a', 'q', 'p', 'c', 'x'}
	for _, op := range ops {
		f.Add(op)
	}

	// Add some invalid operations
	f.Add(byte(0))
	f.Add(byte(255))
	f.Add(byte('z'))

	f.Fuzz(func(t *testing.T, opByte byte) {
		// Create a test environment with shorter path for Unix socket limit
		// Unix sockets have a path length limit (typically 104-108 bytes)
		tmpDir, err := os.MkdirTemp("/tmp", "fuzz")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(tmpDir) })
		
		superviseDir := filepath.Join(tmpDir, "supervise")
		if err := os.MkdirAll(superviseDir, 0o755); err != nil {
			t.Fatal(err)
		}

		controlPath := filepath.Join(superviseDir, "control")

		// Create a mock control socket
		listener, err := net.Listen("unix", controlPath)
		if err != nil {
			t.Fatalf("Failed to create unix socket: %v", err)
		}
		defer func() { _ = listener.Close() }()

		// Accept connections in background
		received := make(chan byte, 1)
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()

			var buf [1]byte
			if _, err := conn.Read(buf[:]); err == nil {
				received <- buf[0]
			}
		}()

		// Create client
		client, err := NewClientRunit(tmpDir)
		if err != nil {
			t.Fatal(err)
		}

		// Create an operation from the fuzzed byte
		var op Operation
		switch opByte {
		case 'u':
			op = OpUp
		case 'd':
			op = OpDown
		case 'o':
			op = OpOnce
		case 't':
			op = OpTerm
		case 'k':
			op = OpKill
		case 'h':
			op = OpHUP
		case 'i':
			op = OpInterrupt
		case 'a':
			op = OpAlarm
		case 'q':
			op = OpQuit
		case 'p':
			op = OpPause
		case 'c':
			op = OpCont
		case 'x':
			op = OpExit
		default:
			// Test with an arbitrary operation value
			op = Operation(opByte)
		}

		// Test that send doesn't panic
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_ = client.send(ctx, op)

		// Check if we received the expected byte (for valid operations)
		select {
		case cmd := <-received:
			if cmd != op.Byte() {
				t.Errorf("received command = %c, want %c", cmd, op.Byte())
			}
		case <-time.After(50 * time.Millisecond):
			// Timeout is ok for invalid operations
		}
	})
}

// FuzzStatusParsing tests Status method with various file contents
func FuzzStatusParsing(f *testing.F) {
	// Add valid status data
	f.Add(makeStatusData(1234, 'u', 0, 1))
	f.Add(makeStatusData(0, 'd', 0, 0))

	// Add edge cases
	f.Add([]byte{})
	f.Add(make([]byte, StatusFileSize-1))
	f.Add(make([]byte, StatusFileSize+1))

	// Add random data
	randomData := make([]byte, StatusFileSize)
	for i := range randomData {
		randomData[i] = byte(i)
	}
	f.Add(randomData)

	f.Fuzz(func(t *testing.T, statusData []byte) {
		tmpDir := t.TempDir()
		superviseDir := filepath.Join(tmpDir, "supervise")
		if err := os.MkdirAll(superviseDir, 0o755); err != nil {
			t.Fatal(err)
		}

		statusPath := filepath.Join(superviseDir, "status")
		if err := renameio.WriteFile(statusPath, statusData, 0o644); err != nil {
			t.Fatal(err)
		}

		client, err := NewClientRunit(tmpDir)
		if err != nil {
			t.Fatal(err)
		}

		// Test that Status doesn't panic
		ctx := context.Background()
		status, err := client.Status(ctx)

		// If successful, verify reasonable values
		if err == nil {
			if status.PID < 0 {
				t.Errorf("negative PID: %d", status.PID)
			}
			if status.Uptime < 0 {
				t.Errorf("negative uptime: %v", status.Uptime)
			}
		}
	})
}
