package recording

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const wavHeaderSize = 44

type wavEncoder struct {
	file      writeSyncCloser
	dataBytes uint64
	closed    bool
}

func newWAVEncoder(path string) (*wavEncoder, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	encoder := &wavEncoder{file: file}
	if err := encoder.writeHeader(0); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return encoder, nil
}

func newWAVEncoderForWriter(writer writeSyncCloser) (*wavEncoder, error) {
	encoder := &wavEncoder{file: writer}
	if err := encoder.writeHeader(0); err != nil {
		return nil, err
	}
	return encoder, nil
}

func (w *wavEncoder) WritePCM(data []byte) error {
	if w.closed {
		return errors.New("WAV encoder is closed")
	}
	if len(data)%BlockAlign != 0 {
		return fmt.Errorf("PCM packet length %d is not block-aligned", len(data))
	}
	if w.dataBytes+uint64(len(data)) > uint64(^uint32(0))-36 {
		return errors.New("classic RIFF WAV size limit reached")
	}
	n, err := w.file.Write(data)
	w.dataBytes += uint64(n)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

func (w *wavEncoder) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if seeker, ok := w.file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			_ = w.file.Close()
			return err
		}
	} else {
		_ = w.file.Close()
		return errors.New("WAV output is not seekable")
	}
	if err := w.writeHeader(uint32(w.dataBytes)); err != nil {
		_ = w.file.Close()
		return err
	}
	if err := w.file.Sync(); err != nil {
		_ = w.file.Close()
		return err
	}
	return w.file.Close()
}

func (w *wavEncoder) Abort() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return w.file.Close()
}

func (w *wavEncoder) writeHeader(dataSize uint32) error {
	header := make([]byte, wavHeaderSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], Channels)
	binary.LittleEndian.PutUint32(header[24:28], SampleRate)
	binary.LittleEndian.PutUint32(header[28:32], SampleRate*BlockAlign)
	binary.LittleEndian.PutUint16(header[32:34], BlockAlign)
	binary.LittleEndian.PutUint16(header[34:36], BitsPerSample)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)
	_, err := w.file.Write(header)
	return err
}
