package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	database "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AcceptedVisionReviewSetReader func(context.Context, *gorm.DB, []domain.GenerationRevisionRef) ([]agentapp.VisionReviewExecutionState, error)
type ReferenceCandidateSetStore struct {
	database *gorm.DB
	reviews  AcceptedVisionReviewSetReader
}

func NewReferenceCandidateSetStore(db *gorm.DB, reviews AcceptedVisionReviewSetReader) (*ReferenceCandidateSetStore, error) {
	if db == nil || reviews == nil {
		return nil, errors.New("candidate Set database and review Owner required")
	}
	return &ReferenceCandidateSetStore{database: db, reviews: reviews}, nil
}
func (store *ReferenceCandidateSetStore) within(ctx context.Context, operation func(*gorm.DB) error) error {
	if _, nested := store.database.Statement.ConnPool.(gorm.TxCommitter); nested {
		return errors.New("candidate Set requires its own commit boundary")
	}
	return database.WithinSerializableTransaction(ctx, store.database, operation)
}

func (store *ReferenceCandidateSetStore) current(ctx context.Context, tx *gorm.DB, workspaceID string, command app.ReferenceCandidateSetCommand) (domain.ReferenceCandidateSet, error) {
	bundles := make([]domain.ReferenceCandidateBundle, 0, len(command.BundleRefs))
	reviewRefs := make([]domain.GenerationRevisionRef, 0, len(command.BundleRefs))
	for _, ref := range command.BundleRefs {
		var row model.GenerationReferenceCandidateBundle
		if err := tx.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ?", ref.ID, workspaceID, command.ProjectID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ReferenceCandidateSet{}, notFound("Candidate Bundle not found")
			}
			return domain.ReferenceCandidateSet{}, err
		}
		bundle, err := decodeReferenceCandidateBundleRow(row)
		if err != nil {
			return domain.ReferenceCandidateSet{}, err
		}
		if ref.Revision != 1 || bundle.ContentHash != ref.ContentHash || bundle.ExecutionRef != command.ExecutionRef {
			return domain.ReferenceCandidateSet{}, &app.Error{Code: "candidate_set_conflict", Message: "Candidate Bundle differs from frozen selection", Status: 409}
		}
		bundles = append(bundles, bundle)
		reviewRefs = append(reviewRefs, bundle.VisionReviewRef)
	}
	// The Agent Owner acquires every review Control before current Owner locks.
	states, err := store.reviews(ctx, tx, reviewRefs)
	if err != nil {
		var reviewError *agentapp.Error
		if errors.As(err, &reviewError) {
			return domain.ReferenceCandidateSet{}, fmt.Errorf("%w: %w", &app.Error{Code: "reference_review_unavailable", Message: "Candidate Set review is no longer current", Status: 409}, err)
		}
		return domain.ReferenceCandidateSet{}, err
	}
	if len(states) != len(bundles) {
		return domain.ReferenceCandidateSet{}, errors.New("candidate Set review coverage changed")
	}
	for i, state := range states {
		bundle := bundles[i]
		subject := state.Record.Invocation.Payload.StageInput.Subject
		if state.Status != "accepted" || state.Candidate.ID != bundle.VisionReviewRef.ID || state.Candidate.Revision != bundle.VisionReviewRef.Revision || state.Candidate.CandidateRevisionHash != bundle.VisionReviewRef.ContentHash || !state.Candidate.CreatedAt.Equal(bundle.CreatedAt) || subject.WorkspaceID != bundle.WorkspaceID || subject.ProjectID != bundle.ProjectID || subject.ExecutionRef != bundle.ExecutionRef || subject.TargetRef != bundle.TargetRef || subject.CandidateBundleIndex != bundle.CandidateBundleIndex || subject.BundleInputRef != bundle.BundleInputRef || subject.GenerationRound != bundle.GenerationRound {
			return domain.ReferenceCandidateSet{}, errors.New("candidate Set review does not match persisted Bundle")
		}
	}
	facts, err := readReferenceCandidateSetFacts(ctx, tx, workspaceID, command.ProjectID, command.ExecutionRef)
	if err != nil {
		return domain.ReferenceCandidateSet{}, err
	}
	progress, err := domain.BuildReferenceJobProgress(facts.Job, facts.Calls, facts.States)
	if err != nil {
		return domain.ReferenceCandidateSet{}, err
	}
	if progress.ContentHash != command.ExpectedProgressHash {
		return domain.ReferenceCandidateSet{}, &app.Error{Code: "candidate_set_progress_changed", Message: "Reference execution progress changed", Status: 409}
	}
	value, err := domain.BuildReferenceCandidateSet(facts, bundles)
	if err != nil {
		return domain.ReferenceCandidateSet{}, fmt.Errorf("%w: %w", &app.Error{Code: "candidate_set_incomplete", Message: "Candidate Set requires exact reviewed groups or explicit diagnostics", Status: 409}, err)
	}
	return value, nil
}

