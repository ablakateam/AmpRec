//go:build windows

package ui

import (
	"testing"

	"amprec/internal/config"
	"amprec/internal/win32"
)

func TestSourceChoicesIncludeUnavailableBrowsersAndRunningRoots(t *testing.T) {
	app := &application{
		settings: config.Settings{SourceApp: config.SourceChrome},
		sourceProcesses: map[string][]win32.ProcessInfo{
			config.SourceChrome: {{PID: 20, Name: "chrome.exe"}},
		},
	}
	choices := app.sourceChoices()
	if len(choices) != len(supportedSources) {
		t.Fatalf("got %d choices, want %d: %+v", len(choices), len(supportedSources), choices)
	}
	if choices[1].Source.ID != config.SourceChrome || choices[1].Process.PID != 20 {
		t.Fatalf("Chrome root was not exposed: %+v", choices[1])
	}
	if choices[2].Source.ID != config.SourceEdge || choices[2].Process.PID != 0 {
		t.Fatalf("unavailable Edge choice was not exposed: %+v", choices[2])
	}
}
