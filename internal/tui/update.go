package tui

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rune/internal/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

// ---- Sending user input ----

func (m *model) sendUserMessage() tea.Cmd {
	raw := strings.TrimSpace(m.chatInput.Value())
	if (raw == "" && len(m.composerAttachmentPaths) == 0) || m.streaming {
		return nil
	}
	cleaned, attachmentPaths := extractAttachmentPathsAndCleanText(raw)
	attachmentPaths = mergeUniquePaths(m.composerAttachmentPaths, attachmentPaths)
	imagePaths, _ := splitAttachmentPaths(attachmentPaths)
	text := composeUserMessage(cleaned, attachmentPaths)
	if text == "" {
		return nil
	}
	if m.providerName() == providerAuto {
		routed := m.effectiveProviderName([]chatMessage{{Role: "user", Content: text, ImagePaths: imagePaths, FilePaths: attachmentPaths}})
		reason := autoRouteReason(text, attachmentPaths, routed)
		m.displayMsgs = append(m.displayMsgs, displayMsg{Role: "system", Content: fmt.Sprintf("auto routed this turn to %s — %s", routed, reason)})
	}
	m.chatInput.SetValue("")
	m.composerAttachmentPaths = nil
	m.messages = append(m.messages, chatMessage{Role: "user", Content: text, ImagePaths: imagePaths, FilePaths: attachmentPaths})
	m.displayMsgs = append(m.displayMsgs, displayMsg{Role: "user", Content: text})
	m.pendingAsst = ""
	m.streaming = true
	m.streamHadWrite = false
	m.refreshSystemPromptForPersonalization()
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

	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+k" {
		m.showModelSwitcher = !m.showModelSwitcher
		return m, nil
	}

	if m.showSettings {
		return m.updateSettings(msg)
	}

	if m.showModelSwitcher {
		return m.updateModelSwitcher(msg)
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

	// Ctrl+O returns to the home screen (topic selection / list).
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+o" && (m.state == stateChat || m.state == stateLoading) {
		m.state = stateTopicInput
		m.topicList = core.ListExistingTopics("")
		m.topicCursor = 0
		if len(m.topicList) == 0 {
			m.topicCursor = -1
		} else {
			for i, t := range m.topicList {
				if t == m.topic {
					m.topicCursor = i
					break
				}
			}
		}
		m.topicInput.Focus()
		m.topicInput.SetValue("")
		log.Printf("returned to home screen (topic selection)")
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
		case "ctrl+j", "shift+enter":
			if m.activePane == 0 {
				m.chatInput.InsertRune('\n')
				m.syncComposerAttachments()
			}
		case "backspace":
			if m.activePane == 0 && strings.TrimSpace(m.chatInput.Value()) == "" && len(m.composerAttachmentPaths) > 0 {
				m.composerAttachmentPaths = m.composerAttachmentPaths[:len(m.composerAttachmentPaths)-1]
				break
			}
			if m.activePane == 0 {
				m.chatInput, cmd = m.chatInput.Update(msg)
				m.syncComposerAttachments()
				cmds = append(cmds, cmd)
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
				m.syncComposerAttachments()
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
			cmds = append(cmds, m.watchFiles(), m.primePreview())
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
			m.activePane = 0
		}
		isNew := !msg.hasSession
		log.Printf("openTopic topic=%q workDir=%s isNew=%v sessionType=%q preSession=%v", m.topic, m.workDir, isNew, m.sessionType, m.preSession)
		cmds = append(cmds, m.chatInput.Focus())
		m.chatInput.SetValue("")
		m.composerAttachmentPaths = nil

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
			m.messages = []chatMessage{{Role: "system", Content: core.BuildLegacyMainPromptWithProfile(m.topic, m.personalContext())}}
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
		written := core.WriteFileBlocks(m.pendingAsst, m.workDir, m.streamWritten)
		if len(m.streamWritten) != beforeCount {
			log.Printf("wrote new file block(s); total this stream: %d", len(m.streamWritten))
		}
		if cmd := m.handleWrittenFiles(written); cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Live preview: route any in-progress <<<FILE>>> body into notesView
		// rather than letting it render in the chat as raw text.
		if _, name, partial, ok := core.SplitInProgressFile(m.pendingAsst); ok {
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
				return m, batchCmds(cmds...)
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
			if cmd := m.handleWrittenFiles(core.WriteFileBlocks(m.pendingAsst, m.workDir, m.streamWritten)); cmd != nil {
				cmds = append(cmds, cmd)
			}

			// final = chat-pane version: completed blocks become "wrote NAME".
			final := core.DisplayClean(m.pendingAsst)
			historyContent := m.pendingAsst
			if !m.streamHadWrite {
				if names := core.WriteReceiptNames(final); len(names) > 0 {
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
				if jsonBody, cleaned, ok := core.ExtractSessionContext(final); ok {
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
			} else if opts := core.ExtractOptions(final); len(opts) >= 2 {
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
		cmds = append(cmds, renderFileCmd(m.lastFile, m.notesView.Width))

	case fileRenderedMsg:
		if msg.path == m.lastFile {
			m.notesView.SetContent(msg.content)
			m.notesView.GotoTop()
			log.Printf("preview updated: %s", m.lastFile)
		}
		if m.state == stateChat {
			cmds = append(cmds, m.watchFiles())
		}
	}

	return m, batchCmds(cmds...)
}

func (m *model) openSettings() {
	m.showSettings = true
	m.settingsProvider.SetValue(normalizeProviderName(m.config.Provider))
	m.settingsCodexCommand.SetValue(m.config.CodexCommand)
	m.settingsCodexModel.SetValue(m.config.CodexModel)
	m.settingsAntigravityCommand.SetValue(m.config.AntigravityCommand)
	m.settingsEditor.SetValue(m.config.DocumentEditor)
	m.settingsProfile.SetValue(m.config.PersonalProfile)
	m.settingsFocus = 0
	m.settingsProvider.Focus()
	m.settingsCodexCommand.Blur()
	m.settingsCodexModel.Blur()
	m.settingsAntigravityCommand.Blur()
	m.settingsEditor.Blur()
	m.settingsProfile.Blur()
}

func (m *model) closeSettings() {
	m.config.Provider = normalizeProviderName(m.settingsProvider.Value())
	m.config.CodexCommand = strings.TrimSpace(m.settingsCodexCommand.Value())
	m.config.CodexModel = strings.TrimSpace(m.settingsCodexModel.Value())
	m.config.AntigravityCommand = strings.TrimSpace(m.settingsAntigravityCommand.Value())
	m.config.DocumentEditor = strings.TrimSpace(m.settingsEditor.Value())
	m.config.PersonalProfile = strings.TrimSpace(m.settingsProfile.Value())
	core.SaveAppConfig("", m.config)
	m.settingsProvider.Blur()
	m.settingsCodexCommand.Blur()
	m.settingsCodexModel.Blur()
	m.settingsAntigravityCommand.Blur()
	m.settingsEditor.Blur()
	m.settingsProfile.Blur()
	m.showSettings = false
	m.syncProviderState()
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
		case "tab":
			m.settingsFocus = (m.settingsFocus + 1) % 6
			m.focusSettingsField()
			return m, nil
		case "ctrl+p":
			m.config.PersonalizedMode = !m.config.PersonalizedMode
			core.SaveAppConfig("", m.config)
			return m, nil
		}
	}
	switch m.settingsFocus {
	case 0:
		m.settingsProvider, cmd = m.settingsProvider.Update(msg)
		return m, cmd
	case 1:
		m.settingsCodexCommand, cmd = m.settingsCodexCommand.Update(msg)
		return m, cmd
	case 2:
		m.settingsCodexModel, cmd = m.settingsCodexModel.Update(msg)
		return m, cmd
	case 3:
		m.settingsAntigravityCommand, cmd = m.settingsAntigravityCommand.Update(msg)
		return m, cmd
	case 4:
		m.settingsEditor, cmd = m.settingsEditor.Update(msg)
		return m, cmd
	default:
		m.settingsProfile, cmd = m.settingsProfile.Update(msg)
		return m, cmd
	}
}

func (m *model) updateModelSwitcher(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
	case tea.KeyMsg:
		presets := m.modelSwitcherPresets()
		switch msg.String() {
		case "esc", "ctrl+k":
			m.showModelSwitcher = false
			return m, nil
		case "up", "k":
			if m.switcherCursor > 0 {
				m.switcherCursor--
			}
			return m, nil
		case "down", "j":
			if m.switcherCursor < len(presets)-1 {
				m.switcherCursor++
			}
			return m, nil
		case "enter":
			if m.switcherCursor >= 0 && m.switcherCursor < len(presets) {
				presets[m.switcherCursor].apply(m)
				core.SaveAppConfig("", m.config)
				m.displayMsgs = append(m.displayMsgs, displayMsg{Role: "system", Content: fmt.Sprintf("switched provider preset to %s", presets[m.switcherCursor].label)})
				m.refreshChatView()
			}
			m.showModelSwitcher = false
			return m, nil
		}
	}
	return m, nil
}

