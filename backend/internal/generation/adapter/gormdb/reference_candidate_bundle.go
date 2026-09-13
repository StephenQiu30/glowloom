package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	database "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AcceptedVisionReviewReader func(context.Context, *gorm.DB, domain.GenerationRevisionRef) (agentapp.VisionReviewExecutionState, error)

type ReferenceCandidateBundleStore struct {
	database *gorm.DB
	reviews  AcceptedVisionReviewReader
}

func NewReferenceCandidateBundleStore(db *gorm.DB, reviews AcceptedVisionReviewReader) (*ReferenceCandidateBundleStore, error) {
	if db == nil || reviews == nil {
		return nil, errors.New("candidate Bundle database and review Owner are required")
	}
	return &ReferenceCandidateBundleStore{database: db, reviews: reviews}, nil
}

func (store *ReferenceCandidateBundleStore) within(ctx context.Context, operation func(*gorm.DB) error) error {
	if _, nested := store.database.Statement.ConnPool.(gorm.TxCommitter); nested {
		return errors.New("candidate Bundle requires its own commit boundary")
	}
	return database.WithinSerializableTransaction(ctx, store.database, operation)
}

func (store *ReferenceCandidateBundleStore) current(ctx context.Context, tx *gorm.DB, command app.ReferenceCandidateBundleCommand) (domain.ReferenceCandidateBundle, error) {
	state, err := store.reviews(ctx, tx, command.VisionReviewRef)
	if err != nil {
		var reviewError *agentapp.Error
		if errors.As(err, &reviewError) && (reviewError.Code == "release_not_executable" || reviewError.Code == "stale_vision_review_input" || reviewError.Code == "candidate_conflict") {
			return domain.ReferenceCandidateBundle{}, fmt.Errorf("%w: %w", &app.Error{Code: "reference_review_unavailable", Message: "Reference Bundle review or current input is no longer valid", Status: 409}, err)
		}
		return domain.ReferenceCandidateBundle{}, err
	}
	input := state.Record.Invocation.Payload.StageInput
	subject := input.Subject
	if subject.WorkspaceID != command.WorkspaceID || subject.ProjectID != command.ProjectID || subject.ExecutionRef != command.ExecutionRef || state.Status != "accepted" || state.Candidate.ID != command.VisionReviewRef.ID || state.Candidate.CandidateRevisionHash != command.VisionReviewRef.ContentHash || state.Result == nil {
		return domain.ReferenceCandidateBundle{}, errors.New("candidate Bundle review scope changed")
	}
	review, _, err := contract.DecodeVisionReviewCandidate(state.Result.Candidate)
	if err != nil || review.ValidateFor(subject) != nil {
		return domain.ReferenceCandidateBundle{}, errors.New("candidate Bundle review content changed")
	}
	snapshot, err := loadReferenceBundleSnapshot(ctx, tx, subject.WorkspaceID, subject.ProjectID, subject.ExecutionRef.ID)
	if err != nil {
		return domain.ReferenceCandidateBundle{}, err
	}
	if subject.CandidateBundleIndex < 0 || subject.CandidateBundleIndex >= len(snapshot.bundles.Bundles) {
		return domain.ReferenceCandidateBundle{}, errors.New("candidate Bundle index changed")
	}
	bundle := snapshot.bundles.Bundles[subject.CandidateBundleIndex].Input
	if subject.BundleInputRef != (domain.GenerationActionRef{ID: bundle.ID, ContentHash: bundle.ContentHash}) || subject.TargetRef != bundle.TargetRef || subject.GenerationRound != bundle.GenerationRound {
		return domain.ReferenceCandidateBundle{}, errors.New("candidate Bundle input changed")
	}
	// The Bundle is a mechanical projection of this accepted review. Use its
	// persisted logical time, never the retry's wall clock, for content identity.
	return domain.BuildReferenceCandidateBundle(subject.WorkspaceID, subject.ProjectID, snapshot.bundles, subject.CandidateBundleIndex, command.VisionReviewRef, state.Candidate.CreatedAt)
}

