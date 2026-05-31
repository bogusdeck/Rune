package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

// ---- Sending user input ----

func (m *model) sendUserMessage() tea.Cmd {
	text := strings.TrimSpace(m.chatInput.Value())
	if text == "" || m.streaming {
		return nil
	}
	m.chatInput.SetValue("")
	m.messages = append(m.messages, chatMessage{Role: "user", Content: text})
	m.displayMsgs = append(m.displayMsgs, displayMsg{Role: "user", Content: text})
	m.pendingAsst = ""
	m.streaming = true
	m.streamHadWrite = false
	m.refreshChatView()
	return m.startStreamAndWait()
}

// ---- Top-level Update dispatcher ----

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+t" {
		if m.showSettings {
			m.closeSettings()
		} else {
			m.openSettings()
		}
		return m, nil
	}

	if m.showSettings {
		return m.updateSettings(msg)
	}

	if m.showReader {
		return m.updateReader(msg)
	}

	// Ctrl+L toggles the in-app log viewer from any state. We intercept this
	// before any other routing so it works on the topic screen, the loading
	// splash, and during chat.
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+l" {
		m.showLogs = !m.showLogs
		if m.showLogs {
			m.refreshLogView()
		}
		return m, nil
	}

	// Ctrl+R rewinds the conversation by one user turn (Claude Code / Gemini
	// CLI style). Only meaningful while we're in chat — silently ignored on
	// the topic screen and the loading splash.
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+r" && m.state == stateChat {
		m.rewindOnce()
		return m, nil
	}

	// Ctrl+E toggles the right pane between the markdown preview and a
	// read-only file explorer of workDir.
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+e" && m.state == stateChat {
		m.showExplorer = !m.showExplorer
		m.activePane = 1
		if !m.showExplorer {
			m.explorerScroll = 0
			m.explorerCursor = 0
		}
		log.Printf("explorer toggled: %v", m.showExplorer)
		return m, nil
	}

	// While the log overlay is visible we only handle navigation keys; every
	// other input is swallowed so it doesn't bleed into the chat input or the
	// topic list underneath.
	if m.showLogs {
		var cmd tea.Cmd
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.layout()
			m.refreshLogView()
		case tea.KeyMsg:
			switch msg.String() {
			case "esc", "q":
				m.showLogs = false
			case "r":
				m.refreshLogView()
			case "pgup", "k", "up":
				m.logView.HalfViewUp()
			case "pgdown", "j", "down":
				m.logView.HalfViewDown()
			case "g", "home":
				m.logView.GotoTop()
			case "G", "end":
				m.logView.GotoBottom()
			}
		}
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	if m.state == stateTopicInput {
		return m.updateTopic(msg)
	}

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.ready = true

	case tea.MouseMsg:
		if m.handleSplitResize(msg) {
			break
		}
		// Handle mouse wheel scrolling based on which side the mouse is on.
		// This prevents both panes from scrolling simultaneously.
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if msg.X < m.currentDividerX() {
				m.chatView.LineUp(3)
			} else {
				if m.showExplorer {
					m.explorerCursor -= 3
					if m.explorerCursor < 0 {
						m.explorerCursor = 0
					}
				} else {
					m.notesView.LineUp(3)
				}
			}
		case tea.MouseButtonWheelDown:
			if msg.X < m.currentDividerX() {
				m.chatView.LineDown(3)
			} else {
				if m.showExplorer {
					m.explorerCursor += 3
				} else {
					m.notesView.LineDown(3)
				}
			}
		}

	case tea.KeyMsg:
		// Filter out terminal-query response noise (OSC color, cursor pos,
		// focus events) that bubbletea surfaces as KeyMsgs. These would
		// otherwise dismiss the option picker and leak gibberish into the
		// chat input. ctrl+c is always preserved so quitting works.
		if msg.String() != "ctrl+c" && isTerminalNoise(msg, m.chatOpenedAt) {
			break
		}
		// Inline option picker takes precedence when active.
		if m.optionsActive {
			handled, pickCmd := m.handleOptionKey(msg)
			if pickCmd != nil {
				cmds = append(cmds, pickCmd)
			}
			if handled {
				break
			}
			// Fall through: user pressed something we don't treat as a pick,
			// so dismiss the picker and let the keystroke reach the input.
		}
		switch msg.String() {
		case "enter":
			if m.activePane == 1 && m.showExplorer {
				if c := m.previewSelectedExplorerFile(); c != nil {
					cmds = append(cmds, c)
				}
			} else if m.activePane == 1 && !m.showExplorer {
				m.openReader()
			} else if c := m.sendUserMessage(); c != nil {
				cmds = append(cmds, c)
			}
		case "tab":
			if m.activePane == 0 {
				m.activePane = 1
			} else {
				m.activePane = 0
			}
		case "pgup":
			m.scrollActivePane(-1, true)
		case "pgdown":
			m.scrollActivePane(1, true)
		case "up":
			m.scrollActivePane(-1, false)
		case "down":
			m.scrollActivePane(1, false)
		case "alt+pgup", "shift+pgup":
			m.notesView.HalfViewUp()
		case "alt+pgdown", "shift+pgdown":
			m.notesView.HalfViewDown()
		default:
			if m.activePane == 0 {
				m.chatInput, cmd = m.chatInput.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case loadingTickMsg:
		if m.state == stateLoading {
			m.loadingFrame++
			if m.topicReadyPending && m.loadingFrame >= minLoadingFrames && m.loadingFrame%loadingFrameCount == 0 {
				m.topicReadyPending = false
				cmds = append(cmds, openChatAfter(time.Millisecond))
			} else if m.loadingOpenPending {
				log.Printf("loading tick: starting openTopicData topic=%q", m.topic)
				m.loadingOpenPending = false
				cmds = append(cmds, m.runLoadingWork(openTopicData(m.topic, m.workDir)))
			} else {
				cmds = append(cmds, waitForLoadingMsg(m.streamCh))
			}
		}

	case topicReadyMsg:
		// Resumed-topic loading splash dismissal. Only acts if we are still
		// in stateLoading — if a stream chunk already pulled us into chat,
		// this is a no-op.
		if m.state == stateLoading {
			m.topicReadyPending = true
			cmds = append(cmds, waitForLoadingMsg(m.streamCh))
		}

	case openChatMsg:
		if m.state == stateLoading {
			m.state = stateChat
			m.chatOpenedAt = time.Now()
			m.layoutPanes()
			m.refreshChatView()
			cmds = append(cmds, m.watchFiles())
		}

	case topicOpenedMsg:
		if msg.err != nil {
			m.displayMsgs = append(m.displayMsgs, displayMsg{
				Role:    "system",
				Content: fmt.Sprintf("could not open topic %q: %v", msg.topic, msg.err),
			})
			m.state = stateChat
			m.chatOpenedAt = time.Now()
			m.layout()
			break
		}
		if msg.topic != m.topic {
			break
		}
		start := time.Now()
		m.workDir = msg.workDir
		if msg.hasSession {
			m.applySession(msg.session)
			m.refreshSystemPromptForPersonalization()
			m.showExplorer = true
			m.explorerScroll = 0
			m.explorerCursor = 0
			m.activePane = 1
		}
		isNew := !msg.hasSession
		log.Printf("openTopic topic=%q workDir=%s isNew=%v sessionType=%q preSession=%v", m.topic, m.workDir, isNew, m.sessionType, m.preSession)
		m.chatInput.Focus()
		m.chatInput.SetValue("")

		if isNew {
			m.loadingMessage = fmt.Sprintf("Reading %q… figuring out the best way to start.", m.topic)
			log.Printf("topicOpenedMsg handled new: elapsed=%s", time.Since(start))
			cmds = append(cmds, m.runLoadingWork(m.classifyTopic(m.topic)))
			break
		}

		m.preSession = m.sessionType != "" && m.sessionContext == ""
		switch {
		case m.preSession:
			m.loadingMessage = fmt.Sprintf("Resuming %q… continuing pre-session.", m.topic)
		case m.sessionLabel != "":
			m.loadingMessage = fmt.Sprintf("Resuming %q · %s…", m.topic, m.sessionLabel)
		default:
			m.loadingMessage = fmt.Sprintf("Resuming %q…", m.topic)
		}
		log.Printf("topicOpenedMsg handled resume: elapsed=%s", time.Since(start))
		cmds = append(cmds, m.runLoadingWork(topicReadyAfter(250*time.Millisecond)))

	case classifierDoneMsg:
		if msg.err != nil {
			// Couldn't classify — fall back to a generic main session so the
			// user isn't blocked. Surface a small system note.
			m.displayMsgs = append(m.displayMsgs, displayMsg{
				Role:    "system",
				Content: fmt.Sprintf("classifier failed (%v) — starting a generic session.", msg.err),
			})
			m.sessionType = ""
			m.sessionLabel = ""
			m.messages = []chatMessage{{Role: "system", Content: buildLegacyMainPromptWithProfile(m.topic, m.personalContext())}}
			m.preSession = false
			m.state = stateChat
			m.chatOpenedAt = time.Now()
			m.layout()
			m.saveSession()
			cmds = append(cmds, m.watchFiles())
			break
		}
		log.Printf("classifier ok: type=%q label=%q reason=%q", msg.res.Type, msg.res.Label, msg.res.Reason)
		m.sessionType = msg.res.Type
		m.sessionLabel = msg.res.Label
		// Update the loading-screen status message and kick off the pre-session.
		track := "skill assessment"
		if m.sessionType == "research" {
			track = "research scoping"
		}
		m.loadingMessage = fmt.Sprintf("Classified as %s — starting %s…", m.sessionType, track)
		cmds = append(cmds, m.startPreSession())

	case streamChunkMsg:
		// First chunk after loading? Drop the splash and slide into chat.
		if m.state == stateLoading {
			m.state = stateChat
			m.chatOpenedAt = time.Now()
			m.layout()
		}
		m.pendingAsst += string(msg)
		// Write any newly-completed <<<FILE>>> blocks to disk. We do NOT
		// mutate m.pendingAsst here — it stays raw so the model continues to
		// see the structured format in conversation history.
		if m.streamWritten == nil {
			m.streamWritten = map[string]bool{}
		}
		beforeCount := len(m.streamWritten)
		written := writeFileBlocks(m.pendingAsst, m.workDir, m.streamWritten)
		if len(m.streamWritten) != beforeCount {
			log.Printf("wrote new file block(s); total this stream: %d", len(m.streamWritten))
		}
		if cmd := m.handleWrittenFiles(written); cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Live preview: route any in-progress <<<FILE>>> body into notesView
		// rather than letting it render in the chat as raw text.
		if _, name, partial, ok := splitInProgressFile(m.pendingAsst); ok {
			if m.writingFile != name {
				log.Printf("live-write START name=%q partial=%d bytes (pending=%d)", name, len(partial), len(m.pendingAsst))
				m.showExplorer = false
			}
			m.writingFile = name
			m.updateLivePreview(name, partial)
		} else if m.writingFile != "" {
			log.Printf("live-write END name=%q (handing off to watcher)", m.writingFile)
			m.writingFile = ""
			m.livePreviewSize = 0
		}
		m.refreshChatView()
		cmds = append(cmds, waitForStreamMsg(m.streamCh))

	case streamDoneMsg:
		if msg.err != nil {
			// If cloud failed before any tokens streamed, transparently fall
			// back to the local model and replay the same request. We only do
			// this if we haven't yet shown the user any partial output, to
			// avoid splicing two model outputs together mid-message.
			if m.usingCloud && m.pendingAsst == "" {
				log.Printf("cloud stream failed: %v — switching to local %s", msg.err, m.localModel)
				m.usingCloud = false
				m.modelName = m.localModel
				m.displayMsgs = append(m.displayMsgs, displayMsg{
					Role:    "system",
					Content: fmt.Sprintf("cloud unavailable (%v) — switched to local %s", msg.err, m.localModel),
				})
				m.refreshChatView()
				cmds = append(cmds, m.startStreamAndWait())
				return m, firstCmd(cmds...)
			}
			log.Printf("stream error: %v", msg.err)
			m.displayMsgs = append(m.displayMsgs, displayMsg{Role: "system", Content: "error: " + msg.err.Error()})
			if m.state == stateLoading {
				m.state = stateChat
				m.chatOpenedAt = time.Now()
				m.layout()
				cmds = append(cmds, m.watchFiles())
			}
		} else {
			// Make sure any final <<<FILE>>> block from the last chunk has
			// been committed (idempotent thanks to m.streamWritten).
			if cmd := m.handleWrittenFiles(writeFileBlocks(m.pendingAsst, m.workDir, m.streamWritten)); cmd != nil {
				cmds = append(cmds, cmd)
			}

			// final = chat-pane version: completed blocks become "wrote NAME".
			final := displayClean(m.pendingAsst)
			historyContent := m.pendingAsst
			if !m.streamHadWrite {
				if names := writeReceiptNames(final); len(names) > 0 {
					log.Printf("model emitted write receipt without FILE block: %q", strings.Join(names, ", "))
					final = fmt.Sprintf("I said I wrote %s, but no FILE block was emitted, so no file was created. Please retry the request; I will use the required <<<FILE: ...>>> format.", strings.Join(names, ", "))
					historyContent = final
				}
			}

			// If we are in the pre-session phase, watch for the model emitting
			// <session_context>{...}</session_context>. When it does, strip the
			// tag from the user-visible message and promote to the main session.
			promoted := false
			if m.preSession {
				if jsonBody, cleaned, ok := extractSessionContext(final); ok {
					final = cleaned
					log.Printf("pre-session complete — promoting to main session (context len=%d)", len(jsonBody))
					m.promoteToMainSession(jsonBody)
					promoted = true
				}
			}
			// Keep RAW assistant output (with FILE blocks intact) in
			// conversation history so the model continues to emit the
			// structured format on subsequent turns. Display the cleaned
			// version in the chat pane.
			m.messages = append(m.messages, chatMessage{Role: "assistant", Content: historyContent})
			m.displayMsgs = append(m.displayMsgs, displayMsg{Role: "assistant", Content: final})
			if promoted {
				m.displayMsgs = append(m.displayMsgs, displayMsg{
					Role:    "system",
					Content: "✓ pre-session complete — main session is ready. Ask me anything.",
				})
				// Don't auto-arm the picker on the promotion turn; the user
				// drives the next message.
				m.options = nil
				m.optionsActive = false
			} else if opts := extractOptions(final); len(opts) >= 2 {
				// Did the model offer a numbered menu? Activate the picker.
				m.options = opts
				m.optionCursor = 0
				m.optionsActive = true
			} else {
				m.options = nil
				m.optionsActive = false
			}
		}
		m.pendingAsst = ""
		m.streaming = false
		m.writingFile = ""
		m.livePreviewSize = 0
		m.streamWritten = nil
		m.streamHadWrite = false
		m.refreshChatView()
		m.saveSession()

	case fileChangedMsg:
		m.lastFile = string(msg)
		out := renderFileForWidth(m.lastFile, m.notesView.Width)
		m.notesView.SetContent(out)
		m.notesView.GotoTop()
		log.Printf("preview updated: %s", m.lastFile)
		if m.state == stateChat {
			cmds = append(cmds, m.watchFiles())
		}
	}

	return m, firstCmd(cmds...)
}

