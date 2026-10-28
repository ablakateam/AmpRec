//go:build windows

package ui

import (
	"fmt"
	"math"
	"strings"
	"unsafe"

	"amprec/internal/config"
	"amprec/internal/recording"
	"golang.org/x/sys/windows"
)

var palette = struct {
	background uintptr
	panel      uintptr
	panelLight uintptr
	border     uintptr
	text       uintptr
	muted      uintptr
	aqua       uintptr
	aquaDark   uintptr
	red        uintptr
	amber      uintptr
	green      uintptr
	disabled   uintptr
}{
	background: rgb(13, 24, 32),
	panel:      rgb(24, 40, 51),
	panelLight: rgb(30, 51, 63),
	border:     rgb(49, 75, 88),
	text:       rgb(233, 244, 247),
	muted:      rgb(133, 159, 169),
	aqua:       rgb(75, 219, 190),
	aquaDark:   rgb(31, 105, 102),
	red:        rgb(255, 87, 108),
	amber:      rgb(238, 177, 81),
	green:      rgb(76, 218, 163),
	disabled:   rgb(82, 105, 115),
}

type painter struct {
	app      *application
	hdc      windows.Handle
	regular  windows.Handle
	small    windows.Handle
	semibold windows.Handle
	title    windows.Handle
	tiny     windows.Handle
}

func (a *application) paint() {
	var paint paintStruct
	windowDC, _, _ := procBeginPaint.Call(uintptr(a.window), uintptr(unsafe.Pointer(&paint)))
	if windowDC == 0 {
		return
	}
	defer procEndPaint.Call(uintptr(a.window), uintptr(unsafe.Pointer(&paint)))

	var client rect
	procGetClientRect.Call(uintptr(a.window), uintptr(unsafe.Pointer(&client)))
	memoryDC, _, _ := procCreateCompatibleDC.Call(windowDC)
	bitmap, _, _ := procCreateCompatibleBitmap.Call(windowDC, uintptr(client.Right), uintptr(client.Bottom))
	oldBitmap, _, _ := procSelectObject.Call(memoryDC, bitmap)
	defer func() {
		procSelectObject.Call(memoryDC, oldBitmap)
		procDeleteObject.Call(bitmap)
		procDeleteDC.Call(memoryDC)
	}()

	p := &painter{
		app:      a,
		hdc:      windows.Handle(memoryDC),
		regular:  createFont(a.scale(12), fwNormal),
		small:    createFont(a.scale(10), fwNormal),
		semibold: createFont(a.scale(12), fwSemiBold),
		title:    createFont(a.scale(18), fwBold),
		tiny:     createFont(a.scale(8), fwSemiBold),
	}
	defer p.close()
	fillRect(p.hdc, client, palette.background)
	p.drawHeader()
	if a.showSettings {
		p.drawSettings()
	} else {
		p.drawMain()
	}
	p.drawOverlays()
	procBitBlt.Call(windowDC, 0, 0, uintptr(client.Right), uintptr(client.Bottom), memoryDC, 0, 0, srcCopy)
}

func (p *painter) close() {
	for _, object := range []windows.Handle{p.regular, p.small, p.semibold, p.title, p.tiny} {
		if object != 0 {
			procDeleteObject.Call(uintptr(object))
		}
	}
}

func (p *painter) r(x, y, width, height int32) rect {
	return rect{
		Left: p.app.scale(x), Top: p.app.scale(y),
		Right: p.app.scale(x + width), Bottom: p.app.scale(y + height),
	}
}

func (p *painter) text(value string, area rect, color uintptr, font windows.Handle, format uint32) {
	drawText(p.hdc, value, area, color, font, format)
}

func (p *painter) panel(area rect, radius int32, color uintptr) {
	fillRoundedRect(p.hdc, area, p.app.scale(radius), color)
}

