package recording

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type memoryFile struct {
	data   []byte
	offset int64
	closed bool
}

func (m *memoryFile) Write(p []byte) (int, error) {
	end := int(m.offset) + len(p)
	if end > len(m.data) {
		m.data = append(m.data, make([]byte, end-len(m.data))...)
	}
	copy(m.data[m.offset:], p)
	m.offset = int64(end)
	return len(p), nil
}
func (m *memoryFile) Seek(offset int64, whence int) (int64, error) {
	if whence == 0 {
		m.offset = offset
	}
	return m.offset, nil
}
func (m *memoryFile) Sync() error  { return nil }
func (m *memoryFile) Close() error { m.closed = true; return nil }

func TestWAVHeaderFinalization(t *testing.T) {
	file := &memoryFile{}
	encoder, err := newWAVEncoderForWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	pcm := bytes.Repeat([]byte{1, 2, 3, 4}, 100)
	if err := encoder.WritePCM(pcm); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	if string(file.data[:4]) != "RIFF" || string(file.data[8:12]) != "WAVE" || string(file.data[36:40]) != "data" {
		t.Fatalf("invalid WAV markers: %q", file.data[:44])
	}
	if got := binary.LittleEndian.Uint32(file.data[4:8]); got != uint32(36+len(pcm)) {
		t.Fatalf("RIFF size = %d", got)
	}
	if got := binary.LittleEndian.Uint32(file.data[40:44]); got != uint32(len(pcm)) {
		t.Fatalf("data size = %d", got)
	}
	if !file.closed {
		t.Fatal("output was not closed")
	}
}

func TestWAVRejectsUnalignedPCM(t *testing.T) {
	file := &memoryFile{}
	encoder, _ := newWAVEncoderForWriter(file)
	if err := encoder.WritePCM([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected block-alignment error")
	}
	_ = encoder.Abort()
}
