//go:build windows

package recording

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"amprec/internal/win32"
	"golang.org/x/sys/windows"
)

const mediaFoundationVersion = 0x00020070

var (
	mfplat                        = windows.NewLazySystemDLL("mfplat.dll")
	mfreadwrite                   = windows.NewLazySystemDLL("mfreadwrite.dll")
	procMFStartup                 = mfplat.NewProc("MFStartup")
	procMFShutdown                = mfplat.NewProc("MFShutdown")
	procMFCreateMediaType         = mfplat.NewProc("MFCreateMediaType")
	procMFCreateMemoryBuffer      = mfplat.NewProc("MFCreateMemoryBuffer")
	procMFCreateSample            = mfplat.NewProc("MFCreateSample")
	procMFCreateSinkWriterFromURL = mfreadwrite.NewProc("MFCreateSinkWriterFromURL")

	mfMediaTypeAudio = windows.GUID{Data1: 0x73647561, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
	mfAudioFormatPCM = windows.GUID{Data1: 0x00000001, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
	mfAudioFormatMP3 = windows.GUID{Data1: 0x00000055, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}

	mfMTMajorType              = windows.GUID{Data1: 0x48EBA18E, Data2: 0xF8C9, Data3: 0x4687, Data4: [8]byte{0xBF, 0x11, 0x0A, 0x74, 0xC9, 0xF9, 0x6A, 0x8F}}
	mfMTSubtype                = windows.GUID{Data1: 0xF7E34C9A, Data2: 0x42E8, Data3: 0x4714, Data4: [8]byte{0xB7, 0x4B, 0xCB, 0x29, 0xD7, 0x2C, 0x35, 0xE5}}
	mfMTAudioNumChannels       = windows.GUID{Data1: 0x37E48BF5, Data2: 0x645E, Data3: 0x4C5B, Data4: [8]byte{0x89, 0xDE, 0xAD, 0xA9, 0xE2, 0x9B, 0x69, 0x6A}}
	mfMTAudioSamplesPerSecond  = windows.GUID{Data1: 0x5FAEEAE7, Data2: 0x0290, Data3: 0x4C31, Data4: [8]byte{0x9E, 0x8A, 0xC5, 0x34, 0xF6, 0x8D, 0x9D, 0xBA}}
	mfMTAudioAvgBytesPerSecond = windows.GUID{Data1: 0x1AAB75C8, Data2: 0xCFEF, Data3: 0x451C, Data4: [8]byte{0xAB, 0x95, 0xAC, 0x03, 0x4B, 0x8E, 0x17, 0x31}}
	mfMTAudioBlockAlignment    = windows.GUID{Data1: 0x322DE230, Data2: 0x9EEB, Data3: 0x43BD, Data4: [8]byte{0xAB, 0x7A, 0xFF, 0x41, 0x22, 0x51, 0x54, 0x1D}}
	mfMTAudioBitsPerSample     = windows.GUID{Data1: 0xF2DEB57F, Data2: 0x40FA, Data3: 0x4764, Data4: [8]byte{0xAA, 0x33, 0xED, 0x4F, 0x2D, 0x1F, 0xF6, 0x69}}
	mfMTAllSamplesIndependent  = windows.GUID{Data1: 0xC9173739, Data2: 0x5E56, Data3: 0x461C, Data4: [8]byte{0xB7, 0x13, 0x46, 0xFB, 0x99, 0x5C, 0xB9, 0x5F}}
)

type mp3Encoder struct {
	sinkWriter uintptr
	stream     uint32
	frames     uint64
	startedMF  bool
	closed     bool
}

func newMP3Encoder(path string, bitrateKbps int) (*mp3Encoder, error) {
	if bitrateKbps != 128 && bitrateKbps != 192 && bitrateKbps != 256 && bitrateKbps != 320 {
		return nil, fmt.Errorf("unsupported MP3 bitrate %d kbps", bitrateKbps)
	}
	hr, _, _ := procMFStartup.Call(mediaFoundationVersion, 0)
	if err := win32.HRESULTError("MFStartup", hr); err != nil {
		return nil, err
	}
	encoder := &mp3Encoder{startedMF: true}
	if err := encoder.initialize(path, bitrateKbps); err != nil {
		_ = encoder.Abort()
		return nil, err
	}
	return encoder, nil
}

func (m *mp3Encoder) initialize(path string, bitrateKbps int) error {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var sinkWriter uintptr
	hr, _, _ := procMFCreateSinkWriterFromURL.Call(uintptr(unsafe.Pointer(pathPointer)), 0, 0, uintptr(unsafe.Pointer(&sinkWriter)))
	if err := win32.HRESULTError("MFCreateSinkWriterFromURL", hr); err != nil {
		return err
	}
	m.sinkWriter = sinkWriter

	outputType, err := createMediaType()
	if err != nil {
		return err
	}
	defer win32.Release(outputType)
	if err := setMediaGUID(outputType, &mfMTMajorType, &mfMediaTypeAudio); err != nil {
		return err
	}
	if err := setMediaGUID(outputType, &mfMTSubtype, &mfAudioFormatMP3); err != nil {
		return err
	}
	for _, attribute := range []struct {
		key   *windows.GUID
		value uint32
	}{
		{&mfMTAudioSamplesPerSecond, SampleRate},
		{&mfMTAudioNumChannels, Channels},
		{&mfMTAudioAvgBytesPerSecond, uint32(bitrateKbps * 1000 / 8)},
	} {
		if err := setMediaUINT32(outputType, attribute.key, attribute.value); err != nil {
			return err
		}
	}

	methods := win32.VTable(m.sinkWriter, 14)
	var stream uint32
	hr, _, _ = syscall.SyscallN(methods[3], m.sinkWriter, outputType, uintptr(unsafe.Pointer(&stream)))
	if err := win32.HRESULTError("IMFSinkWriter.AddStream", hr); err != nil {
		return err
	}
	m.stream = stream

	inputType, err := createMediaType()
	if err != nil {
		return err
	}
	defer win32.Release(inputType)
	if err := setMediaGUID(inputType, &mfMTMajorType, &mfMediaTypeAudio); err != nil {
		return err
	}
	if err := setMediaGUID(inputType, &mfMTSubtype, &mfAudioFormatPCM); err != nil {
		return err
	}
	for _, attribute := range []struct {
		key   *windows.GUID
		value uint32
	}{
		{&mfMTAudioSamplesPerSecond, SampleRate},
		{&mfMTAudioNumChannels, Channels},
		{&mfMTAudioBitsPerSample, BitsPerSample},
		{&mfMTAudioBlockAlignment, BlockAlign},
		{&mfMTAudioAvgBytesPerSecond, SampleRate * BlockAlign},
		{&mfMTAllSamplesIndependent, 1},
	} {
		if err := setMediaUINT32(inputType, attribute.key, attribute.value); err != nil {
			return err
		}
	}
	hr, _, _ = syscall.SyscallN(methods[4], m.sinkWriter, uintptr(stream), inputType, 0)
	if err := win32.HRESULTError("IMFSinkWriter.SetInputMediaType", hr); err != nil {
		return err
	}
	hr, _, _ = syscall.SyscallN(methods[5], m.sinkWriter)
	return win32.HRESULTError("IMFSinkWriter.BeginWriting", hr)
}

func createMediaType() (uintptr, error) {
	var mediaType uintptr
	hr, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&mediaType)))
	return mediaType, win32.HRESULTError("MFCreateMediaType", hr)
}

