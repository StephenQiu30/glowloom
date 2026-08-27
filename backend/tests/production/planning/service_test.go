package planning_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

func TestCreatePlanPreservesEverySourceRune(t *testing.T) {
	text := "第一集\n《开始》\n内景·房间·日\n小兰：走吧\n\n第二集\n《结束》\n外景·街道·夜\n小兰离开。"
	firstEnd := len([]rune("第一集"))
	secondStart := len([]rune("第一集\n《开始》\n内景·房间·日\n小兰：走吧\n\n"))
	store := &planningStore{source: domain.Source{
		DocumentRevisionID: "revision-1",
		NormalizedHash:     strings.Repeat("a", 64),
		NormalizedText:     text,
		Blocks: []domain.Block{
			{ID: "a", Position: 1, Kind: "episode_marker", SourceStart: 0, SourceEnd: firstEnd},
			{ID: "b", Position: 2, Kind: "action", SourceStart: firstEnd + 1, SourceEnd: firstEnd + 5},
			{ID: "c", Position: 3, Kind: "scene_heading", SourceStart: firstEnd + 6, SourceEnd: firstEnd + 13},
			{ID: "d", Position: 4, Kind: "dialogue", SourceStart: firstEnd + 14, SourceEnd: secondStart - 2},
			{ID: "e", Position: 5, Kind: "separator", SourceStart: secondStart - 1, SourceEnd: secondStart - 1},
			{ID: "f", Position: 6, Kind: "episode_marker", SourceStart: secondStart, SourceEnd: secondStart + firstEnd},
			{ID: "g", Position: 7, Kind: "action", SourceStart: secondStart + firstEnd + 1, SourceEnd: secondStart + firstEnd + 5},
			{ID: "h", Position: 8, Kind: "action", SourceStart: secondStart + firstEnd + 6, SourceEnd: len([]rune(text))},
		},
	}}
	service := planningapp.NewService(store, planningapp.Config{
		Now:   func() time.Time { return time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC) },
		NewID: sequenceIDs(),
	})
	view, err := service.CreatePlan(context.Background(), planningapp.Actor{UserID: "user-1", TokenVersion: 1}, planningapp.CreatePlanCommand{
		RevisionID: "revision-1", Strategy: "explicit_markers", TargetDurationMS: 90_000,
		IdempotencyKey: "create-plan-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposals := view.Plan.Proposals
	if len(proposals) != 2 || proposals[0].Title != "开始" || proposals[1].Title != "结束" {
		t.Fatalf("unexpected proposals: %#v", proposals)
	}
	if proposals[0].SourceStart != 0 || proposals[0].SourceEnd != proposals[1].SourceStart ||
		proposals[1].SourceEnd != len([]rune(text)) {
		t.Fatalf("source is not conserved: %#v", proposals)
	}
	hash := sha256.Sum256([]byte(string([]rune(text)[proposals[0].SourceStart:proposals[0].SourceEnd])))
	if proposals[0].ContentHash != hex.EncodeToString(hash[:]) {
		t.Fatal("proposal hash does not bind its exact source range")
	}
}

type planningStore struct {
	source               domain.Source
	segmentation         planningapp.EpisodeSegmentationSource
	plan                 domain.Plan
	commit               domain.ImportCommit
	receipts             []platformcommand.Receipt
	episodes             []domain.Episode
	versions             []planningapp.Version
	events               []domain.OutboxEvent
	published            bool
	episodeSetApplyCount int
	episodeSetApplyError error
}

func (store *planningStore) WithinTransaction(_ context.Context, operation func(planningapp.Repository) error) error {
	return operation(store)
}
func (store *planningStore) RevisionSource(context.Context, planningapp.Actor, string, bool) (domain.Source, string, string, error) {
	return store.source, "workspace-1", "project-1", nil
}
func (store *planningStore) FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error) {
	for _, receipt := range store.receipts {
		if receipt.Operation == "episode_plan.apply" {
			return receipt, nil
		}
	}
	return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
}
func (store *planningStore) CreateReceipt(_ context.Context, receipt platformcommand.Receipt) error {
	store.receipts = append(store.receipts, receipt)
	return nil
}
func (store *planningStore) CreatePlan(_ context.Context, plan domain.Plan) error {
	store.plan = plan
	return nil
}
func (store *planningStore) GetPlan(context.Context, planningapp.Actor, string, bool) (domain.Plan, error) {
	return store.plan, nil
}
func (store *planningStore) SavePlan(_ context.Context, plan domain.Plan) error {
	store.plan = plan
	return nil
}
func (store *planningStore) ProjectImpact(context.Context, planningapp.Actor, string, bool) (domain.Impact, error) {
	return domain.Impact{}, nil
}
func (store *planningStore) HasConfirmedBible(context.Context, string, string) (bool, error) {
	return store.published, nil
}
func (store *planningStore) GetEpisodeSegmentationSource(
	context.Context,
	planningapp.Actor,
	string,
	bool,
) (planningapp.EpisodeSegmentationSource, error) {
	return store.segmentation, nil
}
func (store *planningStore) GetEpisodeSet(
	context.Context,
	planningapp.Actor,
	planningapp.EpisodeSetReference,
) ([]domain.Episode, []planningapp.Version, error) {
	return append([]domain.Episode(nil), store.episodes...), append([]planningapp.Version(nil), store.versions...), nil
}
func (store *planningStore) ApplyEpisodeSet(
	_ context.Context,
	episodes []domain.Episode,
	versions []planningapp.Version,
	events []domain.OutboxEvent,
) error {
	store.episodeSetApplyCount++
	if store.episodeSetApplyError != nil {
		return store.episodeSetApplyError
	}
	store.episodes = append([]domain.Episode(nil), episodes...)
	store.versions = append([]planningapp.Version(nil), versions...)
	store.events = append([]domain.OutboxEvent(nil), events...)
	return nil
}
func (*planningStore) Materialize(context.Context, domain.Plan, domain.ImportCommit, []domain.Episode, []planningapp.Version) error {
	return errors.New("not implemented")
}
func (store *planningStore) GetCommit(context.Context, planningapp.Actor, string, bool) (domain.ImportCommit, error) {
	return store.commit, nil
}
func (*planningStore) GetPlanCommit(context.Context, planningapp.Actor, string) (domain.ImportCommit, error) {
	return domain.ImportCommit{}, errors.New("not implemented")
}
func (store *planningStore) Publish(_ context.Context, commit domain.ImportCommit, _ []domain.Structure, events []domain.OutboxEvent) error {
	store.commit, store.events = commit, append([]domain.OutboxEvent(nil), events...)
	return nil
}

