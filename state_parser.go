package svcmgr

import (
	"encoding/binary"
	"fmt"
	"time"
)

// StateParser defines the interface for parsing supervision system status files
type StateParser interface {
	// Parse parses the raw status file data into a Status struct
	Parse(data []byte) (Status, error)
	// ValidateSize checks if the data size is valid for this parser
	ValidateSize(size int) bool
	// Name returns the name of this parser for debugging
	Name() string
}

// RunitStateParser parses Runit status files (20 bytes)
type RunitStateParser struct{}

func (p *RunitStateParser) Name() string {
	return "runit"
}

func (p *RunitStateParser) ValidateSize(size int) bool {
	return size == RunitStatusSize
}

func (p *RunitStateParser) Parse(data []byte) (Status, error) {
	if len(data) != RunitStatusSize {
		return Status{}, fmt.Errorf("invalid runit status size: %d bytes (expected %d)", len(data), RunitStatusSize)
	}

	st := Status{}

	// Extract timestamp (bytes 0-7, big-endian TAI64)
	tai64 := binary.BigEndian.Uint64(data[RunitTAI64Start:RunitTAI64End])
	if tai64 > TAI64Offset {
		unixSec := int64(tai64 - TAI64Offset)
		if unixSec > 0 && unixSec < 253402300800 { // Year 9999
			st.Since = time.Unix(unixSec, 0)
			st.Uptime = time.Since(st.Since)
			if st.Uptime < 0 {
				st.Uptime = 0
			}
		}
	}

	// Extract PID (bytes 12-15, little-endian)
	st.PID = int(binary.LittleEndian.Uint32(data[RunitPIDStart:RunitPIDEnd]))

	// Parse flags
	isPaused := data[RunitPausedFlag] != 0
	wantChar := data[RunitWantFlag]
	hasProcess := data[RunitRunFlag] != 0
	isTerminating := data[RunitTermFlag] != 0

	// Set flags
	st.Flags.WantUp = wantChar == 'u'
	st.Flags.WantDown = wantChar == 'd'
	st.Flags.NormallyUp = st.Flags.WantUp

	// Determine state
	switch {
	case !hasProcess && wantChar == 'd':
		st.State = StateDown
	case !hasProcess && wantChar == 'u':
		st.State = StateStarting
	case hasProcess && isPaused:
		st.State = StatePaused
	case hasProcess && isTerminating:
		st.State = StateFinishing
	case hasProcess && wantChar == 'u':
		st.State = StateRunning
	case hasProcess && wantChar == 'd':
		st.State = StateStopping
	default:
		st.State = StateUnknown
	}

	return st, nil
}

// DaemontoolsStateParser parses Daemontools status files (18 bytes)
type DaemontoolsStateParser struct{}

func (p *DaemontoolsStateParser) Name() string {
	return "daemontools"
}

func (p *DaemontoolsStateParser) ValidateSize(size int) bool {
	return size == DaemontoolsStatusSize
}

func (p *DaemontoolsStateParser) Parse(data []byte) (Status, error) {
	if len(data) != DaemontoolsStatusSize {
		return Status{}, fmt.Errorf("invalid daemontools status size: %d bytes (expected %d)", len(data), DaemontoolsStatusSize)
	}

	st := Status{}

	// Extract timestamp (bytes 0-7, big-endian TAI64)
	tai64 := binary.BigEndian.Uint64(data[DaemontoolsTAI64Start:DaemontoolsTAI64End])
	if tai64 > TAI64Offset {
		unixSec := int64(tai64 - TAI64Offset)
		if unixSec > 0 && unixSec < 253402300800 {
			st.Since = time.Unix(unixSec, 0)
			st.Uptime = time.Since(st.Since)
			if st.Uptime < 0 {
				st.Uptime = 0
			}
		}
	}

	// Extract PID (bytes 12-15, little-endian)
	st.PID = int(binary.LittleEndian.Uint32(data[DaemontoolsPIDStart:DaemontoolsPIDEnd]))

	// Parse flags
	wantChar := data[DaemontoolsWantFlag]
	st.Flags.WantUp = wantChar == 'u'
	st.Flags.WantDown = wantChar == 'd'
	st.Flags.NormallyUp = st.Flags.WantUp

	// Determine state
	if st.PID > 0 {
		if st.Flags.WantUp {
			st.State = StateRunning
		} else {
			st.State = StateStopping
		}
	} else {
		if st.Flags.WantUp {
			st.State = StateStarting
		} else {
			st.State = StateDown
		}
	}

	return st, nil
}

