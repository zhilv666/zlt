package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLogTailReturnsMissingAsEmpty(t *testing.T) {
	content, err := readLogTail(filepath.Join(t.TempDir(), "missing.log"), 10)
	if err != nil {
		t.Fatalf("read missing log: %v", err)
	}
	if content != "" {
		t.Fatalf("expected empty content, got %q", content)
	}
}

func TestReadLogTailReturnsLastNLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("1\n2\n3\n4\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	content, err := readLogTail(path, 2)
	if err != nil {
		t.Fatalf("read log tail: %v", err)
	}
	trimmed := strings.TrimSpace(content)
	if trimmed != "4" {
		t.Fatalf("unexpected log tail %q", content)
	}
}
