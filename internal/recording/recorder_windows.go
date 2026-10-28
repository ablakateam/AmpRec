//go:build windows

package recording

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"amprec/internal/config"
	"amprec/internal/win32"
	"golang.org/x/sys/windows"
)

type Status int

const (
	StatusIdle Status = iota
	StatusStarting
	StatusRecording
	StatusFinalizing
	StatusError
)

type StartOptions struct {
	PID          uint32
	Format       string
	BitrateKbps  int
	OutputFolder string
}

type Snapshot struct {
	Status    Status
	StartedAt time.Time
	Level     float32
	Frames    uint64
	LastFile  string
	LastError string
	GainDB    float64
}

type session struct {
	stopEvent     windows.Handle
	packets       chan []byte
	captureDone   chan error
	writerDone    chan error
	done          chan struct{}
	finalPath     string
	temporaryPath string
	startedAt     time.Time
	levelBits     atomic.Uint32
	frames        atomic.Uint64
	stopRequested atomic.Bool
	resultError   error
	gain          float32
}

type Recorder struct {
	mu        sync.Mutex
	status    Status
	session   *session
	lastFile  string
	lastError string
}

func NewRecorder() *Recorder {
	return &Recorder{status: StatusIdle}
}

func (r *Recorder) Start(options StartOptions) error {
	if options.PID == 0 {
		return errors.New("no Winamp process is selected")
	}
	if options.Format != config.FormatMP3 && options.Format != config.FormatWAV {
		return fmt.Errorf("unsupported output format %q", options.Format)
	}
	if options.OutputFolder == "" {
		return errors.New("output folder is empty")
	}
	if err := os.MkdirAll(options.OutputFolder, 0o700); err != nil {
		return fmt.Errorf("create output folder: %w", err)
	}
	info, err := os.Stat(options.OutputFolder)
	if err != nil {
		return fmt.Errorf("open output folder: %w", err)
	}
	if !info.IsDir() {
		return errors.New("the selected output path is not a folder")
	}

	r.mu.Lock()
	if r.status == StatusStarting || r.status == StatusRecording || r.status == StatusFinalizing {
		r.mu.Unlock()
		return errors.New("a recording is already active")
	}
	r.status = StatusStarting
	r.lastError = ""
	r.mu.Unlock()

	finalPath, temporaryPath := uniqueOutputPaths(options.OutputFolder, options.Format, time.Now())
	stopEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		r.failStart(fmt.Errorf("create stop event: %w", err))
		return err
	}
	compensation := win32.CaptureVolumeCompensation(options.PID)
	s := &session{
		stopEvent:     stopEvent,
		packets:       make(chan []byte, 32),
		captureDone:   make(chan error, 1),
		writerDone:    make(chan error, 1),
		done:          make(chan struct{}),
		finalPath:     finalPath,
		temporaryPath: temporaryPath,
		startedAt:     time.Now(),
		gain:          compensation.Gain,
	}

	writerReady := make(chan error, 1)
	go runWriter(s, options, writerReady)
	if err := <-writerReady; err != nil {
		windows.CloseHandle(stopEvent)
		_ = os.Remove(temporaryPath)
		r.failStart(err)
		return err
	}

	captureReady := make(chan error, 1)
	go runCapture(s, options.PID, captureReady)
	if err := <-captureReady; err != nil {
		close(s.packets)
		<-s.writerDone
		windows.CloseHandle(stopEvent)
		_ = os.Remove(temporaryPath)
		r.failStart(err)
		return err
	}

	r.mu.Lock()
	r.session = s
	r.status = StatusRecording
	r.mu.Unlock()
	go r.monitor(s)
	return nil
}

func runWriter(s *session, options StartOptions, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := win32.CoInitializeMTA(); err != nil {
		ready <- err
		s.writerDone <- err
		return
	}
	defer win32.CoUninitialize()

	var output encoder
	var err error
	if options.Format == config.FormatWAV {
		output, err = newWAVEncoder(s.temporaryPath)
	} else {
		output, err = newMP3Encoder(s.temporaryPath, options.BitrateKbps)
	}
	if err != nil {
		ready <- fmt.Errorf("initialize %s encoder: %w", options.Format, err)
		s.writerDone <- err
		return
	}
	ready <- nil

	var firstError error
	for packet := range s.packets {
		if firstError != nil {
			continue
		}
		if err := output.WritePCM(packet); err != nil {
			firstError = fmt.Errorf("write encoded audio: %w", err)
			_ = windows.SetEvent(s.stopEvent)
		}
	}
	if closeErr := output.Close(); closeErr != nil {
		closeErr = fmt.Errorf("finalize output: %w", closeErr)
		if firstError == nil {
			firstError = closeErr
		} else {
			firstError = errors.Join(firstError, closeErr)
		}
	}
	s.writerDone <- firstError
}

