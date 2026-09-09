package gormdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type structureIdentityCandidate struct {
	record   model.SceneAnalysisCandidateRevision
	identity agentcontract.SceneAnalysisCandidateRevisionIdentity
}

func prepareStructureIdentityGateInput(
	database *gorm.DB,
	run model.WorkflowRun,
	node model.NodeRunProjection,
	input domain.NodeInputSnapshot,
	now time.Time,
) (model.WorkflowHumanGateInput, domain.StructureIdentityGateInput, error) {
	bindings, err := structureIdentityGateBindings(input)
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, err
	}
	source, revision, err := loadStructureIdentitySource(database, run, bindings["source"])
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, err
	}
	candidates := make(map[string]structureIdentityCandidate, 4)
	for _, expected := range []struct {
		port, stage, candidateType string
	}{
		{"spans", "propose_script_spans", "script_span_candidate"},
		{"facts", "extract_scene_facts", "scene_fact_candidate"},
		{"identities", "resolve_identities", "identity_resolution_candidate"},
		{"review", "review_candidate", "structure_identity_review_candidate"},
	} {
		candidate, loadErr := loadStructureIdentityCandidate(
			database, run, source, bindings[expected.port], expected.stage, expected.candidateType,
		)
		if loadErr != nil {
			return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, loadErr
		}
		candidates[expected.port] = candidate
	}

	var reviewPayload agentcontract.SceneAnalysisPayload
	if err = json.Unmarshal(candidates["review"].record.SourceInvocation.Payload, &reviewPayload); err != nil ||
		reviewPayload.Validate() != nil || len(reviewPayload.SourceRefs) != 1 || reviewPayload.SourceRefs[0] != source ||
		!structureIdentityUpstreamsMatch(reviewPayload.UpstreamCandidates, candidates) {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, errors.New("structure identity Gate review payload has drifted")
	}
	var reviewInput agentcontract.StructureIdentityReviewInput
	var reviewCandidate agentcontract.StructureIdentityReviewCandidate
	if err = json.Unmarshal(reviewPayload.StageInput, &reviewInput); err != nil || reviewInput.Validate() != nil ||
		json.Unmarshal(candidates["review"].record.Candidate, &reviewCandidate) != nil ||
		agentcontract.ValidateStructureIdentityReviewCandidate(json.RawMessage(candidates["review"].record.Candidate), reviewInput) != nil {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, errors.New("structure identity Gate reviewed Candidate has drifted")
	}
	if !equalJSON(reviewInput.SpanCandidate, candidates["spans"].record.Candidate) ||
		!equalJSON(reviewInput.SceneFactCandidate, candidates["facts"].record.Candidate) ||
		!equalJSON(reviewInput.IdentityCandidate, candidates["identities"].record.Candidate) ||
		reviewInput.SpanCandidateRevisionID != candidates["spans"].identity.CandidateRevisionID ||
		reviewInput.SpanCandidateRevisionHash != candidates["spans"].identity.CandidateRevisionHash ||
		reviewInput.SceneFactCandidateRevisionID != candidates["facts"].identity.CandidateRevisionID ||
		reviewInput.SceneFactCandidateRevisionHash != candidates["facts"].identity.CandidateRevisionHash ||
		reviewInput.IdentityCandidateRevisionID != candidates["identities"].identity.CandidateRevisionID ||
		reviewInput.IdentityCandidateRevisionHash != candidates["identities"].identity.CandidateRevisionHash {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, errors.New("structure identity Gate read set has drifted")
	}

	var spans agentcontract.ScriptSpanCandidate
	if err = json.Unmarshal(candidates["spans"].record.Candidate, &spans); err != nil ||
		agentcontract.ValidateScriptSpanCandidate(json.RawMessage(candidates["spans"].record.Candidate), revision.NormalizedText) != nil {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, errors.New("structure identity Gate Span Candidate has drifted")
	}
	evidence := structureIdentityEvidence(source.VersionID, spans, reviewCandidate)
	affectedScopes := make([]string, len(spans.Spans))
	for index, span := range spans.Spans {
		sceneID := uuid.NewSHA1(run.ProjectID, []byte("lanverse:scene:"+span.TemporarySpanID))
		affectedScopes[index] = "scene:" + sceneID.String()
	}

	projectHead, err := structureIdentityProjectHead(database, run.ProjectID)
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, err
	}
	allowedDecisions := []string{"changes_requested", "rejected"}
	if !structureIdentityReviewBlocksApproval(reviewCandidate) {
		allowedDecisions = append(allowedDecisions, "approved")
	}
	gate, encoded, err := domain.NewStructureIdentityGateInput(domain.StructureIdentityGateInputDraft{
		WorkspaceID: run.WorkspaceID.String(), ProjectID: run.ProjectID.String(),
		WorkflowRunID: run.ID.String(), NodeRunID: node.ID.String(),
		Subject: domain.StructureIdentityGateSubject{
			SourceVersion: source, SpanCandidate: candidates["spans"].identity,
			SceneFactCandidate: candidates["facts"].identity,
			IdentityCandidate:  candidates["identities"].identity,
			ReviewCandidate:    candidates["review"].identity,
		},
		EvidenceRefs: evidence,
		Impact: domain.HumanGateImpactSummary{
			AffectedScopeKeys: affectedScopes,
			PreservedFamilies: []string{"script_source", "script_spans", "scene_facts", "identity_resolution"},
			InvalidatedFamilies: []string{
				"production_world", "visual_foundation", "reference_selection", "storyboard",
			},
		},
		AllowedDecisions:    allowedDecisions,
		ExpectedProjectHead: projectHead,
		ExpectedBibleHead: domain.HumanGateExpectedHead{
			OwnerKind: "production/bible", LogicalID: run.ProjectID.String(), Revision: 0,
		},
	})
	if err != nil {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, err
	}
	record := model.WorkflowHumanGateInput{
		ID:          uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:structure-identity-gate-input:"+node.ID.String()+":"+gate.InputHash)),
		WorkspaceID: run.WorkspaceID, ProjectID: run.ProjectID, WorkflowRunID: run.ID, NodeRunID: node.ID,
		GateKey: gate.GateKey, GateInstanceKey: gate.GateInstanceKey, SubjectType: gate.SubjectType,
		SubjectHash: gate.SubjectHash, EffectPlanHash: gate.EffectPlanHash, InputHash: gate.InputHash,
		Input: datatypes.JSON(encoded), CreatedAt: now.UTC(),
	}
	var existing model.WorkflowHumanGateInput
	loadErr := database.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "node_run_id = ?", node.ID).Error
	if loadErr == nil {
		if !sameStructureIdentityGateInput(existing, record) {
			return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, errors.New("structure identity Gate input has drifted")
		}
		return existing, gate, nil
	}
	if !errors.Is(loadErr, gorm.ErrRecordNotFound) {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, loadErr
	}
	if err = database.Omit(clause.Associations).Create(&record).Error; err != nil {
		return model.WorkflowHumanGateInput{}, domain.StructureIdentityGateInput{}, err
	}
	return record, gate, nil
}

