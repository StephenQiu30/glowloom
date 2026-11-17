package application

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// LocalInput records only inference provenance, never PCM bytes or a filesystem path.
type LocalInput struct {
	Model      string    `json:"model"`
	JobID      uuid.UUID `json:"job_id"`
	Attempt    int       `json:"attempt"`
	SHA256     string    `json:"sha256"`
	Language   string    `json:"language"`
	DurationMS int64     `json:"duration_ms"`
}

func prepareLocal(c domain.Call, raw json.RawMessage) (domain.Call, error) {
	var in LocalInput
	if json.Unmarshal(raw, &in) != nil || in.Model != "local/whisper" || in.JobID == uuid.Nil || in.Attempt < 1 || len(in.SHA256) != 64 || len(in.Language) > 16 || in.Language == "" || in.DurationMS < 1 || in.DurationMS > 86400000 {
		return c, domain.ErrInvalid
	}
	encoded, _ := json.Marshal(in)
	normalized, err := domain.CanonicalJSON(raw)
	expected, _ := domain.CanonicalJSON(encoded)
	if err != nil || string(expected) != string(normalized) {
		return c, domain.ErrInvalid
	}
	c.Raw = expected
	encoded, _ = json.Marshal(struct {
		Scope, Actor, Project, Run uuid.UUID
		Origin                     string
		Raw                        json.RawMessage
	}{c.Scope, c.Actor, c.Project, c.Run, c.Origin, c.Raw})
	c.Hash = domain.Digest(encoded)
	c.Limit = 0
	return c, nil
}
