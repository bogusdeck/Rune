package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	providerOllama      = "ollama"
	providerCodex       = "codex"
	providerAntigravity = "antigravity"
	providerAuto        = "auto"
)

type llmProvider interface {
	Name() string
	SupportsImages() bool
	Complete(ctx context.Context, prompt string, imagePaths []string) (string, error)
	Stream(ctx context.Context, messages []chatMessage, includeReminder bool, ch chan<- tea.Msg)
}

func normalizeProviderName(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case providerCodex:
		return providerCodex
	case providerAntigravity:
		return providerAntigravity
	case providerAuto:
		return providerAuto
	default:
		return providerOllama
	}
}

func (m *model) providerName() string {
	return normalizeProviderName(m.config.Provider)
}

func (m *model) providerDisplay() string {
	switch m.providerName() {
	case providerAuto:
		return "auto"
	case providerCodex:
		return "codex"
	case providerAntigravity:
		return "antigravity"
	default:
		return "ollama"
	}
}

func (m *model) runtimeLabel() string {
	switch m.providerName() {
	case providerAuto:
		return m.autoProviderName()
	case providerCodex:
		if strings.TrimSpace(m.config.CodexModel) != "" {
			return m.config.CodexModel
		}
		return "default"
	case providerAntigravity:
		return "external"
	default:
		return m.modelName
	}
}

func (m *model) providerModeLabel() string {
	switch m.providerName() {
	case providerAuto:
		return "auto"
	case providerCodex:
		return "codex"
	case providerAntigravity:
		return "antigravity"
	default:
		if m.usingCloud {
			return "cloud"
		}
		return "local"
	}
}

func (m *model) currentProvider() llmProvider {
	return m.providerForMessages(m.messages)
}

func (m *model) providerForMessages(messages []chatMessage) llmProvider {
	switch m.effectiveProviderName(messages) {
	case providerCodex:
		return codexProvider{workDir: m.workDir, command: strings.TrimSpace(m.config.CodexCommand), model: strings.TrimSpace(m.config.CodexModel)}
	case providerAntigravity:
		return antigravityProvider{workDir: m.workDir, command: strings.TrimSpace(m.config.AntigravityCommand)}
	default:
		return ollamaProvider{
			url:      m.ollamaURL,
			model:    m.modelName,
			apiKey:   m.apiKey,
			workDir:  m.workDir,
			useCloud: m.usingCloud,
			local:    m.localModel,
		}
	}
}

func (m *model) effectiveProviderName(messages []chatMessage) string {
	switch m.providerName() {
	case providerAuto:
		return m.autoProviderNameForMessages(messages)
	case providerCodex:
		return providerCodex
	case providerAntigravity:
		return providerAntigravity
	default:
		return providerOllama
	}
}

func completeWithProvider(m *model, prompt string, imagePaths []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	messages := []chatMessage{{Role: "user", Content: prompt, ImagePaths: imagePaths}}
	return m.providerForMessages(messages).Complete(ctx, prompt, imagePaths)
}

func (m *model) autoProviderName() string {
	return m.autoProviderNameForMessages(m.messages)
}

func (m *model) autoProviderNameForMessages(messages []chatMessage) string {
	last := lastUserMessage(messages)
	text := ""
	images := 0
	files := 0
	if last != nil {
		text = last.Content
		images = len(last.ImagePaths)
		files = len(last.FilePaths)
	}
	if images > 0 {
		if m.providerAvailable(providerCodex) {
			return providerCodex
		}
		if m.providerAvailable(providerAntigravity) {
			return providerAntigravity
		}
		return providerOllama
	}
	if files > 0 {
		if m.providerAvailable(providerCodex) {
			return providerCodex
		}
		if m.providerAvailable(providerAntigravity) {
			return providerAntigravity
		}
		return providerOllama
	}

	complexity := estimatePromptComplexity(text)
	switch complexity {
	case "complex":
		if m.providerAvailable(providerCodex) {
			return providerCodex
		}
		if m.providerAvailable(providerAntigravity) {
			return providerAntigravity
		}
		return providerOllama
	case "medium":
		if m.providerAvailable(providerAntigravity) {
			return providerAntigravity
		}
		if m.providerAvailable(providerCodex) {
			return providerCodex
		}
		return providerOllama
	default:
		return providerOllama
	}
}

