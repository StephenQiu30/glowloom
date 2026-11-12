package domain

import prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"

// TextInput reads the canonical prompt projection saved by the canvas editor.
// Consumers receive the same frozen text and reference semantics as chat input;
// rich-text JSON and legacy editor fields are not independently interpreted.
func (n Node) TextInput() (prompt.ConversationInput, error) {
	if n.NodeType != NodeTypeText || n.Config.Composer == nil {
		return prompt.ConversationInput{}, ErrInvalidCommand
	}
	return prompt.NewConversationInput(*n.Config.Composer)
}