func (m *model) openSettings() {
	m.showSettings = true
	m.settingsProfile.SetValue(m.config.PersonalProfile)
	_ = m.settingsProfile.Focus()
}

func (m *model) closeSettings() {
	m.config.PersonalProfile = strings.TrimSpace(m.settingsProfile.Value())
	saveAppConfig(m.config)
	m.settingsProfile.Blur()
	m.showSettings = false
	m.refreshSystemPromptForPersonalization()
}

func (m *model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.closeSettings()
			return m, nil
		case "ctrl+p":
			m.config.PersonalizedMode = !m.config.PersonalizedMode
			saveAppConfig(m.config)
			return m, nil
		}
	}
	m.settingsProfile, cmd = m.settingsProfile.Update(msg)
	return m, cmd
}

func (m *model) openReader() {
	if m.lastFile == "" {
		return
	}
	m.showReader = true
	m.refreshReaderView()
}

func (m *model) refreshReaderView() {
	w := m.width
	h := m.height
	if w < 20 {
		w = 20
	}
	if h < 8 {
		h = 8
	}
	m.readerView.Width = w - 4
	m.readerView.Height = h - 6
	out := renderFileForWidth(m.lastFile, m.readerView.Width)
	if out == "" {
		out = "(could not render file)"
	}
	m.readerView.SetContent(out)
	m.readerView.GotoTop()
}

