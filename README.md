# Notes Maker

A terminal-based (TUI) notes generation tool written in Go. Pick a topic, and Claude Code generates detailed markdown notes which you can watch being written live in a split-pane terminal UI.

## Architecture

Split-pane TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea):

- **Left pane — Claude Assistant**
  - Spawns `claude-local --qwen` (falls back to `claude`) inside a real PTY via `creack/pty`.
  - Uses `hinshun/vt10x` as a virtual terminal emulator so ANSI escapes, colors, and cursor movements from Claude's TUI render correctly.
  - User keystrokes are forwarded to the PTY.

- **Right pane — Notes Preview**
  - Uses `fsnotify` to watch the working directory for `.md` file changes.
  - When Claude writes/updates a markdown file, it's re-rendered with `glamour` into a scrollable viewport.

## Tech Stack

- `bubbletea` + `bubbles/viewport` + `lipgloss` — TUI framework & styling
- `glamour` — markdown rendering in terminal
- `creack/pty` + `hinshun/vt10x` — embed Claude's interactive CLI inside the app
- `fsnotify` — live file watching

## Installation

```bash
go mod tidy
go build -o notes-maker
```

## Usage

```bash
./notes-maker
```

Keybindings:
- `ctrl+c` — quit
- arrows / pgup / pgdn — scroll notes preview
- type — chat with Claudew