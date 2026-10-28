//go:build windows

// windowcheck captures and exercises AmpRec's custom Win32 surface for integration tests.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmClose       = 0x0010
	wmCommand     = 0x0111
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmKeyDown     = 0x0100
	wmKeyUp       = 0x0101
	wmAppTray     = 0x8000 + 20
	srcCopy       = 0x00CC0020
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	findWindow             = user32.NewProc("FindWindowW")
	getClientRect          = user32.NewProc("GetClientRect")
	getDpiForWindow        = user32.NewProc("GetDpiForWindow")
	isWindowVisible        = user32.NewProc("IsWindowVisible")
	postMessage            = user32.NewProc("PostMessageW")
	printWindow            = user32.NewProc("PrintWindow")
	setProcessDPIContext   = user32.NewProc("SetProcessDpiAwarenessContext")
	getDC                  = user32.NewProc("GetDC")
	releaseDC              = user32.NewProc("ReleaseDC")
	createCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	createCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	selectObject           = gdi32.NewProc("SelectObject")
	deleteObject           = gdi32.NewProc("DeleteObject")
	deleteDC               = gdi32.NewProc("DeleteDC")
	getDIBits              = gdi32.NewProc("GetDIBits")
)

type rect struct{ Left, Top, Right, Bottom int32 }
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type result struct {
	Found      bool   `json:"found"`
	Visible    bool   `json:"visible"`
	Width      int32  `json:"width"`
	Height     int32  `json:"height"`
	DPI        uint32 `json:"dpi"`
	Screenshot string `json:"screenshot,omitempty"`
}

func main() {
	setProcessDPIContext.Call(^uintptr(3))
	output := flag.String("output", "", "PNG output path")
	click := flag.String("click", "", "client coordinate x,y")
	closeWindow := flag.Bool("close", false, "send WM_CLOSE")
	trayToggle := flag.Bool("tray-toggle", false, "send the tray left-click callback")
	exitApp := flag.Bool("exit", false, "request a graceful tray Exit")
	key := flag.String("key", "", "post Tab, Return, Space, or Escape")
	wait := flag.Duration("wait", 300*time.Millisecond, "wait after action")
	flag.Parse()
	className, _ := windows.UTF16PtrFromString("AmpRec.MainWindow.v1")
	hwnd, _, _ := findWindow.Call(uintptr(unsafe.Pointer(className)), 0)
	if hwnd == 0 {
		writeResult(result{})
		os.Exit(2)
	}
	dpiResult, _, _ := getDpiForWindow.Call(hwnd)
	dpi := uint32(dpiResult)
	if dpi == 0 {
		dpi = 96
	}
	if *click != "" {
		parts := strings.Split(*click, ",")
		if len(parts) != 2 {
			fatal("click must be x,y")
		}
		x, err := strconv.Atoi(parts[0])
		if err != nil {
			fatal(err.Error())
		}
		y, err := strconv.Atoi(parts[1])
		if err != nil {
			fatal(err.Error())
		}
		x = x * int(dpi) / 96
		y = y * int(dpi) / 96
		position := uintptr(uint32(uint16(x)) | uint32(uint16(y))<<16)
		postMessage.Call(hwnd, wmLButtonDown, 1, position)
		postMessage.Call(hwnd, wmLButtonUp, 0, position)
	}
	if *closeWindow {
		postMessage.Call(hwnd, wmClose, 0, 0)
	}
	if *trayToggle {
		postMessage.Call(hwnd, wmAppTray, 0, wmLButtonUp)
	}
	if *exitApp {
		postMessage.Call(hwnd, wmCommand, 1005, 0)
	}
	if *key != "" {
		keys := map[string]uintptr{"Tab": 0x09, "Return": 0x0D, "Space": 0x20, "Escape": 0x1B}
		code, ok := keys[*key]
		if !ok {
			fatal("key must be Tab, Return, Space, or Escape")
		}
		postMessage.Call(hwnd, wmKeyDown, code, 0)
		postMessage.Call(hwnd, wmKeyUp, code, 0)
	}
	if *click != "" || *closeWindow || *trayToggle || *exitApp || *key != "" {
		time.Sleep(*wait)
	}
	if *exitApp {
		hwnd, _, _ = findWindow.Call(uintptr(unsafe.Pointer(className)), 0)
		if hwnd == 0 {
			writeResult(result{})
			return
		}
	}
	var client rect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	visible, _, _ := isWindowVisible.Call(hwnd)
	state := result{Found: true, Visible: visible != 0, Width: client.Right, Height: client.Bottom, DPI: dpi}
	if *output != "" && visible != 0 {
		if err := capture(hwnd, client.Right, client.Bottom, *output); err != nil {
			fatal(err.Error())
		}
		state.Screenshot = *output
	}
	writeResult(state)
}

func capture(hwnd uintptr, width, height int32, output string) error {
	windowDC, _, _ := getDC.Call(hwnd)
	if windowDC == 0 {
		return fmt.Errorf("GetDC failed")
	}
	defer releaseDC.Call(hwnd, windowDC)
	memoryDC, _, _ := createCompatibleDC.Call(windowDC)
	if memoryDC == 0 {
		return fmt.Errorf("CreateCompatibleDC failed")
	}
	defer deleteDC.Call(memoryDC)
	bitmap, _, _ := createCompatibleBitmap.Call(windowDC, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer deleteObject.Call(bitmap)
	old, _, _ := selectObject.Call(memoryDC, bitmap)
	defer selectObject.Call(memoryDC, old)
	ok, _, _ := printWindow.Call(hwnd, memoryDC, 3)
	if ok == 0 {
		return fmt.Errorf("PrintWindow failed")
	}
	header := bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: width, Height: -height, Planes: 1, BitCount: 32}
	pixels := make([]byte, int(width*height*4))
	lines, _, _ := getDIBits.Call(memoryDC, bitmap, 0, uintptr(height), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&header)), 0)
	if lines == 0 {
		return fmt.Errorf("GetDIBits failed")
	}
	imageData := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	for y := 0; y < int(height); y++ {
		for x := 0; x < int(width); x++ {
			offset := (y*int(width) + x) * 4
			imageData.SetRGBA(x, y, color.RGBA{R: pixels[offset+2], G: pixels[offset+1], B: pixels[offset], A: 255})
		}
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, imageData)
}

func writeResult(value result) {
	data, _ := json.Marshal(value)
	fmt.Println(string(data))
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
