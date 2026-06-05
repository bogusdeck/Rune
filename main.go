package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"rune/internal/tui"
)

func main() {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, "notes")
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, ".rune.log")

	if f, err := tea.LogToFile(logPath, ""); err == nil {
		defer f.Close()
		log.Printf("---- rune boot ----")
	} else {
		fmt.Printf("warning: could not open log file %s: %v\n", logPath, err)
	}
	fmt.Printf("logs: %s  (press ctrl+l in-app, or `tail -f %s`)\n", logPath, logPath)

	if err := tui.Run(logPath); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
