package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"rune/internal/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// ---- Layout ----

func (m *model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	m.layoutPanes()

	if m.state == stateChat {
		// Rebuild the markdown renderer for assistant messages whenever the
		// chat pane width changes, so word-wrap stays correct. Glamour setup is
		// expensive, so never do this while the loading screen is trying to
		// paint immediately.
		if r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.chatView.Width-2),
		); err == nil {
			m.chatRenderer = r
		}
		m.refreshChatView()
	}
}

func (m *model) layoutPanes() {
	paneWidth := max(20, m.width-4)
	paneHeight := m.height - 5
	composerHeight := m.chatComposerHeight()

	m.chatInput.SetWidth(max(20, paneWidth-8))
	m.chatInput.SetHeight(composerHeight)
	m.settingsProvider.Width = max(20, m.width-8)
	m.settingsCodexCommand.Width = max(20, m.width-8)
	m.settingsCodexModel.Width = max(20, m.width-8)
	m.settingsAntigravityCommand.Width = max(20, m.width-8)
	m.settingsAntigravityModel.Width = max(20, m.width-8)
	m.settingsClaudeCommand.Width = max(20, m.width-8)
	m.settingsClaudeModel.Width = max(20, m.width-8)
	m.settingsEditor.Width = max(20, m.width-8)
	m.settingsProfile.SetWidth(max(20, m.width-8))
	m.settingsProfile.SetHeight(max(6, m.height-12))

	m.chatView.Width = paneWidth - 2
	m.notesView.Width = paneWidth - 2

	inputBoxHeight := lipgloss.Height(lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("99")).
		Padding(0, 1).
		Render(m.chatInput.View()))
	attachmentsHeight := 0
	if attachments := m.renderComposerAttachments(paneWidth - 2); attachments != "" {
		attachmentsHeight = lipgloss.Height(attachments)
	}
	livePreviewNoticeHeight := 0
	if notice := m.renderLivePreviewNotice(paneWidth - 2); notice != "" {
		livePreviewNoticeHeight = lipgloss.Height(notice)
	}

	leftChromeHeight := 1 + livePreviewNoticeHeight + inputBoxHeight + attachmentsHeight + 1
	m.chatView.Height = max(6, paneHeight-2-leftChromeHeight)
	m.notesView.Height = paneHeight - 2
}

func (m *model) chatComposerHeight() int {
	lines := strings.Count(m.chatInput.Value(), "\n") + 1
	return max(3, min(6, lines))
}

// ---- Styles ----

var (
	userBadge  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("» you")
	aiBadge    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render("◉ ai")
	sysStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	streamHint = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("▍")
	userBody   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	pickerLabel    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	pickerItem     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	pickerSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57"))
)

// ---- Chat pane rendering ----

func (m *model) renderMarkdown(s string) string {
	if m.chatRenderer == nil {
		return s
	}
	out, err := m.chatRenderer.Render(s)
	if err != nil {
		return s
	}
	return strings.Trim(out, "\n")
}

