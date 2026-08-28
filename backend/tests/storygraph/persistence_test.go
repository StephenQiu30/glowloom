package storygraph_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestStoryGraphPublishesImmutableLinearVersionsWithRealPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL StoryGraph journey")
	}
	database, err := platformdatabase.Open(context.Background(), databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	fixture := seedStoryGraphOwners(t, func(value any) error { return database.Create(value).Error }, "linear")
	count := func(value any, query string, args ...any) (int64, error) {
		var result int64
		errorFound := database.Model(value).Where(query, args...).Count(&result).Error
		return result, errorFound
	}
	clock := time.Date(2026, time.August, 27, 3, 0, 0, 0, time.UTC)
	service := storygraphapp.NewService(storygraphgorm.New(database), storygraphapp.Config{
		Now: func() time.Time {
			clock = clock.Add(time.Second)
			return clock
		},
		NewID: uuid.NewString,
	})
	actor := storygraphapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}

	first, err := service.Compile(context.Background(), actor, storygraphapp.CompileCommand{
		ProjectID: fixture.projectID.String(), ExpectedHeadRevision: 0,
		ExpectedCurrentContentHash: "", IdempotencyKey: "compile-linear-1",
	})
	if err != nil {
		t.Fatalf("compile first StoryGraph: %v", err)
	}
	if first.Version.VersionNo != 1 || first.Head.Revision != 1 || first.Head.CurrentVersionID != first.Version.ID ||
		first.Version.ParentVersionID != nil || first.Version.Status != "published" ||
		first.Version.ContentHash != first.Head.CurrentContentHash || len(first.Version.OwnerSetHash) != 64 {
		t.Fatalf("unexpected first publication: %#v", first)
	}
	if len(first.Version.Nodes) != 5 || len(first.Version.Edges) != 4 {
		t.Fatalf("compiler did not project source/episode/scene/dialogue/beat: nodes=%d edges=%d", len(first.Version.Nodes), len(first.Version.Edges))
	}
	assertPublicationCounts(t, count, fixture, 1, 1, 1, 1)

	replayed, err := service.Compile(context.Background(), actor, storygraphapp.CompileCommand{
		ProjectID: fixture.projectID.String(), ExpectedHeadRevision: 0,
		ExpectedCurrentContentHash: "", IdempotencyKey: "compile-linear-1",
	})
	if err != nil || replayed.Version.ID != first.Version.ID || replayed.Receipt.ID != first.Receipt.ID {
		t.Fatalf("idempotent replay diverged: %#v, error=%v", replayed, err)
	}
	assertPublicationCounts(t, count, fixture, 1, 1, 1, 1)

	staleResult, err := service.Compile(context.Background(), actor, storygraphapp.CompileCommand{
		ProjectID: fixture.projectID.String(), ExpectedHeadRevision: 0,
		ExpectedCurrentContentHash: "", IdempotencyKey: "compile-linear-stale",
	})
	if !storygraphapp.IsStale(err) || staleResult.Head.Revision != 1 || staleResult.Head.CurrentContentHash != first.Version.ContentHash {
		t.Fatalf("stale head was not explicit: result=%#v error=%v", staleResult, err)
	}
	assertPublicationCounts(t, count, fixture, 1, 1, 1, 1)

	immutableError := database.Model(&model.StoryGraphVersion{}).
		Where("id = ?", first.Version.ID).
		Update("content_hash", strings.Repeat("f", 64)).Error
	if immutableError == nil {
		t.Fatal("published StoryGraphVersion accepted an in-place update")
	}
	var persistedFirst model.StoryGraphVersion
	if err = database.First(&persistedFirst, "id = ?", first.Version.ID).Error; err != nil || persistedFirst.ContentHash != first.Version.ContentHash {
		t.Fatalf("first version changed after update attempt: %#v error=%v", persistedFirst, err)
	}
	deleteError := database.Delete(&model.StoryGraphVersion{ID: persistedFirst.ID}).Error
	if !errors.Is(deleteError, model.ErrImmutableStoryGraphVersion) {
		t.Fatalf("published StoryGraphVersion accepted deletion: %v", deleteError)
	}

	if err = database.Model(&model.Episode{}).Where("id = ?", fixture.episodeID).
		Updates(map[string]any{"name": "第二版标题", "revision": 2, "updated_at": clock.Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	second, err := service.Compile(context.Background(), actor, storygraphapp.CompileCommand{
		ProjectID: fixture.projectID.String(), ExpectedHeadRevision: first.Head.Revision,
		ExpectedCurrentContentHash: first.Head.CurrentContentHash, IdempotencyKey: "compile-linear-2",
	})
	if err != nil {
		t.Fatalf("compile second StoryGraph: %v", err)
	}
	if second.Version.VersionNo != 2 || second.Version.ParentVersionID == nil || *second.Version.ParentVersionID != first.Version.ID ||
		second.Version.ParentContentHash == nil || *second.Version.ParentContentHash != first.Version.ContentHash ||
		second.Version.ContentHash == first.Version.ContentHash || second.Head.Revision != 2 {
		t.Fatalf("second version is not a linear child: %#v", second)
	}
	firstEpisode := findNode(t, first.Version.Nodes, storygraph.NodeTypeEpisode)
	secondEpisode := findNode(t, second.Version.Nodes, storygraph.NodeTypeEpisode)
	if firstEpisode.StoryNodeKey != secondEpisode.StoryNodeKey || firstEpisode.ContentHash == secondEpisode.ContentHash {
		t.Fatalf("Owner version change broke stable key semantics: first=%#v second=%#v", firstEpisode, secondEpisode)
	}
	assertPublicationCounts(t, count, fixture, 2, 1, 2, 2)

	lateReplay, err := service.Compile(context.Background(), actor, storygraphapp.CompileCommand{
		ProjectID: fixture.projectID.String(), ExpectedHeadRevision: 0,
		ExpectedCurrentContentHash: "", IdempotencyKey: "compile-linear-1",
	})
	if err != nil || lateReplay.Version.ID != first.Version.ID || lateReplay.Receipt.ID != first.Receipt.ID ||
		lateReplay.Head.CurrentVersionID != first.Version.ID || lateReplay.Head.Revision != 1 {
		t.Fatalf("late idempotent replay did not return the original publication: %#v, error=%v", lateReplay, err)
	}
	assertPublicationCounts(t, count, fixture, 2, 1, 2, 2)

	var event model.OutboxEvent
	if err = database.Where("project_id = ?", fixture.projectID).Order("occurred_at").First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if event.EventType != "StoryGraphVersionPublished" || event.AggregateKind != "storygraph" ||
		event.AggregateID != fixture.projectID.String() || event.AggregateRevision != first.Version.VersionNo ||
		strings.Contains(string(event.Payload), fixture.text) || len(event.PayloadHash) != 64 {
		t.Fatalf("unsafe or incomplete outbox event: %#v", event)
	}
	if _, envelopeErr := eventing.NewEnvelope(eventing.OutboxEvent{
		ID: event.ID.String(), EventType: event.EventType, EventVersion: event.EventVersion,
		WorkspaceID: event.WorkspaceID.String(), ProjectID: event.ProjectID.String(),
		AggregateKind: event.AggregateKind, AggregateID: event.AggregateID,
		AggregateRevision: event.AggregateRevision, SourceReceiptID: event.SourceReceiptID.String(),
		Payload: json.RawMessage(event.Payload), PayloadHash: event.PayloadHash, OccurredAt: event.OccurredAt,
	}, eventing.TraceContext{RequestID: event.SourceReceiptID.String()}); envelopeErr != nil {
		t.Fatalf("StoryGraph outbox cannot become the strict Kafka envelope: %v", envelopeErr)
	}
}

func TestStoryGraphConcurrentCASPublishesAtMostOneVersion(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL StoryGraph journey")
	}
	database, err := platformdatabase.Open(context.Background(), databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	fixture := seedStoryGraphOwners(t, func(value any) error { return database.Create(value).Error }, "concurrent")
	count := func(value any, query string, args ...any) (int64, error) {
		var result int64
		errorFound := database.Model(value).Where(query, args...).Count(&result).Error
		return result, errorFound
	}
	service := storygraphapp.NewService(storygraphgorm.New(database), storygraphapp.Config{
		Now:   func() time.Time { return time.Date(2026, time.August, 27, 4, 0, 0, 0, time.UTC) },
		NewID: uuid.NewString,
	})
	actor := storygraphapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	start := make(chan struct{})
	results := make(chan storygraphapp.CompileResult, 2)
	errorsFound := make(chan error, 2)
	var group sync.WaitGroup
	for index := 0; index < 2; index++ {
		group.Add(1)
		go func(key string) {
			defer group.Done()
			<-start
			result, compileErr := service.Compile(context.Background(), actor, storygraphapp.CompileCommand{
				ProjectID: fixture.projectID.String(), ExpectedHeadRevision: 0,
				ExpectedCurrentContentHash: "", IdempotencyKey: key,
			})
			if compileErr != nil {
				errorsFound <- compileErr
				return
			}
			results <- result
		}(fmt.Sprintf("compile-concurrent-%d", index))
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsFound)
	if len(results) != 1 || len(errorsFound) != 1 {
		t.Fatalf("concurrent CAS results=%d errors=%d", len(results), len(errorsFound))
	}
	assertPublicationCounts(t, count, fixture, 1, 1, 1, 1)
}

func TestStoryGraphOutboxFailureRollsBackVersionHeadAndReceipt(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL StoryGraph journey")
	}
	database, err := platformdatabase.Open(context.Background(), databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	fixture := seedStoryGraphOwners(t, func(value any) error { return database.Create(value).Error }, "rollback")
	count := func(value any, query string, args ...any) (int64, error) {
		var result int64
		errorFound := database.Model(value).Where(query, args...).Count(&result).Error
		return result, errorFound
	}
	versionID, receiptID, duplicateEventID := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, time.August, 27, 5, 0, 0, 0, time.UTC)
	seed := model.OutboxEvent{
		ID: duplicateEventID, EventType: "SeedEvent", EventVersion: 1,
		WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
		AggregateKind: "seed", AggregateID: uuid.NewString(), AggregateRevision: 1,
		SourceReceiptID: uuid.New(), Payload: []byte(`{"seed":true}`), PayloadHash: hashText("seed"),
		Status: "pending", OccurredAt: now, CreatedAt: now,
	}
	if err := database.Create(&seed).Error; err != nil {
		t.Fatal(err)
	}
	ids := []string{versionID.String(), receiptID.String(), duplicateEventID.String()}
	index := 0
	service := storygraphapp.NewService(storygraphgorm.New(database), storygraphapp.Config{
		Now: func() time.Time { return now },
		NewID: func() string {
			value := ids[index]
			index++
			return value
		},
	})
	_, err = service.Compile(context.Background(), storygraphapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, storygraphapp.CompileCommand{
		ProjectID: fixture.projectID.String(), ExpectedHeadRevision: 0,
		ExpectedCurrentContentHash: "", IdempotencyKey: "compile-rollback",
	})
	if err == nil {
		t.Fatal("duplicate Outbox ID did not fail publication")
	}
	assertPublicationCounts(t, count, fixture, 0, 0, 0, 1)
	var receiptCount int64
	if countErr := database.Model(&model.CommandReceipt{}).Where("id = ?", receiptID).Count(&receiptCount).Error; countErr != nil || receiptCount != 0 {
		t.Fatalf("failed transaction retained receipt: count=%d error=%v", receiptCount, countErr)
	}
}

