package gormdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type resolvedNodeExecution struct {
	Input         domain.NodeInputSnapshot
	InputJSON     json.RawMessage
	InputHash     string
	Execution     domain.NodeExecution
	CacheMaterial domain.NodeCacheKeyMaterial
	CacheKey      string
}

func resolveNodeExecution(transaction *gorm.DB, run model.WorkflowRun, node model.NodeRunProjection) (resolvedNodeExecution, error) {
	var definition model.WorkflowDefinitionVersion
	if err := transaction.First(&definition, "id = ?", run.WorkflowDefinitionVersionID).Error; err != nil {
		return resolvedNodeExecution{}, normalizeNotFound(err)
	}
	var snapshot model.RunInputSnapshot
	if err := transaction.Where(
		"id = ? AND workflow_definition_version_id = ?", run.RunInputSnapshotID, run.WorkflowDefinitionVersionID,
	).First(&snapshot).Error; err != nil {
		return resolvedNodeExecution{}, normalizeNotFound(err)
	}
	compiled, err := compiledFacts(definition, snapshot)
	if err != nil {
		return resolvedNodeExecution{}, err
	}
	if compiled.Definition.WorkspaceID != run.WorkspaceID.String() || compiled.Definition.ProjectID != run.ProjectID.String() ||
		compiled.RunInputSnapshot.WorkspaceID != run.WorkspaceID.String() ||
		compiled.RunInputSnapshot.ProjectID != run.ProjectID.String() {
		return resolvedNodeExecution{}, errors.New("workflow node input ownership has drifted")
	}

	executions := make(map[string]domain.NodeExecution, len(compiled.Definition.NodeExecutions))
	for _, execution := range compiled.Definition.NodeExecutions {
		executions[execution.NodeID] = execution
	}
	execution, exists := executions[node.NodeID]
	if !exists || execution.DefinitionKey != node.DefinitionKey || execution.DefinitionVersion != node.DefinitionVersion ||
		execution.Executor != node.Executor || execution.RiskLevel != node.RiskLevel || len(execution.OutputPorts) == 0 {
		return resolvedNodeExecution{}, errors.New("workflow node input execution descriptor has drifted")
	}
	var graphNode authoring.Node
	graphNodeFound := false
	for _, candidate := range compiled.Definition.ExecutionGraph.Nodes {
		if candidate.ID == node.NodeID {
			graphNode, graphNodeFound = candidate, true
			break
		}
	}
	if !graphNodeFound || graphNode.DefinitionKey != node.DefinitionKey || graphNode.DefinitionVersion != node.DefinitionVersion {
		return resolvedNodeExecution{}, errors.New("workflow node input graph identity has drifted")
	}

	var projections []model.NodeRunProjection
	if err = transaction.Where("workflow_run_id = ?", run.ID).Find(&projections).Error; err != nil {
		return resolvedNodeExecution{}, err
	}
	projectionByNode := make(map[string]model.NodeRunProjection, len(projections))
	for _, projection := range projections {
		if _, duplicated := projectionByNode[projection.NodeID]; duplicated {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source %s is duplicated", projection.NodeID)
		}
		projectionByNode[projection.NodeID] = projection
	}

	bindings := make([]domain.NodeInputBinding, 0, len(execution.InputPorts))
	for _, edge := range compiled.Definition.ExecutionGraph.Edges {
		if edge.ToNodeID != node.NodeID {
			continue
		}
		sourceProjection, sourceExists := projectionByNode[edge.FromNodeID]
		sourceExecution, executionExists := executions[edge.FromNodeID]
		if !sourceExists || !executionExists || sourceProjection.DefinitionKey != sourceExecution.DefinitionKey ||
			sourceProjection.DefinitionVersion != sourceExecution.DefinitionVersion ||
			sourceProjection.Executor != sourceExecution.Executor || sourceProjection.RiskLevel != sourceExecution.RiskLevel {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source %s is missing", edge.FromNodeID)
		}
		sourcePort, sourcePortExists := executionPort(sourceExecution.OutputPorts, edge.FromPort)
		targetPort, targetPortExists := executionPort(execution.InputPorts, edge.ToPort)
		if !sourcePortExists || !targetPortExists || sourcePort.ValueType != targetPort.ValueType {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input edge %s has drifted", edge.ID)
		}
		sourceResult, resultErr := completedNodeResult(sourceProjection)
		if resultErr != nil {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source %s: %w", edge.FromNodeID, resultErr)
		}
		if resultErr = domain.ValidateNodeOutputPorts(sourceResult.Output, sourceExecution.OutputPorts); resultErr != nil {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source %s output: %w", edge.FromNodeID, resultErr)
		}
		output, outputExists := outputBinding(sourceResult.Output, edge.FromPort)
		if !outputExists || output.ValueType != targetPort.ValueType {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source port %s has drifted", edge.FromPort)
		}
		bindings = append(bindings, domain.NodeInputBinding{
			Port: edge.ToPort, ValueType: targetPort.ValueType, SourceKind: domain.NodeInputSourceNodeOutput,
			SourceNodeID: edge.FromNodeID, SourcePort: edge.FromPort,
			ReferenceID: output.ReferenceID, ReferenceVersion: output.ReferenceVersion, ContentHash: output.ContentHash,
		})
	}
	for _, binding := range compiled.Definition.ExecutionGraph.Bindings {
		if binding.NodeID != node.NodeID {
			continue
		}
		port, portExists := executionPort(execution.InputPorts, binding.Port)
		value, variableExists := compiled.Definition.ExecutionGraph.Variables[binding.Variable]
		if !portExists || !variableExists || port.ValueType != binding.ValueType {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node variable binding %s has drifted", binding.Port)
		}
		bindings = append(bindings, domain.NodeInputBinding{
			Port: binding.Port, ValueType: binding.ValueType, SourceKind: domain.NodeInputSourceVariable,
			Variable: binding.Variable, Value: append([]byte(nil), value...),
		})
	}
	if err = validateResolvedInputPorts(bindings, execution.InputPorts); err != nil {
		return resolvedNodeExecution{}, err
	}
	config, err := nodeExecutionConfig(transaction, run, node, projections, graphNode.Config)
	if err != nil {
		return resolvedNodeExecution{}, err
	}
	input, inputJSON, inputHash, err := domain.BuildNodeInput(domain.NodeInputSnapshot{
		SchemaVersion: domain.NodeInputSchemaVersion, Config: config, Bindings: bindings,
		FrozenInputs: compiled.RunInputSnapshot.FrozenInputs,
	})
	if err != nil {
		return resolvedNodeExecution{}, err
	}
	cacheMaterial, cacheKey, err := domain.BuildNodeCacheMaterial(execution, input, compiled.Definition.RuntimeContractVersion)
	if err != nil {
		return resolvedNodeExecution{}, err
	}
	if execution.CachePolicy == "never" {
		cacheKey = ""
	}
	return resolvedNodeExecution{
		Input: input, InputJSON: inputJSON, InputHash: inputHash, Execution: execution,
		CacheMaterial: cacheMaterial, CacheKey: cacheKey,
	}, nil
}