func (m *model) focusSettingsField() {
	m.settingsProvider.Blur()
	m.settingsCodexCommand.Blur()
	m.settingsCodexModel.Blur()
	m.settingsAntigravityCommand.Blur()
	m.settingsEditor.Blur()
	m.settingsProfile.Blur()

	switch m.settingsFocus {
	case 0:
		m.settingsProvider.Focus()
	case 1:
		m.settingsCodexCommand.Focus()
	case 2:
		m.settingsCodexModel.Focus()
	case 3:
		m.settingsAntigravityCommand.Focus()
	case 4:
		m.settingsEditor.Focus()
	default:
		_ = m.settingsProfile.Focus()
	}
}

func (m *model) openReader() {
	if m.lastFile == "" {
		return
	}
	m.showReader = true
	m.readerRawMode = false
	m.readerStatus = ""
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
	out := renderReaderFile(m.lastFile, m.readerView.Width, m.readerRawMode)
	if out == "" {
		out = "(could not render file)"
	}
	m.readerView.SetContent(out)
	m.readerView.GotoTop()
}

func (m *model) updateReader(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case editorDoneMsg:
		if msg.path != "" {
			m.lastFile = msg.path
		}
		if msg.err != nil {
			m.readerStatus = "editor failed: " + msg.err.Error()
		} else {
			m.readerStatus = "saved and reloaded"
		}
		m.refreshReaderView()
		if m.lastFile != "" {
			m.notesView.SetContent("Rendering preview...")
			return m, renderFileCmd(m.lastFile, m.notesView.Width)
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshReaderView()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.showReader = false
			return m, nil
		case "m":
			m.readerRawMode = !m.readerRawMode
			m.readerStatus = ""
			m.refreshReaderView()
			return m, nil
		case "e":
			return m, m.editCurrentReaderFile()
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

func (m *model) editCurrentReaderFile() tea.Cmd {
	if m.lastFile == "" {
		m.readerStatus = "no file selected"
		return nil
	}
	editor := configuredEditor(m.config.DocumentEditor)
	m.readerStatus = "editing with " + editor
	cmd := exec.Command("/bin/sh", "-lc", editor+" "+shellQuote(m.lastFile))
	path := m.lastFile
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{path: path, err: err}
	})
}