func setMediaGUID(mediaType uintptr, key, value *windows.GUID) error {
	methods := win32.VTable(mediaType, 33)
	hr, _, _ := syscall.SyscallN(methods[24], mediaType, uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(value)))
	return win32.HRESULTError("IMFMediaType.SetGUID", hr)
}

func setMediaUINT32(mediaType uintptr, key *windows.GUID, value uint32) error {
	methods := win32.VTable(mediaType, 33)
	hr, _, _ := syscall.SyscallN(methods[21], mediaType, uintptr(unsafe.Pointer(key)), uintptr(value))
	return win32.HRESULTError("IMFMediaType.SetUINT32", hr)
}

func (m *mp3Encoder) WritePCM(data []byte) error {
	if m.closed || m.sinkWriter == 0 {
		return errors.New("MP3 encoder is closed")
	}
	if len(data) == 0 {
		return nil
	}
	if len(data)%BlockAlign != 0 {
		return fmt.Errorf("PCM packet length %d is not block-aligned", len(data))
	}
	var buffer uintptr
	hr, _, _ := procMFCreateMemoryBuffer.Call(uintptr(len(data)), uintptr(unsafe.Pointer(&buffer)))
	if err := win32.HRESULTError("MFCreateMemoryBuffer", hr); err != nil {
		return err
	}
	defer win32.Release(buffer)
	bufferMethods := win32.VTable(buffer, 8)
	var destination uintptr
	var maxLength uint32
	var currentLength uint32
	hr, _, _ = syscall.SyscallN(bufferMethods[3], buffer, uintptr(unsafe.Pointer(&destination)), uintptr(unsafe.Pointer(&maxLength)), uintptr(unsafe.Pointer(&currentLength)))
	if err := win32.HRESULTError("IMFMediaBuffer.Lock", hr); err != nil {
		return err
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(destination)), len(data)), data)
	unlockHR, _, _ := syscall.SyscallN(bufferMethods[4], buffer)
	if err := win32.HRESULTError("IMFMediaBuffer.Unlock", unlockHR); err != nil {
		return err
	}
	hr, _, _ = syscall.SyscallN(bufferMethods[6], buffer, uintptr(len(data)))
	if err := win32.HRESULTError("IMFMediaBuffer.SetCurrentLength", hr); err != nil {
		return err
	}

	var sample uintptr
	hr, _, _ = procMFCreateSample.Call(uintptr(unsafe.Pointer(&sample)))
	if err := win32.HRESULTError("MFCreateSample", hr); err != nil {
		return err
	}
	defer win32.Release(sample)
	sampleMethods := win32.VTable(sample, 47)
	hr, _, _ = syscall.SyscallN(sampleMethods[42], sample, buffer)
	if err := win32.HRESULTError("IMFSample.AddBuffer", hr); err != nil {
		return err
	}
	packetFrames := uint64(len(data) / BlockAlign)
	sampleTime := m.frames * 10_000_000 / SampleRate
	duration := packetFrames * 10_000_000 / SampleRate
	hr, _, _ = syscall.SyscallN(sampleMethods[36], sample, uintptr(sampleTime))
	if err := win32.HRESULTError("IMFSample.SetSampleTime", hr); err != nil {
		return err
	}
	hr, _, _ = syscall.SyscallN(sampleMethods[38], sample, uintptr(duration))
	if err := win32.HRESULTError("IMFSample.SetSampleDuration", hr); err != nil {
		return err
	}
	writerMethods := win32.VTable(m.sinkWriter, 14)
	hr, _, _ = syscall.SyscallN(writerMethods[6], m.sinkWriter, uintptr(m.stream), sample)
	if err := win32.HRESULTError("IMFSinkWriter.WriteSample", hr); err != nil {
		return err
	}
	m.frames += packetFrames
	return nil
}

func (m *mp3Encoder) Close() error {
	if m.closed {
		return nil
	}
	m.closed = true
	var finalizeErr error
	if m.sinkWriter != 0 {
		methods := win32.VTable(m.sinkWriter, 14)
		hr, _, _ := syscall.SyscallN(methods[11], m.sinkWriter)
		finalizeErr = win32.HRESULTError("IMFSinkWriter.Finalize", hr)
		win32.Release(m.sinkWriter)
		m.sinkWriter = 0
	}
	if m.startedMF {
		hr, _, _ := procMFShutdown.Call()
		if shutdownErr := win32.HRESULTError("MFShutdown", hr); finalizeErr == nil {
			finalizeErr = shutdownErr
		}
		m.startedMF = false
	}
	return finalizeErr
}

func (m *mp3Encoder) Abort() error {
	if m.closed {
		return nil
	}
	m.closed = true
	if m.sinkWriter != 0 {
		win32.Release(m.sinkWriter)
		m.sinkWriter = 0
	}
	if m.startedMF {
		procMFShutdown.Call()
		m.startedMF = false
	}
	return nil
}