func (m *model) providerAvailable(name string) bool {
	switch name {
	case providerCodex:
		return fileExistsOnPath(firstNonEmpty(m.config.CodexCommand, "codex"))
	case providerAntigravity:
		return commandSeemsAvailable(firstNonEmpty(m.config.AntigravityCommand, "antigravity"))
	default:
		return true
	}
}

func commandSeemsAvailable(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	return fileExistsOnPath(fields[0])
}

func lastUserMessage(messages []chatMessage) *chatMessage {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return &messages[i]
		}
	}
	return nil
}

func estimatePromptComplexity(text string) string {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return "simple"
	}
	long := len(text) > 500
	medium := len(text) > 180
	complexTerms := []string{
		"debug", "bug", "refactor", "architecture", "design", "performance",
		"optimize", "review", "analyze", "investigate", "production",
		"compare", "tradeoff", "root cause", "system design", "strategy",
	}
	score := 0
	if long {
		score += 2
	} else if medium {
		score++
	}
	for _, term := range complexTerms {
		if strings.Contains(text, term) {
			score++
		}
	}
	switch {
	case score >= 3:
		return "complex"
	case score >= 1:
		return "medium"
	default:
		return "simple"
	}
}

func autoRouteReason(text string, attachmentPaths []string, routed string) string {
	if len(attachmentPaths) > 0 {
		images, docs := splitAttachmentPaths(attachmentPaths)
		if len(images) > 0 && len(docs) > 0 {
			return "image and document attachments detected"
		}
		if len(images) > 0 {
			return "image attachment detected"
		}
		return "document attachment detected"
	}
	switch estimatePromptComplexity(text) {
	case "complex":
		return "complex request detected"
	case "medium":
		return "medium complexity request detected"
	default:
		if routed == providerOllama {
			return "short/simple text request"
		}
		return "provider availability fallback"
	}
}

type switcherPreset struct {
	label       string
	description string
	apply       func(*model)
}

func (m *model) modelSwitcherPresets() []switcherPreset {
	return []switcherPreset{
		{
			label:       "Auto",
			description: "Route by complexity and attachments",
			apply: func(m *model) {
				m.config.Provider = providerAuto
				m.syncProviderState()
			},
		},
		{
			label:       "Ollama Cloud",
			description: "Prefer cloud Ollama model",
			apply: func(m *model) {
				m.config.Provider = providerOllama
				m.usingCloud = true
				m.modelName = m.cloudModel
			},
		},
		{
			label:       "Ollama Local",
			description: "Use local Ollama fallback model",
			apply: func(m *model) {
				m.config.Provider = providerOllama
				m.usingCloud = false
				m.modelName = m.localModel
			},
		},
		{
			label:       "Codex",
			description: "Use local Codex CLI backend",
			apply: func(m *model) {
				m.config.Provider = providerCodex
				m.syncProviderState()
			},
		},
		{
			label:       "Antigravity",
			description: "Use Antigravity CLI backend",
			apply: func(m *model) {
				m.config.Provider = providerAntigravity
				m.syncProviderState()
			},
		},
	}
}

func latestImages(messages []chatMessage) []string {
	for i := len(messages) - 1; i >= 0; i-- {
		if len(messages[i].ImagePaths) > 0 {
			return append([]string(nil), messages[i].ImagePaths...)
		}
	}
	return nil
}

func transcriptPrompt(messages []chatMessage, includeReminder bool) string {
	var b strings.Builder
	b.WriteString("You are responding inside Rune, a terminal note-taking app.\n")
	b.WriteString("Continue the conversation below and reply only with the assistant's next message.\n")
	if includeReminder {
		b.WriteString("\n")
		b.WriteString(fileBlockReminderPrompt)
		b.WriteString("\n")
	}
	b.WriteString("\nConversation transcript:\n\n")
	for _, msg := range messages {
		role := strings.Title(msg.Role)
		if msg.Role == "system" {
			role = "System"
		}
		b.WriteString(role)
		b.WriteString(":\n")
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}
	b.WriteString("Assistant:\n")
	return b.String()
}

type commandProvider struct {
	name            string
	command         string
	args            []string
	imageFlag       string
	modelFlag       string
	model           string
	workDir         string
	supportsImages  bool
	stdinPromptOnly bool
}

func (p commandProvider) Name() string { return p.name }

