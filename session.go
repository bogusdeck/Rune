package main

import "notes_maker/pkg/core"

type sessionFile = core.SessionFile

func slugify(topic string) string {
	return core.Slugify(topic)
}

func sessionPath(workDir string) string {
	return core.SessionPath(workDir)
}

func loadSessionFile(workDir string) (sessionFile, bool) {
	return core.LoadSessionFile(workDir)
}

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
	s, ok := loadSessionFile(m.workDir)
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
			if opts := extractOptions(d.Content); len(opts) >= 2 {
				m.options = opts
				m.optionCursor = 0
				m.optionsActive = true
			}
		}
		break
	}
}

func listExistingTopics() []string {
	return core.ListExistingTopics("")
}
