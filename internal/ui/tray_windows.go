//go:build windows

package ui

import (
	"fmt"
	"time"
	"unsafe"

	"amprec/internal/recording"
	"golang.org/x/sys/windows"
)

func (a *application) trayData(status recording.Status) notifyIconData {
	iconID := uintptr(101)
	if status == recording.StatusRecording || status == recording.StatusFinalizing {
		iconID = 102
	}
	icon, _, _ := procLoadIconW.Call(uintptr(a.instance), iconID)
	if icon == 0 {
		icon, _, _ = procLoadIconW.Call(0, idiApplication)
	}
	data := notifyIconData{
		Size:            uint32(unsafe.Sizeof(notifyIconData{})),
		Window:          a.window,
		ID:              1,
		Flags:           nifMessage | nifIcon | nifTip | nifGUID | nifShowTip,
		CallbackMessage: wmAppTray,
		Icon:            windows.Handle(icon),
		GUID:            trayGUID,
	}
	tip := "AmpRec - " + a.selectedSource().Display + " ready"
	if status == recording.StatusRecording {
		tip = "AmpRec - Recording " + a.elapsed(a.recorder.Snapshot())
	} else if status == recording.StatusFinalizing {
		tip = "AmpRec - Finalizing recording"
	} else if a.selectedPID == 0 {
		tip = "AmpRec - Waiting for " + a.selectedSource().Display
	}
	copyUTF16(data.Tip[:], tip)
	return data
}

func (a *application) addTrayIcon() {
	data := a.trayData(a.recorder.Snapshot().Status)
	result, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	if result == 0 {
		return
	}
	a.trayAdded = true
	data.TimeoutOrVersion = notifyIconVersion4
	procShellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&data)))
}

func (a *application) updateTrayIcon(snapshot recording.Snapshot) {
	if !a.trayAdded {
		return
	}
	data := a.trayData(snapshot.Status)
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&data)))
}

func (a *application) removeTrayIcon() {
	if !a.trayAdded {
		return
	}
	data := a.trayData(recording.StatusIdle)
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
	a.trayAdded = false
}

func (a *application) notify(title, body string, isError bool) {
	if !a.trayAdded {
		return
	}
	data := a.trayData(a.recorder.Snapshot().Status)
	data.Flags = nifInfo | nifGUID
	copyUTF16(data.InfoTitle[:], title)
	copyUTF16(data.Info[:], body)
	data.InfoFlags = niifInfo
	if isError {
		data.InfoFlags = niifError
	}
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&data)))
}

func (a *application) onTray(event uint32) {
	switch event {
	case wmLButtonUp:
		visible, _, _ := procIsWindowVisible.Call(uintptr(a.window))
		if visible != 0 {
			procShowWindow.Call(uintptr(a.window), swHide)
		} else {
			a.showWindow(false)
		}
	case wmRButtonUp, wmContextMenu:
		a.showTrayMenu()
	}
}

func (a *application) showWindow(settings bool) {
	a.showSettings = settings
	a.closeMenus()
	procShowWindow.Call(uintptr(a.window), swRestore)
	procSetForegroundWindow.Call(uintptr(a.window))
	a.invalidate()
}

func (a *application) showTrayMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	snapshot := a.recorder.Snapshot()
	startText := "Start Recording"
	if snapshot.Status == recording.StatusRecording || snapshot.Status == recording.StatusFinalizing {
		startText = "Stop Recording"
	}
	startFlags := uintptr(mfString)
	if a.busy || (a.selectedPID == 0 && snapshot.Status == recording.StatusIdle) {
		startFlags |= mfGrayed
	}
	a.appendMenu(menu, startFlags, trayStartStop, startText)
	a.appendMenu(menu, mfString, trayOpenFolder, "Open Recordings Folder")
	a.appendMenu(menu, mfString, trayShow, "Show AmpRec")
	settingsFlags := uintptr(mfString)
	if a.isRecordingLocked() {
		settingsFlags |= mfGrayed
	}
	a.appendMenu(menu, settingsFlags, traySettings, "Settings")
	a.appendMenu(menu, mfSeparator, 0, "")
	a.appendMenu(menu, mfString, trayExit, "Exit")

	var cursor point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	procSetForegroundWindow.Call(uintptr(a.window))
	command, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd, uintptr(cursor.X), uintptr(cursor.Y), 0, uintptr(a.window), 0)
	if command != 0 {
		a.onTrayCommand(uint32(command))
	}
}

func (a *application) appendMenu(menu uintptr, flags uintptr, id uint32, label string) {
	text := utf16(label)
	var pointer uintptr
	if len(text) > 0 {
		pointer = uintptr(unsafe.Pointer(&text[0]))
	}
	procAppendMenuW.Call(menu, flags, uintptr(id), pointer)
}

func (a *application) onTrayCommand(command uint32) {
	switch command {
	case trayStartStop:
		status := a.recorder.Snapshot().Status
		if status == recording.StatusRecording {
			a.stopRecording("user")
		} else if status == recording.StatusIdle || status == recording.StatusError {
			if a.selectedPID != 0 && !a.busy {
				a.startRecording()
			}
		}
	case trayOpenFolder:
		a.openRecordingsFolder()
	case trayShow:
		a.showWindow(false)
	case traySettings:
		if !a.isRecordingLocked() {
			a.showWindow(true)
		}
	case trayExit:
		a.exiting = true
		a.requestClose()
	}
}

func (a *application) trayDiagnostic() string {
	snapshot := a.recorder.Snapshot()
	return fmt.Sprintf("status=%d pid=%d elapsed=%s at=%s", snapshot.Status, a.selectedPID, a.elapsed(snapshot), time.Now().Format(time.RFC3339))
}
