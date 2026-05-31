package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	leftPaneWidth, rightPaneWidth := m.splitPaneWidths()
	paneHeight := m.height - 4

	m.chatView.Width = leftPaneWidth - 2
	m.chatView.Height = paneHeight - 4

	m.notesView.Width = rightPaneWidth - 2
	m.notesView.Height = paneHeight - 2

	m.chatInput.Width = leftPaneWidth - 6
	m.settingsProfile.SetWidth(max(20, m.width-8))
	m.settingsProfile.SetHeight(max(6, m.height-12))
}

func (m *model) splitPaneWidths() (int, int) {
	available := m.width - 4
	if available < 20 {
		return max(10, available/2), max(10, available/2)
	}
	percent := m.splitPercent
	if percent == 0 {
		percent = 70
	}
	percent = max(25, min(75, percent))
	left := available * percent / 100
	right := available - left
	if right < 24 && available > 64 {
		right = 24
		left = available - right
	}
	if left < 40 && available > 64 {
		left = 40
		right = available - left
	}
	return left, right
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
		cleaned, name, _, inProgress := splitInProgressFile(m.pendingAsst)
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

	leftPaneWidth, rightPaneWidth := m.splitPaneWidths()
	paneHeight := m.height - 4

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")).Padding(0, 1)
	loc := "local"
	if m.usingCloud {
		loc = "cloud"
	}
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
	header := headerStyle.Render(fmt.Sprintf("NOTES MAKER — %s  (%s)  model: %s [%s]%s",
		m.topic, m.workDir, m.modelName, loc, phase))

	paneStyle := func(active bool) lipgloss.Style {
		borderColor := lipgloss.Color("62")
		if active {
			borderColor = lipgloss.Color("213")
		}
		return lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Width(leftPaneWidth).
			Height(paneHeight)
	}
	rightPaneStyle := func(active bool) lipgloss.Style {
		borderColor := lipgloss.Color("62")
		if active {
			borderColor = lipgloss.Color("213")
		}
		return lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Width(rightPaneWidth).
			Height(paneHeight)
	}
	activeLabel := func(label string, active bool) string {
		if !active {
			return label
		}
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Render(label)
	}

	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(strings.Repeat("─", leftPaneWidth-2))
	inputLine := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render("> ") + m.chatInput.View()

	leftInner := lipgloss.JoinVertical(lipgloss.Left,
		m.chatView.View(),
		sep,
		inputLine,
	)
	leftPane := paneStyle(m.activePane == 0).Render(leftInner)

	var rightInner string
	var rightLabel string
	if m.showExplorer {
		rightInner = m.renderExplorer(rightPaneWidth-2, paneHeight-2)
		rightLabel = " [2] Explorer "
	} else {
		rightInner = m.notesView.View()
		previewName := filepath.Base(m.lastFile)
		if m.lastFile == "" {
			previewName = "(no file yet)"
		}
		rightLabel = fmt.Sprintf(" [2] Notes Preview: %s ", previewName)
	}
	rightPane := rightPaneStyle(m.activePane == 1).Render(rightInner)

	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left, activeLabel(" [1] Chat ", m.activePane == 0), leftPane),
		lipgloss.JoinVertical(lipgloss.Left, activeLabel(rightLabel, m.activePane == 1), rightPane),
	)

	statusLine := ""
	if m.streaming {
		statusLine = "  • streaming…"
	} else if m.optionsActive {
		statusLine = "  • picker active (↑↓ / 1–9 / enter / esc)"
	}
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(
		" ctrl+c: quit  •  tab: switch pane  •  enter: reader/preview file  •  arrows/pgup/pgdn: scroll active pane  •  ctrl+r: rewind  •  ctrl+e: explorer  •  ctrl+t: settings  •  ctrl+l: logs" + statusLine)

	return lipgloss.JoinVertical(lipgloss.Left, header, panes, help)
}

// ---- Topic-selection screen ----

