//go:build darwin

// Package unix provides platform-specific Unix constants.
package unix

import "syscall"

// ONonblock is the non-blocking I/O flag for Darwin.
const ONonblock = syscall.O_NONBLOCK
