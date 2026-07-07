package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// rollingFile is a minimal size-based rotating file writer (no external deps).
// When the active file would exceed maxBytes it is rotated to "<path>.1",
// shifting older backups up to maxBackups; the oldest is discarded.
type rollingFile struct {
	mu         sync.Mutex
	path       string
	maxBytes   int64
	maxBackups int
	f          *os.File
	size       int64
}

func newRollingFile(path string, maxBytes int64, maxBackups int) (*rollingFile, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	r := &rollingFile{path: path, maxBytes: maxBytes, maxBackups: maxBackups}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rollingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f = f
	r.size = info.Size()
	return nil
}

func (r *rollingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return len(p), nil
	}
	if r.size+int64(len(p)) > r.maxBytes {
		_ = r.rotate()
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate closes the active file, shifts backups (.1→.2, …), moves the current
// file to ".1", and opens a fresh active file.
func (r *rollingFile) rotate() error {
	if r.f != nil {
		r.f.Close()
		r.f = nil
	}
	// Drop the oldest, then shift each backup up by one.
	oldest := fmt.Sprintf("%s.%d", r.path, r.maxBackups)
	_ = os.Remove(oldest)
	for i := r.maxBackups - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
	}
	if r.maxBackups >= 1 {
		_ = os.Rename(r.path, r.path+".1")
	}
	return r.open()
}

// truncate empties the active log file (and removes backups).
func (r *rollingFile) truncate() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		r.f.Close()
		r.f = nil
	}
	for i := 1; i <= r.maxBackups; i++ {
		_ = os.Remove(fmt.Sprintf("%s.%d", r.path, i))
	}
	if err := os.Truncate(r.path, 0); err != nil && !os.IsNotExist(err) {
		// fall through to re-open; a fresh file is created if missing
	}
	return r.open()
}

func (r *rollingFile) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		r.f.Close()
		r.f = nil
	}
}
