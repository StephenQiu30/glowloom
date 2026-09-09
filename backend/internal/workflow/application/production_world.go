package application

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

const ProductionWorldAssemblyStage = "assemble_production_world"

type ProductionWorldAssemblyCommand struct {
	WorkflowRunID string
	NodeRunID     string
	InputHash     string
	Draft         worlddomain.ProductionWorldCandidateDraft
	Leaves        []agentcontract.AggregateLeafCandidateRef
}

type ProductionWorldCandidateRecord struct {
	ManifestID           string
	CandidateRevisionID  string
	WorkspaceID          string
	ProjectID            string
	WorkflowRunID        string
	NodeRunID            string
	Stage                string
	RootInputHash        string
	Candidate            json.RawMessage
	CandidateContentHash string
	Leaves               []agentcontract.AggregateLeafCandidateRef
	CreatedAt            time.Time
}

type ProductionWorldCandidateRevision struct {
	ID                    string
	StageInstanceKey      string
	Revision              int64
	Candidate             json.RawMessage
	CandidateContentHash  string
	CandidateRevisionHash string
}

type ProductionWorldCandidateStore interface {
	EnsureProductionWorldCandidate(context.Context, ProductionWorldCandidateRecord) (ProductionWorldCandidateRevision, error)
}

type ProductionWorldAssembler interface {
	AssembleProductionWorld(context.Context, ProductionWorldAssemblyCommand) (ProductionWorldCandidateRevision, error)
}

type ProductionWorldAssemblyConfig struct {
	Now   func() time.Time
	NewID func() string
}

type ProductionWorldAssemblyService struct {
	store  ProductionWorldCandidateStore
	config ProductionWorldAssemblyConfig
}

func NewProductionWorldAssemblyService(
	store ProductionWorldCandidateStore,
	config ProductionWorldAssemblyConfig,
) (*ProductionWorldAssemblyService, error) {
	if store == nil {
		return nil, errors.New("Production World Candidate store is required")
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.NewID == nil {
		config.NewID = uuid.NewString
	}
	return &ProductionWorldAssemblyService{store: store, config: config}, nil
}

func (service *ProductionWorldAssemblyService) AssembleProductionWorld(
	ctx context.Context,
	command ProductionWorldAssemblyCommand,
) (ProductionWorldCandidateRevision, error) {
	if service == nil || service.store == nil || !productionWorldHash(command.InputHash) {
		return ProductionWorldCandidateRevision{}, errors.New("invalid Production World assembly command")
	}
	for _, identifier := range []string{command.WorkflowRunID, command.NodeRunID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return ProductionWorldCandidateRevision{}, errors.New("invalid Production World assembly command")
		}
	}
	candidate, candidateJSON, err := worlddomain.NewProductionWorldCandidate(command.Draft)
	if err != nil {
		return ProductionWorldCandidateRevision{}, err
	}
	leaves := append([]agentcontract.AggregateLeafCandidateRef(nil), command.Leaves...)
	slices.SortFunc(leaves, func(left, right agentcontract.AggregateLeafCandidateRef) int {
		if left.StageInstanceKey != right.StageInstanceKey {
			return strings.Compare(left.StageInstanceKey, right.StageInstanceKey)
		}
		return strings.Compare(left.ShardKey, right.ShardKey)
	})
	if !validProductionWorldLeaves(candidate, leaves) {
		return ProductionWorldCandidateRevision{}, errors.New("Production World aggregate leaves drifted")
	}
	return service.store.EnsureProductionWorldCandidate(ctx, ProductionWorldCandidateRecord{
		ManifestID: service.config.NewID(), CandidateRevisionID: service.config.NewID(),
		WorkspaceID: candidate.WorkspaceID, ProjectID: candidate.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		Stage: ProductionWorldAssemblyStage, RootInputHash: command.InputHash,
		Candidate: candidateJSON, CandidateContentHash: candidate.ContentHash,
		Leaves: leaves, CreatedAt: service.config.Now().UTC(),
	})
}

func validProductionWorldLeaves(value worlddomain.ProductionWorldCandidate, leaves []agentcontract.AggregateLeafCandidateRef) bool {
	if len(leaves) != 3 {
		return false
	}
	expected := map[string]agentcontract.SceneAnalysisCandidateRevisionIdentity{
		value.UpstreamCandidates.ProductionEntity.CandidateRevisionID:      value.UpstreamCandidates.ProductionEntity,
		value.UpstreamCandidates.SceneOccurrence.CandidateRevisionID:       value.UpstreamCandidates.SceneOccurrence,
		value.UpstreamCandidates.InteractionContinuity.CandidateRevisionID: value.UpstreamCandidates.InteractionContinuity,
	}
	for _, leaf := range leaves {
		candidate, exists := expected[leaf.CandidateRevisionID]
		if !exists || leaf.ShardKey != candidate.ShardKey ||
			leaf.CandidateRevisionHash != candidate.CandidateRevisionHash || !productionWorldHash(leaf.StageInstanceKey) {
			return false
		}
		delete(expected, leaf.CandidateRevisionID)
	}
	return len(expected) == 0
}

func productionWorldHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
