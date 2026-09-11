package gormdb

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func prepareProductionWorldGateInput(
	database *gorm.DB,
	run model.WorkflowRun,
	node model.NodeRunProjection,
	input domain.NodeInputSnapshot,
	now time.Time,
) (model.WorkflowHumanGateInput, domain.ProductionWorldGateInput, []string, error) {
	binding, err := productionWorldGateBinding(input)
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.ProductionWorldGateInput{}, nil, err
	}
	revision, candidate, err := loadProductionWorldGateCandidate(database, run, binding)
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.ProductionWorldGateInput{}, nil, err
	}
	if err = validateProductionWorldFormalReadSet(database, run, candidate); err != nil {
		return model.WorkflowHumanGateInput{}, domain.ProductionWorldGateInput{}, nil, err
	}
	expectedHeads := productionWorldInitialExpectedHeads(candidate)
	gate, encoded, err := domain.NewProductionWorldGateInput(domain.ProductionWorldGateInputDraft{
		WorkspaceID: run.WorkspaceID.String(), ProjectID: run.ProjectID.String(),
		WorkflowRunID: run.ID.String(), NodeRunID: node.ID.String(),
		CandidateRevisionID: revision.ID.String(), CandidateRevision: revision.RevisionNo,
		CandidateRevisionHash: revision.CandidateRevisionHash, Candidate: candidate,
		AllowedDecisions: []string{"approved", "changes_requested", "rejected"}, ExpectedHeads: expectedHeads,
	})
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.ProductionWorldGateInput{}, nil, err
	}
	record := model.WorkflowHumanGateInput{
		ID: uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte("lanverse:production-world-gate-input:"+node.ID.String()+":"+gate.InputHash),
		),
		WorkspaceID: run.WorkspaceID, ProjectID: run.ProjectID, WorkflowRunID: run.ID, NodeRunID: node.ID,
		GateKey: gate.GateKey, GateInstanceKey: gate.GateInstanceKey, SubjectType: gate.SubjectType,
		SubjectHash: gate.SubjectHash, EffectPlanHash: gate.EffectPlanHash, InputHash: gate.InputHash,
		Input: datatypes.JSON(encoded), CreatedAt: now.UTC(),
	}
	var existing model.WorkflowHumanGateInput
	loadErr := database.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "node_run_id = ?", node.ID).Error
	if loadErr == nil {
		if !sameProductionWorldGateInput(existing, record) {
			return model.WorkflowHumanGateInput{}, domain.ProductionWorldGateInput{}, nil, errors.New("Production World Gate input has drifted")
		}
		return existing, gate, productionWorldCandidateIDs(gate, candidate), nil
	}
	if !errors.Is(loadErr, gorm.ErrRecordNotFound) {
		return model.WorkflowHumanGateInput{}, domain.ProductionWorldGateInput{}, nil, loadErr
	}
	if err = database.Omit(clause.Associations).Create(&record).Error; err != nil {
		return model.WorkflowHumanGateInput{}, domain.ProductionWorldGateInput{}, nil, err
	}
	return record, gate, productionWorldCandidateIDs(gate, candidate), nil
}

func productionWorldInitialExpectedHeads(candidate worlddomain.ProductionWorldCandidate) []domain.ProductionWorldExpectedHead {
	result := []domain.ProductionWorldExpectedHead{
		{OwnerKind: "asset", VersionFamily: "asset_identity_state_set", ScopeKind: "project", ScopeKey: "project:" + candidate.ProjectID},
		{OwnerKind: "production/bible", VersionFamily: "bible_production_world_set", ScopeKind: "project", ScopeKey: "project:" + candidate.ProjectID},
	}
	for _, episode := range candidate.SharedProof.PlanningEpisodeScopes {
		result = append(result, domain.ProductionWorldExpectedHead{
			OwnerKind: "production/planning", VersionFamily: "planning_scene_set",
			ScopeKind: "episode", ScopeKey: episode.ScopeKey,
		})
	}
	return append(result, domain.ProductionWorldExpectedHead{
		OwnerKind: "production/planning", VersionFamily: "planning_structure_rebase_set",
		ScopeKind: "project", ScopeKey: "project:" + candidate.ProjectID,
	})
}

func productionWorldGateBinding(input domain.NodeInputSnapshot) (domain.NodeInputBinding, error) {
	if len(input.Bindings) != 1 {
		return domain.NodeInputBinding{}, errors.New("workflow Production World Human Gate has an incomplete read set")
	}
	binding := input.Bindings[0]
	if binding.Port != "candidate" || binding.ValueType != "production_world_candidate" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "candidate" {
		return domain.NodeInputBinding{}, errors.New("workflow Production World Human Gate input has drifted")
	}
	return binding, nil
}

