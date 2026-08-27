package review_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	reviewhttp "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/httpapi"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	reviewdomain "github.com/StephenQiu30/lanverse/backend/internal/review/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	publicTaskID      = "00000000-0000-0000-0000-000000000101"
	publicProjectID   = "00000000-0000-0000-0000-000000000102"
	publicDecisionID  = "00000000-0000-0000-0000-000000000103"
	publicClaimToken  = "00000000-0000-0000-0000-000000000104"
	publicSubjectHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestPublicHumanTaskHandlerListsWithoutClaimTokenAndReturnsOwnerTokenOnDetail(t *testing.T) {
	now := time.Date(2026, time.August, 27, 11, 0, 0, 0, time.UTC)
	claimedBy := "00000000-0000-0000-0000-000000000105"
	expiresAt := now.Add(5 * time.Minute)
	task := reviewdomain.HumanTask{
		ID: publicTaskID, ProjectID: publicProjectID, Status: "CLAIMED", Revision: 2,
		SubjectType: "workflow_node_output", SubjectID: publicTaskID, SubjectRevision: 1,
		SubjectHash: publicSubjectHash, AllowedDecisions: []string{"approved", "changes_requested", "rejected"},
		ClaimedBy: &claimedBy, ClaimToken: stringPointer(publicClaimToken), ClaimExpiresAt: &expiresAt,
		CreatedAt: now, UpdatedAt: now,
	}
	reviews := &publicReviewStub{page: reviewdomain.HumanTaskPage{Tasks: []reviewdomain.HumanTask{task}}, detail: reviewdomain.HumanTaskDetail{Task: task}}
	coordinator := &publicCoordinatorStub{}
	mux := http.NewServeMux()
	reviewhttp.New(reviews, coordinator, publicAuthenticator{userID: claimedBy}).Register(mux)

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+publicProjectID+"/human-tasks?status=active&limit=20", nil))
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), publicClaimToken) ||
		!strings.Contains(list.Body.String(), `"subject_hash":"`+publicSubjectHash+`"`) {
		t.Fatalf("list response=%d %s", list.Code, list.Body.String())
	}

	detail := httptest.NewRecorder()
	mux.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/api/v1/human-tasks/"+publicTaskID, nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), publicClaimToken) {
		t.Fatalf("detail response=%d %s", detail.Code, detail.Body.String())
	}
}

func TestPublicHumanTaskHandlerMapsLeaseDecisionAndResumeWithoutClientOwnedGateFacts(t *testing.T) {
	now := time.Date(2026, time.August, 27, 11, 30, 0, 0, time.UTC)
	task := reviewdomain.HumanTask{
		ID: publicTaskID, ProjectID: publicProjectID, Status: "CLAIMED", Revision: 2,
		SubjectType: "workflow_node_output", SubjectID: publicTaskID, SubjectRevision: 1,
		SubjectHash: publicSubjectHash, AllowedDecisions: []string{"approved", "changes_requested", "rejected"},
		ClaimedBy:  stringPointer("00000000-0000-0000-0000-000000000105"),
		ClaimToken: stringPointer(publicClaimToken), CreatedAt: now, UpdatedAt: now,
	}
	decision := reviewdomain.ReviewDecision{
		ID: publicDecisionID, HumanTaskID: publicTaskID, Decision: "approved",
		SubjectRevision: 1, SubjectHash: publicSubjectHash, CreatedAt: now,
	}
	reviews := &publicReviewStub{
		claim:    reviewdomain.ClaimResult{Task: task, ClaimToken: publicClaimToken},
		decision: reviewdomain.DecisionResult{Task: task, Decision: decision},
		detail:   reviewdomain.HumanTaskDetail{Task: task, Decision: &decision},
	}
	coordinator := &publicCoordinatorStub{coordination: workflowdomain.HumanGateCoordination{
		ReviewDecisionID: publicDecisionID, DecisionStatus: "recorded",
		OwnerApplyStatus: "completed", WorkflowResumeStatus: "unknown",
	}}
	mux := http.NewServeMux()
	reviewhttp.New(reviews, coordinator, publicAuthenticator{userID: "00000000-0000-0000-0000-000000000105"}).Register(mux)

	claim := httptest.NewRecorder()
	mux.ServeHTTP(claim, httptest.NewRequest(http.MethodPost, "/api/v1/human-tasks/"+publicTaskID+"/claims",
		strings.NewReader(`{"expected_revision":1,"idempotency_key":"claim-1"}`)))
	if claim.Code != http.StatusOK || reviews.claimCommand.TaskID != publicTaskID ||
		!strings.Contains(claim.Body.String(), publicClaimToken) {
		t.Fatalf("claim response=%d %s command=%#v", claim.Code, claim.Body.String(), reviews.claimCommand)
	}

	decide := httptest.NewRecorder()
	mux.ServeHTTP(decide, httptest.NewRequest(http.MethodPost, "/api/v1/human-tasks/"+publicTaskID+"/decisions",
		strings.NewReader(`{"claim_token":"`+publicClaimToken+`","expected_task_revision":2,`+
			`"expected_subject_revision":1,"expected_subject_hash":"`+publicSubjectHash+`",`+
			`"decision":"approved","selected_candidate_id":null,"idempotency_key":"decision-1"}`)))
	if decide.Code != http.StatusAccepted || reviews.decideCommand.ExpectedSubjectHash != publicSubjectHash ||
		coordinator.resumeDecisionID != publicDecisionID ||
		!strings.Contains(decide.Body.String(), `"workflow_resume_status":"unknown"`) {
		t.Fatalf("decision response=%d %s command=%#v coordinator=%#v", decide.Code, decide.Body.String(), reviews.decideCommand, coordinator)
	}

	resume := httptest.NewRecorder()
	mux.ServeHTTP(resume, httptest.NewRequest(http.MethodPost,
		"/api/v1/review-decisions/"+publicDecisionID+"/resume", nil))
	if resume.Code != http.StatusAccepted || coordinator.resumeCalls != 2 {
		t.Fatalf("resume response=%d %s calls=%d", resume.Code, resume.Body.String(), coordinator.resumeCalls)
	}
}

