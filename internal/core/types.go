package core

type ChatMessage struct {
	Role       string   `json:"role"`
	Content    string   `json:"content"`
	ImagePaths []string `json:"image_paths,omitempty"`
}

type DisplayMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type SessionFile struct {
	Topic          string           `json:"topic"`
	Type           string           `json:"type,omitempty"`
	Label          string           `json:"label,omitempty"`
	SessionContext string           `json:"session_context,omitempty"`
	Messages       []ChatMessage    `json:"messages"`
	DisplayMsgs    []DisplayMessage `json:"display_msgs"`
}
