package reference_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	referenceapp "github.com/StephenQiu30/lanverse/backend/internal/production/reference/application"
	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
)

type referenceCoverageReader struct {
	facts referenceapp.ReferenceCoverageFacts
	err   error
	calls int
}

func (reader *referenceCoverageReader) ReadCurrentReferenceCoverage(
	context.Context,
	string,
	string,
) (referenceapp.ReferenceCoverageFacts, error) {
	reader.calls++
	return reader.facts, reader.err
}

type referenceCoverageProjectReader struct {
	project projectdomain.Project
	err     error
}

func (reader referenceCoverageProjectReader) Get(
	context.Context,
	projectapp.Actor,
	string,
) (projectdomain.Project, error) {
	return reader.project, reader.err
}

func TestReferenceCoverageQueryComputesHonestCanonicalRows(t *testing.T) {
	approved := referenceBriefApprovedPlan(t)
	anchor := referenceBriefTargetByKind(t, approved, "character_identity_anchor")
	appearance := referenceBriefTargetByKind(t, approved, "character_appearance")
	acceptedAt := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	reader := &referenceCoverageReader{facts: referenceapp.ReferenceCoverageFacts{
		Plan:    approved.Version,
		Targets: []referencedomain.ReferencePlanTargetVersion{appearance, anchor},
		BriefExecutions: []referenceapp.ReferenceBriefExecutionFact{{
			TargetBusinessKey: anchor.TargetBusinessKey,
			InvocationID:      uuid.NewString(),
			InputHash:         referenceCoverageHash("anchor-input"),
			Status:            "accepted",
			Candidate: &referenceapp.ReferenceBriefCandidateRef{
				RevisionID: uuid.NewString(), Revision: 1,
				RevisionHash: referenceCoverageHash("anchor-revision"),
				ContentHash:  referenceCoverageHash("anchor-content"),
			},
			UpdatedAt: acceptedAt,
		}},
	}}
	query := referenceapp.NewReferenceCoverageQuery(
		reader,
		referenceCoverageProjectReader{project: projectdomain.Project{
			ID: approved.Version.ProjectID, WorkspaceID: approved.Version.WorkspaceID,
		}},
	)

	matrix, err := query.GetMatrix(context.Background(), referenceapp.Actor{}, approved.Version.ProjectID)
	if err != nil || matrix.SchemaVersion != referenceapp.ReferenceCoverageMatrixSchema ||
		matrix.PlanVersionID != approved.Version.ID || len(matrix.ContentHash) != 64 || len(matrix.Rows) != 2 ||
		matrix.Summary.RequiredTotal != 2 || matrix.Summary.ReferenceReady || matrix.Summary.SelectedTotal != 0 {
		t.Fatalf("Reference Coverage Matrix=%#v err=%v", matrix, err)
	}
	if matrix.Rows[0].TargetBusinessKey != anchor.TargetBusinessKey || matrix.Rows[0].Status != "planned" ||
		matrix.Rows[0].BriefStatus != "accepted" || matrix.Rows[0].BriefCandidate == nil ||
		!reflect.DeepEqual(matrix.Rows[0].AllowedActions, []string{"view_target"}) {
		t.Fatalf("base Reference row=%#v", matrix.Rows[0])
	}
	if matrix.Rows[1].TargetBusinessKey != appearance.TargetBusinessKey || matrix.Rows[1].Status != "blocked" ||
		matrix.Rows[1].BriefStatus != "dependency_blocked" || len(matrix.Rows[1].Blockers) != 1 ||
		matrix.Rows[1].Blockers[0].TargetBusinessKey != anchor.TargetBusinessKey {
		t.Fatalf("dependent Reference row=%#v", matrix.Rows[1])
	}

	reader.facts.Targets[0], reader.facts.Targets[1] = reader.facts.Targets[1], reader.facts.Targets[0]
	replayed, err := query.GetMatrix(context.Background(), referenceapp.Actor{}, approved.Version.ProjectID)
	if err != nil || !reflect.DeepEqual(replayed, matrix) {
		t.Fatalf("reordered facts changed Matrix: got=%#v want=%#v err=%v", replayed, matrix, err)
	}
	detail, err := query.GetTarget(
		context.Background(), referenceapp.Actor{}, approved.Version.ProjectID, appearance.ID,
	)
	if err != nil || detail.Target.ID != appearance.ID || detail.Row.TargetVersionID != appearance.ID ||
		detail.Row.Status != "blocked" || len(detail.ContentHash) != 64 {
		t.Fatalf("Reference Target detail=%#v err=%v", detail, err)
	}
}

func TestReferenceCoverageQueryRejectsPermissionAndFactDrift(t *testing.T) {
	approved := referenceBriefApprovedPlan(t)
	reader := &referenceCoverageReader{facts: referenceapp.ReferenceCoverageFacts{Plan: approved.Version}}
	denied := errors.New("current permission revoked")
	query := referenceapp.NewReferenceCoverageQuery(
		reader,
		referenceCoverageProjectReader{err: denied},
	)
	if _, err := query.GetMatrix(context.Background(), referenceapp.Actor{}, approved.Version.ProjectID); !errors.Is(err, denied) || reader.calls != 0 {
		t.Fatal("Reference Coverage facts were read before current Project authorization")
	}

	reader.err = referenceapp.ErrReferenceCoverageDrift
	query = referenceapp.NewReferenceCoverageQuery(
		reader,
		referenceCoverageProjectReader{project: projectdomain.Project{
			ID: approved.Version.ProjectID, WorkspaceID: approved.Version.WorkspaceID,
		}},
	)
	if _, err := query.GetMatrix(context.Background(), referenceapp.Actor{}, approved.Version.ProjectID); err == nil {
		t.Fatal("Reference Coverage fact drift was accepted")
	} else {
		var problem *referenceapp.QueryError
		if !errors.As(err, &problem) || problem.Status != 409 || problem.Code != "formal_version_drift" {
			t.Fatalf("Reference Coverage fact drift error=%#v", err)
		}
	}

	reader.err = nil
	reader.facts.Targets = append([]referencedomain.ReferencePlanTargetVersion(nil), approved.Targets...)
	dependent := referenceBriefTargetByKind(t, approved, "character_appearance")
	reader.facts.BriefExecutions = []referenceapp.ReferenceBriefExecutionFact{{
		TargetBusinessKey: dependent.TargetBusinessKey,
		InvocationID:      uuid.NewString(), InputHash: referenceCoverageHash("illegal-dependent-input"),
		Status: "accepted", Candidate: &referenceapp.ReferenceBriefCandidateRef{
			RevisionID: uuid.NewString(), Revision: 1,
			RevisionHash: referenceCoverageHash("illegal-revision"), ContentHash: referenceCoverageHash("illegal-content"),
		}, UpdatedAt: time.Now().UTC(),
	}}
	query = referenceapp.NewReferenceCoverageQuery(
		reader,
		referenceCoverageProjectReader{project: projectdomain.Project{
			ID: approved.Version.ProjectID, WorkspaceID: approved.Version.WorkspaceID,
		}},
	)
	if _, err := query.GetMatrix(context.Background(), referenceapp.Actor{}, approved.Version.ProjectID); err == nil {
		t.Fatal("dependent Target with premature Brief execution was reported as ready")
	}
	if _, err := query.GetTarget(context.Background(), referenceapp.Actor{}, approved.Version.ProjectID, "current"); err == nil {
		t.Fatal("Reference Target detail accepted a mutable selector")
	}
}

func referenceCoverageHash(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}
