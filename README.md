# go-runit

[![Go Reference](https://pkg.go.dev/badge/github.com/axondata/go-runit.svg)](https://pkg.go.dev/github.com/axondata/go-runit)
[![Go Report Card](https://goreportcard.com/badge/github.com/axondata/go-runit)](https://goreportcard.com/report/github.com/axondata/go-runit)
[![Coverage Status](https://coveralls.io/repos/github/axondata/go-runit/badge.svg?branch=main)](https://coveralls.io/github/axondata/go-runit?branch=main)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![GitHub release](https://img.shields.io/github/release/axondata/go-runit.svg)](https://github.com/axondata/go-runit/releases)

Go-native library for runit control without shelling out to `sv`. Speaks directly to each service's `supervise/` endpoints.

## Features

- **Native control**: Write single-byte commands directly to `supervise/control`
- **Binary status decoding**: Parse the 20-byte `supervise/status` record
- **Real-time watching**: Monitor status changes via fsnotify (no polling)
- **Concurrent operations**: Manage multiple services with worker pools
- **Zero allocations**: Optimized hot paths with stack-based operations
- **Platform support**: Linux and macOS (darwin)
- **Dev mode**: Optional unprivileged runsvdir trees for development

## Installation

```bash
go get github.com/axondata/go-runit
```

Optional build tags:
- `fsnotify` - Enable file watching (recommended)
- `devtree_cmd` - Enable dev tree helpers for spawning runsvdir
- `sv_fallback` - Enable text-based status fallback (testing only)

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/axondata/go-runit"
)

func main() {
    // Create client for a service
    client, err := runit.New("/etc/service/web")
    if err != nil {
        log.Fatal(err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    // Start the service
    if err := client.Up(ctx); err != nil {
        log.Fatal(err)
    }

    // Get status
    status, err := client.Status(ctx)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("State: %v\n", status.State)
    fmt.Printf("PID: %d\n", status.PID)
    fmt.Printf("Uptime: %s\n", status.Uptime)
}
```

## API Reference

### Client (Single Service)

```go
// Create a client
client, err := runit.New("/etc/service/myapp",
    runit.WithDialTimeout(3*time.Second),
    runit.WithMaxAttempts(5),
    runit.WithBackoff(10*time.Millisecond, 1*time.Second),
)

// Control commands
client.Up(ctx)            // Start service (send 'u')
client.Down(ctx)          // Stop service (send 'd')
client.Once(ctx)          // Run once (send 'o')
client.Term(ctx)          // Send SIGTERM (send 't')
client.Kill(ctx)          // Send SIGKILL (send 'k')
client.HUP(ctx)           // Send SIGHUP (send 'h')
client.Interrupt(ctx)     // Send SIGINT (send 'i')
client.Alarm(ctx)         // Send SIGALRM (send 'a')
client.Quit(ctx)          // Send SIGQUIT (send 'q')
client.Pause(ctx)         // Send SIGSTOP (send 'p')
client.Cont(ctx)          // Send SIGCONT (send 'c')
client.ExitSupervise(ctx) // Exit supervise (send 'x')

// Get status
status, err := client.Status(ctx)
```

### Status Structure

```go
type Status struct {
    State    State         // Current state (Running, Down, Paused, etc.)
    PID      int           // Process ID (0 if not running)
    Since    time.Time     // When the current state started
    Uptime   time.Duration // How long in current state
    Flags    Flags         // Service flags
    Raw      []byte        // Raw 20-byte status record
}

type State int
const (
    StateUnknown State = iota
    StateDown         // Service is down
    StateStarting     // Want up, not yet running
    StateRunning      // Service is running
    StatePaused       // Service is paused (SIGSTOP)
    StateStopping     // Running but want down
    StateFinishing    // Finish script executing
    StateCrashed      // Down but want up
    StateExited       // Supervise exited
)
```

### Watching Status Changes

```go
// Start watching
events, stop, err := client.Watch(context.Background())
if err != nil {
    log.Fatal(err)
}
defer stop()

// Process events
for event := range events {
    if event.Err != nil {
        log.Printf("Watch error: %v", event.Err)
        continue
    }

    log.Printf("State changed to %v (PID: %d)",
        event.Status.State, event.Status.PID)
}
```

### Manager (Multiple Services)

```go
// Create manager with worker pool
mgr := runit.NewManager(
    runit.WithConcurrency(10),
    runit.WithTimeout(5*time.Second),
)

// Bulk operations
services := []string{
    "/etc/service/web",
    "/etc/service/db",
    "/etc/service/cache",
}

// Start all services
err := mgr.Up(ctx, services...)

// Get all statuses
statuses, err := mgr.Status(ctx, services...)
for svc, status := range statuses {
    fmt.Printf("%s: %v (PID %d)\n", svc, status.State, status.PID)
}

// Stop all services
err = mgr.Down(ctx, services...)
```

### Dev Tree (Development Mode)

Build with `-tags devtree_cmd` to enable:

```go
// Create dev tree
tree, err := runit.NewDevTree("/tmp/my-runit")
if err != nil {
    log.Fatal(err)
}

// Initialize directories
err = tree.Ensure()

// Start runsvdir
err = tree.EnsureRunsvdir()

// Enable a service
err = tree.EnableService("myapp")

// Disable a service
err = tree.DisableService("myapp")
```

### Service Builder

```go
builder := runit.NewServiceBuilder("myapp", "/tmp/services")

builder.
    WithCmd([]string{"/usr/bin/myapp", "--port", "8080"}).
    WithCwd("/var/myapp").
    WithEnv("NODE_ENV", "production").
    WithChpst(func(c *runit.ChpstBuilder) {
        c.User = "myapp"
        c.LimitMem = 1024 * 1024 * 512  // 512MB
        c.LimitFiles = 1024
    }).
    WithSvlogd(func(s *runit.SvlogdBuilder) {
        s.Size = 10000000  // 10MB per file
        s.Num = 10         // Keep 10 files
    })

err := builder.Build()
```

## Control Commands Reference

| Method | Byte | Signal | Description |
|--------|------|--------|-------------|
| `Up()` | `u` | - | Start service (want up) |
| `Once()` | `o` | - | Run service once |
| `Down()` | `d` | - | Stop service (want down) |
| `Term()` | `t` | SIGTERM | Graceful termination |
| `Interrupt()` | `i` | SIGINT | Interrupt |
| `HUP()` | `h` | SIGHUP | Reload configuration |
| `Alarm()` | `a` | SIGALRM | Alarm signal |
| `Quit()` | `q` | SIGQUIT | Quit with core dump |
| `Kill()` | `k` | SIGKILL | Force kill |
| `Pause()` | `p` | SIGSTOP | Pause process |
| `Cont()` | `c` | SIGCONT | Continue process |
| `ExitSupervise()` | `x` | - | Terminate supervise |

## Status Binary Format

The 20-byte `supervise/status` record:

```
Bytes 0-7:   TAI64N seconds (big-endian uint64)
Bytes 8-11:  TAI64N nanoseconds (big-endian uint32)
Bytes 12-15: PID (big-endian uint32)
Byte 16:     Paused flag (non-zero = paused)
Byte 17:     Want flag ('u' = up, 'd' = down)
Byte 18:     Term flag (non-zero = TERM sent)
Byte 19:     Run flag (non-zero = normally up)
```

## Error Handling

The library provides typed errors:

```go
var (
    ErrNotSupervised   // supervise directory missing
    ErrControlNotReady // control socket not accepting
    ErrTimeout         // operation timed out
    ErrDecode          // status decode failed
)

// Operation errors include context
type OpError struct {
    Op   string // Operation name
    Path string // File path
    Err  error  // Underlying error
}
```

## Testing

### Unit Tests

Run the standard unit tests:

```bash
go test ./...
```

### Integration Tests

The library includes comprehensive integration tests that verify functionality against real runit services. These tests require runit tools (`runsv`, `runsvdir`) to be installed.

```bash
# Run integration tests
go test -tags=integration -v ./...

# Run a specific integration test
go test -tags=integration -v -run TestIntegrationSingleService
```

Integration tests cover:
- Service lifecycle (start, stop, restart)
- Signal handling (TERM, HUP, etc.)
- Status monitoring and state transitions
- Watch functionality with fsnotify
- Services with different exit codes
- ServiceBuilder generated services

## Performance

Benchmarks on Apple M3 Pro:

```
BenchmarkStatusDecode-12         21870621    47.61 ns/op    24 B/op    1 allocs/op
BenchmarkStatusDecodeParallel-12 82882237    14.29 ns/op    24 B/op    1 allocs/op
BenchmarkStateString-12          1000000000   0.71 ns/op     0 B/op    0 allocs/op
BenchmarkOperationString-12      1000000000   0.69 ns/op     0 B/op    0 allocs/op
```

- **Status decode**: ~48ns/op with only 1 allocation (for Raw byte copy)
- **Parallel decode**: ~14ns/op when running concurrently
- **State/Op strings**: <1ns/op with zero allocations
- **Control send**: Sub-millisecond for local sockets
- **Watch events**: Debounced at 25ms by default (configurable)

## Examples

See the `examples/` directory for complete examples:

- `examples/basic/` - Simple service control
- `examples/watch/` - Real-time status monitoring
- `examples/manager/` - Bulk service operations
- `examples/devtree/` - Development environment setup

## Requirements

- Go 1.21+
- runit or daemontools-compatible supervisor
- Linux or macOS

## License

Apache 2.0 - See [LICENSE](LICENSE) file for details.
