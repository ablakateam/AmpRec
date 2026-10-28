//go:build windows

package recording

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"amprec/internal/config"
	"amprec/internal/win32"
	"golang.org/x/sys/windows"
)

func TestRecorderMP3ProcessCapture(t *testing.T) {
	winampPath := os.Getenv("AMPREC_TEST_WINAMP")
	if winampPath == "" {
		t.Skip("set AMPREC_TEST_WINAMP to the native tone-player executable")
	}
	winamp := exec.Command(winampPath, "-frequency=440", "-seconds=5")
	if err := winamp.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = winamp.Process.Kill(); _, _ = winamp.Process.Wait() }()
	time.Sleep(350 * time.Millisecond)

	recorder := NewRecorder()
	if err := recorder.Start(StartOptions{
		PID:          uint32(winamp.Process.Pid),
		Format:       config.FormatMP3,
		BitrateKbps:  320,
		OutputFolder: t.TempDir(),
	}); err != nil {
		t.Fatalf("start MP3 process capture: %v", err)
	}
	time.Sleep(2 * time.Second)
	if err := recorder.Stop(); err != nil {
		t.Fatalf("stop MP3 process capture: %v", err)
	}
	snapshot := recorder.Snapshot()
	if snapshot.Status != StatusIdle || filepath.Ext(snapshot.LastFile) != ".mp3" {
		t.Fatalf("unexpected final snapshot: %+v", snapshot)
	}
	info, err := os.Stat(snapshot.LastFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 50_000 {
		t.Fatalf("captured MP3 is unexpectedly small: %d bytes", info.Size())
	}
	verifyDuration(t, snapshot.LastFile, 1.8, 2.3)
}

func TestBrowserProcessTreeMP3Capture(t *testing.T) {
	browserPath := os.Getenv("AMPREC_TEST_BROWSER")
	rendererPath := os.Getenv("AMPREC_TEST_RENDERER")
	if browserPath == "" || rendererPath == "" {
		t.Skip("set AMPREC_TEST_BROWSER and AMPREC_TEST_RENDERER to the browser fixtures")
	}
	browser := exec.Command(browserPath, "-renderer="+rendererPath, "-frequency=523", "-seconds=3")
	if err := browser.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = browser.Process.Kill(); _, _ = browser.Process.Wait() }()
	time.Sleep(350 * time.Millisecond)

	recorder := NewRecorder()
	if err := recorder.Start(StartOptions{
		PID:          uint32(browser.Process.Pid),
		Format:       config.FormatMP3,
		BitrateKbps:  320,
		OutputFolder: t.TempDir(),
	}); err != nil {
		t.Fatalf("start browser process-tree capture: %v", err)
	}
	time.Sleep(2 * time.Second)
	if err := recorder.Stop(); err != nil {
		t.Fatalf("stop browser process-tree capture: %v", err)
	}
	snapshot := recorder.Snapshot()
	if snapshot.Status != StatusIdle || filepath.Ext(snapshot.LastFile) != ".mp3" {
		t.Fatalf("unexpected final snapshot: %+v", snapshot)
	}
	info, err := os.Stat(snapshot.LastFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 50_000 {
		t.Fatalf("captured browser MP3 is unexpectedly small: %d bytes", info.Size())
	}
	verifyDuration(t, snapshot.LastFile, 1.8, 2.3)
}

func TestProcessLoopbackExcludesOtherProcess(t *testing.T) {
	winampPath := os.Getenv("AMPREC_TEST_WINAMP")
	otherPath := os.Getenv("AMPREC_TEST_OTHER_AUDIO")
	if winampPath == "" || otherPath == "" {
		t.Skip("set AMPREC_TEST_WINAMP and AMPREC_TEST_OTHER_AUDIO to native tone-player executables")
	}
	winamp := exec.Command(winampPath, "-frequency=440", "-seconds=5")
	other := exec.Command(otherPath, "-frequency=880", "-seconds=5")
	if err := winamp.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = winamp.Process.Kill(); _, _ = winamp.Process.Wait() }()
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Process.Kill(); _, _ = other.Process.Wait() }()
	time.Sleep(350 * time.Millisecond)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := win32.CoInitializeMTA(); err != nil {
		t.Fatal(err)
	}
	defer win32.CoUninitialize()
	capture, err := newProcessCapture(uint32(winamp.Process.Pid))
	if err != nil {
		t.Fatalf("activate process loopback: %v", err)
	}
	defer capture.Close()
	if err := capture.Start(); err != nil {
		t.Fatal(err)
	}
	stopEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(stopEvent)
	go func() {
		time.Sleep(2 * time.Second)
		_ = windows.SetEvent(stopEvent)
	}()

	path := filepath.Join(t.TempDir(), "process-loopback.wav")
	encoder, err := newWAVEncoder(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := capture.Run(stopEvent, func(packet []byte, _ float32) error {
		return encoder.WritePCM(packet)
	}); err != nil {
		_ = encoder.Abort()
		t.Fatalf("capture process audio: %v", err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < wavHeaderSize+SampleRate*BlockAlign {
		t.Fatalf("capture too short: %d bytes", len(data))
	}
	samples := leftChannel(data[wavHeaderSize:])
	target := goertzel(samples, 440, SampleRate)
	leak := goertzel(samples, 880, SampleRate)
	if target < 1e8 {
		t.Fatalf("selected process tone was not captured (energy %.0f)", target)
	}
	if target < leak*20 {
		t.Fatalf("unselected process leaked into capture: target energy %.0f, other energy %.0f", target, leak)
	}
	t.Logf("process isolation verified: 440 Hz energy %.0f, 880 Hz energy %.0f, ratio %.1f", target, leak, target/math.Max(leak, 1))
}

func leftChannel(pcm []byte) []float64 {
	frames := len(pcm) / BlockAlign
	result := make([]float64, frames)
	for frame := 0; frame < frames; frame++ {
		result[frame] = float64(int16(binary.LittleEndian.Uint16(pcm[frame*BlockAlign:])))
	}
	return result
}

func goertzel(samples []float64, frequency float64, sampleRate float64) float64 {
	coefficient := 2 * math.Cos(2*math.Pi*frequency/sampleRate)
	var previous, previous2 float64
	for _, sample := range samples {
		current := sample + coefficient*previous - previous2
		previous2 = previous
		previous = current
	}
	return previous2*previous2 + previous*previous - coefficient*previous*previous2
}
