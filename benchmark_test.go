package svcmgr

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// BenchmarkStatusDecode measures the performance of decoding status files
func BenchmarkStatusDecode(b *testing.B) {
	// Create a sample status buffer
	var buf bytes.Buffer
	tai64nSec := uint64(time.Now().Unix()) + TAI64Base
	tai64nNano := uint32(time.Now().Nanosecond())
	pid := uint32(1234)

	_ = binary.Write(&buf, binary.BigEndian, tai64nSec)
	_ = binary.Write(&buf, binary.BigEndian, tai64nNano)
	_ = binary.Write(&buf, binary.BigEndian, pid)
	buf.WriteByte(0)   // paused
	buf.WriteByte('u') // want up
	buf.WriteByte(0)   // term
	buf.WriteByte(1)   // run

	data := buf.Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := decodeStatusRunit(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStatusDecodeParallel measures parallel decode performance
func BenchmarkStatusDecodeParallel(b *testing.B) {
	var buf bytes.Buffer
	tai64nSec := uint64(time.Now().Unix()) + TAI64Base
	tai64nNano := uint32(time.Now().Nanosecond())
	pid := uint32(1234)

	_ = binary.Write(&buf, binary.BigEndian, tai64nSec)
	_ = binary.Write(&buf, binary.BigEndian, tai64nNano)
	_ = binary.Write(&buf, binary.BigEndian, pid)
	buf.WriteByte(0)   // paused
	buf.WriteByte('u') // want up
	buf.WriteByte(0)   // term
	buf.WriteByte(1)   // run

	data := buf.Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := decodeStatusRunit(data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkStateString measures State.String() performance
func BenchmarkStateString(b *testing.B) {
	states := []State{
		StateUnknown,
		StateDown,
		StateStarting,
		StateRunning,
		StatePaused,
		StateStopping,
		StateFinishing,
		StateCrashed,
		StateExited,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = states[i%len(states)].String()
	}
}

// BenchmarkOperationString measures Operation.String() performance
func BenchmarkOperationString(b *testing.B) {
	ops := []Operation{
		OpUp,
		OpDown,
		OpTerm,
		OpKill,
		OpPause,
		OpCont,
		OpStatus,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ops[i%len(ops)].String()
	}
}

// BenchmarkOperationByte measures Operation.Byte() performance
func BenchmarkOperationByte(b *testing.B) {
	ops := []Operation{
		OpUp,
		OpDown,
		OpTerm,
		OpKill,
		OpPause,
		OpCont,
		OpExit,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ops[i%len(ops)].Byte()
	}
}
