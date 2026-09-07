package application

import (
	"errors"
	"strings"
	"time"

	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

func providerTerminalOutcomeMatchesReceipt(
	receipt domain.ProviderResultReceipt,
	outcome ProviderOutcome,
	occurredAtProvided bool,
) bool {
	expectedStatus, expectedCount := domain.ProviderResultSucceeded, 1
	if outcome.Status == ProviderOutcomeFailed {
		expectedStatus, expectedCount = domain.ProviderResultFailed, 0
	}
	if receipt.Status != expectedStatus || receipt.OutputCount != expectedCount ||
		receipt.ProviderEventID != outcome.ProviderEventID || receipt.FailureCode != outcome.FailureCode ||
		receipt.ProviderUsageObservation != outcome.ProviderUsageObservation ||
		!optionalProviderOutputEqual(receipt.Output, outcome.Output) {
		return false
	}
	return !occurredAtProvided || receipt.OccurredAt.Equal(outcome.OccurredAt)
}

func optionalProviderOutputEqual(left, right *ProviderOutput) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func providerOutcomeConflict() error {
	return &Error{
		Code: "provider_outcome_conflict", Message: "Generation Provider returned a conflicting terminal outcome",
		Status: 409, NextAction: "manual_provider_reconciliation",
	}
}

func providerQueryTemporarilyUnavailable() error {
	return &Error{
		Code: "provider_query_temporarily_unavailable", Message: "Generation Provider query is temporarily unavailable",
		Status: 503, NextAction: "retry_provider_query",
	}
}

func providerQueryFailureKind(err error) string {
	var typed ProviderQueryFailure
	if errors.As(err, &typed) {
		kind := strings.TrimSpace(typed.ProviderQueryFailureKind())
		if kind == ProviderQueryFailureIdentityUnrecoverable || kind == ProviderQueryFailureRetryable {
			return kind
		}
	}
	return ProviderQueryFailureRetryable
}

func providerSubmitFailureKind(err error) string {
	var typed ProviderSubmitFailure
	if errors.As(err, &typed) &&
		strings.TrimSpace(typed.ProviderSubmitFailureKind()) == ProviderSubmitFailureIdentityRecoverable {
		return ProviderSubmitFailureIdentityRecoverable
	}
	return ""
}

func validateAuthorizationBinding(intent domain.Intent, authorization domain.ExecutionAuthorization, _ bool) error {
	if validateIntent(intent) != nil || !validUUID(authorization.IntentID) || !validUUID(authorization.ClaimToken) ||
		!validUUID(authorization.TargetID) || !intentHashPattern.MatchString(authorization.TargetHash) ||
		intent.ID != authorization.IntentID || intent.ClaimToken == nil || *intent.ClaimToken != authorization.ClaimToken ||
		intent.TargetID != authorization.TargetID || intent.TargetHash != authorization.TargetHash ||
		intent.BindingVersionID != authorization.BindingVersionID || intent.BindingRevision != authorization.BindingRevision ||
		intent.BindingContentHash != authorization.BindingContentHash ||
		intent.ConnectionVersionID != authorization.ConnectionVersionID ||
		intent.CredentialVersionID != authorization.CredentialVersionID ||
		intent.ModelProfileVersionID != authorization.ModelProfileVersionID ||
		intent.ModelProfileRevision != authorization.ModelProfileRevision ||
		intent.ModelProfileContentHash != authorization.ModelProfileContentHash ||
		intent.PriceQuoteID != authorization.PriceQuoteID || intent.PriceQuoteRevision != authorization.PriceQuoteRevision ||
		intent.PriceQuoteContentHash != authorization.PriceQuoteContentHash || intent.BillingMetric != authorization.BillingMetric ||
		intent.CostReservationID != authorization.CostReservationID ||
		intent.QuotaReservationID != authorization.QuotaReservationID ||
		intent.ClaimFencingVersion != authorization.ClaimFencingVersion || authorization.IntentRevision != 3 ||
		intent.EstimatedUnits != authorization.EstimatedUnits || intent.ClaimExpiresAt == nil ||
		!intent.ClaimExpiresAt.Equal(authorization.ExpiresAt) {
		return conflict("Generation Provider authorization has drifted")
	}
	return nil
}

func validateIntentTargetBinding(target domain.GenerationTarget, intent domain.Intent) error {
	if domain.ValidateGenerationTarget(target) != nil || target.ID != intent.TargetID || target.WorkspaceID != intent.WorkspaceID ||
		target.ProjectID != intent.ProjectID || target.TargetHash != intent.TargetHash || target.CreatedBy != intent.CreatedBy {
		return conflict("GenerationTarget and intent have drifted")
	}
	return nil
}

func validateProviderTargetBinding(
	target domain.GenerationTarget,
	intent domain.Intent,
	request domain.GenerationRequest,
) error {
	if err := validateIntentTargetBinding(target, intent); err != nil || request.TargetID != target.ID ||
		request.TargetHash != target.TargetHash || request.WorkspaceID != target.WorkspaceID ||
		request.ProjectID != target.ProjectID || request.CreatedBy != target.CreatedBy ||
		request.EstimatedUnits != intent.EstimatedUnits {
		return conflict("GenerationTarget, intent and request have drifted")
	}
	return nil
}

func validateProjectProviderBinding(value domain.ProjectProviderBindingVersion) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validProviderPurpose(value.Purpose) || !providerIdentifierPattern.MatchString(value.ProviderKey) ||
		(value.Modality != domain.MediaModalityImage && value.Modality != domain.MediaModalityVideo) ||
		!validUUID(value.ConnectionVersionID) || !validUUID(value.CredentialVersionID) ||
		!validUUID(value.ModelProfileVersionID) || !providerIdentifierPattern.MatchString(value.AdapterContractVersion) ||
		value.Revision < 1 || !validUUID(value.CreatedBy) || value.CreatedAt.IsZero() {
		return conflict("Generation Provider binding facts have drifted")
	}
	hash, err := projectProviderBindingContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Generation Provider binding facts have drifted")
	}
	return nil
}