func (store *ReferenceCandidateBundleStore) MaterializeReferenceCandidateBundle(ctx context.Context, actor app.Actor, command app.ReferenceCandidateBundleCommand) (domain.ReferenceCandidateBundle, error) {
	var value domain.ReferenceCandidateBundle
	err := store.within(ctx, func(tx *gorm.DB) error {
		repo := repository{database: tx}
		if err := repo.AuthorizeProject(ctx, actor, command.WorkspaceID, command.ProjectID, true); err != nil {
			return err
		}
		var err error
		value, err = store.current(ctx, tx, command)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		row := model.GenerationReferenceCandidateBundle{ID: uuid.MustParse(value.ID), WorkspaceID: uuid.MustParse(value.WorkspaceID), ProjectID: uuid.MustParse(value.ProjectID), ExecutionID: uuid.MustParse(value.ExecutionRef.ID), VisionCandidateID: uuid.MustParse(value.VisionReviewRef.ID), ContentHash: value.ContentHash, Content: datatypes.JSON(raw), CreatedAt: value.CreatedAt}
		if err := tx.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var persisted model.GenerationReferenceCandidateBundle
		if err := tx.WithContext(ctx).First(&persisted, "id = ?", value.ID).Error; err != nil {
			return err
		}
		checked, err := decodeReferenceCandidateBundleRow(persisted)
		if err != nil || !reflect.DeepEqual(checked, value) {
			return errors.New("candidate Bundle persisted identity conflicts")
		}
		return nil
	})
	if err != nil {
		return domain.ReferenceCandidateBundle{}, err
	}
	return value, nil
}

func (store *ReferenceCandidateBundleStore) ReadReferenceCandidateBundle(ctx context.Context, actor app.Actor, projectID, bundleID string) (domain.ReferenceCandidateBundle, error) {
	var value domain.ReferenceCandidateBundle
	err := store.within(ctx, func(tx *gorm.DB) error {
		repo := repository{database: tx}
		if err := repo.AuthorizeProject(ctx, actor, "", projectID, false); err != nil {
			return err
		}
		var row model.GenerationReferenceCandidateBundle
		if err := tx.WithContext(ctx).Where("id = ? AND project_id = ?", bundleID, projectID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound("Reference candidate Bundle not found")
			}
			return err
		}
		var err error
		value, err = decodeReferenceCandidateBundleRow(row)
		if err != nil {
			return err
		}
		current, err := store.current(ctx, tx, app.ReferenceCandidateBundleCommand{WorkspaceID: value.WorkspaceID, ProjectID: projectID, ExecutionRef: value.ExecutionRef, VisionReviewRef: value.VisionReviewRef})
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(current, value) {
			return errors.New("candidate Bundle no longer matches current facts")
		}
		return nil
	})
	if err != nil {
		return domain.ReferenceCandidateBundle{}, err
	}
	return value, nil
}

func decodeReferenceCandidateBundleRow(row model.GenerationReferenceCandidateBundle) (domain.ReferenceCandidateBundle, error) {
	v, err := domain.DecodeReferenceCandidateBundle(json.RawMessage(row.Content))
	if err != nil || v.ID != row.ID.String() || v.WorkspaceID != row.WorkspaceID.String() || v.ProjectID != row.ProjectID.String() || v.ExecutionRef.ID != row.ExecutionID.String() || v.VisionReviewRef.ID != row.VisionCandidateID.String() || v.ContentHash != row.ContentHash || !v.CreatedAt.Equal(row.CreatedAt) {
		return domain.ReferenceCandidateBundle{}, errors.New("candidate Bundle columns differ from content")
	}
	return v, nil
}

var _ app.ReferenceCandidateBundlePersistence = (*ReferenceCandidateBundleStore)(nil)
