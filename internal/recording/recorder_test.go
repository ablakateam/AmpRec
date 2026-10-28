package recording

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"amprec/internal/config"
)

func TestRecorderRejectsFileAsOutputFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := NewRecorder()
	err := recorder.Start(StartOptions{PID: 1, Format: config.FormatWAV, OutputFolder: path})
	if err == nil {
		t.Fatal("expected an output-folder validation error")
	}
}

func TestUniqueOutputPathsKeepEncoderExtension(t *testing.T) {
	finalPath, temporaryPath := uniqueOutputPaths(t.TempDir(), config.FormatMP3, time.Date(2026, 10, 26, 12, 34, 56, 0, time.UTC))
	if filepath.Ext(finalPath) != ".mp3" || filepath.Ext(temporaryPath) != ".mp3" {
		t.Fatalf("unexpected extensions: %q %q", finalPath, temporaryPath)
	}
	if !strings.Contains(temporaryPath, ".part.mp3") {
		t.Fatalf("temporary path does not preserve MP3 extension: %q", temporaryPath)
	}
}

func TestPCMPeak(t *testing.T) {
	packet := []byte{0, 0, 0xff, 0x7f, 0x00, 0x80, 0, 0}
	level := pcmPeak(packet)
	if level < 0.999 || level > 1 {
		t.Fatalf("peak = %f", level)
	}
}

func TestApplyPCM16Gain(t *testing.T) {
	packet := []byte{0x10, 0x27, 0xf0, 0xd8, 0xff, 0x7f, 0x00, 0x80}
	applyPCM16Gain(packet, 2)
	values := []int16{
		int16(binary.LittleEndian.Uint16(packet[0:2])),
		int16(binary.LittleEndian.Uint16(packet[2:4])),
		int16(binary.LittleEndian.Uint16(packet[4:6])),
		int16(binary.LittleEndian.Uint16(packet[6:8])),
	}
	want := []int16{20000, -20000, 32767, -32768}
	for index := range want {
		if values[index] != want[index] {
			t.Fatalf("sample %d = %d, want %d", index, values[index], want[index])
		}
	}
}
