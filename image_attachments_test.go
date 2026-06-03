package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractImagePathsAndCleanText(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "diagram.png")
	if err := os.WriteFile(image, []byte("png"), 0o644); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	text := "Explain this architecture\n" + image + "\nFocus on data flow"
	cleaned, paths := extractImagePathsAndCleanText(text)
	if cleaned != "Explain this architecture\nFocus on data flow" {
		t.Fatalf("cleaned text = %q", cleaned)
	}
	if len(paths) != 1 || paths[0] != image {
		t.Fatalf("paths = %v", paths)
	}
}

func TestParseDroppedImagePathEscapedSpaces(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "system diagram.png")
	if err := os.WriteFile(image, []byte("png"), 0o644); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	escaped := strings.ReplaceAll(image, " ", `\ `)
	got, ok := parseDroppedImagePath(escaped)
	if !ok || got != image {
		t.Fatalf("got %q ok=%v want %q", got, ok, image)
	}
}

func TestParseDroppedImagePathFileURI(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "system diagram.png")
	if err := os.WriteFile(image, []byte("png"), 0o644); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	uri := "file://" + strings.ReplaceAll(image, " ", "%20")
	got, ok := parseDroppedImagePath(uri)
	if !ok || got != image {
		t.Fatalf("got %q ok=%v want %q", got, ok, image)
	}
}