func (m *model) refreshChatView() {
	var b strings.Builder
	msgs := m.displayMsgs
	if len(msgs) > maxRenderedChatMessages {
		skipped := len(msgs) - maxRenderedChatMessages
		b.WriteString(sysStyle.Render(fmt.Sprintf("showing latest %d messages (%d earlier in session history)", maxRenderedChatMessages, skipped)) + "\n\n")
		msgs = msgs[skipped:]
	}
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			b.WriteString(userBadge + "\n")
			b.WriteString(userBody.Width(max(20, m.chatView.Width-2)).Render(msg.Content) + "\n\n")
		case "assistant":
			b.WriteString(aiBadge + "\n")
			b.WriteString(m.renderMarkdown(msg.Content) + "\n\n")
		case "system":
			b.WriteString(sysStyle.Width(max(20, m.chatView.Width-2)).Render(msg.Content) + "\n\n")
		}
	}
	if m.streaming {
		b.WriteString(aiBadge + "\n")
		// splitInProgressFile already runs displayClean on the buffer, so
		// completed FILE blocks come back as "wrote NAME" markers and only an
		// in-progress block (if any) remains as a raw open tag.
		cleaned, name, _, inProgress := core.SplitInProgressFile(m.pendingAsst)
		switch {
		case inProgress:
			cleaned = strings.TrimRight(cleaned, "\n")
			if cleaned != "" {
				b.WriteString(cleaned + " " + streamHint + "\n")
			} else {
				b.WriteString(sysStyle.Render(fmt.Sprintf("writing %s in preview…", name)) + "\n")
			}
		case strings.TrimSpace(cleaned) == "":
			b.WriteString(sysStyle.Render("thinking… ") + streamHint + "\n")
		default:
			b.WriteString(cleaned + " " + streamHint + "\n")
		}
	}
	if m.optionsActive && !m.streaming {
		labelWidth := max(20, m.chatView.Width-2)
		b.WriteString(pickerLabel.Width(labelWidth).Render(
			"choose an option  —  ↑↓ / 1–9 to pick • enter to confirm • esc to type freely",
		) + "\n")
		itemWidth := max(20, m.chatView.Width-2)
		for i, opt := range m.options {
			b.WriteString(renderPickerOption(i+1, opt, i == m.optionCursor, itemWidth))
		}
		b.WriteString("\n")
	}
	m.chatView.SetContent(b.String())
	m.chatView.GotoBottom()
}

const maxRenderedChatMessages = 18

func renderPickerOption(index int, text string, selected bool, width int) string {
	prefix := fmt.Sprintf("  %d. ", index)
	if selected {
		prefix = fmt.Sprintf("› %d. ", index)
	}
	available := max(8, width-len(prefix))
	lines := wrapPlainText(text, available)
	if len(lines) == 0 {
		lines = []string{""}
	}

	style := pickerItem
	if selected {
		style = pickerSelected
	}

	var b strings.Builder
	for i, line := range lines {
		linePrefix := prefix
		if i > 0 {
			linePrefix = strings.Repeat(" ", len(prefix))
		}
		b.WriteString(style.Render(linePrefix+line) + "\n")
	}
	return b.String()
}

