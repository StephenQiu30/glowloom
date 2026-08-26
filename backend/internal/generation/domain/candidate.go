package domain

import (
	"slices"
	"time"
)

const (
	CandidateQCPassed = "QC_PASSED"
	CandidateQCFailed = "QC_FAILED"
	QCPassed          = "PASSED"
	QCFailed          = "FAILED"
)

type Candidate struct {
	ID, WorkspaceID, ProjectID                   string
	ProviderJobID, OutputKey, ArtifactID         string
	ArtifactSHA256, MediaType, Status, CreatedBy string
	ArtifactRevision, Width, Height, Revision    int
	CreatedAt, UpdatedAt                         time.Time
}

type ImageQCPolicy struct {
	Version           string   `json:"version"`
	AllowedMediaTypes []string `json:"allowed_media_types"`
	MinWidth          int      `json:"min_width"`
	MinHeight         int      `json:"min_height"`
	MaxPixels         int64    `json:"max_pixels"`
}

type QCReport struct {
	ID, WorkspaceID, CandidateID   string
	Policy                         ImageQCPolicy
	PolicyHash, Status, ReportHash string
	FailureCodes                   []string
	CreatedAt                      time.Time
}

type CandidateWithReport struct {
	Candidate Candidate
	Report    QCReport
}

func EvaluateImage(mediaType string, width, height int, policy ImageQCPolicy) []string {
	failures := make([]string, 0, 4)
	if !slices.Contains(policy.AllowedMediaTypes, mediaType) {
		failures = append(failures, "media_type_not_allowed")
	}
	if width < policy.MinWidth {
		failures = append(failures, "width_below_minimum")
	}
	if height < policy.MinHeight {
		failures = append(failures, "height_below_minimum")
	}
	if height > 0 && int64(width) > policy.MaxPixels/int64(height) {
		failures = append(failures, "pixel_count_exceeded")
	}
	return failures
}
