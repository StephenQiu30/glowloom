package application

import "github.com/google/uuid"

// AgentQuoteContext binds server orchestration to its original quote receipt.
type AgentQuoteContext struct {
	SessionID uuid.UUID `json:"session_id"`
	TurnID    uuid.UUID `json:"turn_id"`
	Phase     string    `json:"phase"`
	Policy    string    `json:"policy"`
}

// Valid limits Agent quotes to the documented server policy and phases.
func (c AgentQuoteContext) Valid() bool {
	return c.SessionID != uuid.Nil && c.TurnID != uuid.Nil && c.Policy == "image-agent-json-v1" && (c.Phase == "plan" || c.Phase == "image" || c.Phase == "summary")
}
