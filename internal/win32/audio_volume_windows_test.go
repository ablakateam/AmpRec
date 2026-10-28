//go:build windows

package win32

import (
	"math"
	"testing"
)

func TestCompensationGain(t *testing.T) {
	tests := []struct {
		name    string
		winamp  float32
		session float32
		want    float32
	}{
		{"full volume", 1, 1, 1},
		{"Winamp 220 of 255", 220.0 / 255.0, 1, 255.0 / 220.0},
		{"combined attenuation", 0.8, 0.5, 2.5},
		{"bounded gain", 0.1, 0.1, maxCaptureCompensation},
		{"muted source", 0, 1, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := compensationGain(test.winamp, test.session)
			if math.Abs(float64(got-test.want)) > 0.0001 {
				t.Fatalf("gain = %.6f, want %.6f", got, test.want)
			}
		})
	}
}