func structureIdentityGateBindings(input domain.NodeInputSnapshot) (map[string]domain.NodeInputBinding, error) {
	expectedTypes := map[string]string{
		"source": "script_source_version", "spans": "script_span_candidate",
		"facts": "scene_fact_candidate", "identities": "identity_resolution_candidate",
		"review": "structure_identity_review_candidate",
	}
	if len(input.Bindings) != len(expectedTypes) {
		return nil, errors.New("workflow Structure Identity Human Gate has an incomplete read set")
	}
	bindings := make(map[string]domain.NodeInputBinding, len(input.Bindings))
	for _, binding := range input.Bindings {
		if expectedTypes[binding.Port] != binding.ValueType || binding.SourceKind != domain.NodeInputSourceNodeOutput {
			return nil, errors.New("workflow Structure Identity Human Gate input has drifted")
		}
		bindings[binding.Port] = binding
	}
	if len(bindings) != len(expectedTypes) {
		return nil, errors.New("workflow Structure Identity Human Gate input is duplicated")
	}
	return bindings, nil
}

func loadStructureIdentitySource(
	database *gorm.DB,
	run model.WorkflowRun,
	binding domain.NodeInputBinding,
) (agentcontract.ScriptSourceVersionIdentity, model.DocumentRevision, error) {
	revisionID, err := uuid.Parse(binding.ReferenceID)
	if err != nil {
		return agentcontract.ScriptSourceVersionIdentity{}, model.DocumentRevision{}, errors.New("structure identity Gate source identity is invalid")
	}
	var revision model.DocumentRevision
	if err = database.First(&revision, "id = ?", revisionID).Error; err != nil {
		return agentcontract.ScriptSourceVersionIdentity{}, model.DocumentRevision{}, normalizeNotFound(err)
	}
	var document model.ScriptDocument
	if err = database.First(&document, "id = ?", revision.DocumentID).Error; err != nil {
		return agentcontract.ScriptSourceVersionIdentity{}, model.DocumentRevision{}, normalizeNotFound(err)
	}
	var head model.ScriptSourceScopeHead
	if err = database.First(&head, "project_id = ?", run.ProjectID).Error; err != nil {
		return agentcontract.ScriptSourceVersionIdentity{}, model.DocumentRevision{}, normalizeNotFound(err)
	}
	if revision.WorkspaceID != run.WorkspaceID || document.WorkspaceID != run.WorkspaceID || document.ProjectID != run.ProjectID ||
		document.ID != revision.DocumentID || head.WorkspaceID != run.WorkspaceID ||
		head.CurrentDocumentRevisionID != revision.ID || head.DocumentLogicalID != document.ID ||
		binding.ReferenceVersion != strconv.Itoa(revision.VersionNo) || binding.ContentHash != revision.NormalizedHash {
		return agentcontract.ScriptSourceVersionIdentity{}, model.DocumentRevision{}, errors.New("structure identity Gate source Head has drifted")
	}
	source := agentcontract.ScriptSourceVersionIdentity{
		OwnerKind: "production/script", LogicalID: document.ID.String(), VersionID: revision.ID.String(),
		Revision: int64(revision.VersionNo), ContentHash: revision.NormalizedHash, CreatedAt: revision.CreatedAt.UTC(),
	}
	return source, revision, nil
}