func configuredEditor(configured string) string {
	if editor := strings.TrimSpace(configured); editor != "" {
		return editor
	}
	if editor := strings.TrimSpace(os.Getenv("EDITOR")); editor != "" {
		return editor
	}
	return "nano"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
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
	m.notesView.SetContent("Rendering preview...")
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
	m.workDir = filepath.Join(home, "notes", core.Slugify(topic))
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
	m.writingFile = ""
	m.livePreviewSize = 0
	m.lastFile = ""
	m.composerAttachmentPaths = nil
	m.notesView.SetContent("")
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
		session, ok := core.LoadSessionFile(workDir)
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

func batchCmds(cmds ...tea.Cmd) tea.Cmd {
	var nonNil []tea.Cmd
	for _, cmd := range cmds {
		if cmd != nil {
			nonNil = append(nonNil, cmd)
		}
	}
	if len(nonNil) == 0 {
		return nil
	}
	return tea.Batch(nonNil...)
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
// file body that justifies repainting the notes preview. Live writes render
// raw markdown instead of Glamour so the typing effect stays immediate.
const livePreviewMinDelta = 8

// updateLivePreview pushes the partial body of an in-progress <<<FILE>>>
// block into the notes preview pane as raw text with a cursor. The final
// fileChangedMsg renders the saved markdown once the block is complete.
func (m *model) updateLivePreview(name, body string) {
	if name == "" {
		return
	}
	full := filepath.Join(m.workDir, name)
	m.lastFile = full
	if len(body)-m.livePreviewSize < livePreviewMinDelta && m.livePreviewSize != 0 {
		return
	}
	log.Printf("live-preview paint name=%q body=%d bytes (notesView w=%d)", name, len(body), m.notesView.Width)
	m.livePreviewSize = len(body)

	if strings.TrimSpace(body) == "" {
		body = "waiting for file content..."
	}
	ruleWidth := max(8, min(len(name)+8, m.notesView.Width))
	out := fmt.Sprintf("writing %s\n%s\n\n%s▍", name, strings.Repeat("-", ruleWidth), body)
	m.notesView.SetContent(out)
	m.notesView.GotoBottom()
}

func renderFileForWidth(path string, width int) string {
	return renderReaderFile(path, width, false)
}

func renderReaderFile(path string, width int, raw bool) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	body := string(content)
	if raw {
		return body
	}
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
		systemPrompt = core.BuildPreSessionResearchPromptWithProfile(m.topic, m.personalContext())
	} else {
		// Default to the skill track for any unknown classification.
		systemPrompt = core.BuildPreSessionSkillPromptWithProfile(m.topic, m.personalContext())
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
		m.messages[0].Content = core.BuildMainSessionPromptWithProfile(contextJSON, m.personalContext())
	} else {
		m.messages = append([]chatMessage{{Role: "system", Content: core.BuildMainSessionPromptWithProfile(contextJSON, m.personalContext())}}, m.messages...)
	}
	m.refreshSystemPromptForPersonalization()
}

