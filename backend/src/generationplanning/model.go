package generationplanning

import "github.com/google/uuid"

type Plan struct {
	ID                   uuid.UUID `json:"id"`
	ProjectID            uuid.UUID `json:"project_id"`
	TargetType           string    `json:"target_type"`
	TargetID             uuid.UUID `json:"target_id"`
	Status               string    `json:"status"`
	ExecutionDisposition string    `json:"execution_disposition,omitempty"`
	InputSnapshotHash    string    `json:"input_snapshot_hash"`
	PromptHash           string    `json:"prompt_hash"`
}

type Item struct {
	ID            uuid.UUID `json:"id"`
	PlanID        uuid.UUID `json:"plan_id"`
	Ordinal       int       `json:"ordinal"`
	CapabilityKey string    `json:"capability_key"`
	Prompt        string    `json:"prompt"`
	Status        string    `json:"status"`
}
