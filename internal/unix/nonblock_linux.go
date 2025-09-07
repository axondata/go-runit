//go:build linux

// Package unix provides platform-specific Unix constants.
package unix

import "syscall"

// ONonblock is the non-blocking I/O flag for Linux.
const ONonblock = syscall.O_NONBLOCK
