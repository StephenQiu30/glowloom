package domain

import (
	"encoding/hex"
	"errors"
	"math"
	"strings"

	"github.com/google/uuid"
)

// ErrInvalidPromptPreparation means immutable template evidence is inconsistent.
var ErrInvalidPromptPreparation = errors.New("invalid prompt preparation")

// PromptPreparation records why a quote used its one frozen prompt input.
// Text belongs only to OperationInput; this value carries selection and hashes.
type PromptPreparation struct {
	Version               int        `json:"version"`
	Operation             string     `json:"operation"`
	Policy                string     `json:"policy"`
	TemplateID            *uuid.UUID `json:"template_id,omitempty"`
	TemplateVersion       int        `json:"template_version,omitempty"`
	CustomizationID       *uuid.UUID `json:"customization_id,omitempty"`
	CustomizationRevision int64      `json:"customization_revision,omitempty"`
	RequestSHA256         string     `json:"request_sha256"`
	UserPromptSHA256      string     `json:"user_prompt_sha256"`
	ContentSHA256         string     `json:"content_sha256"`
}

// Validate checks the closed policy and complete digest facts without reading preferences.
func (p PromptPreparation) Validate() error {
	if p.Version != 1 || !validTemplateOperation(p.Operation) ||
		!validPromptDigest(p.RequestSHA256) || !validPromptDigest(p.UserPromptSHA256) || !validPromptDigest(p.ContentSHA256) {
		return ErrInvalidPromptPreparation
	}
	if p.Policy == "bypass_video" {
		if p.Operation != "storyboard_video" || p.TemplateID != nil || p.TemplateVersion != 0 ||
			p.CustomizationID != nil || p.CustomizationRevision != 0 || p.ContentSHA256 != p.UserPromptSHA256 {
			return ErrInvalidPromptPreparation
		}
		return nil
	}
	if p.Policy != "compiled" || p.Operation == "storyboard_video" || p.TemplateID == nil || *p.TemplateID == uuid.Nil ||
		p.TemplateVersion != 1 || p.CustomizationRevision < 0 || p.CustomizationRevision > math.MaxInt32 ||
		(p.CustomizationID == nil) != (p.CustomizationRevision == 0) ||
		(p.CustomizationID != nil && *p.CustomizationID == uuid.Nil) {
		return ErrInvalidPromptPreparation
	}
	return nil
}

// ValidateFor binds immutable evidence to the operation's generation capability.
func (p PromptPreparation) ValidateFor(capability string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	want := "text.structured"
	switch p.Operation {
	case "character_turnaround", "storyboard_first_frame":
		want = "image.generate"
	case "storyboard_video":
		want = "video.generate"
	}
	if capability != want {
		return ErrInvalidPromptPreparation
	}
	return nil
}

func validPromptDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validTemplateOperation(operation string) bool {
	switch operation {
	case "chapter_assets_extract", "character_extract", "character_turnaround", "storyboard_plan", "storyboard_repair", "storyboard_first_frame", "storyboard_video", "short_drama_outline", "skill_draft":
		return true
	default:
		return false
	}
}
