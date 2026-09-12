package gormdb

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	presetgorm "github.com/StephenQiu30/lanverse/backend/internal/preset/adapter/gormdb"
	presetcatalog "github.com/StephenQiu30/lanverse/backend/internal/preset/catalog"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func prepareVisualFoundationScopeGateInput(
	database *gorm.DB,
	run model.WorkflowRun,
	node model.NodeRunProjection,
	input domain.NodeInputSnapshot,
	now time.Time,
) (model.WorkflowHumanGateInput, domain.VisualFoundationScopeGateInput, []string, error) {
	bindings, err := visualFoundationScopeGateBindings(input)
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, err
	}
	ctx := database.Statement.Context
	world, err := currentReferencePlanWorld(ctx, database, run.WorkspaceID.String(), run.ProjectID.String())
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, err
	}
	if bindings["storygraph"].ReferenceID != world.StoryGraphVersionID ||
		bindings["storygraph"].ContentHash != world.StoryGraphContentHash {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, errors.New("Visual Foundation Scope Gate StoryGraph has drifted")
	}
	var storyGraphVersion model.StoryGraphVersion
	if err = database.First(&storyGraphVersion, "id = ?", world.StoryGraphVersionID).Error; err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, normalizeNotFound(err)
	}
	if storyGraphVersion.WorkspaceID != run.WorkspaceID || storyGraphVersion.ProjectID != run.ProjectID ||
		bindings["storygraph"].ReferenceVersion != strconv.FormatInt(storyGraphVersion.VersionNo, 10) {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, errors.New("Visual Foundation Scope Gate StoryGraph version has drifted")
	}
	selection, err := presetgorm.NewProjectSelectionStore(database).Current(
		ctx, run.WorkspaceID.String(), run.ProjectID.String(),
	)
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, err
	}
	if bindings["selection"].ReferenceID != selection.ID ||
		bindings["selection"].ReferenceVersion != strconv.FormatInt(selection.Revision, 10) ||
		bindings["selection"].ContentHash != selection.ContentHash {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, errors.New("Visual Foundation Scope Gate Preset selection has drifted")
	}
	release, found, err := presetcatalog.FindCuratedRelease(selection.PresetRelease.Key, selection.PresetRelease.Release)
	if err != nil || !found || release.ContentHash != selection.PresetRelease.ContentHash {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, errors.New("Visual Foundation Scope Gate Preset release is unavailable")
	}
	visual, err := exactReferencePlanVisualFoundationCandidate(
		ctx, database, run.WorkspaceID.String(), run.ProjectID.String(),
		bindings["visual_foundation"].ReferenceID, bindings["visual_foundation"].ContentHash,
	)
	if err != nil || bindings["visual_foundation"].ReferenceVersion != strconv.FormatInt(visual.Revision, 10) {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, errors.New("Visual Foundation Scope Gate Visual Candidate has drifted")
	}
	if err = validateVisualFoundationScopeCandidateSource(
		database, run, bindings["visual_foundation"], agentcontract.VisualFoundationStageKey,
	); err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, err
	}
	reference, referenceInput, projection, err := loadVisualFoundationScopeReferenceCandidate(
		database, run, bindings["reference_plan"],
	)
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, err
	}
	subject, _, err := domain.NewVisualFoundationScopeSubject(domain.VisualFoundationScopeSubjectDraft{
		ConfirmedProductionWorld: world, ProjectPresetSelection: selection, PresetRelease: release,
		VisualFoundationCandidate: domain.VisualFoundationScopeCandidateRevisionMaterial{
			RevisionID: visual.ID, Revision: visual.Revision, RevisionHash: visual.RevisionHash,
			ContentHash: visual.CandidateContentHash, Candidate: json.RawMessage(visual.Candidate),
		},
		ReferencePlanInput: referenceInput,
		ReferencePlanCandidate: domain.VisualFoundationScopeCandidateRevisionMaterial{
			RevisionID: reference.ID.String(), Revision: reference.RevisionNo,
			RevisionHash: reference.CandidateRevisionHash, ContentHash: reference.CandidateContentHash,
			Candidate: json.RawMessage(reference.Candidate),
		},
		ExpectedReferenceTargetSet: projection.ExpectedTargetSet,
	})
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, err
	}
	imageCapability, err := visualFoundationScopeImageCapability(database, run)
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, err
	}
	referenceLogicalID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:reference-plan:"+run.ProjectID.String()))
	gate, encoded, err := domain.NewVisualFoundationScopeGateInput(domain.VisualFoundationScopeGateInputDraft{
		WorkspaceID: run.WorkspaceID.String(), ProjectID: run.ProjectID.String(),
		WorkflowRunID: run.ID.String(), NodeRunID: node.ID.String(), Subject: subject,
		PresetRelease: release, ImageGenerationCapability: imageCapability,
		ExpectedPresetHead: domain.HumanGateExpectedHead{
			OwnerKind: "preset", LogicalID: run.ProjectID.String(), Revision: 0,
		},
		ExpectedReferenceHead: domain.HumanGateExpectedHead{
			OwnerKind: "production/reference", LogicalID: referenceLogicalID.String(), Revision: 0,
		},
	})
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, err
	}
	record := model.WorkflowHumanGateInput{
		ID:          uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:visual-foundation-scope-gate-input:"+node.ID.String()+":"+gate.InputHash)),
		WorkspaceID: run.WorkspaceID, ProjectID: run.ProjectID, WorkflowRunID: run.ID, NodeRunID: node.ID,
		GateKey: gate.GateKey, GateInstanceKey: gate.GateInstanceKey, SubjectType: gate.SubjectType,
		SubjectHash: gate.SubjectHash, EffectPlanHash: gate.EffectPlanHash, InputHash: gate.InputHash,
		Input: datatypes.JSON(encoded), CreatedAt: now.UTC(),
	}
	var existing model.WorkflowHumanGateInput
	loadErr := database.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "node_run_id = ?", node.ID).Error
	if loadErr == nil {
		if !sameProductionWorldGateInput(existing, record) {
			return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, errors.New("Visual Foundation Scope Gate input has drifted")
		}
		return existing, gate, visualFoundationScopeCandidateIDs(subject), nil
	}
	if !errors.Is(loadErr, gorm.ErrRecordNotFound) {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, loadErr
	}
	if err = database.Omit(clause.Associations).Create(&record).Error; err != nil {
		return model.WorkflowHumanGateInput{}, domain.VisualFoundationScopeGateInput{}, nil, err
	}
	return record, gate, visualFoundationScopeCandidateIDs(subject), nil
}

