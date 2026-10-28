//go:build windows

package recording

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"amprec/internal/win32"
	"golang.org/x/sys/windows"
)

const (
	virtualAudioDeviceProcessLoopback = "VAD\\Process_Loopback"
	vtBlob                            = 65
	audioClientActivationProcessLoop  = 1
	processLoopbackIncludeTree        = 0

	audclntSharemodeShared              = 0
	audclntStreamflagsLoopback          = 0x00020000
	audclntStreamflagsEventCallback     = 0x00040000
	audclntStreamflagsAutoConvertPCM    = 0x80000000
	audclntStreamflagsSrcDefaultQuality = 0x08000000
	audclntBufferflagsSilent            = 0x00000002
	activationTimeoutMilliseconds       = 10000
)

var (
	mmdevapi                        = windows.NewLazySystemDLL("mmdevapi.dll")
	procActivateAudioInterfaceAsync = mmdevapi.NewProc("ActivateAudioInterfaceAsync")

	iidIAudioClient                             = windows.GUID{Data1: 0x1CB9AD4C, Data2: 0xDBFA, Data3: 0x4c32, Data4: [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}}
	iidIAudioCaptureClient                      = windows.GUID{Data1: 0xC8ADBD64, Data2: 0xE71E, Data3: 0x48a0, Data4: [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17}}
	iidIActivateAudioInterfaceCompletionHandler = windows.GUID{Data1: 0x41D949AB, Data2: 0x9862, Data3: 0x444A, Data4: [8]byte{0x80, 0xF6, 0xC2, 0x61, 0x33, 0x4D, 0xA5, 0xEB}}
	iidIAgileObject                             = windows.GUID{Data1: 0x94EA2B94, Data2: 0xE9CC, Data3: 0x49E0, Data4: [8]byte{0xC0, 0xFF, 0xEE, 0x64, 0xCA, 0x8F, 0x5B, 0x90}}
)

type audioClientActivationParams struct {
	ActivationType uint32
	TargetPID      uint32
	LoopbackMode   uint32
}

type propVariantBlob struct {
	VT        uint16
	Reserved1 uint16
	Reserved2 uint16
	Reserved3 uint16
	BlobSize  uint32
	Padding   uint32
	BlobData  uintptr
}

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	ExtraSize      uint16
}

type activationHandlerVTable struct {
	QueryInterface    uintptr
	AddRef            uintptr
	Release           uintptr
	ActivateCompleted uintptr
}

type activationHandler struct {
	VTable      *activationHandlerVTable
	References  atomic.Uint32
	DoneEvent   windows.Handle
	Result      atomic.Uint32
	AudioClient atomic.Uintptr
}

var activationVTable = activationHandlerVTable{
	QueryInterface:    syscall.NewCallback(activationQueryInterface),
	AddRef:            syscall.NewCallback(activationAddRef),
	Release:           syscall.NewCallback(activationRelease),
	ActivateCompleted: syscall.NewCallback(activationCompleted),
}

func activationQueryInterface(this, iidPointer, objectPointer uintptr) uintptr {
	if objectPointer == 0 || iidPointer == 0 {
		return win32.EPointer
	}
	iid := (*windows.GUID)(unsafe.Pointer(iidPointer))
	if win32.EqualGUID(iid, &win32.IIDIUnknown) || win32.EqualGUID(iid, &iidIActivateAudioInterfaceCompletionHandler) || win32.EqualGUID(iid, &iidIAgileObject) {
		*(*uintptr)(unsafe.Pointer(objectPointer)) = this
		activationAddRef(this)
		return win32.SOK
	}
	*(*uintptr)(unsafe.Pointer(objectPointer)) = 0
	return win32.ENoInterface
}

func activationAddRef(this uintptr) uintptr {
	handler := (*activationHandler)(unsafe.Pointer(this))
	return uintptr(handler.References.Add(1))
}

func activationRelease(this uintptr) uintptr {
	handler := (*activationHandler)(unsafe.Pointer(this))
	for {
		current := handler.References.Load()
		if current == 0 {
			return 0
		}
		if handler.References.CompareAndSwap(current, current-1) {
			return uintptr(current - 1)
		}
	}
}