func loadProductionWorldGateCandidate(
	database *gorm.DB,
	run model.WorkflowRun,
	binding domain.NodeInputBinding,
) (model.StageCandidateRevision, worlddomain.ProductionWorldCandidate, error) {
	candidateID, err := uuid.Parse(binding.ReferenceID)
	if err != nil {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate Candidate identity is invalid")
	}
	var revision model.StageCandidateRevision
	if err = database.First(&revision, "id = ?", candidateID).Error; err != nil {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, normalizeNotFound(err)
	}
	candidate, _, err := worlddomain.DecodeProductionWorldCandidate(json.RawMessage(revision.Candidate))
	if err != nil || candidate.WorkspaceID != run.WorkspaceID.String() || candidate.ProjectID != run.ProjectID.String() ||
		revision.WorkspaceID != run.WorkspaceID || revision.OriginKind != "aggregate" || revision.RevisionNo != 1 ||
		binding.ReferenceVersion != strconv.FormatInt(revision.RevisionNo, 10) ||
		binding.ContentHash != revision.CandidateRevisionHash || revision.CandidateContentHash != candidate.ContentHash {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate Candidate has drifted")
	}
	var origin agentcontract.AggregateCandidateOrigin
	if json.Unmarshal(revision.AggregateOrigin, &origin) != nil {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate aggregate origin is invalid")
	}
	expectedRevisionHash, err := (agentcontract.CandidateRevisionMaterial{
		StageInstanceKey: revision.StageInstanceKey, RevisionNo: revision.RevisionNo,
		OriginKind: "aggregate", AggregateOrigin: &origin, CandidateContentHash: revision.CandidateContentHash,
	}).Hash()
	if err != nil || expectedRevisionHash != revision.CandidateRevisionHash {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate Candidate Revision has drifted")
	}
	var head model.StageCandidateHead
	if err = database.First(&head, "stage_instance_key = ?", revision.StageInstanceKey).Error; err != nil {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, normalizeNotFound(err)
	}
	if head.WorkspaceID != run.WorkspaceID || head.CurrentRevisionID != revision.ID ||
		head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash || head.Revision != revision.RevisionNo {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate Candidate Head has drifted")
	}
	manifestID, parseErr := uuid.Parse(origin.ShardManifestID)
	if parseErr != nil {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate Manifest identity is invalid")
	}
	var manifest model.ShardManifest
	if err = database.First(&manifest, "id = ? AND version = ?", manifestID, origin.ManifestVersion).Error; err != nil {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, normalizeNotFound(err)
	}
	if manifest.WorkspaceID != run.WorkspaceID || manifest.WorkflowRunID != run.ID ||
		manifest.Stage != application.ProductionWorldAssemblyStage ||
		manifest.ManifestHash != origin.ShardManifestHash {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate Manifest has drifted")
	}
	var sourceNode model.NodeRunProjection
	if err = database.First(&sourceNode, "id = ?", manifest.NodeRunID).Error; err != nil {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, normalizeNotFound(err)
	}
	if sourceNode.WorkflowRunID != run.ID || sourceNode.NodeID != binding.SourceNodeID ||
		sourceNode.DefinitionKey != "production.production_world_assembly" ||
		sourceNode.Executor != "activity.production_world_assembly" {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate source Node has drifted")
	}
	sourceResult, resultErr := completedNodeResult(sourceNode)
	if resultErr != nil || len(sourceResult.Output.Bindings) != 1 ||
		sourceResult.Output.Bindings[0].Port != "candidate" ||
		sourceResult.Output.Bindings[0].ValueType != "production_world_candidate" ||
		sourceResult.Output.Bindings[0].ReferenceID != binding.ReferenceID ||
		sourceResult.Output.Bindings[0].ReferenceVersion != binding.ReferenceVersion ||
		sourceResult.Output.Bindings[0].ContentHash != binding.ContentHash {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate source output has drifted")
	}
	manifestRecord := application.ProductionWorldCandidateRecord{
		ManifestID: manifest.ID.String(), CandidateRevisionID: revision.ID.String(),
		WorkspaceID: run.WorkspaceID.String(), ProjectID: run.ProjectID.String(),
		WorkflowRunID: manifest.WorkflowRunID.String(), NodeRunID: manifest.NodeRunID.String(),
		Stage: manifest.Stage, RootInputHash: manifest.RootInputHash,
		Candidate: json.RawMessage(revision.Candidate), CandidateContentHash: revision.CandidateContentHash,
		Leaves: origin.LeafCandidates, CreatedAt: manifest.CreatedAt,
	}
	identities, parseErr := parseProductionWorldRecordIdentities(manifestRecord)
	if parseErr != nil {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, parseErr
	}
	expectedManifest, buildErr := buildProductionWorldManifest(manifestRecord, identities, manifest.ID)
	var persistedLeaves []agentcontract.AggregateLeafCandidateRef
	if buildErr != nil || json.Unmarshal(manifest.Shards, &persistedLeaves) != nil ||
		manifest.CoverageHash != expectedManifest.CoverageHash || manifest.ManifestHash != expectedManifest.ManifestHash ||
		!reflect.DeepEqual(persistedLeaves, origin.LeafCandidates) {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, errors.New("Production World Gate Manifest proof has drifted")
	}
	if err = validateProductionWorldLeafHeads(database, candidate, origin.LeafCandidates, run.WorkspaceID, run.ProjectID); err != nil {
		return model.StageCandidateRevision{}, worlddomain.ProductionWorldCandidate{}, err
	}
	return revision, candidate, nil
}