func (m *model) updateReader(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshReaderView()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.showReader = false
			return m, nil
		case "pgup", "k":
			m.readerView.HalfViewUp()
		case "pgdown", "j":
			m.readerView.HalfViewDown()
		case "up":
			m.readerView.LineUp(1)
		case "down":
			m.readerView.LineDown(1)
		case "g", "home":
			m.readerView.GotoTop()
		case "G", "end":
			m.readerView.GotoBottom()
		}
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.readerView.LineUp(3)
		case tea.MouseButtonWheelDown:
			m.readerView.LineDown(3)
		}
	}
	m.readerView, cmd = m.readerView.Update(msg)
	return m, cmd
}

// ---- Topic-screen update ----

func (m *model) updateTopic(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if len(m.topicList) == 0 {
				return m, nil
			}
			if m.topicCursor <= 0 {
				m.topicCursor = -1 // jump to input
			} else {
				m.topicCursor--
			}
			return m, nil
		case "down":
			if len(m.topicList) == 0 {
				return m, nil
			}
			if m.topicCursor < 0 {
				m.topicCursor = 0
			} else if m.topicCursor < len(m.topicList)-1 {
				m.topicCursor++
			} else {
				m.topicCursor = -1 // wrap to input
			}
			return m, nil
		case "enter":
			var topic string
			if m.topicCursor >= 0 && m.topicCursor < len(m.topicList) {
				topic = m.topicList[m.topicCursor]
			} else {
				topic = strings.TrimSpace(m.topicInput.Value())
			}
			if topic == "" {
				return m, nil
			}
			log.Printf("topic enter: %q cursor=%d", topic, m.topicCursor)
			return m, m.beginOpenTopic(topic)
		default:
			// Any printable input shifts focus to the new-topic field.
			m.topicCursor = -1
		}
	}
	m.topicInput, cmd = m.topicInput.Update(msg)
	return m, cmd
}

