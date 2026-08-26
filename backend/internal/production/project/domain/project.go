package domain

import "time"

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

type Project struct {
	ID               string
	WorkspaceID      string
	Name             string
	Description      *string
	AspectRatio      string
	Language         string
	VisualStyle      *string
	TargetDurationMS int
	Status           Status
	Revision         int
	ArchivedAt       *time.Time
	ArchivedBy       *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