func runCapture(s *session, pid uint32, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := win32.CoInitializeMTA(); err != nil {
		ready <- err
		s.captureDone <- err
		return
	}
	defer win32.CoUninitialize()

	capture, err := newProcessCapture(pid)
	if err != nil {
		ready <- fmt.Errorf("activate Winamp process capture: %w", err)
		s.captureDone <- err
		return
	}
	defer capture.Close()
	if err := capture.Start(); err != nil {
		ready <- fmt.Errorf("start Winamp process capture: %w", err)
		s.captureDone <- err
		return
	}
	ready <- nil
	err = capture.Run(s.stopEvent, func(packet []byte, _ float32) error {
		applyPCM16Gain(packet, s.gain)
		level := pcmPeak(packet)
		s.levelBits.Store(math.Float32bits(level))
		s.frames.Add(uint64(len(packet) / BlockAlign))
		select {
		case s.packets <- packet:
			return nil
		default:
			return errors.New("audio writer could not keep up; recording stopped to prevent data loss")
		}
	})
	close(s.packets)
	s.captureDone <- err
}

func (r *Recorder) monitor(s *session) {
	captureErr := <-s.captureDone
	writerErr := <-s.writerDone
	finalErr := errors.Join(captureErr, writerErr)
	completedPath := s.finalPath
	if finalErr == nil {
		if err := os.Rename(s.temporaryPath, s.finalPath); err != nil {
			finalErr = fmt.Errorf("commit recording: %w", err)
		}
	} else if _, statErr := os.Stat(s.temporaryPath); statErr == nil {
		completedPath = recoveredPath(s.finalPath)
		if renameErr := os.Rename(s.temporaryPath, completedPath); renameErr != nil {
			finalErr = errors.Join(finalErr, fmt.Errorf("preserve partial recording: %w", renameErr))
			completedPath = s.temporaryPath
		}
	}
	windows.CloseHandle(s.stopEvent)
	s.resultError = finalErr

	r.mu.Lock()
	if r.session == s {
		r.session = nil
		if finalErr != nil {
			r.status = StatusError
			r.lastError = finalErr.Error()
		} else {
			r.status = StatusIdle
			r.lastError = ""
		}
		r.lastFile = completedPath
	}
	r.mu.Unlock()
	close(s.done)
}

func (r *Recorder) Stop() error {
	r.mu.Lock()
	s := r.session
	if s == nil {
		r.mu.Unlock()
		return nil
	}
	if r.status != StatusFinalizing {
		r.status = StatusFinalizing
		s.stopRequested.Store(true)
		_ = windows.SetEvent(s.stopEvent)
	}
	r.mu.Unlock()
	<-s.done
	return s.resultError
}

func (r *Recorder) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := Snapshot{Status: r.status, LastFile: r.lastFile, LastError: r.lastError}
	if r.session != nil {
		snapshot.StartedAt = r.session.startedAt
		snapshot.Level = math.Float32frombits(r.session.levelBits.Load())
		snapshot.Frames = r.session.frames.Load()
		if r.session.gain > 0 {
			snapshot.GainDB = 20 * math.Log10(float64(r.session.gain))
		}
	}
	return snapshot
}

func applyPCM16Gain(packet []byte, gain float32) {
	if gain <= 1.0001 {
		return
	}
	for index := 0; index+1 < len(packet); index += 2 {
		sample := int32(int16(uint16(packet[index]) | uint16(packet[index+1])<<8))
		scaled := int32(math.Round(float64(float32(sample) * gain)))
		if scaled > 32767 {
			scaled = 32767
		} else if scaled < -32768 {
			scaled = -32768
		}
		packet[index] = byte(uint16(int16(scaled)))
		packet[index+1] = byte(uint16(int16(scaled)) >> 8)
	}
}

func (r *Recorder) failStart(err error) {
	r.mu.Lock()
	r.status = StatusError
	r.lastError = err.Error()
	r.mu.Unlock()
}

func uniqueOutputPaths(folder, format string, now time.Time) (string, string) {
	extension := strings.ToLower(format)
	base := "AmpRec_" + now.Format("2006-01-02_15-04-05")
	for suffix := 0; ; suffix++ {
		name := base
		if suffix > 0 {
			name = fmt.Sprintf("%s_%02d", base, suffix)
		}
		finalPath := filepath.Join(folder, name+"."+extension)
		temporaryPath := filepath.Join(folder, "."+name+".part."+extension)
		if _, err := os.Stat(finalPath); errors.Is(err, os.ErrNotExist) {
			if _, err := os.Stat(temporaryPath); errors.Is(err, os.ErrNotExist) {
				return finalPath, temporaryPath
			}
		}
	}
}

func recoveredPath(finalPath string) string {
	extension := filepath.Ext(finalPath)
	return strings.TrimSuffix(finalPath, extension) + "_recovered" + extension
}
