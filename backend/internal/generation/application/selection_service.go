package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const applySelectionOperation = "generation.candidate.select"

var ErrSelectionNotFound = errors.New("generation candidate selection not found")

type SelectionDecision struct {
	ID, WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	HumanTaskID, SubjectType, SubjectID                  string
	SubjectRevision                                      int
	CandidateIDs                                         []string
	SelectedCandidateID, ReviewerID                      string
}

type ReviewDecisionReader interface {
	GetSelectedDecision(context.Context, Actor, string) (SelectionDecision, error)
}

type CandidateReadiness interface {
	RequireQCPassed(context.Context, Actor, string) (domain.CandidateWithReport, error)
}

type SelectionRepository interface {
	AuthorizeProject(context.Context, Actor, string, string, bool) error
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	EnsureReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
	EnsureSelection(context.Context, domain.CandidateSelection) (domain.CandidateSelection, error)
	GetSelection(context.Context, string) (domain.CandidateSelection, error)
}

type SelectionTransactionManager interface {
	WithinSelectionTransaction(context.Context, func(SelectionRepository) error) error
}

type SelectionConfig struct {
	Now   func() time.Time
	NewID func() string
}

type SelectionService struct {
	transactions SelectionTransactionManager
	candidates   CandidateReadiness
	decisions    ReviewDecisionReader
	config       SelectionConfig
}

type ApplySelectionCommand struct {
	ReviewDecisionID, IdempotencyKey string
}

type ApplySelectionResult struct {
	Selection domain.CandidateSelection
	Receipt   platformcommand.Receipt
}

type selectionReceipt struct {
	SelectionID string `json:"selection_id"`
}

type selectionCommandHashInput struct {
	ActorID, ReviewDecisionID string
}

type candidateSetHashInput struct {
	Candidates []domain.CandidateReference `json:"candidates"`
}

type selectionContentHashInput struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID                    string
	SubjectType, SubjectID                           string
	SubjectRevision                                  int
	Candidates                                       []domain.CandidateReference
	CandidateSetHash, SelectedCandidateID            string
	SelectedArtifactID, SelectedArtifactSHA256       string
	ReviewerID                                       string
}

func NewSelectionService(
	transactions SelectionTransactionManager,
	candidates CandidateReadiness,
	decisions ReviewDecisionReader,
	config SelectionConfig,
) *SelectionService {
	return &SelectionService{transactions: transactions, candidates: candidates, decisions: decisions, config: config}
}

