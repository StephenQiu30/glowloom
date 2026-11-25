package postgres

import (
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

type packageCommandRecord struct {
	ActorID, OrgID, IdemKey, JobID uuid.UUID
	Action, RequestSHA256          string
	Response                       []byte
}

func (packageCommandRecord) TableName() string { return "media.package_command" }

type packageObjectRecord struct {
	JobID            uuid.UUID
	ObjectIndex      int
	ObjectLocationID *uuid.UUID
	ObjectKey        string
	ByteSize         int64
	ContentType      string
	SHA256           string
	Status           application.PackageObjectStatus
}

func (packageObjectRecord) TableName() string { return "media.package_object" }