func (m *model) viewTopicScreen() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
	itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedItem := lipgloss.NewStyle().
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("57")).
		Bold(true)

	width := 78
	if m.width > 0 {
		width = min(92, max(56, m.width-8))
	}
	listHeight := 8
	if m.height > 0 {
		listHeight = max(4, min(10, m.height-16))
	}

	var body strings.Builder
	body.WriteString(titleStyle.Render("NOTES MAKER") + "\n")
	body.WriteString(mutedStyle.Render("Study notes, live preview, reusable context") + "\n\n")

	mode := "off"
	if m.config.PersonalizedMode {
		mode = "on"
	}
	profileState := "empty"
	if strings.TrimSpace(m.config.PersonalProfile) != "" {
		profileState = "saved"
	}
	cloudState := "local"
	if m.usingCloud {
		cloudState = "cloud"
	}
	body.WriteString(labelStyle.Render("Config") + "\n")
	body.WriteString(fmt.Sprintf("  model: %s  [%s]\n", m.modelName, cloudState))
	body.WriteString(fmt.Sprintf("  personalized mode: %s  profile: %s\n", mode, profileState))
	body.WriteString(fmt.Sprintf("  notes: %s\n\n", notesRoot()))

	if len(m.topicList) > 0 {
		body.WriteString(labelStyle.Render("Previous Conversations") + "\n")
		start := 0
		if m.topicCursor >= listHeight {
			start = m.topicCursor - listHeight + 1
		}
		end := min(len(m.topicList), start+listHeight)
		if start > 0 {
			body.WriteString(metaStyle.Render(fmt.Sprintf("  ... %d above", start)) + "\n")
		}
		for i := start; i < end; i++ {
			t := m.topicList[i]
			line := fmt.Sprintf("  %-38s", truncatePlain(t, 38))
			if i == m.topicCursor {
				body.WriteString(selectedItem.Render("› "+truncatePlain(t, 40)) + "\n")
			} else {
				body.WriteString(itemStyle.Render(line) + "\n")
			}
		}
		if end < len(m.topicList) {
			body.WriteString(metaStyle.Render(fmt.Sprintf("  ... %d more", len(m.topicList)-end)) + "\n")
		}
		body.WriteString("\n")
	} else {
		body.WriteString(labelStyle.Render("Previous Conversations") + "\n")
		body.WriteString(metaStyle.Render("  No saved topics yet.") + "\n\n")
	}

	newLabel := labelStyle.Render("New Topic")
	if m.topicCursor < 0 {
		newLabel = selectedItem.Render("› New Topic")
	}
	body.WriteString(newLabel + "\n")
	body.WriteString(m.topicInput.View() + "\n\n")

	body.WriteString(metaStyle.Render("enter: open  •  ↑/↓: select  •  type: new topic  •  ctrl+t: settings  •  ctrl+c: quit"))

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(width).
		Render(body.String())

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
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
		helpStyle.Render("ctrl+p: toggle  •  esc/ctrl+t: save and close"),
		"",
		labelStyle.Render("User keypoints"),
		helpStyle.Render("Keep stable facts here: education, experience, goals, preferences, recurring context."),
	)

	body := lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.settingsProfile.View(),
	)

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("57")).
		Padding(1, 2).
		Width(max(40, m.width-4)).
		Height(max(10, m.height-4)).
		Render(body)

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
	help := helpStyle.Render("esc/q: back  •  up/down/pgup/pgdn: scroll  •  g/G: top/bottom")

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("57")).
		Width(m.width - 2).
		Height(m.height - 4).
		Render(m.readerView.View())

	return lipgloss.JoinVertical(lipgloss.Left,
		title+"  "+path,
		box,
		help,
	)
}

// ---- Loading screen ----

func (m *model) viewLoadingScreen() string {
	spinner := loadingSpinner(m.loadingFrame)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Render("NOTES MAKER")
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

// refreshLogView reads the tail of logPath into the log viewport. Called when
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

	content := readLogTail(logPath, logTailBytes)
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
	b.WriteString(metaStyle.Render(fmt.Sprintf("  %d item(s) · tree view", total)) + "\n\n")

	root := filepath.Base(m.workDir)
	if root == "." || root == string(filepath.Separator) {
		root = m.workDir
	}
	b.WriteString(dirStyle.Render("  "+root+"/") + "\n")

	lines := renderTreeLines(entries, "", width, nameStyle, dirStyle, metaStyle)
	maxLines := height - 5
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
	return b.String()
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
	path := pathStyle.Render(logPath)
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