func (p *painter) focusRect(area rect) {
	if !p.app.keyboardMode {
		return
	}
	drawLine(p.hdc, area.Left, area.Top, area.Right, area.Top, palette.aqua, p.app.scale(2))
	drawLine(p.hdc, area.Right, area.Top, area.Right, area.Bottom, palette.aqua, p.app.scale(2))
	drawLine(p.hdc, area.Right, area.Bottom, area.Left, area.Bottom, palette.aqua, p.app.scale(2))
	drawLine(p.hdc, area.Left, area.Bottom, area.Left, area.Top, palette.aqua, p.app.scale(2))
}

func (p *painter) drawHeader() {
	p.panel(p.r(16, 15, 28, 28), 14, palette.panelLight)
	for index, height := range []int32{6, 11, 16, 10, 5} {
		x := p.app.scale(23 + int32(index)*3)
		center := p.app.scale(29)
		drawLine(p.hdc, x, center-p.app.scale(height)/2, x, center+p.app.scale(height)/2, palette.aqua, p.app.scale(2))
	}
	p.text("AmpRec", p.r(53, 13, 170, 24), palette.text, p.title, dtSingleLine|dtVCenter)
	p.text("APP AUDIO RECORDER", p.r(54, 36, 180, 10), palette.muted, p.tiny, dtSingleLine)

	// Settings sliders.
	for index, offset := range []int32{0, 5, 10} {
		y := p.app.scale(19 + offset)
		drawLine(p.hdc, p.app.scale(327), y, p.app.scale(341), y, palette.aqua, p.app.scale(1))
		knob := int32(331 + (index%2)*6)
		p.panel(p.r(knob, 17+offset, 3, 4), 2, palette.aqua)
	}
	drawLine(p.hdc, p.app.scale(363), p.app.scale(21), p.app.scale(373), p.app.scale(31), palette.muted, p.app.scale(1))
	drawLine(p.hdc, p.app.scale(373), p.app.scale(21), p.app.scale(363), p.app.scale(31), palette.muted, p.app.scale(1))
	drawLine(p.hdc, p.app.scale(16), p.app.scale(55), p.app.scale(374), p.app.scale(55), palette.border, p.app.scale(1))
}