func wrapPlainText(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := ""
	for _, word := range words {
		if current == "" {
			for len(word) > width {
				lines = append(lines, word[:width])
				word = word[width:]
			}
			current = word
			continue
		}
		if len(current)+1+len(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		for len(word) > width {
			lines = append(lines, word[:width])
			word = word[width:]
		}
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// ---- Top-level View ----

func (m *model) View() string {
	if m.showReader {
		return m.viewReaderScreen()
	}
	if m.showSettings {
		return m.viewSettingsScreen()
	}
	if m.showModelSwitcher {
		return m.viewModelSwitcher()
	}
	if m.showLogs {
		return m.viewLogScreen()
	}
	if m.state == stateTopicInput {
		return m.viewTopicScreen()
	}
	if m.state == stateLoading {
		return m.viewLoadingScreen()
	}

	if !m.ready {
		return "Initializing..."
	}

	paneWidth := max(20, m.width-4)
	paneHeight := m.height - 5

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")).Padding(0, 1)
	loc := m.providerModeLabel()
	// Compose a phase tag like " · skill · pre-session" so the user always
	// knows where they are in the pipeline.
	phase := ""
	switch {
	case m.sessionType != "" && m.preSession:
		phase = fmt.Sprintf(" · %s · pre-session", m.sessionType)
	case m.sessionType != "":
		phase = fmt.Sprintf(" · %s", m.sessionType)
	}
	if m.sessionLabel != "" {
		phase += " · " + m.sessionLabel
	}
	header := headerStyle.Render(fmt.Sprintf("RUNE — %s  (%s)  provider: %s  model: %s [%s]%s",
		m.topic, m.workDir, m.providerDisplay(), m.runtimeLabel(), loc, phase))

	paneStyle := func(active bool) lipgloss.Style {
		borderColor := lipgloss.Color("62")
		if active {
			borderColor = lipgloss.Color("213")
		}
		return lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Width(paneWidth).
			Height(paneHeight)
	}
	activeLabel := func(label string, active bool, width int) string {
		text := truncatePlain(label, max(8, width))
		style := lipgloss.NewStyle().Width(width).MaxWidth(width)
		if active {
			style = style.Bold(true).Foreground(lipgloss.Color("213"))
		}
		return style.Render(text)
	}

	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(strings.Repeat("─", paneWidth-2))
	inputHint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("enter: send  •  shift+enter / ctrl+j: newline")
	inputBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("99")).
		Padding(0, 1).
		Render(m.chatInput.View())
	attachments := m.renderComposerAttachments(paneWidth - 2)
	livePreviewNotice := m.renderLivePreviewNotice(paneWidth - 2)

	leftInner := lipgloss.JoinVertical(lipgloss.Left,
		m.chatView.View(),
		livePreviewNotice,
		sep,
		inputBox,
		attachments,
		inputHint,
	)
	leftPane := paneStyle(m.activePane == 0).Render(leftInner)

	var rightInner string
	var rightLabel string
	if m.showExplorer {
		rightInner = m.renderExplorer(paneWidth-2, paneHeight-2)
		rightLabel = " [2] Explorer "
	} else {
		rightInner = m.notesView.View()
		previewName := filepath.Base(m.lastFile)
		if m.lastFile == "" {
			previewName = "(no file yet)"
		}
		if m.writingFile != "" {
			rightLabel = fmt.Sprintf(" [2] Writing: %s ", m.writingFile)
		} else {
			rightLabel = fmt.Sprintf(" [2] Notes Preview: %s ", previewName)
		}
	}
	rightPane := paneStyle(m.activePane == 1).Render(rightInner)

	var panes string
	if m.activePane == 0 {
		panes = lipgloss.JoinVertical(lipgloss.Left, activeLabel(" [1] Chat ", true, paneWidth), leftPane)
	} else {
		panes = lipgloss.JoinVertical(lipgloss.Left, activeLabel(strings.TrimSpace(rightLabel), true, paneWidth), rightPane)
	}

	statusLine := ""
	if m.streaming {
		statusLine = "  • streaming…"
	} else if m.optionsActive {
		statusLine = "  • picker active (↑↓ / 1–9 / enter / esc)"
	}

	row1 := " ctrl+c: quit  •  tab: switch pane"
	row2 := " ctrl+o: home  •  ctrl+e: explorer  •  ctrl+t: settings  •  ctrl+k: switch model  •  ctrl+r: rewind  •  ctrl+l: logs" + statusLine
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	helpLines := []string{
		helpStyle.Render(row1),
		helpStyle.Render(row2),
	}
	help := lipgloss.JoinVertical(lipgloss.Left, helpLines...)

	return lipgloss.JoinVertical(lipgloss.Left, header, panes, help)
}



func (m *model) renderComposerAttachments(width int) string {
	if len(m.composerAttachmentPaths) == 0 {
		return ""
	}
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true).Render("attachments")
	chipStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)
	var chips []string
	for _, path := range m.composerAttachmentPaths {
		kind := "file"
		if supportedImageExt[strings.ToLower(filepath.Ext(path))] {
			kind = "image"
		}
		chips = append(chips, chipStyle.Render(kind+": "+truncatePlain(filepath.Base(path), max(12, width/3))))
	}
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("backspace on empty input removes last attachment")
	return lipgloss.JoinVertical(lipgloss.Left, label, lipgloss.JoinHorizontal(lipgloss.Left, chips...), help)
}

func (m *model) renderLivePreviewNotice(width int) string {
	if m.activePane != 0 {
		return ""
	}
	text := m.fileNotice
	if m.writingFile != "" {
		text = fmt.Sprintf("writing %s live in preview - press tab to view", filepath.Base(m.writingFile))
	}
	if text == "" {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("57")).
		Bold(true).
		Width(max(20, width)).
		Padding(0, 1).
		Render(truncatePlain(text, max(20, width-2)))
}

// ---- Topic-selection screen ----

