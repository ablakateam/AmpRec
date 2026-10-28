//go:build windows

package ui

import (
	"amprec/internal/config"
	"amprec/internal/win32"
)

type sourceDefinition struct {
	ID         string
	Display    string
	Executable string
	Browser    bool
}

type sourceChoice struct {
	Source  sourceDefinition
	Process win32.ProcessInfo
}

var supportedSources = []sourceDefinition{
	{ID: config.SourceWinamp, Display: "Winamp", Executable: "winamp.exe"},
	{ID: config.SourceChrome, Display: "Google Chrome", Executable: "chrome.exe", Browser: true},
	{ID: config.SourceEdge, Display: "Microsoft Edge", Executable: "msedge.exe", Browser: true},
	{ID: config.SourceFirefox, Display: "Mozilla Firefox", Executable: "firefox.exe", Browser: true},
	{ID: config.SourceBrave, Display: "Brave", Executable: "brave.exe", Browser: true},
}

func sourceByID(id string) sourceDefinition {
	for _, source := range supportedSources {
		if source.ID == id {
			return source
		}
	}
	return supportedSources[0]
}

func sourceExecutables() []string {
	result := make([]string, 0, len(supportedSources))
	for _, source := range supportedSources {
		result = append(result, source.Executable)
	}
	return result
}

func (a *application) selectedSource() sourceDefinition {
	return sourceByID(a.settings.SourceApp)
}

func (a *application) sourceChoices() []sourceChoice {
	var result []sourceChoice
	for _, source := range supportedSources {
		processes := a.sourceProcesses[source.ID]
		if len(processes) == 0 {
			result = append(result, sourceChoice{Source: source})
			continue
		}
		for _, process := range processes {
			result = append(result, sourceChoice{Source: source, Process: process})
		}
	}
	return result
}
