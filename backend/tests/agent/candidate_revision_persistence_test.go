package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
)

func TestRepairCandidateRevisionAdvancesHeadOnceUnderContention(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Candidate repair persistence journey")
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

	now := time.Date(2026, time.August, 28, 1, 0, 0, 0, time.UTC)
	workspace := model.Workspace{
		ID: uuid.New(), Name: "Candidate Repair", Status: "active", Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err = database.Create(&workspace).Error; err != nil {
		t.Fatalf("seed Candidate repair workspace: %v", err)
	}
	stageInstanceKey := contractHash(workspace.ID.String() + ":base-candidate")
	baseCandidate := json.RawMessage(`{"value":"base"}`)
	baseContentHash, err := contract.CanonicalHash(baseCandidate)
	if err != nil {
		t.Fatal(err)
	}
	aggregateOrigin := contract.AggregateCandidateOrigin{
		ShardManifestID: uuid.NewString(), ManifestVersion: 1,
		ShardManifestHash: strings.Repeat("b", 64),
		LeafCandidates: []contract.AggregateLeafCandidateRef{{
			StageInstanceKey: strings.Repeat("c", 64), ShardKey: "root",
			CandidateRevisionID: uuid.NewString(), CandidateRevisionHash: strings.Repeat("d", 64),
		}},
	}
	baseRevisionHash, err := (contract.CandidateRevisionMaterial{
		StageInstanceKey: stageInstanceKey, RevisionNo: 1, OriginKind: "aggregate",
		AggregateOrigin: &aggregateOrigin, CandidateContentHash: baseContentHash,
	}).Hash()
	if err != nil {
		t.Fatal(err)
	}
	aggregateOriginJSON, err := json.Marshal(aggregateOrigin)
	if err != nil {
		t.Fatal(err)
	}
	baseRevision := model.StageCandidateRevision{
		ID: uuid.New(), WorkspaceID: workspace.ID, StageInstanceKey: stageInstanceKey,
		RevisionNo: 1, OriginKind: "aggregate", AggregateOrigin: aggregateOriginJSON,
		Candidate: append([]byte(nil), baseCandidate...), CandidateContentHash: baseContentHash,
		CandidateRevisionHash: baseRevisionHash, CreatedAt: now,
	}
	if err = database.Create(&baseRevision).Error; err != nil {
		t.Fatalf("seed base Candidate Revision: %v", err)
	}
	head := model.StageCandidateHead{
		WorkspaceID: workspace.ID, StageInstanceKey: stageInstanceKey,
		CurrentRevisionID: baseRevision.ID, CurrentCandidateRevisionHash: baseRevisionHash,
		Revision: 1, UpdatedAt: now,
	}
	if err = database.Create(&head).Error; err != nil {
		t.Fatalf("seed base Candidate Head: %v", err)
	}

	create := func(value any) error { return database.Create(value).Error }
	firstInvocation := seedSucceededRepairInvocation(t, create, workspace.ID, now, "first-"+workspace.ID.String())
	secondInvocation := seedSucceededRepairInvocation(t, create, workspace.ID, now, "second-"+workspace.ID.String())
	commands := []agentgorm.CandidateRepairAdvance{
		{
			WorkspaceID: workspace.ID, StageInstanceKey: stageInstanceKey,
			ExpectedRevisionID: baseRevision.ID, ExpectedCandidateRevisionHash: baseRevisionHash,
			ExpectedHeadRevision: 1, RepairInvocationID: firstInvocation.ID,
			RepairResultHash: *firstInvocation.ResultHash,
			Candidate:        json.RawMessage(`{"value":"first"}`), CreatedAt: now.Add(time.Second),
		},
		{
			WorkspaceID: workspace.ID, StageInstanceKey: stageInstanceKey,
			ExpectedRevisionID: baseRevision.ID, ExpectedCandidateRevisionHash: baseRevisionHash,
			ExpectedHeadRevision: 1, RepairInvocationID: secondInvocation.ID,
			RepairResultHash: *secondInvocation.ResultHash,
			Candidate:        json.RawMessage(`{"value":"second"}`), CreatedAt: now.Add(2 * time.Second),
		},
	}
	type outcome struct {
		revision model.StageCandidateRevision
		err      error
	}
	start := make(chan struct{})
	results := make(chan outcome, len(commands))
	var workers sync.WaitGroup
	for _, command := range commands {
		workers.Add(1)
		go func(value agentgorm.CandidateRepairAdvance) {
			defer workers.Done()
			<-start
			revision, applyErr := agentgorm.AdvanceCandidateHeadWithRepair(database, value)
			results <- outcome{revision: revision, err: applyErr}
		}(command)
	}
	close(start)
	workers.Wait()
	close(results)

	var winner model.StageCandidateRevision
	conflicts := 0
	for result := range results {
		switch {
		case result.err == nil:
			if winner.ID != uuid.Nil {
				t.Fatalf("multiple competing repairs succeeded: first=%s second=%s", winner.ID, result.revision.ID)
			}
			winner = result.revision
		case errors.Is(result.err, agentgorm.ErrCandidateHeadConflict):
			conflicts++
		default:
			t.Fatalf("unexpected Candidate repair result: %v", result.err)
		}
	}
	if winner.ID == uuid.Nil || conflicts != 1 {
		t.Fatalf("Candidate repair contention did not produce one winner: winner=%s conflicts=%d", winner.ID, conflicts)
	}

	var persistedHead model.StageCandidateHead
	if err = database.First(&persistedHead, "stage_instance_key = ?", stageInstanceKey).Error; err != nil {
		t.Fatal(err)
	}
	if persistedHead.CurrentRevisionID != winner.ID ||
		persistedHead.CurrentCandidateRevisionHash != winner.CandidateRevisionHash || persistedHead.Revision != 2 {
		t.Fatalf("Candidate Head did not move to the single repair winner: %#v", persistedHead)
	}
	var revisions []model.StageCandidateRevision
	if err = database.Where("stage_instance_key = ?", stageInstanceKey).Order("revision_no").Find(&revisions).Error; err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 || revisions[0].ID != baseRevision.ID || revisions[1].ID != winner.ID ||
		winner.RevisionNo != 2 || winner.ParentCandidateRevisionID == nil || *winner.ParentCandidateRevisionID != baseRevision.ID ||
		winner.ParentCandidateRevisionHash == nil || *winner.ParentCandidateRevisionHash != baseRevisionHash || winner.OriginKind != "repair" {
		t.Fatalf("Candidate repair lineage drifted: revisions=%#v", revisions)
	}
	var repairOrigin contract.RepairCandidateOrigin
	if err = json.Unmarshal(winner.RepairOrigin, &repairOrigin); err != nil {
		t.Fatal(err)
	}
	winnerContentHash, err := contract.CanonicalHash(json.RawMessage(winner.Candidate))
	if err != nil || winnerContentHash != winner.CandidateContentHash {
		t.Fatalf("repaired Candidate content hash drifted: got=%s persisted=%s err=%v", winnerContentHash, winner.CandidateContentHash, err)
	}
	expectedRevisionHash, err := (contract.CandidateRevisionMaterial{
		StageInstanceKey: stageInstanceKey, RevisionNo: 2,
		ParentCandidateRevisionHash: &baseRevisionHash, OriginKind: "repair", RepairOrigin: &repairOrigin,
		CandidateContentHash: winnerContentHash,
	}).Hash()
	if err != nil || expectedRevisionHash != winner.CandidateRevisionHash {
		t.Fatalf("repaired Candidate Revision hash drifted: got=%s persisted=%s err=%v", expectedRevisionHash, winner.CandidateRevisionHash, err)
	}
	noChangeInvocation := seedSucceededRepairInvocation(t, create, workspace.ID, now, "no-change-"+workspace.ID.String())
	if _, err = agentgorm.AdvanceCandidateHeadWithRepair(database, agentgorm.CandidateRepairAdvance{
		WorkspaceID: workspace.ID, StageInstanceKey: stageInstanceKey,
		ExpectedRevisionID: winner.ID, ExpectedCandidateRevisionHash: winner.CandidateRevisionHash,
		ExpectedHeadRevision: winner.RevisionNo, RepairInvocationID: noChangeInvocation.ID,
		RepairResultHash: *noChangeInvocation.ResultHash,
		Candidate:        json.RawMessage(winner.Candidate), CreatedAt: now.Add(3 * time.Second),
	}); !errors.Is(err, agentgorm.ErrCandidateRepairNoChange) {
		t.Fatalf("no-op Candidate repair was not rejected: %v", err)
	}
	var revisionCount int64
	if err = database.Model(&model.StageCandidateRevision{}).Where("stage_instance_key = ?", stageInstanceKey).Count(&revisionCount).Error; err != nil || revisionCount != 2 {
		t.Fatalf("no-op Candidate repair left a Revision: count=%d err=%v", revisionCount, err)
	}
	if err = database.Model(&baseRevision).Update("candidate_content_hash", strings.Repeat("f", 64)).Error; !errors.Is(err, model.ErrImmutableStageCandidateRevision) {
		t.Fatalf("base Candidate Revision accepted mutation: %v", err)
	}
}

func seedSucceededRepairInvocation(
	t *testing.T,
	create func(value any) error,
	workspaceID uuid.UUID,
	now time.Time,
	label string,
) model.AgentInvocation {
	t.Helper()
	patch := json.RawMessage(`{"target":"` + label + `"}`)
	resultHash, err := contract.CanonicalHash(patch)
	if err != nil {
		t.Fatal(err)
	}
	candidateType := "candidate_repair_patch"
	record := model.AgentInvocation{
		ID: uuid.New(), WorkspaceID: workspaceID, RequestType: "candidate_repair",
		RequestID: uuid.New(), Kind: "storygraph_stage", WireSchemaVersion: contract.StoryGraphWireSchemaVersion,
		Stage: "repair_candidate", ShardKey: label, StageInstanceKey: contractHash(label + ":stage"),
		ShardManifestHash: contractHash(label + ":manifest"), InputHash: contractHash(label + ":input"),
		ExecutionPolicy: []byte(`{}`), Payload: []byte(`{}`), Status: "succeeded",
		ResultHash: &resultHash, CandidateType: &candidateType, Candidate: append([]byte(nil), patch...),
		Attempts: 1, ClaimVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err = create(&record); err != nil {
		t.Fatalf("seed %s repair Invocation: %v", label, err)
	}
	return record
}

func contractHash(value string) string {
	hash, _ := contract.CanonicalHash(json.RawMessage(`{"value":"` + value + `"}`))
	return hash
}
