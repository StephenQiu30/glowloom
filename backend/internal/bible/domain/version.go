package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrCorruptHistory means persisted immutable facts disagree with their typed content.
var ErrCorruptHistory = errors.New("corrupt Bible history")

// Head contains current pointers for one stable Bible identity.
type Head struct {
	ID                 uuid.UUID  `json:"id"`
	OrgID              uuid.UUID  `json:"org_id"`
	ProjectID          uuid.UUID  `json:"project_id"`
	Kind               Kind       `json:"kind"`
	Revision           int64      `json:"revision"`
	CurrentVersionID   uuid.UUID  `json:"current_version_id"`
	ConfirmedVersionID *uuid.UUID `json:"confirmed_version_id,omitempty"`
	RedirectID         *uuid.UUID `json:"redirect_id,omitempty"`
	Deleted            bool       `json:"deleted"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// VersionOrigin records manual content separately from a verified generated result.
type VersionOrigin string

// Origins distinguish manual editing from verified generation provenance.
const (
	OriginManual VersionOrigin = "manual"
	OriginAI     VersionOrigin = "ai"
)

// ResultSource is an owner-proven immutable generation output provenance.
type ResultSource struct {
	OperationID  uuid.UUID `json:"operation_id"`
	OutputID     uuid.UUID `json:"output_id"`
	OutputSHA256 string    `json:"output_sha256"`
	InputSHA256  string    `json:"input_sha256"`
}

// Version contains one complete typed immutable identity snapshot.
type Version struct {
	ID            uuid.UUID         `json:"id"`
	EntryID       uuid.UUID         `json:"entry_id"`
	OrgID         uuid.UUID         `json:"org_id"`
	ProjectID     uuid.UUID         `json:"project_id"`
	Kind          Kind              `json:"kind"`
	Number        int64             `json:"number"`
	PreviousID    *uuid.UUID        `json:"previous_id,omitempty"`
	ActorID       uuid.UUID         `json:"actor_id"`
	CreatedAt     time.Time         `json:"created_at"`
	Origin        VersionOrigin     `json:"origin"`
	Result        *ResultSource     `json:"result,omitempty"`
	ContentSHA256 string            `json:"content_sha256"`
	Character     *CharacterContent `json:"character,omitempty"`
	Location      *LocationContent  `json:"location,omitempty"`
	Prop          *PropContent      `json:"prop,omitempty"`
}

// Content returns canonical typed bytes with exactly one matching content kind.
func (v Version) Content() ([]byte, string, error) {
	switch v.Kind {
	case KindCharacter:
		if v.Character == nil || v.Location != nil || v.Prop != nil {
			return nil, "", ErrInvalidContent
		}
		return EncodeCharacter(*v.Character)
	case KindLocation:
		if v.Location == nil || v.Character != nil || v.Prop != nil {
			return nil, "", ErrInvalidContent
		}
		return EncodeLocation(*v.Location)
	case KindProp:
		if v.Prop == nil || v.Character != nil || v.Location != nil {
			return nil, "", ErrInvalidContent
		}
		return EncodeProp(*v.Prop)
	default:
		return nil, "", ErrInvalidContent
	}
}

// Validate verifies the version identity, provenance and exact canonical hash.
func (v Version) Validate() error {
	if v.ID == uuid.Nil || v.EntryID == uuid.Nil || v.OrgID == uuid.Nil || v.ProjectID == uuid.Nil || v.ActorID == uuid.Nil || v.Number < 1 || v.CreatedAt.IsZero() || (v.PreviousID != nil && (*v.PreviousID == uuid.Nil || *v.PreviousID == v.ID)) {
		return ErrInvalidContent
	}
	if (v.Origin == OriginManual && v.Result != nil) || (v.Origin == OriginAI && (v.Result == nil || v.Result.OperationID == uuid.Nil || v.Result.OutputID == uuid.Nil || !validSHA(v.Result.InputSHA256) || !validSHA(v.Result.OutputSHA256))) || (v.Origin != OriginManual && v.Origin != OriginAI) {
		return ErrInvalidContent
	}
	_, hash, err := v.Content()
	if err != nil || hash != v.ContentSHA256 {
		return ErrInvalidContent
	}
	return nil
}

// Name returns the validated display name without normalizing user text.
func (v Version) Name() string {
	switch v.Kind {
	case KindCharacter:
		if v.Character != nil {
			return v.Character.Name
		}
	case KindLocation:
		if v.Location != nil {
			return v.Location.Name
		}
	case KindProp:
		if v.Prop != nil {
			return v.Prop.Name
		}
	}
	return ""
}

// VisuallyReady indicates complete multi-angle evidence on the selected look.
func (l LookContent) VisuallyReady() bool {
	front, side, back := false, false, false
	for _, r := range l.References {
		switch r.Role {
		case RoleTurnaround:
			return true
		case RoleFront:
			front = true
		case RoleSide:
			side = true
		case RoleBack:
			back = true
		}
	}
	return front && side && back
}

// Confirmation permanently records the version accepted for a stable identity.
type Confirmation struct {
	ID        uuid.UUID `json:"id"`
	EntryID   uuid.UUID `json:"entry_id"`
	VersionID uuid.UUID `json:"version_id"`
	Revision  int64     `json:"revision"`
	ActorID   uuid.UUID `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Redirect permanently records a merge without rewriting historical versions.
type Redirect struct {
	ID              uuid.UUID `json:"id"`
	SourceID        uuid.UUID `json:"source_id"`
	TargetID        uuid.UUID `json:"target_id"`
	SourceVersionID uuid.UUID `json:"source_version_id"`
	TargetVersionID uuid.UUID `json:"target_version_id"`
	ActorID         uuid.UUID `json:"actor_id"`
	CreatedAt       time.Time `json:"created_at"`
}

// Split preserves the reviewed parent version and the newly created identity/version.
type Split struct {
	ID              uuid.UUID `json:"id"`
	SourceID        uuid.UUID `json:"source_id"`
	SourceVersionID uuid.UUID `json:"source_version_id"`
	TargetID        uuid.UUID `json:"target_id"`
	TargetVersionID uuid.UUID `json:"target_version_id"`
	ActorID         uuid.UUID `json:"actor_id"`
	CreatedAt       time.Time `json:"created_at"`
}
