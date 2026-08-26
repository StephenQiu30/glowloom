package domain

import "time"

const (
	ProviderJobDispatching = "DISPATCHING"
	ProviderJobRunning     = "RUNNING"
	ProviderJobUnknown     = "UNKNOWN"
	ProviderJobSucceeded   = "SUCCEEDED"
	ProviderJobFailed      = "FAILED"

	ProviderResultSucceeded = "SUCCEEDED"
	ProviderResultFailed    = "FAILED"
)

type ProviderBinding struct {
	ID, WorkspaceID, ProjectID        string
	Capability, ProviderKey, ModelKey string
	CredentialRef, ContentHash        string
	Revision                          int64
	CreatedBy                         string
	CreatedAt                         time.Time
}

type GenerationRequest struct {
	ID, WorkspaceID, ProjectID, IntentID string
	BindingID                            string
	BindingRevision                      int64
	Capability, ProviderKey, ModelKey    string
	CredentialRef, RequestKey, InputHash string
	Units                                int64
	ContentHash, CreatedBy               string
	CreatedAt                            time.Time
}

type ProviderOutput struct {
	OutputKey        string `json:"output_key"`
	StagingObjectKey string `json:"staging_object_key"`
	SHA256           string `json:"sha256"`
	MediaType        string `json:"media_type"`
	Bytes            int64  `json:"bytes"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
}

type ProviderJob struct {
	ID, WorkspaceID, ProjectID, IntentID, RequestID string
	ProviderKey, RequestKey, ProviderJobKey         string
	Status, ProviderReceiptID, ContentHash          string
	Revision                                        int64
	CreatedAt, UpdatedAt                            time.Time
}

type ProviderResultReceipt struct {
	ID, WorkspaceID, ProjectID, JobID, RequestID string
	ProviderKey, ProviderJobKey, ProviderEventID string
	Status, FailureCode, ContentHash             string
	ActualUnits                                  int64
	Outputs                                      []ProviderOutput
	OccurredAt, ReceivedAt                       time.Time
}

func SameProviderBinding(left, right ProviderBinding) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.Capability == right.Capability && left.ProviderKey == right.ProviderKey && left.ModelKey == right.ModelKey &&
		left.CredentialRef == right.CredentialRef && left.Revision == right.Revision &&
		left.ContentHash == right.ContentHash && left.CreatedBy == right.CreatedBy && left.CreatedAt.Equal(right.CreatedAt)
}

func SameGenerationRequest(left, right GenerationRequest) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.IntentID == right.IntentID && left.BindingID == right.BindingID && left.BindingRevision == right.BindingRevision &&
		left.Capability == right.Capability && left.ProviderKey == right.ProviderKey && left.ModelKey == right.ModelKey &&
		left.CredentialRef == right.CredentialRef && left.RequestKey == right.RequestKey &&
		left.InputHash == right.InputHash && left.Units == right.Units && left.ContentHash == right.ContentHash &&
		left.CreatedBy == right.CreatedBy && left.CreatedAt.Equal(right.CreatedAt)
}

func SameProviderJob(left, right ProviderJob) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.IntentID == right.IntentID && left.RequestID == right.RequestID && left.ProviderKey == right.ProviderKey &&
		left.RequestKey == right.RequestKey && left.ProviderJobKey == right.ProviderJobKey && left.Status == right.Status &&
		left.ProviderReceiptID == right.ProviderReceiptID && left.Revision == right.Revision &&
		left.ContentHash == right.ContentHash && left.CreatedAt.Equal(right.CreatedAt) && left.UpdatedAt.Equal(right.UpdatedAt)
}

func SameProviderResultReceipt(left, right ProviderResultReceipt) bool {
	if left.ID != right.ID || left.WorkspaceID != right.WorkspaceID || left.ProjectID != right.ProjectID ||
		left.JobID != right.JobID || left.RequestID != right.RequestID || left.ProviderKey != right.ProviderKey ||
		left.ProviderJobKey != right.ProviderJobKey || left.ProviderEventID != right.ProviderEventID ||
		left.Status != right.Status || left.ActualUnits != right.ActualUnits || left.FailureCode != right.FailureCode ||
		left.ContentHash != right.ContentHash || !left.OccurredAt.Equal(right.OccurredAt) ||
		!left.ReceivedAt.Equal(right.ReceivedAt) || len(left.Outputs) != len(right.Outputs) {
		return false
	}
	for index := range left.Outputs {
		if left.Outputs[index] != right.Outputs[index] {
			return false
		}
	}
	return true
}