func (p *painter) drawMain() {
	a := p.app
	snapshot := a.recorder.Snapshot()
	source := a.selectedSource()
	process, found := selectedProcess(a.processes, a.selectedPID)
	statusColor := palette.green
	status := strings.ToUpper(source.Display) + " CONNECTED"
	if snapshot.Status == recording.StatusRecording {
		statusColor = palette.red
		status = "RECORDING"
	} else if snapshot.Status == recording.StatusStarting {
		statusColor = palette.amber
		status = "STARTING CAPTURE"
	} else if snapshot.Status == recording.StatusFinalizing {
		statusColor = palette.amber
		status = "FINALIZING"
	} else if a.statusError || snapshot.Status == recording.StatusError {
		statusColor = palette.red
		status = "RECORDING ERROR"
	} else if !found {
		statusColor = palette.amber
		status = strings.ToUpper(source.Display) + " NOT FOUND"
	} else if strings.Contains(strings.ToLower(a.statusMessage), "saved") {
		statusColor = palette.green
		status = "SAVED SUCCESSFULLY"
	}
	p.panel(p.r(17, 69, 7, 7), 4, statusColor)
	p.text(status, p.r(29, 62, 220, 20), statusColor, p.tiny, dtSingleLine|dtVCenter)
	if snapshot.Status == recording.StatusRecording || snapshot.Status == recording.StatusFinalizing {
		p.text(a.elapsed(snapshot), p.r(278, 62, 96, 20), palette.text, p.semibold, dtSingleLine|dtVCenter|dtCenter)
	} else if message := a.normalizedStatusMessage(); message != "" {
		p.text(message, p.r(165, 62, 209, 20), statusColor, p.small, dtSingleLine|dtVCenter|dtEndEllipsis)
	}

	p.panel(p.r(16, 93, 358, 128), 8, palette.panel)
	p.text("AUDIO SOURCE", p.r(27, 101, 180, 16), palette.muted, p.tiny, dtSingleLine|dtVCenter)
	if found {
		p.text(source.Display, p.r(27, 117, 180, 22), palette.text, p.semibold, dtSingleLine|dtVCenter)
		p.text(fmt.Sprintf("%s  -  PID %d", process.Name, process.PID), p.r(181, 117, 162, 22), palette.muted, p.small, dtSingleLine|dtVCenter|dtEndEllipsis)
	} else {
		p.text(source.Display, p.r(27, 117, 180, 22), palette.text, p.semibold, dtSingleLine|dtVCenter)
		p.text("Waiting for "+source.Display+"...", p.r(27, 140, 285, 20), palette.muted, p.small, dtSingleLine|dtVCenter|dtEndEllipsis)
	}
	if !a.isRecordingLocked() {
		drawLine(p.hdc, p.app.scale(349), p.app.scale(122), p.app.scale(353), p.app.scale(126), palette.muted, p.app.scale(1))
		drawLine(p.hdc, p.app.scale(353), p.app.scale(126), p.app.scale(357), p.app.scale(122), palette.muted, p.app.scale(1))
	}
	p.drawMeter(snapshot, found)

	p.text("OUTPUT SETTINGS", p.r(17, 229, 250, 15), palette.muted, p.tiny, dtSingleLine|dtVCenter)
	p.drawSelectCard(16, 248, 171, 54, "FORMAT", a.settings.Format, true)
	quality := fmt.Sprintf("%d kbps", a.settings.BitrateKbps)
	qualityEnabled := a.settings.Format == config.FormatMP3
	if !qualityEnabled {
		quality = "16-bit PCM"
	}
	p.drawSelectCard(195, 248, 179, 54, "QUALITY", quality, qualityEnabled)

	p.text("SAVE TO", p.r(17, 308, 100, 14), palette.muted, p.tiny, dtSingleLine|dtVCenter)
	p.panel(p.r(16, 324, 358, 54), 7, palette.panel)
	p.text(shortenPath(a.settings.OutputFolder, 46), p.r(28, 328, 299, 45), palette.text, p.small, dtSingleLine|dtVCenter|dtEndEllipsis)
	p.panel(p.r(338, 340, 24, 21), 6, palette.panelLight)
	p.text("...", p.r(338, 334, 24, 24), palette.muted, p.semibold, dtCenter|dtVCenter|dtSingleLine)

	buttonColor := palette.aqua
	buttonText := "START RECORDING"
	textColor := palette.background
	if snapshot.Status == recording.StatusRecording {
		buttonColor = palette.red
		buttonText = "STOP RECORDING"
	} else if snapshot.Status == recording.StatusFinalizing {
		buttonColor = palette.disabled
		buttonText = "FINALIZING..."
	} else if snapshot.Status == recording.StatusStarting || a.busy {
		buttonColor = palette.disabled
		buttonText = "STARTING..."
	} else if !found {
		buttonColor = palette.disabled
		buttonText = "START RECORDING"
		textColor = palette.muted
	}
	p.panel(p.r(16, 397, 358, 39), 7, buttonColor)
	indicatorColor := palette.background
	if textColor == palette.muted {
		indicatorColor = palette.muted
	}
	p.panel(p.r(40, 413, 6, 6), 3, indicatorColor)
	p.text(buttonText, p.r(65, 397, 260, 39), textColor, p.semibold, dtCenter|dtVCenter|dtSingleLine)
	if a.keyboardMode {
		areas := []rect{p.r(318, 12, 36, 40), p.r(16, 93, 358, 128), p.r(16, 248, 171, 54), p.r(195, 248, 179, 54), p.r(16, 324, 358, 54), p.r(16, 397, 358, 39)}
		if a.focus >= 0 && a.focus < len(areas) {
			p.focusRect(areas[a.focus])
		}
	}
}

