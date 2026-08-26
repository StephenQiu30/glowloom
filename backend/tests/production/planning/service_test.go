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
	source   domain.Source
	plan     domain.Plan
	receipts []platformcommand.Receipt
}

func (store *planningStore) WithinTransaction(_ context.Context, operation func(planningapp.Repository) error) error {
	return operation(store)
}
func (store *planningStore) RevisionSource(context.Context, planningapp.Actor, string, bool) (domain.Source, string, string, error) {
	return store.source, "workspace-1", "project-1", nil
}
func (store *planningStore) FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error) {
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
func (*planningStore) HasConfirmedBible(context.Context, string, string) (bool, error) {
	return false, errors.New("not implemented")
}
func (*planningStore) Materialize(context.Context, domain.Plan, domain.ImportCommit, []domain.Episode, []planningapp.Version) error {
	return errors.New("not implemented")
}
func (*planningStore) GetCommit(context.Context, planningapp.Actor, string, bool) (domain.ImportCommit, error) {
	return domain.ImportCommit{}, errors.New("not implemented")
}
func (*planningStore) GetPlanCommit(context.Context, planningapp.Actor, string) (domain.ImportCommit, error) {
	return domain.ImportCommit{}, errors.New("not implemented")
}
func (*planningStore) Publish(context.Context, domain.ImportCommit, []domain.Structure) error {
	return errors.New("not implemented")
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
