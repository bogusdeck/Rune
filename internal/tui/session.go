package tui

import "rune/internal/core"

func (m *model) saveSession() {
	core.SaveSessionFile(m.workDir, sessionFile{
		Topic:          m.topic,
		Type:           m.sessionType,
		Label:          m.sessionLabel,
		SessionContext: m.sessionContext,
		Messages:       m.messages,
		DisplayMsgs:    m.displayMsgs,
	})
}

func (m *model) loadSession() bool {
	s, ok := core.LoadSessionFile(m.workDir)
	if !ok {
		return false
	}
	m.applySession(s)
	return true
}

func (m *model) applySession(s sessionFile) {
	m.messages = s.Messages
	m.displayMsgs = s.DisplayMsgs
	m.sessionType = s.Type
	m.sessionLabel = s.Label
	m.sessionContext = s.SessionContext

	m.restoreOptionPicker()
}

func (m *model) restoreOptionPicker() {
	m.options = nil
	m.optionsActive = false
	for i := len(m.displayMsgs) - 1; i >= 0; i-- {
		d := m.displayMsgs[i]
		if d.Role == "system" {
			continue
		}
		if d.Role == "assistant" {
			if opts := core.ExtractOptions(d.Content); len(opts) >= 2 {
				m.options = opts
				m.optionCursor = 0
				m.optionsActive = true
			}
		}
		break
	}
}