func TestStoryGraphCompilationEnforcesTokenMembershipAndWriteRole(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL StoryGraph journey")
	}
	database, err := platformdatabase.Open(context.Background(), databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	fixture := seedStoryGraphOwners(t, func(value any) error { return database.Create(value).Error }, "authorization")
	now := time.Date(2026, time.August, 27, 6, 0, 0, 0, time.UTC)
	outsiderID, viewerID := uuid.New(), uuid.New()
	for _, record := range []any{
		&model.UserAccount{ID: outsiderID, EmailNormalized: outsiderID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "Outsider", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.UserAccount{ID: viewerID, EmailNormalized: viewerID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "Viewer", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: fixture.workspaceID, UserID: viewerID, Role: "viewer", Status: "active", JoinedAt: now},
	} {
		if err = database.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := storygraphapp.NewService(storygraphgorm.New(database), storygraphapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString})
	command := storygraphapp.CompileCommand{ProjectID: fixture.projectID.String(), ExpectedHeadRevision: 0, IdempotencyKey: "compile-authorization"}
	tests := []struct {
		name, code string
		actor      storygraphapp.Actor
	}{
		{"stale token", "unauthenticated", storygraphapp.Actor{UserID: fixture.userID.String(), TokenVersion: 2}},
		{"missing membership", "not_found", storygraphapp.Actor{UserID: outsiderID.String(), TokenVersion: 1}},
		{"viewer role", "forbidden", storygraphapp.Actor{UserID: viewerID.String(), TokenVersion: 1}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, compileErr := service.Compile(context.Background(), testCase.actor, command)
			assertApplicationError(t, compileErr, testCase.code)
		})
	}
	var versionCount int64
	if err = database.Model(&model.StoryGraphVersion{}).Where("project_id = ?", fixture.projectID).Count(&versionCount).Error; err != nil || versionCount != 0 {
		t.Fatalf("denied publication left a Version: count=%d error=%v", versionCount, err)
	}
}