func nodeExecutionConfig(
	transaction *gorm.DB,
	run model.WorkflowRun,
	node model.NodeRunProjection,
	projections []model.NodeRunProjection,
	base json.RawMessage,
) (json.RawMessage, error) {
	if run.RepairDecisionID == nil || run.RepairDecisionHash == nil || run.RerunRootNodeID == nil {
		return append(json.RawMessage(nil), base...), nil
	}
	if run.SourceWorkflowRunID == nil {
		return nil, errors.New("workflow repair source identity is missing")
	}
	var decision model.ReviewDecision
	if err := transaction.First(&decision, "id = ?", *run.RepairDecisionID).Error; err != nil {
		return nil, normalizeNotFound(err)
	}
	var task model.HumanTask
	if err := transaction.First(&task, "id = ?", decision.HumanTaskID).Error; err != nil {
		return nil, normalizeNotFound(err)
	}
	payloadHash, hashErr := platformcanonical.Hash(json.RawMessage(decision.DecisionPayload))
	if hashErr != nil || payloadHash != decision.DecisionPayloadHash || decision.DecisionPayloadHash != *run.RepairDecisionHash ||
		decision.ID != *run.RepairDecisionID || decision.Decision != "changes_requested" ||
		decision.WorkspaceID != run.WorkspaceID || decision.HumanTaskID != task.ID || task.Status != "COMPLETED" ||
		decision.SubjectRevision != task.SubjectRevision || decision.SubjectHash != task.SubjectHash ||
		task.WorkflowRunID != *run.SourceWorkflowRunID || task.WorkspaceID != run.WorkspaceID || task.ProjectID != run.ProjectID {
		return nil, errors.New("workflow repair decision payload has drifted")
	}
	switch task.SubjectType {
	case "structure_identity_gate_input":
		if node.NodeID != *run.RerunRootNodeID {
			return append(json.RawMessage(nil), base...), nil
		}
		return structureIdentityRepairNodeConfig(decision, node, projections, base)
	case "production_world_gate_input":
		return productionWorldRepairNodeConfig(transaction, run, decision, task, node, projections, base)
	default:
		return nil, errors.New("workflow repair subject is unsupported")
	}
}