func validateGenerationRequest(value domain.GenerationRequest) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.IntentID) || !validUUID(value.TargetID) || !validUUID(value.BindingID) || value.BindingRevision < 1 ||
		!intentHashPattern.MatchString(value.BindingContentHash) || !validProviderPurpose(value.Purpose) ||
		!providerIdentifierPattern.MatchString(value.ProviderKey) || !providerIdentifierPattern.MatchString(value.ExternalModelID) ||
		!validUUID(value.ConnectionVersionID) || !validUUID(value.CredentialVersionID) ||
		!validUUID(value.ModelProfileVersionID) || value.ModelProfileRevision < 1 ||
		!intentHashPattern.MatchString(value.ModelProfileContentHash) || !validUUID(value.PriceQuoteID) ||
		value.PriceQuoteRevision < 1 || !intentHashPattern.MatchString(value.PriceQuoteContentHash) ||
		!costdomain.IsBillingMetric(value.BillingMetric) || value.RequestKey != "generation-request:"+value.ID ||
		!intentHashPattern.MatchString(value.TargetHash) || value.EstimatedUnits < 1 ||
		!validUUID(value.CreatedBy) || value.CreatedAt.IsZero() {
		return conflict("Generation request facts have drifted")
	}
	hash, err := generationRequestContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Generation request facts have drifted")
	}
	return nil
}

