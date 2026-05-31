package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var optionLineRe = regexp.MustCompile(`(?m)^\s*(\d+)[.)]\s+(.+\S)\s*$`)

func ExtractOptions(s string) []string {
	matches := optionLineRe.FindAllStringSubmatch(s, -1)
	if len(matches) < 2 {
		return nil
	}
	var opts []string
	for _, m := range matches {
		text := strings.TrimSpace(m[2])
		text = strings.ReplaceAll(text, "**", "")
		text = strings.ReplaceAll(text, "__", "")
		text = strings.Trim(text, "`")
		opts = append(opts, text)
	}
	return opts
}

var sessionContextRe = regexp.MustCompile(`(?s)<session_context>\s*(\{.*?\})\s*</session_context>`)

func ExtractSessionContext(s string) (jsonBody, cleaned string, ok bool) {
	loc := sessionContextRe.FindStringSubmatchIndex(s)
	if loc == nil {
		return "", s, false
	}
	jsonBody = strings.TrimSpace(s[loc[2]:loc[3]])
	cleaned = strings.TrimSpace(s[:loc[0]] + s[loc[1]:])
	return jsonBody, cleaned, true
}

var fileBlockRe = regexp.MustCompile(`(?s)<<<FILE:\s*([^>\r\n]+?)\s*>>>\s*\n?(.*?)<<<END>>>`)
var fileOpenRe = regexp.MustCompile(`<<<FILE:\s*([^>\r\n]+?)\s*>>>\s*\n?`)
var writeReceiptRe = regexp.MustCompile(`(?m)^\s*wrote\s+([^\s]+)\s*$`)

func WriteFileBlocks(buf, workDir string, seen map[string]bool) []string {
	matches := fileBlockRe.FindAllStringSubmatch(buf, -1)
	written := make([]string, 0, len(matches))
	for _, sm := range matches {
		if len(sm) < 3 {
			continue
		}
		name := strings.TrimSpace(sm[1])
		if name == "" {
			continue
		}
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			continue
		}
		if seen != nil && seen[name] {
			continue
		}
		full := filepath.Join(workDir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(full, []byte(sm[2]), 0o644); err != nil {
			continue
		}
		if seen != nil {
			seen[name] = true
		}
		written = append(written, full)
	}
	return written
}

func DisplayClean(buf string) string {
	return fileBlockRe.ReplaceAllStringFunc(buf, func(match string) string {
		sm := fileBlockRe.FindStringSubmatch(match)
		if len(sm) < 3 {
			return match
		}
		return "wrote " + strings.TrimSpace(sm[1])
	})
}

func WriteReceiptNames(s string) []string {
	matches := writeReceiptRe.FindAllStringSubmatch(s, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			names = append(names, strings.TrimSpace(match[1]))
		}
	}
	return names
}

func SplitInProgressFile(buf string) (cleanedChat, name, partial string, ok bool) {
	cleanedChat = DisplayClean(buf)
	loc := fileOpenRe.FindStringSubmatchIndex(cleanedChat)
	if loc == nil {
		return cleanedChat, "", "", false
	}
	name = strings.TrimSpace(cleanedChat[loc[2]:loc[3]])
	partial = cleanedChat[loc[1]:]
	cleanedChat = cleanedChat[:loc[0]]
	return cleanedChat, name, partial, true
}
