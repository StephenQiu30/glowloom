package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformowner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
)

const (
	ReferenceCoverageMatrixSchema = "reference-coverage-matrix-production"
	ReferenceTargetDetailSchema   = "reference-target-detail-production"
)

var (
	ErrReferenceCoverageNotFound = errors.New("Reference Coverage facts not found")
	ErrReferenceCoverageDrift    = errors.New("Reference Coverage facts have drifted")
	referenceCoverageHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Actor struct {
	UserID       string
	TokenVersion int
}

type ReferenceBriefCandidateRef struct {
	RevisionID   string `json:"revision_id"`
	Revision     int64  `json:"revision"`
	RevisionHash string `json:"revision_hash"`
	ContentHash  string `json:"content_hash"`
}

type ReferenceBriefExecutionFact struct {
	TargetBusinessKey string
	InvocationID      string
	InputHash         string
	Status            string
	Candidate         *ReferenceBriefCandidateRef
	UpdatedAt         time.Time
}

type ReferenceCoverageFacts struct {
	Plan            referencedomain.ApprovedReferencePlanVersion
	Targets         []referencedomain.ReferencePlanTargetVersion
	BriefExecutions []ReferenceBriefExecutionFact
}

type ReferenceCoverageBlocker struct {
	Code              string `json:"code"`
	TargetBusinessKey string `json:"target_business_key,omitempty"`
}

type ReferenceCoverageRow struct {
	TargetVersionID             string                      `json:"target_version_id"`
	TargetContentHash           string                      `json:"target_content_hash"`
	TargetBusinessKey           string                      `json:"target_business_key"`
	TargetKind                  string                      `json:"target_kind"`
	Fulfillment                 string                      `json:"fulfillment"`
	WaveKey                     string                      `json:"wave_key"`
	CoverageScopeKeys           []string                    `json:"coverage_scope_keys"`
	DependsOnTargetBusinessKeys []string                    `json:"depends_on_target_business_keys"`
	Status                      string                      `json:"status"`
	BriefStatus                 string                      `json:"brief_status"`
	BriefInvocationID           *string                     `json:"brief_invocation_id,omitempty"`
	BriefInputHash              *string                     `json:"brief_input_hash,omitempty"`
	BriefCandidate              *ReferenceBriefCandidateRef `json:"brief_candidate,omitempty"`
	Blockers                    []ReferenceCoverageBlocker  `json:"blockers"`
	AllowedActions              []string                    `json:"allowed_actions"`
}

type ReferenceCoverageSummary struct {
	RequiredTotal  int  `json:"required_total"`
	OptionalTotal  int  `json:"optional_total"`
	NotGenerated   int  `json:"not_generated"`
	Planned        int  `json:"planned"`
	Generating     int  `json:"generating"`
	Blocked        int  `json:"blocked"`
	SelectedTotal  int  `json:"selected_total"`
	ReferenceReady bool `json:"reference_ready"`
}

type ReferenceCoverageMatrix struct {
	SchemaVersion   string                   `json:"schema_version"`
	PlanVersionID   string                   `json:"plan_version_id"`
	PlanRevision    int64                    `json:"plan_revision"`
	PlanContentHash string                   `json:"plan_content_hash"`
	Rows            []ReferenceCoverageRow   `json:"rows"`
	Summary         ReferenceCoverageSummary `json:"summary"`
	ContentHash     string                   `json:"content_hash"`
}

type ReferenceTargetDetail struct {
	SchemaVersion   string                                     `json:"schema_version"`
	PlanVersionID   string                                     `json:"plan_version_id"`
	PlanRevision    int64                                      `json:"plan_revision"`
	PlanContentHash string                                     `json:"plan_content_hash"`
	Target          referencedomain.ReferencePlanTargetVersion `json:"target"`
	Row             ReferenceCoverageRow                       `json:"row"`
	ContentHash     string                                     `json:"content_hash"`
}

type ReferenceCoverageReader interface {
	ReadCurrentReferenceCoverage(context.Context, string, string) (ReferenceCoverageFacts, error)
}

type ReferenceCoverageProjectReader interface {
	Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error)
}

type ReferenceCoverageQuery struct {
	facts    ReferenceCoverageReader
	projects ReferenceCoverageProjectReader
}

type QueryError struct {
	Code, Message string
	Status        int
}