func (p *painter) drawSelectCard(x, y, width, height int32, label, value string, enabled bool) {
	color := palette.panel
	valueColor := palette.text
	if !enabled {
		color = rgb(21, 35, 44)
		valueColor = palette.disabled
	}
	p.panel(p.r(x, y, width, height), 7, color)
	p.text(label, p.r(x+11, y+6, width-25, 14), palette.muted, p.tiny, dtSingleLine|dtVCenter)
	p.text(value, p.r(x+11, y+23, width-35, 24), valueColor, p.semibold, dtSingleLine|dtVCenter|dtEndEllipsis)
	if enabled {
		drawLine(p.hdc, p.app.scale(x+width-22), p.app.scale(y+30), p.app.scale(x+width-18), p.app.scale(y+34), palette.muted, p.app.scale(1))
		drawLine(p.hdc, p.app.scale(x+width-18), p.app.scale(y+34), p.app.scale(x+width-14), p.app.scale(y+30), palette.muted, p.app.scale(1))
	}
}

func (p *painter) drawMeter(snapshot recording.Snapshot, sourceAvailable bool) {
	baseLevel := float64(snapshot.Level)
	if snapshot.Status != recording.StatusRecording {
		baseLevel = 0.20
	}
	if !sourceAvailable {
		baseLevel = 0.03
	}
	for index := 0; index < 48; index++ {
		wave := 0.30 + 0.70*math.Abs(math.Sin(float64(index)*0.73+float64(p.app.tick)*0.12))
		height := int32(3 + 39*math.Min(1, baseLevel*wave*2.1))
		if snapshot.Status != recording.StatusRecording && sourceAvailable {
			height = int32(5 + 18*wave)
		}
		x := int32(27 + index*7)
		color := palette.aquaDark
		if float64(index)/48 < baseLevel*1.2 || snapshot.Status != recording.StatusRecording {
			color = palette.aqua
		}
		if !sourceAvailable {
			color = palette.disabled
		}
		p.panel(p.r(x, 190-height, 3, height), 2, color)
	}
}

func (p *painter) drawSettings() {
	a := p.app
	p.text("Settings", p.r(16, 67, 220, 28), palette.text, p.title, dtSingleLine|dtVCenter)
	p.text("DEFAULT RECORDING", p.r(17, 94, 200, 13), palette.muted, p.tiny, dtSingleLine)
	p.drawSelectCard(16, 107, 358, 50, "FORMAT", a.settings.Format, true)
	quality := fmt.Sprintf("%d kbps - Stereo", a.settings.BitrateKbps)
	qualityEnabled := a.settings.Format == config.FormatMP3
	if !qualityEnabled {
		quality = "16-bit stereo PCM"
	}
	p.drawSelectCard(16, 168, 358, 50, "QUALITY", quality, qualityEnabled)
	p.text("STARTUP & BEHAVIOR", p.r(17, 231, 200, 13), palette.muted, p.tiny, dtSingleLine)
	p.drawCheckbox(16, 247, "Start with Windows", a.settings.StartWithWindows)
	p.drawCheckbox(16, 281, "Keep running in system tray", a.settings.KeepRunningInTray)
	p.drawCheckbox(16, 315, "Notify when file is saved", a.settings.NotifyWhenSaved)
	p.text("Settings are stored locally. AmpRec has no telemetry or cloud service.", p.r(17, 354, 355, 25), palette.muted, p.small, 0)
	p.panel(p.r(16, 395, 358, 39), 7, palette.aqua)
	p.text("Done", p.r(16, 395, 358, 39), palette.background, p.semibold, dtCenter|dtVCenter|dtSingleLine)
	if a.keyboardMode {
		areas := []rect{p.r(16, 107, 358, 50), p.r(16, 168, 358, 50), p.r(16, 247, 358, 28), p.r(16, 281, 358, 28), p.r(16, 315, 358, 28), p.r(16, 395, 358, 39)}
		if a.focus >= 0 && a.focus < len(areas) {
			p.focusRect(areas[a.focus])
		}
	}
}