func visualFoundationScopeGateBindings(input domain.NodeInputSnapshot) (map[string]domain.NodeInputBinding, error) {
	if len(input.Bindings) != 4 {
		return nil, errors.New("Visual Foundation Scope Gate has an incomplete read set")
	}
	expected := map[string]struct{ valueType, sourcePort string }{
		"storygraph":        {"storygraph_version", "storygraph"},
		"selection":         {"project_preset_selection", "selection"},
		"visual_foundation": {"visual_foundation_candidate", "candidate"},
		"reference_plan":    {"reference_plan_candidate", "candidate"},
	}
	result := make(map[string]domain.NodeInputBinding, len(expected))
	for _, binding := range input.Bindings {
		contract, exists := expected[binding.Port]
		if !exists || binding.ValueType != contract.valueType || binding.SourceKind != domain.NodeInputSourceNodeOutput ||
			binding.SourcePort != contract.sourcePort || binding.SourceNodeID == "" || binding.ReferenceID == "" || binding.ReferenceVersion == "" ||
			len(binding.ContentHash) != 64 {
			return nil, errors.New("Visual Foundation Scope Gate input has drifted")
		}
		result[binding.Port] = binding
	}
	if len(result) != len(expected) {
		return nil, errors.New("Visual Foundation Scope Gate input has drifted")
	}
	return result, nil
}

func validateVisualFoundationScopeCandidateSource(
	database *gorm.DB,
	run model.WorkflowRun,
	binding domain.NodeInputBinding,
	stageKey string,
) error {
	id, err := uuid.Parse(binding.ReferenceID)
	if err != nil {
		return errors.New("Visual Foundation Scope Gate Candidate identity is invalid")
	}
	var record model.SceneAnalysisCandidateRevision
	if err = database.First(&record, "id = ?", id).Error; err != nil {
		return normalizeNotFound(err)
	}
	var invocation model.SceneAnalysisInvocationRecord
	if err = database.First(&invocation, "id = ?", record.SourceInvocationID).Error; err != nil {
		return normalizeNotFound(err)
	}
	var sourceNode model.NodeRunProjection
	if err = database.First(&sourceNode, "id = ?", invocation.NodeRunID).Error; err != nil {
		return normalizeNotFound(err)
	}
	result, resultErr := completedNodeResult(sourceNode)
	if resultErr != nil || invocation.WorkflowRunID != run.ID || invocation.StageKey != stageKey ||
		sourceNode.WorkflowRunID != run.ID || sourceNode.NodeID != binding.SourceNodeID ||
		len(result.Output.Bindings) != 1 || result.Output.Bindings[0].ReferenceID != binding.ReferenceID ||
		result.Output.Bindings[0].Port != binding.SourcePort || result.Output.Bindings[0].ValueType != binding.ValueType ||
		result.Output.Bindings[0].ReferenceVersion != binding.ReferenceVersion ||
		result.Output.Bindings[0].ContentHash != binding.ContentHash {
		return errors.New("Visual Foundation Scope Gate Candidate source output has drifted")
	}
	return nil
}

