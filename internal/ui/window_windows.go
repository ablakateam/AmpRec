//go:build windows

package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"amprec/internal/config"
	"amprec/internal/recording"
	"amprec/internal/win32"
	"golang.org/x/sys/windows"
)

const (
	baseWidth  = 390
	baseHeight = 452
	timerID    = 1

	trayStartStop  = 1001
	trayOpenFolder = 1002
	trayShow       = 1003
	traySettings   = 1004
	trayExit       = 1005
)

var (
	windowClassName = utf16("AmpRec.MainWindow.v1")
	windowTitle     = utf16("AmpRec")
	windowCallback  = callWindowProc(globalWindowProc)
	applications    sync.Map
	trayGUID        = windows.GUID{Data1: 0xEAEF1D69, Data2: 0xA50E, Data3: 0x48F0, Data4: [8]byte{0x9D, 0x1C, 0x91, 0xAE, 0xF7, 0x35, 0x6D, 0x44}}
)

type pendingResult struct {
	kind string
	err  error
}

type application struct {
	window   windows.Handle
	instance windows.Handle
	dpi      uint32

	settings        config.Settings
	recorder        *recording.Recorder
	processes       []win32.ProcessInfo
	sourceProcesses map[string][]win32.ProcessInfo
	selectedPID     uint32

	showSettings    bool
	showFormatMenu  bool
	showQualityMenu bool
	showSourceMenu  bool
	busy            bool
	exiting         bool
	trayAdded       bool
	lastTrayStatus  recording.Status
	tick            uint32

	statusMessage string
	statusError   bool
	focus         int
	keyboardMode  bool
	pendingMu     sync.Mutex
	pending       *pendingResult
}

func Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := win32.CoInitializeSTA(); err != nil {
		return err
	}
	defer win32.CoUninitialize()

	mutexName, _ := windows.UTF16PtrFromString("Local\\AmpRec.SingleInstance.v1")
	mutex, mutexErr := windows.CreateMutex(nil, false, mutexName)
	if mutex != 0 {
		defer windows.CloseHandle(mutex)
	}
	if errors.Is(mutexErr, windows.ERROR_ALREADY_EXISTS) {
		existing, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(&windowClassName[0])), 0)
		if existing != 0 {
			procShowWindow.Call(existing, swRestore)
			procSetForegroundWindow.Call(existing)
		}
		return nil
	}

	procSetProcessDpiAwarenessContext.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	settings, settingsErr := config.Load()
	app := &application{settings: settings, recorder: recording.NewRecorder(), dpi: 96, sourceProcesses: make(map[string][]win32.ProcessInfo)}
	if settingsErr != nil {
		app.statusMessage = "Settings were reset: " + settingsErr.Error()
		app.statusError = true
	}
	if err := app.createWindow(); err != nil {
		return err
	}
	defer app.cleanup()
	app.scanProcesses()
	app.addTrayIcon()
	procSetTimer.Call(uintptr(app.window), timerID, 100, 0)
	procShowWindow.Call(uintptr(app.window), swShow)
	procUpdateWindow.Call(uintptr(app.window))

	var message message
	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("GetMessageW failed: %w", callErr)
		}
		if result == 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func (a *application) createWindow() error {
	instance, _, _ := procGetModuleHandleW.Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	icon, _, _ := procLoadIconW.Call(instance, 101)
	if icon == 0 {
		icon, _, _ = procLoadIconW.Call(0, idiApplication)
	}
	class := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		Style:      csHRedraw | csVRedraw,
		WndProc:    windowCallback,
		Instance:   windows.Handle(instance),
		Icon:       windows.Handle(icon),
		Cursor:     windows.Handle(cursor),
		Background: windows.Handle(colorWindow + 1),
		ClassName:  &windowClassName[0],
		IconSmall:  windows.Handle(icon),
	}
	atom, _, registerErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 && !errors.Is(registerErr, windows.ERROR_CLASS_ALREADY_EXISTS) {
		return fmt.Errorf("RegisterClassExW failed: %w", registerErr)
	}

	screenWidth, _, _ := procGetSystemMetrics.Call(0)
	screenHeight, _, _ := procGetSystemMetrics.Call(1)
	x := (int32(screenWidth) - baseWidth) / 2
	y := (int32(screenHeight) - baseHeight) / 2
	handle, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(&windowClassName[0])),
		uintptr(unsafe.Pointer(&windowTitle[0])),
		wsPopup|wsSysMenu|wsMinimizeBox,
		uintptr(x), uintptr(y), baseWidth, baseHeight,
		0, 0, instance, 0,
	)
	if handle == 0 {
		return fmt.Errorf("CreateWindowExW failed: %w", createErr)
	}
	a.window = windows.Handle(handle)
	a.instance = windows.Handle(instance)
	applications.Store(a.window, a)
	dpi, _, _ := procGetDpiForWindow.Call(handle)
	if dpi != 0 {
		a.dpi = uint32(dpi)
	}
	dark := uint32(1)
	corner := uint32(2)
	procDwmSetWindowAttribute.Call(handle, 20, uintptr(unsafe.Pointer(&dark)), unsafe.Sizeof(dark))
	procDwmSetWindowAttribute.Call(handle, 33, uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))
	if a.dpi != 96 {
		width := a.scale(baseWidth)
		height := a.scale(baseHeight)
		procSetWindowPos.Call(handle, 0, 0, 0, uintptr(width), uintptr(height), 0x0002|0x0004)
	}
	return nil
}

