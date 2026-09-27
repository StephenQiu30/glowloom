// Package domain owns project specifications and lifecycle invariants.
package domain

import (
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalidProject means a project specification violates its persisted contract.
var ErrInvalidProject = errors.New("invalid project")

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
	Revision            int64
}

// Validate checks the core project specifications before persistence.
func (p Project) Validate() error {
	name := strings.TrimSpace(p.Name)
	if p.ID == uuid.Nil || p.OrgID == uuid.Nil || name == "" ||
		!utf8.ValidString(p.Name) || utf8.RuneCountInString(p.Name) > 50 ||
		(p.AspectRatio != "9:16" && p.AspectRatio != "16:9") ||
		p.Resolution != "1080p" ||
		(p.Status != "active" && p.Status != "archived") ||
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