func activationCompleted(this, operation uintptr) uintptr {
	handler := (*activationHandler)(unsafe.Pointer(this))
	var activationResult uint32
	var audioClient uintptr
	methods := win32.VTable(operation, 4)
	hr, _, _ := syscall.SyscallN(methods[3], operation, uintptr(unsafe.Pointer(&activationResult)), uintptr(unsafe.Pointer(&audioClient)))
	if win32.Failed(hr) {
		handler.Result.Store(uint32(hr))
	} else {
		handler.Result.Store(activationResult)
		handler.AudioClient.Store(audioClient)
	}
	_ = windows.SetEvent(handler.DoneEvent)
	return win32.SOK
}

type processCapture struct {
	audioClient   uintptr
	captureClient uintptr
	sampleEvent   windows.Handle
	started       bool
}

func newProcessCapture(pid uint32) (*processCapture, error) {
	doneEvent, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("create activation event: %w", err)
	}
	defer windows.CloseHandle(doneEvent)

	handler := &activationHandler{VTable: &activationVTable, DoneEvent: doneEvent}
	handler.References.Store(1)
	parameters := audioClientActivationParams{
		ActivationType: audioClientActivationProcessLoop,
		TargetPID:      pid,
		LoopbackMode:   processLoopbackIncludeTree,
	}
	variant := propVariantBlob{
		VT:       vtBlob,
		BlobSize: uint32(unsafe.Sizeof(parameters)),
		BlobData: uintptr(unsafe.Pointer(&parameters)),
	}
	device, err := windows.UTF16PtrFromString(virtualAudioDeviceProcessLoopback)
	if err != nil {
		return nil, err
	}
	var operation uintptr
	hr, _, _ := procActivateAudioInterfaceAsync.Call(
		uintptr(unsafe.Pointer(device)),
		uintptr(unsafe.Pointer(&iidIAudioClient)),
		uintptr(unsafe.Pointer(&variant)),
		uintptr(unsafe.Pointer(handler)),
		uintptr(unsafe.Pointer(&operation)),
	)
	if err := win32.HRESULTError("ActivateAudioInterfaceAsync", hr); err != nil {
		return nil, err
	}
	defer win32.Release(operation)

	waitResult, err := windows.WaitForSingleObject(doneEvent, activationTimeoutMilliseconds)
	runtime.KeepAlive(handler)
	runtime.KeepAlive(parameters)
	runtime.KeepAlive(variant)
	if err != nil {
		return nil, fmt.Errorf("wait for process-loopback activation: %w", err)
	}
	if waitResult == win32.WaitTimeout {
		return nil, errors.New("process-loopback activation timed out")
	}
	if activationHR := uintptr(handler.Result.Load()); win32.Failed(activationHR) {
		return nil, win32.HRESULTError("process-loopback activation", activationHR)
	}
	audioClient := handler.AudioClient.Load()
	if audioClient == 0 {
		return nil, errors.New("process-loopback activation returned no audio client")
	}

	capture := &processCapture{audioClient: audioClient}
	if err := capture.initialize(); err != nil {
		capture.Close()
		return nil, err
	}
	return capture, nil
}

func (c *processCapture) initialize() error {
	format := waveFormatEx{
		FormatTag:      1,
		Channels:       Channels,
		SamplesPerSec:  SampleRate,
		AvgBytesPerSec: SampleRate * BlockAlign,
		BlockAlign:     BlockAlign,
		BitsPerSample:  BitsPerSample,
	}
	flags := uint32(audclntStreamflagsLoopback | audclntStreamflagsEventCallback | audclntStreamflagsAutoConvertPCM | audclntStreamflagsSrcDefaultQuality)
	methods := win32.VTable(c.audioClient, 15)
	hr, _, _ := syscall.SyscallN(methods[3], c.audioClient, audclntSharemodeShared, uintptr(flags), 0, 0, uintptr(unsafe.Pointer(&format)), 0)
	if err := win32.HRESULTError("IAudioClient.Initialize", hr); err != nil {
		return err
	}

	var captureClient uintptr
	hr, _, _ = syscall.SyscallN(methods[14], c.audioClient, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&captureClient)))
	if err := win32.HRESULTError("IAudioClient.GetService", hr); err != nil {
		return err
	}
	c.captureClient = captureClient

	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return fmt.Errorf("create sample event: %w", err)
	}
	c.sampleEvent = event
	hr, _, _ = syscall.SyscallN(methods[13], c.audioClient, uintptr(event))
	if err := win32.HRESULTError("IAudioClient.SetEventHandle", hr); err != nil {
		return err
	}
	return nil
}

