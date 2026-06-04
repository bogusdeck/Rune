# Rune

Rune is a terminal-first note-making app built with Go.  
Pick a topic, chat through it, and generate structured markdown notes that are written to disk and previewed live inside a split-pane TUI.

## Why Rune

Rune is designed for people who want to learn, research, and write notes without leaving the terminal.

It helps you:

- start from a topic instead of a blank file
- guide a session as either learning or research
- generate markdown notes directly into your notes folder
- preview files live while they are being created
- resume older topic sessions later

## Features

- Split-pane TUI for chat and live notes preview
- Topic classification into `skill` or `research` workflows
- Pre-session guidance before the main session starts
- Markdown file generation with live filesystem watching
- Persistent session history per topic
- Local/cloud Ollama model selection with fallback support
- In-app log viewer, file explorer, and settings screen

## Feature Highlights

### Guided Topic Sessions

Rune starts from a topic, classifies the session, and guides the user through a more structured path instead of dropping them into an empty workspace.

### Live Markdown Preview

Generated markdown files are written directly to disk and rendered live in the preview pane, so users can see their notes take shape immediately.

### Resumable Sessions

Each topic keeps its own local session state, making it easy to come back later and continue work without starting over.

### Terminal-First Workflow

Rune is built for users who prefer learning, researching, and writing without leaving the terminal.

## Demo Flow

1. Start Rune
2. Enter a topic such as `Java`, `system design`, or `linear algebra`
3. Rune classifies the topic and begins a guided session
4. Notes are generated as markdown files
5. The right pane updates live as files are written
6. You can reopen the same topic later and continue where you left off

## Demo Videos

Use this section to showcase how Rune works in practice with recordings, GIFs, or short feature clips.

### Full Walkthrough

Replace this placeholder with your main demo video:

```md
[![Watch the full demo](assets/demo-cover.png)](https://your-video-link-here)
```

### Feature Clips

You can add smaller demos for individual features here:

- Topic classification
- Live notes preview
- Resume previous session
- File explorer and settings
- Ollama model fallback

Example format:

```md
#### Live Preview Demo
[Watch video](https://your-video-link-here)
```

## Requirements

- Go `1.25.3`
- Ollama running locally or reachable through `OLLAMA_URL`

Default Ollama endpoint:

```bash
http://localhost:11434
```

Optional environment variables:

```bash
OLLAMA_URL
OLLAMA_MODEL
OLLAMA_CLOUD_MODEL
OLLAMA_API_KEY
OLLAMA_NO_CLOUD=1
```

## Installation

Clone the repository and build from source:

```bash
git clone https://github.com/bogusdeck/Rune
cd Rune
go build -o rune .
```

## Usage

Run the app:

```bash
./rune
```

If someone installs the project on a new laptop, this is the expected workflow:

```bash
git clone https://github.com/bogusdeck/Rune
cd Rune
go build -o rune .
./rune
```

## Controls

- `Enter` send chat input
- `Tab` switch active pane
- `Ctrl+E` toggle the file explorer
- `Ctrl+L` open the in-app log viewer
- `Ctrl+O` return to topic selection
- `Ctrl+R` rewind one user turn
- `Ctrl+T` open settings
- `Ctrl+C` quit

## Project Structure

```text
.
├── internal/core/   # private helper logic: prompts, parser, config, sessions
├── main.go          # app entrypoint
├── model.go         # Bubble Tea model and app state
├── update.go        # event handling and session orchestration
├── view.go          # UI rendering
├── ollama.go        # Ollama client and streaming logic
├── classifier.go    # topic classification
├── watcher.go       # markdown file watcher
└── README.md
```

## Notes Storage

Rune stores topic sessions and generated notes under your local notes directory in your home folder.

That includes:

- generated markdown files
- per-topic session state
- local Rune configuration
- log output

## For Contributors

Useful local commands:

```bash
go test ./...
go build ./...
gofmt -w .
```

## Publishing Notes

Before pushing this project to GitHub, keep the repository source-only:

- do not commit compiled binaries like `rune`
- do not commit local machine files like `.DS_Store`
- keep setup instructions in the README so users can build locally

## License

Add a `LICENSE` file before publishing if you want others to use, modify, or distribute the project clearly.