func globalWindowProc(hwnd, message, wParam, lParam uintptr) uintptr {
	value, exists := applications.Load(windows.Handle(hwnd))
	if !exists {
		return defWindowProc(windows.Handle(hwnd), uint32(message), wParam, lParam)
	}
	a := value.(*application)
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		a.paint()
		return 0
	case wmTimer:
		a.onTimer()
		return 0
	case wmKeyDown:
		a.onKey(uint32(wParam))
		return 0
	case wmLButtonUp:
		a.onClick(lowWordSigned(lParam), highWordSigned(lParam))
		return 0
	case wmNCHitTest:
		result := defWindowProc(a.window, uint32(message), wParam, lParam)
		if result == htClient {
			cursor := point{X: lowWordSigned(lParam), Y: highWordSigned(lParam)}
			procScreenToClient.Call(uintptr(a.window), uintptr(unsafe.Pointer(&cursor)))
			if a.toBase(cursor.Y) < 54 && a.toBase(cursor.X) < 318 {
				return htCaption
			}
		}
		return result
	case wmQueryEndSession:
		return 1
	case wmEndSession:
		if wParam != 0 {
			_ = a.recorder.Stop()
		}
		return 0
	case wmDPICHanged:
		a.dpi = uint32(wParam & 0xffff)
		suggested := (*rect)(unsafe.Pointer(lParam))
		procSetWindowPos.Call(uintptr(a.window), 0, uintptr(suggested.Left), uintptr(suggested.Top), uintptr(suggested.Right-suggested.Left), uintptr(suggested.Bottom-suggested.Top), 0x0004)
		a.invalidate()
		return 0
	case wmAppTray:
		a.onTray(uint32(lParam & 0xffff))
		return 0
	case wmCommand:
		a.onTrayCommand(uint32(wParam & 0xffff))
		return 0
	case wmAppRefresh:
		a.applyPending()
		return 0
	case wmClose:
		a.requestClose()
		return 0
	case wmAppExitReady:
		procDestroyWindow.Call(uintptr(a.window))
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	return defWindowProc(a.window, uint32(message), wParam, lParam)
}

func (a *application) onTimer() {
	a.tick++
	if a.tick%10 == 0 {
		a.scanProcesses()
	}
	snapshot := a.recorder.Snapshot()
	if snapshot.Status == recording.StatusError && snapshot.LastError != "" && !a.busy {
		a.statusMessage = snapshot.LastError
		a.statusError = true
	}
	if snapshot.Status != a.lastTrayStatus {
		a.updateTrayIcon(snapshot)
		a.lastTrayStatus = snapshot.Status
	} else if snapshot.Status == recording.StatusRecording && a.tick%10 == 0 {
		a.updateTrayIcon(snapshot)
	}
	a.invalidate()
}

