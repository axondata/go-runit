package runit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/renameio/v2"
)

func createTestService(t *testing.T, dir, name string, pid int, want byte) string {
	serviceDir := filepath.Join(dir, name)
	superviseDir := filepath.Join(serviceDir, "supervise")
	if err := os.MkdirAll(superviseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	statusPath := filepath.Join(superviseDir, "status")
	statusData := makeStatusData(pid, want, 0, byte(pid))
	if err := renameio.WriteFile(statusPath, statusData, 0o644); err != nil {
		t.Fatal(err)
	}

	return serviceDir
}

func TestManagerStatus(t *testing.T) {
	tmpDir := t.TempDir()

	svc1 := createTestService(t, tmpDir, "service1", 1001, 'u')
	svc2 := createTestService(t, tmpDir, "service2", 0, 'd')
	svc3 := createTestService(t, tmpDir, "service3", 1003, 'u')

	mgr := NewManager(
		WithConcurrency(2),
		WithTimeout(1*time.Second),
	)

	ctx := context.Background()
	statuses, err := mgr.Status(ctx, svc1, svc2, svc3)
	if err != nil {
		t.Fatal(err)
	}

	if len(statuses) != 3 {
		t.Fatalf("got %d statuses, want 3", len(statuses))
	}

	if s, ok := statuses[svc1]; !ok {
		t.Error("missing status for service1")
	} else if s.PID != 1001 {
		t.Errorf("service1 PID = %d, want 1001", s.PID)
	}

	if s, ok := statuses[svc2]; !ok {
		t.Error("missing status for service2")
	} else if s.PID != 0 {
		t.Errorf("service2 PID = %d, want 0", s.PID)
	}

	if s, ok := statuses[svc3]; !ok {
		t.Error("missing status for service3")
	} else if s.PID != 1003 {
		t.Errorf("service3 PID = %d, want 1003", s.PID)
	}
}

func TestManagerEmptyServices(t *testing.T) {
	mgr := NewManager()

	ctx := context.Background()

	statuses, err := mgr.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 0 {
		t.Errorf("got %d statuses, want 0", len(statuses))
	}

	if err := mgr.Up(ctx); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Down(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestManagerConcurrency(t *testing.T) {
	tmpDir := t.TempDir()

	var services []string
	for i := 0; i < 10; i++ {
		svc := createTestService(t, tmpDir, fmt.Sprintf("service%d", i), 1000+i, 'u')
		services = append(services, svc)
	}

	mgr := NewManager(WithConcurrency(3))

	start := time.Now()
	ctx := context.Background()
	statuses, err := mgr.Status(ctx, services...)
	if err != nil {
		t.Fatal(err)
	}
	duration := time.Since(start)

	if len(statuses) != 10 {
		t.Fatalf("got %d statuses, want 10", len(statuses))
	}

	t.Logf("Processed 10 services with concurrency 3 in %v", duration)
}

func TestMultiError(t *testing.T) {
	merr := &MultiError{}

	if err := merr.Err(); err != nil {
		t.Error("empty MultiError should return nil")
	}

	merr.Add(nil)
	if err := merr.Err(); err != nil {
		t.Error("MultiError with nil errors should return nil")
	}

	err1 := &OpError{Op: OpStatus, Path: "/path", Err: ErrTimeout}
	merr.Add(err1)

	if err := merr.Err(); err == nil {
		t.Error("MultiError with errors should return non-nil")
	}

	if merr.Error() != err1.Error() {
		t.Errorf("single error message = %v, want %v", merr.Error(), err1.Error())
	}

	err2 := &OpError{Op: OpStatus, Path: "/path2", Err: ErrDecode}
	merr.Add(err2)

	if merr.Error() != "2 errors occurred" {
		t.Errorf("multiple errors message = %v, want '2 errors occurred'", merr.Error())
	}
}