func loadStructureIdentityCandidate(
	database *gorm.DB,
	run model.WorkflowRun,
	source agentcontract.ScriptSourceVersionIdentity,
	binding domain.NodeInputBinding,
	stageKey, candidateType string,
) (structureIdentityCandidate, error) {
	candidateID, err := uuid.Parse(binding.ReferenceID)
	if err != nil {
		return structureIdentityCandidate{}, errors.New("structure identity Gate Candidate identity is invalid")
	}
	var record model.SceneAnalysisCandidateRevision
	if err = database.Preload("SourceInvocation").First(&record, "id = ?", candidateID).Error; err != nil {
		return structureIdentityCandidate{}, normalizeNotFound(err)
	}
	invocation := record.SourceInvocation
	if record.WorkspaceID != run.WorkspaceID || record.ProjectID != run.ProjectID || record.CandidateType != candidateType ||
		binding.ReferenceVersion != strconv.FormatInt(record.RevisionNo, 10) ||
		binding.ContentHash != record.CandidateRevisionHash || invocation.ID != record.SourceInvocationID ||
		invocation.WorkspaceID != run.WorkspaceID || invocation.ProjectID != run.ProjectID ||
		invocation.WorkflowRunID != run.ID || invocation.StageKey != stageKey || invocation.ShardKey != "script:full" ||
		invocation.SourceVersionID.String() != source.VersionID || invocation.SourceHash != source.ContentHash ||
		invocation.Status != "accepted" {
		return structureIdentityCandidate{}, errors.New("structure identity Gate Candidate lineage has drifted")
	}
	var payload agentcontract.SceneAnalysisPayload
	if json.Unmarshal(invocation.Payload, &payload) != nil || payload.Validate() != nil ||
		len(payload.SourceRefs) != 1 || payload.SourceRefs[0] != source {
		return structureIdentityCandidate{}, errors.New("structure identity Gate invocation payload has drifted")
	}
	var result model.SceneAnalysisResult
	if err = database.First(&result, "id = ?", record.SourceResultID).Error; err != nil {
		return structureIdentityCandidate{}, normalizeNotFound(err)
	}
	decodedResult, decodeErr := agentcontract.DecodeSceneAnalysisAttemptResult(result.Result)
	computedResultHash, hashErr := decodedResult.ComputeResultHash()
	candidateContentHash, candidateHashErr := platformcanonical.Hash(json.RawMessage(record.Candidate))
	if decodeErr != nil || hashErr != nil || candidateHashErr != nil || result.Status != "accepted" ||
		result.InputHash != invocation.InputHash || result.OutputHash == nil || *result.OutputHash != record.CandidateContentHash ||
		decodedResult.Status != "accepted" || decodedResult.ResultHash != record.SourceResultHash ||
		computedResultHash != record.SourceResultHash || decodedResult.OutputHash == nil ||
		*decodedResult.OutputHash != record.CandidateContentHash || candidateContentHash != record.CandidateContentHash ||
		!equalJSON(decodedResult.Candidate, record.Candidate) {
		return structureIdentityCandidate{}, errors.New("structure identity Gate Candidate result has drifted")
	}
	candidateRevisionMaterial, marshalErr := json.Marshal(map[string]any{
		"contract_id": "scene-analysis-candidate-revision", "stage_instance_key": record.StageInstanceKey,
		"revision": record.RevisionNo, "candidate_type": record.CandidateType,
		"source_invocation_id": record.SourceInvocationID.String(), "source_result_id": record.SourceResultID.String(),
		"source_result_hash": record.SourceResultHash, "candidate_content_hash": record.CandidateContentHash,
	})
	candidateRevisionHash, hashErr := platformcanonical.Hash(candidateRevisionMaterial)
	if marshalErr != nil || hashErr != nil || candidateRevisionHash != record.CandidateRevisionHash {
		return structureIdentityCandidate{}, errors.New("structure identity Gate Candidate Revision has drifted")
	}
	identity := agentcontract.SceneAnalysisCandidateRevisionIdentity{
		StageKey: stageKey, ShardKey: invocation.ShardKey, CandidateRevisionID: record.ID.String(),
		CandidateRevisionHash: record.CandidateRevisionHash, SourceInvocationID: invocation.ID.String(),
		SourceResultHash: record.SourceResultHash,
	}
	if identity.Validate() != nil {
		return structureIdentityCandidate{}, errors.New("structure identity Gate Candidate reference is invalid")
	}
	return structureIdentityCandidate{record: record, identity: identity}, nil
}