func (p *painter) drawCheckbox(x, y int32, label string, checked bool) {
	color := palette.panelLight
	if checked {
		color = palette.aqua
	}
	p.panel(p.r(x, y+5, 16, 16), 4, color)
	if checked {
		drawLine(p.hdc, p.app.scale(x+4), p.app.scale(y+13), p.app.scale(x+7), p.app.scale(y+16), palette.background, p.app.scale(2))
		drawLine(p.hdc, p.app.scale(x+7), p.app.scale(y+16), p.app.scale(x+13), p.app.scale(y+9), palette.background, p.app.scale(2))
	}
	p.text(label, p.r(x+25, y, 320, 27), palette.text, p.regular, dtSingleLine|dtVCenter)
}

func (p *painter) drawOverlays() {
	a := p.app
	if a.showSourceMenu {
		choices := a.sourceChoices()
		rows := len(choices)
		if rows > 8 {
			rows = 8
		}
		height := int32(rows * 36)
		p.panel(p.r(20, 126, 350, height+7), 7, palette.panelLight)
		for index, choice := range choices {
			if index >= 8 {
				break
			}
			y := int32(129 + index*36)
			selected := choice.Source.ID == a.settings.SourceApp && choice.Process.PID == a.selectedPID
			if selected {
				p.panel(p.r(24, y, 342, 32), 5, palette.aquaDark)
			}
			label := choice.Source.Display
			detail := "not detected"
			detailColor := palette.muted
			if choice.Process.PID != 0 {
				detail = fmt.Sprintf("PID %d", choice.Process.PID)
				detailColor = palette.text
			}
			p.text(label, p.r(34, y, 205, 32), palette.text, p.semibold, dtSingleLine|dtVCenter|dtEndEllipsis)
			p.text(detail, p.r(220, y, 112, 32), detailColor, p.small, dtSingleLine|dtVCenter|dtEndEllipsis)
			if selected {
				p.drawCheckmark(347, y+16, palette.aqua)
			}
		}
	}
	if a.showFormatMenu {
		top := int32(307)
		if a.showSettings {
			top = 159
		}
		p.panel(p.r(18, top, 170, 76), 7, palette.panelLight)
		p.menuRow(22, top+3, 162, 34, "MP3", a.settings.Format == config.FormatMP3)
		p.menuRow(22, top+39, 162, 34, "WAV (PCM)", a.settings.Format == config.FormatWAV)
	}
	if a.showQualityMenu {
		top := int32(255)
		if a.showSettings {
			top = 220
		}
		p.panel(p.r(196, top, 176, 122), 7, palette.panelLight)
		for index, bitrate := range []int{320, 256, 192, 128} {
			p.menuRow(200, top+int32(index*30)+2, 168, 28, fmt.Sprintf("%d kbps", bitrate), a.settings.BitrateKbps == bitrate)
		}
	}
}

func (p *painter) menuRow(x, y, width, height int32, label string, selected bool) {
	if selected {
		p.panel(p.r(x, y, width, height), 5, palette.aquaDark)
	}
	p.text(label, p.r(x+10, y, width-42, height), palette.text, p.regular, dtSingleLine|dtVCenter)
	if selected {
		p.drawCheckmark(x+width-22, y+height/2, palette.aqua)
	}
}

func (p *painter) drawCheckmark(centerX, centerY int32, color uintptr) {
	drawLine(p.hdc, p.app.scale(centerX-5), p.app.scale(centerY), p.app.scale(centerX-1), p.app.scale(centerY+4), color, p.app.scale(2))
	drawLine(p.hdc, p.app.scale(centerX-1), p.app.scale(centerY+4), p.app.scale(centerX+6), p.app.scale(centerY-5), color, p.app.scale(2))
}
