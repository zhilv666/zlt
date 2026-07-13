package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriterRotatesOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// maxSize 100 bytes, keep 2 backups.
	w, err := NewRotatingWriter(path, 100, 2)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer w.Close()

	line := strings.Repeat("x", 40) + "\n" // 41 bytes/line
	for i := 0; i < 10; i++ {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	// Current file must be under the limit...
	if fi, _ := os.Stat(path); fi.Size() > 100 {
		t.Fatalf("current file %d exceeds max 100", fi.Size())
	}
	// ...backups .1 and .2 exist, but .3 must not (capped at 2).
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected backup .1: %v", err)
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Fatalf("expected backup .2: %v", err)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("expected no backup .3, got err=%v", err)
	}
}

func TestRotatingWriterKeepsOversizedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	w, err := NewRotatingWriter(path, 10, 1)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer w.Close()

	big := []byte(strings.Repeat("y", 50)) // larger than maxSize on an empty file
	if _, err := w.Write(big); err != nil {
		t.Fatalf("write: %v", err)
	}
	if fi, _ := os.Stat(path); fi.Size() != 50 {
		t.Fatalf("oversized line should land whole, got size %d", fi.Size())
	}
}

func TestRotatingWriterTruncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	w, err := NewRotatingWriter(path, 0, 0)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer w.Close()

	if _, err := w.Write([]byte("hello world\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Truncate(); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if fi, _ := os.Stat(path); fi.Size() != 0 {
		t.Fatalf("expected empty file after truncate, got %d", fi.Size())
	}
	// Writing after truncate must still work and start from zero.
	if _, err := w.Write([]byte("again\n")); err != nil {
		t.Fatalf("write after truncate: %v", err)
	}
	if fi, _ := os.Stat(path); fi.Size() != int64(len("again\n")) {
		t.Fatalf("unexpected size after re-write: %d", fi.Size())
	}
}

func TestTailLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	var b strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&b, "line-%d\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Ask for the last 5 lines with a generous byte budget.
	got, err := TailLines(path, 5, 1<<20)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	if len(lines) != 5 || lines[0] != "line-96" || lines[4] != "line-100" {
		t.Fatalf("unexpected tail: %q", lines)
	}
}

func TestTailLinesBoundedWindowDropsPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// Lines are 7 bytes ("aaaaaa\n"); a 20-byte window lands mid-line.
	content := strings.Repeat("aaaaaa\n", 10) + "LASTLINE\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := TailLines(path, 100, 20)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	// No line in the result may be a truncated fragment of "aaaaaa".
	for _, line := range strings.Split(strings.TrimRight(string(got), "\n"), "\n") {
		if line != "aaaaaa" && line != "LASTLINE" {
			t.Fatalf("found partial/invalid line %q in %q", line, string(got))
		}
	}
}

func TestTailLinesMissingFile(t *testing.T) {
	got, err := TailLines(filepath.Join(t.TempDir(), "nope.log"), 10, 1<<20)
	if err != nil || got != nil {
		t.Fatalf("missing file should yield (nil,nil), got (%q,%v)", got, err)
	}
}

func TestRotatingWriterSetLimitsTakesEffect(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// Rotation disabled to start.
	w, err := NewRotatingWriter(path, 0, 3)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer w.Close()

	if _, err := w.Write([]byte(strings.Repeat("a", 200))); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("must not rotate while disabled, err=%v", err)
	}

	// Tighten the limit; the next write should now push over and rotate.
	w.SetLimits(50, 3)
	if _, err := w.Write([]byte("trigger\n")); err != nil {
		t.Fatalf("write after SetLimits: %v", err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotation after SetLimits, err=%v", err)
	}
}

func TestParseLevelAndLevelString(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{" warn ", slog.LevelWarn},
		{"error", slog.LevelError},
	}
	for _, tc := range cases {
		got, ok := ParseLevel(tc.in)
		if !ok || got != tc.want {
			t.Fatalf("ParseLevel(%q) = (%v,%v), want (%v,true)", tc.in, got, ok, tc.want)
		}
	}
	if _, ok := ParseLevel("verbose"); ok {
		t.Fatal("ParseLevel should reject unknown levels")
	}

	for level, want := range map[slog.Level]string{
		slog.LevelDebug: "debug",
		slog.LevelInfo:  "info",
		slog.LevelWarn:  "warn",
		slog.LevelError: "error",
	} {
		if got := LevelString(level); got != want {
			t.Fatalf("LevelString(%v) = %q, want %q", level, got, want)
		}
	}
}

func TestFileSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if FileSize(path) != 0 {
		t.Fatal("missing file should report size 0")
	}
	if err := os.WriteFile(path, []byte("12345"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if FileSize(path) != 5 {
		t.Fatalf("size = %d, want 5", FileSize(path))
	}
}
