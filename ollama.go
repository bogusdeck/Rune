package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"rune/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

// probeModel verifies that the given model is reachable by issuing a tiny
// non-streaming /api/chat request with a short timeout. Returns nil on success
// or an error describing why the model is unreachable. Used at startup to
// decide between cloud and local.
func probeModel(url, modelName, apiKey string) error {
	body, _ := json.Marshal(ollamaReq{
		Model:    modelName,
		Messages: []ollamaMessage{{Role: "user", Content: "ping"}},
		Stream:   false,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", url+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

// ollamaReq matches the request body of POST /api/chat.
type ollamaReq struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

// ollamaStreamResp matches a single newline-delimited JSON object from a
// streaming /api/chat response.
type ollamaStreamResp struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
}

// startStream launches an HTTP request to Ollama in a background goroutine and
// pushes streamChunkMsg / streamDoneMsg into m.streamCh. The returned tea.Cmd
// itself returns nil immediately; the bubble tea loop is fed via waitForStreamMsg.
func (m *model) startStream() tea.Cmd {
	url := m.ollamaURL + "/api/chat"
	modelName := m.modelName
	apiKey := m.apiKey
	msgs := llmMessages(m.messages)
	if !m.preSession {
		msgs = append(msgs, ollamaMessage{Role: "system", Content: fileBlockReminderPrompt})
	}
	ch := m.streamCh
	return func() tea.Msg {
		go func() {
			body, _ := json.Marshal(ollamaReq{Model: modelName, Messages: msgs, Stream: true})
			req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(body))
			if err != nil {
				ch <- streamDoneMsg{err: err}
				return
			}
			req.Header.Set("Content-Type", "application/json")
			if apiKey != "" {
				req.Header.Set("Authorization", "Bearer "+apiKey)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				ch <- streamDoneMsg{err: err}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				b, _ := io.ReadAll(resp.Body)
				ch <- streamDoneMsg{err: fmt.Errorf("ollama %s: %s", resp.Status, strings.TrimSpace(string(b)))}
				return
			}
			scanner := bufio.NewScanner(resp.Body)
			scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
			for scanner.Scan() {
				line := scanner.Bytes()
				if len(line) == 0 {
					continue
				}
				var sr ollamaStreamResp
				if err := json.Unmarshal(line, &sr); err != nil {
					continue
				}
				if sr.Message.Content != "" {
					ch <- streamChunkMsg(sr.Message.Content)
				}
				if sr.Done {
					break
				}
			}
			if err := scanner.Err(); err != nil {
				ch <- streamDoneMsg{err: err}
				return
			}
			ch <- streamDoneMsg{err: nil}
		}()
		return nil
	}
}

func llmMessages(messages []chatMessage) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(messages))
	for _, msg := range messages {
		content := msg.Content
		if msg.Role == "assistant" && isOnlyWriteReceipt(content) {
			content = "The host displayed a legacy file-write receipt here. The original FILE block content is unavailable in this saved history. Do not imitate this receipt; use a full <<<FILE: ...>>>...<<<END>>> block when writing files."
		}
		out = append(out, ollamaMessage{
			Role:    msg.Role,
			Content: content,
			Images:  encodeLocalImages(msg.ImagePaths),
		})
	}
	return out
}

func isOnlyWriteReceipt(s string) bool {
	if len(core.WriteReceiptNames(s)) == 0 {
		return false
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "wrote ") {
			return false
		}
	}
	return true
}

func (m *model) startStreamAndWait() tea.Cmd {
	return func() tea.Msg {
		if cmd := m.startStream(); cmd != nil {
			_ = cmd()
		}
		return <-m.streamCh
	}
}

// waitForStreamMsg returns a tea.Cmd that blocks on m.streamCh and forwards
// the next message to the bubble-tea event loop.
func waitForStreamMsg(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}
