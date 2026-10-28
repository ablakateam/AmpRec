//go:build windows

package win32

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	SOK          = uintptr(0)
	ENoInterface = uintptr(0x80004002)
	EPointer     = uintptr(0x80004003)
	CoInitMTA    = 0
	CoInitSTA    = 2
	WaitObject0  = 0
	WaitTimeout  = 258
	Infinite     = 0xFFFFFFFF
)

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	procCreateGuid       = ole32.NewProc("CoCreateGuid")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")

	IIDIUnknown = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

func Failed(hr uintptr) bool {
	return int32(hr) < 0
}

func HRESULTError(operation string, hr uintptr) error {
	if !Failed(hr) {
		return nil
	}
	return fmt.Errorf("%s failed (HRESULT 0x%08X)", operation, uint32(hr))
}

func CoInitializeMTA() error {
	hr, _, _ := procCoInitializeEx.Call(0, CoInitMTA)
	return HRESULTError("CoInitializeEx", hr)
}

func CoInitializeSTA() error {
	hr, _, _ := procCoInitializeEx.Call(0, CoInitSTA)
	return HRESULTError("CoInitializeEx", hr)
}

func CoUninitialize() {
	procCoUninitialize.Call()
}

func CoTaskMemFree(pointer uintptr) {
	if pointer != 0 {
		procCoTaskMemFree.Call(pointer)
	}
}

func Release(object uintptr) {
	if object == 0 {
		return
	}
	vtable := *(*uintptr)(unsafe.Pointer(object))
	methods := unsafe.Slice((*uintptr)(unsafe.Pointer(vtable)), 3)
	syscall.SyscallN(methods[2], object)
}

func VTable(object uintptr, count int) []uintptr {
	vtable := *(*uintptr)(unsafe.Pointer(object))
	return unsafe.Slice((*uintptr)(unsafe.Pointer(vtable)), count)
}

func EqualGUID(a, b *windows.GUID) bool {
	return *a == *b
}

func NewGUID() (windows.GUID, error) {
	var guid windows.GUID
	hr, _, _ := procCreateGuid.Call(uintptr(unsafe.Pointer(&guid)))
	return guid, HRESULTError("CoCreateGuid", hr)
}

func CoCreateInstance(classID, interfaceID *windows.GUID) (uintptr, error) {
	var object uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(classID)),
		0,
		23, // CLSCTX_ALL
		uintptr(unsafe.Pointer(interfaceID)),
		uintptr(unsafe.Pointer(&object)),
	)
	return object, HRESULTError("CoCreateInstance", hr)
}
