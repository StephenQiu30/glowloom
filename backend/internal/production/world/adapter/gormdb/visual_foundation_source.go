package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/world/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

type visualFoundationSourceRepository struct {
	database *gorm.DB
}

func NewVisualFoundationSourceRepository(database *gorm.DB) application.VisualFoundationSourceRepository {
	return &visualFoundationSourceRepository{database: database}
}

func (repository *visualFoundationSourceRepository) GetCurrentVisualFoundationSource(
	ctx context.Context,
	workspaceID, projectID string,
) (domain.ConfirmedVisualFoundationSource, error) {
	if repository == nil || repository.database == nil {
		return domain.ConfirmedVisualFoundationSource{}, application.ErrVisualFoundationSourceUnavailable
	}
	head, version, err := biblegorm.NewProductionWorldRepository(repository.database).
		GetProductionWorldBibleHead(ctx, workspaceID, projectID, false)
	if errors.Is(err, bibleapp.ErrProductionWorldBibleHeadNotFound) {
		return domain.ConfirmedVisualFoundationSource{}, application.ErrVisualFoundationSourceUnavailable
	}
	if err != nil {
		return domain.ConfirmedVisualFoundationSource{}, err
	}
	candidateID, err := uuid.Parse(version.Candidate.VersionID)
	if err != nil || candidateID == uuid.Nil {
		return domain.ConfirmedVisualFoundationSource{}, errors.New("Production World Candidate identity has drifted")
	}
	var record model.StageCandidateRevision
	if err = repository.database.WithContext(ctx).First(&record, "id = ?", candidateID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ConfirmedVisualFoundationSource{}, application.ErrVisualFoundationSourceUnavailable
		}
		return domain.ConfirmedVisualFoundationSource{}, err
	}
	if record.WorkspaceID.String() != workspaceID || record.ID.String() != version.Candidate.VersionID ||
		record.RevisionNo != version.Candidate.Revision || record.CandidateRevisionHash != version.Candidate.ContentHash ||
		record.OriginKind != "aggregate" || record.ParentCandidateRevisionID != nil ||
		record.ParentCandidateRevisionHash != nil || record.InvocationOrigin != nil || record.RepairOrigin != nil ||
		record.SourceInvocationID != nil || record.SourceResultHash != nil {
		return domain.ConfirmedVisualFoundationSource{}, errors.New("Production World Candidate revision has drifted")
	}
	var origin agentcontract.AggregateCandidateOrigin
	if err = json.Unmarshal(record.AggregateOrigin, &origin); err != nil {
		return domain.ConfirmedVisualFoundationSource{}, errors.New("Production World Candidate origin has drifted")
	}
	revisionHash, err := (agentcontract.CandidateRevisionMaterial{
		StageInstanceKey: record.StageInstanceKey, RevisionNo: record.RevisionNo,
		OriginKind: "aggregate", AggregateOrigin: &origin, CandidateContentHash: record.CandidateContentHash,
	}).Hash()
	if err != nil || revisionHash != record.CandidateRevisionHash {
		return domain.ConfirmedVisualFoundationSource{}, errors.New("Production World Candidate revision proof has drifted")
	}
	candidate, _, err := domain.DecodeProductionWorldCandidate(json.RawMessage(record.Candidate))
	if err != nil {
		return domain.ConfirmedVisualFoundationSource{}, err
	}
	return domain.NewConfirmedVisualFoundationSource(
		version, head.CollectionRootHash, record.ID.String(), record.RevisionNo,
		record.CandidateRevisionHash, record.CandidateContentHash, candidate,
	)
}