func (m *model) viewTopicScreen() string {
	accentA := lipgloss.Color("213")
	accentB := lipgloss.Color("99")
	accentC := lipgloss.Color("81")
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(accentC)
	cardBorder := lipgloss.Color("62")
	selectedItem := lipgloss.NewStyle().
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("57")).
		Bold(true)
	itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	width := 96
	if m.width > 0 {
		width = min(116, max(72, m.width-8))
	}
	listHeight := 8
	if m.height > 0 {
		listHeight = max(4, min(10, m.height-24))
	}

	mode := "off"
	if m.config.PersonalizedMode {
		mode = "on"
	}
	profileState := "empty"
	if strings.TrimSpace(m.config.PersonalProfile) != "" {
		profileState = "saved"
	}
	logo := renderRuneLogo(width - 8)
	tagline := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("254")).Render("Terminal notes for learning and research")
	subtitle := mutedStyle.Render("Live preview • reusable context • focused topic sessions")

	hero := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(accentB).
		Padding(1, 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, logo, tagline, subtitle))

	statsLine := lipgloss.JoinHorizontal(lipgloss.Top,
		homeBadge("Provider", m.providerDisplay(), accentA),
		homeBadge("Model", truncatePlain(m.runtimeLabel(), 28), accentB),
		homeBadge("Mode", m.providerModeLabel(), accentC),
		homeBadge("Profile", profileState, lipgloss.Color("111")),
		homeBadge("Topics", fmt.Sprintf("%d", len(m.topicList)), lipgloss.Color("75")),
	)

	var recent strings.Builder
	recent.WriteString(labelStyle.Render("Recent Topics") + "\n")
	if len(m.topicList) > 0 {
		start := 0
		if m.topicCursor >= listHeight {
			start = m.topicCursor - listHeight + 1
		}
		end := min(len(m.topicList), start+listHeight)
		if start > 0 {
			recent.WriteString(metaStyle.Render(fmt.Sprintf("... %d above", start)) + "\n")
		}
		for i := start; i < end; i++ {
			t := truncatePlain(m.topicList[i], 36)
			if i == m.topicCursor {
				recent.WriteString(selectedItem.Width(40).Render("› "+t) + "\n")
			} else {
				recent.WriteString(itemStyle.Render("  "+t) + "\n")
			}
		}
		if end < len(m.topicList) {
			recent.WriteString(metaStyle.Render(fmt.Sprintf("... %d more", len(m.topicList)-end)) + "\n")
		}
	} else {
		recent.WriteString(metaStyle.Render("No saved topics yet. Start with a fresh one.") + "\n")
	}

	recentCard := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(cardBorder).
		Padding(1, 2).
		Width(46).
		Render(recent.String())

	configBody := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Session Setup"),
		textStyle.Render(fmt.Sprintf("Personalized mode: %s", mode)),
		textStyle.Render(fmt.Sprintf("Notes directory: %s", truncatePlain(notesRoot(), 42))),
		textStyle.Render(""),
		labelStyle.Render("New Topic"),
		m.topicInput.View(),
		metaStyle.Render("Examples: Java, Kubernetes, Linear Algebra, System Design"),
	)
	if m.topicCursor < 0 {
		configBody = lipgloss.JoinVertical(lipgloss.Left,
			labelStyle.Render("Session Setup"),
			textStyle.Render(fmt.Sprintf("Personalized mode: %s", mode)),
			textStyle.Render(fmt.Sprintf("Notes directory: %s", truncatePlain(notesRoot(), 42))),
			textStyle.Render(""),
			selectedItem.Render("› New Topic"),
			m.topicInput.View(),
			metaStyle.Render("Examples: Java, Kubernetes, Linear Algebra, System Design"),
		)
	}
	setupCard := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(cardBorder).
		Padding(1, 2).
		Width(46).
		Render(configBody)

	content := lipgloss.JoinHorizontal(lipgloss.Top, recentCard, "  ", setupCard)

	footer := mutedStyle.Render("enter: open  •  ↑/↓: select topic  •  type: create new topic  •  ctrl+t: settings  •  ctrl+c: quit")

	shell := lipgloss.NewStyle().
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(accentB).
		Padding(1, 2).
		Width(width).
		Render(lipgloss.JoinVertical(lipgloss.Left, hero, "", statsLine, "", content, "", footer))

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, shell)
	}
	return shell
}

