package domain

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrInvalidProjectFolder rejects incomplete directory or placement facts.
	ErrInvalidProjectFolder = errors.New("invalid project folder")
	// ErrProjectFolderNotFound hides directories outside the current personal library.
	ErrProjectFolderNotFound = errors.New("project folder not found")
	// ErrProjectFolderRevisionConflict preserves concurrent library edits.
	ErrProjectFolderRevisionConflict = errors.New("project folder revision conflict")
	// ErrProjectFolderCoverUnavailable rejects a cover that is no longer eligible.
	ErrProjectFolderCoverUnavailable = errors.New("project folder cover unavailable")
)

// FolderCover references one reviewed image without persisting a preview URL.
type FolderCover struct {
	ProjectID uuid.UUID `json:"project_id"`
	AssetID   uuid.UUID `json:"asset_id"`
}

// ProjectFolder is a flat personal project-library directory, not a project aggregate.
type ProjectFolder struct {
	ID         uuid.UUID    `json:"id"`
	OrgID      uuid.UUID    `json:"org_id"`
	ActorID    uuid.UUID    `json:"actor_id"`
	Name       string       `json:"name"`
	Cover      *FolderCover `json:"cover"`
	Revision   int64        `json:"revision"`
	IsDelete   bool         `json:"is_delete"`
	DeleteTime *time.Time   `json:"delete_time"`
	CreateTime time.Time    `json:"create_time"`
	UpdateTime time.Time    `json:"update_time"`
}

// ValidFolderName preserves names as content while rejecting malformed identifiers.
func ValidFolderName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 160 && !strings.ContainsRune(name, 0)
}

// Validate checks persisted scope, timestamps, cover identity and revision.
func (f ProjectFolder) Validate() error {
	if f.ID == uuid.Nil || f.OrgID == uuid.Nil || f.ActorID == uuid.Nil || !ValidFolderName(f.Name) || f.Revision < 1 || f.Revision > math.MaxInt32 || f.CreateTime.IsZero() || f.UpdateTime.IsZero() || f.IsDelete != (f.DeleteTime != nil) || f.DeleteTime != nil && f.DeleteTime.IsZero() || f.Cover != nil && (f.Cover.ProjectID == uuid.Nil || f.Cover.AssetID == uuid.Nil) {
		return ErrInvalidProjectFolder
	}
	return nil
}

// FolderPlacement is one actor's classification; revision zero denotes virtual root.
type FolderPlacement struct {
	OrgID     uuid.UUID  `json:"org_id"`
	ActorID   uuid.UUID  `json:"actor_id"`
	ProjectID uuid.UUID  `json:"project_id"`
	FolderID  *uuid.UUID `json:"folder_id"`
	Revision  int64      `json:"revision"`
}

// Validate rejects synthetic root revisions and incomplete persisted assignments.
func (p FolderPlacement) Validate() error {
	if p.OrgID == uuid.Nil || p.ActorID == uuid.Nil || p.ProjectID == uuid.Nil || p.Revision < 0 || p.Revision > math.MaxInt32 || p.FolderID != nil && (*p.FolderID == uuid.Nil || p.Revision == 0) {
		return ErrInvalidProjectFolder
	}
	return nil
}