func (service *SelectionService) ApplySelection(
	ctx context.Context,
	actor Actor,
	command ApplySelectionCommand,
) (ApplySelectionResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.ReviewDecisionID = strings.TrimSpace(command.ReviewDecisionID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.candidates == nil || service.decisions == nil ||
		service.config.Now == nil || service.config.NewID == nil || actor.TokenVersion < 1 ||
		!validUUID(actor.UserID) || !validUUID(command.ReviewDecisionID) || command.IdempotencyKey == "" ||
		len(command.IdempotencyKey) > 200 {
		return ApplySelectionResult{}, invalid("Invalid generation candidate selection request")
	}
	decision, references, candidateSetHash, selected, err := service.selectionProof(ctx, actor, command.ReviewDecisionID)
	if err != nil {
		return ApplySelectionResult{}, normalizeSelectionError(err)
	}
	contentHash, err := selectionContentHash(decision, references, candidateSetHash, selected)
	if err != nil {
		return ApplySelectionResult{}, err
	}
	selectionID := strings.TrimSpace(service.config.NewID())
	if !validUUID(selectionID) {
		return ApplySelectionResult{}, errors.New("generation selection identifier is invalid")
	}
	now := service.config.Now().UTC()
	desired := domain.CandidateSelection{
		ID: selectionID, WorkspaceID: decision.WorkspaceID, ProjectID: decision.ProjectID,
		WorkflowRunID: decision.WorkflowRunID, NodeRunID: decision.NodeRunID,
		HumanTaskID: decision.HumanTaskID, ReviewDecisionID: decision.ID,
		SubjectType: decision.SubjectType, SubjectID: decision.SubjectID, SubjectRevision: decision.SubjectRevision,
		Candidates: references, CandidateSetHash: candidateSetHash, SelectedCandidateID: decision.SelectedCandidateID,
		SelectedArtifactID: selected.ArtifactID, SelectedArtifactSHA256: selected.ArtifactSHA256,
		ReviewerID: decision.ReviewerID, ContentHash: contentHash, Revision: 1,
		CreatedBy: actor.UserID, CreatedAt: now,
	}
	inputHash, err := platformcommand.InputHash(selectionCommandHashInput{
		ActorID: actor.UserID, ReviewDecisionID: command.ReviewDecisionID,
	})
	if err != nil {
		return ApplySelectionResult{}, err
	}

	var result ApplySelectionResult
	err = service.transactions.WithinSelectionTransaction(ctx, func(repo SelectionRepository) error {
		if authorizeErr := repo.AuthorizeProject(ctx, actor, decision.WorkspaceID, decision.ProjectID, true); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, decision.WorkspaceID, applySelectionOperation, command.IdempotencyKey); findErr == nil {
			return replaySelection(ctx, repo, receipt, inputHash, desired, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		selection, ensureErr := repo.EnsureSelection(ctx, desired)
		if ensureErr != nil {
			return ensureErr
		}
		if !domain.SameSelectionBinding(selection, desired) {
			return platformcommand.ErrInputMismatch
		}
		encoded, encodeErr := platformcommand.Result(selectionReceipt{SelectionID: selection.ID})
		if encodeErr != nil {
			return encodeErr
		}
		receiptID := strings.TrimSpace(service.config.NewID())
		if !validUUID(receiptID) {
			return errors.New("generation selection receipt identifier is invalid")
		}
		receipt, ensureErr := repo.EnsureReceipt(ctx, platformcommand.Receipt{
			ID: receiptID, WorkspaceID: decision.WorkspaceID, Operation: applySelectionOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: selection.ID,
			Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
		})
		if ensureErr != nil {
			return ensureErr
		}
		result = ApplySelectionResult{Selection: selection, Receipt: receipt}
		return nil
	})
	return result, normalizeSelectionError(err)
}

func (service *SelectionService) RequireSelected(
	ctx context.Context,
	actor Actor,
	selectionID string,
) (domain.CandidateSelection, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	selectionID = strings.TrimSpace(selectionID)
	if service == nil || service.transactions == nil || service.candidates == nil || service.decisions == nil ||
		actor.TokenVersion < 1 || !validUUID(actor.UserID) || !validUUID(selectionID) {
		return domain.CandidateSelection{}, notFound("Generation candidate selection not found")
	}
	var persisted domain.CandidateSelection
	err := service.transactions.WithinSelectionTransaction(ctx, func(repo SelectionRepository) error {
		selection, loadErr := repo.GetSelection(ctx, selectionID)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = repo.AuthorizeProject(ctx, actor, selection.WorkspaceID, selection.ProjectID, false); loadErr != nil {
			return loadErr
		}
		persisted = selection
		return nil
	})
	if err != nil {
		return domain.CandidateSelection{}, normalizeSelectionError(err)
	}
	decision, references, candidateSetHash, selected, err := service.selectionProof(ctx, actor, persisted.ReviewDecisionID)
	if err != nil {
		return domain.CandidateSelection{}, normalizeSelectionError(err)
	}
	contentHash, err := selectionContentHash(decision, references, candidateSetHash, selected)
	if err != nil {
		return domain.CandidateSelection{}, err
	}
	expected := domain.CandidateSelection{
		WorkspaceID: decision.WorkspaceID, ProjectID: decision.ProjectID,
		WorkflowRunID: decision.WorkflowRunID, NodeRunID: decision.NodeRunID,
		HumanTaskID: decision.HumanTaskID, ReviewDecisionID: decision.ID,
		SubjectType: decision.SubjectType, SubjectID: decision.SubjectID, SubjectRevision: decision.SubjectRevision,
		Candidates: references, CandidateSetHash: candidateSetHash, SelectedCandidateID: decision.SelectedCandidateID,
		SelectedArtifactID: selected.ArtifactID, SelectedArtifactSHA256: selected.ArtifactSHA256,
		ReviewerID: decision.ReviewerID, ContentHash: contentHash, Revision: 1,
	}
	if !domain.SameSelectionBinding(persisted, expected) {
		return domain.CandidateSelection{}, conflict("Generation candidate selection facts have drifted")
	}
	return persisted, nil
}

func (service *SelectionService) selectionProof(
	ctx context.Context,
	actor Actor,
	decisionID string,
) (SelectionDecision, []domain.CandidateReference, string, domain.CandidateReference, error) {
	decision, err := service.decisions.GetSelectedDecision(ctx, actor, decisionID)
	if err != nil {
		return SelectionDecision{}, nil, "", domain.CandidateReference{}, err
	}
	if err = validateSelectionDecision(decision, decisionID); err != nil {
		return SelectionDecision{}, nil, "", domain.CandidateReference{}, err
	}
	references := make([]domain.CandidateReference, 0, len(decision.CandidateIDs))
	var selected domain.CandidateReference
	for _, candidateID := range decision.CandidateIDs {
		bundle, readinessErr := service.candidates.RequireQCPassed(ctx, actor, candidateID)
		if readinessErr != nil {
			return SelectionDecision{}, nil, "", domain.CandidateReference{}, readinessErr
		}
		if bundle.Candidate.ID != candidateID || bundle.Candidate.WorkspaceID != decision.WorkspaceID ||
			bundle.Candidate.ProjectID != decision.ProjectID || bundle.Report.CandidateID != candidateID ||
			bundle.Report.WorkspaceID != decision.WorkspaceID {
			return SelectionDecision{}, nil, "", domain.CandidateReference{}, conflict("Generation review candidate scope has drifted")
		}
		reference := domain.CandidateReference{
			ID: bundle.Candidate.ID, Revision: bundle.Candidate.Revision,
			ArtifactID: bundle.Candidate.ArtifactID, ArtifactRevision: bundle.Candidate.ArtifactRevision,
			ArtifactSHA256: bundle.Candidate.ArtifactSHA256,
			QCReportID:     bundle.Report.ID, QCReportHash: bundle.Report.ReportHash,
		}
		references = append(references, reference)
		if candidateID == decision.SelectedCandidateID {
			selected = reference
		}
	}
	if selected.ID == "" {
		return SelectionDecision{}, nil, "", domain.CandidateReference{}, conflict("Selected generation candidate is not frozen by review")
	}
	candidateSetHash, err := candidateReferencesHash(references)
	if err != nil {
		return SelectionDecision{}, nil, "", domain.CandidateReference{}, err
	}
	return decision, references, candidateSetHash, selected, nil
}

func candidateReferencesHash(references []domain.CandidateReference) (string, error) {
	return platformcommand.InputHash(candidateSetHashInput{Candidates: references})
}

func validateSelectionDecision(value SelectionDecision, expectedID string) error {
	identifiers := []string{
		value.ID, value.WorkspaceID, value.ProjectID, value.WorkflowRunID, value.NodeRunID,
		value.HumanTaskID, value.SubjectID, value.SelectedCandidateID, value.ReviewerID,
	}
	for _, identifier := range identifiers {
		if !validUUID(identifier) {
			return conflict("Generation review decision binding has drifted")
		}
	}
	if value.ID != expectedID || value.SubjectType != "generation_candidate_selection" ||
		value.SubjectRevision < 1 || len(value.CandidateIDs) == 0 || len(value.CandidateIDs) > 100 {
		return conflict("Generation review decision binding has drifted")
	}
	candidates := append([]string(nil), value.CandidateIDs...)
	for _, candidateID := range candidates {
		if !validUUID(candidateID) {
			return conflict("Generation review candidate binding has drifted")
		}
	}
	slices.Sort(candidates)
	compacted := slices.Compact(candidates)
	if len(compacted) != len(value.CandidateIDs) || !slices.Equal(compacted, value.CandidateIDs) ||
		!slices.Contains(value.CandidateIDs, value.SelectedCandidateID) {
		return conflict("Generation review candidate binding has drifted")
	}
	return nil
}

func selectionContentHash(
	decision SelectionDecision,
	references []domain.CandidateReference,
	candidateSetHash string,
	selected domain.CandidateReference,
) (string, error) {
	return platformcommand.InputHash(selectionContentHashInput{
		WorkspaceID: decision.WorkspaceID, ProjectID: decision.ProjectID,
		WorkflowRunID: decision.WorkflowRunID, NodeRunID: decision.NodeRunID,
		HumanTaskID: decision.HumanTaskID, ReviewDecisionID: decision.ID,
		SubjectType: decision.SubjectType, SubjectID: decision.SubjectID, SubjectRevision: decision.SubjectRevision,
		Candidates: references, CandidateSetHash: candidateSetHash, SelectedCandidateID: decision.SelectedCandidateID,
		SelectedArtifactID: selected.ArtifactID, SelectedArtifactSHA256: selected.ArtifactSHA256,
		ReviewerID: decision.ReviewerID,
	})
}

func replaySelection(
	ctx context.Context,
	repo SelectionRepository,
	receipt platformcommand.Receipt,
	inputHash string,
	desired domain.CandidateSelection,
	result *ApplySelectionResult,
) error {
	replayed, err := platformcommand.Replay[selectionReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	selection, err := repo.GetSelection(ctx, replayed.SelectionID)
	if err != nil {
		return err
	}
	if receipt.ResourceID != selection.ID || !domain.SameSelectionBinding(selection, desired) {
		return platformcommand.ErrInputMismatch
	}
	*result = ApplySelectionResult{Selection: selection, Receipt: receipt}
	return nil
}

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func normalizeSelectionError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Generation candidate selection command or binding has drifted")
	}
	if errors.Is(err, ErrSelectionNotFound) {
		return notFound("Generation candidate selection not found")
	}
	return normalizeError(err)
}
