package logging

import (
	"bytes"
	"io"
	"os"
)

// FileSize returns the size of path in bytes, or 0 if it cannot be stat'd
// (missing file, permission error). Callers use it as a cheap change signal
// before deciding to re-read a log.
func FileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// TailLines returns up to the last n lines of the file at path as raw bytes,
// reading at most maxBytes from the end of the file. This keeps memory and CPU
// bounded regardless of how large the log has grown — the previous approach read
// the entire file on every request.
//
// A missing file yields (nil, nil). When the read window starts mid-file the
// first (partial) line is dropped so callers never see a truncated leading line.
func TailLines(path string, n int, maxBytes int64) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}

	readBytes := size
	if maxBytes > 0 && readBytes > maxBytes {
		readBytes = maxBytes
	}

	buf := make([]byte, readBytes)
	if _, err := file.ReadAt(buf, size-readBytes); err != nil && err != io.EOF {
		return nil, err
	}

	// Dropped the partial leading line only when we did not read from the start.
	if readBytes < size {
		if idx := bytes.IndexByte(buf, '\n'); idx >= 0 {
			buf = buf[idx+1:]
		}
	}

	return lastNLines(buf, n), nil
}

// lastNLines returns the last n newline-separated lines of buf, without a
// trailing newline. A single terminating newline is treated as a line
// terminator (not an extra empty line), so asking for n lines of an
// newline-terminated file returns n real lines.
func lastNLines(buf []byte, n int) []byte {
	if len(buf) == 0 {
		return buf
	}
	if buf[len(buf)-1] == '\n' {
		buf = buf[:len(buf)-1]
	}
	lines := bytes.Split(buf, []byte("\n"))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return bytes.Join(lines, []byte("\n"))
}