func (m *model) scrollActivePane(direction int, page bool) {
	step := 1
	if page {
		step = 8
	}
	if m.activePane == 0 {
		if direction < 0 {
			if page {
				m.chatView.HalfViewUp()
			} else {
				m.chatView.LineUp(1)
			}
		} else if page {
			m.chatView.HalfViewDown()
		} else {
			m.chatView.LineDown(1)
		}
		return
	}
	if m.showExplorer {
		width := max(20, m.notesView.Width)
		explorerLines := m.explorerLines(width)
		if len(explorerLines) == 0 {
			m.explorerCursor = 0
			m.explorerScroll = 0
			return
		}
		if direction < 0 {
			m.explorerCursor -= step
			if m.explorerCursor < 0 {
				m.explorerCursor = 0
			}
		} else {
			m.explorerCursor += step
			if m.explorerCursor > len(explorerLines)-1 {
				m.explorerCursor = len(explorerLines) - 1
			}
		}
		return
	}
	if direction < 0 {
		if page {
			m.notesView.HalfViewUp()
		} else {
			m.notesView.LineUp(1)
		}
	} else if page {
		m.notesView.HalfViewDown()
	} else {
		m.notesView.LineDown(1)
	}
}

func (m *model) currentDividerX() int {
	leftWidth, _ := m.splitPaneWidths()
	return leftWidth + 2
}

