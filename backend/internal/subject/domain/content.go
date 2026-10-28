// Package domain defines immutable space-owned subject content.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalid rejects malformed immutable subject content.
var ErrInvalid = errors.New("invalid subject content")

// Content budgets bound new subject settings independently of Bible history.
const (
	MaxNameUnits          = 20
	MaxDescriptionScalars = 8192
	MaxReferences         = 120
)

// MediaFact is owner-proven frozen evidence, never a client-supplied URL.
type MediaFact struct {
	AssetID         uuid.UUID  `json:"asset_id"`
	Revision        int64      `json:"asset_revision"`
	Kind            string     `json:"kind"`
	MIMEType        string     `json:"mime_type"`
	FileName        string     `json:"file_name"`
	SHA256          string     `json:"sha256"`
	ByteSize        int64      `json:"byte_size"`
	SourceProjectID *uuid.UUID `json:"source_project_id,omitempty"`
	PersonalOrgID   *uuid.UUID `json:"personal_org_id,omitempty"`
	PersonalActorID *uuid.UUID `json:"personal_actor_id,omitempty"`
	RenditionID     *uuid.UUID `json:"rendition_id,omitempty"`
	RenditionSHA256 *string    `json:"rendition_sha256,omitempty"`
	DurationMS      *int64     `json:"duration_ms,omitempty"`
	Width           *int32     `json:"width,omitempty"`
	Height          *int32     `json:"height,omitempty"`
}

// Binding gives one adopted media fact its own immutable identity.
type Binding struct {
	ID    uuid.UUID `json:"binding_id"`
	Media MediaFact `json:"media"`
}

// Voice names an actually uploaded audio sample.
type Voice struct {
	Name    string  `json:"name"`
	Binding Binding `json:"binding"`
}

// Content is one complete subject version.
type Content struct {
	SchemaVersion int       `json:"schema_version"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Primary       Binding   `json:"primary"`
	References    []Binding `json:"references"`
	Voice         *Voice    `json:"voice,omitempty"`
}

// Head retains stable identity independently of immutable versions.
type Head struct {
	ID              uuid.UUID
	OwnerScopeID    uuid.UUID
	PhysicalScopeID uuid.UUID
	Revision        int64
	VersionID       uuid.UUID
	CreatedBy       uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

// Version preserves the exact content and its canonical digest.
type Version struct {
	ID              uuid.UUID
	SubjectID       uuid.UUID
	OwnerScopeID    uuid.UUID
	PhysicalScopeID uuid.UUID
	Number          int64
	PreviousID      *uuid.UUID
	CreatedBy       uuid.UUID
	CreatedAt       time.Time
	Content         Content
	SHA256          string
}

func validHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

// UTF16Length matches the browser maxlength contract, including surrogate pairs.
func UTF16Length(value string) int { return len(utf16.Encode([]rune(value))) }

// Validate checks persisted media evidence without granting current access.
func (m MediaFact) Validate() error {
	if m.AssetID == uuid.Nil || m.Revision < 1 || m.ByteSize < 1 || !validHash(m.SHA256) || strings.TrimSpace(m.MIMEType) == "" || !utf8.ValidString(m.FileName) || len(m.FileName) > 1024 {
		return ErrInvalid
	}
	if m.Kind != "image" && m.Kind != "video" && m.Kind != "audio" {
		return ErrInvalid
	}
	if m.SourceProjectID != nil {
		if *m.SourceProjectID == uuid.Nil || m.PersonalOrgID != nil || m.PersonalActorID != nil {
			return ErrInvalid
		}
	} else if m.PersonalOrgID == nil || m.PersonalActorID == nil || *m.PersonalOrgID == uuid.Nil || *m.PersonalActorID == uuid.Nil {
		return ErrInvalid
	}
	if (m.RenditionID == nil) != (m.RenditionSHA256 == nil) || m.RenditionID != nil && (*m.RenditionID == uuid.Nil || !validHash(*m.RenditionSHA256)) {
		return ErrInvalid
	}
	if m.DurationMS != nil && *m.DurationMS <= 0 || m.Width != nil && *m.Width <= 0 || m.Height != nil && *m.Height <= 0 {
		return ErrInvalid
	}
	return nil
}

// Validate enforces the new subject write contract without touching Bible history.
func (c Content) Validate() error {
	if c.SchemaVersion != 1 || !utf8.ValidString(c.Name) || strings.TrimSpace(c.Name) != c.Name || c.Name == "" || UTF16Length(c.Name) > MaxNameUnits || strings.ContainsRune(c.Name, 0) || !utf8.ValidString(c.Description) || utf8.RuneCountInString(c.Description) > MaxDescriptionScalars || strings.ContainsRune(c.Description, 0) || len(c.References) > MaxReferences {
		return ErrInvalid
	}
	if c.Primary.ID == uuid.Nil || c.Primary.Media.Validate() != nil || c.Primary.Media.Kind != "image" && c.Primary.Media.Kind != "video" {
		return ErrInvalid
	}
	seen := map[uuid.UUID]bool{c.Primary.ID: true}
	assets := map[uuid.UUID]bool{c.Primary.Media.AssetID: true}
	for _, ref := range c.References {
		if ref.ID == uuid.Nil || seen[ref.ID] || assets[ref.Media.AssetID] || ref.Media.Kind != "image" || ref.Media.Validate() != nil {
			return ErrInvalid
		}
		seen[ref.ID], assets[ref.Media.AssetID] = true, true
	}
	if c.Voice != nil {
		voice := c.Voice
		if !utf8.ValidString(voice.Name) || strings.TrimSpace(voice.Name) == "" || utf8.RuneCountInString(voice.Name) > 512 || voice.Binding.ID == uuid.Nil || seen[voice.Binding.ID] || voice.Binding.Media.Kind != "audio" || voice.Binding.Media.Validate() != nil {
			return ErrInvalid
		}
	}
	return nil
}

// Digest returns a canonical immutable representation.
func Digest(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:]), nil
}