func (a *application) scanProcesses() {
	processesByExecutable, err := win32.FindRootProcessesByNames(sourceExecutables())
	if err != nil {
		a.statusMessage = "Could not inspect running processes: " + err.Error()
		a.statusError = true
		return
	}
	for _, source := range supportedSources {
		a.sourceProcesses[source.ID] = processesByExecutable[source.Executable]
	}
	source := a.selectedSource()
	processes := a.sourceProcesses[source.ID]
	previousSelected := a.selectedPID
	a.processes = processes
	selectedExists := false
	for _, process := range processes {
		if process.PID == a.selectedPID {
			selectedExists = true
			break
		}
	}
	snapshot := a.recorder.Snapshot()
	if snapshot.Status == recording.StatusRecording && previousSelected != 0 && !selectedExists && !a.busy {
		a.stopRecording("lost")
		return
	}
	if !selectedExists && snapshot.Status != recording.StatusRecording && snapshot.Status != recording.StatusFinalizing {
		if len(processes) > 0 {
			a.selectedPID = processes[0].PID
		} else {
			a.selectedPID = 0
		}
	}
}

func (a *application) onClick(x, y int32) {
	a.keyboardMode = false
	x = a.toBase(x)
	y = a.toBase(y)
	if x >= 354 && y <= 54 {
		a.requestClose()
		return
	}
	if x >= 318 && x < 354 && y <= 54 {
		if !a.isRecordingLocked() {
			a.showSettings = !a.showSettings
			a.closeMenus()
		}
		a.invalidate()
		return
	}
	if a.handleOpenMenuClick(x, y) {
		return
	}
	if a.showSettings {
		a.onSettingsClick(x, y)
	} else {
		a.onMainClick(x, y)
	}
}

func (a *application) onKey(key uint32) {
	a.keyboardMode = true
	if key == vkEscape {
		if a.showFormatMenu || a.showQualityMenu || a.showSourceMenu {
			a.closeMenus()
		} else if a.showSettings {
			a.showSettings = false
			a.focus = 5
		} else {
			a.requestClose()
		}
		a.invalidate()
		return
	}
	if key == vkTab {
		count := 6
		shift, _, _ := procGetKeyState.Call(vkShift)
		if uint16(shift)&0x8000 != 0 {
			a.focus = (a.focus + count - 1) % count
		} else {
			a.focus = (a.focus + 1) % count
		}
		a.closeMenus()
		a.invalidate()
		return
	}
	if key != vkReturn && key != vkSpace {
		return
	}
	if a.showSettings {
		switch a.focus {
		case 0:
			a.onSettingsClick(100, 130)
		case 1:
			a.onSettingsClick(100, 190)
		case 2:
			a.onSettingsClick(100, 260)
		case 3:
			a.onSettingsClick(100, 294)
		case 4:
			a.onSettingsClick(100, 328)
		case 5:
			a.onSettingsClick(195, 414)
		}
	} else {
		switch a.focus {
		case 0:
			if !a.isRecordingLocked() {
				a.showSettings = true
				a.focus = 0
			}
		case 1:
			a.onMainClick(180, 150)
		case 2:
			a.onMainClick(100, 275)
		case 3:
			a.onMainClick(280, 275)
		case 4:
			a.onMainClick(180, 345)
		case 5:
			a.onMainClick(180, 416)
		}
	}
	a.invalidate()
}

func (a *application) handleOpenMenuClick(x, y int32) bool {
	if a.showSourceMenu {
		choices := a.sourceChoices()
		if x >= 20 && x <= 370 && y >= 129 {
			index := int((y - 129) / 36)
			if index >= 0 && index < len(choices) && index < 8 {
				choice := choices[index]
				a.settings.SourceApp = choice.Source.ID
				a.processes = a.sourceProcesses[choice.Source.ID]
				a.selectedPID = choice.Process.PID
				a.statusMessage = ""
				a.statusError = false
				a.persistSettings()
				a.updateTrayIcon(a.recorder.Snapshot())
			}
		}
		a.closeMenus()
		a.invalidate()
		return true
	}
	if a.showFormatMenu {
		top := int32(307)
		if a.showSettings {
			top = 159
		}
		if x >= 18 && x <= 188 && y >= top && y < top+76 {
			if y < top+38 {
				a.settings.Format = config.FormatMP3
			} else {
				a.settings.Format = config.FormatWAV
			}
			a.persistSettings()
		}
		a.closeMenus()
		a.invalidate()
		return true
	}
	if a.showQualityMenu {
		top := int32(255)
		if a.showSettings {
			top = 220
		}
		if x >= 196 && x <= 372 && y >= top && y < top+120 {
			options := []int{320, 256, 192, 128}
			index := int((y - top) / 30)
			if index >= 0 && index < len(options) {
				a.settings.BitrateKbps = options[index]
				a.persistSettings()
			}
		}
		a.closeMenus()
		a.invalidate()
		return true
	}
	return false
}