func (m *model) handleSplitResize(msg tea.MouseMsg) bool {
	if m.state != stateChat || m.width <= 0 {
		return false
	}
	divider := m.currentDividerX()
	nearDivider := msg.X >= divider-1 && msg.X <= divider+1
	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonLeft && nearDivider {
			m.resizingSplit = true
			return true
		}
	case tea.MouseActionRelease:
		if m.resizingSplit {
			m.resizingSplit = false
			return true
		}
	case tea.MouseActionMotion:
		if m.resizingSplit {
			m.setSplitFromMouseX(msg.X)
			return true
		}
	}
	return false
}

func (m *model) setSplitFromMouseX(x int) {
	available := m.width - 4
	if available <= 0 {
		return
	}
	percent := x * 100 / available
	m.splitPercent = max(25, min(75, percent))
	m.layout()
}

func (m *model) previewSelectedExplorerFile() tea.Cmd {
	lines := m.explorerLines(max(20, m.notesView.Width))
	if len(lines) == 0 {
		return nil
	}
	if m.explorerCursor < 0 {
		m.explorerCursor = 0
	}
	if m.explorerCursor >= len(lines) {
		m.explorerCursor = len(lines) - 1
	}
	selected := lines[m.explorerCursor]
	if selected.isDir || selected.path == "" {
		return nil
	}
	m.showExplorer = false
	m.activePane = 1
	return func() tea.Msg {
		return fileChangedMsg(selected.path)
	}
}

