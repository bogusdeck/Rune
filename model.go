package main

import (
	"fmt"
	"os"
	"time"

	"notes_maker/pkg/core"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

// ---- App state ----

type appState int

const (
	stateTopicInput appState = iota
	stateLoading             // waiting for the first greeting chunk on a new topic
	stateChat
)

const (
	defaultOllamaURL = "http://localhost:11434"
	// Local fallback when cloud is unreachable. Matches the --qwen default
	// in ~/.zshrc-config/.zshrc-tork.
	defaultLocalModel = "qwen2.5-coder:7b"
	// Preferred cloud model. Routed through the local Ollama daemon after
	// `ollama signin`; the ":...-cloud" suffix tells the daemon to proxy
	// the request to Ollama Cloud.
	defaultCloudModel = "qwen3-coder:480b-cloud"
)

// ---- Bubble-tea messages ----

type fileChangedMsg string
type streamChunkMsg string
type streamDoneMsg struct{ err error }

// loadingTickMsg drives the animated gradient on the loading screen.
type loadingTickMsg struct{}
type openChatMsg struct{}

// topicReadyMsg fires after a resumed topic has had a moment to lay out and
// prime its preview, signalling that we can dismiss the loading splash and
// drop into stateChat.
type topicReadyMsg struct{}

type topicOpenedMsg struct {
	topic      string
	workDir    string
	session    sessionFile
	hasSession bool
	err        error
}

// ---- Chat data types ----

type chatMessage = core.ChatMessage
type displayMsg = core.DisplayMessage

// ---- Top-level model ----

type model struct {
	state appState

	topicInput      textinput.Model
	chatInput       textinput.Model
	settingsProfile textarea.Model

	chatRenderer *glamour.TermRenderer

	topic   string
	workDir string

	ollamaURL string
	modelName string // currently active model
	apiKey    string
	config    appConfig

	cloudModel string // preferred cloud model
	localModel string // local fallback
	usingCloud bool   // true while we're talking to cloud

	topicList   []string // existing topics under ~/notes/
	topicCursor int      // -1 = input focused; 0..n-1 = list highlight

	messages    []chatMessage // sent to ollama
	displayMsgs []displayMsg  // rendered in chat pane
	pendingAsst string        // assistant text currently being streamed
	streaming   bool

	// While the model is mid-stream of a <<<FILE: name>>> block, writingFile
	// holds that name so the chat pane can show a "writing X…" spinner
	// instead of the raw body. livePreviewSize records how many bytes of the
	// in-progress body we have already pushed into notesView, used to
	// throttle re-rendering with glamour.
	writingFile     string
	livePreviewSize int

	// streamWritten tracks file names that writeFileBlocks has already
	// committed to disk during the current stream. Prevents the per-chunk
	// loop from re-writing the same file (and re-firing fsnotify events) on
	// every token. Reset to nil at the start of each user turn.
	streamWritten  map[string]bool
	streamHadWrite bool

	options       []string // numbered choices extracted from last assistant reply
	optionCursor  int
	optionsActive bool

	// Pre-session pipeline state.
	sessionType    string // "skill" | "research" | "" (unknown / legacy)
	sessionLabel   string // short label produced by the classifier
	sessionContext string // JSON body emitted in <session_context>; empty during pre-session
	preSession     bool   // true while the user is still answering pre-session questions

	// chatOpenedAt records when we transitioned into the chat screen, so we
	// can ignore the burst of terminal-query responses (OSC color, cursor
	// position, focus, etc.) that bubbletea surfaces as KeyMsgs right after
	// the alt-screen mounts. Without this gate those bytes both dismiss the
	// option picker and leak into the chat input as gibberish.
	chatOpenedAt time.Time

	// loadingFrame advances on every loadingTickMsg; the loading screen's
	// gradient bar uses it to render the sweeping wave.
	loadingFrame       int
	loadingMessage     string
	loadingOpenPending bool
	topicReadyPending  bool

	streamCh chan tea.Msg

	chatView   viewport.Model
	notesView  viewport.Model
	readerView viewport.Model
	lastFile   string

	// In-app log viewer (toggled with Ctrl+L). When showLogs is true the View
	// renders a full-screen scrollable pane showing the contents of logPath
	// instead of the normal chat layout. State of the chat is preserved.
	showLogs   bool
	logView    viewport.Model
	showReader    bool
	readerRawMode bool
	// Settings overlay (Ctrl+T). Used for persistent app configuration.
	showSettings bool

	// Right-pane explorer (toggled with Ctrl+E). When true, the right pane
	// shows a list of files in workDir instead of the markdown preview.
	showExplorer   bool
	explorerScroll int
	explorerCursor int
	// activePane controls which split pane receives simple scroll keys.
	// 0 = chat, 1 = right pane (preview/explorer).
	activePane    int
	splitPercent  int
	resizingSplit bool

	width  int
	height int
	ready  bool
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "e.g. Java, Kubernetes, Linear Algebra..."
	ti.Focus()
	ti.CharLimit = 80
	ti.Width = 60

	ci := textinput.New()
	ci.Placeholder = "Message... (Enter to send)"
	ci.CharLimit = 4000

	settingsProfile := textarea.New()
	settingsProfile.Placeholder = "Education, experience, goals, learning preferences, recurring context..."
	settingsProfile.CharLimit = 4000
	settingsProfile.ShowLineNumbers = false
	settingsProfile.Blur()

	chatVP := viewport.New(0, 0)
	chatVP.MouseWheelEnabled = true
	notesVP := viewport.New(0, 0)
	notesVP.MouseWheelEnabled = true
	readerVP := viewport.New(0, 0)
	readerVP.MouseWheelEnabled = true
	logVP := viewport.New(0, 0)
	logVP.MouseWheelEnabled = true

	ollama := os.Getenv("OLLAMA_URL")
	if ollama == "" {
		ollama = defaultOllamaURL
	}
	localModel := os.Getenv("OLLAMA_MODEL")
	if localModel == "" {
		localModel = defaultLocalModel
	}
	cloudModel := os.Getenv("OLLAMA_CLOUD_MODEL")
	if cloudModel == "" {
		cloudModel = defaultCloudModel
	}
	// Optional: forwarded to the daemon as a Bearer token. Harmless for local.
	apiKey := os.Getenv("OLLAMA_API_KEY")
	cfg := loadAppConfig()
	settingsProfile.SetValue(cfg.PersonalProfile)

	// Decide which model to start with. Cloud is preferred; we probe it once
	// synchronously (with a short timeout) and silently fall back to local on
	// any error. Set OLLAMA_NO_CLOUD=1 to skip the probe entirely.
	active := localModel
	usingCloud := false
	if os.Getenv("OLLAMA_NO_CLOUD") == "" {
		if err := probeModel(ollama, cloudModel, apiKey); err == nil {
			active = cloudModel
			usingCloud = true
			fmt.Printf("✓ ollama cloud reachable, using %s\n", cloudModel)
		} else {
			fmt.Printf("⚠ cloud unavailable (%v); falling back to local %s\n", err, localModel)
		}
	}

	topics := listExistingTopics()
	cursor := -1
	if len(topics) > 0 {
		cursor = 0
	}

	return model{
		state:           stateTopicInput,
		topicInput:      ti,
		chatInput:       ci,
		settingsProfile: settingsProfile,
		chatView:        chatVP,
		notesView:       notesVP,
		readerView:      readerVP,
		logView:         logVP,
		ollamaURL:       ollama,
		modelName:       active,
		localModel:      localModel,
		cloudModel:      cloudModel,
		usingCloud:      usingCloud,
		apiKey:          apiKey,
		config:          cfg,
		streamCh:        make(chan tea.Msg, 64),
		topicList:       topics,
		topicCursor:     cursor,
		splitPercent:    70,
	}
}

func (m *model) Init() tea.Cmd {
	return textinput.Blink
}
