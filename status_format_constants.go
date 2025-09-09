package runit

// Status file format constants for each supervision system

// Runit status file format (20 bytes)
const (
	RunitStatusSize = 20

	// Byte positions
	RunitTAI64Start = 0 // TAI64 seconds (8 bytes, big-endian)
	RunitTAI64End   = 8
	RunitNanoStart  = 8 // Nanoseconds (4 bytes, big-endian)
	RunitNanoEnd    = 12
	RunitPIDStart   = 12 // PID (4 bytes, little-endian)
	RunitPIDEnd     = 16
	RunitPausedFlag = 16 // Paused flag
	RunitWantFlag   = 17 // Want flag ('u' or 'd')
	RunitTermFlag   = 18 // Term flag
	RunitRunFlag    = 19 // Run flag (service has process)
)

// Daemontools status file format (18 bytes)
// Based on actual observation: uses TAI64N (12 bytes) + PID (4 bytes) + flags (2 bytes)
const (
	DaemontoolsStatusSize = 18

	// Byte positions
	DaemontoolsTAI64Start = 0 // TAI64 seconds (8 bytes, big-endian)
	DaemontoolsTAI64End   = 8
	DaemontoolsNanoStart  = 8 // Nanoseconds (4 bytes, big-endian)
	DaemontoolsNanoEnd    = 12
	DaemontoolsPIDStart   = 12 // PID (4 bytes, little-endian)
	DaemontoolsPIDEnd     = 16
	DaemontoolsStatusFlag = 16 // Status/reserved byte
	DaemontoolsWantFlag   = 17 // Want flag ('u' or 'd')
)

// S6 status file formats
// S6 has two incompatible formats:
// - Old format (35 bytes): Older S6 versions (exact cutoff unclear, but suspect < 2.20.0 as 2.12.0.x uses this)
// - New format (43 bytes): Newer S6 versions (current upstream uses this, >= 2.20.0)
const (
	S6StatusSizePre220  = 35 // Pre-2.20.0 format size
	S6StatusSizeCurrent = 43 // Current format size (>= 2.20.0)
	
	// S6MaxStatusSize is the maximum size of any S6 status format.
	// We use this when allocating buffers to ensure we can read any S6 status file version.
	// This allows us to read the actual file size and then determine which format to use for decoding.
	S6MaxStatusSize = S6StatusSizeCurrent

	// Pre-2.20.0 S6 format (35 bytes)
	// bytes 0-11:  TAI64N timestamp
	// bytes 12-23: TAI64N ready timestamp
	// bytes 24-27: reserved/zeros
	// bytes 28-31: PID (big-endian uint32)
	// bytes 32-34: flags/status
	S6TimestampStartPre220 = 0  // TAI64N timestamp start
	S6TimestampEndPre220   = 12 // TAI64N timestamp end
	S6ReadyStartPre220     = 12 // TAI64N ready timestamp start
	S6ReadyEndPre220       = 24 // TAI64N ready timestamp end
	S6PIDStartPre220       = 28 // PID start (big-endian uint32)
	S6PIDEndPre220         = 32 // PID end
	S6FlagsBytePre220      = 34 // Flags byte

	// Current S6 format (43 bytes, S6 >= 2.20.0)
	// bytes 0-11:  tain timestamp
	// bytes 12-23: tain readystamp
	// bytes 24-31: PID (big-endian uint64)
	// bytes 32-39: PGID (big-endian uint64)
	// bytes 40-41: wstat (big-endian uint16)
	// byte 42:     flags
	S6TimestampStartCurrent = 0  // tain timestamp start
	S6TimestampEndCurrent   = 12 // tain timestamp end
	S6ReadyStartCurrent     = 12 // tain readystamp start
	S6ReadyEndCurrent       = 24 // tain readystamp end
	S6PIDStartCurrent       = 24 // PID start (big-endian uint64)
	S6PIDEndCurrent         = 32 // PID end
	S6PGIDStartCurrent      = 32 // PGID start (big-endian uint64)
	S6PGIDEndCurrent        = 40 // PGID end
	S6WstatStartCurrent     = 40 // wstat start (big-endian uint16)
	S6WstatEndCurrent       = 42 // wstat end
	S6FlagsByteCurrent      = 42 // Flags byte
)

// S6 flag bits are already defined in status_decode.go
