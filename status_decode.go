package svcmgr

import (
	"encoding/binary"
	"fmt"
	"time"
)

// State represents the current state of a runit service
type State int

const (
	// StateUnknown indicates the state could not be determined
	StateUnknown State = iota
	// StateDown indicates the service is down and wants to be down
	StateDown
	// StateStarting indicates the service wants to be up but is not running yet
	StateStarting
	// StateRunning indicates the service is running and wants to be up
	StateRunning
	// StatePaused indicates the service is paused (SIGSTOP)
	StatePaused
	// StateStopping indicates the service is running but wants to be down
	StateStopping
	// StateFinishing indicates the finish script is executing
	StateFinishing
	// StateCrashed indicates the service is down but wants to be up
	StateCrashed
	// StateExited indicates the supervise process has exited
	StateExited
)

// S6FormatVersion represents the S6 status file format version
type S6FormatVersion int

const (
	// S6FormatUnknown indicates the format could not be determined
	S6FormatUnknown S6FormatVersion = iota
	// S6FormatPre220 is the old 35-byte format (S6 < 2.20.x)
	S6FormatPre220
	// S6FormatCurrent is the new 43-byte format (S6 >= 2.20.0)
	S6FormatCurrent
)

// TAI64 constants
const (
	// TAI64Offset is the TAI64 epoch offset (2^62)
	// TAI64 stores seconds since 1970-01-01 00:00:00 TAI
	TAI64Offset = uint64(1) << 62
)

// S6 status flag bits (byte 0 of S6 status file)
const (
	S6FlagUp         = 1 << 0 // bit 0: service is up
	S6FlagNormallyUp = 1 << 1 // bit 1: service normally up
	S6FlagWantUp     = 1 << 2 // bit 2: admin wants service up
	S6FlagReady      = 1 << 3 // bit 3: service sent readiness notification
	S6FlagPaused     = 1 << 4 // bit 4: service is paused
	S6FlagFinishing  = 1 << 5 // bit 5: finish script is running
)

// State string constants
const (
	stateUnknownStr   = "unknown"
	stateDownStr      = "down"
	stateStartingStr  = "starting"
	stateRunningStr   = "running"
	statePausedStr    = "paused"
	stateStoppingStr  = "stopping"
	stateFinishingStr = "finishing"
	stateCrashedStr   = "crashed"
	stateExitedStr    = "exited"
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateDown:
		return stateDownStr
	case StateStarting:
		return stateStartingStr
	case StateRunning:
		return stateRunningStr
	case StatePaused:
		return statePausedStr
	case StateStopping:
		return stateStoppingStr
	case StateFinishing:
		return stateFinishingStr
	case StateCrashed:
		return stateCrashedStr
	case StateExited:
		return stateExitedStr
	default:
		return stateUnknownStr
	}
}

// Flags represents service configuration flags from the status file
type Flags struct {
	// WantUp indicates the service is configured to be up
	WantUp bool
	// WantDown indicates the service is configured to be down
	WantDown bool
	// NormallyUp indicates the service should be started on boot
	NormallyUp bool
}

// Status represents the decoded state of a runit service
type Status struct {
	// State is the inferred service state
	State State
	// PID is the process ID of the service (0 if not running)
	PID int
	// Since is the timestamp when the service entered its current state
	Since time.Time
	// Uptime is the duration since the service entered its current state.
	// This field provides a snapshot of the uptime at the moment of status read.
	// Note: This value becomes stale immediately after reading as time progresses.
	// It's included for convenience and compatibility with sv output format.
	// For accurate time calculations, use Since with time.Since(status.Since).
	Uptime time.Duration
	// Ready indicates if the service has signaled readiness (S6 and potentially systemd)
	// For S6: Set when the service has sent a readiness notification
	// For other systems: May indicate similar readiness states if supported
	Ready bool
	// ReadySince is the timestamp when the service became ready (if available)
	// Only populated for S6 currently, zero value for other systems
	ReadySince time.Time
	// Flags contains service configuration flags
	Flags Flags
	// Raw contains the original 20-byte status record as an array (stack allocated)
	Raw [StatusFileSize]byte
	// S6Format indicates which S6 format version was detected (only set for S6 status files)
	S6Format S6FormatVersion
}

// DecodeStatusRunit decodes a 20-byte runit status file
func DecodeStatusRunit(data []byte) (Status, error) {
	return decodeStatusRunit(data)
}