func renderRuneLogo(maxWidth int) string {
	lines := []string{
		"██████╗ ██╗   ██╗███╗   ██╗███████╗",
		"██╔══██╗██║   ██║████╗  ██║██╔════╝",
		"██████╔╝██║   ██║██╔██╗ ██║█████╗  ",
		"██╔══██╗██║   ██║██║╚██╗██║██╔══╝  ",
		"██║  ██║╚██████╔╝██║ ╚████║███████╗",
		"╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚══════╝",
	}
	colors := []lipgloss.Color{"213", "177", "141", "105", "69", "63"}
	var out []string
	for i, line := range lines {
		styled := lipgloss.NewStyle().Bold(true).Foreground(colors[i%len(colors)]).Render(line)
		out = append(out, styled)
	}
	logo := strings.Join(out, "\n")
	if maxWidth > 0 {
		return lipgloss.NewStyle().MaxWidth(maxWidth).Render(logo)
	}
	return logo
}

func homeBadge(label, value string, border lipgloss.Color) string {
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(border)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Render(labelStyle.Render(label) + "  " + valueStyle.Render(value))
}

func notesRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/notes"
	}
	return filepath.Join(home, "notes")
}

func truncatePlain(s string, maxLen int) string {
	if maxLen < 4 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

func (m *model) viewSettingsScreen() string {
	if m.width <= 0 || m.height <= 0 {
		return "settings"
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	onStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	offStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))

	mode := offStyle.Render("off")
	if m.config.PersonalizedMode {
		mode = onStyle.Render("on")
	}

	header := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("SETTINGS"),
		fmt.Sprintf("%s %s", labelStyle.Render("Personalized mode:"), mode),
		helpStyle.Render("tab: switch field  •  ctrl+p: toggle  •  esc/ctrl+t: save and close"),
		"",
		labelStyle.Render("Provider"),
		helpStyle.Render("Choose backend: ollama, codex, claude, antigravity, or auto."),
		m.settingsProvider.View(),
		"",
		labelStyle.Render("Codex command"),
		helpStyle.Render("CLI used for Codex backend, typically just: codex"),
		m.settingsCodexCommand.View(),
		"",
		labelStyle.Render("Codex model"),
		helpStyle.Render("Optional. Leave empty to use Codex default model."),
		m.settingsCodexModel.View(),
		"",
		labelStyle.Render("Antigravity command"),
		helpStyle.Render("CLI used for Antigravity backend, typically just: agy"),
		m.settingsAntigravityCommand.View(),
		"",
		labelStyle.Render("Antigravity model"),
		helpStyle.Render("Optional. Leave empty to use Antigravity default model."),
		m.settingsAntigravityModel.View(),
		"",
		labelStyle.Render("Claude command"),
		helpStyle.Render("CLI used for Claude backend, typically just: claude"),
		m.settingsClaudeCommand.View(),
		"",
		labelStyle.Render("Claude model"),
		helpStyle.Render("Optional. Examples: sonnet, opus, or a full Claude model name."),
		m.settingsClaudeModel.View(),
		"",
		labelStyle.Render("Document editor"),
		helpStyle.Render("Used by reader edit mode. Examples: vim, nano, emacs, code --wait."),
		m.settingsEditor.View(),
		"",
		labelStyle.Render("User keypoints"),
		helpStyle.Render("Keep stable facts here: education, experience, goals, preferences, recurring context."),
	)

	boxWidth := max(40, m.width-6)
	boxHeight := max(12, m.height-4)
	profileHeight := max(4, min(10, boxHeight-38))
	m.settingsProfile.SetHeight(profileHeight)

	body := lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.settingsProfile.View(),
	)

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("57")).
		Padding(1, 2).
		Width(boxWidth).
		Height(boxHeight).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *model) viewModelSwitcher() string {
	if m.width <= 0 || m.height <= 0 {
		return "model switcher"
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57"))

	var lines []string
	lines = append(lines, titleStyle.Render("PROVIDER / MODEL SWITCHER"))
	lines = append(lines, metaStyle.Render("ctrl+k/esc: close  •  ↑↓: choose provider+model  •  enter: apply"))
	lines = append(lines, "")
	for i, preset := range m.modelSwitcherPresets() {
		line := fmt.Sprintf("%s — %s", preset.label, preset.description)
		if i == m.switcherCursor {
			lines = append(lines, selectedStyle.Render("› "+line))
		} else {
			lines = append(lines, labelStyle.Render("  "+line))
		}
	}

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("57")).
		Padding(1, 2).
		Width(min(72, max(44, m.width-10))).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *model) viewReaderScreen() string {
	if m.width <= 0 || m.height <= 0 {
		return "reader"
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	name := "(no file)"
	if m.lastFile != "" {
		name = filepath.Base(m.lastFile)
	}
	title := titleStyle.Render("READER")
	path := pathStyle.Render(name)
	mode := "rendered markdown"
	if m.readerRawMode {
		mode = "raw markdown"
	}
	modeLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render(mode)
	help := helpStyle.Render("m: toggle raw/render  •  esc/q: back  •  up/down/pgup/pgdn: scroll  •  g/G: top/bottom")
	if m.readerStatus != "" {
		help = helpStyle.Render(m.readerStatus + "  •  e: edit  •  m: toggle raw/render  •  esc/q: back")
	} else {
		help = helpStyle.Render("e: edit  •  m: toggle raw/render  •  esc/q: back  •  up/down/pgup/pgdn: scroll  •  g/G: top/bottom")
	}

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("57")).
		Width(m.width - 2).
		Height(m.height - 4).
		Render(m.readerView.View())

	return lipgloss.JoinVertical(lipgloss.Left,
		title+"  "+path+"  "+modeLabel,
		box,
		help,
	)
}

