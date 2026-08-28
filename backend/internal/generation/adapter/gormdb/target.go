package gormdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type TargetStore struct{ database *gorm.DB }

func NewTargetStore(database *gorm.DB) *TargetStore { return &TargetStore{database: database} }

func (store *TargetStore) Ensure(ctx context.Context, desired domain.GenerationTarget) (domain.GenerationTarget, error) {
	if store == nil || store.database == nil {
		return domain.GenerationTarget{}, errors.New("GenerationTarget store is not configured")
	}
	record, err := generationTargetRecord(desired)
	if err != nil {
		return domain.GenerationTarget{}, err
	}
	if err = store.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}}, DoNothing: true,
	}).Create(&record).Error; err != nil {
		return domain.GenerationTarget{}, fmt.Errorf("ensure GenerationTarget: %w", err)
	}
	persisted, err := findGenerationTarget(ctx, store.database, desired.ID)
	if err != nil {
		return domain.GenerationTarget{}, err
	}
	if !domain.SameGenerationTarget(persisted, desired) {
		return domain.GenerationTarget{}, errors.New("GenerationTarget identity is bound to different facts")
	}
	return persisted, nil
}

func (store *TargetStore) Find(ctx context.Context, targetID string) (domain.GenerationTarget, error) {
	if store == nil || store.database == nil {
		return domain.GenerationTarget{}, errors.New("GenerationTarget store is not configured")
	}
	return findGenerationTarget(ctx, store.database, targetID)
}

func findGenerationTarget(ctx context.Context, database *gorm.DB, targetID string) (domain.GenerationTarget, error) {
	id, err := uuid.Parse(targetID)
	if err != nil {
		return domain.GenerationTarget{}, application.ErrGenerationTargetNotFound
	}
	var record model.GenerationTarget
	if err = database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.GenerationTarget{}, application.ErrGenerationTargetNotFound
		}
		return domain.GenerationTarget{}, err
	}
	target, err := generationTargetDomain(record)
	if err != nil || domain.ValidateGenerationTarget(target) != nil {
		return domain.GenerationTarget{}, errors.New("persisted GenerationTarget has drifted")
	}
	return target, nil
}

func generationTargetRecord(value domain.GenerationTarget) (model.GenerationTarget, error) {
	if err := domain.ValidateGenerationTarget(value); err != nil {
		return model.GenerationTarget{}, err
	}
	ids := make([]uuid.UUID, 4)
	for index, raw := range []string{value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy} {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return model.GenerationTarget{}, err
		}
		ids[index] = parsed
	}
	source, err := json.Marshal(value.SourceOwnerRef)
	if err != nil {
		return model.GenerationTarget{}, err
	}
	policy, err := json.Marshal(value.PolicySnapshotRef)
	if err != nil {
		return model.GenerationTarget{}, err
	}
	var payload any
	if value.Kind == domain.GenerationTargetReferenceAsset {
		payload = value.ReferenceAsset
	} else {
		payload = value.ShotFrame
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return model.GenerationTarget{}, err
	}
	return model.GenerationTarget{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], Kind: value.Kind,
		SourceOwnerRef: source, SourceContentHash: value.SourceOwnerRef.ContentHash,
		PolicySnapshotRef: policy, PolicyContentHash: value.PolicySnapshotRef.ContentHash,
		Payload: encodedPayload, TargetHash: value.TargetHash, Revision: value.Revision,
		CreatedBy: ids[3], CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func generationTargetDomain(value model.GenerationTarget) (domain.GenerationTarget, error) {
	var source, policy domain.FrozenOwnerReference
	if err := decodeStrictTargetJSON(value.SourceOwnerRef, &source); err != nil {
		return domain.GenerationTarget{}, err
	}
	if err := decodeStrictTargetJSON(value.PolicySnapshotRef, &policy); err != nil {
		return domain.GenerationTarget{}, err
	}
	input := domain.GenerationTargetInput{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(), Kind: value.Kind,
		SourceOwnerRef: source, PolicySnapshotRef: policy, Revision: value.Revision,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}
	switch value.Kind {
	case domain.GenerationTargetReferenceAsset:
		var payload domain.ReferenceAssetTarget
		if err := decodeStrictTargetJSON(value.Payload, &payload); err != nil {
			return domain.GenerationTarget{}, err
		}
		input.ReferenceAsset = &payload
	case domain.GenerationTargetShotFrame:
		var payload domain.ShotFrameTarget
		if err := decodeStrictTargetJSON(value.Payload, &payload); err != nil {
			return domain.GenerationTarget{}, err
		}
		input.ShotFrame = &payload
	default:
		return domain.GenerationTarget{}, errors.New("persisted GenerationTarget kind is invalid")
	}
	target, err := domain.NewGenerationTarget(input)
	if err != nil || target.TargetHash != value.TargetHash || source.ContentHash != value.SourceContentHash ||
		policy.ContentHash != value.PolicyContentHash {
		return domain.GenerationTarget{}, errors.New("persisted GenerationTarget snapshot has drifted")
	}
	return target, nil
}

func decodeStrictTargetJSON(encoded []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("GenerationTarget JSON contains trailing data")
	}
	return nil
}