func (c *processCapture) Start() error {
	if c.audioClient == 0 {
		return errors.New("audio client is not initialized")
	}
	methods := win32.VTable(c.audioClient, 15)
	hr, _, _ := syscall.SyscallN(methods[10], c.audioClient)
	if err := win32.HRESULTError("IAudioClient.Start", hr); err != nil {
		return err
	}
	c.started = true
	return nil
}

func (c *processCapture) Run(stopEvent windows.Handle, consume func([]byte, float32) error) error {
	for {
		result, err := windows.WaitForMultipleObjects([]windows.Handle{stopEvent, c.sampleEvent}, false, win32.Infinite)
		if err != nil {
			return fmt.Errorf("wait for audio packet: %w", err)
		}
		switch result {
		case win32.WaitObject0:
			if c.started {
				methods := win32.VTable(c.audioClient, 15)
				hr, _, _ := syscall.SyscallN(methods[11], c.audioClient)
				if err := win32.HRESULTError("IAudioClient.Stop", hr); err != nil {
					return err
				}
				c.started = false
			}
			return c.drain(consume)
		case win32.WaitObject0 + 1:
			if err := c.drain(consume); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected audio wait result %d", result)
		}
	}
}

func (c *processCapture) drain(consume func([]byte, float32) error) error {
	methods := win32.VTable(c.captureClient, 6)
	for {
		var nextFrames uint32
		hr, _, _ := syscall.SyscallN(methods[5], c.captureClient, uintptr(unsafe.Pointer(&nextFrames)))
		if err := win32.HRESULTError("IAudioCaptureClient.GetNextPacketSize", hr); err != nil {
			return err
		}
		if nextFrames == 0 {
			return nil
		}
		var dataPointer uintptr
		var frames uint32
		var flags uint32
		var devicePosition uint64
		var qpcPosition uint64
		hr, _, _ = syscall.SyscallN(
			methods[3], c.captureClient,
			uintptr(unsafe.Pointer(&dataPointer)),
			uintptr(unsafe.Pointer(&frames)),
			uintptr(unsafe.Pointer(&flags)),
			uintptr(unsafe.Pointer(&devicePosition)),
			uintptr(unsafe.Pointer(&qpcPosition)),
		)
		if err := win32.HRESULTError("IAudioCaptureClient.GetBuffer", hr); err != nil {
			return err
		}
		byteCount := int(frames) * BlockAlign
		packet := make([]byte, byteCount)
		if flags&audclntBufferflagsSilent == 0 && dataPointer != 0 && byteCount > 0 {
			copy(packet, unsafe.Slice((*byte)(unsafe.Pointer(dataPointer)), byteCount))
		}
		level := pcmPeak(packet)
		releaseHR, _, _ := syscall.SyscallN(methods[4], c.captureClient, uintptr(frames))
		if err := win32.HRESULTError("IAudioCaptureClient.ReleaseBuffer", releaseHR); err != nil {
			return err
		}
		if err := consume(packet, level); err != nil {
			return err
		}
	}
}

func (c *processCapture) Close() {
	if c.started && c.audioClient != 0 {
		methods := win32.VTable(c.audioClient, 15)
		syscall.SyscallN(methods[11], c.audioClient)
		c.started = false
	}
	if c.sampleEvent != 0 {
		windows.CloseHandle(c.sampleEvent)
		c.sampleEvent = 0
	}
	win32.Release(c.captureClient)
	c.captureClient = 0
	win32.Release(c.audioClient)
	c.audioClient = 0
}

func pcmPeak(packet []byte) float32 {
	var peak int32
	for index := 0; index+1 < len(packet); index += 2 {
		sample := int32(int16(uint16(packet[index]) | uint16(packet[index+1])<<8))
		if sample < 0 {
			sample = -sample
		}
		if sample > peak {
			peak = sample
		}
	}
	if peak > 32767 {
		peak = 32767
	}
	return float32(peak) / 32767
}
