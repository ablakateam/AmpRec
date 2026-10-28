//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
	"unsafe"

	"amprec/internal/config"
	"amprec/internal/ui"
	"golang.org/x/sys/windows"
)

var (
	version = "1.1.0"
	commit  = "development"
)

func main() {
	defer func() {
		if recovered := recover(); recovered != nil {
			reportFatal(fmt.Errorf("unexpected failure: %v\n%s", recovered, debug.Stack()))
		}
	}()
	if err := ui.Run(); err != nil {
		reportFatal(err)
	}
}

func reportFatal(err error) {
	directory, directoryErr := config.Dir()
	if directoryErr == nil {
		_ = os.MkdirAll(directory, 0o700)
		logPath := filepath.Join(directory, "AmpRec.log")
		entry := fmt.Sprintf("%s version=%s commit=%s error=%v\n", time.Now().Format(time.RFC3339), version, commit, err)
		file, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if openErr == nil {
			_, _ = file.WriteString(entry)
			_ = file.Close()
		}
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	text, _ := windows.UTF16PtrFromString("AmpRec could not start.\n\n" + err.Error())
	title, _ := windows.UTF16PtrFromString("AmpRec")
	messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
