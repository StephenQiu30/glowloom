package domain

import "time"

const (
	ReadinessPendingValidation = "PENDING_VALIDATION"
	ReadinessReady             = "READY"
	ReadinessQuarantined       = "QUARANTINED"
	ReadinessUnavailable       = "UNAVAILABLE"
	ReadinessTombstoned        = "TOMBSTONED"

	LocationStaging = "STAGING"
	LocationPrimary = "PRIMARY"
)

type Artifact struct {
	ID, WorkspaceID, ProjectID             string
	SourceType, SourceID, OutputKey        string
	MediaType, SHA256, Status, FailureCode string
	SizeBytes                              int64
	Width, Height, Revision                int
	CreatedAt, UpdatedAt                   time.Time
}

type Location struct {
	ID, WorkspaceID, ArtifactID               string
	StorageProfile, Bucket, ObjectKey, Region string
	Checksum, Status                          string
	LocationNo                                int
	CreatedAt, UpdatedAt                      time.Time
}

type ArtifactWithLocation struct {
	Artifact Artifact
	Location Location
}