func (problem *QueryError) Error() string { return problem.Message }

func NewReferenceCoverageQuery(
	facts ReferenceCoverageReader,
	projects ReferenceCoverageProjectReader,
) *ReferenceCoverageQuery {
	return &ReferenceCoverageQuery{facts: facts, projects: projects}
}

func (query *ReferenceCoverageQuery) GetMatrix(
	ctx context.Context,
	actor Actor,
	projectID string,
) (ReferenceCoverageMatrix, error) {
	matrix, _, err := query.loadMatrix(ctx, actor, projectID)
	return matrix, err
}

func (query *ReferenceCoverageQuery) GetTarget(
	ctx context.Context,
	actor Actor,
	projectID string,
	targetVersionID string,
) (ReferenceTargetDetail, error) {
	if parsed, err := uuid.Parse(targetVersionID); err != nil || parsed == uuid.Nil {
		return ReferenceTargetDetail{}, &QueryError{Code: "validation_failed", Message: "Invalid Reference Target identity", Status: 422}
	}
	matrix, facts, err := query.loadMatrix(ctx, actor, projectID)
	if err != nil {
		return ReferenceTargetDetail{}, err
	}
	for _, target := range facts.Targets {
		if target.ID != targetVersionID {
			continue
		}
		rowIndex := slices.IndexFunc(matrix.Rows, func(row ReferenceCoverageRow) bool {
			return row.TargetVersionID == targetVersionID
		})
		if rowIndex < 0 {
			break
		}
		detail := ReferenceTargetDetail{
			SchemaVersion: ReferenceTargetDetailSchema,
			PlanVersionID: matrix.PlanVersionID, PlanRevision: matrix.PlanRevision,
			PlanContentHash: matrix.PlanContentHash, Target: target, Row: matrix.Rows[rowIndex],
		}
		detail.ContentHash, err = referenceTargetDetailHash(detail)
		if err != nil {
			return ReferenceTargetDetail{}, err
		}
		return detail, nil
	}
	return ReferenceTargetDetail{}, &QueryError{Code: "not_found", Message: "Current Reference Target not found", Status: 404}
}

func (query *ReferenceCoverageQuery) loadMatrix(
	ctx context.Context,
	actor Actor,
	projectID string,
) (ReferenceCoverageMatrix, ReferenceCoverageFacts, error) {
	project, err := query.authorize(ctx, actor, projectID)
	if err != nil {
		return ReferenceCoverageMatrix{}, ReferenceCoverageFacts{}, err
	}
	facts, err := query.facts.ReadCurrentReferenceCoverage(ctx, project.WorkspaceID, projectID)
	if errors.Is(err, ErrReferenceCoverageNotFound) {
		return ReferenceCoverageMatrix{}, ReferenceCoverageFacts{}, &QueryError{
			Code: "not_found", Message: "Current Reference Plan not found", Status: 404,
		}
	}
	if errors.Is(err, ErrReferenceCoverageDrift) {
		return ReferenceCoverageMatrix{}, ReferenceCoverageFacts{}, &QueryError{
			Code: "formal_version_drift", Message: "Reference Coverage facts have drifted", Status: 409,
		}
	}
	if err != nil {
		return ReferenceCoverageMatrix{}, ReferenceCoverageFacts{}, err
	}
	matrix, err := buildReferenceCoverageMatrix(facts, project.WorkspaceID, projectID)
	if err != nil {
		return ReferenceCoverageMatrix{}, ReferenceCoverageFacts{}, &QueryError{
			Code: "formal_version_drift", Message: "Reference Coverage facts have drifted", Status: 409,
		}
	}
	return matrix, facts, nil
}

func (query *ReferenceCoverageQuery) authorize(
	ctx context.Context,
	actor Actor,
	projectID string,
) (projectdomain.Project, error) {
	if query == nil || query.facts == nil || query.projects == nil {
		return projectdomain.Project{}, errors.New("Reference Coverage query is unavailable")
	}
	if parsed, err := uuid.Parse(projectID); err != nil || parsed == uuid.Nil {
		return projectdomain.Project{}, &QueryError{Code: "validation_failed", Message: "Invalid Project identity", Status: 422}
	}
	project, err := query.projects.Get(
		ctx, projectapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, projectID,
	)
	if err != nil {
		var problem *projectapp.Error
		if errors.As(err, &problem) {
			return projectdomain.Project{}, &QueryError{Code: problem.Code, Message: problem.Message, Status: problem.Status}
		}
		return projectdomain.Project{}, err
	}
	if project.ID != projectID || uuid.Validate(project.WorkspaceID) != nil {
		return projectdomain.Project{}, &QueryError{Code: "formal_version_drift", Message: "Project scope has drifted", Status: 409}
	}
	return project, nil
}