func (m *model) personalContext() string {
	return core.PersonalizedContext(m.config.PersonalProfile, m.config.PersonalizedMode)
}

func (m *model) refreshSystemPromptForPersonalization() {
	if len(m.messages) == 0 || m.messages[0].Role != "system" {
		return
	}
	var base string
	switch {
	case m.sessionContext != "":
		base = core.BuildMainSessionPromptWithProfile(m.sessionContext, m.personalContext())
	case m.preSession && m.sessionType == "research":
		base = core.BuildPreSessionResearchPromptWithProfile(m.topic, m.personalContext())
	case m.preSession:
		base = core.BuildPreSessionSkillPromptWithProfile(m.topic, m.personalContext())
	default:
		base = core.BuildLegacyMainPromptWithProfile(m.topic, m.personalContext())
	}
	if !m.preSession {
		base += "\n\n" + m.currentFilesContext()
	}
	m.messages[0].Content = base
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
	stashedFiles := append([]string(nil), m.messages[lastUser].FilePaths...)
	if len(stashedFiles) == 0 {
		stashedFiles = append([]string(nil), m.messages[lastUser].ImagePaths...)
	}
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
	m.chatInput.CursorEnd()
	m.composerAttachmentPaths = stashedFiles

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
	m.composerAttachmentPaths = nil
	return m.sendUserMessage()
}

func (m *model) syncComposerAttachments() {
	cleaned, paths := extractAttachmentPathsAndCleanText(m.chatInput.Value())
	if len(paths) == 0 {
		return
	}
	m.composerAttachmentPaths = mergeUniquePaths(m.composerAttachmentPaths, paths)
	m.chatInput.SetValue(cleaned)
	m.chatInput.CursorEnd()
}