// S6StateParserPre220 parses S6 status files for versions < 2.20.0 (35 bytes)
type S6StateParserPre220 struct{}

func (p *S6StateParserPre220) Name() string {
	return "s6-pre-2.20.0"
}

func (p *S6StateParserPre220) ValidateSize(size int) bool {
	return size == S6StatusSizePre220
}

func (p *S6StateParserPre220) Parse(data []byte) (Status, error) {
	if len(data) != S6StatusSizePre220 {
		return Status{}, fmt.Errorf("invalid s6 pre-2.20.0 status size: %d bytes (expected %d)", len(data), S6StatusSizePre220)
	}

	st := Status{
		S6Format: S6FormatPre220,
	}

	// Extract PID (bytes 28-31 as big-endian uint32)
	st.PID = int(binary.BigEndian.Uint32(data[S6PIDStartPre220:S6PIDEndPre220]))

	// Extract timestamp (bytes 0-7, big-endian TAI64)
	tai64 := binary.BigEndian.Uint64(data[0:8])
	if tai64 > TAI64Offset {
		unixSec := int64(tai64 - TAI64Offset)
		if unixSec > 0 && unixSec < 253402300800 {
			st.Since = time.Unix(unixSec, 0)
			st.Uptime = time.Since(st.Since)
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
	st.Flags.NormallyUp = (flagByte & 0x02) != 0
	st.Ready = (flagByte & 0x08) != 0

	// Determine state based on PID
	if st.PID > 0 {
		st.State = StateRunning
		st.Flags.WantUp = true
	} else {
		st.State = StateDown
		st.Flags.WantDown = true
	}

	return st, nil
}

// S6StateParserCurrent parses S6 status files for versions >= 2.20.0 (43 bytes)
type S6StateParserCurrent struct{}

func (p *S6StateParserCurrent) Name() string {
	return "s6-current"
}

func (p *S6StateParserCurrent) ValidateSize(size int) bool {
	return size == S6StatusSizeCurrent
}

func (p *S6StateParserCurrent) Parse(data []byte) (Status, error) {
	if len(data) != S6StatusSizeCurrent {
		return Status{}, fmt.Errorf("invalid s6 current status size: %d bytes (expected %d)", len(data), S6StatusSizeCurrent)
	}

	st := Status{
		S6Format: S6FormatCurrent,
	}

	// Extract PID (bytes 24-31 as big-endian uint64)
	pid := binary.BigEndian.Uint64(data[S6PIDStartCurrent:S6PIDEndCurrent])
	st.PID = int(pid)

	// Extract timestamp (bytes 0-7, big-endian TAI64)
	tai64 := binary.BigEndian.Uint64(data[0:8])
	if tai64 > TAI64Offset {
		unixSec := int64(tai64 - TAI64Offset)
		if unixSec > 0 && unixSec < 253402300800 {
			st.Since = time.Unix(unixSec, 0)
			st.Uptime = time.Since(st.Since)
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
			st.State = StateStarting
		} else {
			st.State = StateDown
		}
	}

	return st, nil
}

// GetStateParser returns the appropriate parser for the given service type and data size
func GetStateParser(serviceType ServiceType, dataSize int) (StateParser, error) {
	switch serviceType {
	case ServiceTypeRunit:
		parser := &RunitStateParser{}
		if !parser.ValidateSize(dataSize) {
			return nil, fmt.Errorf("invalid runit status size: %d", dataSize)
		}
		return parser, nil

	case ServiceTypeDaemontools:
		parser := &DaemontoolsStateParser{}
		if !parser.ValidateSize(dataSize) {
			return nil, fmt.Errorf("invalid daemontools status size: %d", dataSize)
		}
		return parser, nil

	case ServiceTypeS6:
		// Choose parser based on data size
		if dataSize == S6StatusSizePre220 {
			return &S6StateParserPre220{}, nil
		} else if dataSize == S6StatusSizeCurrent {
			return &S6StateParserCurrent{}, nil
		}
		return nil, fmt.Errorf("invalid s6 status size: %d (expected %d or %d)",
			dataSize, S6StatusSizePre220, S6StatusSizeCurrent)

	default:
		return nil, fmt.Errorf("unknown service type: %v", serviceType)
	}
}
