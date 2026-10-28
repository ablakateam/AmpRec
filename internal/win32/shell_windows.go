//go:build windows

package win32

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	biFReturnOnlyFSDirs = 0x0001
	biFEditBox          = 0x0010
	biFNewDialogStyle   = 0x0040
	swShowNormal        = 1
)

var (
	shell32                  = windows.NewLazySystemDLL("shell32.dll")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	procShellExecuteW        = shell32.NewProc("ShellExecuteW")
)

type browseInfo struct {
	Owner       windows.Handle
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	LParam      uintptr
	Image       int32
}

func BrowseForFolder(owner windows.Handle, title string) (string, bool, error) {
	titlePointer, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return "", false, err
	}
	displayName := make([]uint16, windows.MAX_PATH)
	info := browseInfo{
		Owner:       owner,
		DisplayName: &displayName[0],
		Title:       titlePointer,
		Flags:       biFReturnOnlyFSDirs | biFEditBox | biFNewDialogStyle,
	}
	pidl, _, callErr := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&info)))
	if pidl == 0 {
		if callErr != syscall.Errno(0) {
			return "", false, callErr
		}
		return "", false, nil
	}
	defer CoTaskMemFree(pidl)
	path := make([]uint16, 32768)
	ok, _, callErr := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		return "", false, fmt.Errorf("SHGetPathFromIDListW failed: %w", callErr)
	}
	return windows.UTF16ToString(path), true, nil
}

func OpenFolder(path string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	result, _, callErr := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, swShowNormal)
	if result <= 32 {
		return fmt.Errorf("open folder failed (code %d): %w", result, callErr)
	}
	return nil
}

func SetStartWithWindows(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !enabled {
		err := key.DeleteValue("AmpRec")
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return key.SetStringValue("AmpRec", `"`+executable+`"`)
}
