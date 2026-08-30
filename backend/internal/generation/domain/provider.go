package domain

import "time"

const (
	ProviderJobPending          = "PENDING"
	ProviderJobRunning          = "RUNNING"
	ProviderJobOutcomeUnknown   = "OUTCOME_UNKNOWN"
	ProviderJobSucceeded        = "SUCCEEDED"
	ProviderJobPartialSucceeded = "PARTIAL_SUCCEEDED"
	ProviderJobFailed           = "FAILED"

	ProviderCallPending        = "PENDING"
	ProviderCallDispatching    = "DISPATCHING"
	ProviderCallSubmitted      = "SUBMITTED"
	ProviderCallRunning        = "RUNNING"
	ProviderCallOutcomeUnknown = "OUTCOME_UNKNOWN"
	ProviderCallSucceeded      = "SUCCEEDED"
	ProviderCallFailed         = "FAILED"

	ProviderResultSucceeded = "SUCCEEDED"
	ProviderResultFailed    = "FAILED"
)

type GenerationRequest struct {
	ID, WorkspaceID, ProjectID, IntentID           string
	TargetID                                       string
	BindingID                                      string
	BindingRevision                                int64
	BindingContentHash                             string
	Purpose, ProviderKey, ExternalModelID          string
	ConnectionVersionID, CredentialVersionID       string
	ModelProfileVersionID, ModelProfileContentHash string
	ModelProfileRevision                           int64
	PriceQuoteID, PriceQuoteContentHash            string
	PriceQuoteRevision                             int64
	BillingMetric, RequestKey, TargetHash          string
	EstimatedUnits                                 int64
	ContentHash, CreatedBy                         string
	CreatedAt                                      time.Time
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

type ProviderUsageObservation struct {
	InputTokens     int64 `json:"input_tokens,omitempty"`
	OutputTokens    int64 `json:"output_tokens,omitempty"`
	TotalTokens     int64 `json:"total_tokens,omitempty"`
	ImageCount      int64 `json:"image_count,omitempty"`
	VideoDurationMS int64 `json:"video_duration_ms,omitempty"`
}

type ProviderJob struct {
	ID, WorkspaceID, ProjectID, IntentID, RequestID string
	ProviderKey, RequestKey, Status, CallSetHash    string
	CallCount, DispatchedCallCount                  int
	SucceededCallCount, FailedCallCount             int
	Revision                                        int64
	ContentHash                                     string
	CreatedAt, UpdatedAt                            time.Time
}

type ProviderCall struct {
	ID, WorkspaceID, ProjectID, JobID              string
	CallKey, RequestHash, Status                   string
	LocalFailureCode, RemoteRequestID, RemoteJobID string
	CandidateIndex, RequestedOutputCount           int
	DispatchBoundaryEnteredAt                      *time.Time
	QueryDeadlineAt                                *time.Time
	RemoteExpiresAt                                *time.Time
	Revision                                       int64
	ContentHash                                    string
	CreatedAt, UpdatedAt                           time.Time
}

type ProviderResultReceipt struct {
	ID, WorkspaceID, ProjectID, CallID string
	ProviderEventID                    string
	Status, FailureCode                string
	OutputCount                        int
	ProviderUsageObservation           ProviderUsageObservation
	ProviderUsageHash                  string
	Output                             *ProviderOutput
	ContentHash                        string
	OccurredAt, ReceivedAt             time.Time
}

func SameGenerationRequest(left, right GenerationRequest) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.IntentID == right.IntentID && left.TargetID == right.TargetID &&
		left.BindingID == right.BindingID && left.BindingRevision == right.BindingRevision &&
		left.BindingContentHash == right.BindingContentHash &&
		left.Purpose == right.Purpose && left.ProviderKey == right.ProviderKey && left.ExternalModelID == right.ExternalModelID &&
		left.ConnectionVersionID == right.ConnectionVersionID && left.CredentialVersionID == right.CredentialVersionID &&
		left.ModelProfileVersionID == right.ModelProfileVersionID &&
		left.ModelProfileRevision == right.ModelProfileRevision && left.ModelProfileContentHash == right.ModelProfileContentHash &&
		left.PriceQuoteID == right.PriceQuoteID && left.PriceQuoteRevision == right.PriceQuoteRevision &&
		left.PriceQuoteContentHash == right.PriceQuoteContentHash && left.BillingMetric == right.BillingMetric &&
		left.RequestKey == right.RequestKey && left.TargetHash == right.TargetHash &&
		left.EstimatedUnits == right.EstimatedUnits && left.ContentHash == right.ContentHash &&
		left.CreatedBy == right.CreatedBy && left.CreatedAt.Equal(right.CreatedAt)
}

func SameProviderJob(left, right ProviderJob) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.IntentID == right.IntentID && left.RequestID == right.RequestID && left.ProviderKey == right.ProviderKey &&
		left.RequestKey == right.RequestKey && left.Status == right.Status && left.CallSetHash == right.CallSetHash &&
		left.CallCount == right.CallCount && left.DispatchedCallCount == right.DispatchedCallCount &&
		left.SucceededCallCount == right.SucceededCallCount && left.FailedCallCount == right.FailedCallCount &&
		left.Revision == right.Revision && left.ContentHash == right.ContentHash &&
		left.CreatedAt.Equal(right.CreatedAt) && left.UpdatedAt.Equal(right.UpdatedAt)
}

func SameProviderCall(left, right ProviderCall) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.JobID == right.JobID && left.CandidateIndex == right.CandidateIndex && left.CallKey == right.CallKey &&
		left.RequestHash == right.RequestHash && left.RequestedOutputCount == right.RequestedOutputCount &&
		left.Status == right.Status && left.LocalFailureCode == right.LocalFailureCode &&
		left.RemoteRequestID == right.RemoteRequestID && left.RemoteJobID == right.RemoteJobID &&
		optionalProviderTimeEqual(left.DispatchBoundaryEnteredAt, right.DispatchBoundaryEnteredAt) &&
		optionalProviderTimeEqual(left.QueryDeadlineAt, right.QueryDeadlineAt) &&
		optionalProviderTimeEqual(left.RemoteExpiresAt, right.RemoteExpiresAt) &&
		left.Revision == right.Revision && left.ContentHash == right.ContentHash &&
		left.CreatedAt.Equal(right.CreatedAt) && left.UpdatedAt.Equal(right.UpdatedAt)
}

func SameProviderResultReceipt(left, right ProviderResultReceipt) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.CallID == right.CallID && left.ProviderEventID == right.ProviderEventID && left.Status == right.Status &&
		left.OutputCount == right.OutputCount && left.FailureCode == right.FailureCode &&
		left.ProviderUsageObservation == right.ProviderUsageObservation && left.ProviderUsageHash == right.ProviderUsageHash &&
		optionalProviderOutputEqual(left.Output, right.Output) && left.ContentHash == right.ContentHash &&
		left.OccurredAt.Equal(right.OccurredAt) && left.ReceivedAt.Equal(right.ReceivedAt)
}

func optionalProviderTimeEqual(left, right *time.Time) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && left.Equal(*right))
}

func optionalProviderOutputEqual(left, right *ProviderOutput) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}
