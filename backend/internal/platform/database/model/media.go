package model

import (
	"time"

	"github.com/google/uuid"
)

type MediaObject struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey"`
	WorkspaceID      uuid.UUID  `gorm:"type:uuid;not null;index:ix_med_objects_workspace_status,priority:1"`
	Kind             string     `gorm:"type:varchar(20);not null;check:ck_med_object_kind,kind IN ('image','video','audio','subtitle','delivery','document')"`
	SourceType       string     `gorm:"type:varchar(20);not null;check:ck_med_object_source,source_type IN ('upload','generated','rendered')"`
	Status           string     `gorm:"type:varchar(20);not null;index:ix_med_objects_workspace_status,priority:2;check:ck_med_object_status,status IN ('active','archived')"`
	CurrentVersionID *uuid.UUID `gorm:"type:uuid"`
	Revision         int        `gorm:"not null;check:ck_med_object_revision,revision >= 1"`
	CreatedAt        time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt        time.Time  `gorm:"type:timestamptz;not null"`
	Workspace        Workspace  `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (MediaObject) TableName() string { return "med_objects" }

type MediaVersion struct {
	ID              uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID     uuid.UUID   `gorm:"type:uuid;not null;index:ix_med_versions_workspace_created,priority:1"`
	MediaObjectID   uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_med_version_object_no,priority:1"`
	VersionNo       int         `gorm:"not null;uniqueIndex:uq_med_version_object_no,priority:2;check:ck_med_version_no,version_no >= 1"`
	Filename        string      `gorm:"type:varchar(255);not null"`
	SHA256          string      `gorm:"type:char(64);not null;index:ix_med_versions_sha256"`
	SizeBytes       int64       `gorm:"not null;check:ck_med_version_size,size_bytes > 0"`
	MIMEType        string      `gorm:"type:varchar(120);not null"`
	ObjectKey       string      `gorm:"type:varchar(600);not null;uniqueIndex"`
	ProbeStatus     string      `gorm:"type:varchar(20);not null;check:ck_med_probe_status,probe_status IN ('pending','ready','failed','quarantined')"`
	ProbeAttempt    int         `gorm:"not null;check:ck_med_probe_attempt,probe_attempt >= 0"`
	ProbeErrorCode  *string     `gorm:"type:varchar(80)"`
	ProbeSummary    *string     `gorm:"type:text"`
	ProbeNextAction *string     `gorm:"type:varchar(80)"`
	Width           *int        `gorm:"check:ck_med_version_width,width IS NULL OR width > 0"`
	Height          *int        `gorm:"check:ck_med_version_height,height IS NULL OR height > 0"`
	DurationMS      *int        `gorm:"check:ck_med_version_duration,duration_ms IS NULL OR duration_ms >= 0"`
	Codec           *string     `gorm:"type:varchar(80)"`
	Container       *string     `gorm:"type:varchar(80)"`
	CreatedAt       time.Time   `gorm:"type:timestamptz;not null;index:ix_med_versions_workspace_created,priority:2,sort:desc"`
	MediaObject     MediaObject `gorm:"foreignKey:MediaObjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Workspace       Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (MediaVersion) TableName() string { return "med_versions" }

type UploadSession struct {
	ID             uuid.UUID    `gorm:"type:uuid;primaryKey"`
	WorkspaceID    uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:uq_med_upload_workspace_key,priority:1"`
	MediaObjectID  *uuid.UUID   `gorm:"type:uuid"`
	Status         string       `gorm:"type:varchar(20);not null;check:ck_med_upload_status,status IN ('pending','completed','expired','failed')"`
	Kind           string       `gorm:"type:varchar(20);not null"`
	Filename       string       `gorm:"type:varchar(255);not null"`
	SizeBytes      int64        `gorm:"not null;check:ck_med_upload_size,size_bytes > 0"`
	MIMEType       string       `gorm:"type:varchar(120);not null"`
	SHA256         string       `gorm:"type:char(64);not null"`
	ObjectKey      string       `gorm:"type:varchar(600);not null;uniqueIndex"`
	IdempotencyKey string       `gorm:"type:varchar(200);not null;uniqueIndex:uq_med_upload_workspace_key,priority:2"`
	ExpiresAt      time.Time    `gorm:"type:timestamptz;not null;index:ix_med_upload_expires"`
	CompletedAt    *time.Time   `gorm:"type:timestamptz"`
	CreatedAt      time.Time    `gorm:"type:timestamptz;not null"`
	UpdatedAt      time.Time    `gorm:"type:timestamptz;not null"`
	Workspace      Workspace    `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	MediaObject    *MediaObject `gorm:"foreignKey:MediaObjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
}

func (UploadSession) TableName() string { return "med_upload_sessions" }
