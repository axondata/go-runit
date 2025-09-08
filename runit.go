package runit

import (
	"io/fs"
	"time"
)

// Runit directory and file constants
const (
	// SuperviseDir is the subdirectory containing runit control files
	SuperviseDir = "supervise"

	// ControlFile is the control socket/FIFO file name
	ControlFile = "control"

	// StatusFile is the binary status file name
	StatusFile = "status"

	// StatusFileSize is the exact size of the binary status record in bytes
	// Reference: https://github.com/g-pape/runit/blob/master/src/sv.c#L53
	// char svstatus[20];
	StatusFileSize = 20

	// DefaultWatchDebounce is the default debounce time for status file watching
	DefaultWatchDebounce = 25 * time.Millisecond

	// DefaultDialTimeout is the default timeout for control socket connections
	DefaultDialTimeout = 2 * time.Second

	// DefaultWriteTimeout is the default timeout for control write operations
	DefaultWriteTimeout = 1 * time.Second

	// DefaultReadTimeout is the default timeout for status read operations
	DefaultReadTimeout = 1 * time.Second

	// DefaultBackoffMin is the minimum backoff duration for retries
	DefaultBackoffMin = 10 * time.Millisecond

	// DefaultBackoffMax is the maximum backoff duration for retries
	DefaultBackoffMax = 1 * time.Second

	// DefaultMaxAttempts is the default maximum number of retry attempts
	DefaultMaxAttempts = 10
)

// Binary paths with defaults that can be overridden
const (
	// DefaultChpstPath is the default path to the chpst binary
	DefaultChpstPath = "chpst"

	// DefaultSvlogdPath is the default path to the svlogd binary
	DefaultSvlogdPath = "svlogd"

	// DefaultRunsvdirPath is the default path to the runsvdir binary
	DefaultRunsvdirPath = "runsvdir"

	// DefaultSvPath is the default path to the sv binary (for fallback mode)
	DefaultSvPath = "sv"
)

// File modes
const (
	// DirMode is the default mode for created directories
	DirMode = 0o755

	// FileMode is the default mode for created files
	FileMode = 0o644

	// ExecMode is the default mode for executable scripts
	ExecMode = 0o755
)

// Operation represents a control operation type
type Operation int

const (
	// OpUnknown represents an unknown operation
	OpUnknown Operation = iota
	// OpUp starts the service (want up)
	OpUp
	// OpOnce starts the service once
	OpOnce
	// OpDown stops the service (want down)
	OpDown
	// OpTerm sends SIGTERM to the service
	OpTerm
	// OpInterrupt sends SIGINT to the service
	OpInterrupt
	// OpHUP sends SIGHUP to the service
	OpHUP
	// OpAlarm sends SIGALRM to the service
	OpAlarm
	// OpQuit sends SIGQUIT to the service
	OpQuit
	// OpKill sends SIGKILL to the service
	OpKill
	// OpPause sends SIGSTOP to the service
	OpPause
	// OpCont sends SIGCONT to the service
	OpCont
	// OpExit terminates the supervise process
	OpExit
	// OpStatus represents a status query operation
	OpStatus
)

// Operation string constants
const (
	opUnknownStr   = "unknown"
	opUpStr        = "up"
	opOnceStr      = "once"
	opDownStr      = "down"
	opTermStr      = "term"
	opInterruptStr = "interrupt"
	opHUPStr       = "hup"
	opAlarmStr     = "alarm"
	opQuitStr      = "quit"
	opKillStr      = "kill"
	opPauseStr     = "pause"
	opContStr      = "cont"
	opExitStr      = "exit"
	opStatusStr    = "status"
)

// String returns the string representation of an Operation
func (op Operation) String() string {
	switch op {
	case OpUp:
		return opUpStr
	case OpOnce:
		return opOnceStr
	case OpDown:
		return opDownStr
	case OpTerm:
		return opTermStr
	case OpInterrupt:
		return opInterruptStr
	case OpHUP:
		return opHUPStr
	case OpAlarm:
		return opAlarmStr
	case OpQuit:
		return opQuitStr
	case OpKill:
		return opKillStr
	case OpPause:
		return opPauseStr
	case OpCont:
		return opContStr
	case OpExit:
		return opExitStr
	case OpStatus:
		return opStatusStr
	default:
		return opUnknownStr
	}
}

// Byte returns the control byte for this operation
func (op Operation) Byte() byte {
	switch op {
	case OpUp:
		return 'u'
	case OpOnce:
		return 'o'
	case OpDown:
		return 'd'
	case OpTerm:
		return 't'
	case OpInterrupt:
		return 'i'
	case OpHUP:
		return 'h'
	case OpAlarm:
		return 'a'
	case OpQuit:
		return 'q'
	case OpKill:
		return 'k'
	case OpPause:
		return 'p'
	case OpCont:
		return 'c'
	case OpExit:
		return 'x'
	default:
		return 0
	}
}

// TAI64N constants for timestamp decoding
const (
	// TAI64Base is the TAI64 epoch offset from Unix epoch (1970-01-01 00:00:10 TAI)
	// Reference: https://github.com/g-pape/runit/blob/master/src/tai.h#L12
	// #define tai_unix(t,u) ((void) ((t)->x = 4611686018427387914ULL + (uint64) (u)))
	// This value is 2^62 + 10 seconds (TAI is 10 seconds ahead of UTC at Unix epoch)
	// Calculated as: (1 << 62) + 10
	TAI64Base = uint64(1<<62) + 10 // 4611686018427387914
)

// DefaultUmask is the default umask for created files
var DefaultUmask fs.FileMode = 0o022
