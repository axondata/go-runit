package runit

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClientNew(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("missing supervise dir", func(t *testing.T) {
		_, err := New(tmpDir)
		if err == nil {
			t.Fatal("expected error for missing supervise dir")
		}
	})

	t.Run("with supervise dir", func(t *testing.T) {
		superviseDir := filepath.Join(tmpDir, "test-service", "supervise")
		if err := os.MkdirAll(superviseDir, 0o755); err != nil {
			t.Fatal(err)
		}

		client, err := New(filepath.Join(tmpDir, "test-service"))
		if err != nil {
			t.Fatal(err)
		}

		if client.ServiceDir != filepath.Join(tmpDir, "test-service") {
			t.Errorf("ServiceDir = %v, want %v", client.ServiceDir, filepath.Join(tmpDir, "test-service"))
		}
	})
}

func TestClientOptions(t *testing.T) {
	tmpDir := t.TempDir()
	superviseDir := filepath.Join(tmpDir, "supervise")
	if err := os.MkdirAll(superviseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	client, err := New(tmpDir,
		WithDialTimeout(3*time.Second),
		WithWriteTimeout(2*time.Second),
		WithReadTimeout(4*time.Second),
		WithBackoff(20*time.Millisecond, 2*time.Second),
		WithMaxAttempts(10),
	)
	if err != nil {
		t.Fatal(err)
	}

	if client.DialTimeout != 3*time.Second {
		t.Errorf("DialTimeout = %v, want %v", client.DialTimeout, 3*time.Second)
	}
	if client.WriteTimeout != 2*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", client.WriteTimeout, 2*time.Second)
	}
	if client.ReadTimeout != 4*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", client.ReadTimeout, 4*time.Second)
	}
	if client.BackoffMin != 20*time.Millisecond {
		t.Errorf("BackoffMin = %v, want %v", client.BackoffMin, 20*time.Millisecond)
	}
	if client.BackoffMax != 2*time.Second {
		t.Errorf("BackoffMax = %v, want %v", client.BackoffMax, 2*time.Second)
	}
	if client.MaxAttempts != 10 {
		t.Errorf("MaxAttempts = %v, want %v", client.MaxAttempts, 10)
	}
}

func TestClientSend(t *testing.T) {
	tmpDir := t.TempDir()
	superviseDir := filepath.Join(tmpDir, "supervise")
	if err := os.MkdirAll(superviseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	controlPath := filepath.Join(superviseDir, "control")

	listener, err := net.Listen("unix", controlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

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

	client, err := New(tmpDir, WithMaxAttempts(1))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := client.Up(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case cmd := <-received:
		if cmd != 'u' {
			t.Errorf("received command = %c, want u", cmd)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for command")
	}
}

func TestClientStatus(t *testing.T) {
	tmpDir := t.TempDir()
	superviseDir := filepath.Join(tmpDir, "supervise")
	if err := os.MkdirAll(superviseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	statusPath := filepath.Join(superviseDir, "status")
	statusData := makeStatusData(1234, 'u', 0, 1)
	if err := os.WriteFile(statusPath, statusData, 0o644); err != nil {
		t.Fatal(err)
	}

	client, err := New(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	status, err := client.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if status.PID != 1234 {
		t.Errorf("PID = %v, want 1234", status.PID)
	}
	if status.State != StateRunning {
		t.Errorf("State = %v, want StateRunning", status.State)
	}
}
