package runit

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
	// Flags contains service configuration flags
	Flags Flags
	// Raw contains the original 20-byte status record as an array (stack allocated)
	Raw [StatusFileSize]byte
}

// decodeStatus decodes a 20-byte runit/daemontools status record.
// The format is:
//
//	bytes 0-7:   TAI64N seconds (big-endian uint64)
//	bytes 8-11:  TAI64N nanoseconds (big-endian uint32)
//	bytes 12-15: PID (big-endian uint32)
//	byte 16:     paused flag
//	byte 17:     want flag ('u' for up, 'd' for down)
//	byte 18:     term flag (finish script running)
//	byte 19:     run flag (normally up)
func decodeStatus(data []byte) (Status, error) {
	if len(data) != StatusFileSize {
		return Status{}, fmt.Errorf("%w: expected %d bytes, got %d", ErrDecode, StatusFileSize, len(data))
	}

	var st Status
	copy(st.Raw[:], data)

	// Extract binary fields from the status file
	extractStatusFields(&st, data)

	// Decode the timestamp
	decodeTimestamp(&st, data)

	// Decode flags from status bytes
	decodeFlags(&st, data)

	// Determine the service state
	st.State = determineState(st.PID, st.Flags, data)

	return st, nil
}

// Status file layout offsets (from runit source)
const (
	offsetTAI64Sec  = 0  // bytes 0-7: TAI64N seconds
	offsetTAI64Nano = 8  // bytes 8-11: TAI64N nanoseconds
	offsetPID       = 12 // bytes 12-15: PID
	offsetPaused    = 16 // byte 16: paused flag
	offsetWant      = 17 // byte 17: want flag
	offsetTerm      = 18 // byte 18: term flag
	offsetRun       = 19 // byte 19: run flag
)

// extractStatusFields extracts PID from the status data
func extractStatusFields(st *Status, data []byte) {
	pid := binary.BigEndian.Uint32(data[offsetPID:offsetPaused])
	st.PID = int(pid)
}

// decodeTimestamp decodes the TAI64N timestamp
func decodeTimestamp(st *Status, data []byte) {
	tai64nSec := binary.BigEndian.Uint64(data[offsetTAI64Sec:offsetTAI64Nano])
	tai64nNano := binary.BigEndian.Uint32(data[offsetTAI64Nano:offsetPID])

	// TAI64N stores seconds since 1970-01-01 00:00:00 TAI as a 64-bit value
	// with an offset of 2^62. TAI is 10 seconds ahead of UTC at Unix epoch.
	if tai64nSec > 0 {
		// The TAI64N epoch is 2^62 + 10 seconds from Unix epoch
		// This is equivalent to 4611686018427387914 (TAI64Base constant)
		unixSec := int64(tai64nSec - TAI64Base)

		if unixSec > 0 && unixSec < 253402300800 { // Sanity check: before year 10000
			st.Since = time.Unix(unixSec, int64(tai64nNano))
			// Calculate uptime as a snapshot at the time of reading
			uptime := time.Since(st.Since)
			// Ensure uptime is non-negative (guard against future timestamps or clock skew)
			if uptime >= 0 {
				st.Uptime = uptime
			}
		}
	}
}

// decodeFlags decodes the service flags
func decodeFlags(st *Status, data []byte) {
	wantFlag := data[offsetWant]
	runFlag := data[offsetRun]

	st.Flags.WantUp = wantFlag == 'u'
	st.Flags.WantDown = wantFlag == 'd'
	st.Flags.NormallyUp = runFlag != 0
}

// determineState determines the service state based on flags and PID
func determineState(pid int, flags Flags, data []byte) State {
	isRunning := pid > 0
	isPaused := data[offsetPaused] != 0
	isFinishing := data[offsetTerm] != 0

	switch {
	case !isRunning && flags.WantDown:
		return StateDown
	case !isRunning && flags.WantUp && !isFinishing:
		return StateCrashed
	case !isRunning && isFinishing:
		// Finish script is running (main process has exited)
		return StateFinishing
	case isRunning && isPaused:
		return StatePaused
	case isRunning && isFinishing:
		// This shouldn't normally happen, but handle it
		return StateFinishing
	case isRunning && flags.WantDown:
		return StateStopping
	case isRunning && flags.WantUp:
		return StateRunning
	case isRunning:
		return StateRunning
	default:
		return StateUnknown
	}
}