func TestPublishCreatesOneScriptEventPerPublishedVersionInTheOwnerTransaction(t *testing.T) {
	now := time.Date(2026, time.August, 27, 20, 0, 0, 0, time.UTC)
	workspaceID := "63000000-0000-0000-0000-000000000001"
	projectID := "63000000-0000-0000-0000-000000000002"
	revisionID := "63000000-0000-0000-0000-000000000003"
	episodeID := "63000000-0000-0000-0000-000000000004"
	versionID := "63000000-0000-0000-0000-000000000005"
	commitID := "63000000-0000-0000-0000-000000000006"
	planID := "63000000-0000-0000-0000-000000000007"
	store := &planningStore{
		published: true,
		plan:      domain.Plan{ID: planID, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: revisionID},
		commit: domain.ImportCommit{
			ID: commitID, WorkspaceID: workspaceID, ProjectID: projectID, PlanID: planID,
			Status: "materialized", Revision: 1, Segments: []domain.Segment{{
				EpisodeID: episodeID, DraftVersionID: versionID, DocumentRevisionID: revisionID,
				SourceStart: 4, SourceEnd: 12, SourceHash: strings.Repeat("a", 64), Content: "内景·房间·夜",
			}},
		},
	}
	service := planningapp.NewService(store, planningapp.Config{Now: func() time.Time { return now }, NewID: sequenceIDs()})
	result, err := service.Publish(context.Background(), planningapp.Actor{UserID: workspaceID, TokenVersion: 1}, planningapp.PublishCommand{
		CommitID: commitID, ExpectedRevision: 1, IdempotencyKey: "publish-with-script-event",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "published" || len(store.events) != 1 || len(store.receipts) != 1 {
		t.Fatalf("script publication did not atomically stage receipt and event: result=%#v events=%#v receipts=%#v", result, store.events, store.receipts)
	}
	event := store.events[0]
	if event.EventType != "ScriptVersionPublished" || event.AggregateKind != "episode_script" || event.AggregateID != episodeID ||
		event.AggregateRevision != 1 || event.SourceReceiptID != store.receipts[0].ID || event.PayloadHash == "" {
		t.Fatalf("script event identity is incomplete: %#v", event)
	}
}
func (*planningStore) ListEpisodes(context.Context, planningapp.Actor, string) ([]domain.Episode, error) {
	return nil, errors.New("not implemented")
}
func (*planningStore) GetEpisode(context.Context, planningapp.Actor, string, bool) (domain.Episode, error) {
	return domain.Episode{}, errors.New("not implemented")
}
func (*planningStore) GetStructure(context.Context, planningapp.Actor, string, bool) (domain.Structure, error) {
	return domain.Structure{}, errors.New("not implemented")
}
func (*planningStore) GetEpisodeStructure(context.Context, planningapp.Actor, string) (domain.Structure, error) {
	return domain.Structure{}, errors.New("not implemented")
}
func (*planningStore) SaveStructure(context.Context, domain.Structure) error {
	return errors.New("not implemented")
}

func sequenceIDs() func() string {
	index := 0
	return func() string {
		index++
		return fmt.Sprintf("00000000-0000-0000-0000-%012d", index)
	}
}

func TestApplyEpisodePlanAtomicallyPublishesEveryBoundaryAndReplaysTheOwnerReceipt(t *testing.T) {
	now := time.Date(2026, time.August, 28, 8, 0, 0, 0, time.UTC)
	text := "第一集\n雨夜相逢。\n第二集\n破晓离别。"
	secondStart := len([]rune("第一集\n雨夜相逢。\n"))
	firstEpisode := 1
	secondEpisode := 2
	firstEvidence := bibledomain.Evidence{
		SourceStart: 0, SourceEnd: 3, TextHash: bibledomain.SourceTextHash("第一集"), ExactAnchor: "第一集", EpisodeNumber: &firstEpisode,
	}
	secondEvidence := bibledomain.Evidence{
		SourceStart: secondStart, SourceEnd: secondStart + 3, TextHash: bibledomain.SourceTextHash("第二集"), ExactAnchor: "第二集", EpisodeNumber: &secondEpisode,
	}
	store := &planningStore{segmentation: planningapp.EpisodeSegmentationSource{
		CandidateRevisionID:   "63000000-0000-0000-0000-000000000101",
		CandidateRevisionHash: strings.Repeat("a", 64), CandidateRevision: 1,
		WorkspaceID:          "63000000-0000-0000-0000-000000000102",
		ProjectID:            "63000000-0000-0000-0000-000000000103",
		DocumentRevisionID:   "63000000-0000-0000-0000-000000000104",
		DocumentRevisionHash: bibledomain.SourceTextHash(text), NormalizedText: text,
		TargetDurationMS: 90_000,
		AllowedEvidence:  []bibledomain.Evidence{firstEvidence, secondEvidence},
		Markers: []bibledomain.EpisodeSegmentationMarker{
			{EpisodeNumber: 1, Label: "第一集", Evidence: firstEvidence},
			{EpisodeNumber: 2, Label: "第二集", Evidence: secondEvidence},
		},
		Candidate: bibledomain.EpisodeSegmentationCandidate{
			Boundaries: []bibledomain.EpisodeBoundary{
				{BoundaryKey: "episode-001", EpisodeOrder: 1, Title: "雨夜相逢", AbsoluteStart: 0, AbsoluteEnd: secondStart, Evidence: []bibledomain.Evidence{firstEvidence}},
				{BoundaryKey: "episode-002", EpisodeOrder: 2, Title: "破晓离别", AbsoluteStart: secondStart, AbsoluteEnd: len([]rune(text)), Evidence: []bibledomain.Evidence{secondEvidence}},
			},
			ReviewIssues: []bibledomain.ReviewIssue{},
		},
	}}
	service := planningapp.NewService(store, planningapp.Config{Now: func() time.Time { return now }, NewID: sequenceIDs()})
	command := planningapp.ApplyEpisodePlanCommand{
		CandidateRevisionID:       store.segmentation.CandidateRevisionID,
		CandidateRevisionHash:     store.segmentation.CandidateRevisionHash,
		ExpectedCandidateRevision: 1,
		ReviewDecisionID:          "63000000-0000-0000-0000-000000000105",
		IdempotencyKey:            "workflow-review:63000000-0000-0000-0000-000000000105",
	}

	result, err := service.ApplyEpisodePlan(context.Background(), planningapp.Actor{
		UserID: "63000000-0000-0000-0000-000000000106", TokenVersion: 1,
	}, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Set.ID != result.Receipt.ID || result.Set.ContentHash == "" || result.Set.CandidateRevisionID != command.CandidateRevisionID ||
		result.Receipt.Operation != "episode_plan.apply" || result.Receipt.ResourceID != command.CandidateRevisionID {
		t.Fatalf("Episode set receipt is incomplete: %#v", result)
	}
	if len(result.Set.Episodes) != 2 || len(store.episodes) != 2 || len(store.versions) != 2 || len(store.events) != 2 || store.episodeSetApplyCount != 1 {
		t.Fatalf("Episode set was not applied as one complete batch: result=%#v episodes=%#v versions=%#v events=%#v", result, store.episodes, store.versions, store.events)
	}
	for index := range store.episodes {
		episode, version := store.episodes[index], store.versions[index]
		if episode.CurrentScriptVersionID == nil || *episode.CurrentScriptVersionID != version.ID || episode.Status != "active" ||
			version.Status != "published" || version.VersionNo != 1 || version.ContentHash != bibledomain.SourceTextHash(version.Content) ||
			store.events[index].SourceReceiptID != result.Receipt.ID {
			t.Fatalf("published Episode boundary %d is incomplete: episode=%#v version=%#v event=%#v", index, episode, version, store.events[index])
		}
	}

	replayed, err := service.ApplyEpisodePlan(context.Background(), planningapp.Actor{
		UserID: "63000000-0000-0000-0000-000000000106", TokenVersion: 1,
	}, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Set.ID != result.Set.ID || replayed.Set.ContentHash != result.Set.ContentHash ||
		replayed.Receipt.ID != result.Receipt.ID || store.episodeSetApplyCount != 1 {
		t.Fatalf("Episode set replay created a second owner effect: first=%#v replay=%#v apply_count=%d", result, replayed, store.episodeSetApplyCount)
	}
}

func TestApplyEpisodePlanRejectsBlockingCandidateWithoutOwnerWrites(t *testing.T) {
	store := &planningStore{segmentation: planningapp.EpisodeSegmentationSource{
		CandidateRevisionID: "63000000-0000-0000-0000-000000000201", CandidateRevisionHash: strings.Repeat("b", 64), CandidateRevision: 1,
		WorkspaceID: "63000000-0000-0000-0000-000000000202", ProjectID: "63000000-0000-0000-0000-000000000203",
		DocumentRevisionID: "63000000-0000-0000-0000-000000000204", DocumentRevisionHash: bibledomain.SourceTextHash("第一集"),
		NormalizedText: "第一集", TargetDurationMS: 90_000,
		AllowedEvidence: []bibledomain.Evidence{{SourceStart: 0, SourceEnd: 3, TextHash: bibledomain.SourceTextHash("第一集"), ExactAnchor: "第一集"}},
		Candidate: bibledomain.EpisodeSegmentationCandidate{
			Boundaries:   []bibledomain.EpisodeBoundary{{BoundaryKey: "episode-001", EpisodeOrder: 1, Title: "第一集", AbsoluteStart: 0, AbsoluteEnd: 3, Evidence: []bibledomain.Evidence{{SourceStart: 0, SourceEnd: 3, TextHash: bibledomain.SourceTextHash("第一集"), ExactAnchor: "第一集"}}}},
			ReviewIssues: []bibledomain.ReviewIssue{{IssueKey: "episode-boundary-uncertain", Code: "boundary_uncertain", Severity: "blocking", Scope: "episode-001", Summary: "边界仍不确定", Evidence: []bibledomain.Evidence{}}},
		},
	}}
	service := planningapp.NewService(store, planningapp.Config{Now: time.Now, NewID: sequenceIDs()})
	_, err := service.ApplyEpisodePlan(context.Background(), planningapp.Actor{UserID: "63000000-0000-0000-0000-000000000205", TokenVersion: 1}, planningapp.ApplyEpisodePlanCommand{
		CandidateRevisionID: store.segmentation.CandidateRevisionID, CandidateRevisionHash: store.segmentation.CandidateRevisionHash,
		ExpectedCandidateRevision: 1, ReviewDecisionID: "63000000-0000-0000-0000-000000000206", IdempotencyKey: "workflow-review:blocking",
	})
	if err == nil {
		t.Fatal("blocking Episode segmentation candidate was applied")
	}
	if len(store.episodes) != 0 || len(store.versions) != 0 || len(store.events) != 0 || len(store.receipts) != 0 || store.episodeSetApplyCount != 0 {
		t.Fatalf("blocking candidate produced owner writes: %#v", store)
	}
}