// isTerminalNoise returns true when msg is almost certainly not real user
// input but a byproduct of the terminal answering one of bubbletea/lipgloss's
// startup queries (OSC color, cursor position, focus reporting, …). These
// arrive as KeyMsgs because the bubbletea version we depend on doesn't parse
// the OSC/CSI responses internally.
func isTerminalNoise(msg tea.KeyMsg, mountedAt time.Time) bool {
	// Alt-modified keys: we never bind anything to alt+X, and the terminal's
	// escape responses always start with alt+] or alt+\\ (the leading ESC byte).
	// However, we must allow alt+left, alt+right, alt+b, alt+f, alt+pgup, alt+pgdown for navigation.
	if msg.Alt {
		s := msg.String()
		if s == "alt+left" || s == "alt+right" || s == "alt+b" || s == "alt+f" || s == "alt+pgup" || s == "alt+pgdown" {
			return false
		}
		return true
	}
	// Some terminals deliver paste/drop payloads as one multi-rune KeyRunes
	// event. Keep those, but still drop obvious OSC/CSI response chunks.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
		if looksLikeDroppedPathChunk(string(msg.Runes)) {
			return false
		}
		return true
	}
	// Belt-and-suspenders: drop anything in the first 400ms after the chat
	// screen mounts, which is when the bulk of the noise burst lands.
	if !mountedAt.IsZero() && time.Since(mountedAt) < 400*time.Millisecond {
		return true
	}
	return false
}

func looksLikeDroppedPathChunk(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(s, "file://") {
		return true
	}
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if _, ok := parseDroppedAttachmentPath(line); ok {
			return true
		}
	}
	return false
}

func prevWordStart(val string, pos int) int {
	if pos <= 0 {
		return 0
	}
	runes := []rune(val)
	if pos > len(runes) {
		pos = len(runes)
	}

	i := pos - 1
	// Skip trailing whitespace
	for i >= 0 && (runes[i] == ' ' || runes[i] == '\t' || runes[i] == '\n' || runes[i] == '\r') {
		i--
	}
	// Skip word characters
	if i >= 0 {
		isWordChar := (runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') || (runes[i] >= '0' && runes[i] <= '9') || runes[i] == '_'
		for i >= 0 && ((runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') || (runes[i] >= '0' && runes[i] <= '9') || runes[i] == '_') == isWordChar && !(runes[i] == ' ' || runes[i] == '\t' || runes[i] == '\n' || runes[i] == '\r') {
			i--
		}
	}
	return i + 1
}

func nextWordStart(val string, pos int) int {
	runes := []rune(val)
	if pos >= len(runes) {
		return len(runes)
	}

	i := pos
	// Skip word characters
	isWordChar := (runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') || (runes[i] >= '0' && runes[i] <= '9') || runes[i] == '_'
	for i < len(runes) && ((runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') || (runes[i] >= '0' && runes[i] <= '9') || runes[i] == '_') == isWordChar && !(runes[i] == ' ' || runes[i] == '\t' || runes[i] == '\n' || runes[i] == '\r') {
		i++
	}
	// Skip trailing whitespace to reach the start of the next word
	for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t' || runes[i] == '\n' || runes[i] == '\r') {
		i++
	}
	return i
}

func renderFileCmd(path string, width int) tea.Cmd {
	return func() tea.Msg {
		out := renderFileForWidth(path, width)
		return fileRenderedMsg{path: path, content: out}
	}
}

func (m *model) currentFilesContext() string {
	files := listWorkspaceFiles(m.workDir)
	if len(files) == 0 {
		return "Files currently existing in the notes directory: (none yet)"
	}
	return "Files currently existing in the notes directory:\n- " + strings.Join(files, "\n- ")
}

func listWorkspaceFiles(dir string) []string {
	entries := readTreeEntries(dir)
	var files []string
	var walk func([]treeEntry, string)
	walk = func(items []treeEntry, prefix string) {
		for _, item := range items {
			rel := item.name
			if prefix != "" {
				rel = prefix + "/" + item.name
			}
			if item.isDir {
				walk(item.children, rel)
			} else {
				files = append(files, rel)
			}
		}
	}
	walk(entries, "")
	return files
}
