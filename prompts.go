package main

import "notes_maker/pkg/core"

const fileBlockReminderPrompt = `Host integration reminder:
- If the user asks you to create, save, write, or update notes/files, you MUST emit the full raw file block:
<<<FILE: filename.md>>>
content
<<<END>>>
- Never answer only "wrote filename.md". The app creates that receipt after it successfully writes the file block.
- If you cannot create useful file content yet, ask a short clarifying question instead of pretending a file was written.`

func buildClassifierPrompt(topic string) string {
	return core.BuildClassifierPrompt(topic)
}

func buildPreSessionSkillPrompt(topic string) string {
	return core.BuildPreSessionSkillPrompt(topic)
}

func buildPreSessionSkillPromptWithProfile(topic, personalContext string) string {
	return core.BuildPreSessionSkillPromptWithProfile(topic, personalContext)
}

func buildPreSessionResearchPrompt(topic string) string {
	return core.BuildPreSessionResearchPrompt(topic)
}

func buildPreSessionResearchPromptWithProfile(topic, personalContext string) string {
	return core.BuildPreSessionResearchPromptWithProfile(topic, personalContext)
}

func buildMainSessionPrompt(sessionContextJSON string) string {
	return core.BuildMainSessionPrompt(sessionContextJSON)
}

func buildMainSessionPromptWithProfile(sessionContextJSON, personalContext string) string {
	return core.BuildMainSessionPromptWithProfile(sessionContextJSON, personalContext)
}

func buildLegacyMainPrompt(topic string) string {
	return core.BuildLegacyMainPrompt(topic)
}

func buildLegacyMainPromptWithProfile(topic, personalContext string) string {
	return core.BuildLegacyMainPromptWithProfile(topic, personalContext)
}
