package main

import (
	"encoding/base64"
	"net/url"
	"os"
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

func extractImagePathsAndCleanText(text string) (string, []string) {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	var imagePaths []string

	for _, line := range lines {
		if path, ok := parseDroppedImagePath(line); ok {
			imagePaths = append(imagePaths, path)
			continue
		}
		kept = append(kept, line)
	}

	return strings.TrimSpace(strings.Join(kept, "\n")), imagePaths
}

func mergeUniqueImagePaths(base []string, more []string) []string {
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

func parseDroppedImagePath(line string) (string, bool) {
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
	if !supportedImageExt[ext] {
		return "", false
	}
	info, err := os.Stat(s)
	if err != nil || info.IsDir() {
		return "", false
	}
	return s, true
}

func composeUserMessage(text string, imagePaths []string) string {
	text = strings.TrimSpace(text)
	if len(imagePaths) == 0 {
		return text
	}
	var b strings.Builder
	if text == "" {
		b.WriteString("Please analyze the attached image.")
	} else {
		b.WriteString(text)
	}
	b.WriteString("\n\nAttached image")
	if len(imagePaths) > 1 {
		b.WriteString("s")
	}
	b.WriteString(":\n")
	for _, path := range imagePaths {
		b.WriteString("- ")
		b.WriteString(path)
		b.WriteString("\n")
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
