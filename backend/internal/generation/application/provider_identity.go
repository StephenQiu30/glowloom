package application

import (
	"sort"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

type generationRequestHashInput struct {
	WorkspaceID, ProjectID, IntentID, TargetID string
	BindingID, BindingContentHash              string
	BindingRevision                            int64
	Purpose, ProviderKey, ExternalModelID      string
	ConnectionVersionID, CredentialVersionID   string
	ModelProfileVersionID                      string
	ModelProfileRevision                       int64
	ModelProfileContentHash                    string
	PriceQuoteID, PriceQuoteContentHash        string
	PriceQuoteRevision                         int64
	BillingMetric, RequestKey, TargetHash      string
	EstimatedUnits                             int64
}

type providerJobHashInput struct {
	WorkspaceID, ProjectID, IntentID, RequestID  string
	ProviderKey, RequestKey, Status, CallSetHash string
	CallCount, DispatchedCallCount               int
	SucceededCallCount, FailedCallCount          int
	Revision                                     int64
}

type providerCallRequestHashInput struct {
	RequestID, RequestContentHash string
	CandidateIndex                int
	RequestedOutputCount          int
}

type providerCallHashInput struct {
	WorkspaceID, ProjectID, JobID, CallKey, RequestHash string
	CandidateIndex, RequestedOutputCount                int
	Status, LocalFailureCode                            string
	RemoteRequestID, RemoteJobID                        string
	DispatchBoundaryEnteredAt                           string
	QueryDeadlineAt                                     string
	RemoteExpiresAt                                     string
	Revision                                            int64
}

type providerCallSetHashItem struct {
	ID, CallKey, RequestHash, CallContentHash string
	ReceiptID, ReceiptContentHash             string
	CandidateIndex, RequestedOutputCount      int
}

type providerReceiptHashInput struct {
	WorkspaceID, ProjectID, CallID       string
	ProviderEventID, Status, FailureCode string
	OutputCount                          int
	ProviderUsageObservation             domain.ProviderUsageObservation
	ProviderUsageHash                    string
	Output                               *ProviderOutput
	OccurredAt                           string
}

func generationRequestContentHash(value domain.GenerationRequest) (string, error) {
	return platformcommand.InputHash(generationRequestHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, IntentID: value.IntentID, TargetID: value.TargetID,
		BindingID: value.BindingID, BindingRevision: value.BindingRevision, BindingContentHash: value.BindingContentHash,
		Purpose: value.Purpose, ProviderKey: value.ProviderKey, ExternalModelID: value.ExternalModelID,
		ConnectionVersionID: value.ConnectionVersionID, CredentialVersionID: value.CredentialVersionID,
		ModelProfileVersionID: value.ModelProfileVersionID, ModelProfileRevision: value.ModelProfileRevision,
		ModelProfileContentHash: value.ModelProfileContentHash, PriceQuoteID: value.PriceQuoteID,
		PriceQuoteRevision: value.PriceQuoteRevision, PriceQuoteContentHash: value.PriceQuoteContentHash,
		BillingMetric: value.BillingMetric, RequestKey: value.RequestKey, TargetHash: value.TargetHash,
		EstimatedUnits: value.EstimatedUnits,
	})
}

func providerJobContentHash(value domain.ProviderJob) (string, error) {
	return platformcommand.InputHash(providerJobHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, IntentID: value.IntentID,
		RequestID: value.RequestID, ProviderKey: value.ProviderKey, RequestKey: value.RequestKey,
		Status: value.Status, CallSetHash: value.CallSetHash, CallCount: value.CallCount,
		DispatchedCallCount: value.DispatchedCallCount, SucceededCallCount: value.SucceededCallCount,
		FailedCallCount: value.FailedCallCount, Revision: value.Revision,
	})
}

func providerCallRequestHash(request domain.GenerationRequest, candidateIndex int) (string, error) {
	return platformcommand.InputHash(providerCallRequestHashInput{
		RequestID: request.ID, RequestContentHash: request.ContentHash,
		CandidateIndex: candidateIndex, RequestedOutputCount: 1,
	})
}

func providerCallContentHash(value domain.ProviderCall) (string, error) {
	dispatchAt := ""
	if value.DispatchBoundaryEnteredAt != nil {
		dispatchAt = value.DispatchBoundaryEnteredAt.UTC().Format(time.RFC3339Nano)
	}
	return platformcommand.InputHash(providerCallHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, JobID: value.JobID,
		CallKey: value.CallKey, RequestHash: value.RequestHash, CandidateIndex: value.CandidateIndex,
		RequestedOutputCount: value.RequestedOutputCount, Status: value.Status,
		LocalFailureCode: value.LocalFailureCode, RemoteRequestID: value.RemoteRequestID,
		RemoteJobID: value.RemoteJobID, DispatchBoundaryEnteredAt: dispatchAt,
		QueryDeadlineAt: optionalProviderTimeHash(value.QueryDeadlineAt),
		RemoteExpiresAt: optionalProviderTimeHash(value.RemoteExpiresAt), Revision: value.Revision,
	})
}

func providerCallSetContentHash(
	calls []domain.ProviderCall,
	receipts []domain.ProviderResultReceipt,
) (string, error) {
	ordered := append([]domain.ProviderCall(nil), calls...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].CandidateIndex < ordered[right].CandidateIndex })
	receiptByCall := make(map[string]domain.ProviderResultReceipt, len(receipts))
	for _, receipt := range receipts {
		receiptByCall[receipt.CallID] = receipt
	}
	items := make([]providerCallSetHashItem, 0, len(ordered))
	for _, call := range ordered {
		receipt := receiptByCall[call.ID]
		items = append(items, providerCallSetHashItem{
			ID: call.ID, CallKey: call.CallKey, RequestHash: call.RequestHash, CallContentHash: call.ContentHash,
			ReceiptID: receipt.ID, ReceiptContentHash: receipt.ContentHash,
			CandidateIndex: call.CandidateIndex, RequestedOutputCount: call.RequestedOutputCount,
		})
	}
	return platformcommand.InputHash(items)
}

func providerResultReceiptContentHash(value domain.ProviderResultReceipt) (string, error) {
	return platformcommand.InputHash(providerReceiptHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, CallID: value.CallID,
		ProviderEventID: value.ProviderEventID, Status: value.Status, FailureCode: value.FailureCode,
		OutputCount: value.OutputCount, ProviderUsageObservation: value.ProviderUsageObservation,
		ProviderUsageHash: value.ProviderUsageHash, Output: cloneProviderOutput(value.Output),
		OccurredAt: value.OccurredAt.UTC().Format(time.RFC3339Nano),
	})
}

func optionalProviderTimeHash(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
