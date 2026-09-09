package production

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	worldapp "github.com/StephenQiu30/lanverse/backend/internal/production/world/application"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	productionBibleConfirmOperation       = "production_bible.confirm"
	episodePlanConfirmOperation           = "episode_plan.confirm"
	episodePlanApplyOperation             = "episode_plan.apply"
	episodePlanningApplyOperation         = "episode_planning.apply"
	episodeStructureBatchConfirmOperation = "episode_structure.confirm_batch"
	storyboardFreezeIntentSetOperation    = "storyboard.freeze_intent_set"
)

type BibleOwner interface {
	Confirm(context.Context, bibleapp.Actor, bibleapp.ConfirmCommand) (bibleapp.ConfirmResult, error)
}

type StructureIdentityOwner interface {
	ConfirmStructureIdentitySet(context.Context, bibleapp.Actor, bibleapp.ConfirmStructureIdentitySetCommand) (bibledomain.ConfirmStructureIdentitySetResult, error)
}

type ProjectEpisodeOwner interface {
	ConfirmEpisodeLifecycle(context.Context, projectapp.Actor, projectapp.ConfirmEpisodeLifecycleCommand) (projectdomain.EpisodeLifecycleSet, error)
}

type PlanningConfirmationOwner interface {
	ConfirmPlan(context.Context, planningapp.Actor, planningapp.ConfirmPlanCommand) (planningapp.ConfirmPlanResult, error)
	ApplyEpisodePlan(context.Context, planningapp.Actor, planningapp.ApplyEpisodePlanCommand) (planningapp.ApplyEpisodePlanResult, error)
	ConfirmPublishedStructureBatch(context.Context, planningapp.Actor, planningapp.ConfirmStructureBatchCommand) (planningapp.ConfirmStructureBatchResult, error)
}

type EpisodePlanningOwner interface {
	ApplyEpisodePlanningCandidate(context.Context, planningapp.Actor, planningapp.ApplyEpisodePlanningCandidateCommand) (planningapp.ApplyEpisodePlanningCandidateResult, error)
}

type StoryboardSetOwner interface {
	FreezeIntentSet(context.Context, storyboardapp.Actor, storyboardapp.FreezeIntentSetCommand) (storyboardapp.FreezeIntentSetResult, error)
}

type ProductionWorldOwner interface {
	ConfirmProductionWorld(context.Context, worldapp.ConfirmProductionWorldCommand) (worlddomain.ConfirmProductionWorldResult, error)
}

type Applier struct {
	bibles              BibleOwner
	structureIdentities StructureIdentityOwner
	projects            ProjectEpisodeOwner
	plans               PlanningConfirmationOwner
	planningCandidates  EpisodePlanningOwner
	storyboards         StoryboardSetOwner
	productionWorlds    ProductionWorldOwner
}

func New(
	bibles BibleOwner,
	structureIdentities StructureIdentityOwner,
	projects ProjectEpisodeOwner,
	plans PlanningConfirmationOwner,
	planningCandidates EpisodePlanningOwner,
	storyboards StoryboardSetOwner,
	productionWorlds ProductionWorldOwner,
) *Applier {
	return &Applier{
		bibles: bibles, structureIdentities: structureIdentities, projects: projects,
		plans: plans, planningCandidates: planningCandidates, storyboards: storyboards,
		productionWorlds: productionWorlds,
	}
}