func (m *model) handleWrittenFiles(paths []string) tea.Cmd {
	if len(paths) == 0 {
		return nil
	}
	last := paths[len(paths)-1]
	for _, path := range paths {
		log.Printf("file block written: %s", path)
	}
	m.lastFile = last
	m.streamHadWrite = true
	m.showExplorer = false
	m.selectExplorerPath(last)
	return func() tea.Msg {
		return fileChangedMsg(last)
	}
}

func (m *model) selectExplorerPath(path string) {
	if path == "" {
		return
	}
	lines := m.explorerLines(max(20, m.notesView.Width))
	for i, line := range lines {
		if filepath.Clean(line.path) == filepath.Clean(path) {
			m.explorerCursor = i
			if m.explorerScroll > i {
				m.explorerScroll = i
			}
			return
		}
	}
}

// ---- Topic transitions & helpers ----

// beginOpenTopic transitions to the loading screen immediately, then opens
// the topic workspace in a command so disk work cannot freeze the home screen.
func (m *model) beginOpenTopic(topic string) tea.Cmd {
	home, _ := os.UserHomeDir()
	m.topic = topic
	m.workDir = filepath.Join(home, "notes", slugify(topic))
	m.messages = nil
	m.displayMsgs = nil
	m.sessionType = ""
	m.sessionLabel = ""
	m.sessionContext = ""
	m.preSession = false
	m.options = nil
	m.optionsActive = false
	m.pendingAsst = ""
	m.streaming = false
	m.streamHadWrite = false
	m.showExplorer = false
	m.explorerScroll = 0
	m.explorerCursor = 0
	m.activePane = 0
	m.state = stateLoading
	m.loadingFrame = 0
	m.loadingMessage = fmt.Sprintf("Opening %q…", topic)
	m.loadingOpenPending = true
	m.topicReadyPending = false
	log.Printf("loading shown: topic=%q workDir=%s", topic, m.workDir)
	return waitForLoadingMsg(m.streamCh)
}

func openTopicData(topic, workDir string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			return topicOpenedMsg{topic: topic, workDir: workDir, err: err}
		}
		session, ok := loadSessionFile(workDir)
		log.Printf("openTopicData done: topic=%q hasSession=%v elapsed=%s", topic, ok, time.Since(start))
		return topicOpenedMsg{
			topic:      topic,
			workDir:    workDir,
			session:    session,
			hasSession: ok,
		}
	}
}

// loadingTick fires a loadingTickMsg every ~120ms while the loading screen is
// visible, advancing the animated gradient.
func loadingTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return loadingTickMsg{}
	})
}

const (
	loadingFrameCount = 8
	minLoadingFrames  = loadingFrameCount
)

func waitForLoadingMsg(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-ch:
			return msg
		case <-time.After(120 * time.Millisecond):
			return loadingTickMsg{}
		}
	}
}

func firstCmd(cmds ...tea.Cmd) tea.Cmd {
	for _, cmd := range cmds {
		if cmd != nil {
			return cmd
		}
	}
	return nil
}

func (m *model) runLoadingWork(cmd tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		done := make(chan tea.Msg, 1)
		go func() {
			if msg := cmd(); msg != nil {
				done <- msg
			}
		}()
		select {
		case msg := <-done:
			return msg
		case <-time.After(120 * time.Millisecond):
			go func() {
				if msg := <-done; msg != nil {
					m.streamCh <- msg
				}
			}()
			return loadingTickMsg{}
		}
	}
}

// topicReadyAfter fires a topicReadyMsg once after d, used to dismiss the
// resume-time loading splash.
func topicReadyAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return topicReadyMsg{}
	})
}

func openChatAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return openChatMsg{}
	})
}

// livePreviewMinDelta is the smallest growth (in bytes) of the in-progress
// file body that justifies a re-render of the notes preview. Glamour is
// expensive enough that re-running it on every chunk would chew CPU; this
// throttle keeps the preview feeling live without burning frames.
const livePreviewMinDelta = 24