type storyGraphOwnerFixture struct {
	userID, workspaceID, projectID, episodeID uuid.UUID
	text                                      string
}

func seedStoryGraphOwners(t *testing.T, create func(any) error, suffix string) storyGraphOwnerFixture {
	t.Helper()
	now := time.Date(2026, time.August, 27, 2, 0, 0, 0, time.UTC)
	userID, workspaceID, projectID := uuid.New(), uuid.New(), uuid.New()
	documentID, revisionID, episodeID := uuid.New(), uuid.New(), uuid.New()
	scriptVersionID, structureID := uuid.New(), uuid.New()
	text := "第一集\n内景·书房·夜\n小岚：我们开始吧。\n镜头转向窗外。"
	sceneID, dialogueID, beatID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	episodeNumber := 1
	evidence := bibledomain.Evidence{
		SourceStart: 0, SourceEnd: len([]rune(text)), TextHash: bibledomain.SourceTextHash(text),
		ExactAnchor: text, EpisodeNumber: &episodeNumber,
	}
	sceneValues := []planningdomain.Scene{{
		ID: sceneID, TemporaryKey: "scene:study", Heading: "内景·书房·夜", Position: 1,
		SourceStart: 0, SourceEnd: len([]rune(text)), Evidence: []bibledomain.Evidence{evidence},
		Dialogues: []planningdomain.Dialogue{{
			ID: dialogueID, TemporaryKey: "dialogue:start", Speaker: "小岚", Text: "我们开始吧。",
			SourceStart: 0, SourceEnd: len([]rune(text)), Evidence: []bibledomain.Evidence{evidence},
		}},
		NarrativeUnits: []planningdomain.NarrativeUnit{{
			ID: beatID, TemporaryKey: "beat:window", Kind: "action", Text: "镜头转向窗外。",
			SourceStart: 0, SourceEnd: len([]rune(text)), Evidence: []bibledomain.Evidence{evidence},
		}},
		Occurrences: []planningdomain.Occurrence{}, Claims: []planningdomain.PlanningClaim{}, Tasks: []planningdomain.ProductionTask{},
	}}
	scenes, err := json.Marshal(sceneValues)
	if err != nil {
		t.Fatal(err)
	}
	structureHash, err := bibledomain.CanonicalStoryHash(struct {
		Schema string                 `json:"schema"`
		Scenes []planningdomain.Scene `json:"scenes"`
	}{"episode-planning-owner-v1", sceneValues})
	if err != nil {
		t.Fatal(err)
	}
	currentScriptVersionID := scriptVersionID
	records := []any{
		&model.UserAccount{ID: userID, EmailNormalized: userID.String() + "-" + suffix + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "StoryGraph Test", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: workspaceID, Name: "StoryGraph " + suffix, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "owner", Status: "active", JoinedAt: now},
		&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "StoryGraph Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 90_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.ScriptDocument{ID: documentID, WorkspaceID: workspaceID, ProjectID: projectID, Title: "StoryGraph Script", SourceType: "text", Language: "zh-CN", RightsDeclaration: "原创测试文本", Status: "active", Revision: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.DocumentRevision{ID: revisionID, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: 1, SourceType: "text", RawText: text, RawHash: hashText(text), NormalizedText: text, NormalizedHash: hashText(text), NormalizerVersion: "test-v1", NormalizationMap: []byte(`{}`), CodepointCount: len([]rune(text)), AnalysisStatus: "deterministic", AnalyzerVersion: "test-v1", Blocks: []byte(`[]`), Issues: []byte(`[]`), CreatedBy: userID, CreatedAt: now},
		&model.Episode{ID: episodeID, WorkspaceID: workspaceID, ProjectID: projectID, Name: "第一集", Position: 1, TargetDurationMS: 90_000, Status: "active", Revision: 1, CurrentScriptVersionID: &currentScriptVersionID, CreatedAt: now, UpdatedAt: now},
		&model.EpisodeScriptVersion{ID: scriptVersionID, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, VersionNo: 1, DocumentRevisionID: revisionID, SourceStart: 0, SourceEnd: len([]rune(text)), Content: text, ContentHash: hashText(text), Status: "published", CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.EpisodeStructure{ID: structureID, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, ScriptVersionID: scriptVersionID, Status: "confirmed", Scenes: scenes, ResultHash: structureHash, Revision: 1, ConfirmedBy: &userID, ConfirmedAt: &now, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
	}
	for _, record := range records {
		if err = create(record); err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}
	return storyGraphOwnerFixture{userID: userID, workspaceID: workspaceID, projectID: projectID, episodeID: episodeID, text: text}
}

func assertPublicationCounts(t *testing.T, count func(any, string, ...any) (int64, error), fixture storyGraphOwnerFixture, versions, heads, receipts, events int64) {
	t.Helper()
	checks := []struct {
		model any
		query string
		args  []any
		want  int64
	}{
		{&model.StoryGraphVersion{}, "project_id = ?", []any{fixture.projectID}, versions},
		{&model.StoryGraphHead{}, "project_id = ?", []any{fixture.projectID}, heads},
		{&model.CommandReceipt{}, "workspace_id = ? AND operation = ?", []any{fixture.workspaceID, "storygraph.compile"}, receipts},
		{&model.OutboxEvent{}, "project_id = ?", []any{fixture.projectID}, events},
	}
	for _, check := range checks {
		actual, err := count(check.model, check.query, check.args...)
		if err != nil || actual != check.want {
			t.Fatalf("count %T = %d, want %d, error=%v", check.model, actual, check.want, err)
		}
	}
}

func findNode(t *testing.T, nodes []storygraph.Node, nodeType storygraph.NodeType) storygraph.Node {
	t.Helper()
	for _, node := range nodes {
		if node.NodeType == nodeType {
			return node
		}
	}
	t.Fatalf("node type %s not found", nodeType)
	return storygraph.Node{}
}

func hashText(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func assertApplicationError(t *testing.T, err error, code string) {
	t.Helper()
	var applicationError *storygraphapp.Error
	if !errors.As(err, &applicationError) || applicationError.Code != code {
		t.Fatalf("error=%v, want code=%s", err, code)
	}
}
