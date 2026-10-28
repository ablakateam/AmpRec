//go:build windows

// toneplayer is a deterministic WinMM renderer used only by AmpRec integration tests.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	sampleRate    = 44100
	channels      = 2
	bitsPerSample = 16
	waveMapper    = ^uint32(0)
	whdrDone      = 0x00000001
)

var (
	winmm                  = windows.NewLazySystemDLL("winmm.dll")
	waveOutOpen            = winmm.NewProc("waveOutOpen")
	waveOutPrepareHeader   = winmm.NewProc("waveOutPrepareHeader")
	waveOutWrite           = winmm.NewProc("waveOutWrite")
	waveOutUnprepareHeader = winmm.NewProc("waveOutUnprepareHeader")
	waveOutClose           = winmm.NewProc("waveOutClose")
)

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	ExtraSize      uint16
}

type waveHeader struct {
	Data          uintptr
	BufferLength  uint32
	BytesRecorded uint32
	User          uintptr
	Flags         uint32
	Loops         uint32
	Next          uintptr
	Reserved      uintptr
}

func main() {
	frequency := flag.Float64("frequency", 440, "tone frequency in Hz")
	seconds := flag.Float64("seconds", 4, "duration in seconds")
	flag.Parse()
	if err := play(*frequency, *seconds); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func play(frequency, seconds float64) error {
	frames := int(sampleRate * seconds)
	data := make([]byte, frames*channels*2)
	for frame := 0; frame < frames; frame++ {
		sample := int16(math.Sin(2*math.Pi*frequency*float64(frame)/sampleRate) * 8000)
		for channel := 0; channel < channels; channel++ {
			offset := (frame*channels + channel) * 2
			binary.LittleEndian.PutUint16(data[offset:offset+2], uint16(sample))
		}
	}
	format := waveFormatEx{
		FormatTag:      1,
		Channels:       channels,
		SamplesPerSec:  sampleRate,
		AvgBytesPerSec: sampleRate * channels * 2,
		BlockAlign:     channels * 2,
		BitsPerSample:  bitsPerSample,
	}
	var output uintptr
	result, _, _ := waveOutOpen.Call(uintptr(unsafe.Pointer(&output)), uintptr(waveMapper), uintptr(unsafe.Pointer(&format)), 0, 0, 0)
	if result != 0 {
		return fmt.Errorf("waveOutOpen failed with MMRESULT %d", result)
	}
	header := waveHeader{Data: uintptr(unsafe.Pointer(&data[0])), BufferLength: uint32(len(data))}
	result, _, _ = waveOutPrepareHeader.Call(output, uintptr(unsafe.Pointer(&header)), unsafe.Sizeof(header))
	if result != 0 {
		waveOutClose.Call(output)
		return fmt.Errorf("waveOutPrepareHeader failed with MMRESULT %d", result)
	}
	result, _, _ = waveOutWrite.Call(output, uintptr(unsafe.Pointer(&header)), unsafe.Sizeof(header))
	if result != 0 {
		waveOutUnprepareHeader.Call(output, uintptr(unsafe.Pointer(&header)), unsafe.Sizeof(header))
		waveOutClose.Call(output)
		return fmt.Errorf("waveOutWrite failed with MMRESULT %d", result)
	}
	deadline := time.Now().Add(time.Duration(seconds*float64(time.Second)) + 2*time.Second)
	for header.Flags&whdrDone == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	result, _, _ = waveOutUnprepareHeader.Call(output, uintptr(unsafe.Pointer(&header)), unsafe.Sizeof(header))
	waveOutClose.Call(output)
	if result != 0 {
		return fmt.Errorf("waveOutUnprepareHeader failed with MMRESULT %d", result)
	}
	if header.Flags&whdrDone == 0 {
		return fmt.Errorf("audio playback timed out")
	}
	return nil
}