// updateLivePreview pushes the partial body of an in-progress <<<FILE>>>
// block into the notes preview pane. Throttled by livePreviewMinDelta so
// glamour rendering stays cheap during fast streams.
func (m *model) updateLivePreview(name, body string) {
	if name == "" {
		return
	}
	full := filepath.Join(m.workDir, name)
	// Always update the header label so the user sees the right filename.
	m.lastFile = full
	if len(body)-m.livePreviewSize < livePreviewMinDelta && m.livePreviewSize != 0 {
		return
	}
	log.Printf("live-preview paint name=%q body=%d bytes (notesView w=%d)", name, len(body), m.notesView.Width)
	m.livePreviewSize = len(body)

	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	var rendered string
	if ext == "" || ext == "md" || ext == "markdown" {
		rendered = fmt.Sprintf("# %s _(writing…)_\n\n%s", name, body)
	} else {
		rendered = fmt.Sprintf("# %s _(writing…)_\n\n```%s\n%s\n```\n", name, ext, body)
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(m.notesView.Width),
	)
	if err != nil {
		return
	}
	out, err := renderer.Render(rendered)
	if err != nil {
		return
	}
	m.notesView.SetContent(out)
	m.notesView.GotoBottom()
}

func renderFileForWidth(path string, width int) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	body := string(content)
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	if ext != "" && ext != "md" && ext != "markdown" {
		body = fmt.Sprintf("# %s\n\n```%s\n%s\n```\n", filepath.Base(path), ext, body)
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(width))
	if err != nil {
		return body
	}
	out, err := renderer.Render(body)
	if err != nil {
		return body
	}
	return out
}

// startPreSession seeds the conversation with the appropriate pre-session
// system prompt and a hidden bootstrap user turn that triggers the model's
// first question. It is called once, right after the classifier completes.
func (m *model) startPreSession() tea.Cmd {
	var systemPrompt string
	if m.sessionType == "research" {
		systemPrompt = buildPreSessionResearchPromptWithProfile(m.topic, m.personalContext())
	} else {
		// Default to the skill track for any unknown classification.
		systemPrompt = buildPreSessionSkillPromptWithProfile(m.topic, m.personalContext())
	}
	m.messages = []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "Begin the pre-session — ask your first question."},
	}
	m.preSession = true
	m.pendingAsst = ""
	m.streaming = true
	m.refreshChatView()
	if cmd := m.startStream(); cmd != nil {
		_ = cmd()
	}
	return waitForLoadingMsg(m.streamCh)
}

// promoteToMainSession swaps the system prompt to the main-session prompt now
// that we have the user's <session_context>. The full pre-session conversation
// is preserved so the model still has memory of what the user said.
func (m *model) promoteToMainSession(contextJSON string) {
	m.sessionContext = contextJSON
	m.preSession = false
	if len(m.messages) > 0 && m.messages[0].Role == "system" {
		m.messages[0].Content = buildMainSessionPromptWithProfile(contextJSON, m.personalContext())
	} else {
		m.messages = append([]chatMessage{{Role: "system", Content: buildMainSessionPromptWithProfile(contextJSON, m.personalContext())}}, m.messages...)
	}
}

func (m *model) personalContext() string {
	return personalizedContext(m.config.PersonalProfile, m.config.PersonalizedMode)
}

func (m *model) refreshSystemPromptForPersonalization() {
	if len(m.messages) == 0 || m.messages[0].Role != "system" {
		return
	}
	switch {
	case m.sessionContext != "":
		m.messages[0].Content = buildMainSessionPromptWithProfile(m.sessionContext, m.personalContext())
	case m.preSession && m.sessionType == "research":
		m.messages[0].Content = buildPreSessionResearchPromptWithProfile(m.topic, m.personalContext())
	case m.preSession:
		m.messages[0].Content = buildPreSessionSkillPromptWithProfile(m.topic, m.personalContext())
	default:
		m.messages[0].Content = buildLegacyMainPromptWithProfile(m.topic, m.personalContext())
	}
	m.saveSession()
}

// ---- Rewind ----

