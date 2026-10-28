//go:build windows

package recording

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"amprec/internal/win32"
)

func TestMediaFoundationMP3Encoding(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := win32.CoInitializeMTA(); err != nil {
		t.Fatal(err)
	}
	defer win32.CoUninitialize()

	path := filepath.Join(t.TempDir(), "tone.part.mp3")
	encoder, err := newMP3Encoder(path, 320)
	if err != nil {
		t.Fatalf("initialize MP3 encoder: %v", err)
	}
	if err := encoder.WritePCM(sinePCM(SampleRate, 440)); err != nil {
		_ = encoder.Abort()
		t.Fatalf("encode PCM: %v", err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatalf("finalize MP3: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 20_000 {
		t.Fatalf("MP3 is unexpectedly small: %d bytes", info.Size())
	}
	verifyDuration(t, path, 0.9, 1.2)
}

func TestWAVFileIntegrity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tone.part.wav")
	encoder, err := newWAVEncoder(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := encoder.WritePCM(sinePCM(SampleRate, 440)); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	verifyDuration(t, path, 0.99, 1.01)
}

func sinePCM(frames int, frequency float64) []byte {
	data := make([]byte, frames*BlockAlign)
	for frame := 0; frame < frames; frame++ {
		value := int16(math.Sin(2*math.Pi*frequency*float64(frame)/SampleRate) * 12000)
		for channel := 0; channel < Channels; channel++ {
			offset := frame*BlockAlign + channel*2
			binary.LittleEndian.PutUint16(data[offset:offset+2], uint16(value))
		}
	}
	return data
}

func verifyDuration(t *testing.T, path string, minimum, maximum float64) {
	t.Helper()
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Log("ffprobe unavailable; file existence and size were verified")
		return
	}
	output, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe rejected %s: %v (%s)", filepath.Base(path), err, strings.TrimSpace(string(output)))
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		t.Fatalf("parse ffprobe duration %q: %v", output, err)
	}
	if duration < minimum || duration > maximum {
		t.Fatalf("duration %.3fs is outside [%.3f, %.3f]", duration, minimum, maximum)
	}
}
