package tui

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var supportedImageExt = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".gif":  true,
	".bmp":  true,
}

var supportedDocumentExt = map[string]bool{
	".pdf":      true,
	".txt":      true,
	".md":       true,
	".markdown": true,
	".csv":      true,
	".json":     true,
	".yaml":     true,
	".yml":      true,
	".xml":      true,
	".doc":      true,
	".docx":     true,
	".rtf":      true,
}

func extractAttachmentPathsAndCleanText(text string) (string, []string) {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	var paths []string

	for _, line := range lines {
		if path, ok := parseDroppedAttachmentPath(line); ok {
			paths = append(paths, path)
			continue
		}
		kept = append(kept, line)
	}

	return strings.TrimSpace(strings.Join(kept, "\n")), paths
}

func mergeUniquePaths(base []string, more []string) []string {
	seen := make(map[string]bool, len(base))
	out := make([]string, 0, len(base)+len(more))
	for _, path := range base {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	for _, path := range more {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func parseDroppedAttachmentPath(line string) (string, bool) {
	s := strings.TrimSpace(line)
	if s == "" {
		return "", false
	}
	if unquoted, err := strconv.Unquote(s); err == nil {
		s = unquoted
	}
	s = strings.Trim(s, "'\"")
	if strings.HasPrefix(s, "file://") {
		if parsed, err := url.Parse(s); err == nil {
			if decoded, err := url.PathUnescape(parsed.Path); err == nil {
				s = decoded
			} else {
				s = parsed.Path
			}
		}
	}
	s = strings.ReplaceAll(s, `\ `, " ")
	s = strings.ReplaceAll(s, `\(`, "(")
	s = strings.ReplaceAll(s, `\)`, ")")
	s = strings.ReplaceAll(s, `\&`, "&")
	if !filepath.IsAbs(s) {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(s))
	if !supportedImageExt[ext] && !supportedDocumentExt[ext] {
		return "", false
	}
	info, err := os.Stat(s)
	if err != nil || info.IsDir() {
		return "", false
	}
	return s, true
}

func splitAttachmentPaths(paths []string) (images []string, docs []string) {
	for _, path := range paths {
		if supportedImageExt[strings.ToLower(filepath.Ext(path))] {
			images = append(images, path)
			continue
		}
		docs = append(docs, path)
	}
	return images, docs
}

func composeUserMessage(text string, attachmentPaths []string) string {
	text = strings.TrimSpace(text)
	if len(attachmentPaths) == 0 {
		return text
	}
	var b strings.Builder
	if text == "" {
		b.WriteString("Please analyze the attached file.")
	} else {
		b.WriteString(text)
	}
	b.WriteString("\n\nAttached file")
	if len(attachmentPaths) > 1 {
		b.WriteString("s")
	}
	b.WriteString(":\n")
	for _, path := range attachmentPaths {
		b.WriteString("- ")
		b.WriteString(path)
		b.WriteString("\n")
	}
	if docs := documentContext(attachmentPaths); docs != "" {
		b.WriteString("\n")
		b.WriteString(docs)
	}
	return strings.TrimSpace(b.String())
}

func encodeLocalImages(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out = append(out, base64.StdEncoding.EncodeToString(b))
	}
	return out
}

func documentContext(paths []string) string {
	var sections []string
	for _, path := range paths {
		if supportedImageExt[strings.ToLower(filepath.Ext(path))] {
			continue
		}
		if text := extractDocumentText(path); text != "" {
			sections = append(sections, fmt.Sprintf("Document content from %s:\n%s", path, text))
		}
	}
	if len(sections) == 0 {
		return ""
	}
	return "Attached document context:\n\n" + strings.Join(sections, "\n\n---\n\n")
}

func extractDocumentText(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".md", ".markdown", ".csv", ".json", ".yaml", ".yml", ".xml":
		b, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return truncateAttachmentText(string(b))
	case ".pdf":
		out, err := exec.Command("pdftotext", "-layout", path, "-").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return truncateAttachmentText(string(out))
		}
	case ".doc", ".docx", ".rtf":
		out, err := exec.Command("textutil", "-convert", "txt", "-stdout", path).Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return truncateAttachmentText(string(out))
		}
	}
	return fmt.Sprintf("(Rune could not extract text from this %s file. The file path is still attached for capable providers.)", strings.TrimPrefix(ext, "."))
}

func truncateAttachmentText(s string) string {
	s = strings.TrimSpace(s)
	const maxAttachmentChars = 24000
	if len(s) <= maxAttachmentChars {
		return s
	}
	return s[:maxAttachmentChars] + "\n\n[truncated by Rune]"
}
