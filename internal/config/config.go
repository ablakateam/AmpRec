package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	FormatMP3 = "MP3"
	FormatWAV = "WAV"

	SourceWinamp  = "winamp"
	SourceChrome  = "chrome"
	SourceEdge    = "edge"
	SourceFirefox = "firefox"
	SourceBrave   = "brave"
)

type Settings struct {
	Format            string `json:"format"`
	BitrateKbps       int    `json:"bitrateKbps"`
	OutputFolder      string `json:"outputFolder"`
	StartWithWindows  bool   `json:"startWithWindows"`
	KeepRunningInTray bool   `json:"keepRunningInTray"`
	NotifyWhenSaved   bool   `json:"notifyWhenSaved"`
	SourceApp         string `json:"sourceApp"`
}

func Default() Settings {
	music, err := os.UserHomeDir()
	if err == nil {
		music = filepath.Join(music, "Music", "AmpRec")
	} else {
		music = filepath.Join(os.TempDir(), "AmpRec")
	}
	if override := os.Getenv("AMPREC_OUTPUT_DIR"); override != "" {
		music = override
	}
	return Settings{
		Format:            FormatMP3,
		BitrateKbps:       320,
		OutputFolder:      music,
		KeepRunningInTray: true,
		NotifyWhenSaved:   true,
		SourceApp:         SourceWinamp,
	}
}

func Dir() (string, error) {
	if override := os.Getenv("AMPREC_CONFIG_DIR"); override != "" {
		return filepath.Abs(override)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "AmpRec"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func Load() (Settings, error) {
	settings := Default()
	path, err := Path()
	if err != nil {
		return settings, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return Default(), err
	}
	settings.normalize()
	return settings, nil
}

func (s *Settings) normalize() {
	if s.Format != FormatMP3 && s.Format != FormatWAV {
		s.Format = FormatMP3
	}
	switch s.BitrateKbps {
	case 128, 192, 256, 320:
	default:
		s.BitrateKbps = 320
	}
	if s.OutputFolder == "" {
		s.OutputFolder = Default().OutputFolder
	}
	switch s.SourceApp {
	case SourceWinamp, SourceChrome, SourceEdge, SourceFirefox, SourceBrave:
	default:
		s.SourceApp = SourceWinamp
	}
}

func Save(settings Settings) error {
	settings.normalize()
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "settings.json.tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return replaceFile(tmp, filepath.Join(dir, "settings.json"))
}

func replaceFile(source, destination string) error {
	_ = os.Remove(destination)
	return os.Rename(source, destination)
}
