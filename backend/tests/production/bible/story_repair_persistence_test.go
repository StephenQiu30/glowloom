package bible_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestStoryCandidateRepairPersistsOneReceiptAndExactStaleClosure(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Candidate repair transaction journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 28, 3, 0, 0, 0, time.UTC)
	user := model.UserAccount{
		ID: uuid.New(), EmailNormalized: uuid.NewString() + "@example.com", PasswordHash: "test-only",
		TokenVersion: 1, DisplayName: "Repair Owner", Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	workspace := model.Workspace{
		ID: uuid.New(), Name: "Candidate Repair Receipt", Status: "active", Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	membership := model.Membership{
		ID: uuid.New(), WorkspaceID: workspace.ID, UserID: user.ID,
		Role: "owner", Status: "active", JoinedAt: now,
	}
	project := model.Project{
		ID: uuid.New(), WorkspaceID: workspace.ID, Name: "Repair Project", AspectRatio: "16:9",
		Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	for _, record := range []any{&user, &workspace, &membership, &project} {
		if err = database.Create(record).Error; err != nil {
			t.Fatalf("seed Candidate repair owner: %v", err)
		}
	}

	evidence := domain.Evidence{
		SourceStart: 0, SourceEnd: 2, TextHash: domain.SourceTextHash("林一"), ExactAnchor: "林一",
	}
	baseCandidate := storyReconciliationReviewCandidate(evidence)
	baseCandidateJSON := mustJSON(t, baseCandidate)
	baseContentHash, err := agentcontract.CanonicalHash(baseCandidateJSON)
	if err != nil {
		t.Fatal(err)
	}
	baseStageKey := contractHashForRepair(t, workspace.ID.String()+":reconcile-root")
	aggregateOrigin := agentcontract.AggregateCandidateOrigin{
		ShardManifestID: uuid.NewString(), ManifestVersion: 1,
		ShardManifestHash: strings.Repeat("1", 64),
		LeafCandidates: []agentcontract.AggregateLeafCandidateRef{{
			StageInstanceKey: strings.Repeat("2", 64), ShardKey: "story-reduce:root",
			CandidateRevisionID: uuid.NewString(), CandidateRevisionHash: strings.Repeat("3", 64),
		}},
	}
	baseRevisionHash, err := (agentcontract.CandidateRevisionMaterial{
		StageInstanceKey: baseStageKey, RevisionNo: 1, OriginKind: "aggregate",
		AggregateOrigin: &aggregateOrigin, CandidateContentHash: baseContentHash,
	}).Hash()
	if err != nil {
		t.Fatal(err)
	}
	aggregateOriginJSON, _ := json.Marshal(aggregateOrigin)
	baseRevision := model.StageCandidateRevision{
		ID: uuid.New(), WorkspaceID: workspace.ID, StageInstanceKey: baseStageKey,
		RevisionNo: 1, OriginKind: "aggregate", AggregateOrigin: aggregateOriginJSON,
		Candidate: append([]byte(nil), baseCandidateJSON...), CandidateContentHash: baseContentHash,
		CandidateRevisionHash: baseRevisionHash, CreatedAt: now,
	}
	baseHead := model.StageCandidateHead{
		WorkspaceID: workspace.ID, StageInstanceKey: baseStageKey,
		CurrentRevisionID: baseRevision.ID, CurrentCandidateRevisionHash: baseRevisionHash,
		Revision: 1, UpdatedAt: now,
	}
	if err = database.Create(&baseRevision).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Create(&baseHead).Error; err != nil {
		t.Fatal(err)
	}

	sourceRef := agentcontract.StageSourceRef{
		OwnerKind: "production/script", OwnerLogicalID: uuid.NewString(), OwnerVersionID: uuid.NewString(),
		Revision: 1, ContentHash: strings.Repeat("4", 64),
	}
	baseUpstream := agentcontract.StageUpstreamCandidateRef{
		Stage: "reconcile_story", ShardKey: "story-reduce:root",
		CandidateRevisionID: baseRevision.ID.String(), CandidateRevisionHash: baseRevisionHash,
		SourceInvocationID: uuid.NewString(), SourceResultHash: baseContentHash,
	}
	reviewInput := agentcontract.StoryGraphReviewStageInput{
		ReviewedStage: "reconcile_story", TargetCandidateRevisionID: baseRevision.ID.String(),
		TargetCandidateRevisionHash: baseRevisionHash, CandidateItemStart: 0, CandidateItemEnd: 1,
		TargetCandidate: baseCandidateJSON,
		DeterministicGate: agentcontract.StoryGraphDeterministicGateResult{
			GateVersion:               agentcontract.BibleDeterministicGateVersion,
			TargetCandidateRevisionID: baseRevision.ID.String(), TargetCandidateRevisionHash: baseRevisionHash,
			Blockers: []agentcontract.StoryGraphGateBlocker{},
		},
	}
	reviewInputJSON := mustJSON(t, reviewInput)
	reviewRequest := newRepairJourneyInvocation(
		t, workspace.ID, project.ID, "review_storygraph", "review:bible:0000", "story_review",
		sourceRef, []agentcontract.StageUpstreamCandidateRef{baseUpstream}, reviewInputJSON,
	)
	targetIssue := agentcontract.StoryGraphReviewIssue{
		IssueKey: "issue:canonical", Code: "canonical_name_ambiguous", Severity: "blocking",
		Scope: "entity", SubjectKey: stringPointer("character:lin-yi"), Summary: "规范名冲突",
		Evidence: []agentcontract.StoryGraphEvidence{{
			SourceStart: evidence.SourceStart, SourceEnd: evidence.SourceEnd,
			TextHash: evidence.TextHash, ExactAnchor: evidence.ExactAnchor,
		}},
	}
	reviewCandidate := agentcontract.StoryGraphReviewCandidate{
		ReviewedStage: "reconcile_story", TargetCandidateRevisionID: baseRevision.ID.String(),
		TargetCandidateRevisionHash: baseRevisionHash,
		ReviewIssues:                []agentcontract.StoryGraphReviewIssue{targetIssue},
	}
	reviewCandidateJSON := mustJSON(t, reviewCandidate)
	reviewInvocation, reviewRevision, reviewHead := persistedInvocationCandidate(
		t, reviewRequest, workspace.ID, "storygraph_review", reviewCandidateJSON, now,
	)
	for _, record := range []any{&reviewInvocation, &reviewRevision, &reviewHead} {
		if err = database.Create(record).Error; err != nil {
			t.Fatalf("seed Review dependency: %v", err)
		}
	}
	reviewAggregateStageKey := contractHashForRepair(t, workspace.ID.String()+":review-aggregate")
	reviewAggregateOrigin := agentcontract.AggregateCandidateOrigin{
		ShardManifestID: uuid.NewString(), ManifestVersion: 1,
		ShardManifestHash: strings.Repeat("a", 64),
		LeafCandidates: []agentcontract.AggregateLeafCandidateRef{{
			StageInstanceKey: reviewInvocation.StageInstanceKey, ShardKey: "review:bible:0000",
			CandidateRevisionID:   reviewRevision.ID.String(),
			CandidateRevisionHash: reviewRevision.CandidateRevisionHash,
		}},
	}
	reviewAggregateHash, err := (agentcontract.CandidateRevisionMaterial{
		StageInstanceKey: reviewAggregateStageKey, RevisionNo: 1, OriginKind: "aggregate",
		AggregateOrigin: &reviewAggregateOrigin, CandidateContentHash: reviewRevision.CandidateContentHash,
	}).Hash()
	if err != nil {
		t.Fatal(err)
	}
	reviewAggregateOriginJSON := mustJSON(t, reviewAggregateOrigin)
	reviewAggregateRevision := model.StageCandidateRevision{
		ID: uuid.New(), WorkspaceID: workspace.ID, StageInstanceKey: reviewAggregateStageKey,
		RevisionNo: 1, OriginKind: "aggregate",
		AggregateOrigin:       append([]byte(nil), reviewAggregateOriginJSON...),
		Candidate:             append([]byte(nil), reviewCandidateJSON...),
		CandidateContentHash:  reviewRevision.CandidateContentHash,
		CandidateRevisionHash: reviewAggregateHash, CreatedAt: now,
	}
	reviewAggregateHead := model.StageCandidateHead{
		WorkspaceID: workspace.ID, StageInstanceKey: reviewAggregateStageKey,
		CurrentRevisionID:            reviewAggregateRevision.ID,
		CurrentCandidateRevisionHash: reviewAggregateHash, Revision: 1, UpdatedAt: now,
	}
	for _, record := range []any{&reviewAggregateRevision, &reviewAggregateHead} {
		if err = database.Create(record).Error; err != nil {
			t.Fatalf("seed Review aggregate dependency: %v", err)
		}
	}

	fragment := mustJSON(t, baseCandidate.CanonicalEntities[0])
	fragmentHash, err := agentcontract.StoryGraphCandidateFragmentHash(fragment)
	if err != nil {
		t.Fatal(err)
	}
	repairInput := agentcontract.StoryGraphRepairStageInput{
		TargetCandidateRevisionID: baseRevision.ID.String(), TargetCandidateRevisionHash: baseRevisionHash,
		ReviewCandidateRevisionID:   reviewRevision.ID.String(),
		ReviewCandidateRevisionHash: reviewRevision.CandidateRevisionHash,
		TargetIssue:                 targetIssue,
		AllowedTargets: []agentcontract.StoryGraphRepairAllowedTarget{{
			CandidateKey: "character:lin-yi", AllowedFields: []string{"canonical_name"},
			BaseFragmentHash: fragmentHash, Fragment: fragment,
		}},
		ReadOnlyAdjacency: []agentcontract.StoryGraphRepairReadOnlyFragment{},
		RepairRound:       1, MaxRepairRounds: 2,
	}
	repairInputJSON := mustJSON(t, repairInput)
	repairRequest := newRepairJourneyInvocation(
		t, workspace.ID, project.ID, "repair_candidate", "repair:bible:0000", "candidate_repair",
		sourceRef,
		[]agentcontract.StageUpstreamCandidateRef{
			baseUpstream,
			{
				Stage: "review_storygraph", ShardKey: "review:bible:0000",
				CandidateRevisionID:   reviewRevision.ID.String(),
				CandidateRevisionHash: reviewRevision.CandidateRevisionHash,
				SourceInvocationID:    reviewInvocation.ID.String(),
				SourceResultHash:      *reviewInvocation.ResultHash,
			},
		},
		repairInputJSON,
	)
	repairPatch := agentcontract.CandidateRepairPatch{
		TargetCandidateRevisionID: baseRevision.ID.String(), TargetCandidateRevisionHash: baseRevisionHash,
		Operations: []agentcontract.StoryGraphRepairOperation{{
			TargetCandidateKey: "character:lin-yi", BaseFragmentHash: fragmentHash,
			FieldName:   "canonical_name",
			Replacement: agentcontract.StoryGraphRepairReplacement{Text: stringPointer("林逸")},
		}},
		ReviewIssues: []agentcontract.StoryGraphReviewIssue{},
	}
	repairPatchJSON := mustJSON(t, repairPatch)
	repairInvocation := persistedInvocation(t, repairRequest, workspace.ID, "candidate_repair_patch", repairPatchJSON, "succeeded", now)
	if err = database.Create(&repairInvocation).Error; err != nil {
		t.Fatalf("seed Repair Invocation: %v", err)
	}

	segmentRequest := newRepairJourneyInvocation(
		t, workspace.ID, project.ID, "segment_episodes", "segment:0000", "episode_segmentation",
		sourceRef,
		[]agentcontract.StageUpstreamCandidateRef{{
			Stage: "review_storygraph", ShardKey: "review:aggregate",
			CandidateRevisionID: reviewAggregateRevision.ID.String(), CandidateRevisionHash: reviewAggregateHash,
			SourceInvocationID: reviewInvocation.ID.String(), SourceResultHash: reviewRevision.CandidateContentHash,
		}},
		json.RawMessage(`{}`),
	)
	segmentInvocation, segmentRevision, segmentHead := persistedInvocationCandidate(
		t, segmentRequest, workspace.ID, "episode_plan_candidate", json.RawMessage(`{"episodes":[]}`), now,
	)
	for _, record := range []any{&segmentInvocation, &segmentRevision, &segmentHead} {
		if err = database.Create(record).Error; err != nil {
			t.Fatalf("seed transitive segment dependency: %v", err)
		}
	}
	detailRequest := newRepairJourneyInvocation(
		t, workspace.ID, project.ID, "analyze_episode", "episode:0001", "episode_analysis",
		sourceRef,
		[]agentcontract.StageUpstreamCandidateRef{{
			Stage: "segment_episodes", ShardKey: "segment:0000",
			CandidateRevisionID: segmentRevision.ID.String(), CandidateRevisionHash: segmentRevision.CandidateRevisionHash,
			SourceInvocationID: segmentInvocation.ID.String(), SourceResultHash: *segmentInvocation.ResultHash,
		}},
		json.RawMessage(`{}`),
	)
	detailInvocation := persistedInvocation(t, detailRequest, workspace.ID, "", nil, "queued", now)
	if err = database.Create(&detailInvocation).Error; err != nil {
		t.Fatalf("seed transitive detail dependency: %v", err)
	}
	unrelatedRequest := newRepairJourneyInvocation(
		t, workspace.ID, project.ID, "detail_shots", "shots:other", "shot_batch",
		sourceRef,
		[]agentcontract.StageUpstreamCandidateRef{{
			Stage: "analyze_episode", ShardKey: "episode:other", CandidateRevisionID: uuid.NewString(),
			CandidateRevisionHash: strings.Repeat("5", 64), SourceInvocationID: uuid.NewString(),
			SourceResultHash: strings.Repeat("6", 64),
		}},
		json.RawMessage(`{}`),
	)
	unrelatedInvocation := persistedInvocation(t, unrelatedRequest, workspace.ID, "", nil, "queued", now)
	if err = database.Create(&unrelatedInvocation).Error; err != nil {
		t.Fatalf("seed unrelated dependency: %v", err)
	}

	malformedDependency := model.AgentInvocation{
		ID: uuid.New(), WorkspaceID: workspace.ID, RequestType: "stale_closure_probe", RequestID: uuid.New(),
		Kind: "storygraph_stage", WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage: "segment_episodes", ShardKey: "malformed", StageInstanceKey: strings.Repeat("7", 64),
		ShardManifestHash: strings.Repeat("8", 64), InputHash: strings.Repeat("9", 64),
		ExecutionPolicy: []byte(`{}`),
		Payload: append([]byte(nil), mustJSON(
			t,
			map[string]any{"upstream_candidates": []agentcontract.StageUpstreamCandidateRef{baseUpstream}},
		)...),
		Status: "queued", Attempts: 0, ClaimVersion: 0, CreatedAt: now, UpdatedAt: now,
	}
	if err = database.Create(&malformedDependency).Error; err != nil {
		t.Fatalf("seed malformed dependency: %v", err)
	}

	store := biblegorm.New(database)
	service := bibleapp.NewStoryCandidateRepairService(store, bibleapp.Config{
		Now: func() time.Time { return now.Add(time.Minute) }, NewID: uuid.NewString,
	})
	actor := bibleapp.Actor{UserID: user.ID.String(), TokenVersion: 1}
	command := bibleapp.StoryCandidateRepairCommand{
		WorkspaceID: workspace.ID.String(), StageInstanceKey: baseStageKey,
		ExpectedRevisionID: baseRevision.ID.String(), ExpectedCandidateRevisionHash: baseRevisionHash,
		ExpectedHeadRevision: 1, RepairInvocationID: repairInvocation.ID.String(),
		IdempotencyKey: "apply-canonical-name-repair",
	}
	if _, err = service.Apply(ctx, actor, command); err == nil {
		t.Fatal("Candidate repair committed before proving its full stale closure")
	}
	var headAfterRollback model.StageCandidateHead
	if err = database.First(&headAfterRollback, "stage_instance_key = ?", baseStageKey).Error; err != nil {
		t.Fatal(err)
	}
	var rollbackRevisionCount, rollbackReceiptCount, rollbackStaleCount int64
	if err = database.Model(&model.StageCandidateRevision{}).Where("workspace_id = ? AND stage_instance_key = ?", workspace.ID, baseStageKey).Count(&rollbackRevisionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("workspace_id = ? AND operation = ?", workspace.ID, bibleapp.StoryCandidateRepairOperation).Count(&rollbackReceiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StageInstanceStaleness{}).Where("workspace_id = ?", workspace.ID).Count(&rollbackStaleCount).Error; err != nil {
		t.Fatal(err)
	}
	if headAfterRollback.CurrentRevisionID != baseRevision.ID || rollbackRevisionCount != 1 ||
		rollbackReceiptCount != 0 || rollbackStaleCount != 0 {
		t.Fatalf("failed stale closure left partial facts: head=%#v revisions=%d receipts=%d stale=%d",
			headAfterRollback, rollbackRevisionCount, rollbackReceiptCount, rollbackStaleCount)
	}
	if err = database.Delete(&malformedDependency).Error; err != nil {
		t.Fatalf("remove test-only malformed dependency: %v", err)
	}

	start := make(chan struct{})
	results := make(chan bibleapp.StoryCandidateRepairResult, 2)
	errorsFound := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			value, applyErr := service.Apply(ctx, actor, command)
			results <- value
			errorsFound <- applyErr
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for applyErr := range errorsFound {
		if applyErr != nil {
			t.Fatalf("idempotent Candidate repair failed: %v", applyErr)
		}
	}
	var first bibleapp.StoryCandidateRepairResult
	for value := range results {
		if first.ReceiptID == "" {
			first = value
			continue
		}
		if !reflect.DeepEqual(value, first) {
			t.Fatalf("concurrent Candidate repair did not replay one Receipt: first=%#v second=%#v", first, value)
		}
	}
	expectedStale := []string{
		reviewInvocation.StageInstanceKey,
		reviewAggregateStageKey,
		segmentInvocation.StageInstanceKey,
		detailInvocation.StageInstanceKey,
	}
	if first.CandidateRevisionNo != 2 || !reflect.DeepEqual(first.StaleStageInstanceKeys, expectedStale) {
		t.Fatalf("Candidate repair result drifted: %#v", first)
	}

	var currentHead model.StageCandidateHead
	if err = database.First(&currentHead, "stage_instance_key = ?", baseStageKey).Error; err != nil {
		t.Fatal(err)
	}
	if currentHead.CurrentRevisionID.String() != first.CandidateRevisionID ||
		currentHead.CurrentCandidateRevisionHash != first.CandidateRevisionHash || currentHead.Revision != 2 {
		t.Fatalf("Candidate Head did not advance with its Receipt: %#v", currentHead)
	}
	var repaired model.StageCandidateRevision
	if err = database.First(&repaired, "id = ?", currentHead.CurrentRevisionID).Error; err != nil {
		t.Fatal(err)
	}
	var repairedCandidate domain.StoryReconciliationCandidate
	if err = json.Unmarshal(repaired.Candidate, &repairedCandidate); err != nil {
		t.Fatal(err)
	}
	if repairedCandidate.CanonicalEntities[0].CanonicalName != "林逸" ||
		repaired.ParentCandidateRevisionID == nil || *repaired.ParentCandidateRevisionID != baseRevision.ID {
		t.Fatalf("Candidate repair content or lineage drifted: %#v", repaired)
	}
	var stale []model.StageInstanceStaleness
	if err = database.Where("workspace_id = ?", workspace.ID).Order("created_at").Order("stage_instance_key").Find(&stale).Error; err != nil {
		t.Fatal(err)
	}
	actualStale := make([]string, len(stale))
	for index := range stale {
		actualStale[index] = stale[index].StageInstanceKey
	}
	if !sameStrings(actualStale, expectedStale) || slicesContain(actualStale, repairInvocation.StageInstanceKey) ||
		slicesContain(actualStale, unrelatedInvocation.StageInstanceKey) {
		t.Fatalf("persisted stale closure was not exact: %v", actualStale)
	}
	for _, record := range stale {
		if record.StageInstanceKey == reviewAggregateStageKey && record.InvocationID != nil {
			t.Fatalf("Backend aggregate staleness unexpectedly points at an Invocation: %#v", record)
		}
	}
	var revisionCount, receiptCount int64
	if err = database.Model(&model.StageCandidateRevision{}).Where("workspace_id = ? AND stage_instance_key = ?", workspace.ID, baseStageKey).Count(&revisionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("workspace_id = ? AND operation = ?", workspace.ID, bibleapp.StoryCandidateRepairOperation).Count(&receiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if revisionCount != 2 || receiptCount != 1 {
		t.Fatalf("Candidate repair fact counts = revisions %d receipts %d", revisionCount, receiptCount)
	}
	if err = database.Model(&stale[0]).Update("cause_candidate_revision_hash", strings.Repeat("f", 64)).Error; !errors.Is(err, model.ErrImmutableStageInstanceStaleness) {
		t.Fatalf("Stage staleness fact accepted mutation: %v", err)
	}

	command.IdempotencyKey = "apply-after-head-changed"
	if _, err = service.Apply(ctx, actor, command); err == nil {
		t.Fatal("Candidate repair with a new key reused a stale expected Head")
	}
	if err = database.Model(&model.CommandReceipt{}).Where("workspace_id = ? AND operation = ?", workspace.ID, bibleapp.StoryCandidateRepairOperation).Count(&receiptCount).Error; err != nil || receiptCount != 1 {
		t.Fatalf("conflicting Candidate repair left a Receipt: count=%d err=%v", receiptCount, err)
	}
}

func newRepairJourneyInvocation(
	t *testing.T,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	stage string,
	shardKey string,
	shardKind string,
	sourceRef agentcontract.StageSourceRef,
	upstream []agentcontract.StageUpstreamCandidateRef,
	stageInput json.RawMessage,
) agentcontract.StageInvocation {
	t.Helper()
	invocation, err := agentcontract.NewStageInvocation(
		uuid.NewString(),
		agentcontract.StoryGraphDefinition().ExecutionPolicy(),
		agentcontract.StageInvocationPayload{
			Stage: stage, ShardKey: shardKey, WorkspaceID: workspaceID.String(), ProjectID: projectID.String(),
			SourceRefs: []agentcontract.StageSourceRef{sourceRef}, UpstreamCandidates: upstream,
			ShardManifestRef: agentcontract.ShardManifestRef{
				ManifestID: uuid.NewString(), Version: 1, Hash: contractHashForRepair(t, stage+":"+shardKey),
			},
			Shard:      agentcontract.InvocationShard{Kind: shardKind, Key: shardKey, TreePath: stage + "." + shardKey},
			StageInput: stageInput,
		},
	)
	if err != nil {
		t.Fatalf("build %s journey invocation: %v", stage, err)
	}
	return invocation
}

func persistedInvocationCandidate(
	t *testing.T,
	request agentcontract.StageInvocation,
	workspaceID uuid.UUID,
	candidateType string,
	candidate json.RawMessage,
	now time.Time,
) (model.AgentInvocation, model.StageCandidateRevision, model.StageCandidateHead) {
	t.Helper()
	invocation := persistedInvocation(t, request, workspaceID, candidateType, candidate, "succeeded", now)
	origin := agentcontract.InvocationCandidateOrigin{
		SourceInvocationID: invocation.ID.String(), SourceResultHash: *invocation.ResultHash,
	}
	originJSON := mustJSON(t, origin)
	revisionHash, err := (agentcontract.CandidateRevisionMaterial{
		StageInstanceKey: invocation.StageInstanceKey, RevisionNo: 1, OriginKind: "invocation",
		InvocationOrigin: &origin, CandidateContentHash: *invocation.ResultHash,
	}).Hash()
	if err != nil {
		t.Fatal(err)
	}
	revision := model.StageCandidateRevision{
		ID: uuid.New(), WorkspaceID: workspaceID, StageInstanceKey: invocation.StageInstanceKey,
		RevisionNo: 1, OriginKind: "invocation", InvocationOrigin: append([]byte(nil), originJSON...),
		SourceInvocationID: &invocation.ID, SourceResultHash: invocation.ResultHash,
		Candidate: append([]byte(nil), candidate...), CandidateContentHash: *invocation.ResultHash,
		CandidateRevisionHash: revisionHash, CreatedAt: now,
	}
	head := model.StageCandidateHead{
		WorkspaceID: workspaceID, StageInstanceKey: invocation.StageInstanceKey,
		CurrentRevisionID: revision.ID, CurrentCandidateRevisionHash: revisionHash,
		Revision: 1, UpdatedAt: now,
	}
	return invocation, revision, head
}

func persistedInvocation(
	t *testing.T,
	request agentcontract.StageInvocation,
	workspaceID uuid.UUID,
	candidateType string,
	candidate json.RawMessage,
	status string,
	now time.Time,
) model.AgentInvocation {
	t.Helper()
	policyJSON := mustJSON(t, request.ExecutionPolicy)
	payloadJSON := mustJSON(t, request.Payload)
	stageKey, err := request.StageInstanceKey()
	if err != nil {
		t.Fatal(err)
	}
	record := model.AgentInvocation{
		ID: uuid.MustParse(request.InvocationID), WorkspaceID: workspaceID,
		RequestType: "candidate_repair_journey", RequestID: uuid.New(), Kind: request.Kind,
		WireSchemaVersion: request.WireSchemaVersion, Stage: request.Payload.Stage,
		ShardKey: request.Payload.ShardKey, StageInstanceKey: stageKey,
		ShardManifestHash: request.Payload.ShardManifestRef.Hash, InputHash: request.InputHash,
		ExecutionPolicy: append([]byte(nil), policyJSON...), Payload: append([]byte(nil), payloadJSON...), Status: status,
		Attempts: 0, ClaimVersion: 0, CreatedAt: now, UpdatedAt: now,
	}
	if len(candidate) > 0 {
		resultHash, hashErr := agentcontract.CanonicalHash(candidate)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		record.ResultHash = &resultHash
		record.CandidateType = &candidateType
		record.Candidate = append([]byte(nil), candidate...)
		record.Attempts = 1
		record.ClaimVersion = 1
		completed := now
		record.CompletedAt = &completed
	}
	return record
}

func contractHashForRepair(t *testing.T, value string) string {
	t.Helper()
	hash, err := agentcontract.CanonicalHash(mustJSON(t, map[string]string{"value": value}))
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	remaining := make(map[string]int, len(left))
	for _, value := range left {
		remaining[value]++
	}
	for _, value := range right {
		remaining[value]--
	}
	for _, count := range remaining {
		if count != 0 {
			return false
		}
	}
	return true
}

func slicesContain(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
