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

type CandidateReference struct {
	ID               string `json:"id"`
	Revision         int    `json:"revision"`
	ArtifactID       string `json:"artifact_id"`
	ArtifactRevision int    `json:"artifact_revision"`
	ArtifactSHA256   string `json:"artifact_sha256"`
	QCReportID       string `json:"qc_report_id"`
	QCReportHash     string `json:"qc_report_hash"`
}

type CandidateSelection struct {
	ID, WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID                        string
	SubjectType, SubjectID                               string
	SubjectRevision                                      int
	Candidates                                           []CandidateReference
	CandidateSetHash, SelectedCandidateID                string
	SelectedArtifactID, SelectedArtifactSHA256           string
	ReviewerID, ContentHash                              string
	Revision                                             int
	CreatedBy                                            string
	CreatedAt                                            time.Time
}

func SameSelectionBinding(left, right CandidateSelection) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.WorkflowRunID == right.WorkflowRunID && left.NodeRunID == right.NodeRunID &&
		left.HumanTaskID == right.HumanTaskID && left.ReviewDecisionID == right.ReviewDecisionID &&
		left.SubjectType == right.SubjectType && left.SubjectID == right.SubjectID &&
		left.SubjectRevision == right.SubjectRevision && slices.Equal(left.Candidates, right.Candidates) &&
		left.CandidateSetHash == right.CandidateSetHash && left.SelectedCandidateID == right.SelectedCandidateID &&
		left.SelectedArtifactID == right.SelectedArtifactID &&
		left.SelectedArtifactSHA256 == right.SelectedArtifactSHA256 && left.ReviewerID == right.ReviewerID &&
		left.ContentHash == right.ContentHash && left.Revision == 1 && right.Revision == 1
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
