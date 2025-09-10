# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-09-07

### Added
- Initial stable release
- Native runit service control without shelling out to `sv`
- Binary status file decoding (20-byte supervise/status format)
- Full control operations (Up, Down, Term, Kill, HUP, etc.)
- Real-time status monitoring with fsnotify
- Concurrent service manager for bulk operations
- ServiceBuilder for creating service directories
- ChpstBuilder for process control configuration
- SvlogdBuilder for logging setup
- DevTree for unprivileged development environments
- Comprehensive unit and integration tests
- Performance benchmarks
- Support for Linux and macOS (darwin)
- Context-aware operations with timeouts
- Exponential backoff for control operations
- Type-safe operation enums
- Full godoc documentation
- Apache 2.0 license

### Technical Details
- Status decode performance: ~38ns/op with zero allocations
- Parallel decode: ~10ns/op with zero allocations
- State/Operation strings: <1ns/op with zero allocations
- TAI64N timestamp decoding based on official runit source
- Compatible with daemontools/runit specification

[1.0.0]: https://github.com/axondata/go-svcmgr/releases/tag/v1.0.0
