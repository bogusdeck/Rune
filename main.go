package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// logPath is where every log.Printf in the app lands. We keep it as a package
// global so the in-app log viewer (Ctrl+L) can read the same file.
var logPath string

func main() {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, "notes")
	_ = os.MkdirAll(logDir, 0o755)
	logPath = filepath.Join(logDir, ".notes-maker.log")

	if f, err := tea.LogToFile(logPath, ""); err == nil {
		defer f.Close()
		log.Printf("---- notes-maker boot ----")
	} else {
		fmt.Printf("warning: could not open log file %s: %v\n", logPath, err)
	}
	fmt.Printf("logs: %s  (press ctrl+l in-app, or `tail -f %s`)\n", logPath, logPath)

	m := initialModel()
	if _, err := tea.NewProgram(&m, tea.WithAltScreen(), tea.WithMouseAllMotion()).Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