// decodeStatusRunit decodes a 20-byte runit status file
func decodeStatusRunit(data []byte) (Status, error) {
	if len(data) != RunitStatusSize {
		return Status{}, fmt.Errorf("%w: runit status file must be %d bytes, got %d", ErrDecode, RunitStatusSize, len(data))
	}

	var st Status
	copy(st.Raw[:], data)

	// Decode TAI64N timestamp
	tai64Sec := binary.BigEndian.Uint64(data[RunitTAI64Start:RunitTAI64End])
	tai64Nano := binary.BigEndian.Uint32(data[RunitNanoStart:RunitNanoEnd])
	if tai64Sec > TAI64Offset {
		unixSec := int64(tai64Sec - TAI64Offset)
		if unixSec > 0 && unixSec < 253402300800 { // Sanity check: before year 10000
			st.Since = time.Unix(unixSec, int64(tai64Nano))
			st.Uptime = time.Since(st.Since)
			// Ensure uptime is never negative (can happen with corrupted/fuzzed data)
			if st.Uptime < 0 {
				st.Uptime = 0
			}
		}
	}

	// Extract PID
	st.PID = int(binary.LittleEndian.Uint32(data[RunitPIDStart:RunitPIDEnd]))

	// Decode flags from status bytes
	pausedFlag := data[RunitPausedFlag]
	wantFlag := data[RunitWantFlag]
	termFlag := data[RunitTermFlag]
	runFlag := data[RunitRunFlag]

	st.Flags.WantUp = wantFlag == 'u'
	st.Flags.WantDown = wantFlag == 'd'
	st.Flags.NormallyUp = runFlag != 0

	// Determine the service state
	// For runit, runFlag indicates the service has a running process
	isRunning := st.PID > 0
	isPaused := pausedFlag != 0
	isFinishing := termFlag != 0

	switch {
	case !isRunning && st.Flags.WantDown:
		st.State = StateDown
	case !isRunning && st.Flags.WantUp && !isFinishing:
		st.State = StateCrashed
	case !isRunning && isFinishing:
		st.State = StateFinishing
	case isRunning && isPaused:
		st.State = StatePaused
	case isRunning && isFinishing:
		st.State = StateFinishing
	case isRunning && st.Flags.WantDown:
		st.State = StateStopping
	case isRunning && st.Flags.WantUp:
		st.State = StateRunning
	case isRunning:
		st.State = StateRunning
	default:
		st.State = StateUnknown
	}

	return st, nil
}

// DecodeStatusDaemontools decodes an 18-byte daemontools status file
func DecodeStatusDaemontools(data []byte) (Status, error) {
	return decodeStatusDaemontools(data)
}

// decodeStatusDaemontools decodes an 18-byte daemontools status file
func decodeStatusDaemontools(data []byte) (Status, error) {
	if len(data) != DaemontoolsStatusSize {
		return Status{}, fmt.Errorf("%w: daemontools status file must be %d bytes, got %d", ErrDecode, DaemontoolsStatusSize, len(data))
	}

	var st Status
	// Copy what we have (only 18 bytes for daemontools)
	copy(st.Raw[:DaemontoolsStatusSize], data)

	// Decode TAI64N timestamp
	tai64Sec := binary.BigEndian.Uint64(data[DaemontoolsTAI64Start:DaemontoolsTAI64End])
	tai64Nano := binary.BigEndian.Uint32(data[DaemontoolsNanoStart:DaemontoolsNanoEnd])
	if tai64Sec > TAI64Offset {
		unixSec := int64(tai64Sec - TAI64Offset)
		if unixSec > 0 && unixSec < 253402300800 { // Sanity check: before year 10000
			st.Since = time.Unix(unixSec, int64(tai64Nano))
			st.Uptime = time.Since(st.Since)
			// Ensure uptime is never negative (can happen with corrupted/fuzzed data)
			if st.Uptime < 0 {
				st.Uptime = 0
			}
		}
	}

	// Extract PID
	st.PID = int(binary.LittleEndian.Uint32(data[DaemontoolsPIDStart:DaemontoolsPIDEnd]))

	// Decode flags
	pausedFlag := byte(0) // daemontools doesn't have paused flag
	wantFlag := data[DaemontoolsWantFlag]
	runFlag := byte(0)
	if st.PID > 0 {
		runFlag = 1 // infer run from PID
	}
	finishFlag := byte(0) // No finish flag in this format

	st.Flags.WantUp = wantFlag == 'u'
	st.Flags.WantDown = wantFlag == 'd'

	// Determine state based on flags
	isRunning := runFlag != 0
	isPaused := pausedFlag != 0
	isFinishing := finishFlag != 0

	switch {
	case !isRunning && st.Flags.WantDown:
		st.State = StateDown
	case !isRunning && st.Flags.WantUp && !isFinishing:
		st.State = StateCrashed
	case !isRunning && isFinishing:
		st.State = StateFinishing
	case isRunning && isPaused:
		st.State = StatePaused
	case isRunning && isFinishing:
		st.State = StateFinishing
	case isRunning && st.Flags.WantDown:
		st.State = StateStopping
	case isRunning && st.Flags.WantUp:
		st.State = StateRunning
	case isRunning:
		st.State = StateRunning
	default:
		st.State = StateUnknown
	}

	return st, nil
}

// DecodeStatusS6 decodes an s6 status file (35 bytes)
func DecodeStatusS6(data []byte) (Status, error) {
	return decodeStatusS6(data)
}

