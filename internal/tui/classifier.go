package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"rune/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

// classifierResult is the parsed response from the classifier prompt.
type classifierResult struct {
	Type   string `json:"type"`   // "skill" or "research"
	Label  string `json:"label"`  // short 3-5 word description
	Reason string `json:"reason"` // one-sentence justification
}

// classifierDoneMsg is delivered to Update when the classifier call finishes.
type classifierDoneMsg struct {
	res classifierResult
	err error
}

// ollamaChatResp is the body of a non-streaming /api/chat reply.
type ollamaChatResp struct {
	Message chatMessage `json:"message"`
}

// classifyTopic returns a tea.Cmd that asks the LLM to classify the topic and
// posts a classifierDoneMsg back to Update. Runs as a single non-streaming
// request because the result is tiny and we want it fully assembled before
// branching the UI.
func (m *model) classifyTopic(topic string) tea.Cmd {
	prompt := core.BuildClassifierPrompt(topic)

	return func() tea.Msg {
		start := time.Now()
		raw, err := completeWithProvider(m, prompt, nil)
		if err != nil {
			log.Printf("classifier failed: topic=%q elapsed=%s err=%v", topic, time.Since(start), err)
			return classifierDoneMsg{err: err}
		}
		res, err := parseClassifierJSON(raw)
		if err != nil {
			return classifierDoneMsg{err: err}
		}
		log.Printf("classifier done: topic=%q elapsed=%s", topic, time.Since(start))
		return classifierDoneMsg{res: res}
	}
}

// jsonObjectRe finds the first balanced-looking JSON object in a string.
// Models sometimes wrap the response in prose or markdown fences; this lets us
// recover the JSON body anyway.
var jsonObjectRe = regexp.MustCompile(`(?s)\{.*\}`)

func parseClassifierJSON(s string) (classifierResult, error) {
	s = strings.TrimSpace(s)
	// Strip ```json ... ``` fences if the model emitted them.
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	candidate := s
	if !strings.HasPrefix(candidate, "{") {
		if m := jsonObjectRe.FindString(s); m != "" {
			candidate = m
		}
	}
	var res classifierResult
	if err := json.Unmarshal([]byte(candidate), &res); err != nil {
		return res, fmt.Errorf("classifier JSON: %w (got %q)", err, s)
	}
	res.Type = strings.ToLower(strings.TrimSpace(res.Type))
	if res.Type != "skill" && res.Type != "research" {
		return res, fmt.Errorf("classifier returned invalid type %q", res.Type)
	}
	return res, nil
}
