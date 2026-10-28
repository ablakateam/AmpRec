//go:build windows

// sessionvolume reports the Windows audio-session volume for a process.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"amprec/internal/win32"
	"golang.org/x/sys/windows"
)

var (
	clsidMMDeviceEnumerator  = windows.GUID{Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C, Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator   = windows.GUID{Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35, Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioSessionManager2 = windows.GUID{Data1: 0x77AA99A0, Data2: 0x1BD6, Data3: 0x484F, Data4: [8]byte{0x8B, 0xC7, 0x2C, 0x65, 0x4C, 0x9A, 0x9B, 0x6F}}
	iidIAudioSessionControl2 = windows.GUID{Data1: 0xBFB7FF88, Data2: 0x7239, Data3: 0x4FC9, Data4: [8]byte{0x8F, 0xA2, 0x07, 0xC9, 0x50, 0xBE, 0x9C, 0x6D}}
	iidISimpleAudioVolume    = windows.GUID{Data1: 0x87CE5498, Data2: 0x68D6, Data3: 0x44E5, Data4: [8]byte{0x92, 0x15, 0x6D, 0xA4, 0x7E, 0xF8, 0x83, 0xD8}}
)

func main() {
	pid := flag.Uint("pid", 0, "target process ID")
	flag.Parse()
	if *pid == 0 {
		fmt.Fprintln(os.Stderr, "-pid is required")
		os.Exit(2)
	}
	compensation := win32.CaptureVolumeCompensation(uint32(*pid))
	fmt.Printf("winamp_scalar=%.6f session_scalar=%.6f capture_gain=%.6f\n", compensation.WinampScalar, compensation.SessionScalar, compensation.Gain)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := win32.CoInitializeMTA(); err != nil {
		fail(err)
	}
	defer win32.CoUninitialize()

	enumerator, err := win32.CoCreateInstance(&clsidMMDeviceEnumerator, &iidIMMDeviceEnumerator)
	if err != nil {
		fail(err)
	}
	defer win32.Release(enumerator)
	var device uintptr
	methods := win32.VTable(enumerator, 8)
	hr, _, _ := syscall.SyscallN(methods[4], enumerator, 0, 1, uintptr(unsafe.Pointer(&device)))
	if err := win32.HRESULTError("IMMDeviceEnumerator.GetDefaultAudioEndpoint", hr); err != nil {
		fail(err)
	}
	defer win32.Release(device)
	var manager uintptr
	methods = win32.VTable(device, 7)
	hr, _, _ = syscall.SyscallN(methods[3], device, uintptr(unsafe.Pointer(&iidIAudioSessionManager2)), 23, 0, uintptr(unsafe.Pointer(&manager)))
	if err := win32.HRESULTError("IMMDevice.Activate", hr); err != nil {
		fail(err)
	}
	defer win32.Release(manager)
	var sessions uintptr
	methods = win32.VTable(manager, 10)
	hr, _, _ = syscall.SyscallN(methods[5], manager, uintptr(unsafe.Pointer(&sessions)))
	if err := win32.HRESULTError("IAudioSessionManager2.GetSessionEnumerator", hr); err != nil {
		fail(err)
	}
	defer win32.Release(sessions)
	var count int32
	methods = win32.VTable(sessions, 5)
	hr, _, _ = syscall.SyscallN(methods[3], sessions, uintptr(unsafe.Pointer(&count)))
	if err := win32.HRESULTError("IAudioSessionEnumerator.GetCount", hr); err != nil {
		fail(err)
	}
	for index := int32(0); index < count; index++ {
		var control uintptr
		hr, _, _ = syscall.SyscallN(methods[4], sessions, uintptr(index), uintptr(unsafe.Pointer(&control)))
		if win32.Failed(hr) || control == 0 {
			continue
		}
		control2 := queryInterface(control, &iidIAudioSessionControl2)
		if control2 == 0 {
			win32.Release(control)
			continue
		}
		var processID uint32
		controlMethods := win32.VTable(control2, 17)
		syscall.SyscallN(controlMethods[14], control2, uintptr(unsafe.Pointer(&processID)))
		if processID == uint32(*pid) {
			volume := queryInterface(control, &iidISimpleAudioVolume)
			if volume == 0 {
				fail(fmt.Errorf("session has no ISimpleAudioVolume"))
			}
			var scalar float32
			volumeMethods := win32.VTable(volume, 7)
			hr, _, _ = syscall.SyscallN(volumeMethods[4], volume, uintptr(unsafe.Pointer(&scalar)))
			win32.Release(volume)
			win32.Release(control2)
			win32.Release(control)
			if err := win32.HRESULTError("ISimpleAudioVolume.GetMasterVolume", hr); err != nil {
				fail(err)
			}
			decibels := math.Inf(-1)
			if scalar > 0 {
				decibels = 20 * math.Log10(float64(scalar))
			}
			fmt.Printf("pid=%d scalar=%.6f percent=%.1f attenuation_db=%.2f\n", processID, scalar, scalar*100, decibels)
			return
		}
		win32.Release(control2)
		win32.Release(control)
	}
	fail(fmt.Errorf("no audio session found for PID %d", *pid))
}

func queryInterface(object uintptr, iid *windows.GUID) uintptr {
	var result uintptr
	methods := win32.VTable(object, 3)
	hr, _, _ := syscall.SyscallN(methods[0], object, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&result)))
	if win32.Failed(hr) {
		return 0
	}
	return result
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