func (applier *Applier) ApplyHumanGateDecision(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier == nil || strings.TrimSpace(actor.UserID) == "" ||
		(application.Decision != "approved" && application.Decision != "selected") ||
		strings.TrimSpace(application.Candidate.ReferenceID) == "" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	if application.Executor == "gate.episode_plan_review" {
		return applier.applyEpisodePlan(ctx, actor, application)
	}
	if application.Executor == "gate.structure_identity_review" {
		return applier.applyStructureIdentity(ctx, actor, application)
	}
	if application.Executor == "gate.episode_structure_review" {
		return applier.applyEpisodeStructures(ctx, actor, application)
	}
	if application.Executor == "gate.storyboard_review" {
		return applier.freezeStoryboardIntents(ctx, actor, application)
	}
	if application.Executor == "gate.production_world_review" {
		return applier.applyProductionWorld(ctx, actor, application)
	}
	if applier.bibles == nil || application.Executor != "gate.production_bible_review" ||
		application.Candidate.ValueType != "story_reconciliation_candidate" ||
		application.OutputPort != "bible" || application.OutputValueType != "production_bible_version" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedCandidateRevision, err := strconv.ParseInt(application.Candidate.ReferenceVersion, 10, 64)
	if err != nil || expectedCandidateRevision < 1 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid production bible candidate revision")
	}
	var config struct {
		ExpectedVersion int `json:"expected_bible_version"`
	}
	if json.Unmarshal(application.NodeConfig, &config) != nil || config.ExpectedVersion < 1 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Production Bible Human Gate config")
	}
	var documentID, documentHash string
	for _, reference := range application.FrozenInputs {
		if reference.Kind != "script_revision" {
			continue
		}
		if documentID != "" {
			return domain.HumanGateOwnerResult{}, errors.New("production Bible Human Gate has multiple script revisions")
		}
		documentID, documentHash = reference.ID, reference.Hash
	}
	if documentID == "" || len(documentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("production Bible Human Gate has no frozen script revision")
	}
	result, err := applier.bibles.Confirm(ctx, bibleapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, bibleapp.ConfirmCommand{
		CandidateRevisionID:       application.Candidate.ReferenceID,
		CandidateRevisionHash:     application.Candidate.ContentHash,
		ExpectedCandidateRevision: expectedCandidateRevision,
		DocumentRevisionID:        documentID, DocumentRevisionHash: documentHash,
		ExpectedVersion: config.ExpectedVersion, ReviewDecisionID: application.ReviewDecisionID,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	if err = validateConfirmedBible(application, actor, result.Version, result.Receipt.Operation, result.Receipt.ResourceID,
		result.Receipt.WorkspaceID, result.Receipt.CreatedBy); err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: result.Version.ID, ReferenceVersion: strconv.Itoa(result.Version.Version),
			ContentHash: result.Version.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyProductionWorld(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier.productionWorlds == nil || application.Decision != "approved" ||
		application.Candidate.ValueType != "production_world_candidate" ||
		application.OutputPort != "world" || application.OutputValueType != "production_world_owner_set" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported Production World Human Gate owner application")
	}
	material, err := domain.DecodeProductionWorldOwnerMaterial(application.OwnerMaterial)
	if err != nil || material.GateInputID == "" || material.GateInput.WorkspaceID != application.WorkspaceID ||
		material.GateInput.ProjectID != application.ProjectID || material.GateInput.WorkflowRunID != application.WorkflowRunID ||
		material.GateInput.NodeRunID != application.NodeRunID || material.GateInput.InputHash == "" ||
		material.GateInput.Subject.ProductionWorldCandidate.CandidateRevisionID != application.Candidate.ReferenceID ||
		material.GateInput.Subject.ProductionWorldCandidate.CandidateRevisionHash != application.Candidate.ContentHash ||
		material.Candidate.ContentHash != material.GateInput.Subject.ProductionWorldCandidate.CandidateContentHash {
		return domain.HumanGateOwnerResult{}, errors.New("Production World Human Gate material has drifted")
	}
	expectedHeads := make([]worldapp.ExpectedHead, len(material.GateInput.Subject.ExpectedHeads))
	for index, head := range material.GateInput.Subject.ExpectedHeads {
		expectedHeads[index] = worldapp.ExpectedHead{
			OwnerKind: head.OwnerKind, VersionFamily: head.VersionFamily, ScopeKind: head.ScopeKind,
			ScopeKey: head.ScopeKey, Revision: head.Revision, ContentHash: head.ContentHash,
		}
	}
	commandID := uuid.NewSHA1(
		uuid.NameSpaceURL,
		[]byte("lanverse:confirm-production-world:"+application.ReviewDecisionID),
	).String()
	candidate := material.GateInput.Subject.ProductionWorldCandidate
	result, err := applier.productionWorlds.ConfirmProductionWorld(ctx, worldapp.ConfirmProductionWorldCommand{
		CommandID: commandID, WorkspaceID: application.WorkspaceID, ProjectID: application.ProjectID,
		ActorID: actor.UserID, GateInputID: material.GateInputID, GateInputHash: material.GateInput.InputHash,
		ReviewDecisionID: application.ReviewDecisionID, CandidateRevisionID: candidate.CandidateRevisionID,
		CandidateRevision: candidate.CandidateRevision, CandidateRevisionHash: candidate.CandidateRevisionHash,
		IdempotencyKey: "workflow-production-world:" + application.ReviewDecisionID,
		ExpectedHeads:  expectedHeads, Candidate: material.Candidate,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	verified, verifyErr := worlddomain.CompleteConfirmProductionWorldResult(result)
	if verifyErr != nil || verified.ResultContentHash != result.ResultContentHash ||
		verified.ReceiptContentHash != result.ReceiptContentHash || result.CommandID != commandID ||
		result.CommandReceiptID == "" || result.CommandContractID != worlddomain.ConfirmProductionWorldContract ||
		result.SubjectRef.VersionID != candidate.CandidateRevisionID ||
		result.SubjectRef.Revision != candidate.CandidateRevision || result.SubjectRef.ContentHash != candidate.CandidateRevisionHash ||
		result.CommittedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("Production World owner result does not match Workflow Gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: result.CommandReceiptID, ReferenceVersion: "1", ContentHash: result.ReceiptContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.CommandReceiptID, Operation: worlddomain.ConfirmProductionWorldOperation,
		Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyStructureIdentity(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier.projects == nil || applier.structureIdentities == nil || application.Decision != "approved" ||
		application.Candidate.ValueType != "structure_identity_review_candidate" ||
		application.OutputPort != "identities" || application.OutputValueType != "structure_identity_set_version" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported Structure Identity Human Gate owner application")
	}
	material, err := domain.DecodeStructureIdentityOwnerMaterial(application.OwnerMaterial)
	if err != nil || material.GateInput.WorkspaceID != application.WorkspaceID ||
		material.GateInput.ProjectID != application.ProjectID || material.GateInput.WorkflowRunID != application.WorkflowRunID ||
		material.GateInput.NodeRunID != application.NodeRunID || material.GateInput.InputHash == "" ||
		material.GateInput.Subject.ReviewCandidate.CandidateRevisionID != application.Candidate.ReferenceID ||
		material.GateInput.Subject.ReviewCandidate.CandidateRevisionHash != application.Candidate.ContentHash {
		return domain.HumanGateOwnerResult{}, errors.New("Structure Identity Human Gate material has drifted")
	}
	byStage := make(map[string]domain.StructureIdentityOwnerCandidate, len(material.Candidates))
	for _, candidate := range material.Candidates {
		byStage[candidate.Identity.StageKey] = candidate
	}
	var spans agentcontract.ScriptSpanCandidate
	var facts agentcontract.SceneFactCandidate
	var identities agentcontract.IdentityResolutionCandidate
	var review agentcontract.StructureIdentityReviewCandidate
	if json.Unmarshal(byStage["propose_script_spans"].Candidate, &spans) != nil ||
		json.Unmarshal(byStage["extract_scene_facts"].Candidate, &facts) != nil ||
		json.Unmarshal(byStage["resolve_identities"].Candidate, &identities) != nil ||
		json.Unmarshal(byStage["review_candidate"].Candidate, &review) != nil ||
		spans.SourceVersionID != material.GateInput.Subject.SourceVersion.VersionID ||
		facts.SourceVersionID != spans.SourceVersionID || identities.SourceVersionID != spans.SourceVersionID ||
		review.SourceVersionID != spans.SourceVersionID || slices.ContainsFunc(review.ReviewIssues, func(issue agentcontract.CandidateReviewIssue) bool {
		return issue.Severity == "blocking"
	}) {
		return domain.HumanGateOwnerResult{}, errors.New("Structure Identity reviewed Candidate set has drifted")
	}
	projectStep, bibleStep, err := structureIdentityEffectSteps(material.GateInput)
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	episodeSpans := make([]projectdomain.EpisodeLifecycleSpan, len(spans.Episodes))
	for index, episode := range spans.Episodes {
		heading := ""
		if episode.Heading != nil {
			heading = *episode.Heading
		}
		episodeSpans[index] = projectdomain.EpisodeLifecycleSpan{
			TemporaryEpisodeID: episode.TemporaryEpisodeID, Position: episode.Position,
			SourceStart: episode.CodepointStart, SourceEnd: episode.CodepointEnd, Heading: heading,
		}
	}
	projectResult, err := applier.projects.ConfirmEpisodeLifecycle(ctx, projectapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, projectapp.ConfirmEpisodeLifecycleCommand{
		WorkspaceID: application.WorkspaceID, ProjectID: application.ProjectID,
		GateInputID: material.GateInputID, GateInputHash: material.GateInput.InputHash,
		ReviewDecisionID:        application.ReviewDecisionID,
		SourceVersionID:         material.GateInput.Subject.SourceVersion.VersionID,
		SourceHash:              material.GateInput.Subject.SourceVersion.ContentHash,
		ExpectedProjectRevision: int(projectStep.ExpectedHead.Revision),
		ExpectedActiveOrderHash: projectStep.ExpectedHead.ContentHash,
		EpisodeSpans:            episodeSpans,
		IdempotencyKey:          "workflow-structure-identity-project:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	command, err := buildStructureIdentityCommand(application, material, spans, facts, identities, projectResult, bibleStep)
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	bibleResult, err := applier.structureIdentities.ConfirmStructureIdentitySet(ctx, bibleapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, command)
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	version := bibleResult.Version
	if version.WorkspaceID != application.WorkspaceID || version.ProjectID != application.ProjectID ||
		version.GateInputID != material.GateInputID || version.GateInputHash != material.GateInput.InputHash ||
		version.ReviewDecisionID != application.ReviewDecisionID || version.ProjectEpisodeReceiptID != projectResult.ID ||
		bibleResult.CommandOperation != bibledomain.StructureIdentityCommandOperation ||
		bibleResult.CommandReceiptID == "" || bibleResult.Receipt.VersionID != version.ID ||
		bibleResult.Receipt.CheckpointKey != bibledomain.StructureIdentityCheckpointKey ||
		bibleResult.Receipt.CollectionFamily != bibledomain.StructureIdentityCollectionFamily {
		return domain.HumanGateOwnerResult{}, errors.New("Structure Identity owner result does not match Workflow Gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: version.ID, ReferenceVersion: strconv.Itoa(version.Version), ContentHash: version.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: bibleResult.CommandReceiptID, Operation: bibleResult.CommandOperation,
		Output: output, OutputHash: outputHash,
	}, nil
}

func structureIdentityEffectSteps(
	gate domain.StructureIdentityGateInput,
) (domain.StructureIdentityEffectStep, domain.StructureIdentityEffectStep, error) {
	if len(gate.EffectPlan.Steps) != 2 {
		return domain.StructureIdentityEffectStep{}, domain.StructureIdentityEffectStep{}, errors.New("Structure Identity effect plan is incomplete")
	}
	projectStep, bibleStep := gate.EffectPlan.Steps[0], gate.EffectPlan.Steps[1]
	if projectStep.OwnerKind != "production/project" || projectStep.OwnerCommand != "confirm_project_episode_lifecycle" ||
		bibleStep.OwnerKind != "production/bible" || bibleStep.OwnerCommand != "confirm_structure_identity_set" {
		return domain.StructureIdentityEffectStep{}, domain.StructureIdentityEffectStep{}, errors.New("Structure Identity effect plan has drifted")
	}
	return projectStep, bibleStep, nil
}

func buildStructureIdentityCommand(
	application domain.HumanGateOwnerApplication,
	material domain.StructureIdentityOwnerMaterial,
	spans agentcontract.ScriptSpanCandidate,
	facts agentcontract.SceneFactCandidate,
	identities agentcontract.IdentityResolutionCandidate,
	episodes projectdomain.EpisodeLifecycleSet,
	bibleStep domain.StructureIdentityEffectStep,
) (bibleapp.ConfirmStructureIdentitySetCommand, error) {
	projectID, err := uuid.Parse(application.ProjectID)
	if err != nil {
		return bibleapp.ConfirmStructureIdentitySetCommand{}, errors.New("Structure Identity Project identity is invalid")
	}
	episodesByTemporaryID := make(map[string]projectdomain.EpisodeLifecycleEpisodeRef, len(episodes.Episodes))
	for _, episode := range episodes.Episodes {
		episodesByTemporaryID[episode.TemporaryEpisodeID] = episode
	}
	sceneFactsBySpan := make(map[string]agentcontract.SceneFact, len(facts.Scenes))
	for _, scene := range facts.Scenes {
		sceneFactsBySpan[scene.SpanID] = scene
	}
	scenes := make([]bibledomain.StructureIdentitySceneRef, len(spans.Spans))
	for index, span := range spans.Spans {
		episode := episodesByTemporaryID[span.EpisodeSpanID]
		fact, exists := sceneFactsBySpan[span.TemporarySpanID]
		if episode.EpisodeID == "" || !exists || fact.SourceStart != span.CodepointStart || fact.SourceEnd != span.CodepointEnd {
			return bibleapp.ConfirmStructureIdentitySetCommand{}, errors.New("Structure Identity Scene mapping has drifted")
		}
		sceneID := uuid.NewSHA1(projectID, []byte("lanverse:scene:"+span.TemporarySpanID)).String()
		scenes[index] = bibledomain.StructureIdentitySceneRef{
			TemporaryEpisodeID: span.EpisodeSpanID, EpisodeID: episode.EpisodeID,
			TemporarySpanID: span.TemporarySpanID, TemporarySceneID: fact.TemporarySceneID,
			SceneOwnerLogicalID: sceneID, ScopeKey: "scene:" + sceneID,
			SourceStart: span.CodepointStart, SourceEnd: span.CodepointEnd, EvidenceHash: span.Evidence.TextHash,
		}
	}
	identityValues := make([]bibledomain.StructureIdentity, len(identities.ResolvedClusters))
	mentionMappings := make([]bibledomain.StructureIdentityMentionMapping, 0, identities.Coverage.MentionCount)
	for index, cluster := range identities.ResolvedClusters {
		identityKey := uuid.NewSHA1(projectID, []byte("lanverse:identity:"+cluster.TemporaryIdentityKey)).String()
		if cluster.Resolution == "reuse" && cluster.ReuseIdentityKey != nil {
			identityKey = *cluster.ReuseIdentityKey
		}
		identityValues[index] = bibledomain.StructureIdentity{
			TemporaryIdentityKey: cluster.TemporaryIdentityKey, IdentityKey: identityKey,
			Kind: cluster.Kind, Resolution: cluster.Resolution, ReuseIdentityKey: cluster.ReuseIdentityKey,
			CanonicalName: cluster.CanonicalName, Aliases: append([]string(nil), cluster.Aliases...),
		}
		for _, mention := range cluster.MentionRefs {
			key := identityKey
			mentionMappings = append(mentionMappings, structureIdentityMentionMapping(mention, "resolved", &key))
		}
	}
	for _, ambiguous := range identities.AmbiguousMentions {
		mentionMappings = append(mentionMappings, structureIdentityMentionMapping(ambiguous.MentionRef, "unresolved", nil))
	}
	for _, rejected := range identities.RejectedMentions {
		mentionMappings = append(mentionMappings, structureIdentityMentionMapping(rejected.MentionRef, "unresolved", nil))
	}
	slices.SortFunc(mentionMappings, func(left, right bibledomain.StructureIdentityMentionMapping) int {
		if left.SourceStart != right.SourceStart {
			return left.SourceStart - right.SourceStart
		}
		if left.SourceEnd != right.SourceEnd {
			return left.SourceEnd - right.SourceEnd
		}
		if compared := strings.Compare(left.Kind, right.Kind); compared != 0 {
			return compared
		}
		return strings.Compare(left.TemporarySceneID, right.TemporarySceneID)
	})
	mentionUniverseHash, err := platformcommand.InputHash(mentionMappings)
	if err != nil {
		return bibleapp.ConfirmStructureIdentitySetCommand{}, err
	}
	scopes := make([]string, len(scenes))
	for index, scene := range scenes {
		scopes[index] = scene.ScopeKey
	}
	scopeSetHash, err := platformcommand.InputHash(scopes)
	if err != nil {
		return bibleapp.ConfirmStructureIdentitySetCommand{}, err
	}
	candidateRefs := make([]bibledomain.StructureIdentityCandidateRef, len(material.Candidates))
	for index, candidate := range material.Candidates {
		candidateRefs[index] = bibledomain.StructureIdentityCandidateRef{
			StageKey: candidate.Identity.StageKey, ShardKey: candidate.Identity.ShardKey,
			CandidateRevisionID:   candidate.Identity.CandidateRevisionID,
			CandidateRevisionHash: candidate.Identity.CandidateRevisionHash,
			SourceInvocationID:    candidate.Identity.SourceInvocationID, SourceResultHash: candidate.Identity.SourceResultHash,
			SkillReleaseID: candidate.Release.SkillReleaseID, SkillReleaseHash: candidate.Release.SkillReleaseHash,
			StageReleaseHash: candidate.Release.StageReleaseHash, BundleContentHash: candidate.Release.BundleContentHash,
			AgentImageDigest: candidate.Release.AgentImageDigest,
		}
	}
	return bibleapp.ConfirmStructureIdentitySetCommand{
		WorkspaceID: application.WorkspaceID, ProjectID: application.ProjectID,
		GateInputID: material.GateInputID, GateInputHash: material.GateInput.InputHash,
		ReviewDecisionID: application.ReviewDecisionID, ProjectEpisodeReceiptID: episodes.ID,
		DocumentRevisionID:   material.GateInput.Subject.SourceVersion.VersionID,
		DocumentRevisionHash: material.GateInput.Subject.SourceVersion.ContentHash,
		SpanIndexID:          material.SpanIndexID, SpanIndexHash: material.SpanIndexHash,
		ExpectedHeadRevision: bibleStep.ExpectedHead.Revision, ExpectedHeadHash: bibleStep.ExpectedHead.ContentHash,
		CandidateRefs: candidateRefs, SceneRefs: scenes, Identities: identityValues, MentionMappings: mentionMappings,
		Coverage: bibledomain.StructureIdentityCoverage{
			SceneCount: len(scenes), IdentityCount: len(identityValues), MentionCount: len(mentionMappings),
			ResolvedCount:       identities.Coverage.ResolvedCount,
			UnresolvedCount:     identities.Coverage.AmbiguousCount + identities.Coverage.RejectedCount,
			MentionUniverseHash: mentionUniverseHash, ScopeSetHash: scopeSetHash,
		},
		IdempotencyKey: "workflow-structure-identity-bible:" + application.ReviewDecisionID,
	}, nil
}

func structureIdentityMentionMapping(
	mention agentcontract.IdentityMentionRef,
	resolution string,
	identityKey *string,
) bibledomain.StructureIdentityMentionMapping {
	return bibledomain.StructureIdentityMentionMapping{
		Kind: mention.Kind, OccurrenceRole: mention.OccurrenceRole, TemporarySceneID: mention.TemporarySceneID,
		SourceStart: mention.SourceStart, SourceEnd: mention.SourceEnd,
		TextHash: mention.TextHash, ExactAnchor: mention.ExactAnchor,
		Resolution: resolution, IdentityKey: identityKey,
	}
}

func (applier *Applier) freezeStoryboardIntents(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier.storyboards == nil || application.Candidate.ValueType != "storyboard_intent_candidate_set" ||
		application.OutputPort != "intents" || application.OutputValueType != "approved_storyboard_intents" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedRevision, err := strconv.ParseInt(application.Candidate.ReferenceVersion, 10, 64)
	if err != nil || expectedRevision < 1 || len(application.Candidate.ContentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Storyboard Intent Candidate")
	}
	result, err := applier.storyboards.FreezeIntentSet(ctx, storyboardapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, storyboardapp.FreezeIntentSetCommand{
		WorkspaceID: application.WorkspaceID, ProjectID: application.ProjectID,
		CandidateRevisionID:       application.Candidate.ReferenceID,
		CandidateRevisionHash:     application.Candidate.ContentHash,
		ExpectedCandidateRevision: expectedRevision, ReviewDecisionID: application.ReviewDecisionID,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	set, approved := result.Set, result.Approved
	if approved.ID != result.Receipt.ID || approved.WorkspaceID != application.WorkspaceID ||
		approved.ProjectID != application.ProjectID || approved.DraftSetID != set.ID ||
		approved.CandidateRevisionID != application.Candidate.ReferenceID ||
		approved.CandidateRevisionHash != application.Candidate.ContentHash ||
		approved.CandidateRevision != expectedRevision || approved.ReviewDecisionID != application.ReviewDecisionID ||
		len(approved.Scenes) == 0 || len(approved.VisualRequirementsHash) != 64 || len(approved.ContentHash) != 64 ||
		set.WorkspaceID != application.WorkspaceID || set.ProjectID != application.ProjectID ||
		set.Status != "intent_frozen" || set.Revision != approved.DraftSetRevision+1 ||
		set.ResultHash == nil || *set.ResultHash != approved.ContentHash ||
		result.Receipt.Operation != storyboardFreezeIntentSetOperation || result.Receipt.ResourceID != set.ID ||
		result.Receipt.WorkspaceID != application.WorkspaceID || result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("storyboard owner result does not match workflow gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: approved.ID, ReferenceVersion: "1", ContentHash: approved.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyEpisodeStructures(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if application.Candidate.ValueType == "episode_planning_candidate_set" {
		return applier.applyEpisodePlanning(ctx, actor, application)
	}
	if applier.plans == nil || application.Candidate.ValueType != "episode_structure_candidate" ||
		application.OutputPort != "structures" || application.OutputValueType != "episode_structures" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedRevision, err := strconv.Atoi(application.Candidate.ReferenceVersion)
	if err != nil || expectedRevision < 1 || len(application.Candidate.ContentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Episode Structure batch candidate")
	}
	result, err := applier.plans.ConfirmPublishedStructureBatch(ctx, planningapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, planningapp.ConfirmStructureBatchCommand{
		CommitID: application.Candidate.ReferenceID, ExpectedRevision: expectedRevision,
		ExpectedContentHash: application.Candidate.ContentHash,
		IdempotencyKey:      "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	batch := result.Batch
	if batch.Commit.ID != application.Candidate.ReferenceID || batch.Commit.Status != "published" ||
		batch.Commit.Revision != expectedRevision || batch.Commit.WorkspaceID != application.WorkspaceID ||
		batch.Commit.ProjectID != application.ProjectID || batch.ContentHash != application.Candidate.ContentHash ||
		len(batch.Structures) == 0 || result.Receipt.Operation != episodeStructureBatchConfirmOperation ||
		result.Receipt.ResourceID != batch.Commit.ID || result.Receipt.WorkspaceID != application.WorkspaceID ||
		result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("episode Structure owner result does not match workflow gate")
	}
	for _, structure := range batch.Structures {
		if structure.WorkspaceID != application.WorkspaceID || structure.ProjectID != application.ProjectID ||
			structure.Status != "confirmed" || structure.ConfirmedBy == nil || *structure.ConfirmedBy != actor.UserID {
			return domain.HumanGateOwnerResult{}, errors.New("episode Structure owner result does not match workflow gate")
		}
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: batch.Commit.ID, ReferenceVersion: strconv.Itoa(batch.Commit.Revision),
			ContentHash: batch.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, normalizeOwnerApplyError(err)
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyEpisodePlanning(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier.planningCandidates == nil || application.OutputPort != "structures" ||
		application.OutputValueType != "planning_owner_set" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported Episode Planning Human Gate owner application")
	}
	expectedRevision, err := strconv.ParseInt(application.Candidate.ReferenceVersion, 10, 64)
	if err != nil || expectedRevision < 1 || len(application.Candidate.ContentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Episode Planning Candidate revision")
	}
	result, err := applier.planningCandidates.ApplyEpisodePlanningCandidate(ctx, planningapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, planningapp.ApplyEpisodePlanningCandidateCommand{
		WorkspaceID: application.WorkspaceID, ProjectID: application.ProjectID,
		CandidateRevisionID:       application.Candidate.ReferenceID,
		CandidateRevisionHash:     application.Candidate.ContentHash,
		ExpectedCandidateRevision: expectedRevision, ReviewDecisionID: application.ReviewDecisionID,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	set := result.Set
	if set.ID != result.Receipt.ID || set.WorkspaceID != application.WorkspaceID || set.ProjectID != application.ProjectID ||
		set.CandidateRevisionID != application.Candidate.ReferenceID ||
		set.CandidateRevisionHash != application.Candidate.ContentHash || set.CandidateRevision != expectedRevision ||
		set.ReviewDecisionID != application.ReviewDecisionID || len(set.ContentHash) != 64 || len(set.Structures) == 0 ||
		result.Receipt.Operation != episodePlanningApplyOperation ||
		result.Receipt.ResourceID != application.Candidate.ReferenceID || result.Receipt.WorkspaceID != application.WorkspaceID ||
		result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("episode Planning owner result does not match workflow gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: set.ID, ReferenceVersion: "1", ContentHash: set.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyEpisodePlan(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if application.Candidate.ValueType == "episode_segmentation_candidate" {
		return applier.applyEpisodeSegmentation(ctx, actor, application)
	}
	if applier.plans == nil || application.Candidate.ValueType != "episode_plan_candidate" ||
		application.OutputPort != "episodes" || application.OutputValueType != "episode_plan" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedRevision, err := strconv.Atoi(application.Candidate.ReferenceVersion)
	if err != nil || expectedRevision < 1 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Episode Plan candidate revision")
	}
	result, err := applier.plans.ConfirmPlan(ctx, planningapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, planningapp.ConfirmPlanCommand{
		PlanID: application.Candidate.ReferenceID, ExpectedRevision: expectedRevision,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	plan := result.View.Plan
	if err = validateConfirmedEpisodePlan(application, actor, plan, expectedRevision,
		result.Receipt.Operation, result.Receipt.ResourceID, result.Receipt.WorkspaceID, result.Receipt.CreatedBy); err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: plan.ID, ReferenceVersion: strconv.Itoa(plan.Revision), ContentHash: plan.InputHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func (applier *Applier) applyEpisodeSegmentation(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier.plans == nil || application.OutputPort != "episodes" || application.OutputValueType != "episode_set" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported Episode segmentation Human Gate owner application")
	}
	expectedRevision, err := strconv.ParseInt(application.Candidate.ReferenceVersion, 10, 64)
	if err != nil || expectedRevision < 1 || len(application.Candidate.ContentHash) != 64 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid Episode segmentation Candidate revision")
	}
	result, err := applier.plans.ApplyEpisodePlan(ctx, planningapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, planningapp.ApplyEpisodePlanCommand{
		CandidateRevisionID: application.Candidate.ReferenceID, CandidateRevisionHash: application.Candidate.ContentHash,
		ExpectedCandidateRevision: expectedRevision, ReviewDecisionID: application.ReviewDecisionID,
		IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	set := result.Set
	if set.ID != result.Receipt.ID || set.WorkspaceID != application.WorkspaceID || set.ProjectID != application.ProjectID ||
		set.CandidateRevisionID != application.Candidate.ReferenceID ||
		set.CandidateRevisionHash != application.Candidate.ContentHash || set.CandidateRevision != expectedRevision ||
		len(set.ContentHash) != 64 || len(set.Episodes) == 0 || result.Receipt.Operation != episodePlanApplyOperation ||
		result.Receipt.ResourceID != application.Candidate.ReferenceID || result.Receipt.WorkspaceID != application.WorkspaceID ||
		result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("episode Plan owner result does not match workflow gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: set.ID, ReferenceVersion: "1", ContentHash: set.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func normalizeOwnerApplyError(err error) error {
	var workflowError *workflowapp.Error
	if errors.As(err, &workflowError) {
		return err
	}
	var bibleError *bibleapp.Error
	if errors.As(err, &bibleError) {
		return &workflowapp.Error{
			Code: bibleError.Code, Message: bibleError.Message,
			NextAction: bibleError.NextAction, Status: bibleError.Status,
		}
	}
	var planningError *planningapp.Error
	if errors.As(err, &planningError) {
		return &workflowapp.Error{
			Code: planningError.Code, Message: planningError.Message,
			NextAction: planningError.NextAction, Status: planningError.Status,
		}
	}
	var projectError *projectapp.Error
	if errors.As(err, &projectError) {
		return &workflowapp.Error{
			Code: projectError.Code, Message: projectError.Message,
			NextAction: projectError.NextAction, Status: projectError.Status,
		}
	}
	var storyboardError *storyboardapp.Error
	if errors.As(err, &storyboardError) {
		return &workflowapp.Error{
			Code: storyboardError.Code, Message: storyboardError.Message,
			NextAction: storyboardError.NextAction, Status: storyboardError.Status,
		}
	}
	if errors.Is(err, worldapp.ErrProductionWorldConfirmationConflict) {
		return &workflowapp.Error{
			Code: "resource_conflict", Message: "Production World confirmation input has changed", Status: 409,
		}
	}
	return err
}

func validateConfirmedBible(
	application domain.HumanGateOwnerApplication,
	actor workflowapp.Actor,
	bible bibledomain.ProductionBibleVersion,
	operation string,
	resourceID string,
	workspaceID string,
	createdBy string,
) error {
	if bible.ID == application.Candidate.ReferenceID || bible.CandidateRevisionID != application.Candidate.ReferenceID ||
		bible.CandidateRevisionHash != application.Candidate.ContentHash || bible.WorkspaceID != application.WorkspaceID ||
		bible.ProjectID != application.ProjectID || bible.ReviewDecisionID != application.ReviewDecisionID ||
		operation != productionBibleConfirmOperation || resourceID != bible.ID ||
		workspaceID != application.WorkspaceID || createdBy != actor.UserID {
		return errors.New("production bible owner result does not match workflow gate")
	}
	return nil
}

func validateConfirmedEpisodePlan(
	application domain.HumanGateOwnerApplication,
	actor workflowapp.Actor,
	plan planningdomain.Plan,
	expectedRevision int,
	operation string,
	resourceID string,
	workspaceID string,
	createdBy string,
) error {
	if plan.Status != "confirmed" || plan.ID != application.Candidate.ReferenceID || plan.Revision != expectedRevision+1 ||
		plan.WorkspaceID != application.WorkspaceID || plan.ProjectID != application.ProjectID ||
		plan.InputHash != application.Candidate.ContentHash || operation != episodePlanConfirmOperation ||
		resourceID != plan.ID || workspaceID != application.WorkspaceID || createdBy != actor.UserID {
		return errors.New("episode Plan owner result does not match workflow gate")
	}
	return nil
}

var _ workflowapp.HumanGateOwnerApplier = (*Applier)(nil)
