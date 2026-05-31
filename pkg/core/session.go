package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func Slugify(topic string) string {
	re := regexp.MustCompile(`[^A-Za-z0-9]+`)
	s := re.ReplaceAllString(strings.TrimSpace(topic), "_")
	return strings.Trim(s, "_")
}

func SessionPath(workDir string) string {
	return filepath.Join(workDir, ".chat.json")
}

func LoadSessionFile(workDir string) (SessionFile, bool) {
	b, err := os.ReadFile(SessionPath(workDir))
	if err != nil {
		return SessionFile{}, false
	}
	var s SessionFile
	if err := json.Unmarshal(b, &s); err != nil {
		return SessionFile{}, false
	}
	return s, true
}

func SaveSessionFile(workDir string, s SessionFile) {
	if workDir == "" {
		return
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(SessionPath(workDir), b, 0o644)
}

func ListExistingTopics(notesRoot string) []string {
	if notesRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		notesRoot = filepath.Join(home, "notes")
	}
	entries, err := os.ReadDir(notesRoot)
	if err != nil {
		return nil
	}
	var topics []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			topics = append(topics, e.Name())
		}
	}
	return topics
}