func buildReferenceCoverageMatrix(
	facts ReferenceCoverageFacts,
	workspaceID string,
	projectID string,
) (ReferenceCoverageMatrix, error) {
	plan := facts.Plan
	if plan.WorkspaceID != workspaceID || plan.ProjectID != projectID || uuid.Validate(plan.ID) != nil ||
		plan.Revision < 1 || !referenceCoverageHashPattern.MatchString(plan.ContentHash) ||
		len(facts.Targets) != len(plan.TargetVersionRefs) {
		return ReferenceCoverageMatrix{}, errors.New("invalid Reference Coverage Plan")
	}
	targets := append([]referencedomain.ReferencePlanTargetVersion(nil), facts.Targets...)
	for _, target := range targets {
		if !slices.ContainsFunc(plan.TargetVersionRefs, func(ref platformowner.VersionRef) bool {
			return exactReferenceCoverageTargetRef(ref, target)
		}) {
			return ReferenceCoverageMatrix{}, errors.New("Reference Coverage Target is outside current Plan")
		}
	}
	dag, err := referencedomain.BuildReferenceTargetDependencyDAG(plan.ID, targets)
	if err != nil {
		return ReferenceCoverageMatrix{}, err
	}
	waves := make(map[string]string, len(dag.Nodes))
	for _, node := range dag.Nodes {
		waves[node.TargetBusinessKey] = node.WaveKey
	}
	executions := make(map[string]ReferenceBriefExecutionFact, len(facts.BriefExecutions))
	for _, execution := range facts.BriefExecutions {
		targetIndex := slices.IndexFunc(targets, func(target referencedomain.ReferencePlanTargetVersion) bool {
			return target.TargetBusinessKey == execution.TargetBusinessKey
		})
		if targetIndex < 0 || len(targets[targetIndex].DependsOnTargetBusinessKeys) != 0 ||
			targets[targetIndex].Fulfillment == "not_generated" || validateReferenceBriefExecution(execution) != nil {
			return ReferenceCoverageMatrix{}, errors.New("invalid Reference Brief execution fact")
		}
		if _, duplicate := executions[execution.TargetBusinessKey]; duplicate {
			return ReferenceCoverageMatrix{}, errors.New("duplicate Reference Brief execution fact")
		}
		executions[execution.TargetBusinessKey] = execution
	}
	sort.Slice(targets, func(left, right int) bool {
		leftRank, rightRank := referenceCoverageWaveRank(waves[targets[left].TargetBusinessKey]), referenceCoverageWaveRank(waves[targets[right].TargetBusinessKey])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return targets[left].TargetBusinessKey < targets[right].TargetBusinessKey
	})
	matrix := ReferenceCoverageMatrix{
		SchemaVersion: ReferenceCoverageMatrixSchema,
		PlanVersionID: plan.ID, PlanRevision: plan.Revision, PlanContentHash: plan.ContentHash,
		Rows: make([]ReferenceCoverageRow, len(targets)),
	}
	for index, target := range targets {
		row := ReferenceCoverageRow{
			TargetVersionID: target.ID, TargetContentHash: target.ContentHash,
			TargetBusinessKey: target.TargetBusinessKey, TargetKind: target.TargetKind,
			Fulfillment: target.Fulfillment, WaveKey: waves[target.TargetBusinessKey],
			CoverageScopeKeys:           append([]string{}, target.CoverageScopeKeys...),
			DependsOnTargetBusinessKeys: append([]string{}, target.DependsOnTargetBusinessKeys...),
			Blockers:                    []ReferenceCoverageBlocker{}, AllowedActions: []string{"view_target"},
		}
		switch {
		case target.Fulfillment == "not_generated":
			row.WaveKey, row.Status, row.BriefStatus = "none", "not_generated", "not_required"
			matrix.Summary.NotGenerated++
		case len(target.DependsOnTargetBusinessKeys) > 0:
			row.Status, row.BriefStatus = "blocked", "dependency_blocked"
			for _, dependency := range target.DependsOnTargetBusinessKeys {
				row.Blockers = append(row.Blockers, ReferenceCoverageBlocker{
					Code: "dependency_asset_version_unavailable", TargetBusinessKey: dependency,
				})
			}
			matrix.Summary.Blocked++
		default:
			row.Status, row.BriefStatus = "planned", "not_started"
			if execution, ok := executions[target.TargetBusinessKey]; ok {
				row.BriefStatus = execution.Status
				row.BriefInvocationID, row.BriefInputHash = &execution.InvocationID, &execution.InputHash
				row.BriefCandidate = execution.Candidate
				switch execution.Status {
				case "queued", "running":
					row.Status = "generating"
				case "rejected", "outcome_unknown":
					row.Status = "blocked"
					row.Blockers = append(row.Blockers, ReferenceCoverageBlocker{Code: "reference_brief_" + execution.Status})
				}
			}
			switch row.Status {
			case "planned":
				matrix.Summary.Planned++
			case "generating":
				matrix.Summary.Generating++
			case "blocked":
				matrix.Summary.Blocked++
			}
		}
		switch target.Fulfillment {
		case "required":
			matrix.Summary.RequiredTotal++
		case "optional":
			matrix.Summary.OptionalTotal++
		}
		matrix.Rows[index] = row
	}
	matrix.ContentHash, err = referenceCoverageMatrixHash(matrix)
	if err != nil {
		return ReferenceCoverageMatrix{}, err
	}
	return matrix, nil
}