func (a *application) onMainClick(x, y int32) {
	if a.isRecordingLocked() {
		if x >= 16 && x <= 374 && y >= 395 && y <= 438 && !a.busy {
			a.stopRecording("user")
		}
		return
	}
	if x >= 16 && x <= 374 && y >= 96 && y <= 220 {
		a.showSourceMenu = true
		a.invalidate()
		return
	}
	if x >= 16 && x <= 187 && y >= 251 && y <= 303 {
		a.showFormatMenu = true
		a.invalidate()
		return
	}
	if x >= 195 && x <= 374 && y >= 251 && y <= 303 && a.settings.Format == config.FormatMP3 {
		a.showQualityMenu = true
		a.invalidate()
		return
	}
	if x >= 16 && x <= 374 && y >= 318 && y <= 373 {
		a.chooseOutputFolder()
		return
	}
	if x >= 16 && x <= 374 && y >= 395 && y <= 438 && !a.busy && a.selectedPID != 0 {
		a.startRecording()
	}
}

func (a *application) onSettingsClick(x, y int32) {
	if x >= 16 && x <= 374 && y >= 393 && y <= 435 {
		a.showSettings = false
		a.persistSettings()
		a.invalidate()
		return
	}
	if x >= 16 && x <= 374 && y >= 105 && y <= 157 {
		a.showFormatMenu = true
		a.invalidate()
		return
	}
	if x >= 16 && x <= 374 && y >= 166 && y <= 218 && a.settings.Format == config.FormatMP3 {
		a.showQualityMenu = true
		a.invalidate()
		return
	}
	switch {
	case y >= 247 && y <= 275:
		a.settings.StartWithWindows = !a.settings.StartWithWindows
		if err := win32.SetStartWithWindows(a.settings.StartWithWindows); err != nil {
			a.statusMessage = "Startup setting failed: " + err.Error()
			a.statusError = true
			a.settings.StartWithWindows = !a.settings.StartWithWindows
		}
	case y >= 281 && y <= 309:
		a.settings.KeepRunningInTray = !a.settings.KeepRunningInTray
	case y >= 315 && y <= 343:
		a.settings.NotifyWhenSaved = !a.settings.NotifyWhenSaved
	default:
		return
	}
	a.persistSettings()
	a.invalidate()
}

func (a *application) startRecording() {
	a.busy = true
	a.statusMessage = "Starting process-specific capture..."
	a.statusError = false
	options := recording.StartOptions{
		PID:          a.selectedPID,
		Format:       a.settings.Format,
		BitrateKbps:  a.settings.BitrateKbps,
		OutputFolder: a.settings.OutputFolder,
	}
	go func() {
		err := a.recorder.Start(options)
		a.setPending("start", err)
	}()
}

func (a *application) stopRecording(kind string) {
	a.busy = true
	a.statusMessage = "Finalizing recording..."
	a.statusError = false
	go func() {
		err := a.recorder.Stop()
		a.setPending(kind, err)
	}()
}

func (a *application) setPending(kind string, err error) {
	a.pendingMu.Lock()
	a.pending = &pendingResult{kind: kind, err: err}
	a.pendingMu.Unlock()
	postMessage(a.window, wmAppRefresh)
}