// ---- Loading screen ----

func (m *model) viewLoadingScreen() string {
	spinner := loadingSpinner(m.loadingFrame)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Render("RUNE")
	msg := m.loadingMessage
	if msg == "" {
		msg = "Preparing…"
	}
	msgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	spinStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("ctrl+c to cancel")

	body := lipgloss.JoinVertical(lipgloss.Center,
		fmt.Sprintf("%s  %s", spinStyle.Render(spinner), title),
		msgStyle.Render(msg),
		"",
		hint,
	)

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("57")).
		Padding(1, 4).
		Render(body)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

func loadingSpinner(frame int) string {
	frames := []string{
		"*.......",
		".*......",
		"..*.....",
		"...*....",
		"....*...",
		".....*..",
		"......*.",
		".......*",
	}
	return frames[frame%len(frames)]
}

// ---- Log overlay (Ctrl+L) ----

// logTailBytes caps how much of the log file we read into the viewer; older
// entries beyond this are simply scrolled off. Plenty for a session's worth
// of debug output and keeps the viewport snappy.
const logTailBytes = 256 * 1024

// refreshLogView reads the tail of LogPath into the log viewport. Called when
// the user toggles the overlay on or presses 'r' to refresh.
func (m *model) refreshLogView() {
	w := m.width
	h := m.height
	if w < 20 {
		w = 20
	}
	if h < 6 {
		h = 6
	}
	// Leave room for our 1-line title + 1-line help footer + box borders.
	m.logView.Width = w - 4
	m.logView.Height = h - 6

	content := readLogTail(LogPath, logTailBytes)
	if content == "" {
		content = "(no log output yet — actions you take in the app will appear here)"
	}
	m.logView.SetContent(content)
	m.logView.GotoBottom()
}

// readLogTail returns at most maxBytes from the end of path. If the file is
// smaller, the whole thing is returned. Errors collapse to a friendly string
// so the viewer never crashes the UI.
func readLogTail(path string, maxBytes int64) string {
	if path == "" {
		return "(log path not configured)"
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Sprintf("(could not open %s: %v)", path, err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return fmt.Sprintf("(could not stat %s: %v)", path, err)
	}
	size := stat.Size()
	offset := int64(0)
	if size > maxBytes {
		offset = size - maxBytes
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return fmt.Sprintf("(seek failed: %v)", err)
	}
	buf := make([]byte, size-offset)
	if _, err := f.Read(buf); err != nil {
		return fmt.Sprintf("(read failed: %v)", err)
	}
	out := string(buf)
	// Drop a partial first line when we sliced into the middle of one.
	if offset > 0 {
		if i := strings.IndexByte(out, '\n'); i >= 0 {
			out = out[i+1:]
		}
	}
	return out
}

// writingSpinnerLine returns a one-line "writing X… (size)" status string for
// the chat pane while the model is mid-stream of a <<<FILE>>> block. The
// spinner glyph is derived from the body length so it self-animates as more
// chunks arrive, without needing a tea.Tick.
func writingSpinnerLine(name, partial string) string {
	glyphs := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	g := glyphs[(len(partial)/8)%len(glyphs)]

	spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)

	return fmt.Sprintf("%s %s %s   %s",
		spinnerStyle.Render(string(g)),
		labelStyle.Render("writing"),
		nameStyle.Render(name),
		metaStyle.Render(fmt.Sprintf("· %s · live preview on the right →", humanSize(int64(len(partial))))),
	)
}