func readReferenceCandidateSetFacts(ctx context.Context, tx *gorm.DB, workspaceID, projectID string, expected domain.GenerationRevisionRef) (domain.ReferenceBundleFacts, error) {
	targets := &referenceTargetRepository{referenceAuthorizationRepository{repository{database: tx}}}
	executions := &referenceExecutionRepository{referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}}, tx}
	execution, err := executions.FindReferenceExecution(ctx, workspaceID, projectID, expected.ID)
	if err != nil {
		return domain.ReferenceBundleFacts{}, err
	}
	if expected != (domain.GenerationRevisionRef{ID: execution.ID, Revision: execution.Revision, ContentHash: execution.ContentHash}) {
		return domain.ReferenceBundleFacts{}, &app.Error{Code: "candidate_set_conflict", Message: "Reference execution identity changed", Status: 409}
	}
	if err := executions.ValidateReferenceExecutionHead(ctx, execution); err != nil {
		return domain.ReferenceBundleFacts{}, err
	}
	target, err := targets.FindReferenceGenerationTarget(ctx, workspaceID, projectID, execution.ReadSet.TargetRef.ID)
	if err != nil {
		return domain.ReferenceBundleFacts{}, err
	}
	receipt, err := targets.FindReferenceAuthorization(ctx, target.GenerationAuthorizationRef.ID)
	if err != nil {
		return domain.ReferenceBundleFacts{}, err
	}
	authorization, err := domain.DecodeReferenceGenerationAuthorization(receipt.Result)
	if err != nil {
		return domain.ReferenceBundleFacts{}, err
	}
	// Read permission belongs to this caller; input validity belongs to the
	// original author, whose exact token/receipt the Target Owner revalidates.
	target, err = app.ReadCurrentReferenceGenerationTarget(ctx, targets, app.Actor{UserID: authorization.AuthorizedBy, TokenVersion: authorization.MembershipTokenVersion}, app.ReadReferenceGenerationTargetQuery{WorkspaceID: workspaceID, ProjectID: projectID, TargetRef: execution.ReadSet.TargetRef})
	if err != nil {
		return domain.ReferenceBundleFacts{}, err
	}
	if execution.ReadSet.TargetReadSetRoot != target.TargetReadSetRoot {
		return domain.ReferenceBundleFacts{}, errors.New("candidate Set Target read set drifted")
	}
	job, calls, states, err := executions.readReferenceProviderJob(ctx, workspaceID, projectID, expected.ID)
	if err != nil {
		return domain.ReferenceBundleFacts{}, err
	}
	if job.ExecutionRef != expected {
		return domain.ReferenceBundleFacts{}, errors.New("candidate Set Job differs from Execution")
	}
	facts := domain.ReferenceBundleFacts{WorkspaceID: workspaceID, ProjectID: projectID, TargetRef: execution.ReadSet.TargetRef, GenerationRound: target.GenerationRound, TargetKind: target.TargetKind, DependencyRootHash: target.DependencyRootHash, Output: target.OutputContract, Job: job, Calls: calls, States: states}
	progress, err := domain.BuildReferenceJobProgress(job, calls, states)
	if err != nil {
		return domain.ReferenceBundleFacts{}, err
	}
	if progress.OutcomeUnknown > 0 {
		return facts, nil
	}
	var rows []model.GenerationReferenceStagedMedia
	if err := tx.WithContext(ctx).Where("call_key IN ?", job.CallKeys).Find(&rows).Error; err != nil {
		return domain.ReferenceBundleFacts{}, err
	}
	for _, row := range rows {
		media, err := referenceStagedMediaFromRecord(row)
		if err != nil {
			return domain.ReferenceBundleFacts{}, err
		}
		facts.Media = append(facts.Media, media)
	}
	return facts, nil
}

