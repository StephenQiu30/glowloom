package workflow_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	storyboardgeneration "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/generation"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type selectedImageSource struct {
	mu         sync.Mutex
	selections map[string]storyboardapp.SelectedImageSnapshot
	actor      storyboardapp.Actor
}

type generationSelectionReader struct {
	selection generationdomain.CandidateSelection
	actor     generationapp.Actor
	id        string
}

func (reader *generationSelectionReader) RequireSelected(
	_ context.Context,
	actor generationapp.Actor,
	selectionID string,
) (generationdomain.CandidateSelection, error) {
	reader.actor, reader.id = actor, selectionID
	return reader.selection, nil
}

func TestProductionSelectedImageSourceMapsOnlyGenerationOwnerSnapshot(t *testing.T) {
	selectionID, workspaceID, projectID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	candidateID, artifactID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	selectionHash, artifactHash := strings.Repeat("a", 64), strings.Repeat("b", 64)
	reader := &generationSelectionReader{selection: generationdomain.CandidateSelection{
		ID: selectionID, WorkspaceID: workspaceID, ProjectID: projectID, Revision: 1, ContentHash: selectionHash,
		SelectedCandidateID: candidateID, SelectedArtifactID: artifactID, SelectedArtifactSHA256: artifactHash,
		Candidates: []generationdomain.CandidateReference{{
			ID: candidateID, Revision: 2, ArtifactID: artifactID, ArtifactRevision: 3, ArtifactSHA256: artifactHash,
		}},
	}}
	source := storyboardgeneration.NewSelectedImageSource(reader)
	actor := storyboardapp.Actor{UserID: userID, TokenVersion: 4}
	selected, err := source.RequireSelectedImage(context.Background(), actor, selectionID)
	if err != nil || selected.ID != selectionID || selected.CandidateID != candidateID ||
		selected.CandidateRevision != 2 || selected.ArtifactID != artifactID || selected.ArtifactRevision != 3 ||
		selected.ArtifactSHA256 != artifactHash || selected.ContentHash != selectionHash {
		t.Fatalf("map Generation selected image snapshot: selected=%#v err=%v", selected, err)
	}
	if reader.id != selectionID || reader.actor.UserID != userID || reader.actor.TokenVersion != 4 {
		t.Fatalf("Generation Selection reader call drifted: id=%s actor=%#v", reader.id, reader.actor)
	}
	reader.selection.SelectedArtifactSHA256 = strings.Repeat("c", 64)
	if _, err = source.RequireSelectedImage(context.Background(), actor, selectionID); err == nil {
		t.Fatal("Production adapter accepted a drifted Generation Artifact snapshot")
	}
}

func (source *selectedImageSource) RequireSelectedImage(
	_ context.Context,
	actor storyboardapp.Actor,
	selectionID string,
) (storyboardapp.SelectedImageSnapshot, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.actor = actor
	selection, found := source.selections[selectionID]
	if !found {
		return storyboardapp.SelectedImageSnapshot{}, errors.New("selection not found")
	}
	return selection, nil
}

