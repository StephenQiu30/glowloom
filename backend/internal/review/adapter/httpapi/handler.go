package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	reviewdomain "github.com/StephenQiu30/lanverse/backend/internal/review/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type ReviewService interface {
	ListTasks(context.Context, reviewapp.Actor, reviewapp.ListTasksQuery) (reviewdomain.HumanTaskPage, error)
	GetTask(context.Context, reviewapp.Actor, string) (reviewdomain.HumanTaskDetail, error)
	Claim(context.Context, reviewapp.Actor, reviewapp.ClaimCommand) (reviewdomain.ClaimResult, error)
	Renew(context.Context, reviewapp.Actor, reviewapp.RenewCommand) (reviewdomain.ClaimResult, error)
	Release(context.Context, reviewapp.Actor, reviewapp.ReleaseCommand) (reviewdomain.HumanTask, error)
	Decide(context.Context, reviewapp.Actor, reviewapp.DecideCommand) (reviewdomain.DecisionResult, error)
}

type HumanGateCoordinator interface {
	GetHumanGate(context.Context, workflowapp.Actor, string) (workflowdomain.HumanGateCoordination, error)
	ResumeHumanGate(context.Context, workflowapp.Actor, string) (workflowdomain.HumanGateCoordination, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type Handler struct {
	reviews       ReviewService
	coordinator   HumanGateCoordinator
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

func New(reviews ReviewService, coordinator HumanGateCoordinator, authenticator Authenticator) *Handler {
	return &Handler{
		reviews: reviews, coordinator: coordinator, authenticator: authenticator,
		validator: platformvalidation.New(),
	}
}

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{project_id}/human-tasks", handler.list)
	mux.HandleFunc("GET /api/human-tasks/{human_task_id}", handler.get)
	mux.HandleFunc("POST /api/human-tasks/{human_task_id}/claims", handler.claim)
	mux.HandleFunc("POST /api/human-tasks/{human_task_id}/claim-renewals", handler.renew)
	mux.HandleFunc("POST /api/human-tasks/{human_task_id}/claim-releases", handler.release)
	mux.HandleFunc("POST /api/human-tasks/{human_task_id}/decisions", handler.decide)
	mux.HandleFunc("POST /api/review-decisions/{review_decision_id}/resume", handler.resume)
}

type claimRequest struct {
	ExpectedRevision int    `json:"expected_revision" validate:"gte=1"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}

type claimTokenRequest struct {
	ClaimToken       string `json:"claim_token" validate:"required,uuid"`
	ExpectedRevision int    `json:"expected_revision" validate:"gte=1"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}

type decisionRequest struct {
	ClaimToken              string  `json:"claim_token" validate:"required,uuid"`
	ExpectedTaskRevision    int     `json:"expected_task_revision" validate:"gte=1"`
	ExpectedSubjectRevision int     `json:"expected_subject_revision" validate:"gte=1"`
	ExpectedSubjectHash     string  `json:"expected_subject_hash" validate:"required,len=64,hexadecimal"`
	Decision                string  `json:"decision" validate:"required,oneof=approved rejected changes_requested selected"`
	SelectedCandidateID     *string `json:"selected_candidate_id"`
	IdempotencyKey          string  `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *Handler) list(writer http.ResponseWriter, request *http.Request) {
	reviewActor, _, ok := handler.actors(writer, request)
	if !ok {
		return
	}
	query, valid := parseListQuery(request)
	if !valid {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{
			Code: "validation_failed", Message: "Request validation failed", Status: http.StatusUnprocessableEntity,
		})
		return
	}
	query.ProjectID = request.PathValue("project_id")
	page, err := handler.reviews.ListTasks(request.Context(), reviewActor, query)
	if err != nil {
		handler.writeError(writer, request, err, nil)
		return
	}
	items := make([]map[string]any, len(page.Tasks))
	for index, task := range page.Tasks {
		items[index] = presentTask(task, false)
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{
		"items": items, "next_after": nullableString(page.NextAfter),
	}})
}

func (handler *Handler) get(writer http.ResponseWriter, request *http.Request) {
	reviewActor, workflowActor, ok := handler.actors(writer, request)
	if !ok {
		return
	}
	detail, err := handler.reviews.GetTask(request.Context(), reviewActor, request.PathValue("human_task_id"))
	if err != nil {
		handler.writeError(writer, request, err, nil)
		return
	}
	var coordination *workflowdomain.HumanGateCoordination
	if detail.Decision != nil {
		status, statusErr := handler.coordinator.GetHumanGate(request.Context(), workflowActor, detail.Decision.ID)
		if statusErr != nil {
			handler.writeError(writer, request, statusErr, nil)
			return
		}
		coordination = &status
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentDetail(detail, coordination)})
}

func (handler *Handler) claim(writer http.ResponseWriter, request *http.Request) {
	reviewActor, _, ok := handler.actors(writer, request)
	if !ok {
		return
	}
	var payload claimRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.reviews.Claim(request.Context(), reviewActor, reviewapp.ClaimCommand{
		TaskID: request.PathValue("human_task_id"), ExpectedRevision: payload.ExpectedRevision,
		IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err, nil)
		return
	}
	result.Task.ClaimToken = &result.ClaimToken
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"task": presentTask(result.Task, true)}})
}

func (handler *Handler) renew(writer http.ResponseWriter, request *http.Request) {
	reviewActor, _, ok := handler.actors(writer, request)
	if !ok {
		return
	}
	var payload claimTokenRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.reviews.Renew(request.Context(), reviewActor, reviewapp.RenewCommand{
		TaskID: request.PathValue("human_task_id"), ClaimToken: payload.ClaimToken,
		ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err, nil)
		return
	}
	result.Task.ClaimToken = &result.ClaimToken
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"task": presentTask(result.Task, true)}})
}

func (handler *Handler) release(writer http.ResponseWriter, request *http.Request) {
	reviewActor, _, ok := handler.actors(writer, request)
	if !ok {
		return
	}
	var payload claimTokenRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	task, err := handler.reviews.Release(request.Context(), reviewActor, reviewapp.ReleaseCommand{
		TaskID: request.PathValue("human_task_id"), ClaimToken: payload.ClaimToken,
		ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err, nil)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"task": presentTask(task, false)}})
}

func (handler *Handler) decide(writer http.ResponseWriter, request *http.Request) {
	reviewActor, workflowActor, ok := handler.actors(writer, request)
	if !ok {
		return
	}
	var payload decisionRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	if !validSelectedCandidate(payload) {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{
			Code: "validation_failed", Message: "Request validation failed", Status: http.StatusUnprocessableEntity,
		})
		return
	}
	selected := ""
	if payload.SelectedCandidateID != nil {
		selected = *payload.SelectedCandidateID
	}
	result, err := handler.reviews.Decide(request.Context(), reviewActor, reviewapp.DecideCommand{
		TaskID: request.PathValue("human_task_id"), ClaimToken: payload.ClaimToken,
		ExpectedTaskRevision: payload.ExpectedTaskRevision, ExpectedSubjectRevision: payload.ExpectedSubjectRevision,
		ExpectedSubjectHash: payload.ExpectedSubjectHash, Decision: payload.Decision,
		SelectedCandidateID: selected, IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err, nil)
		return
	}
	coordination, resumeErr := handler.coordinator.ResumeHumanGate(request.Context(), workflowActor, result.Decision.ID)
	if resumeErr != nil {
		handler.writeError(writer, request, resumeErr, &coordination)
		return
	}
	status := http.StatusAccepted
	if coordination.WorkflowResumeStatus == "completed" {
		status = http.StatusOK
	}
	platformhttp.WriteJSON(writer, status, map[string]any{"data": presentDecisionResult(result, coordination)})
}

func (handler *Handler) resume(writer http.ResponseWriter, request *http.Request) {
	_, workflowActor, ok := handler.actors(writer, request)
	if !ok {
		return
	}
	if !emptyBody(request) {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{
			Code: "validation_failed", Message: "Request validation failed", Status: http.StatusUnprocessableEntity,
		})
		return
	}
	coordination, err := handler.coordinator.ResumeHumanGate(
		request.Context(), workflowActor, request.PathValue("review_decision_id"),
	)
	if err != nil {
		handler.writeError(writer, request, err, &coordination)
		return
	}
	status := http.StatusAccepted
	if coordination.WorkflowResumeStatus == "completed" {
		status = http.StatusOK
	}
	platformhttp.WriteJSON(writer, status, map[string]any{"data": map[string]any{
		"coordination": presentCoordination(coordination),
	}})
}

func (handler *Handler) actors(
	writer http.ResponseWriter,
	request *http.Request,
) (reviewapp.Actor, workflowapp.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{
			Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized, NextAction: "login",
		})
		return reviewapp.Actor{}, workflowapp.Actor{}, false
	}
	return reviewapp.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion},
		workflowapp.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func (handler *Handler) writeError(
	writer http.ResponseWriter,
	request *http.Request,
	err error,
	coordination *workflowdomain.HumanGateCoordination,
) {
	problem := platformhttp.Problem{Code: "internal_error", Message: "Internal server error", Status: http.StatusInternalServerError}
	if errors.Is(err, reviewapp.ErrNotFound) || errors.Is(err, workflowapp.ErrNotFound) {
		problem = platformhttp.Problem{Code: "not_found", Message: "Review resource not found", Status: http.StatusNotFound}
	}
	var reviewError *reviewapp.Error
	if errors.As(err, &reviewError) {
		problem = platformhttp.Problem{Code: reviewError.Code, Message: reviewError.Message, Status: reviewError.Status}
	}
	var workflowError *workflowapp.Error
	if errors.As(err, &workflowError) {
		problem = platformhttp.Problem{
			Code: workflowError.Code, Message: workflowError.Message, Status: workflowError.Status,
			NextAction: workflowError.NextAction,
		}
	}
	if coordination != nil && coordination.ReviewDecisionID != "" {
		problem.Details = presentCoordination(*coordination)
	}
	platformhttp.WriteProblem(writer, request, problem)
}

func parseListQuery(request *http.Request) (reviewapp.ListTasksQuery, bool) {
	values := request.URL.Query()
	for key := range values {
		switch key {
		case "status", "subject_type", "limit", "after":
		default:
			return reviewapp.ListTasksQuery{}, false
		}
		if len(values[key]) != 1 {
			return reviewapp.ListTasksQuery{}, false
		}
	}
	limit := 50
	if raw := values.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return reviewapp.ListTasksQuery{}, false
		}
		limit = parsed
	}
	status := values.Get("status")
	if status == "" {
		status = "active"
	}
	return reviewapp.ListTasksQuery{
		Status: status, SubjectType: values.Get("subject_type"), Limit: limit, After: values.Get("after"),
	}, true
}

func validSelectedCandidate(payload decisionRequest) bool {
	if payload.Decision == "selected" {
		if payload.SelectedCandidateID == nil {
			return false
		}
		_, err := uuid.Parse(strings.TrimSpace(*payload.SelectedCandidateID))
		return err == nil
	}
	return payload.SelectedCandidateID == nil
}

func emptyBody(request *http.Request) bool {
	if request.Body == nil || request.Body == http.NoBody {
		return true
	}
	content, err := io.ReadAll(io.LimitReader(request.Body, 1))
	return err == nil && len(content) == 0
}

func presentDetail(detail reviewdomain.HumanTaskDetail, coordination *workflowdomain.HumanGateCoordination) map[string]any {
	result := map[string]any{"task": presentTask(detail.Task, detail.Task.ClaimToken != nil)}
	if detail.Decision == nil {
		result["decision"], result["coordination"] = nil, nil
		return result
	}
	result["decision"] = presentDecision(*detail.Decision)
	if coordination == nil {
		result["coordination"] = nil
	} else {
		result["coordination"] = presentCoordination(*coordination)
	}
	return result
}

func presentDecisionResult(result reviewdomain.DecisionResult, coordination workflowdomain.HumanGateCoordination) map[string]any {
	return map[string]any{
		"task": presentTask(result.Task, false), "decision": presentDecision(result.Decision),
		"coordination": presentCoordination(coordination),
	}
}

func presentTask(task reviewdomain.HumanTask, includeClaimToken bool) map[string]any {
	var claim any
	if task.ClaimedBy != nil {
		value := map[string]any{"claimed_by": *task.ClaimedBy, "expires_at": task.ClaimExpiresAt}
		if includeClaimToken && task.ClaimToken != nil {
			value["claim_token"] = *task.ClaimToken
		}
		claim = value
	}
	return map[string]any{
		"id": task.ID, "workspace_id": task.WorkspaceID, "project_id": task.ProjectID,
		"workflow_run_id": task.WorkflowRunID, "node_run_id": task.NodeRunID,
		"subject_type": task.SubjectType, "subject_id": task.SubjectID,
		"subject_revision": task.SubjectRevision, "subject_hash": task.SubjectHash,
		"candidate_ids": task.CandidateIDs, "rubric_version": task.RubricVersion,
		"allowed_decisions": task.AllowedDecisions, "status": task.Status, "claim": claim,
		"revision": task.Revision, "created_at": task.CreatedAt, "updated_at": task.UpdatedAt,
	}
}

func presentDecision(decision reviewdomain.ReviewDecision) map[string]any {
	return map[string]any{
		"id": decision.ID, "human_task_id": decision.HumanTaskID, "decision": decision.Decision,
		"subject_revision": decision.SubjectRevision, "subject_hash": decision.SubjectHash,
		"selected_candidate_id": nullableString(decision.SelectedCandidateID),
		"created_by":            decision.CreatedBy, "created_at": decision.CreatedAt,
	}
}

func presentCoordination(value workflowdomain.HumanGateCoordination) map[string]any {
	return map[string]any{
		"review_decision_id": value.ReviewDecisionID, "decision_status": value.DecisionStatus,
		"owner_apply_status": value.OwnerApplyStatus, "owner_receipt_id": nullableText(value.OwnerReceiptID),
		"workflow_resume_status":     value.WorkflowResumeStatus,
		"workflow_signal_receipt_id": nullableText(value.SignalReceiptID),
		"conflict_code":              nullableText(value.ConflictCode),
	}
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

var _ ReviewService = (*reviewapp.Service)(nil)
var _ HumanGateCoordinator = (*workflowapp.HumanGateCoordinator)(nil)
