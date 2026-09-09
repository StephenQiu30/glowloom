package project_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

func TestConfirmEpisodeLifecyclePublishesAndReplaysOneProjectCheckpoint(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Project Episode lifecycle journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	rootDatabase := database
	t.Cleanup(func() { _ = platformdatabase.Close(rootDatabase) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatal(err)
	}
	database = database.Begin()
	if database.Error != nil {
		t.Fatal(database.Error)
	}
	t.Cleanup(func() { _ = database.Rollback().Error })

	now := time.Date(2026, time.September, 9, 4, 0, 0, 0, time.UTC)
	userID, workspaceID, projectID := uuid.New(), uuid.New(), uuid.New()
	documentID, revisionID, spanIndexID := uuid.New(), uuid.New(), uuid.New()
	text := "第一集\n1-1 港口 日 外\n阿青回到港口。\n第二集\n2-1 仓库 夜 内\n阿青找到钥匙。"
	sourceHash := sha256HexEpisodeLifecycle(text)
	for _, value := range []any{
		&model.UserAccount{ID: userID, EmailNormalized: uuid.NewString() + "@example.com", PasswordHash: "hash", TokenVersion: 1, DisplayName: "Owner", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: workspaceID, Name: "Lifecycle", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "owner", Status: "active", JoinedAt: now},
		&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "Lifecycle", AspectRatio: "16:9", Language: "zh-CN", TargetDurationMS: 90000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.ScriptDocument{ID: documentID, WorkspaceID: workspaceID, ProjectID: projectID, Title: "Script", SourceType: "text", Language: "zh-CN", RightsDeclaration: "authorized", Status: "active", Revision: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.DocumentRevision{ID: revisionID, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: 1, SourceType: "text", RawText: text, RawHash: sourceHash, NormalizedText: text, NormalizedHash: sourceHash, NormalizerVersion: "unicode-production", NormalizationMap: []byte(`{}`), CodepointCount: len([]rune(text)), AnalysisStatus: "deterministic", AnalyzerVersion: "script-parser-production", Blocks: []byte(`[]`), Issues: []byte(`[]`), CreatedBy: userID, CreatedAt: now},
		&model.SourceSpanIndexVersion{ID: spanIndexID, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: revisionID, SourceHash: sourceHash, NewlineNormalization: "lf", CodepointIndexRule: "unicode-code-point", CodepointCount: len([]rune(text)), UTF8ByteCount: len([]byte(text)), IndexManifest: []byte(`{"schema":"source-index"}`), ContentHash: sha256HexEpisodeLifecycle("span-index"), CreatedBy: userID, CreatedAt: now},
		&model.ScriptSourceScopeHead{ProjectID: projectID, WorkspaceID: workspaceID, DocumentLogicalID: documentID, CurrentDocumentRevisionID: revisionID, CurrentSpanIndexID: spanIndexID, HeadRevision: 1, HeadHash: sha256HexEpisodeLifecycle("source-head"), UpdatedAt: now},
	} {
		if err = database.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	emptyOrderHash, err := platformcommand.InputHash([]struct {
		ID       string
		Position int
	}{})
	if err != nil {
		t.Fatal(err)
	}
	firstEnd := len([]rune("第一集\n1-1 港口 日 外\n阿青回到港口。\n"))
	command := projectapp.ConfirmEpisodeLifecycleCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(),
		GateInputID: uuid.NewString(), GateInputHash: sha256HexEpisodeLifecycle("gate-input"),
		ReviewDecisionID: uuid.NewString(), SourceVersionID: revisionID.String(), SourceHash: sourceHash,
		ExpectedProjectRevision: 1, ExpectedActiveOrderHash: emptyOrderHash,
		EpisodeSpans: []projectdomain.EpisodeLifecycleSpan{
			{TemporaryEpisodeID: "episode_001", Position: 1, SourceStart: 0, SourceEnd: firstEnd, Heading: "第一集"},
			{TemporaryEpisodeID: "episode_002", Position: 2, SourceStart: firstEnd, SourceEnd: len([]rune(text)), Heading: "第二集"},
		},
		IdempotencyKey: "gate-1-project:" + projectID.String(),
	}
	service := projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString)
	actor := projectapp.Actor{UserID: userID.String(), TokenVersion: 1}
	first, err := service.ConfirmEpisodeLifecycle(ctx, actor, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ConfirmEpisodeLifecycle(ctx, actor, command)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.CollectionRootHash != second.CollectionRootHash ||
		first.SchemaVersion != projectdomain.EpisodeLifecycleSetSchemaVersion || len(first.Episodes) != 2 ||
		first.ReviewDecisionID != command.ReviewDecisionID || first.GateInputID != command.GateInputID {
		t.Fatalf("lifecycle first=%#v second=%#v", first, second)
	}
	var episodeCount, scriptVersionCount, receiptCount int64
	if err = database.Model(&model.Episode{}).Where("project_id = ?", projectID).Count(&episodeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.EpisodeScriptVersion{}).Where("project_id = ?", projectID).Count(&scriptVersionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).
		Where("workspace_id = ? AND operation = ?", workspaceID, "project.confirm_episode_lifecycle").
		Count(&receiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if episodeCount != 2 || scriptVersionCount != 2 || receiptCount != 1 {
		t.Fatalf("episodes=%d scripts=%d receipts=%d", episodeCount, scriptVersionCount, receiptCount)
	}
	for index, episode := range first.Episodes {
		if episode.Position != index+1 || episode.SourceStart != command.EpisodeSpans[index].SourceStart ||
			episode.SourceEnd != command.EpisodeSpans[index].SourceEnd || len(episode.ContentHash) != 64 {
			t.Fatalf("episode ref[%d]=%#v", index, episode)
		}
	}

	drifted := command
	drifted.EpisodeSpans = append([]projectdomain.EpisodeLifecycleSpan(nil), command.EpisodeSpans...)
	drifted.EpisodeSpans[0].Heading = "被篡改"
	if _, err = service.ConfirmEpisodeLifecycle(ctx, actor, drifted); !projectapp.IsCode(err, projectapp.CodeIdempotencyConflict) {
		t.Fatalf("same key with drifted input error = %v", err)
	}

	failingCommand := command
	failingCommand.GateInputID = uuid.NewString()
	failingCommand.GateInputHash = sha256HexEpisodeLifecycle("next-gate-input")
	failingCommand.ReviewDecisionID = uuid.NewString()
	failingCommand.ExpectedProjectRevision = first.ProjectRevision
	failingCommand.ExpectedActiveOrderHash = first.ActiveOrderHash
	failingCommand.IdempotencyKey = "gate-1-project-receipt-failure:" + projectID.String()
	failingCommand.EpisodeSpans = append([]projectdomain.EpisodeLifecycleSpan(nil), command.EpisodeSpans...)
	failingCommand.EpisodeSpans[0].Heading = "事务不应保留的标题"
	failingService := projectapp.NewService(projectgorm.New(database), func() time.Time { return now.Add(time.Minute) }, func() string {
		return first.ID
	})
	if _, err = failingService.ConfirmEpisodeLifecycle(ctx, actor, failingCommand); err == nil {
		t.Fatal("Episode lifecycle receipt collision did not fail")
	}
	var persistedProject model.Project
	if err = database.First(&persistedProject, "id = ?", projectID).Error; err != nil {
		t.Fatal(err)
	}
	var persistedEpisodes []model.Episode
	if err = database.Where("project_id = ?", projectID).Order("position").Find(&persistedEpisodes).Error; err != nil {
		t.Fatal(err)
	}
	var auditCount int64
	if err = database.Model(&model.AuditEvent{}).
		Where("workspace_id = ? AND action = ?", workspaceID, "project.episode_lifecycle_confirmed").
		Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.CommandReceipt{}).
		Where("workspace_id = ? AND operation = ?", workspaceID, "project.confirm_episode_lifecycle").
		Count(&receiptCount).Error; err != nil {
		t.Fatal(err)
	}
	if persistedProject.Revision != first.ProjectRevision || len(persistedEpisodes) != 2 ||
		persistedEpisodes[0].Name != "第一集" || receiptCount != 1 || auditCount != 1 {
		t.Fatalf(
			"receipt failure leaked facts: project=%#v episodes=%#v receipts=%d audits=%d",
			persistedProject, persistedEpisodes, receiptCount, auditCount,
		)
	}
}

func TestConfirmEpisodeLifecycleRejectsIncompleteCoverageWithoutWriting(t *testing.T) {
	service := projectapp.NewService(&rejectingEpisodeLifecycleStore{}, time.Now, uuid.NewString)
	_, err := service.ConfirmEpisodeLifecycle(context.Background(), projectapp.Actor{UserID: uuid.NewString(), TokenVersion: 1}, projectapp.ConfirmEpisodeLifecycleCommand{
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), GateInputID: uuid.NewString(),
		GateInputHash: sha256HexEpisodeLifecycle("gate"), ReviewDecisionID: uuid.NewString(),
		SourceVersionID: uuid.NewString(), SourceHash: sha256HexEpisodeLifecycle("source"),
		ExpectedProjectRevision: 1, ExpectedActiveOrderHash: sha256HexEpisodeLifecycle("order"),
		EpisodeSpans:   []projectdomain.EpisodeLifecycleSpan{{TemporaryEpisodeID: "episode_001", Position: 1, SourceStart: 1, SourceEnd: 2, Heading: "第一集"}},
		IdempotencyKey: "gate-1-project",
	})
	if !projectapp.IsCode(err, projectapp.CodeInvalidRequest) {
		t.Fatalf("incomplete coverage error = %v", err)
	}
}

type rejectingEpisodeLifecycleStore struct{}

func (*rejectingEpisodeLifecycleStore) WithinTransaction(context.Context, func(projectapp.Repository) error) error {
	return nil
}

func sha256HexEpisodeLifecycle(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
