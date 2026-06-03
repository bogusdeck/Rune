package main

const fileBlockReminderPrompt = `Host integration reminder:
- If the user asks you to create, save, write, or update notes/files, you MUST emit the full raw file block:
<<<FILE: filename.md>>>
content
<<<END>>>
- Never answer only "wrote filename.md". The app creates that receipt after it successfully writes the file block.
- If you cannot create useful file content yet, ask a short clarifying question instead of pretending a file was written.`