func TestShotImageBindingVersionsConvergeThroughProductionOwner(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run Shot image binding integration")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Shot image binding database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Shot image binding GORM catalog: %v", err)
	}
	now := time.Date(2026, time.August, 27, 5, 0, 0, 0, time.UTC)
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	shotID := seedFormalStoryboardShot(t, func(value any) error { return database.Create(value).Error }, fixture, now)
	actor := storyboardapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}

	first := selectedImage(fixture, "1")
	second := selectedImage(fixture, "2")
	third := selectedImage(fixture, "3")
	fourth := selectedImage(fixture, "4")
	source := &selectedImageSource{selections: map[string]storyboardapp.SelectedImageSnapshot{
		first.ID: first, second.ID: second, third.ID: third, fourth.ID: fourth,
	}}
	service := storyboardapp.NewShotImageBindingService(
		storyboardgorm.New(database), source,
		storyboardapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	command := storyboardapp.BindSelectedImageCommand{
		ShotID: shotID.String(), CandidateSelectionID: first.ID,
		ExpectedShotRevision: 1, ExpectedShotContentHash: strings.Repeat("9", 64),
		ExpectedCurrentRevision: 0, IdempotencyKey: "shot-image-first",
	}
	input, _, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion,
		Config:        []byte(`{"expected_current_revision":0}`),
		Bindings: []workflow.NodeInputBinding{
			{
				Port: "shot", ValueType: "production_shot", SourceKind: workflow.NodeInputSourceNodeOutput,
				SourceNodeID: "shot-source", SourcePort: "shot", ReferenceID: shotID.String(),
				ReferenceVersion: "1", ContentHash: strings.Repeat("9", 64),
			},
			{
				Port: "selection", ValueType: "generation_candidate_selection", SourceKind: workflow.NodeInputSourceNodeOutput,
				SourceNodeID: "selection-gate", SourcePort: "selection", ReferenceID: first.ID,
				ReferenceVersion: "1", ContentHash: first.ContentHash,
			},
		},
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}},
	})
	if err != nil {
		t.Fatalf("build Shot image binding node input: %v", err)
	}
	executor := workflowproduction.NewNodeExecutor(nil, nil, nil, nil, nil, service)
	executed, err := executor.Execute(ctx, workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "shot-binding",
			Executor: "activity.production_shot_image_binding", Attempt: 1,
		},
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		InitiatorUserID: fixture.userID.String(), InitiatorTokenVersion: 1,
		IdempotencyKey: command.IdempotencyKey, Input: input, InputHash: inputHash,
		OutputPorts: []authoring.PortDefinition{{Key: "binding", ValueType: "production_shot_image_binding", Required: true}},
	})
	if err != nil || executed.Status != "SUCCEEDED" || len(executed.Output.Bindings) != 1 ||
		executed.Output.Bindings[0].ValueType != "production_shot_image_binding" ||
		executed.Output.Bindings[0].ReferenceVersion != "1" {
		t.Fatalf("execute Workflow Shot image binding node: result=%#v err=%v", executed, err)
	}

	const callers = 8
	results := make(chan storyboardapp.BindSelectedImageResult, callers)
	errorsFound := make(chan error, callers)
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, bindErr := service.BindSelectedImage(ctx, actor, command)
			if bindErr != nil {
				errorsFound <- bindErr
				return
			}
			results <- result
		}()
	}
	workers.Wait()
	close(results)
	close(errorsFound)
	for bindErr := range errorsFound {
		t.Fatalf("bind selected image concurrently: %v", bindErr)
	}
	var bindingID, receiptID string
	for result := range results {
		if result.Binding.Revision != 1 || result.Binding.ShotID != shotID.String() ||
			result.Binding.CandidateSelectionID != first.ID || result.Receipt.Operation != "storyboard.shot.bind_selected_image" {
			t.Fatalf("unexpected first Shot image binding: %#v", result)
		}
		if bindingID == "" {
			bindingID, receiptID = result.Binding.ID, result.Receipt.ID
		}
		if result.Binding.ID != bindingID || result.Receipt.ID != receiptID {
			t.Fatalf("concurrent binding did not converge: %#v", result)
		}
	}
	if source.actor != actor {
		t.Fatalf("selection source did not receive current actor: %#v", source.actor)
	}

	drifted := command
	drifted.CandidateSelectionID = second.ID
	if _, err = service.BindSelectedImage(ctx, actor, drifted); err == nil {
		t.Fatal("idempotency replay accepted a different CandidateSelection")
	}
	driftedSelection := source.selections[first.ID]
	driftedSelection.ArtifactSHA256 = strings.Repeat("6", 64)
	source.selections[first.ID] = driftedSelection
	if _, err = service.BindSelectedImage(ctx, actor, command); err == nil {
		t.Fatal("idempotency replay accepted a drifted selected Artifact snapshot")
	}
	source.selections[first.ID] = first
	if _, err = service.BindSelectedImage(ctx, actor, storyboardapp.BindSelectedImageCommand{
		ShotID: shotID.String(), CandidateSelectionID: first.ID,
		ExpectedShotRevision: 1, ExpectedShotContentHash: strings.Repeat("9", 64),
		ExpectedCurrentRevision: 1, IdempotencyKey: "shot-image-duplicate-selection",
	}); err == nil {
		t.Fatal("Shot image binding accepted the same Selection with a new idempotency key")
	}
	wrongProject := selectedImage(fixture, "5")
	wrongProject.ProjectID = uuid.NewString()
	source.selections[wrongProject.ID] = wrongProject
	if _, err = service.BindSelectedImage(ctx, actor, storyboardapp.BindSelectedImageCommand{
		ShotID: shotID.String(), CandidateSelectionID: wrongProject.ID,
		ExpectedShotRevision: 1, ExpectedShotContentHash: strings.Repeat("9", 64),
		ExpectedCurrentRevision: 1, IdempotencyKey: "shot-image-cross-project",
	}); err == nil {
		t.Fatal("Shot image binding accepted a cross-project Selection")
	}
	wrongWorkspace := selectedImage(fixture, "6")
	wrongWorkspace.WorkspaceID = uuid.NewString()
	source.selections[wrongWorkspace.ID] = wrongWorkspace
	if _, err = service.BindSelectedImage(ctx, actor, storyboardapp.BindSelectedImageCommand{
		ShotID: shotID.String(), CandidateSelectionID: wrongWorkspace.ID,
		ExpectedShotRevision: 1, ExpectedShotContentHash: strings.Repeat("9", 64),
		ExpectedCurrentRevision: 1, IdempotencyKey: "shot-image-cross-workspace",
	}); err == nil {
		t.Fatal("Shot image binding accepted a cross-workspace Selection")
	}

	replaced, err := service.BindSelectedImage(ctx, actor, storyboardapp.BindSelectedImageCommand{
		ShotID: shotID.String(), CandidateSelectionID: second.ID,
		ExpectedShotRevision: 1, ExpectedShotContentHash: strings.Repeat("9", 64),
		ExpectedCurrentRevision: 1, IdempotencyKey: "shot-image-second",
	})
	if err != nil || replaced.Binding.Revision != 2 || replaced.Binding.CandidateSelectionID != second.ID {
		t.Fatalf("append second Shot image binding: result=%#v err=%v", replaced, err)
	}

	type attempt struct {
		selection storyboardapp.SelectedImageSnapshot
		key       string
	}
	attempts := []attempt{{third, "shot-image-third"}, {fourth, "shot-image-fourth"}}
	concurrentErrors := make(chan error, len(attempts))
	concurrentResults := make(chan storyboardapp.BindSelectedImageResult, len(attempts))
	workers = sync.WaitGroup{}
	for _, value := range attempts {
		workers.Add(1)
		go func(value attempt) {
			defer workers.Done()
			result, bindErr := service.BindSelectedImage(ctx, actor, storyboardapp.BindSelectedImageCommand{
				ShotID: shotID.String(), CandidateSelectionID: value.selection.ID,
				ExpectedShotRevision: 1, ExpectedShotContentHash: strings.Repeat("9", 64),
				ExpectedCurrentRevision: 2, IdempotencyKey: value.key,
			})
			if bindErr != nil {
				concurrentErrors <- bindErr
				return
			}
			concurrentResults <- result
		}(value)
	}
	workers.Wait()
	close(concurrentErrors)
	close(concurrentResults)
	if len(concurrentResults) != 1 || len(concurrentErrors) != 1 {
		t.Fatalf("concurrent replacements did not yield one winner: successes=%d errors=%d", len(concurrentResults), len(concurrentErrors))
	}
	winner := <-concurrentResults
	if winner.Binding.Revision != 3 {
		t.Fatalf("concurrent winner has wrong revision: %#v", winner.Binding)
	}

	current, err := service.RequireCurrentShotImage(ctx, actor, shotID.String())
	if err != nil || current.ID != winner.Binding.ID || current.Revision != 3 {
		t.Fatalf("read current Shot image binding: current=%#v err=%v", current, err)
	}
	var bindingCount, receiptCount int64
	if err = database.Model(&model.StoryboardShotImageBindingVersion{}).
		Where("shot_id = ?", shotID).Count(&bindingCount).Error; err != nil {
		t.Fatalf("count Shot image binding versions: %v", err)
	}
	if err = database.Model(&model.CommandReceipt{}).
		Where("workspace_id = ? AND operation = ?", fixture.workspaceID, "storyboard.shot.bind_selected_image").
		Count(&receiptCount).Error; err != nil {
		t.Fatalf("count Shot image binding receipts: %v", err)
	}
	if bindingCount != 3 || receiptCount != 3 {
		t.Fatalf("Shot image binding facts drifted: bindings=%d receipts=%d", bindingCount, receiptCount)
	}

	if err = database.Model(&model.StoryboardShotImageBindingVersion{}).Where("id = ?", winner.Binding.ID).
		Update("content_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject Shot image binding hash drift: %v", err)
	}
	if _, err = service.RequireCurrentShotImage(ctx, actor, shotID.String()); err == nil {
		t.Fatal("current Shot image binding accepted a drifted Content Hash")
	}
	if err = database.Model(&model.StoryboardShotImageBindingVersion{}).Where("id = ?", winner.Binding.ID).
		Update("content_hash", winner.Binding.ContentHash).Error; err != nil {
		t.Fatalf("restore Shot image binding hash: %v", err)
	}
	if err = database.Model(&model.StoryboardShot{}).Where("id = ?", shotID).
		Updates(map[string]any{"revision": 2, "content_hash": strings.Repeat("6", 64)}).Error; err != nil {
		t.Fatalf("inject Shot revision drift: %v", err)
	}
	if _, err = service.RequireCurrentShotImage(ctx, actor, shotID.String()); err == nil {
		t.Fatal("current image binding accepted a drifted Shot revision")
	}
	if err = database.Model(&model.StoryboardShot{}).Where("id = ?", shotID).
		Updates(map[string]any{"revision": 1, "content_hash": strings.Repeat("9", 64)}).Error; err != nil {
		t.Fatalf("restore Shot revision: %v", err)
	}

	if err = database.Model(&model.StoryboardShot{}).Where("id = ?", shotID).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive Shot: %v", err)
	}
	if _, err = service.BindSelectedImage(ctx, actor, storyboardapp.BindSelectedImageCommand{
		ShotID: shotID.String(), CandidateSelectionID: first.ID,
		ExpectedShotRevision: 1, ExpectedShotContentHash: strings.Repeat("9", 64),
		ExpectedCurrentRevision: 3, IdempotencyKey: "shot-image-archived",
	}); err == nil {
		t.Fatal("archived Shot accepted an image binding")
	}
	if err = database.Model(&model.StoryboardShot{}).Where("id = ?", shotID).Update("status", "active").Error; err != nil {
		t.Fatalf("restore Shot: %v", err)
	}
	if err = database.Model(&model.UserAccount{}).Where("id = ?", fixture.userID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke binding actor token: %v", err)
	}
	if _, err = service.RequireCurrentShotImage(ctx, actor, shotID.String()); err == nil {
		t.Fatal("revoked actor read the current Shot image binding")
	}
}

