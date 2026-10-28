//go:build windows

package win32

import (
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxCaptureCompensation = float32(4.0) // +12.04 dB safety limit.

var (
	volumeUser32            = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows         = volumeUser32.NewProc("EnumWindows")
	procGetClassNameW       = volumeUser32.NewProc("GetClassNameW")
	procGetWindowThreadPID  = volumeUser32.NewProc("GetWindowThreadProcessId")
	procSendMessageTimeoutW = volumeUser32.NewProc("SendMessageTimeoutW")

	clsidMMDeviceEnumerator  = windows.GUID{Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C, Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator   = windows.GUID{Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35, Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioSessionManager2 = windows.GUID{Data1: 0x77AA99A0, Data2: 0x1BD6, Data3: 0x484F, Data4: [8]byte{0x8B, 0xC7, 0x2C, 0x65, 0x4C, 0x9A, 0x9B, 0x6F}}
	iidIAudioSessionControl2 = windows.GUID{Data1: 0xBFB7FF88, Data2: 0x7239, Data3: 0x4FC9, Data4: [8]byte{0x8F, 0xA2, 0x07, 0xC9, 0x50, 0xBE, 0x9C, 0x6D}}
	iidISimpleAudioVolume    = windows.GUID{Data1: 0x87CE5498, Data2: 0x68D6, Data3: 0x44E5, Data4: [8]byte{0x92, 0x15, 0x6D, 0xA4, 0x7E, 0xF8, 0x83, 0xD8}}
)

type VolumeCompensation struct {
	WinampScalar  float32
	SessionScalar float32
	Gain          float32
}

// CaptureVolumeCompensation prevents Winamp and its Windows audio session
// volume from being baked into a recording and then applied a second time
// when the recording is replayed through the same path.
func CaptureVolumeCompensation(pid uint32) VolumeCompensation {
	result := VolumeCompensation{WinampScalar: 1, SessionScalar: 1, Gain: 1}
	if scalar, ok := winampVolumeScalar(pid); ok {
		result.WinampScalar = scalar
	}
	if scalar, ok := processSessionVolume(pid); ok {
		result.SessionScalar = scalar
	}
	result.Gain = compensationGain(result.WinampScalar, result.SessionScalar)
	return result
}

func compensationGain(winampScalar, sessionScalar float32) float32 {
	combined := winampScalar * sessionScalar
	gain := float32(1)
	if combined > 0 && combined < 1 {
		gain = 1 / combined
		if gain > maxCaptureCompensation {
			gain = maxCaptureCompensation
		}
	}
	return gain
}

func winampVolumeScalar(pid uint32) (float32, bool) {
	type search struct {
		pid    uint32
		window uintptr
	}
	state := &search{pid: pid}
	callback := syscall.NewCallback(func(window, parameter uintptr) uintptr {
		candidate := (*search)(unsafe.Pointer(parameter))
		var processID uint32
		procGetWindowThreadPID.Call(window, uintptr(unsafe.Pointer(&processID)))
		if processID != candidate.pid {
			return 1
		}
		name := make([]uint16, 64)
		length, _, _ := procGetClassNameW.Call(window, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if length > 0 && strings.EqualFold(windows.UTF16ToString(name[:length]), "Winamp v1.x") {
			candidate.window = window
			return 0
		}
		return 1
	})
	procEnumWindows.Call(callback, uintptr(unsafe.Pointer(state)))
	runtime.KeepAlive(state)
	if state.window == 0 {
		return 1, false
	}
	var result uintptr
	requestCurrentVolume := int64(-666)
	success, _, _ := procSendMessageTimeoutW.Call(
		state.window,
		0x0400, // WM_WA_IPC
		uintptr(requestCurrentVolume),
		122,    // IPC_SETVOLUME; -666 queries without changing it.
		0x0002, // SMTO_ABORTIFHUNG
		500,
		uintptr(unsafe.Pointer(&result)),
	)
	if success == 0 || result > 255 {
		return 1, false
	}
	return float32(result) / 255, true
}

func processSessionVolume(pid uint32) (float32, bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := CoInitializeMTA(); err != nil {
		return 1, false
	}
	defer CoUninitialize()

	enumerator, err := CoCreateInstance(&clsidMMDeviceEnumerator, &iidIMMDeviceEnumerator)
	if err != nil {
		return 1, false
	}
	defer Release(enumerator)
	var device uintptr
	methods := VTable(enumerator, 8)
	hr, _, _ := syscall.SyscallN(methods[4], enumerator, 0, 1, uintptr(unsafe.Pointer(&device)))
	if Failed(hr) || device == 0 {
		return 1, false
	}
	defer Release(device)
	var manager uintptr
	methods = VTable(device, 7)
	hr, _, _ = syscall.SyscallN(methods[3], device, uintptr(unsafe.Pointer(&iidIAudioSessionManager2)), 23, 0, uintptr(unsafe.Pointer(&manager)))
	if Failed(hr) || manager == 0 {
		return 1, false
	}
	defer Release(manager)
	var sessions uintptr
	methods = VTable(manager, 10)
	hr, _, _ = syscall.SyscallN(methods[5], manager, uintptr(unsafe.Pointer(&sessions)))
	if Failed(hr) || sessions == 0 {
		return 1, false
	}
	defer Release(sessions)
	var count int32
	methods = VTable(sessions, 5)
	hr, _, _ = syscall.SyscallN(methods[3], sessions, uintptr(unsafe.Pointer(&count)))
	if Failed(hr) {
		return 1, false
	}
	for index := int32(0); index < count; index++ {
		var control uintptr
		hr, _, _ = syscall.SyscallN(methods[4], sessions, uintptr(index), uintptr(unsafe.Pointer(&control)))
		if Failed(hr) || control == 0 {
			continue
		}
		control2 := queryInterface(control, &iidIAudioSessionControl2)
		if control2 == 0 {
			Release(control)
			continue
		}
		var processID uint32
		controlMethods := VTable(control2, 17)
		syscall.SyscallN(controlMethods[14], control2, uintptr(unsafe.Pointer(&processID)))
		if processID == pid {
			volume := queryInterface(control, &iidISimpleAudioVolume)
			Release(control2)
			Release(control)
			if volume == 0 {
				return 1, false
			}
			var scalar float32
			volumeMethods := VTable(volume, 7)
			hr, _, _ = syscall.SyscallN(volumeMethods[4], volume, uintptr(unsafe.Pointer(&scalar)))
			Release(volume)
			if Failed(hr) || scalar < 0 || scalar > 1 {
				return 1, false
			}
			return scalar, true
		}
		Release(control2)
		Release(control)
	}
	return 1, false
}

func queryInterface(object uintptr, iid *windows.GUID) uintptr {
	var result uintptr
	methods := VTable(object, 3)
	hr, _, _ := syscall.SyscallN(methods[0], object, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&result)))
	if Failed(hr) {
		return 0
	}
	return result
}