func loadVisualFoundationScopeReferenceCandidate(
	database *gorm.DB,
	run model.WorkflowRun,
	binding domain.NodeInputBinding,
) (model.SceneAnalysisCandidateRevision, agentcontract.ReferencePlanInput, workflowapp.ReferencePlanCandidateProjection, error) {
	id, err := uuid.Parse(binding.ReferenceID)
	if err != nil {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, errors.New("Visual Foundation Scope Gate Reference Candidate identity is invalid")
	}
	var record model.SceneAnalysisCandidateRevision
	if err = database.First(&record, "id = ?", id).Error; err != nil {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, normalizeNotFound(err)
	}
	if record.WorkspaceID != run.WorkspaceID || record.ProjectID != run.ProjectID ||
		record.CandidateType != "reference_plan_candidate" || record.RevisionNo < 1 ||
		binding.ReferenceVersion != strconv.FormatInt(record.RevisionNo, 10) ||
		binding.ContentHash != record.CandidateRevisionHash {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, errors.New("Visual Foundation Scope Gate Reference Candidate has drifted")
	}
	var head model.SceneAnalysisCandidateHead
	if err = database.First(&head, "stage_instance_key = ?", record.StageInstanceKey).Error; err != nil {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, normalizeNotFound(err)
	}
	if head.CurrentRevisionID != record.ID || head.CurrentCandidateRevisionHash != record.CandidateRevisionHash ||
		head.Revision != record.RevisionNo || head.WorkspaceID != run.WorkspaceID || head.ProjectID != run.ProjectID {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, errors.New("Visual Foundation Scope Gate Reference Candidate Head has drifted")
	}
	var invocation model.SceneAnalysisInvocationRecord
	if err = database.First(&invocation, "id = ?", record.SourceInvocationID).Error; err != nil {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, normalizeNotFound(err)
	}
	var payload agentcontract.ReferencePlanPayload
	if json.Unmarshal(invocation.Payload, &payload) != nil || payload.Validate() != nil ||
		invocation.WorkflowRunID != run.ID ||
		invocation.StageKey != agentcontract.ReferencePlanStageKey || invocation.Status != "accepted" {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, errors.New("Visual Foundation Scope Gate Reference invocation has drifted")
	}
	contentHash, err := platformcanonical.Hash(json.RawMessage(record.Candidate))
	if err != nil || contentHash != record.CandidateContentHash {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, errors.New("Visual Foundation Scope Gate Reference Candidate content has drifted")
	}
	projection, err := workflowapp.BuildReferencePlanCandidateProjection(payload.StageInput, json.RawMessage(record.Candidate))
	if err != nil {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, err
	}
	revisionMaterial, err := json.Marshal(map[string]any{
		"contract_id":        "reference-plan-candidate-revision-production",
		"stage_instance_key": record.StageInstanceKey, "revision": record.RevisionNo,
		"candidate_type": record.CandidateType, "source_invocation_id": record.SourceInvocationID.String(),
		"source_result_id": record.SourceResultID.String(), "source_result_hash": record.SourceResultHash,
		"candidate_content_hash": record.CandidateContentHash,
	})
	if err != nil {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, err
	}
	revisionHash, err := agentcontract.ProductionCanonicalHash(revisionMaterial)
	if err != nil || revisionHash != record.CandidateRevisionHash {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, errors.New("Visual Foundation Scope Gate Reference Candidate revision has drifted")
	}
	var sourceNode model.NodeRunProjection
	if err = database.First(&sourceNode, "id = ?", invocation.NodeRunID).Error; err != nil {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, normalizeNotFound(err)
	}
	result, resultErr := completedNodeResult(sourceNode)
	if resultErr != nil || sourceNode.WorkflowRunID != run.ID || sourceNode.NodeID != binding.SourceNodeID ||
		len(result.Output.Bindings) != 1 || result.Output.Bindings[0].ReferenceID != binding.ReferenceID ||
		result.Output.Bindings[0].Port != binding.SourcePort || result.Output.Bindings[0].ValueType != binding.ValueType ||
		result.Output.Bindings[0].ReferenceVersion != binding.ReferenceVersion || result.Output.Bindings[0].ContentHash != binding.ContentHash {
		return model.SceneAnalysisCandidateRevision{}, agentcontract.ReferencePlanInput{}, workflowapp.ReferencePlanCandidateProjection{}, errors.New("Visual Foundation Scope Gate Reference source output has drifted")
	}
	return record, payload.StageInput, projection, nil
}

