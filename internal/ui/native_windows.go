//go:build windows

package ui

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy         = 0x0002
	wmPaint           = 0x000F
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmEraseBkgnd      = 0x0014
	wmNCHitTest       = 0x0084
	wmCommand         = 0x0111
	wmTimer           = 0x0113
	wmKeyDown         = 0x0100
	wmLButtonUp       = 0x0202
	wmRButtonUp       = 0x0205
	wmContextMenu     = 0x007B
	wmDPICHanged      = 0x02E0
	wmApp             = 0x8000
	wmAppRefresh      = wmApp + 1
	wmAppExitReady    = wmApp + 2
	wmAppTray         = wmApp + 20

	wsPopup       = 0x80000000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000

	swHide    = 0
	swShow    = 5
	swRestore = 9

	htClient  = 1
	htCaption = 2

	csHRedraw = 0x0002
	csVRedraw = 0x0001

	pmRemove = 0x0001

	dtCenter      = 0x0001
	dtVCenter     = 0x0004
	dtSingleLine  = 0x0020
	dtEndEllipsis = 0x8000
	dtNoClip      = 0x0100
	dtNoPrefix    = 0x0800

	transparent = 1

	fwNormal   = 400
	fwSemiBold = 600
	fwBold     = 700

	psSolid   = 0
	nullBrush = 5
	srcCopy   = 0x00CC0020

	nimAdd        = 0x00000000
	nimModify     = 0x00000001
	nimDelete     = 0x00000002
	nimSetVersion = 0x00000004

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010
	nifGUID    = 0x00000020
	nifShowTip = 0x00000080

	niifInfo  = 0x00000001
	niifError = 0x00000003

	notifyIconVersion4 = 4

	mfString    = 0x00000000
	mfSeparator = 0x00000800
	mfGrayed    = 0x00000001

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	idiApplication = 32512
	idiError       = 32513
	idcArrow       = 32512

	colorWindow = 5

	vkTab    = 0x09
	vkReturn = 0x0D
	vkShift  = 0x10
	vkEscape = 0x1B
	vkSpace  = 0x20
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procUpdateWindow                  = user32.NewProc("UpdateWindow")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procBeginPaint                    = user32.NewProc("BeginPaint")
	procEndPaint                      = user32.NewProc("EndPaint")
	procGetClientRect                 = user32.NewProc("GetClientRect")
	procInvalidateRect                = user32.NewProc("InvalidateRect")
	procSetTimer                      = user32.NewProc("SetTimer")
	procKillTimer                     = user32.NewProc("KillTimer")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procIsWindowVisible               = user32.NewProc("IsWindowVisible")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procScreenToClient                = user32.NewProc("ScreenToClient")
	procGetKeyState                   = user32.NewProc("GetKeyState")
	procCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	procAppendMenuW                   = user32.NewProc("AppendMenuW")
	procTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	procDestroyMenu                   = user32.NewProc("DestroyMenu")
	procLoadCursorW                   = user32.NewProc("LoadCursorW")
	procLoadIconW                     = user32.NewProc("LoadIconW")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procFindWindowW                   = user32.NewProc("FindWindowW")

	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procCreatePen              = gdi32.NewProc("CreatePen")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procGetStockObject         = gdi32.NewProc("GetStockObject")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procCreateFontW            = gdi32.NewProc("CreateFontW")
	procMoveToEx               = gdi32.NewProc("MoveToEx")
	procLineTo                 = gdi32.NewProc("LineTo")
	procCreateRoundRectRgn     = gdi32.NewProc("CreateRoundRectRgn")
	procFillRgn                = gdi32.NewProc("FillRgn")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procBitBlt                 = gdi32.NewProc("BitBlt")

	procFillRect  = user32.NewProc("FillRect")
	procDrawTextW = user32.NewProc("DrawTextW")

	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procShellNotifyIconW      = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW      = kernel32.NewProc("GetModuleHandleW")
)

type point struct {
	X int32
	Y int32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type message struct {
	HWnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}

type paintStruct struct {
	HDC       windows.Handle
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  windows.Handle
}

type notifyIconData struct {
	Size             uint32
	Window           windows.Handle
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             windows.Handle
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GUID             windows.GUID
	BalloonIcon      windows.Handle
}

func rgb(red, green, blue byte) uintptr {
	return uintptr(uint32(red) | uint32(green)<<8 | uint32(blue)<<16)
}

func utf16(value string) []uint16 {
	encoded, _ := windows.UTF16FromString(value)
	return encoded
}

func copyUTF16(destination []uint16, value string) {
	encoded := utf16(value)
	if len(encoded) > len(destination) {
		encoded = encoded[:len(destination)]
		encoded[len(encoded)-1] = 0
	}
	copy(destination, encoded)
}

func defWindowProc(window windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	result, _, _ := procDefWindowProcW.Call(uintptr(window), uintptr(message), wParam, lParam)
	return result
}

func drawText(hdc windows.Handle, value string, area rect, color uintptr, font windows.Handle, format uint32) {
	if value == "" {
		return
	}
	text := utf16(value)
	procSetBkMode.Call(uintptr(hdc), transparent)
	procSetTextColor.Call(uintptr(hdc), color)
	old, _, _ := procSelectObject.Call(uintptr(hdc), uintptr(font))
	procDrawTextW.Call(uintptr(hdc), uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1), uintptr(unsafe.Pointer(&area)), uintptr(format|dtNoPrefix))
	procSelectObject.Call(uintptr(hdc), old)
}

func createFont(pixelHeight int32, weight int32) windows.Handle {
	face := utf16("Segoe UI")
	handle, _, _ := procCreateFontW.Call(
		uintptr(-pixelHeight), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1, 0, 0, 5, 0, uintptr(unsafe.Pointer(&face[0])),
	)
	return windows.Handle(handle)
}

func fillRect(hdc windows.Handle, area rect, color uintptr) {
	brush, _, _ := procCreateSolidBrush.Call(color)
	procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&area)), brush)
	procDeleteObject.Call(brush)
}

func fillRoundedRect(hdc windows.Handle, area rect, radius int32, color uintptr) {
	region, _, _ := procCreateRoundRectRgn.Call(uintptr(area.Left), uintptr(area.Top), uintptr(area.Right+1), uintptr(area.Bottom+1), uintptr(radius), uintptr(radius))
	brush, _, _ := procCreateSolidBrush.Call(color)
	procFillRgn.Call(uintptr(hdc), region, brush)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(region)
}

func drawLine(hdc windows.Handle, x1, y1, x2, y2 int32, color uintptr, width int32) {
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(width), color)
	old, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procMoveToEx.Call(uintptr(hdc), uintptr(x1), uintptr(y1), 0)
	procLineTo.Call(uintptr(hdc), uintptr(x2), uintptr(y2))
	procSelectObject.Call(uintptr(hdc), old)
	procDeleteObject.Call(pen)
}

func lowWordSigned(value uintptr) int32 {
	return int32(int16(uint16(value & 0xffff)))
}

func highWordSigned(value uintptr) int32 {
	return int32(int16(uint16((value >> 16) & 0xffff)))
}

func postMessage(window windows.Handle, message uint32) {
	procPostMessageW.Call(uintptr(window), uintptr(message), 0, 0)
}

func callWindowProc(callback func(uintptr, uintptr, uintptr, uintptr) uintptr) uintptr {
	return syscall.NewCallback(callback)
}
