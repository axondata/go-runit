package runit

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestDecodeStatus(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantState State
		wantPID   int
		wantErr   bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "wrong size",
			data:    make([]byte, 19),
			wantErr: true,
		},
		{
			name:      "service down want down",
			data:      makeStatusData(0, 'd', 0, 0),
			wantState: StateDown,
			wantPID:   0,
		},
		{
			name:      "service down want up",
			data:      makeStatusData(0, 'u', 0, 0),
			wantState: StateCrashed,
			wantPID:   0,
		},
		{
			name:      "service running",
			data:      makeStatusData(1234, 'u', 0, 1),
			wantState: StateRunning,
			wantPID:   1234,
		},
		{
			name:      "service paused",
			data:      makeStatusData(1234, 'u', 1, 1),
			wantState: StatePaused,
			wantPID:   1234,
		},
		{
			name:      "service finishing",
			data:      makeStatusData(1234, 'u', 0, 1, withTermFlag()),
			wantState: StateFinishing,
			wantPID:   1234,
		},
		{
			name:      "service stopping",
			data:      makeStatusData(1234, 'd', 0, 1),
			wantState: StateStopping,
			wantPID:   1234,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := decodeStatus(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if status.State != tt.wantState {
				t.Errorf("State = %v, want %v", status.State, tt.wantState)
			}
			if status.PID != tt.wantPID {
				t.Errorf("PID = %v, want %v", status.PID, tt.wantPID)
			}
		})
	}
}

type statusOption func(*bytes.Buffer)

func withTermFlag() statusOption {
	return func(b *bytes.Buffer) {
		data := b.Bytes()
		data[18] = 1
	}
}

func makeStatusData(pid int, want byte, paused byte, running byte, opts ...statusOption) []byte {
	buf := &bytes.Buffer{}

	const tai64nOffset = 4611686018427387904
	now := time.Now()
	tai64nSec := uint64(now.Unix()) + tai64nOffset
	tai64nNano := uint32(now.Nanosecond())

	_ = binary.Write(buf, binary.BigEndian, tai64nSec)
	_ = binary.Write(buf, binary.BigEndian, tai64nNano)
	_ = binary.Write(buf, binary.BigEndian, uint32(pid))

	buf.WriteByte(paused)
	buf.WriteByte(want)
	buf.WriteByte(0)
	buf.WriteByte(running)

	for _, opt := range opts {
		opt(buf)
	}

	return buf.Bytes()
}

func BenchmarkDecodeStatus(b *testing.B) {
	data := makeStatusData(1234, 'u', 0, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decodeStatus(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateUnknown, "unknown"},
		{StateDown, "down"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StatePaused, "paused"},
		{StateStopping, "stopping"},
		{StateFinishing, "finishing"},
		{StateCrashed, "crashed"},
		{StateExited, "exited"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %v, want %v", tt.state, got, tt.want)
		}
	}
}