func validateProductionWorldFormalReadSet(
	database *gorm.DB,
	run model.WorkflowRun,
	candidate worlddomain.ProductionWorldCandidate,
) error {
	versionID, err := uuid.Parse(candidate.SourceVersion.VersionID)
	if err != nil {
		return errors.New("Production World Gate Source identity is invalid")
	}
	var revision model.DocumentRevision
	if err = database.First(&revision, "id = ?", versionID).Error; err != nil {
		return normalizeNotFound(err)
	}
	var document model.ScriptDocument
	if err = database.First(&document, "id = ?", revision.DocumentID).Error; err != nil {
		return normalizeNotFound(err)
	}
	var sourceHead model.ScriptSourceScopeHead
	if err = database.First(&sourceHead, "project_id = ?", run.ProjectID).Error; err != nil {
		return normalizeNotFound(err)
	}
	expectedSource := agentcontract.ScriptSourceVersionIdentity{
		OwnerKind: "production/script", LogicalID: document.ID.String(), VersionID: revision.ID.String(),
		Revision: int64(revision.VersionNo), ContentHash: revision.NormalizedHash, CreatedAt: revision.CreatedAt.UTC(),
	}
	if revision.WorkspaceID != run.WorkspaceID || document.WorkspaceID != run.WorkspaceID ||
		document.ProjectID != run.ProjectID || candidate.SourceVersion != expectedSource ||
		sourceHead.WorkspaceID != run.WorkspaceID || sourceHead.CurrentDocumentRevisionID != revision.ID ||
		sourceHead.DocumentLogicalID != document.ID {
		return errors.New("Production World Gate Source Head has drifted")
	}
	identityVersionID, err := uuid.Parse(candidate.StructureIdentitySetVersion.VersionID)
	if err != nil {
		return errors.New("Production World Gate StructureIdentitySet identity is invalid")
	}
	var identityVersion model.StructureIdentitySetVersion
	if err = database.First(&identityVersion, "id = ?", identityVersionID).Error; err != nil {
		return normalizeNotFound(err)
	}
	var identityHead model.StructureIdentityScopeHead
	if err = database.First(&identityHead, "project_id = ?", run.ProjectID).Error; err != nil {
		return normalizeNotFound(err)
	}
	identityCollection, buildErr := bibledomain.BuildStructureIdentityCollection(bibledomain.StructureIdentitySetVersion{
		SchemaVersion: bibledomain.StructureIdentitySetSchemaVersion,
		ID:            identityVersion.ID.String(), WorkspaceID: identityVersion.WorkspaceID.String(), ProjectID: identityVersion.ProjectID.String(),
		Version: identityVersion.Version, ContentHash: identityVersion.ContentHash,
	})
	var identityHeadRefs []ownercollection.VersionRef
	if json.Unmarshal(identityHead.CurrentVersionRefs, &identityHeadRefs) != nil {
		return errors.New("Production World Gate StructureIdentitySet Head refs have drifted")
	}
	rebuiltIdentityHead, headErr := bibledomain.NewStructureIdentityScopeHead(
		identityCollection,
		identityVersion.ID.String(),
		identityHead.HeadRevision,
		identityHead.UpdatedAt,
	)
	if identityVersion.WorkspaceID != run.WorkspaceID || identityVersion.ProjectID != run.ProjectID ||
		candidate.StructureIdentitySetVersion.Revision != int64(identityVersion.Version) ||
		candidate.StructureIdentitySetVersion.ContentHash != identityVersion.ContentHash ||
		identityHead.WorkspaceID != run.WorkspaceID || identityHead.CurrentVersionID != identityVersion.ID ||
		buildErr != nil || headErr != nil || identityHead.ScopeKey != rebuiltIdentityHead.ScopeKey ||
		identityHead.ScopeRevision != rebuiltIdentityHead.ScopeRevision ||
		identityHead.ScopeContentHash != rebuiltIdentityHead.ScopeContentHash ||
		identityHead.MemberCount != rebuiltIdentityHead.MemberCount ||
		identityHead.MembersHash != rebuiltIdentityHead.MembersHash ||
		identityHead.CollectionRootHash != rebuiltIdentityHead.CollectionRootHash ||
		identityHead.HeadContentHash != rebuiltIdentityHead.HeadContentHash ||
		!reflect.DeepEqual(identityHeadRefs, rebuiltIdentityHead.CurrentVersionRefs) {
		return errors.New("Production World Gate StructureIdentitySet Head has drifted")
	}
	return nil
}

