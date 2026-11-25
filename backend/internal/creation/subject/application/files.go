package application

import (
	"context"
	"io"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// File is a verified temporary attachment. The consumer must close it.
type File struct {
	Reader   io.ReadSeekCloser
	Name     string
	MIMEType string
	Size     int64
}

// ExportItem pins one explicitly selected subject version.
type ExportItem struct {
	ID        uuid.UUID `json:"id"`
	VersionID uuid.UUID `json:"version_id"`
}

// ExportInput exports an explicit bounded selection, never the entire space implicitly.
type ExportInput struct {
	Items []ExportItem `json:"items"`
}

// Files exposes exact bound media and portable packages without storage addresses.
type Files interface {
	Media(context.Context, identity.Principal, uuid.UUID, uuid.UUID, uuid.UUID) (File, error)
	Export(context.Context, identity.Principal, ExportInput) (File, error)
}
