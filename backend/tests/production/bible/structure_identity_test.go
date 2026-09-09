package bible_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	eventingdomain "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

func TestConfirmStructureIdentitySetPublishesGateOneCheckpoint(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Structure Identity owner journey")
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

	now := time.Date(2026, time.September, 9, 6, 0, 0, 0, time.UTC)
	userID, workspaceID, projectID := uuid.New(), uuid.New(), uuid.New()
	documentID, revisionID, spanIndexID := uuid.New(), uuid.New(), uuid.New()
	text := "第一集\n1-1 港口 日 外\n阿青回到港口。\n第二集\n2-1 仓库 夜 内\n阿青找到钥匙。"
	sourceHash := structureIdentityHash(text)
	spanIndexHash := structureIdentityHash("span-index")
	for _, value := range []any{
		&model.UserAccount{ID: userID, EmailNormalized: uuid.NewString() + "@example.com", PasswordHash: "hash", TokenVersion: 1, DisplayName: "Owner", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: workspaceID, Name: "Structure Identity", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "owner", Status: "active", JoinedAt: now},
		&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "Structure Identity", AspectRatio: "16:9", Language: "zh-CN", TargetDurationMS: 90000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.ScriptDocument{ID: documentID, WorkspaceID: workspaceID, ProjectID: projectID, Title: "Script", SourceType: "text", Language: "zh-CN", RightsDeclaration: "authorized", Status: "active", Revision: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.DocumentRevision{ID: revisionID, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: 1, SourceType: "text", RawText: text, RawHash: sourceHash, NormalizedText: text, NormalizedHash: sourceHash, NormalizerVersion: "unicode-production", NormalizationMap: []byte(`{}`), CodepointCount: len([]rune(text)), AnalysisStatus: "deterministic", AnalyzerVersion: "script-parser-production", Blocks: []byte(`[]`), Issues: []byte(`[]`), CreatedBy: userID, CreatedAt: now},
		&model.SourceSpanIndexVersion{ID: spanIndexID, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: revisionID, SourceHash: sourceHash, NewlineNormalization: "lf", CodepointIndexRule: "unicode-code-point", CodepointCount: len([]rune(text)), UTF8ByteCount: len([]byte(text)), IndexManifest: []byte(`{"schema":"source-index"}`), ContentHash: spanIndexHash, CreatedBy: userID, CreatedAt: now},
		&model.ScriptSourceScopeHead{ProjectID: projectID, WorkspaceID: workspaceID, DocumentLogicalID: documentID, CurrentDocumentRevisionID: revisionID, CurrentSpanIndexID: spanIndexID, HeadRevision: 1, HeadHash: structureIdentityHash("source-head"), UpdatedAt: now},
	} {
		if err = database.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	firstEnd := len([]rune("第一集\n1-1 港口 日 外\n阿青回到港口。\n"))
	emptyOrderHash, err := platformcommand.InputHash([]struct {
		ID       string
		Position int
	}{})
	if err != nil {
		t.Fatal(err)
	}
	gateInputID, reviewDecisionID := uuid.NewString(), uuid.NewString()
	projectService := projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString)
	episodes, err := projectService.ConfirmEpisodeLifecycle(ctx, projectapp.Actor{UserID: userID.String(), TokenVersion: 1}, projectapp.ConfirmEpisodeLifecycleCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), GateInputID: gateInputID,
		GateInputHash: structureIdentityHash("gate-input"), ReviewDecisionID: reviewDecisionID,
		SourceVersionID: revisionID.String(), SourceHash: sourceHash,
		ExpectedProjectRevision: 1, ExpectedActiveOrderHash: emptyOrderHash,
		EpisodeSpans: []projectdomain.EpisodeLifecycleSpan{
			{TemporaryEpisodeID: "episode_001", Position: 1, SourceStart: 0, SourceEnd: firstEnd, Heading: "第一集"},
			{TemporaryEpisodeID: "episode_002", Position: 2, SourceStart: firstEnd, SourceEnd: len([]rune(text)), Heading: "第二集"},
		},
		IdempotencyKey: "gate-1-project:" + projectID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}

	command := structureIdentityCommand(t, workspaceID, projectID, revisionID, spanIndexID, episodes, gateInputID, reviewDecisionID, sourceHash, spanIndexHash, firstEnd, len([]rune(text)))
	service := bibleapp.NewService(biblegorm.New(database), bibleapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString})
	actor := bibleapp.Actor{UserID: userID.String(), TokenVersion: 1}
	assertCounts := func(versions, heads, collectionReceipts, commandReceipts, outboxEvents int64) {
		t.Helper()
		checks := []struct {
			model any
			where string
			args  []any
			want  int64
		}{
			{&model.StructureIdentitySetVersion{}, "project_id = ?", []any{projectID}, versions},
			{&model.StructureIdentityScopeHead{}, "project_id = ?", []any{projectID}, heads},
			{&model.StructureIdentityCollectionReceipt{}, "project_id = ?", []any{projectID}, collectionReceipts},
			{&model.CommandReceipt{}, "workspace_id = ? AND operation = ?", []any{workspaceID, "production_bible.confirm_structure_identity_set"}, commandReceipts},
			{&model.OutboxEvent{}, "project_id = ? AND event_type = ?", []any{projectID, eventingdomain.StructureIdentitySetPublished}, outboxEvents},
		}
		for _, check := range checks {
			var count int64
			if countErr := database.Model(check.model).Where(check.where, check.args...).Count(&count).Error; countErr != nil {
				t.Fatal(countErr)
			}
			if count != check.want {
				t.Fatalf("%T count=%d want=%d", check.model, count, check.want)
			}
		}
	}
	first, err := service.ConfirmStructureIdentitySet(ctx, actor, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ConfirmStructureIdentitySet(ctx, actor, command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version.ID != second.Version.ID || first.Receipt.ID != second.Receipt.ID ||
		first.CommandReceiptID != second.CommandReceiptID || first.CommandOperation != bibledomain.StructureIdentityCommandOperation ||
		first.Version.SchemaVersion != bibledomain.StructureIdentitySetSchemaVersion || first.Version.Version != 1 ||
		first.Version.ProjectEpisodeReceiptID != episodes.ID || first.Receipt.CheckpointKey != bibledomain.StructureIdentityCheckpointKey ||
		first.Receipt.CollectionFamily != bibledomain.StructureIdentityCollectionFamily || len(first.Receipt.CoveredScopeKeys) != 2 {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	queried, err := bibleapp.NewStructureIdentityQuery(biblegorm.New(database), projectService).GetCurrent(
		ctx,
		actor,
		projectID.String(),
	)
	if err != nil || queried.Version.ID != first.Version.ID || queried.Version.ContentHash != first.Version.ContentHash ||
		queried.Receipt.ID != first.Receipt.ID || queried.Receipt.ReceiptContentHash != first.Receipt.ReceiptContentHash {
		t.Fatalf("query current Structure Identity: result=%#v err=%v", queried, err)
	}

	assertCounts(1, 1, 1, 1, 1)
	var outbox model.OutboxEvent
	if err = database.Where("aggregate_id = ?", first.Version.ID).First(&outbox).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = eventingdomain.NewEnvelope(eventingdomain.OutboxEvent{
		ID: outbox.ID.String(), EventType: outbox.EventType, EventVersion: outbox.EventVersion,
		WorkspaceID: outbox.WorkspaceID.String(), ProjectID: outbox.ProjectID.String(),
		AggregateKind: outbox.AggregateKind, AggregateID: outbox.AggregateID,
		AggregateRevision: outbox.AggregateRevision, SourceReceiptID: outbox.SourceReceiptID.String(),
		Payload: json.RawMessage(outbox.Payload), PayloadHash: outbox.PayloadHash, OccurredAt: outbox.OccurredAt,
	}, eventingdomain.TraceContext{RequestID: uuid.NewString()}); err != nil {
		t.Fatalf("committed event is not publishable: %v", err)
	}

	drifted := command
	drifted.MentionMappings = append([]bibledomain.StructureIdentityMentionMapping(nil), command.MentionMappings...)
	drifted.MentionMappings[0].ExactAnchor = "被篡改"
	if _, err = service.ConfirmStructureIdentitySet(ctx, actor, drifted); err == nil {
		t.Fatal("same idempotency key accepted drifted input")
	}

	failing := command
	failing.GateInputID, failing.GateInputHash = uuid.NewString(), structureIdentityHash("next-gate")
	failing.ReviewDecisionID = uuid.NewString()
	failing.ExpectedHeadRevision, failing.ExpectedHeadHash = 1, first.Version.ContentHash
	failing.IdempotencyKey = "gate-1-bible-receipt-failure:" + projectID.String()
	failing.Identities = append([]bibledomain.StructureIdentity(nil), command.Identities...)
	reuseKey := failing.Identities[0].IdentityKey
	failing.Identities[0].Resolution, failing.Identities[0].ReuseIdentityKey = "reuse", &reuseKey
	generatedVersionID, generatedCollectionID := uuid.NewString(), uuid.NewString()
	ids := []string{generatedVersionID, generatedCollectionID, episodes.ID, uuid.NewString()}
	failingService := bibleapp.NewService(biblegorm.New(database), bibleapp.Config{
		Now: func() time.Time { return now.Add(time.Minute) },
		NewID: func() string {
			value := ids[0]
			ids = ids[1:]
			return value
		},
	})
	if _, err = failingService.ConfirmStructureIdentitySet(ctx, actor, failing); err == nil {
		t.Fatal("command receipt collision did not fail")
	}
	assertCounts(1, 1, 1, 1, 1)
	var head model.StructureIdentityScopeHead
	if err = database.First(&head, "project_id = ?", projectID).Error; err != nil {
		t.Fatal(err)
	}
	if head.CurrentVersionID.String() != first.Version.ID || head.HeadRevision != 1 || head.HeadHash != first.Version.ContentHash {
		t.Fatalf("failed transaction changed Head: %#v", head)
	}
	for _, generatedID := range []string{generatedVersionID, generatedCollectionID} {
		var count int64
		if err = database.Table("scr_structure_identity_set_versions").Where("id = ?", generatedID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("failed transaction leaked %s", generatedID)
		}
	}

	var assetCount, episodeStructureCount, storyGraphCount int64
	if err = database.Model(&model.Asset{}).Where("project_id = ?", projectID).Count(&assetCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.EpisodeStructure{}).Where("project_id = ?", projectID).Count(&episodeStructureCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.StoryGraphVersion{}).Where("project_id = ?", projectID).Count(&storyGraphCount).Error; err != nil {
		t.Fatal(err)
	}
	if assetCount != 0 || episodeStructureCount != 0 || storyGraphCount != 0 {
		t.Fatalf("Gate 1 crossed owner boundary: assets=%d structures=%d storygraphs=%d", assetCount, episodeStructureCount, storyGraphCount)
	}
	if err = database.Model(&model.StructureIdentityScopeHead{}).
		Where("project_id = ?", projectID).
		Update("head_hash", structureIdentityHash("tampered-head")).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = bibleapp.NewStructureIdentityQuery(biblegorm.New(database), projectService).GetCurrent(
		ctx,
		actor,
		projectID.String(),
	); err == nil {
		t.Fatal("drifted current Structure Identity Head returned")
	}
}

func structureIdentityCommand(
	t *testing.T,
	workspaceID, projectID, revisionID, spanIndexID uuid.UUID,
	episodes projectdomain.EpisodeLifecycleSet,
	gateInputID, reviewDecisionID, sourceHash, spanIndexHash string,
	firstEnd, sourceEnd int,
) bibleapp.ConfirmStructureIdentitySetCommand {
	t.Helper()
	identityTemporaryKey := "identity_character_aqing"
	identityKey := uuid.NewSHA1(projectID, []byte("lanverse:identity:"+identityTemporaryKey)).String()
	scenes := []bibledomain.StructureIdentitySceneRef{
		{TemporaryEpisodeID: "episode_001", EpisodeID: episodes.Episodes[0].EpisodeID, TemporarySpanID: "span_scene_001", TemporarySceneID: "scene_001", SceneOwnerLogicalID: uuid.NewSHA1(projectID, []byte("lanverse:scene:span_scene_001")).String(), SourceStart: 0, SourceEnd: firstEnd, EvidenceHash: structureIdentityHash("scene-1")},
		{TemporaryEpisodeID: "episode_002", EpisodeID: episodes.Episodes[1].EpisodeID, TemporarySpanID: "span_scene_002", TemporarySceneID: "scene_002", SceneOwnerLogicalID: uuid.NewSHA1(projectID, []byte("lanverse:scene:span_scene_002")).String(), SourceStart: firstEnd, SourceEnd: sourceEnd, EvidenceHash: structureIdentityHash("scene-2")},
	}
	for index := range scenes {
		scenes[index].ScopeKey = "scene:" + scenes[index].SceneOwnerLogicalID
	}
	mentions := []bibledomain.StructureIdentityMentionMapping{
		{Kind: "location", TemporarySceneID: "scene_001", SourceStart: 0, SourceEnd: 1, TextHash: structureIdentityHash("第"), ExactAnchor: "第", Resolution: "unresolved"},
		{Kind: "character", TemporarySceneID: "scene_002", SourceStart: firstEnd, SourceEnd: firstEnd + 1, TextHash: structureIdentityHash("第"), ExactAnchor: "第", Resolution: "resolved", IdentityKey: &identityKey},
	}
	mentionHash, err := platformcommand.InputHash(mentions)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{scenes[0].ScopeKey, scenes[1].ScopeKey}
	scopeHash, err := platformcommand.InputHash(scopes)
	if err != nil {
		t.Fatal(err)
	}
	stages := []string{"extract_scene_facts", "propose_script_spans", "resolve_identities", "review_candidate"}
	candidates := make([]bibledomain.StructureIdentityCandidateRef, len(stages))
	for index, stage := range stages {
		candidates[index] = bibledomain.StructureIdentityCandidateRef{
			StageKey: stage, ShardKey: "script:full",
			CandidateRevisionID: uuid.NewString(), CandidateRevisionHash: structureIdentityHash(stage + ":candidate"),
			SourceInvocationID: uuid.NewString(), SourceResultHash: structureIdentityHash(stage + ":result"),
			SkillReleaseID: uuid.NewString(), SkillReleaseHash: structureIdentityHash(stage + ":skill-release"),
			StageReleaseHash: structureIdentityHash(stage + ":stage-release"), BundleContentHash: structureIdentityHash("bundle"),
			AgentImageDigest: "sha256:" + structureIdentityHash("agent-image"),
		}
	}
	return bibleapp.ConfirmStructureIdentitySetCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), GateInputID: gateInputID,
		GateInputHash: structureIdentityHash("gate-input"), ReviewDecisionID: reviewDecisionID,
		ProjectEpisodeReceiptID: episodes.ID, DocumentRevisionID: revisionID.String(), DocumentRevisionHash: sourceHash,
		SpanIndexID: spanIndexID.String(), SpanIndexHash: spanIndexHash, ExpectedHeadRevision: 0,
		CandidateRefs: candidates, SceneRefs: scenes,
		Identities:      []bibledomain.StructureIdentity{{TemporaryIdentityKey: identityTemporaryKey, IdentityKey: identityKey, Kind: "character", Resolution: "new", CanonicalName: "阿青", Aliases: []string{"阿青"}}},
		MentionMappings: mentions,
		Coverage:        bibledomain.StructureIdentityCoverage{SceneCount: 2, IdentityCount: 1, MentionCount: 2, ResolvedCount: 1, UnresolvedCount: 1, MentionUniverseHash: mentionHash, ScopeSetHash: scopeHash},
		IdempotencyKey:  "gate-1-bible:" + projectID.String(),
	}
}

func structureIdentityHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
