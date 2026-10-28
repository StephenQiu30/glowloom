package application

import "github.com/google/uuid"

// VoiceTopic contains only durable job identifiers, never keys or audio bytes.
const VoiceTopic = "glowloom.subject.voice.requested.v1"

// VoiceDelivery proves one committed original or recovery-only delivery.
type VoiceDelivery struct {
	EventID      uuid.UUID `json:"event_id"`
	JobID        uuid.UUID `json:"job_id"`
	ActorID      uuid.UUID `json:"actor_id"`
	ScopeID      uuid.UUID `json:"scope_id"`
	RecoveryOnly bool      `json:"recovery_only"`
}