// ---- Right-pane explorer & initial preview priming ----

// workDirEntry is used by preview priming to find the newest visible file.
type workDirEntry struct {
	name    string
	path    string
	size    int64
	modTime time.Time
	isDir   bool
}

type treeEntry struct {
	name     string
	path     string
	size     int64
	modTime  time.Time
	isDir    bool
	children []treeEntry
}

type explorerLine struct {
	text  string
	path  string
	isDir bool
}

// listWorkDirFiles enumerates visible files under dir, newest first. It is
// intentionally recursive so preview priming works with folder-based notes.
func listWorkDirFiles(dir string) []workDirEntry {
	var out []workDirEntry
	collectWorkDirFiles(dir, &out)
	sort.Slice(out, func(i, j int) bool { return out[i].modTime.After(out[j].modTime) })
	return out
}

func collectWorkDirFiles(dir string, out *[]workDirEntry) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if e.IsDir() {
			collectWorkDirFiles(path, out)
			continue
		}
		*out = append(*out, workDirEntry{
			name:    name,
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
			isDir:   false,
		})
	}
}

func buildWorkDirTree(dir string) ([]treeEntry, int) {
	entries := readTreeEntries(dir)
	return entries, countTreeEntries(entries)
}

func readTreeEntries(dir string) []treeEntry {
	if dir == "" {
		return nil
	}
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]treeEntry, 0, len(dirEntries))
	for _, e := range dirEntries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		item := treeEntry{
			name:    name,
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
			isDir:   e.IsDir(),
		}
		if item.isDir {
			item.children = readTreeEntries(path)
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].isDir != out[j].isDir {
			return out[i].isDir
		}
		return strings.ToLower(out[i].name) < strings.ToLower(out[j].name)
	})
	return out
}

func countTreeEntries(entries []treeEntry) int {
	total := 0
	for _, e := range entries {
		total++
		if e.isDir {
			total += countTreeEntries(e.children)
		}
	}
	return total
}

// primePreview returns a Cmd that emits a fileChangedMsg for the most
// recently modified file in workDir, so the right pane is populated
// immediately when entering chat (instead of waiting for the next write
// event). If the dir is empty, no message is emitted.
func (m *model) primePreview() tea.Cmd {
	dir := m.workDir
	if dir == "" {
		return nil
	}
	return func() tea.Msg {
		entries := listWorkDirFiles(dir)
		for _, e := range entries {
			if e.isDir {
				continue
			}
			return fileChangedMsg(e.path)
		}
		return nil
	}
}