func TestPublicHumanTaskHandlerRejectsMalformedSelectedCandidateOnce(t *testing.T) {
	reviews := &publicReviewStub{}
	mux := http.NewServeMux()
	reviewhttp.New(reviews, &publicCoordinatorStub{}, publicAuthenticator{
		userID: "00000000-0000-0000-0000-000000000105",
	}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/human-tasks/"+publicTaskID+"/decisions",
		strings.NewReader(`{"claim_token":"`+publicClaimToken+`","expected_task_revision":2,`+
			`"expected_subject_revision":1,"expected_subject_hash":"`+publicSubjectHash+`",`+
			`"decision":"selected","selected_candidate_id":"not-a-uuid","idempotency_key":"invalid-selected"}`)))
	if response.Code != http.StatusUnprocessableEntity || reviews.decideCommand.TaskID != "" ||
		strings.Count(response.Body.String(), `"code":"validation_failed"`) != 1 {
		t.Fatalf("malformed selected candidate response=%d %s command=%#v", response.Code, response.Body.String(), reviews.decideCommand)
	}
}

func TestPublicHumanTaskHandlerCarriesCommittedDecisionWhenOwnerApplyConflicts(t *testing.T) {
	decision := reviewdomain.ReviewDecision{ID: publicDecisionID, HumanTaskID: publicTaskID, Decision: "approved", SubjectRevision: 1, SubjectHash: publicSubjectHash}
	reviews := &publicReviewStub{decision: reviewdomain.DecisionResult{
		Task: reviewdomain.HumanTask{ID: publicTaskID}, Decision: decision,
	}}
	coordinator := &publicCoordinatorStub{
		coordination: workflowdomain.HumanGateCoordination{
			ReviewDecisionID: publicDecisionID, DecisionStatus: "recorded",
			OwnerApplyStatus: "conflict", WorkflowResumeStatus: "pending",
		},
		err: &workflowapp.Error{Code: "resource_conflict", Message: "Owner baseline changed", Status: http.StatusConflict},
	}
	mux := http.NewServeMux()
	reviewhttp.New(reviews, coordinator, publicAuthenticator{userID: "00000000-0000-0000-0000-000000000105"}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/human-tasks/"+publicTaskID+"/decisions",
		strings.NewReader(`{"claim_token":"`+publicClaimToken+`","expected_task_revision":2,`+
			`"expected_subject_revision":1,"expected_subject_hash":"`+publicSubjectHash+`",`+
			`"decision":"approved","selected_candidate_id":null,"idempotency_key":"decision-conflict"}`)))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), publicDecisionID) ||
		!strings.Contains(response.Body.String(), `"owner_apply_status":"conflict"`) {
		t.Fatalf("conflict response=%d %s", response.Code, response.Body.String())
	}
}

type publicReviewStub struct {
	page          reviewdomain.HumanTaskPage
	detail        reviewdomain.HumanTaskDetail
	claim         reviewdomain.ClaimResult
	decision      reviewdomain.DecisionResult
	claimCommand  reviewapp.ClaimCommand
	decideCommand reviewapp.DecideCommand
}

func (stub *publicReviewStub) ListTasks(context.Context, reviewapp.Actor, reviewapp.ListTasksQuery) (reviewdomain.HumanTaskPage, error) {
	return stub.page, nil
}

func (stub *publicReviewStub) GetTask(context.Context, reviewapp.Actor, string) (reviewdomain.HumanTaskDetail, error) {
	return stub.detail, nil
}

func (stub *publicReviewStub) Claim(_ context.Context, _ reviewapp.Actor, command reviewapp.ClaimCommand) (reviewdomain.ClaimResult, error) {
	stub.claimCommand = command
	return stub.claim, nil
}

func (stub *publicReviewStub) Renew(context.Context, reviewapp.Actor, reviewapp.RenewCommand) (reviewdomain.ClaimResult, error) {
	return stub.claim, nil
}

func (stub *publicReviewStub) Release(context.Context, reviewapp.Actor, reviewapp.ReleaseCommand) (reviewdomain.HumanTask, error) {
	return stub.claim.Task, nil
}

func (stub *publicReviewStub) Decide(_ context.Context, _ reviewapp.Actor, command reviewapp.DecideCommand) (reviewdomain.DecisionResult, error) {
	stub.decideCommand = command
	return stub.decision, nil
}

type publicCoordinatorStub struct {
	coordination     workflowdomain.HumanGateCoordination
	err              error
	resumeDecisionID string
	resumeCalls      int
}

func (stub *publicCoordinatorStub) GetHumanGate(context.Context, workflowapp.Actor, string) (workflowdomain.HumanGateCoordination, error) {
	return stub.coordination, nil
}

func (stub *publicCoordinatorStub) ResumeHumanGate(_ context.Context, _ workflowapp.Actor, decisionID string) (workflowdomain.HumanGateCoordination, error) {
	stub.resumeDecisionID = decisionID
	stub.resumeCalls++
	return stub.coordination, stub.err
}

type publicAuthenticator struct {
	userID string
	err    error
}

func (authenticator publicAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	if authenticator.err != nil {
		return authentication.Claims{}, authenticator.err
	}
	if authenticator.userID == "" {
		return authentication.Claims{}, errors.New("missing test user")
	}
	return authentication.Claims{UserID: authenticator.userID, TokenVersion: 1}, nil
}

func stringPointer(value string) *string { return &value }