func visualFoundationScopeImageCapability(
	database *gorm.DB,
	run model.WorkflowRun,
) (domain.VisualFoundationScopeImageGenerationCapability, error) {
	material := struct {
		Purpose    string `json:"purpose"`
		ProjectID  string `json:"project_id"`
		BindingID  string `json:"binding_id,omitempty"`
		BindingRev int64  `json:"binding_revision,omitempty"`
		Binding    string `json:"binding_hash,omitempty"`
		Connection string `json:"connection_hash,omitempty"`
		Profile    string `json:"profile_hash,omitempty"`
		Credential string `json:"credential_fingerprint,omitempty"`
	}{Purpose: "reference_asset", ProjectID: run.ProjectID.String()}
	var binding model.ProjectProviderBindingVersion
	err := database.Where("workspace_id = ? AND project_id = ? AND purpose = ?", run.WorkspaceID, run.ProjectID, "reference_asset").
		Order("revision DESC").First(&binding).Error
	available := false
	if err == nil {
		var connection model.ProviderConnectionVersion
		var profile model.ProviderModelProfileVersion
		var credential model.ProviderCredentialVersion
		if loadErr := database.First(&connection, "id = ?", binding.ConnectionVersionID).Error; loadErr != nil {
			return domain.VisualFoundationScopeImageGenerationCapability{}, normalizeNotFound(loadErr)
		}
		if loadErr := database.First(&profile, "id = ?", binding.ModelProfileVersionID).Error; loadErr != nil {
			return domain.VisualFoundationScopeImageGenerationCapability{}, normalizeNotFound(loadErr)
		}
		if loadErr := database.First(&credential, "id = ?", binding.CredentialVersionID).Error; loadErr != nil {
			return domain.VisualFoundationScopeImageGenerationCapability{}, normalizeNotFound(loadErr)
		}
		material.BindingID, material.BindingRev, material.Binding = binding.ID.String(), binding.Revision, binding.ContentHash
		material.Connection, material.Profile, material.Credential = connection.ContentHash, profile.ContentHash, credential.SecretFingerprint
		available = binding.Modality == "image" && connection.State == "enabled" && profile.State == "enabled" &&
			profile.Modality == "image" && binding.ProviderKey == connection.ProviderKey &&
			binding.ProviderKey == profile.ProviderKey && binding.ConnectionVersionID == connection.ID &&
			binding.ModelProfileVersionID == profile.ID && binding.CredentialVersionID == credential.ID &&
			connection.CredentialVersionID == credential.ID && credential.ProviderKey == binding.ProviderKey &&
			profile.ConnectionKey == connection.ConnectionKey && binding.AdapterContractVersion == connection.AdapterContractVersion
		available = available && binding.AdapterContractVersion == profile.AdapterTransportContract
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.VisualFoundationScopeImageGenerationCapability{}, err
	}
	raw, err := json.Marshal(material)
	if err != nil {
		return domain.VisualFoundationScopeImageGenerationCapability{}, err
	}
	hash, err := platformcanonical.Hash(raw)
	if err != nil {
		return domain.VisualFoundationScopeImageGenerationCapability{}, err
	}
	return domain.VisualFoundationScopeImageGenerationCapability{Available: available, ReadSetHash: hash}, nil
}

func visualFoundationScopeCandidateIDs(subject domain.VisualFoundationScopeSubject) []string {
	values := []string{subject.VisualFoundationCandidate.RevisionID, subject.ReferencePlanCandidate.RevisionID}
	slices.Sort(values)
	return slices.Compact(values)
}
