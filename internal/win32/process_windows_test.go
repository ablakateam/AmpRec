//go:build windows

package win32

import "testing"

func TestRootProcessesGroupsBrowserTrees(t *testing.T) {
	entries := []ProcessInfo{
		{PID: 10, ParentPID: 1, Name: "chrome.exe"},
		{PID: 11, ParentPID: 10, Name: "chrome.exe"},
		{PID: 12, ParentPID: 10, Name: "chrome.exe"},
		{PID: 20, ParentPID: 1, Name: "chrome.exe"},
		{PID: 21, ParentPID: 20, Name: "chrome.exe"},
		{PID: 30, ParentPID: 1, Name: "winamp.exe"},
		{PID: 40, ParentPID: 1, Name: "other.exe"},
	}
	result := rootProcesses(entries, []string{"chrome.exe", "winamp.exe", "firefox.exe"})
	chrome := result["chrome.exe"]
	if len(chrome) != 2 || chrome[0].PID != 10 || chrome[1].PID != 20 {
		t.Fatalf("unexpected Chrome roots: %+v", chrome)
	}
	if winamp := result["winamp.exe"]; len(winamp) != 1 || winamp[0].PID != 30 {
		t.Fatalf("unexpected Winamp roots: %+v", winamp)
	}
	if len(result["firefox.exe"]) != 0 {
		t.Fatalf("unexpected Firefox roots: %+v", result["firefox.exe"])
	}
}