func structureIdentityRepairNodeConfig(
	decision model.ReviewDecision,
	node model.NodeRunProjection,
	projections []model.NodeRunProjection,
	base json.RawMessage,
) (json.RawMessage, error) {
	var baseConfig map[string]json.RawMessage
	if json.Unmarshal(base, &baseConfig) != nil || len(baseConfig) != 0 {
		return nil, errors.New("workflow repair root config is not empty")
	}
	var payload struct {
		ChangeRequest *domain.StructureIdentityChangeRequest `json:"change_request"`
	}
	if err := json.Unmarshal(decision.DecisionPayload, &payload); err != nil || payload.ChangeRequest == nil {
		return nil, errors.New("workflow repair decision payload is invalid")
	}
	source := make([]domain.NodeRunProjection, len(projections))
	for index, projection := range projections {
		source[index] = domain.NodeRunProjection{NodeID: projection.NodeID, Executor: projection.Executor}
	}
	rootNodeID, err := domain.StructureIdentityRepairRootNode(source, *payload.ChangeRequest)
	if err != nil || rootNodeID != node.NodeID {
		return nil, errors.New("workflow repair root has drifted")
	}
	evidenceRefs := make([]agentcontract.StructureIdentityRepairEvidence, len(payload.ChangeRequest.EvidenceRefs))
	for index, evidence := range payload.ChangeRequest.EvidenceRefs {
		evidenceRefs[index] = agentcontract.StructureIdentityRepairEvidence{
			SourceVersionID: evidence.SourceVersionID, SourceStart: evidence.SourceStart,
			SourceEnd: evidence.SourceEnd, TextHash: evidence.TextHash,
		}
	}
	directive := domain.StructureIdentityRepairDirective{
		ReviewDecisionID: decision.ID.String(), DecisionPayloadHash: decision.DecisionPayloadHash,
		IssueRefs:    append([]string(nil), payload.ChangeRequest.IssueRefs...),
		EvidenceRefs: evidenceRefs,
		ChangeSpec: agentcontract.StructureIdentityRepairChange{
			Operation:         payload.ChangeRequest.ChangeSpec.Operation,
			TargetKeys:        append([]string(nil), payload.ChangeRequest.ChangeSpec.TargetKeys...),
			AffectedScopeKeys: append([]string(nil), payload.ChangeRequest.ChangeSpec.AffectedScopeKeys...),
		},
		ReasonCode: payload.ChangeRequest.ReasonCode,
	}
	repairStage := map[string]string{
		"activity.script_span_proposal": "propose_script_spans",
		"activity.identity_resolution":  "resolve_identities",
	}[node.Executor]
	if err := directive.ValidateFor(repairStage); err != nil {
		return nil, errors.New("workflow repair directive is invalid")
	}
	encoded, err := json.Marshal(struct {
		Repair domain.StructureIdentityRepairDirective `json:"repair"`
	}{Repair: directive})
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func productionWorldRepairNodeConfig(
	transaction *gorm.DB,
	run model.WorkflowRun,
	decision model.ReviewDecision,
	task model.HumanTask,
	node model.NodeRunProjection,
	projections []model.NodeRunProjection,
	base json.RawMessage,
) (json.RawMessage, error) {
	var payload struct {
		ChangeRequest *domain.ProductionWorldChangeRequest `json:"change_request"`
	}
	if err := json.Unmarshal(decision.DecisionPayload, &payload); err != nil || payload.ChangeRequest == nil {
		return nil, errors.New("Production World repair decision payload is invalid")
	}
	source := make([]domain.NodeRunProjection, len(projections))
	for index, projection := range projections {
		source[index] = domain.NodeRunProjection{NodeID: projection.NodeID, Executor: projection.Executor}
	}
	rootNodeID, err := domain.ProductionWorldRepairRootNode(source, *payload.ChangeRequest)
	if err != nil || run.RerunRootNodeID == nil || rootNodeID != *run.RerunRootNodeID {
		return nil, errors.New("Production World repair root has drifted")
	}
	rootExecutor := ""
	for _, projection := range projections {
		if projection.NodeID == rootNodeID {
			rootExecutor = projection.Executor
			break
		}
	}
	stageByExecutor := map[string]string{
		"activity.production_entity_derivation":          "derive_production_entities",
		"activity.scene_occurrence_binding":              "bind_scene_occurrences",
		"activity.interaction_continuity_reconciliation": "reconcile_interaction_continuity",
	}
	stageOrder := map[string]int{
		"derive_production_entities": 1, "bind_scene_occurrences": 2, "reconcile_interaction_continuity": 3,
	}
	stageKey, stageExists := stageByExecutor[node.Executor]
	rootStage, rootExists := stageByExecutor[rootExecutor]
	if !stageExists || !rootExists || stageOrder[stageKey] < stageOrder[rootStage] {
		return append(json.RawMessage(nil), base...), nil
	}
	var baseConfig map[string]json.RawMessage
	if json.Unmarshal(base, &baseConfig) != nil || len(baseConfig) != 0 {
		return nil, errors.New("Production World repair stage config is not empty")
	}
	gate, detail, err := loadProductionWorldRepairReview(transaction, task)
	if err != nil || domain.ValidateProductionWorldChangeRequest(gate, detail, *payload.ChangeRequest) != nil {
		return nil, errors.New("Production World repair decision no longer matches its frozen review")
	}
	closure, err := domain.NewProductionWorldRepairClosure(detail.Views, domain.ProductionWorldRepairSelection{
		Operation:  payload.ChangeRequest.ChangeSpec.Operation,
		TargetKeys: append([]string(nil), payload.ChangeRequest.ChangeSpec.TargetKeys...),
	})
	if err != nil {
		return nil, err
	}
	baseCandidate, err := loadProductionWorldRepairBaseCandidate(
		transaction, run, node, stageKey,
	)
	if err != nil {
		return nil, err
	}
	evidenceRefs := make([]agentcontract.StructureIdentityRepairEvidence, len(payload.ChangeRequest.EvidenceRefs))
	for index, evidence := range payload.ChangeRequest.EvidenceRefs {
		evidenceRefs[index] = agentcontract.StructureIdentityRepairEvidence{
			SourceVersionID: evidence.SourceVersionID, SourceStart: evidence.SourceStart,
			SourceEnd: evidence.SourceEnd, TextHash: evidence.TextHash,
		}
	}
	directive := agentcontract.ProductionWorldRepairDirective{
		ReviewDecisionID: decision.ID.String(), DecisionPayloadHash: decision.DecisionPayloadHash,
		IssueRefs: append(
			make([]string, 0, len(payload.ChangeRequest.IssueRefs)),
			payload.ChangeRequest.IssueRefs...,
		),
		EvidenceRefs: evidenceRefs,
		ChangeSpec: agentcontract.ProductionWorldRepairChange{
			Operation:  payload.ChangeRequest.ChangeSpec.Operation,
			TargetKeys: append([]string(nil), payload.ChangeRequest.ChangeSpec.TargetKeys...),
		},
		Closure: agentcontract.ProductionWorldRepairClosure{
			SceneScopeKeys: append([]string(nil), closure.SceneScopeKeys...),
			EntityKeys:     append([]string(nil), closure.EntityKeys...), StateKeys: append([]string(nil), closure.StateKeys...),
			OccurrenceKeys:  append([]string(nil), closure.OccurrenceKeys...),
			InteractionKeys: append([]string(nil), closure.InteractionKeys...),
			ContinuityKeys:  append([]string(nil), closure.ContinuityKeys...), LedgerKeys: append([]string(nil), closure.LedgerKeys...),
		},
		ReasonCode: payload.ChangeRequest.ReasonCode, BaseCandidate: baseCandidate,
	}
	if err = directive.ValidateFor(stageKey); err != nil {
		return nil, fmt.Errorf("Production World repair directive is invalid: %w", err)
	}
	encoded, err := json.Marshal(struct {
		Repair agentcontract.ProductionWorldRepairDirective `json:"production_world_repair"`
	}{Repair: directive})
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func loadProductionWorldRepairReview(
	transaction *gorm.DB,
	task model.HumanTask,
) (domain.ProductionWorldGateInput, domain.ProductionWorldReviewDetail, error) {
	var record model.WorkflowHumanGateInput
	if err := transaction.First(&record, "id = ?", task.SubjectID).Error; err != nil {
		return domain.ProductionWorldGateInput{}, domain.ProductionWorldReviewDetail{}, normalizeNotFound(err)
	}
	gate, _, err := domain.DecodeProductionWorldGateInput(json.RawMessage(record.Input))
	if err != nil || record.WorkspaceID != task.WorkspaceID || record.ProjectID != task.ProjectID ||
		record.WorkflowRunID != task.WorkflowRunID || record.NodeRunID != task.NodeRunID ||
		record.InputHash != task.SubjectHash || gate.InputHash != task.SubjectHash || task.SubjectRevision != 1 {
		return domain.ProductionWorldGateInput{}, domain.ProductionWorldReviewDetail{}, errors.New("Production World repair Gate input has drifted")
	}
	candidateID, err := uuid.Parse(gate.Subject.ProductionWorldCandidate.CandidateRevisionID)
	if err != nil {
		return domain.ProductionWorldGateInput{}, domain.ProductionWorldReviewDetail{}, errors.New("Production World repair Candidate identity is invalid")
	}
	var revision model.StageCandidateRevision
	if err = transaction.First(&revision, "id = ?", candidateID).Error; err != nil {
		return domain.ProductionWorldGateInput{}, domain.ProductionWorldReviewDetail{}, normalizeNotFound(err)
	}
	frozen := gate.Subject.ProductionWorldCandidate
	if revision.WorkspaceID != task.WorkspaceID || revision.OriginKind != "aggregate" ||
		revision.RevisionNo != frozen.CandidateRevision || revision.CandidateRevisionHash != frozen.CandidateRevisionHash ||
		revision.CandidateContentHash != frozen.CandidateContentHash {
		return domain.ProductionWorldGateInput{}, domain.ProductionWorldReviewDetail{}, errors.New("Production World repair Candidate has drifted")
	}
	candidate, _, err := worlddomain.DecodeProductionWorldCandidate(json.RawMessage(revision.Candidate))
	if err != nil {
		return domain.ProductionWorldGateInput{}, domain.ProductionWorldReviewDetail{}, err
	}
	detail, _, err := domain.NewProductionWorldReviewDetail(gate, candidate)
	return gate, detail, err
}

func loadProductionWorldRepairBaseCandidate(
	transaction *gorm.DB,
	run model.WorkflowRun,
	node model.NodeRunProjection,
	stageKey string,
) (agentcontract.ProductionWorldRepairBaseCandidate, error) {
	if run.SourceWorkflowRunID == nil {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, errors.New("Production World repair source Run is missing")
	}
	var sourceNode model.NodeRunProjection
	if err := transaction.Where(
		"workflow_run_id = ? AND node_id = ?", *run.SourceWorkflowRunID, node.NodeID,
	).First(&sourceNode).Error; err != nil {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, normalizeNotFound(err)
	}
	output, _, _, err := domain.ParseNodeOutput(json.RawMessage(sourceNode.Output))
	if err != nil || sourceNode.Status != "SUCCEEDED" || len(output.Bindings) != 1 {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, errors.New("Production World repair source stage output is invalid")
	}
	binding := output.Bindings[0]
	expectedType := map[string]string{
		"derive_production_entities":       "production_entity_fragment_candidate",
		"bind_scene_occurrences":           "scene_binding_fragment_candidate",
		"reconcile_interaction_continuity": "continuity_fragment_candidate",
	}[stageKey]
	candidateID, err := uuid.Parse(binding.ReferenceID)
	if err != nil || binding.Port != "candidate" || binding.ValueType != expectedType {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, errors.New("Production World repair source Candidate binding is invalid")
	}
	var revision model.SceneAnalysisCandidateRevision
	if err = transaction.First(&revision, "id = ?", candidateID).Error; err != nil {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, normalizeNotFound(err)
	}
	if revision.WorkspaceID != run.WorkspaceID || revision.ProjectID != run.ProjectID ||
		revision.CandidateType != expectedType ||
		strconv.FormatInt(revision.RevisionNo, 10) != binding.ReferenceVersion ||
		revision.CandidateRevisionHash != binding.ContentHash {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, errors.New("Production World repair source Candidate has drifted")
	}
	var invocation model.SceneAnalysisInvocationRecord
	if err = transaction.First(&invocation, "id = ?", revision.SourceInvocationID).Error; err != nil {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, normalizeNotFound(err)
	}
	if invocation.WorkspaceID != run.WorkspaceID || invocation.ProjectID != run.ProjectID ||
		invocation.WorkflowRunID != *run.SourceWorkflowRunID || invocation.NodeRunID != sourceNode.ID ||
		invocation.StageKey != stageKey || invocation.ShardKey != "script:full" || invocation.Status != "accepted" ||
		invocation.StageInstanceKey != revision.StageInstanceKey {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, errors.New("Production World repair source invocation has drifted")
	}
	var resultRecord model.SceneAnalysisResult
	if err = transaction.First(&resultRecord, "id = ?", revision.SourceResultID).Error; err != nil {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, normalizeNotFound(err)
	}
	decodedResult, decodeErr := agentcontract.DecodeSceneAnalysisAttemptResult(resultRecord.Result)
	computedResultHash, resultHashErr := decodedResult.ComputeResultHash()
	candidateContentHash, candidateHashErr := platformcanonical.Hash(json.RawMessage(revision.Candidate))
	if decodeErr != nil || resultHashErr != nil || candidateHashErr != nil || resultRecord.Status != "accepted" ||
		resultRecord.InputHash != invocation.InputHash || resultRecord.OutputHash == nil ||
		*resultRecord.OutputHash != revision.CandidateContentHash || decodedResult.Status != "accepted" ||
		decodedResult.ResultHash != revision.SourceResultHash || computedResultHash != revision.SourceResultHash ||
		decodedResult.OutputHash == nil || *decodedResult.OutputHash != revision.CandidateContentHash ||
		candidateContentHash != revision.CandidateContentHash || !equalJSON(decodedResult.Candidate, revision.Candidate) {
		return agentcontract.ProductionWorldRepairBaseCandidate{}, errors.New("Production World repair source result has drifted")
	}
	result := agentcontract.ProductionWorldRepairBaseCandidate{
		Identity: agentcontract.SceneAnalysisCandidateRevisionIdentity{
			StageKey: stageKey, ShardKey: "script:full", CandidateRevisionID: revision.ID.String(),
			CandidateRevisionHash: revision.CandidateRevisionHash,
			SourceInvocationID:    revision.SourceInvocationID.String(), SourceResultHash: revision.SourceResultHash,
		},
		CandidateType: expectedType, CandidateContentHash: revision.CandidateContentHash,
		Candidate: append(json.RawMessage(nil), revision.Candidate...),
	}
	return result, nil
}

func validateResolvedInputPorts(bindings []domain.NodeInputBinding, expected []authoring.PortDefinition) error {
	expectedByPort := make(map[string]authoring.PortDefinition, len(expected))
	for _, port := range expected {
		expectedByPort[port.Key] = port
	}
	observed := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		port, exists := expectedByPort[binding.Port]
		if !exists || port.ValueType != binding.ValueType {
			return errors.New("workflow node input port has drifted")
		}
		if _, duplicated := observed[binding.Port]; duplicated {
			return errors.New("workflow node input port is duplicated")
		}
		observed[binding.Port] = struct{}{}
	}
	for _, port := range expected {
		if _, exists := observed[port.Key]; port.Required && !exists {
			return errors.New("workflow node required input is missing")
		}
	}
	return nil
}

func executionPort(ports []authoring.PortDefinition, key string) (authoring.PortDefinition, bool) {
	for _, port := range ports {
		if port.Key == key {
			return port, true
		}
	}
	return authoring.PortDefinition{}, false
}

func outputBinding(output domain.NodeOutputSnapshot, port string) (domain.NodeOutputBinding, bool) {
	for _, binding := range output.Bindings {
		if binding.Port == port {
			return binding, true
		}
	}
	return domain.NodeOutputBinding{}, false
}
