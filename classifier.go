package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

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
	url := m.ollamaURL
	model := m.modelName
	apiKey := m.apiKey
	prompt := buildClassifierPrompt(topic)

	return func() tea.Msg {
		start := time.Now()
		body, _ := json.Marshal(ollamaReq{
			Model:    model,
			Messages: []chatMessage{{Role: "user", Content: prompt}},
			Stream:   false,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "POST", url+"/api/chat", bytes.NewReader(body))
		if err != nil {
			return classifierDoneMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("classifier failed: topic=%q elapsed=%s err=%v", topic, time.Since(start), err)
			return classifierDoneMsg{err: err}
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			log.Printf("classifier status error: topic=%q elapsed=%s status=%s", topic, time.Since(start), resp.Status)
			return classifierDoneMsg{err: fmt.Errorf("classifier %s: %s", resp.Status, strings.TrimSpace(string(raw)))}
		}
		var chat ollamaChatResp
		if err := json.Unmarshal(raw, &chat); err != nil {
			return classifierDoneMsg{err: fmt.Errorf("decode classifier: %w", err)}
		}
		res, err := parseClassifierJSON(chat.Message.Content)
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