func validateReferenceBriefExecution(value ReferenceBriefExecutionFact) error {
	if uuid.Validate(value.InvocationID) != nil || !referenceCoverageHashPattern.MatchString(value.InputHash) ||
		!slices.Contains([]string{"queued", "running", "accepted", "rejected", "outcome_unknown"}, value.Status) ||
		(value.Status == "accepted") != (value.Candidate != nil) || value.UpdatedAt.IsZero() {
		return errors.New("invalid Reference Brief execution")
	}
	if value.Candidate != nil && (uuid.Validate(value.Candidate.RevisionID) != nil || value.Candidate.Revision < 1 ||
		!referenceCoverageHashPattern.MatchString(value.Candidate.RevisionHash) ||
		!referenceCoverageHashPattern.MatchString(value.Candidate.ContentHash)) {
		return errors.New("invalid Reference Brief Candidate ref")
	}
	return nil
}

func referenceCoverageWaveRank(value string) int {
	switch value {
	case "base":
		return 0
	case "appearance":
		return 1
	case "composition":
		return 2
	default:
		return 3
	}
}

func exactReferenceCoverageTargetRef(
	ref platformowner.VersionRef,
	target referencedomain.ReferencePlanTargetVersion,
) bool {
	return ref.WorkspaceID == target.WorkspaceID && ref.ProjectID == target.ProjectID &&
		ref.OwnerKind == "production/reference" && ref.VersionFamily == "reference_plan_set" &&
		ref.OwnerLogicalID == target.TargetBusinessKey && ref.OwnerVersionID == target.ID &&
		ref.OwnerRevision == target.Revision && ref.OwnerContentHash == target.ContentHash
}

func referenceCoverageMatrixHash(value ReferenceCoverageMatrix) (string, error) {
	material := struct {
		SchemaVersion   string
		PlanVersionID   string
		PlanRevision    int64
		PlanContentHash string
		Rows            []ReferenceCoverageRow
		Summary         ReferenceCoverageSummary
	}{value.SchemaVersion, value.PlanVersionID, value.PlanRevision, value.PlanContentHash, value.Rows, value.Summary}
	raw, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func referenceTargetDetailHash(value ReferenceTargetDetail) (string, error) {
	material := struct {
		SchemaVersion   string
		PlanVersionID   string
		PlanRevision    int64
		PlanContentHash string
		Target          referencedomain.ReferencePlanTargetVersion
		Row             ReferenceCoverageRow
	}{value.SchemaVersion, value.PlanVersionID, value.PlanRevision, value.PlanContentHash, value.Target, value.Row}
	raw, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}