// renderExplorer formats workDir as a tree, similar to editor sidebars. It
// shows folders before files and recurses through non-hidden subdirectories.
func (m *model) renderExplorer(width, height int) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dirStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	entries, total := buildWorkDirTree(m.workDir)

	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf(" Explorer · %s ", m.workDir)) + "\n")
	if len(entries) == 0 {
		b.WriteString("\n")
		b.WriteString(emptyStyle.Render("  (no files yet — once the AI writes one it will appear here)"))
		return b.String()
	}
	b.WriteString(metaStyle.Render(fmt.Sprintf("  %d item(s) · tree view · d delete · a rename", total)) + "\n\n")

	root := filepath.Base(m.workDir)
	if root == "." || root == string(filepath.Separator) {
		root = m.workDir
	}
	b.WriteString(dirStyle.Render("  "+root+"/") + "\n")

	lines := renderTreeLines(entries, "", width, nameStyle, dirStyle, metaStyle)
	footer := m.renderExplorerFooter(metaStyle)
	maxLines := height - 6
	if footer == "" {
		maxLines = height - 5
	}
	if maxLines < 1 {
		maxLines = len(lines)
	}
	if m.explorerCursor > len(lines)-1 {
		m.explorerCursor = max(0, len(lines)-1)
	}
	if m.explorerScroll > m.explorerCursor {
		m.explorerScroll = m.explorerCursor
	}
	if m.explorerCursor >= m.explorerScroll+maxLines {
		m.explorerScroll = m.explorerCursor - maxLines + 1
	}
	start := m.explorerScroll
	if start > 0 {
		b.WriteString(metaStyle.Render(fmt.Sprintf("  ... %d above", start)) + "\n")
		maxLines--
	}
	for i := start; i < len(lines); i++ {
		if i-start >= maxLines {
			b.WriteString(metaStyle.Render(fmt.Sprintf("  ... %d more", len(lines)-i)) + "\n")
			break
		}
		line := lines[i].text
		if i == m.explorerCursor && m.activePane == 1 {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57")).Bold(true).Render(line)
		}
		b.WriteString(line + "\n")
	}
	if footer != "" {
		b.WriteString("\n")
		b.WriteString(footer)
	}
	return b.String()
}

func (m *model) renderExplorerFooter(metaStyle lipgloss.Style) string {
	switch {
	case m.renameTarget != "":
		return metaStyle.Render("  rename: ") + m.renameInput.View()
	case m.deleteTarget != "":
		name := filepath.Base(m.deleteTarget)
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("88")).
			Bold(true).
			Render(fmt.Sprintf("  Delete %q?  y confirm · n/esc cancel  ", name))
	case m.explorerStatus != "":
		return metaStyle.Render("  " + m.explorerStatus)
	default:
		return ""
	}
}

func renderTreeLines(entries []treeEntry, prefix string, width int, nameStyle, dirStyle, metaStyle lipgloss.Style) []explorerLine {
	lines := make([]explorerLine, 0, len(entries))
	for i, e := range entries {
		last := i == len(entries)-1
		branch := "├── "
		nextPrefix := prefix + "│   "
		if last {
			branch = "└── "
			nextPrefix = prefix + "    "
		}
		name := e.name
		if e.isDir {
			name += "/"
		}
		meta := ""
		if !e.isDir {
			meta = fmt.Sprintf("  %s  %s ago", humanSize(e.size), humanAgo(time.Since(e.modTime)))
		}
		name = truncateExplorerName(name, width-len(prefix)-len(branch)-len(meta)-5)
		styledName := nameStyle.Render(name)
		if e.isDir {
			styledName = dirStyle.Render(name)
		}
		lines = append(lines, explorerLine{
			text:  fmt.Sprintf("  %s%s%s%s", prefix, branch, styledName, metaStyle.Render(meta)),
			path:  e.path,
			isDir: e.isDir,
		})
		if e.isDir && len(e.children) > 0 {
			lines = append(lines, renderTreeLines(e.children, nextPrefix, width, nameStyle, dirStyle, metaStyle)...)
		}
	}
	return lines
}

func (m *model) explorerLines(width int) []explorerLine {
	entries, _ := buildWorkDirTree(m.workDir)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dirStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	return renderTreeLines(entries, "", width, nameStyle, dirStyle, metaStyle)
}

func truncateExplorerName(name string, maxWidth int) string {
	if maxWidth < 8 || len(name) <= maxWidth {
		return name
	}
	return name[:maxWidth-1] + "…"
}

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func humanAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// viewLogScreen renders the log overlay as a full-screen scrollable pane.
func (m *model) viewLogScreen() string {
	if m.width <= 0 || m.height <= 0 {
		return "loading…"
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	title := titleStyle.Render("LOGS")
	path := pathStyle.Render(LogPath)
	help := helpStyle.Render(
		"ctrl+l/esc/q: close  •  pgup/pgdn (j/k): scroll  •  g/G: top/bottom  •  r: refresh")

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("57")).
		Width(m.width - 2).
		Height(m.height - 4).
		Render(m.logView.View())

	return lipgloss.JoinVertical(lipgloss.Left,
		title+"  "+path,
		box,
		help,
	)
}