func (a *application) applyPending() {
	a.pendingMu.Lock()
	result := a.pending
	a.pending = nil
	a.pendingMu.Unlock()
	if result == nil {
		return
	}
	a.busy = false
	if result.err != nil {
		a.statusMessage = result.err.Error()
		a.statusError = true
		a.notify("Recording error", result.err.Error(), true)
	} else if result.kind == "start" {
		a.statusMessage = "Recording " + a.selectedSource().Display + " audio"
		a.statusError = false
	} else {
		snapshot := a.recorder.Snapshot()
		if result.kind == "lost" {
			a.statusMessage = a.selectedSource().Display + " exited - recording finalized"
		} else {
			a.statusMessage = "Saved successfully"
		}
		a.statusError = false
		if a.settings.NotifyWhenSaved && snapshot.LastFile != "" {
			a.notify("Recording saved", filepath.Base(snapshot.LastFile), false)
		}
	}
	if a.exiting {
		status := a.recorder.Snapshot().Status
		if status == recording.StatusStarting || status == recording.StatusRecording || status == recording.StatusFinalizing {
			if !a.busy {
				a.stopRecording("exit")
			}
		} else {
			postMessage(a.window, wmAppExitReady)
		}
	}
	a.invalidate()
}

func (a *application) chooseOutputFolder() {
	folder, selected, err := win32.BrowseForFolder(a.window, "Choose where AmpRec saves recordings")
	if err != nil {
		a.statusMessage = "Folder selection failed: " + err.Error()
		a.statusError = true
	} else if selected {
		a.settings.OutputFolder = folder
		a.persistSettings()
	}
	a.invalidate()
}

func (a *application) persistSettings() {
	if err := config.Save(a.settings); err != nil {
		a.statusMessage = "Could not save settings: " + err.Error()
		a.statusError = true
	}
}

func (a *application) requestClose() {
	if a.settings.KeepRunningInTray && !a.exiting {
		procShowWindow.Call(uintptr(a.window), swHide)
		return
	}
	a.exiting = true
	snapshot := a.recorder.Snapshot()
	if snapshot.Status == recording.StatusRecording || snapshot.Status == recording.StatusStarting || snapshot.Status == recording.StatusFinalizing {
		if !a.busy {
			a.stopRecording("exit")
		}
		return
	}
	procDestroyWindow.Call(uintptr(a.window))
}

func (a *application) isRecordingLocked() bool {
	status := a.recorder.Snapshot().Status
	return status == recording.StatusStarting || status == recording.StatusRecording || status == recording.StatusFinalizing
}

func (a *application) closeMenus() {
	a.showFormatMenu = false
	a.showQualityMenu = false
	a.showSourceMenu = false
}

func (a *application) scale(value int32) int32 {
	return value * int32(a.dpi) / 96
}

func (a *application) toBase(value int32) int32 {
	return value * 96 / int32(a.dpi)
}

func (a *application) invalidate() {
	procInvalidateRect.Call(uintptr(a.window), 0, 0)
}

func (a *application) cleanup() {
	if a.window != 0 {
		procKillTimer.Call(uintptr(a.window), timerID)
		a.removeTrayIcon()
		applications.Delete(a.window)
	}
}

func selectedProcess(processes []win32.ProcessInfo, pid uint32) (win32.ProcessInfo, bool) {
	for _, process := range processes {
		if process.PID == pid {
			return process, true
		}
	}
	return win32.ProcessInfo{}, false
}

func shortenPath(path string, limit int) string {
	if len([]rune(path)) <= limit {
		return path
	}
	runes := []rune(path)
	return string(runes[:8]) + "..." + string(runes[len(runes)-(limit-11):])
}

func (a *application) executableFolder() string {
	path := a.settings.OutputFolder
	if path == "" {
		path, _ = os.Getwd()
	}
	return path
}

func (a *application) openRecordingsFolder() {
	if err := os.MkdirAll(a.executableFolder(), 0o700); err != nil {
		a.statusMessage = "Could not create output folder: " + err.Error()
		a.statusError = true
		return
	}
	if err := win32.OpenFolder(a.executableFolder()); err != nil {
		a.statusMessage = err.Error()
		a.statusError = true
	}
}

func (a *application) elapsed(snapshot recording.Snapshot) string {
	if snapshot.StartedAt.IsZero() {
		return "00:00:00"
	}
	duration := time.Since(snapshot.StartedAt)
	if duration < 0 {
		duration = 0
	}
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func (a *application) normalizedStatusMessage() string {
	message := strings.TrimSpace(a.statusMessage)
	if len(message) > 100 {
		message = message[:97] + "..."
	}
	return message
}