// rewindOnce drops the most recent user turn (and everything that came after
// it: assistant reply, file blocks, system notes, picker) and pre-fills the
// chat input with the popped user message so the user can tweak and re-send.
// Press Ctrl+R repeatedly to keep stepping back. Returns true when something
// was actually rewound.
func (m *model) rewindOnce() bool {
	if m.streaming {
		return false
	}
	// Find the last "user" message in the model-facing transcript.
	lastUser := -1
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "user" {
			lastUser = i
			break
		}
	}
	if lastUser < 0 {
		return false
	}
	// During pre-session m.messages[0] is the system prompt and m.messages[1]
	// is the hidden "Begin the pre-session…" bootstrap user turn. We must not
	// erase that or the model loses its instructions.
	if m.preSession && lastUser <= 1 {
		return false
	}
	stashed := m.messages[lastUser].Content
	m.messages = m.messages[:lastUser]

	// Mirror the truncation in the rendered transcript: drop everything from
	// (and including) the most recent user displayMsg. Any interleaved system
	// notes after that point — error banners, pre-session-complete tag, prior
	// rewind banners — get cleaned up too.
	for i := len(m.displayMsgs) - 1; i >= 0; i-- {
		if m.displayMsgs[i].Role == "user" {
			m.displayMsgs = m.displayMsgs[:i]
			break
		}
	}

	// Restore the popped text into the input so the user can edit and resend.
	m.chatInput.SetValue(stashed)
	m.chatInput.SetCursor(len(stashed))

	// Clear any transient state tied to the message we just removed.
	m.options = nil
	m.optionsActive = false
	m.pendingAsst = ""

	m.displayMsgs = append(m.displayMsgs, displayMsg{
		Role:    "system",
		Content: "↶ rewound one turn — edit the message in the input and press enter to retry.",
	})
	m.refreshChatView()
	m.saveSession()
	log.Printf("rewind: dropped to %d messages (preSession=%v)", len(m.messages), m.preSession)
	return true
}

// ---- Inline option picker (Claude-Code-style) ----

// handleOptionKey processes a key while the inline option picker is active.
// Returns (handled, cmd). If handled is false the caller should dismiss the
// picker and let normal input handling take over.
func (m *model) handleOptionKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.optionCursor > 0 {
			m.optionCursor--
		}
		m.refreshChatView()
		return true, nil
	case "down", "j":
		if m.optionCursor < len(m.options)-1 {
			m.optionCursor++
		}
		m.refreshChatView()
		return true, nil
	case "enter":
		return true, m.pickOption(m.optionCursor)
	case "esc":
		m.optionsActive = false
		m.refreshChatView()
		return true, nil
	}
	// Digit shortcut: "1".. picks the Nth option directly.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		r := msg.Runes[0]
		if r >= '1' && r <= '9' {
			idx := int(r - '1')
			if idx < len(m.options) {
				return true, m.pickOption(idx)
			}
		}
	}
	// Real printable keystrokes (any rune that isn't an alt/ctrl combo) mean
	// the user is starting to type — dismiss the picker and fall through to
	// normal input handling. Terminal escape-sequence responses (OSC color
	// queries, focus events, etc.) arrive here as KeyMsgs too; we must NOT
	// treat them as a dismiss signal, or the picker disappears the instant
	// the chat screen mounts. So we only dismiss on a true rune keystroke.
	if msg.Type == tea.KeyRunes && !msg.Alt && len(msg.Runes) > 0 {
		m.optionsActive = false
		m.refreshChatView()
		return false, nil
	}
	// Backspace also clearly indicates the user wants to type, so dismiss.
	if msg.Type == tea.KeyBackspace {
		m.optionsActive = false
		m.refreshChatView()
		return false, nil
	}
	// Anything else (function keys, modifier-only events, terminal noise)
	// is silently consumed so the picker stays put.
	return true, nil
}

// pickOption sends the selected option text as the user's reply.
func (m *model) pickOption(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.options) {
		return nil
	}
	selected := m.options[idx]
	m.optionsActive = false
	m.options = nil
	m.chatInput.SetValue(selected)
	return m.sendUserMessage()
}

// isTerminalNoise returns true when msg is almost certainly not real user
// input but a byproduct of the terminal answering one of bubbletea/lipgloss's
// startup queries (OSC color, cursor position, focus reporting, …). These
// arrive as KeyMsgs because the bubbletea version we depend on doesn't parse
// the OSC/CSI responses internally.
func isTerminalNoise(msg tea.KeyMsg, mountedAt time.Time) bool {
	// Alt-modified keys: we never bind anything to alt+X, and the terminal's
	// escape responses always start with alt+] or alt+\\ (the leading ESC byte).
	if msg.Alt {
		return true
	}
	// Multi-rune KeyRunes events are never produced by typing — humans hit
	// one key at a time. They are always payload chunks of an OSC/CSI
	// response (e.g. "11;rgb:0a0a/0e0e/1414").
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
		return true
	}
	// Belt-and-suspenders: drop anything in the first 400ms after the chat
	// screen mounts, which is when the bulk of the noise burst lands.
	if !mountedAt.IsZero() && time.Since(mountedAt) < 400*time.Millisecond {
		return true
	}
	return false
}