func sameProductionWorldGateInput(left, right model.WorkflowHumanGateInput) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.WorkflowRunID == right.WorkflowRunID && left.NodeRunID == right.NodeRunID && left.GateKey == right.GateKey &&
		left.GateInstanceKey == right.GateInstanceKey && left.SubjectType == right.SubjectType &&
		left.SubjectHash == right.SubjectHash && left.EffectPlanHash == right.EffectPlanHash &&
		left.InputHash == right.InputHash && equalJSON(left.Input, right.Input)
}

func resolveProductionWorldOwnerMaterial(
	database *gorm.DB,
	run model.WorkflowRun,
	node model.NodeRunProjection,
	task model.HumanTask,
	input domain.NodeInputSnapshot,
) (json.RawMessage, error) {
	var record model.WorkflowHumanGateInput
	if err := database.First(&record, "node_run_id = ?", node.ID).Error; err != nil {
		return nil, normalizeNotFound(err)
	}
	gate, _, err := domain.DecodeProductionWorldGateInput(json.RawMessage(record.Input))
	if err != nil || record.WorkspaceID != run.WorkspaceID || record.ProjectID != run.ProjectID ||
		record.WorkflowRunID != run.ID || record.NodeRunID != node.ID || gate.InputHash != record.InputHash ||
		task.SubjectType != "production_world_gate_input" || task.SubjectID != record.ID ||
		task.SubjectRevision != 1 || task.SubjectHash != record.InputHash {
		return nil, errors.New("Production World owner Gate input has drifted")
	}
	binding, err := productionWorldGateBinding(input)
	if err != nil {
		return nil, err
	}
	revision, candidate, err := loadProductionWorldGateCandidate(database, run, binding)
	if err != nil {
		return nil, err
	}
	if err = validateProductionWorldFormalReadSet(database, run, candidate); err != nil {
		return nil, err
	}
	frozen := gate.Subject.ProductionWorldCandidate
	if frozen.CandidateRevisionID != revision.ID.String() || frozen.CandidateRevision != revision.RevisionNo ||
		frozen.CandidateRevisionHash != revision.CandidateRevisionHash ||
		frozen.CandidateContentHash != revision.CandidateContentHash {
		return nil, errors.New("Production World owner Candidate revision has drifted")
	}
	actualCandidateIDs, err := humanTaskCandidateIDs(task.CandidateIDs)
	if err != nil {
		return nil, errors.New("Production World owner Candidate set is invalid")
	}
	wantCandidateIDs := productionWorldCandidateIDs(gate, candidate)
	slices.Sort(actualCandidateIDs)
	if !slices.Equal(actualCandidateIDs, wantCandidateIDs) {
		return nil, errors.New("Production World owner Candidate set has drifted")
	}
	material := domain.ProductionWorldOwnerMaterial{
		SchemaVersion: domain.ProductionWorldOwnerMaterialSchema,
		GateInputID:   record.ID.String(),
		GateInput:     gate,
		Candidate:     candidate,
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		return nil, err
	}
	if _, err = domain.DecodeProductionWorldOwnerMaterial(encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

func productionWorldCandidateIDs(
	value domain.ProductionWorldGateInput,
	candidate worlddomain.ProductionWorldCandidate,
) []string {
	result := []string{
		value.Subject.ProductionWorldCandidate.CandidateRevisionID,
		candidate.UpstreamCandidates.ProductionEntity.CandidateRevisionID,
		value.Subject.SceneOccurrenceCandidate.CandidateRevisionID,
		value.Subject.InteractionCandidate.Candidate.CandidateRevisionID,
	}
	for index := range result {
		result[index] = strings.TrimSpace(result[index])
	}
	slices.Sort(result)
	return slices.Compact(result)
}
