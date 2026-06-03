package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type AppConfig struct {
	PersonalizedMode bool   `json:"personalized_mode"`
	PersonalProfile  string `json:"personal_profile"`
	DocumentEditor   string `json:"document_editor"`
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		PersonalizedMode: false,
		PersonalProfile:  "",
		DocumentEditor:   "",
	}
}

func AppConfigPath(notesRoot string) string {
	if notesRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ".rune-config.json"
		}
		notesRoot = filepath.Join(home, "notes")
	}
	return filepath.Join(notesRoot, ".rune-config.json")
}

func LoadAppConfig(notesRoot string) AppConfig {
	cfg := DefaultAppConfig()
	b, err := os.ReadFile(AppConfigPath(notesRoot))
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return DefaultAppConfig()
	}
	return cfg
}

func SaveAppConfig(notesRoot string, cfg AppConfig) {
	path := AppConfigPath(notesRoot)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o644)
}

func PersonalizedContext(profile string, enabled bool) string {
	profile = strings.TrimSpace(profile)
	if !enabled || profile == "" {
		return ""
	}
	return `Personalized mode is enabled.

Use these stable user notes to adapt examples, pacing, assumptions, and follow-up questions. Treat them as background context, not as the topic itself:
` + profile
}
