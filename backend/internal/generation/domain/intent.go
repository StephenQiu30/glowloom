package domain

import "time"

const (
	IntentPreparing      = "PREPARING"
	IntentPrepared       = "PREPARED"
	IntentClaimed        = "CLAIMED"
	IntentDispatching    = "DISPATCHING"
	IntentSubmitted      = "SUBMITTED"
	IntentOutcomeUnknown = "OUTCOME_UNKNOWN"
	IntentSucceeded      = "SUCCEEDED"
	IntentFailed         = "FAILED"
	IntentCancelled      = "CANCELLED"
)

type Intent struct {
	ID, WorkspaceID, ProjectID, WorkflowRunID, NodeRunID  string
	Metric, InputHash                                     string
	Units                                                 int64
	CostEstimateID, CostReservationID, QuotaReservationID string
	CostEstimateReceiptID, CostReservationReceiptID       string
	QuotaReservationReceiptID, CostReleaseReceiptID       string
	QuotaReleaseReceiptID, CostSettlementReceiptID        string
	QuotaConsumptionReceiptID                             string
	GenerationRequestID, ProviderJobID, ProviderReceiptID string
	Status, ContentHash, CreatedBy                        string
	InitiatorTokenVersion                                 int
	Claimant, ClaimToken                                  *string
	ClaimExpiresAt, CancelledAt                           *time.Time
	ClaimFencingVersion, Revision                         int64
	CreatedAt, UpdatedAt                                  time.Time
}

type ExecutionAuthorization struct {
	IntentID, ClaimToken, InputHash            string
	CostReservationID, QuotaReservationID      string
	ClaimFencingVersion, IntentRevision, Units int64
	ExpiresAt                                  time.Time
}

func SameIntentBinding(left, right Intent) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.WorkflowRunID == right.WorkflowRunID && left.NodeRunID == right.NodeRunID &&
		left.Metric == right.Metric && left.InputHash == right.InputHash && left.Units == right.Units &&
		left.CreatedBy == right.CreatedBy && left.InitiatorTokenVersion == right.InitiatorTokenVersion
}

func SameIntentState(left, right Intent) bool {
	return left.ID == right.ID && SameIntentBinding(left, right) &&
		left.CostEstimateID == right.CostEstimateID && left.CostReservationID == right.CostReservationID &&
		left.QuotaReservationID == right.QuotaReservationID &&
		left.CostEstimateReceiptID == right.CostEstimateReceiptID &&
		left.CostReservationReceiptID == right.CostReservationReceiptID &&
		left.QuotaReservationReceiptID == right.QuotaReservationReceiptID &&
		left.CostReleaseReceiptID == right.CostReleaseReceiptID &&
		left.QuotaReleaseReceiptID == right.QuotaReleaseReceiptID &&
		left.CostSettlementReceiptID == right.CostSettlementReceiptID &&
		left.QuotaConsumptionReceiptID == right.QuotaConsumptionReceiptID &&
		left.GenerationRequestID == right.GenerationRequestID && left.ProviderJobID == right.ProviderJobID &&
		left.ProviderReceiptID == right.ProviderReceiptID &&
		left.Status == right.Status && optionalIntentStringEqual(left.Claimant, right.Claimant) &&
		optionalIntentStringEqual(left.ClaimToken, right.ClaimToken) &&
		optionalIntentTimeEqual(left.ClaimExpiresAt, right.ClaimExpiresAt) &&
		optionalIntentTimeEqual(left.CancelledAt, right.CancelledAt) &&
		left.ClaimFencingVersion == right.ClaimFencingVersion && left.Revision == right.Revision &&
		left.ContentHash == right.ContentHash && left.CreatedAt.Equal(right.CreatedAt) && left.UpdatedAt.Equal(right.UpdatedAt)
}

func optionalIntentStringEqual(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func optionalIntentTimeEqual(left, right *time.Time) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && left.Equal(*right))
}
