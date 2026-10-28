package recording

import "io"

const (
	SampleRate    = 44100
	Channels      = 2
	BitsPerSample = 16
	BlockAlign    = Channels * BitsPerSample / 8
)

type encoder interface {
	WritePCM([]byte) error
	Close() error
	Abort() error
}

type writeSyncCloser interface {
	io.Writer
	Sync() error
	Close() error
}