func validateProviderJob(value domain.ProviderJob) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validUUID(value.IntentID) || !validUUID(value.RequestID) || !providerIdentifierPattern.MatchString(value.ProviderKey) ||
		value.RequestKey == "" || value.CallCount < 1 || value.DispatchedCallCount < 0 ||
		value.SucceededCallCount < 0 || value.FailedCallCount < 0 ||
		value.DispatchedCallCount > value.CallCount || value.SucceededCallCount+value.FailedCallCount > value.CallCount ||
		!intentHashPattern.MatchString(value.CallSetHash) || value.Revision < 1 || value.CreatedAt.IsZero() ||
		value.UpdatedAt.Before(value.CreatedAt) {
		return conflict("Generation Provider job facts have drifted")
	}
	switch value.Status {
	case domain.ProviderJobPending:
		if value.DispatchedCallCount != 0 || value.SucceededCallCount != 0 || value.FailedCallCount != 0 {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobRunning:
		if value.SucceededCallCount+value.FailedCallCount >= value.CallCount {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobOutcomeUnknown:
		if value.DispatchedCallCount == 0 || value.SucceededCallCount+value.FailedCallCount >= value.CallCount {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobSucceeded:
		if value.SucceededCallCount != value.CallCount || value.FailedCallCount != 0 {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobPartialSucceeded:
		if value.SucceededCallCount == 0 || value.FailedCallCount == 0 ||
			value.SucceededCallCount+value.FailedCallCount != value.CallCount {
			return conflict("Generation Provider job facts have drifted")
		}
	case domain.ProviderJobFailed:
		if value.SucceededCallCount != 0 || value.FailedCallCount != value.CallCount {
			return conflict("Generation Provider job facts have drifted")
		}
	default:
		return conflict("Generation Provider job facts have drifted")
	}
	hash, err := providerJobContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Generation Provider job facts have drifted")
	}
	return nil
}

func validateProviderCalls(calls []domain.ProviderCall, request domain.GenerationRequest, job domain.ProviderJob) error {
	if len(calls) != job.CallCount {
		return conflict("Generation Provider Call set has drifted")
	}
	for index, call := range calls {
		if !validUUID(call.ID) || call.WorkspaceID != job.WorkspaceID || call.ProjectID != job.ProjectID ||
			call.JobID != job.ID || call.CandidateIndex != index+1 || call.CallKey != "generation-call:"+call.ID ||
			!intentHashPattern.MatchString(call.RequestHash) || call.RequestedOutputCount != 1 || call.Revision < 1 ||
			call.CreatedAt.IsZero() || call.UpdatedAt.Before(call.CreatedAt) || len(call.RemoteRequestID) > 180 ||
			len(call.RemoteJobID) > 180 || (call.LocalFailureCode != "" && !providerFailurePattern.MatchString(call.LocalFailureCode)) {
			return conflict("Generation Provider Call facts have drifted")
		}
		if (call.QueryDeadlineAt == nil) != (call.RemoteExpiresAt == nil) {
			return conflict("Generation Provider Call retention window has drifted")
		}
		if call.QueryDeadlineAt != nil && (call.DispatchBoundaryEnteredAt == nil ||
			!call.QueryDeadlineAt.After(*call.DispatchBoundaryEnteredAt) ||
			!call.RemoteExpiresAt.After(*call.QueryDeadlineAt)) {
			return conflict("Generation Provider Call retention window has drifted")
		}
		expectedRequestHash, err := providerCallRequestHash(request, call.CandidateIndex)
		if err != nil || expectedRequestHash != call.RequestHash {
			return conflict("Generation Provider Call request has drifted")
		}
		switch call.Status {
		case domain.ProviderCallPending:
			if call.DispatchBoundaryEnteredAt != nil || call.LocalFailureCode != "" || call.RemoteRequestID != "" || call.RemoteJobID != "" {
				return conflict("Generation Provider Call facts have drifted")
			}
			if call.QueryDeadlineAt != nil || call.RemoteExpiresAt != nil {
				return conflict("Generation Provider Call retention window has drifted")
			}
		case domain.ProviderCallDispatching:
			if call.DispatchBoundaryEnteredAt == nil || call.LocalFailureCode != "" || call.RemoteRequestID != "" || call.RemoteJobID != "" {
				return conflict("Generation Provider Call facts have drifted")
			}
			if call.QueryDeadlineAt != nil || call.RemoteExpiresAt != nil {
				return conflict("Generation Provider Call retention window has drifted")
			}
		case domain.ProviderCallSubmitted, domain.ProviderCallRunning:
			if call.DispatchBoundaryEnteredAt == nil || call.LocalFailureCode != "" ||
				(call.RemoteRequestID == "" && call.RemoteJobID == "") || call.QueryDeadlineAt == nil ||
				call.RemoteExpiresAt == nil {
				return conflict("Generation Provider Call facts have drifted")
			}
		case domain.ProviderCallSucceeded, domain.ProviderCallOutcomeUnknown:
			if call.DispatchBoundaryEnteredAt == nil || call.LocalFailureCode != "" {
				return conflict("Generation Provider Call facts have drifted")
			}
		case domain.ProviderCallFailed:
			localFailure := call.DispatchBoundaryEnteredAt == nil && call.LocalFailureCode != "" &&
				call.RemoteRequestID == "" && call.RemoteJobID == "" && call.QueryDeadlineAt == nil &&
				call.RemoteExpiresAt == nil
			remoteFailure := call.DispatchBoundaryEnteredAt != nil && call.LocalFailureCode == ""
			if !localFailure && !remoteFailure {
				return conflict("Generation Provider Call facts have drifted")
			}
		default:
			return conflict("Generation Provider Call facts have drifted")
		}
		hash, err := providerCallContentHash(call)
		if err != nil || hash != call.ContentHash {
			return conflict("Generation Provider Call facts have drifted")
		}
	}
	return nil
}

func validateProviderResultReceipt(value domain.ProviderResultReceipt, call domain.ProviderCall) error {
	if !validUUID(value.ID) || value.WorkspaceID != call.WorkspaceID || value.ProjectID != call.ProjectID ||
		value.CallID != call.ID || len(value.ProviderEventID) > 180 || value.OccurredAt.IsZero() ||
		value.ReceivedAt.Before(value.OccurredAt) || !intentHashPattern.MatchString(value.ProviderUsageHash) ||
		!validProviderUsage(value.ProviderUsageObservation) {
		return conflict("Generation Provider result receipt has drifted")
	}
	usageHash, err := platformcommand.InputHash(value.ProviderUsageObservation)
	if err != nil || usageHash != value.ProviderUsageHash {
		return conflict("Generation Provider usage observation has drifted")
	}
	switch value.Status {
	case domain.ProviderResultSucceeded:
		if value.OutputCount != 1 || value.Output == nil || value.FailureCode != "" ||
			!validProviderOutput(*value.Output) || !providerOutputUsesCallStaging(*value.Output, call.WorkspaceID, call.JobID, call.ID) {
			return conflict("Generation Provider result receipt has drifted")
		}
	case domain.ProviderResultFailed:
		if value.OutputCount != 0 || value.Output != nil || !providerFailurePattern.MatchString(value.FailureCode) {
			return conflict("Generation Provider result receipt has drifted")
		}
	default:
		return conflict("Generation Provider result receipt has drifted")
	}
	hash, err := providerResultReceiptContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Generation Provider result receipt has drifted")
	}
	return nil
}

func validateProviderReceiptSet(calls []domain.ProviderCall, receipts []domain.ProviderResultReceipt) error {
	byCall := make(map[string]domain.ProviderResultReceipt, len(receipts))
	for _, receipt := range receipts {
		if _, exists := byCall[receipt.CallID]; exists {
			return conflict("Generation Provider result receipt set contains duplicate Calls")
		}
		byCall[receipt.CallID] = receipt
	}
	for _, call := range calls {
		receipt, exists := byCall[call.ID]
		if call.Status == domain.ProviderCallSucceeded ||
			(call.Status == domain.ProviderCallFailed && call.DispatchBoundaryEnteredAt != nil) {
			if !exists || validateProviderResultReceipt(receipt, call) != nil ||
				(call.Status == domain.ProviderCallSucceeded) != (receipt.Status == domain.ProviderResultSucceeded) {
				return conflict("Generation Provider terminal Call receipt has drifted")
			}
			delete(byCall, call.ID)
		} else if exists {
			return conflict("Generation Provider Call has an unexpected receipt")
		}
	}
	if len(byCall) != 0 {
		return conflict("Generation Provider receipt set has unknown Calls")
	}
	return nil
}

func normalizedProviderOutcome(value ProviderOutcome, now time.Time) ProviderOutcome {
	value.Status = strings.TrimSpace(value.Status)
	value.RemoteRequestID = strings.TrimSpace(value.RemoteRequestID)
	value.RemoteJobID = strings.TrimSpace(value.RemoteJobID)
	value.ProviderEventID = strings.TrimSpace(value.ProviderEventID)
	value.FailureCode = strings.TrimSpace(value.FailureCode)
	if value.OccurredAt.IsZero() {
		value.OccurredAt = now
	} else {
		value.OccurredAt = value.OccurredAt.UTC().Truncate(time.Microsecond)
	}
	validRemote := len(value.RemoteRequestID) <= 180 && len(value.RemoteJobID) <= 180
	validEvent := len(value.ProviderEventID) <= 180
	validUsage := validProviderUsage(value.ProviderUsageObservation)
	if !value.QueryDeadlineAt.IsZero() {
		value.QueryDeadlineAt = value.QueryDeadlineAt.UTC().Truncate(time.Microsecond)
	}
	if !value.RemoteExpiresAt.IsZero() {
		value.RemoteExpiresAt = value.RemoteExpiresAt.UTC().Truncate(time.Microsecond)
	}
	if value.OccurredAt.After(now) || !validRemote || !validEvent || !validUsage {
		return ProviderOutcome{Status: ProviderOutcomeUnknown, OccurredAt: now}
	}
	switch value.Status {
	case ProviderOutcomeAccepted, ProviderOutcomeRunning:
		validRetentionWindow := value.QueryDeadlineAt.After(now) &&
			value.RemoteExpiresAt.After(value.QueryDeadlineAt) &&
			!value.RemoteExpiresAt.After(now.Add(30*24*time.Hour))
		if (value.RemoteRequestID != "" || value.RemoteJobID != "") && !value.QueryDeadlineAt.IsZero() &&
			!value.RemoteExpiresAt.IsZero() && validRetentionWindow && value.FailureCode == "" && value.Output == nil {
			return value
		}
	case ProviderOutcomeSucceeded:
		if value.FailureCode == "" && value.Output != nil && validProviderOutput(*value.Output) {
			value.Output = cloneProviderOutput(value.Output)
			return value
		}
	case ProviderOutcomeFailed:
		if providerFailurePattern.MatchString(value.FailureCode) && value.Output == nil {
			return value
		}
	case ProviderOutcomeUnknown:
		if value.FailureCode == "" && value.Output == nil {
			return ProviderOutcome{
				Status: ProviderOutcomeUnknown, RemoteRequestID: value.RemoteRequestID, RemoteJobID: value.RemoteJobID,
				OccurredAt: value.OccurredAt, QueryDeadlineAt: value.QueryDeadlineAt,
				RemoteExpiresAt: value.RemoteExpiresAt,
			}
		}
	}
	return ProviderOutcome{Status: ProviderOutcomeUnknown, OccurredAt: now}
}

func validProviderUsage(value domain.ProviderUsageObservation) bool {
	const maximum = int64(1_000_000_000_000)
	return value.InputTokens >= 0 && value.InputTokens <= maximum &&
		value.OutputTokens >= 0 && value.OutputTokens <= maximum && value.TotalTokens >= 0 && value.TotalTokens <= maximum &&
		value.ImageCount >= 0 && value.ImageCount <= maximum && value.VideoDurationMS >= 0 && value.VideoDurationMS <= maximum
}

func validProviderOutput(output ProviderOutput) bool {
	return providerOutputKeyPattern.MatchString(output.OutputKey) && output.StagingObjectKey != "" &&
		len(output.StagingObjectKey) <= 512 && intentHashPattern.MatchString(output.SHA256) && output.Bytes > 0 &&
		(output.MediaType == "image/png" || output.MediaType == "image/jpeg") && output.Width > 0 && output.Height > 0
}

func providerOutputUsesCallStaging(output ProviderOutput, workspaceID, providerJobID, providerCallID string) bool {
	prefix := "staging/" + workspaceID + "/" + providerJobID + "/" + providerCallID + "/"
	return strings.HasPrefix(output.StagingObjectKey, prefix) && !strings.Contains(output.StagingObjectKey, "..") &&
		!strings.HasSuffix(output.StagingObjectKey, "/")
}

func providerJobTerminal(status string) bool {
	return status == domain.ProviderJobSucceeded || status == domain.ProviderJobPartialSucceeded ||
		status == domain.ProviderJobFailed
}

func providerSubmission(
	request domain.GenerationRequest,
	job domain.ProviderJob,
	call domain.ProviderCall,
	target domain.GenerationTarget,
) ProviderSubmission {
	return ProviderSubmission{
		WorkspaceID: request.WorkspaceID, ProjectID: request.ProjectID, ProviderJobID: job.ID,
		ProviderCallID: call.ID, CallKey: call.CallKey, CallRequestHash: call.RequestHash, CandidateIndex: call.CandidateIndex,
		RequestedOutputCount: call.RequestedOutputCount, RequestID: request.ID, RequestKey: request.RequestKey,
		IntentID: request.IntentID, ProviderKey: request.ProviderKey, ExternalModelID: request.ExternalModelID,
		ConnectionVersionID: request.ConnectionVersionID, CredentialVersionID: request.CredentialVersionID,
		BindingID: request.BindingID, BindingRevision: request.BindingRevision, BindingContentHash: request.BindingContentHash,
		ModelProfileVersionID: request.ModelProfileVersionID, ModelProfileRevision: request.ModelProfileRevision,
		ModelProfileContentHash: request.ModelProfileContentHash, PriceQuoteID: request.PriceQuoteID,
		PriceQuoteRevision: request.PriceQuoteRevision, PriceQuoteContentHash: request.PriceQuoteContentHash,
		BillingMetric: request.BillingMetric, EstimatedUnits: request.EstimatedUnits,
		RemoteRequestID: call.RemoteRequestID, RemoteJobID: call.RemoteJobID,
		QueryDeadlineAt: cloneProviderTime(call.QueryDeadlineAt),
		RemoteExpiresAt: cloneProviderTime(call.RemoteExpiresAt),
		TargetHash:      request.TargetHash, Target: target,
	}
}

func providerLocalFailureCode(err error) string {
	var typed ProviderLocalFailure
	if errors.As(err, &typed) {
		value := strings.TrimSpace(typed.ProviderFailureCode())
		if providerFailurePattern.MatchString(value) {
			return value
		}
	}
	return providerPreflightFailed
}

func remoteBindingDrifted(call domain.ProviderCall, outcome ProviderOutcome) bool {
	return (call.RemoteRequestID != "" && outcome.RemoteRequestID != "" && call.RemoteRequestID != outcome.RemoteRequestID) ||
		(call.RemoteJobID != "" && outcome.RemoteJobID != "" && call.RemoteJobID != outcome.RemoteJobID) ||
		(call.QueryDeadlineAt != nil && !outcome.QueryDeadlineAt.IsZero() &&
			!call.QueryDeadlineAt.Equal(outcome.QueryDeadlineAt)) ||
		(call.RemoteExpiresAt != nil && !outcome.RemoteExpiresAt.IsZero() &&
			!call.RemoteExpiresAt.Equal(outcome.RemoteExpiresAt))
}

func canonicalProviderOutcomeBinding(outcome ProviderOutcome) ProviderOutcome {
	outcome.RemoteRequestID = strings.TrimSpace(outcome.RemoteRequestID)
	outcome.RemoteJobID = strings.TrimSpace(outcome.RemoteJobID)
	if !outcome.QueryDeadlineAt.IsZero() {
		outcome.QueryDeadlineAt = outcome.QueryDeadlineAt.UTC().Truncate(time.Microsecond)
	}
	if !outcome.RemoteExpiresAt.IsZero() {
		outcome.RemoteExpiresAt = outcome.RemoteExpiresAt.UTC().Truncate(time.Microsecond)
	}
	return ProviderOutcome{
		RemoteRequestID: outcome.RemoteRequestID,
		RemoteJobID:     outcome.RemoteJobID,
		QueryDeadlineAt: outcome.QueryDeadlineAt,
		RemoteExpiresAt: outcome.RemoteExpiresAt,
	}
}

func invalidProviderOutcomeBinding(call domain.ProviderCall, outcome ProviderOutcome, now time.Time) bool {
	if len(outcome.RemoteRequestID) > 180 || len(outcome.RemoteJobID) > 180 {
		return true
	}
	if outcome.QueryDeadlineAt.IsZero() != outcome.RemoteExpiresAt.IsZero() {
		return true
	}
	if outcome.QueryDeadlineAt.IsZero() {
		return false
	}
	return call.DispatchBoundaryEnteredAt == nil ||
		!outcome.QueryDeadlineAt.After(*call.DispatchBoundaryEnteredAt) ||
		!outcome.RemoteExpiresAt.After(outcome.QueryDeadlineAt) ||
		outcome.RemoteExpiresAt.After(now.Add(30*24*time.Hour))
}

func firstNonEmpty(first, second string) string {
	if first != "" {
		return first
	}
	return second
}

func replaceProviderCall(calls []domain.ProviderCall, replacement domain.ProviderCall) []domain.ProviderCall {
	result := append([]domain.ProviderCall(nil), calls...)
	for index := range result {
		if result[index].ID == replacement.ID {
			result[index] = replacement
			return result
		}
	}
	return result
}

func cloneProviderOutput(value *ProviderOutput) *ProviderOutput {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneProviderTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC().Truncate(time.Microsecond)
	return &cloned
}