func structureIdentityUpstreamsMatch(
	upstreams []agentcontract.SceneAnalysisCandidateRevisionIdentity,
	candidates map[string]structureIdentityCandidate,
) bool {
	if len(upstreams) != 3 {
		return false
	}
	expected := map[string]agentcontract.SceneAnalysisCandidateRevisionIdentity{
		"propose_script_spans": candidates["spans"].identity,
		"extract_scene_facts":  candidates["facts"].identity,
		"resolve_identities":   candidates["identities"].identity,
	}
	for _, upstream := range upstreams {
		if expected[upstream.StageKey] != upstream {
			return false
		}
		delete(expected, upstream.StageKey)
	}
	return len(expected) == 0
}

func structureIdentityEvidence(
	sourceVersionID string,
	spans agentcontract.ScriptSpanCandidate,
	review agentcontract.StructureIdentityReviewCandidate,
) []domain.HumanGateEvidenceRef {
	byKey := make(map[string]domain.HumanGateEvidenceRef)
	add := func(value agentcontract.SourceEvidenceSpan) {
		key := fmt.Sprintf("%020d:%020d:%s", value.SourceStart, value.SourceEnd, value.TextHash)
		byKey[key] = domain.HumanGateEvidenceRef{
			SourceVersionID: sourceVersionID, SourceStart: value.SourceStart,
			SourceEnd: value.SourceEnd, TextHash: value.TextHash,
		}
	}
	for _, span := range spans.Spans {
		add(span.Evidence)
	}
	for _, issue := range review.ReviewIssues {
		for _, item := range issue.Evidence {
			add(item)
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	result := make([]domain.HumanGateEvidenceRef, len(keys))
	for index, key := range keys {
		result[index] = byKey[key]
	}
	return result
}

func structureIdentityReviewBlocksApproval(value agentcontract.StructureIdentityReviewCandidate) bool {
	return slices.ContainsFunc(value.ReviewIssues, func(issue agentcontract.CandidateReviewIssue) bool {
		return issue.Severity == "blocking"
	})
}

func structureIdentityProjectHead(database *gorm.DB, projectID uuid.UUID) (domain.HumanGateExpectedHead, error) {
	var project model.Project
	if err := database.First(&project, "id = ?", projectID).Error; err != nil {
		return domain.HumanGateExpectedHead{}, normalizeNotFound(err)
	}
	var episodes []model.Episode
	if err := database.Where("project_id = ? AND status = ?", projectID, "active").
		Order("position").Order("id").Find(&episodes).Error; err != nil {
		return domain.HumanGateExpectedHead{}, err
	}
	order := make([]struct {
		ID       string `json:"id"`
		Position int    `json:"position"`
	}, len(episodes))
	for index, episode := range episodes {
		order[index] = struct {
			ID       string `json:"id"`
			Position int    `json:"position"`
		}{episode.ID.String(), episode.Position}
	}
	contentHash, err := platformcommand.InputHash(order)
	if err != nil {
		return domain.HumanGateExpectedHead{}, err
	}
	return domain.HumanGateExpectedHead{
		OwnerKind: "production/project", LogicalID: project.ID.String(),
		Revision: int64(project.Revision), ContentHash: contentHash,
	}, nil
}

func sameStructureIdentityGateInput(left, right model.WorkflowHumanGateInput) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.WorkflowRunID == right.WorkflowRunID && left.NodeRunID == right.NodeRunID && left.GateKey == right.GateKey &&
		left.GateInstanceKey == right.GateInstanceKey && left.SubjectType == right.SubjectType &&
		left.SubjectHash == right.SubjectHash && left.EffectPlanHash == right.EffectPlanHash &&
		left.InputHash == right.InputHash && equalJSON(left.Input, right.Input)
}

func structureIdentityCandidateIDs(value domain.StructureIdentityGateInput) []string {
	result := []string{
		value.Subject.SpanCandidate.CandidateRevisionID,
		value.Subject.SceneFactCandidate.CandidateRevisionID,
		value.Subject.IdentityCandidate.CandidateRevisionID,
		value.Subject.ReviewCandidate.CandidateRevisionID,
	}
	for index := range result {
		result[index] = strings.TrimSpace(result[index])
	}
	slices.Sort(result)
	return result
}
