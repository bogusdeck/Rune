package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractAttachmentPathsAndCleanText(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "diagram.png")
	if err := os.WriteFile(image, []byte("png"), 0o644); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	text := "Explain this architecture\n" + image + "\nFocus on data flow"
	cleaned, paths := extractAttachmentPathsAndCleanText(text)
	if cleaned != "Explain this architecture\nFocus on data flow" {
		t.Fatalf("cleaned text = %q", cleaned)
	}
	if len(paths) != 1 || paths[0] != image {
		t.Fatalf("paths = %v", paths)
	}
}

func TestFindAttachmentPathsInText(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "screen shot.png")
	if err := os.WriteFile(image, []byte("png"), 0o644); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	paths := findAttachmentPathsInText(image)
	if len(paths) != 1 || paths[0] != image {
		t.Fatalf("paths = %v, want %q", paths, image)
	}
}

func TestExtractAttachmentKeepsUserTextAndFindsPath(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "resume.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF"), 0o644); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}

	text := "review this\n" + pdf
	cleaned, paths := extractAttachmentPathsAndCleanText(text)
	if cleaned != "review this" {
		t.Fatalf("cleaned text = %q", cleaned)
	}
	if len(paths) != 1 || paths[0] != pdf {
		t.Fatalf("paths = %v, want %q", paths, pdf)
	}
}

func TestParseDroppedImagePathEscapedSpaces(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "system diagram.png")
	if err := os.WriteFile(image, []byte("png"), 0o644); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	escaped := strings.ReplaceAll(image, " ", `\ `)
	got, ok := parseDroppedAttachmentPath(escaped)
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
	got, ok := parseDroppedAttachmentPath(uri)
	if !ok || got != image {
		t.Fatalf("got %q ok=%v want %q", got, ok, image)
	}
}

func TestParseDroppedAttachmentPathPDF(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "resume.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF"), 0o644); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}

	got, ok := parseDroppedAttachmentPath(pdf)
	if !ok || got != pdf {
		t.Fatalf("got %q ok=%v want %q", got, ok, pdf)
	}
}