func (p commandProvider) SupportsImages() bool { return p.supportsImages }

func (p commandProvider) Complete(ctx context.Context, prompt string, imagePaths []string) (string, error) {
	if strings.TrimSpace(p.command) == "" {
		return "", fmt.Errorf("%s command is not configured", p.name)
	}
	tmp, err := os.CreateTemp("", "rune-provider-*.txt")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	args := append([]string(nil), p.args...)
	if p.modelFlag != "" && strings.TrimSpace(p.model) != "" {
		args = append(args, p.modelFlag, p.model)
	}
	if p.imageFlag != "" {
		for _, path := range imagePaths {
			args = append(args, p.imageFlag, path)
		}
	}
	if p.name == providerCodex {
		args = append(args, "--output-last-message", tmpPath, "-")
	}

	cmd := exec.CommandContext(ctx, p.command, args...)
	if p.workDir != "" {
		cmd.Dir = p.workDir
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader(prompt)
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", p.name, msg)
	}

	if p.name == providerCodex {
		b, err := os.ReadFile(tmpPath)
		if err == nil && strings.TrimSpace(string(b)) != "" {
			return strings.TrimSpace(string(b)), nil
		}
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		out = strings.TrimSpace(stderr.String())
	}
	return out, nil
}

func (p commandProvider) Stream(ctx context.Context, messages []chatMessage, includeReminder bool, ch chan<- tea.Msg) {
	go func() {
		prompt := transcriptPrompt(messages, includeReminder)
		reply, err := p.Complete(ctx, prompt, latestImages(messages))
		if err != nil {
			ch <- streamDoneMsg{err: err}
			return
		}
		if reply != "" {
			ch <- streamChunkMsg(reply)
		}
		ch <- streamDoneMsg{err: nil}
	}()
}

type codexProvider struct {
	workDir string
	command string
	model   string
}

func (p codexProvider) Name() string { return providerCodex }

func (p codexProvider) SupportsImages() bool { return true }

func (p codexProvider) Complete(ctx context.Context, prompt string, imagePaths []string) (string, error) {
	cp := commandProvider{
		name:           providerCodex,
		command:        firstNonEmpty(p.command, "codex"),
		args:           []string{"exec", "--skip-git-repo-check", "--ephemeral"},
		imageFlag:      "--image",
		modelFlag:      "--model",
		model:          p.model,
		workDir:        p.workDir,
		supportsImages: true,
	}
	return cp.Complete(ctx, prompt, imagePaths)
}

func (p codexProvider) Stream(ctx context.Context, messages []chatMessage, includeReminder bool, ch chan<- tea.Msg) {
	cp := commandProvider{
		name:           providerCodex,
		command:        firstNonEmpty(p.command, "codex"),
		args:           []string{"exec", "--skip-git-repo-check", "--ephemeral"},
		imageFlag:      "--image",
		modelFlag:      "--model",
		model:          p.model,
		workDir:        p.workDir,
		supportsImages: true,
	}
	cp.Stream(ctx, messages, includeReminder, ch)
}

type antigravityProvider struct {
	workDir string
	command string
}

func (p antigravityProvider) Name() string { return providerAntigravity }

func (p antigravityProvider) SupportsImages() bool { return false }

func (p antigravityProvider) Complete(ctx context.Context, prompt string, _ []string) (string, error) {
	command := strings.TrimSpace(p.command)
	if command == "" {
		command = "antigravity"
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	if p.workDir != "" {
		cmd.Dir = p.workDir
	}
	cmd.Stdin = strings.NewReader(prompt)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", providerAntigravity, msg)
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		out = strings.TrimSpace(stderr.String())
	}
	return out, nil
}

func (p antigravityProvider) Stream(ctx context.Context, messages []chatMessage, includeReminder bool, ch chan<- tea.Msg) {
	go func() {
		reply, err := p.Complete(ctx, transcriptPrompt(messages, includeReminder), nil)
		if err != nil {
			ch <- streamDoneMsg{err: err}
			return
		}
		if reply != "" {
			ch <- streamChunkMsg(reply)
		}
		ch <- streamDoneMsg{err: nil}
	}()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fileExistsOnPath(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	if filepath.IsAbs(name) {
		_, err := os.Stat(name)
		return err == nil
	}
	_, err := exec.LookPath(name)
	return err == nil
}
