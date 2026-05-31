package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileBlocksReturnsWrittenPaths(t *testing.T) {
	dir := t.TempDir()
	seen := map[string]bool{}
	buf := "before\n<<<FILE: notes/01-refresher.md>>>\n# Refresher\n\nBody\n<<<END>>>\nafter"

	written := WriteFileBlocks(buf, dir, seen)
	if len(written) != 1 {
		t.Fatalf("expected 1 written file, got %d", len(written))
	}

	wantPath := filepath.Join(dir, "notes", "01-refresher.md")
	if written[0] != wantPath {
		t.Fatalf("written path = %q, want %q", written[0], wantPath)
	}

	b, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(b) != "# Refresher\n\nBody\n" {
		t.Fatalf("unexpected file content: %q", string(b))
	}

	written = WriteFileBlocks(buf, dir, seen)
	if len(written) != 0 {
		t.Fatalf("expected duplicate write to be skipped, got %d paths", len(written))
	}
}

func TestWriteFileBlocksRejectsUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	buf := "<<<FILE: ../outside.md>>>\nnope\n<<<END>>>\n<<<FILE: /tmp/abs.md>>>\nnope\n<<<END>>>"

	written := WriteFileBlocks(buf, dir, map[string]bool{})
	if len(written) != 0 {
		t.Fatalf("expected unsafe paths to be skipped, got %v", written)
	}
}

func TestWriteReceiptNames(t *testing.T) {
	got := WriteReceiptNames("ok\nwrote 07-advanced-python-refresher.md\nwrote notes/a.md")
	if len(got) != 2 {
		t.Fatalf("expected 2 receipts, got %d", len(got))
	}
	if got[0] != "07-advanced-python-refresher.md" || got[1] != "notes/a.md" {
		t.Fatalf("unexpected receipt names: %v", got)
	}
}
