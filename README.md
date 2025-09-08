# `go-runit`

[![Go Reference](https://pkg.go.dev/badge/github.com/axondata/go-runit.svg)](https://pkg.go.dev/github.com/axondata/go-runit)
[![Go Report Card](https://goreportcard.com/badge/github.com/axondata/go-runit)](https://goreportcard.com/report/github.com/axondata/go-runit)
[![Coverage Status](https://coveralls.io/repos/github/axondata/go-runit/badge.svg?branch=main)](https://coveralls.io/github/axondata/go-runit?branch=main)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![GitHub release](https://img.shields.io/github/release/axondata/go-runit.svg)](https://github.com/axondata/go-runit/releases)

A Go-native library to control [`runit`](https://github.com/g-pape/runit/), [`s6`](https://github.com/skarnet/s6), or any [`daemontools`](https://cr.yp.to/daemontools.html)-compatible process supervisor.

## Features

- **Native control**: Write single-byte commands directly to `supervise/control`
- **Binary status decoding**: Parses `supervise/status` record
- **Real-time watching**: Monitor status changes via fsnotify (no polling)
- **Concurrent operations**: Manage multiple services with worker pools via [`Manager`](https://pkg.go.dev/github.com/axondata/go-runit#Manager)
- **Zero allocations**: Optimized hot paths with stack-based operations
- **Platform support**: Linux and macOS (darwin)
- **Dev mode**: Optional unprivileged `runsvdir` trees for development
- **Compatibility**: Works with [`runit`](https://github.com/g-pape/runit/), [`s6`](https://github.com/skarnet/s6), or any [`daemontools`](https://cr.yp.to/daemontools.html)-compatible process supervision system

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
    "sync"
    "time"

    "github.com/axondata/go-runit"
)

func main() {
    // Create client for a service
    client, err := runit.New("/etc/service/web")
    if err != nil {
        log.Fatal(err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    var wg sync.WaitGroup

    // Start watching in a goroutine
    wg.Add(1)
    go func() {
        defer wg.Done()

        events, stop, err := client.Watch(ctx)
        if err != nil {
            log.Printf("Watch error: %v", err)
            return
        }
        defer stop()

        for event := range events {
            if event.Err != nil {
                log.Printf("Event error: %v", event.Err)
                continue
            }
            log.Printf("State changed: %v (PID: %d)",
                event.Status.State, event.Status.PID)
        }
    }()

    // Control the service in another goroutine
    wg.Add(1)
    go func() {
        defer wg.Done()

        // Give watcher time to start
        time.Sleep(100 * time.Millisecond)

        // Stop the service
        log.Println("Stopping service...")
        if err := client.Down(ctx); err != nil {
            log.Printf("Down error: %v", err)
            return
        }

        time.Sleep(2 * time.Second)

        // Start the service
        log.Println("Starting service...")
        if err := client.Up(ctx); err != nil {
            log.Printf("Up error: %v", err)
            return
        }

        time.Sleep(2 * time.Second)

        // Get final status
        status, err := client.Status(ctx)
        if err != nil {
            log.Printf("Status error: %v", err)
            return
        }

        fmt.Printf("Final state: %v, PID: %d, Uptime: %s\n",
            status.State, status.PID, status.Uptime)

        // Cancel context to stop watcher
        cancel()
    }()

    wg.Wait()
}
```

## API Reference

### [`Client`](https://pkg.go.dev/github.com/axondata/go-runit#Client) (Single Service)

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

See [`Status`](https://pkg.go.dev/github.com/axondata/go-runit#Status) and [`State`](https://pkg.go.dev/github.com/axondata/go-runit#State) types in the API documentation.

### [`Manager`](https://pkg.go.dev/github.com/axondata/go-runit#Manager) (Multiple Services)

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

### [`DevTree`](https://pkg.go.dev/github.com/axondata/go-runit#DevTree) (Development Mode)

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

### [`ServiceBuilder`](https://pkg.go.dev/github.com/axondata/go-runit#ServiceBuilder)

```go
builder := runit.NewServiceBuilder("myapp", "/tmp/services")

builder.
    WithCmd([]string{"/usr/bin/myapp", "--port", "8080"}).
    WithCwd("/var/myapp").
    WithEnv("NODE_ENV", "production").
    WithChpst(func(c *runit.ChpstBuilder) { // See https://pkg.go.dev/github.com/axondata/go-runit#ChpstBuilder
        c.User = "myapp"
        c.LimitMem = 1024 * 1024 * 512  // 512MB
        c.LimitFiles = 1024
    }).
    WithSvlogd(func(s *runit.SvlogdBuilder) { // See https://pkg.go.dev/github.com/axondata/go-runit#SvlogdBuilder
        s.Size = 10000000  // 10MB per file
        s.Num = 10         // Keep 10 files
    })

err := builder.Build()
```

## Compatibility with daemontools and s6

This library works with any daemontools-compatible supervision system, including:
- **runit** - Full support for all operations
- **daemontools** - Compatible except for `Once()` and `Quit()` operations
- **s6** - Full compatibility with all operations

The library provides factory functions for each system (see [compatibility functions](https://pkg.go.dev/github.com/axondata/go-runit#RunitConfig)):

```go
// For runit
config := runit.RunitConfig()
client, err := runit.NewClientWithConfig("/etc/service/myapp", config)

// For daemontools
config := runit.DaemontoolsConfig()
client, err := runit.NewClientWithConfig("/service/myapp", config)

// For s6
config := runit.S6Config()
client, err := runit.NewClientWithConfig("/run/service/myapp", config)

// Service builders for each system
runitBuilder := runit.ServiceBuilderRunit("myapp", "/etc/service")        // See https://pkg.go.dev/github.com/axondata/go-runit#ServiceBuilderRunit
dtBuilder := runit.ServiceBuilderDaemontools("myapp", "/service")         // See https://pkg.go.dev/github.com/axondata/go-runit#ServiceBuilderDaemontools
s6Builder := runit.ServiceBuilderS6("myapp", "/run/service")              // See https://pkg.go.dev/github.com/axondata/go-runit#ServiceBuilderS6
```

### Differences between systems

| Feature | runit | daemontools | s6 |
|---------|-------|-------------|-----|
| Default path | `/etc/service` | `/service` | `/run/service` |
| Privilege tool | `chpst` | `setuidgid` | `s6-setuidgid` |
| Logger | `svlogd` | `multilog` | `s6-log` |
| Scanner | `runsvdir` | `svscan` | `s6-svscan` |
| `Once()` support | ✓ | ✗ | ✓ |
| `Quit()` support | ✓ | ✗ | ✓ |

All three systems use the same binary protocol for `supervise/control` and `supervise/status`, making this library compatible with all of them.

## Control Commands Reference

| Method | Byte | Signal | Description | runit | daemontools | s6 |
|--------|------|--------|-------------|-------|-------------|-----|
| `Up()` | `u` | - | Start service (want up) | ✓ | ✓ | ✓ |
| `Once()` | `o` | - | Run service once | ✓ | ✗ | ✓ |
| `Down()` | `d` | - | Stop service (want down) | ✓ | ✓ | ✓ |
| `Term()` | `t` | SIGTERM | Graceful termination | ✓ | ✓ | ✓ |
| `Interrupt()` | `i` | SIGINT | Interrupt | ✓ | ✓ | ✓ |
| `HUP()` | `h` | SIGHUP | Reload configuration | ✓ | ✓ | ✓ |
| `Alarm()` | `a` | SIGALRM | Alarm signal | ✓ | ✓ | ✓ |
| `Quit()` | `q` | SIGQUIT | Quit with core dump | ✓ | ✗ | ✓ |
| `Kill()` | `k` | SIGKILL | Force kill | ✓ | ✓ | ✓ |
| `Pause()` | `p` | SIGSTOP | Pause process | ✓ | ✓ | ✓ |
| `Cont()` | `c` | SIGCONT | Continue process | ✓ | ✓ | ✓ |
| `ExitSupervise()` | `x` | - | Terminate supervise | ✓ | ✓ | ✓ |

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

The library provides typed errors. See [`OpError`](https://pkg.go.dev/github.com/axondata/go-runit#OpError) and the error variables in the [API documentation](https://pkg.go.dev/github.com/axondata/go-runit#pkg-variables).

## Testing

### Unit Tests

Run the standard unit tests:

```bash
go test ./...
```

### Integration Tests

The library includes integration tests for different supervision systems. Each requires the respective tools to be installed.

#### Runit Integration Tests

Tests for runit require `runsv` and `runsvdir` to be installed:

```bash
# Run all runit integration tests
go test -tags=integration -v ./...

# Or explicitly for runit
go test -tags=integration_runit -v ./...

# Run a specific integration test
go test -tags=integration -v -run TestIntegrationSingleService
```

#### Daemontools Integration Tests

Tests for daemontools require `svscan` and `supervise` to be installed:

```bash
# Run daemontools integration tests
go test -tags=integration_daemontools -v ./...
```

#### S6 Integration Tests

Tests for s6 require `s6-svscan` and `s6-supervise` to be installed:

```bash
# Run s6 integration tests
go test -tags=integration_s6 -v ./...
```

The runit integration tests cover:
- Service lifecycle (start, stop, restart)
- Signal handling (TERM, HUP, etc.)
- Status monitoring and state transitions
- Watch functionality with fsnotify
- Services with different exit codes
- ServiceBuilder generated services

## Performance

Benchmarks on Apple M3 Pro (2025-09-08):

```
BenchmarkStatusDecode-12          32006547	 37.64 ns/op   0 B/op  0 allocs/op
BenchmarkStatusDecodeParallel-12  187062128	  9.825 ns/op  0 B/op  0 allocs/op
BenchmarkDecodeStatus-12          31235832	 37.89 ns/op   0 B/op  0 allocs/op
```

- **Status decode**: ~38ns/op with zero allocations
- **Parallel decode**: ~10ns/op when running concurrently
- **State/Op strings**: <1ns/op with zero allocations
- **Control send**: Sub-millisecond for local sockets
- **Watch events**: Debounced at 25ms by default (configurable)

## Examples

See the `examples/` directory for complete examples:

- `examples/basic/` - Simple service control
- `examples/watch/` - Real-time status monitoring
- `examples/manager/` - Bulk service operations
- `examples/compat/` - Using with daemontools and s6
- `examples/devtree/` - Development environment setup

## Requirements

- Go 1.21+
- [`runit`](https://github.com/g-pape/runit/), [`s6`](https://github.com/skarnet/s6), or any [`daemontools`](https://cr.yp.to/daemontools.html)-compatible process supervisor.
- Linux or macOS

## License

Apache 2.0 - See [LICENSE](LICENSE) file for details.
