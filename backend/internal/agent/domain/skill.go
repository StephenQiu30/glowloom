package domain

import "github.com/google/uuid"

// SkillInput contains one private skill version and its resource contents.
type SkillInput struct {
	RequestID   uuid.UUID         `json:"request_id"`
	SkillID     *uuid.UUID        `json:"skill_id,omitempty"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Body        string            `json:"body"`
	Resources   map[string]string `json:"resources"`
}

// SkillVersion is the safe metadata projection of a private immutable skill version.
type SkillVersion struct {
	SkillID      uuid.UUID `json:"skill_id"`
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	Description  string    `json:"description"`
	ResourceHash string    `json:"resource_hash"`
	Enabled      bool      `json:"enabled"`
}

// SkillContent contains a pinned skill resource and its version metadata.
type SkillContent struct {
	SkillVersion
	ResourcePath string `json:"resource_path"`
	Content      string `json:"content"`
}

// SkillPin binds execution to an exact skill version and resource hash.
type SkillPin struct {
	SkillID      uuid.UUID `json:"skill_id"`
	Version      string    `json:"version"`
	ResourceHash string    `json:"resource_hash"`
}