// decodeStatusS6 decodes an s6 status file
// Supports two formats:
//
//	Old format < v2.20.0 (35 bytes):
//	 bytes 0-11:  TAI64N timestamp
//	 bytes 12-23: TAI64N ready timestamp
//	 bytes 24-27: reserved/zeros
//	 bytes 28-31: PID (big-endian uint32)
//	 bytes 32-34: flags/status
//	New format >= v2.20.0 (43 bytes):
//	 bytes 0-11:  tain timestamp
//	 bytes 12-23: tain readystamp
//	 bytes 24-31: PID (big-endian uint64)
//	 bytes 32-39: PGID (big-endian uint64)
//	 bytes 40-41: wstat (big-endian uint16)
//	 byte 42:     flags
func decodeStatusS6(data []byte) (Status, error) {
	var st Status

	// Only copy first 20 bytes to Raw field for compatibility
	if len(data) >= 20 {
		copy(st.Raw[:], data[:20])
	}

	switch len(data) {
	case S6StatusSizePre220:
		// S6 format < v2.20.0 S6
		st.S6Format = S6FormatPre220
		// PID is at bytes 28-31 as big-endian uint32
		st.PID = int(binary.BigEndian.Uint32(data[S6PIDStartPre220:S6PIDEndPre220]))

		// Extract timestamp (bytes 0-7, big-endian TAI64)
		tai64 := binary.BigEndian.Uint64(data[0:8])
		if tai64 > TAI64Offset {
			unixSec := int64(tai64 - TAI64Offset)
			if unixSec > 0 && unixSec < 253402300800 {
				st.Since = time.Unix(unixSec, 0)
				st.Uptime = time.Since(st.Since)
				// Ensure uptime is never negative (can happen with corrupted/fuzzed data)
				if st.Uptime < 0 {
					st.Uptime = 0
				}
			}
		}

		// Extract ready timestamp (bytes 12-19, big-endian TAI64)
		readyTai64 := binary.BigEndian.Uint64(data[12:20])
		if readyTai64 > TAI64Offset {
			readyUnixSec := int64(readyTai64 - TAI64Offset)
			if readyUnixSec > 0 && readyUnixSec < 253402300800 {
				st.ReadySince = time.Unix(readyUnixSec, 0)
			}
		}

		// Parse flags from byte 34
		flagByte := data[S6FlagsBytePre220]

		// Determine state based on PID
		if st.PID > 0 {
			st.State = StateRunning
			st.Flags.WantUp = true
		} else {
			st.State = StateDown
			st.Flags.WantDown = true
		}

		// Set other flags based on what we can determine
		st.Flags.NormallyUp = (flagByte & 0x02) != 0
		// Check ready flag (0x08) - indicates service sent readiness notification
		st.Ready = (flagByte & 0x08) != 0

	case S6StatusSizeCurrent:
		// Current S6 format (43 bytes)
		st.S6Format = S6FormatCurrent
		// PID is at bytes 24-31 as big-endian uint64
		pid := binary.BigEndian.Uint64(data[S6PIDStartCurrent:S6PIDEndCurrent])
		st.PID = int(pid)

		// Extract timestamp (bytes 0-7, big-endian TAI64)
		tai64 := binary.BigEndian.Uint64(data[0:8])
		if tai64 > TAI64Offset {
			unixSec := int64(tai64 - TAI64Offset)
			if unixSec > 0 && unixSec < 253402300800 {
				st.Since = time.Unix(unixSec, 0)
				st.Uptime = time.Since(st.Since)
				// Ensure uptime is never negative (can happen with corrupted/fuzzed data)
				if st.Uptime < 0 {
					st.Uptime = 0
				}
			}
		}

		// Extract ready timestamp (bytes 12-19, big-endian TAI64)
		readyTai64 := binary.BigEndian.Uint64(data[12:20])
		if readyTai64 > TAI64Offset {
			readyUnixSec := int64(readyTai64 - TAI64Offset)
			if readyUnixSec > 0 && readyUnixSec < 253402300800 {
				st.ReadySince = time.Unix(readyUnixSec, 0)
			}
		}

		// Parse flags from byte 42
		flagByte := data[S6FlagsByteCurrent]
		isPaused := (flagByte & 0x01) != 0
		isFinishing := (flagByte & 0x02) != 0
		wantUp := (flagByte & 0x04) != 0
		isReady := (flagByte & 0x08) != 0

		st.Flags.WantUp = wantUp
		st.Flags.WantDown = !wantUp
		st.Flags.NormallyUp = wantUp
		// Set ready flag - indicates service sent readiness notification
		st.Ready = isReady

		// Determine state
		if st.PID > 0 {
			switch {
			case isPaused:
				st.State = StatePaused
			case isFinishing:
				st.State = StateFinishing
			default:
				st.State = StateRunning
			}
		} else {
			if wantUp {
				st.State = StateCrashed
			} else {
				st.State = StateDown
			}
		}

	default:
		return Status{}, fmt.Errorf("%w: s6 status file must be 35 or 43 bytes, got %d", ErrDecode, len(data))
	}

	return st, nil
}
