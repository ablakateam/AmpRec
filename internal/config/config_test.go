package config

import "testing"

func TestNormalizeSettings(t *testing.T) {
	s := Settings{Format: "OGG", BitrateKbps: 17}
	s.normalize()
	if s.Format != FormatMP3 || s.BitrateKbps != 320 || s.OutputFolder == "" || s.SourceApp != SourceWinamp {
		t.Fatalf("unexpected normalized settings: %+v", s)
	}
}

func TestValidSettingsRemainUnchanged(t *testing.T) {
	s := Settings{Format: FormatWAV, BitrateKbps: 192, OutputFolder: `C:\Music`, SourceApp: SourceChrome}
	s.normalize()
	if s.Format != FormatWAV || s.BitrateKbps != 192 || s.OutputFolder != `C:\Music` || s.SourceApp != SourceChrome {
		t.Fatalf("settings changed unexpectedly: %+v", s)
	}
}

func TestSupportedSourcesRemainSelected(t *testing.T) {
	for _, source := range []string{SourceWinamp, SourceChrome, SourceEdge, SourceFirefox, SourceBrave} {
		settings := Settings{SourceApp: source}
		settings.normalize()
		if settings.SourceApp != source {
			t.Fatalf("source %q was normalized to %q", source, settings.SourceApp)
		}
	}
}