func (store *ReferenceCandidateSetStore) MaterializeReferenceCandidateSet(ctx context.Context, actor app.Actor, command app.ReferenceCandidateSetCommand) (domain.ReferenceCandidateSet, error) {
	var value domain.ReferenceCandidateSet
	err := store.within(ctx, func(tx *gorm.DB) error {
		repo := repository{database: tx}
		scope, err := repo.authorizeProject(ctx, actor, "", command.ProjectID, "write")
		if err != nil {
			return err
		}
		value, err = store.current(ctx, tx, scope.WorkspaceID, command)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		row := model.GenerationReferenceCandidateSet{ID: uuid.MustParse(value.ID), WorkspaceID: uuid.MustParse(value.WorkspaceID), ProjectID: uuid.MustParse(value.ProjectID), ExecutionID: uuid.MustParse(value.ExecutionRef.ID), ContentHash: value.ContentHash, Content: datatypes.JSON(raw), CreatedAt: value.CreatedAt}
		if err := tx.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).First(&row, "id = ?", value.ID).Error; err != nil {
			return err
		}
		persisted, err := decodeReferenceCandidateSetRow(row)
		if err != nil || !reflect.DeepEqual(value, persisted) {
			return errors.New("candidate Set persisted identity conflicts")
		}
		return nil
	})
	if err != nil {
		return domain.ReferenceCandidateSet{}, err
	}
	return value, nil
}

func (store *ReferenceCandidateSetStore) ReadReferenceCandidateSet(ctx context.Context, actor app.Actor, projectID, setID string) (domain.ReferenceCandidateSet, error) {
	var value domain.ReferenceCandidateSet
	err := store.within(ctx, func(tx *gorm.DB) error {
		repo := repository{database: tx}
		scope, err := repo.authorizeProject(ctx, actor, "", projectID, "read")
		if err != nil {
			return err
		}
		var row model.GenerationReferenceCandidateSet
		if err := tx.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ?", setID, scope.WorkspaceID, projectID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound("Reference candidate Set not found")
			}
			return err
		}
		value, err = decodeReferenceCandidateSetRow(row)
		if err != nil {
			return err
		}
		refs := make([]domain.GenerationRevisionRef, 0, len(value.BundleRefs))
		for _, member := range value.BundleRefs {
			refs = append(refs, member.BundleRef)
		}
		current, err := store.current(ctx, tx, scope.WorkspaceID, app.ReferenceCandidateSetCommand{ProjectID: projectID, ExecutionRef: value.ExecutionRef, ExpectedProgressHash: value.ExecutionProgressHash, BundleRefs: refs})
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(value, current) {
			return errors.New("candidate Set facts no longer match")
		}
		return nil
	})
	if err != nil {
		return domain.ReferenceCandidateSet{}, err
	}
	return value, nil
}

func decodeReferenceCandidateSetRow(row model.GenerationReferenceCandidateSet) (domain.ReferenceCandidateSet, error) {
	value, err := domain.DecodeReferenceCandidateSet(json.RawMessage(row.Content))
	if err != nil || value.ID != row.ID.String() || value.WorkspaceID != row.WorkspaceID.String() || value.ProjectID != row.ProjectID.String() || value.ExecutionRef.ID != row.ExecutionID.String() || value.ContentHash != row.ContentHash || !value.CreatedAt.Equal(row.CreatedAt) {
		return domain.ReferenceCandidateSet{}, errors.New("candidate Set columns differ from content")
	}
	return value, nil
}

var _ app.ReferenceCandidateSetPersistence = (*ReferenceCandidateSetStore)(nil)
