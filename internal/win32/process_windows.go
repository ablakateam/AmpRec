//go:build windows

package win32

import (
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type ProcessInfo struct {
	PID       uint32
	ParentPID uint32
	Name      string
}

func FindProcessesByName(executable string) ([]ProcessInfo, error) {
	entries, err := enumerateProcesses()
	if err != nil {
		return nil, err
	}
	var result []ProcessInfo
	for _, entry := range entries {
		if strings.EqualFold(entry.Name, executable) {
			result = append(result, entry)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })
	return result, nil
}

// FindRootProcessesByNames returns one target per independent application tree.
// Chromium and Firefox render audio in child processes; targeting the root with
// PROCESS_LOOPBACK_MODE_INCLUDE_TARGET_PROCESS_TREE captures those children.
func FindRootProcessesByNames(executables []string) (map[string][]ProcessInfo, error) {
	entries, err := enumerateProcesses()
	if err != nil {
		return nil, err
	}
	return rootProcesses(entries, executables), nil
}

func enumerateProcesses() ([]ProcessInfo, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ProcessEntry32{Size: uint32(unsafeSizeofProcessEntry32())}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}
	var result []ProcessInfo
	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		result = append(result, ProcessInfo{PID: entry.ProcessID, ParentPID: entry.ParentProcessID, Name: name})
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				break
			}
			return nil, err
		}
	}
	return result, nil
}

func rootProcesses(entries []ProcessInfo, executables []string) map[string][]ProcessInfo {
	wanted := make(map[string]bool, len(executables))
	for _, executable := range executables {
		wanted[strings.ToLower(executable)] = true
	}
	grouped := make(map[string][]ProcessInfo, len(executables))
	ids := make(map[string]map[uint32]bool, len(executables))
	for _, entry := range entries {
		name := strings.ToLower(entry.Name)
		if !wanted[name] {
			continue
		}
		grouped[name] = append(grouped[name], entry)
		if ids[name] == nil {
			ids[name] = make(map[uint32]bool)
		}
		ids[name][entry.PID] = true
	}
	result := make(map[string][]ProcessInfo, len(executables))
	for name, candidates := range grouped {
		for _, candidate := range candidates {
			if !ids[name][candidate.ParentPID] {
				result[name] = append(result[name], candidate)
			}
		}
		sort.Slice(result[name], func(i, j int) bool { return result[name][i].PID < result[name][j].PID })
	}
	for executable := range wanted {
		if result[executable] == nil {
			result[executable] = []ProcessInfo{}
		}
	}
	return result
}

func ProcessExists(pid uint32) bool {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	var code uint32
	if err := windows.GetExitCodeProcess(process, &code); err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

// Kept as a function so ProcessEntry32.Size remains correct across architectures.
func unsafeSizeofProcessEntry32() uintptr {
	var entry windows.ProcessEntry32
	return unsafe.Sizeof(entry)
}
