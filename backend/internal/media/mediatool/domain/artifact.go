package domain

import (
	"time"

	"github.com/google/uuid"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// Artifact is an exact, private engineering export; it is never a fake media asset.
type Artifact struct {
	ID             uuid.UUID                   `json:"id"`
	JobID          uuid.UUID                   `json:"job_id"`
	ProjectID      uuid.UUID                   `json:"project_id"`
	Attempt        int                         `json:"attempt"`
	Format         OutputKind                  `json:"format"`
	FormatVersion  string                      `json:"format_version"`
	FileName       string                      `json:"file_name"`
	MIMEType       string                      `json:"mime_type"`
	ByteSize       int64                       `json:"byte_size"`
	SHA256         string                      `json:"sha256"`
	ObjectKey      string                      `json:"-"`
	ObjectLocation *mediadomain.ObjectLocation `json:"-"`
	CreatedAt      time.Time                   `json:"created_at"`
}

// IsArtifact distinguishes a ZIP package from playable video/audio outputs.
func (k OutputKind) IsArtifact() bool {
	return k == OutputMediaBundle || k == OutputFCPXML || k == OutputXMEML || k == OutputJianying
}

// FormatVersion pins each supported serializer independently of target applications.
func (k OutputKind) FormatVersion() string {
	switch k {
	case OutputMediaBundle:
		return "1"
	case OutputFCPXML:
		return "1.10"
	case OutputXMEML:
		return "5"
	case OutputJianying:
		return "134.0.0"
	default:
		return ""
	}
}
