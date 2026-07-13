// Package logging provides the application's log transport: a size-based
// rotating file writer, a slog setup helper, and bounded tail readers used to
// serve log content to the web UI. It has no third-party dependencies.
package logging

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RotatingWriter is an io.WriteCloser that appends to a file and rotates it once
// it would exceed MaxSize. Rotation happens on the write path (not just at open
// time), so a long-running producer can never grow a single file without bound.
//
// Rotated files are suffixed .1 (newest) .. .N (oldest); at most MaxBackups are
// kept. It is safe for concurrent use.
type RotatingWriter struct {
	path       string
	maxSize    int64
	maxBackups int

	mu   sync.Mutex
	file *os.File
	size int64
}

// NewRotatingWriter opens (creating parent dirs and the file as needed) path for
// appending. maxSize <= 0 disables rotation; maxBackups <= 0 keeps no backups
// (the file is simply truncated on rotation).
func NewRotatingWriter(path string, maxSize int64, maxBackups int) (*RotatingWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	w := &RotatingWriter{path: path, maxSize: maxSize, maxBackups: maxBackups}
	if err := w.openLocked(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotatingWriter) openLocked() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.openLocked(); err != nil {
			return 0, err
		}
	}
	// Rotate before writing when the incoming payload would push us over the
	// limit — but never rotate an empty file (a single oversized line still has
	// to land somewhere, and rotating here would spin out empty backups).
	if w.maxSize > 0 && w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// SetLimits updates the rotation thresholds live. A maxSize of 0 disables
// rotation. The new limits take effect on the next Write, which will rotate
// immediately if the current file already exceeds the new size.
func (w *RotatingWriter) SetLimits(maxSize int64, maxBackups int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.maxSize = maxSize
	w.maxBackups = maxBackups
}

// Rotate forces a rotation regardless of current size.
func (w *RotatingWriter) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size == 0 {
		return nil
	}
	return w.rotateLocked()
}

func (w *RotatingWriter) rotateLocked() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}

	if w.maxBackups > 0 {
		oldest := fmt.Sprintf("%s.%d", w.path, w.maxBackups)
		if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for i := w.maxBackups - 1; i >= 1; i-- {
			src := fmt.Sprintf("%s.%d", w.path, i)
			dst := fmt.Sprintf("%s.%d", w.path, i+1)
			if err := os.Rename(src, dst); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err := os.Rename(w.path, w.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if err := os.Remove(w.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return w.openLocked()
}

// Truncate empties the current file (used by "clear logs"). Rotated backups are
// left untouched.
//
// It closes and reopens rather than calling file.Truncate on the live handle:
// the handle is opened O_APPEND, which on Windows grants append-only access and
// makes an in-place truncate fail with "access is denied".
func (w *RotatingWriter) Truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}
	if err := os.Truncate(w.path, 0); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return w.openLocked()
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