func selectedImage(fixture compilerProjectFixture, suffix string) storyboardapp.SelectedImageSnapshot {
	return storyboardapp.SelectedImageSnapshot{
		ID: uuid.NewString(), WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		Revision: 1, ContentHash: strings.Repeat(suffix, 64), CandidateID: uuid.NewString(), CandidateRevision: 1,
		ArtifactID: uuid.NewString(), ArtifactRevision: 2, ArtifactSHA256: strings.Repeat(suffix, 64),
	}
}

func seedFormalStoryboardShot(
	t *testing.T,
	create func(any) error,
	fixture compilerProjectFixture,
	now time.Time,
) uuid.UUID {
	t.Helper()
	episodeID, scriptVersionID, structureID := uuid.New(), uuid.New(), uuid.New()
	taskID, batchID, shotID := uuid.New(), uuid.New(), uuid.New()
	resultHash := strings.Repeat("8", 64)
	records := []any{
		&model.Episode{
			ID: episodeID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			Name: "Binding Episode", Position: 1, TargetDurationMS: 90_000,
			Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
		},
		&model.EpisodeScriptVersion{
			ID: scriptVersionID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			EpisodeID: episodeID, VersionNo: 1, DocumentRevisionID: fixture.scriptRevisionID,
			SourceStart: 0, SourceEnd: 4, Content: "雨巷，夜", ContentHash: fixture.normalizedHash,
			Status: "published", CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now,
		},
		&model.EpisodeStructure{
			ID: structureID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			EpisodeID: episodeID, ScriptVersionID: scriptVersionID, Status: "confirmed",
			Scenes: []byte(`[]`), ResultHash: resultHash, Revision: 2,
			ConfirmedBy: &fixture.userID, ConfirmedAt: &now, CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now,
		},
		&model.WorkflowTask{
			ID: taskID, WorkspaceID: fixture.workspaceID, TaskType: "storyboard_draft",
			RequestType: "storyboard_draft_batch", RequestID: batchID, Scope: []byte(`{}`),
			Status: "succeeded", ProgressStage: "complete", CancelStatus: "none", Revision: 1,
			CreatedAt: now, UpdatedAt: now,
		},
		&model.StoryboardDraftBatch{
			ID: batchID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			EpisodeID: episodeID, StructureID: structureID, ScriptVersionID: scriptVersionID, TaskID: taskID,
			Status: "applied", InputHash: strings.Repeat("7", 64), ResultHash: &resultHash,
			Candidate: []byte(`{"shots":[]}`), Decisions: []byte(`{}`), Revision: 4,
			ApprovedBy: &fixture.userID, ApprovedAt: &now, AppliedAt: &now,
			CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now,
		},
		&model.StoryboardShot{
			ID: shotID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			EpisodeID: episodeID, BatchID: batchID, ProposalKey: "shot-1", Position: 1,
			Title: "雨巷", NarrativeUnitIDs: []byte(`[]`), Spec: []byte(`{"duration_ms":1800}`),
			ContentHash: strings.Repeat("9", 64), Status: "active", Revision: 1,
			CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, record := range records {
		if err := create(record); err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}
	return shotID
}

var _ storyboardapp.SelectedImageSource = (*selectedImageSource)(nil)
