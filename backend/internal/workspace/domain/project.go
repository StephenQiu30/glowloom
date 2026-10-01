// Package domain owns project specifications and lifecycle invariants.
package domain

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalidProject means a project specification violates its persisted contract.
var ErrInvalidProject = errors.New("invalid project")

// ErrProjectStateConflict means a lifecycle transition or write is not allowed.
var ErrProjectStateConflict = errors.New("project state conflict")

// ErrProjectRevisionConflict means another command changed the project first.
var ErrProjectRevisionConflict = errors.New("project revision conflict")

// ErrProjectHasInflightOperations means an operation must settle before archival or deletion.
var ErrProjectHasInflightOperations = errors.New("project has inflight operations")

// ErrProjectRestoreExpired means the thirty-day recovery window has closed.
var ErrProjectRestoreExpired = errors.New("project restore window expired")

const projectRecoveryWindow = 30 * 24 * time.Hour

// Project is the identity and mutable settings of one production project.
type Project struct {
	ID                  uuid.UUID
	OrgID               uuid.UUID
	Name                string
	Description         string
	AspectRatio         string
	StyleType           string
	StyleSubtype        string
	StylePresetID       uuid.UUID
	Resolution          string
	AllowOverseasModels bool
	Status              string
	ArchivedAt          *time.Time
	IsDelete            bool
	DeleteTime          *time.Time
	PurgeAfter          *time.Time
	Revision            int64
	CreateTime          time.Time
	UpdateTime          time.Time
}

// Validate checks the core project specifications before persistence.
func (p Project) Validate() error {
	name := strings.TrimSpace(p.Name)
	if p.ID == uuid.Nil || p.OrgID == uuid.Nil || name == "" ||
		!utf8.ValidString(p.Name) || utf8.RuneCountInString(p.Name) > 50 ||
		(p.AspectRatio != "9:16" && p.AspectRatio != "16:9") ||
		p.Resolution != "1080p" ||
		(p.Status != "active" && p.Status != "archived" && p.Status != "copying") ||
		p.Revision < 1 || p.Revision > math.MaxInt32 {
		return ErrInvalidProject
	}
	switch p.StyleType {
	case "realistic":
		if p.StyleSubtype != "" {
			return ErrInvalidProject
		}
	case "stylized":
		switch p.StyleSubtype {
		case "anime_jp", "guofeng_xianxia", "cartoon_3d", "manhwa":
		default:
			return ErrInvalidProject
		}
	default:
		return ErrInvalidProject
	}
	return nil
}

// CanWrite rejects mutations before publication, during archival, or in the recycle bin.
func (p Project) CanWrite() error {
	if p.IsDelete || p.Status == "archived" || p.Status == "copying" {
		return ErrProjectStateConflict
	}
	if p.Status != "active" {
		return ErrInvalidProject
	}
	return nil
}

// Archive makes an active project read only once all blocking operations have settled.
// Callers must check blocking operations in the same transaction as persistence.
func (p *Project) Archive(now time.Time, hasBlockingOperations bool) error {
	if p.IsDelete || p.Status != "active" {
		return ErrProjectStateConflict
	}
	if hasBlockingOperations {
		return ErrProjectHasInflightOperations
	}
	if now.IsZero() || p.Revision < 1 || p.Revision >= math.MaxInt32 {
		return ErrInvalidProject
	}
	archivedAt := now.UTC()
	p.Status = "archived"
	p.ArchivedAt = &archivedAt
	p.Revision++
	return nil
}

// Unarchive resumes an archived project without changing its other settings.
func (p *Project) Unarchive() error {
	if p.IsDelete || p.Status != "archived" {
		return ErrProjectStateConflict
	}
	if p.Revision < 1 || p.Revision >= math.MaxInt32 {
		return ErrInvalidProject
	}
	p.Status = "active"
	p.ArchivedAt = nil
	p.Revision++
	return nil
}

// Delete moves an active or archived project to the recycle bin for thirty days.
// The status is retained so Restore can return it to its prior lifecycle state.
// Callers must check blocking operations in the same transaction as persistence.
func (p *Project) Delete(now time.Time, hasBlockingOperations bool) error {
	if p.IsDelete || (p.Status != "active" && p.Status != "archived") {
		return ErrProjectStateConflict
	}
	if hasBlockingOperations {
		return ErrProjectHasInflightOperations
	}
	if now.IsZero() || p.Revision < 1 || p.Revision >= math.MaxInt32 {
		return ErrInvalidProject
	}
	deletedAt := now.UTC()
	purgeAfter := deletedAt.Add(projectRecoveryWindow)
	p.IsDelete = true
	p.DeleteTime = &deletedAt
	p.PurgeAfter = &purgeAfter
	p.Revision++
	return nil
}

// Restore returns a deleted project to its active or archived state before purge is due.
func (p *Project) Restore(now time.Time) error {
	if !p.IsDelete || (p.Status != "active" && p.Status != "archived") {
		return ErrProjectStateConflict
	}
	if now.IsZero() || p.DeleteTime == nil || p.PurgeAfter == nil ||
		now.Before(*p.DeleteTime) || p.Revision < 1 || p.Revision >= math.MaxInt32 {
		return ErrInvalidProject
	}
	if !now.Before(*p.PurgeAfter) {
		return ErrProjectRestoreExpired
	}
	p.IsDelete = false
	p.DeleteTime = nil
	p.PurgeAfter = nil
	p.Revision++
	return nil
}
