package bible_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type materializationStore struct {
	fakeBibleStore
	scope       bibleapp.MaterializationScope
	write       bibleapp.MaterializationWrite
	createCalls int
}

func (store *materializationStore) WithinTransaction(
	_ context.Context,
	operation func(bibleapp.Repository) error,
) error {
	return operation(store)
}

func (store *materializationStore) PrepareMaterialization(
	context.Context,
	bibleapp.Actor,
	string,
	bool,
) (bibleapp.MaterializationScope, error) {
	return store.scope, nil
}

func (store *materializationStore) CreateMaterialization(
	_ context.Context,
	write bibleapp.MaterializationWrite,
) error {
	store.createCalls++
	store.write = write
	return nil
}

func (store *materializationStore) VerifyMaterialization(
	context.Context,
	bibleapp.Actor,
	bibledomain.Materialization,
) error {
	return nil
}

func TestMaterializeConfirmedBibleKeepsExactIdentityAndReplaysOneReceipt(t *testing.T) {
	version := materializationBibleVersion(t)
	store := &materializationStore{
		fakeBibleStore: fakeBibleStore{
			versions: map[string]bibledomain.ProductionBibleVersion{version.ID: version},
			receipts: map[string]platformcommand.Receipt{},
		},
		scope: bibleapp.MaterializationScope{
			Version:                 version,
			AssetsByEntityKey:       map[string]assetdomain.Asset{},
			SpecificationsByAssetID: map[string][]bibledomain.SpecificationVersion{},
			StatesByAssetID:         map[string][]assetdomain.AssetState{},
			Bindings:                []bibledomain.ProductionBinding{},
		},
	}
	sequence := 100
	service := bibleapp.NewService(store, bibleapp.Config{
		Now: func() time.Time { return time.Date(2026, 8, 28, 6, 0, 0, 0, time.UTC) },
		NewID: func() string {
			sequence++
			return fmt.Sprintf("00000000-0000-0000-0000-%012d", sequence)
		},
	})
	actor := bibleapp.Actor{UserID: version.CreatedBy, TokenVersion: 1}
	command := bibleapp.MaterializeCommand{
		BibleVersionID: version.ID, ExpectedVersion: version.Version,
		ExpectedContentHash: version.ContentHash, IdempotencyKey: "materialize-confirmed-bible",
	}

	result, err := service.MaterializeConfirmedBible(context.Background(), actor, command)
	if err != nil {
		t.Fatalf("materialize confirmed Bible: %v", err)
	}
	if result.Receipt.Operation != "production_bible.materialize_confirmed" ||
		result.Receipt.ResourceID != version.ID || result.Materialization.ContentHash == "" ||
		len(result.Materialization.Assets) != 4 || len(result.Materialization.Specifications) != 4 ||
		len(result.Materialization.States) != 5 || len(result.Materialization.Bindings) != 4 {
		t.Fatalf("materialization result = %#v", result)
	}
	if !strings.Contains(string(result.Receipt.Result), `"identity_key"`) ||
		strings.Contains(string(result.Receipt.Result), `"IdentityKey"`) {
		t.Fatalf("materialization Receipt is not canonical snake_case JSON: %s", result.Receipt.Result)
	}
	assets := map[string]bibledomain.MaterializedAsset{}
	for _, asset := range result.Materialization.Assets {
		assets[asset.IdentityKey] = asset
	}
	if assets["character:twin-a"].ID == assets["character:twin-b"].ID {
		t.Fatal("same canonical name merged two exact confirmed entity keys")
	}
	baseStates := map[string]bibledomain.MaterializedState{}
	for _, state := range result.Materialization.States {
		if state.StateKey == "base" {
			baseStates[state.AssetID] = state
		}
	}
	for _, asset := range result.Materialization.Assets {
		if _, exists := baseStates[asset.ID]; !exists {
			t.Fatalf("asset %s has no deterministic base state", asset.IdentityKey)
		}
	}
	if store.createCalls != 1 || len(store.write.NewAssets) != 4 ||
		len(store.write.NewSpecifications) != 4 || len(store.write.NewStates) != 5 ||
		len(store.write.NewBindings) != 4 {
		t.Fatalf("materialization write = %#v calls=%d", store.write, store.createCalls)
	}
	firstWrite := store.write

	replayed, err := service.MaterializeConfirmedBible(context.Background(), actor, command)
	if err != nil || replayed.Receipt.ID != result.Receipt.ID ||
		replayed.Materialization.ContentHash != result.Materialization.ContentHash || store.createCalls != 1 {
		t.Fatalf("materialization replay = %#v calls=%d error=%v", replayed, store.createCalls, err)
	}
	store.scope.AssetsByEntityKey = map[string]assetdomain.Asset{}
	store.scope.SpecificationsByAssetID = map[string][]bibledomain.SpecificationVersion{}
	store.scope.StatesByAssetID = map[string][]assetdomain.AssetState{}
	for _, asset := range firstWrite.NewAssets {
		store.scope.AssetsByEntityKey[asset.IdentityKey] = asset
	}
	for _, specification := range firstWrite.NewSpecifications {
		store.scope.SpecificationsByAssetID[specification.AssetID] = append(
			store.scope.SpecificationsByAssetID[specification.AssetID], specification,
		)
	}
	for _, state := range firstWrite.NewStates {
		store.scope.StatesByAssetID[state.AssetID] = append(store.scope.StatesByAssetID[state.AssetID], state)
	}
	secondVersion := nextMaterializationBibleVersion(t, version)
	store.scope.Version, store.scope.Bindings = secondVersion, []bibledomain.ProductionBinding{}
	second, err := service.MaterializeConfirmedBible(context.Background(), actor, bibleapp.MaterializeCommand{
		BibleVersionID: secondVersion.ID, ExpectedVersion: secondVersion.Version,
		ExpectedContentHash: secondVersion.ContentHash, IdempotencyKey: "materialize-confirmed-bible-v2",
	})
	if err != nil || store.createCalls != 2 || len(store.write.NewAssets) != 0 ||
		len(store.write.NewSpecifications) != 0 || len(store.write.NewStates) != 0 ||
		len(store.write.NewBindings) != 4 || len(second.Materialization.Bindings) != 4 {
		t.Fatalf("exact Owner fact reuse = %#v write=%#v calls=%d error=%v", second, store.write, store.createCalls, err)
	}
	for index := range result.Materialization.Assets {
		if second.Materialization.Assets[index] != result.Materialization.Assets[index] ||
			second.Materialization.Specifications[index] != result.Materialization.Specifications[index] {
			t.Fatalf("exact immutable Owner fact changed across Bible versions: first=%#v second=%#v", result, second)
		}
	}
	drifted := command
	drifted.ExpectedContentHash = "f" + version.ContentHash[1:]
	_, err = service.MaterializeConfirmedBible(context.Background(), actor, drifted)
	var apiError *bibleapp.Error
	if !errors.As(err, &apiError) || apiError.Code != "resource_conflict" {
		t.Fatalf("drifted materialization error = %#v", err)
	}
}

func nextMaterializationBibleVersion(
	t *testing.T,
	current bibledomain.ProductionBibleVersion,
) bibledomain.ProductionBibleVersion {
	t.Helper()
	value, err := bibledomain.NewProductionBibleVersion(bibledomain.ProductionBibleVersionInput{
		ID:          "00000000-0000-0000-0000-000000000008",
		WorkspaceID: current.WorkspaceID, ProjectID: current.ProjectID,
		DocumentRevisionID: current.DocumentRevisionID, DocumentRevisionHash: current.DocumentRevisionHash,
		CandidateRevisionID: current.CandidateRevisionID, CandidateRevisionNo: current.CandidateRevisionNo,
		CandidateRevisionHash: current.CandidateRevisionHash, CandidateContentHash: current.CandidateContentHash,
		Version: current.Version + 1, ReviewDecisionID: "00000000-0000-0000-0000-000000000009",
		Snapshot: current.Snapshot, CreatedBy: current.CreatedBy, CreatedAt: current.CreatedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("build next Production Bible Version: %v", err)
	}
	return value
}

func materializationBibleVersion(t *testing.T) bibledomain.ProductionBibleVersion {
	t.Helper()
	episode := 1
	evidence := bibledomain.Evidence{
		SourceStart: 0, SourceEnd: 2, ExactAnchor: "双生",
		TextHash: bibledomain.SourceTextHash("双生"), EpisodeNumber: &episode,
	}
	spec := func(appearance string) bibledomain.AssetSpecCandidate {
		return bibledomain.AssetSpecCandidate{
			Appearance: &appearance, Temperament: []string{}, Goals: []string{}, Relationships: []string{},
			VisualElements: []string{}, NegativeConstraints: []string{},
			PerformanceTraits: []string{}, AllowedUsage: []string{},
		}
	}
	candidate := bibledomain.StoryReconciliationCandidate{
		CanonicalEntities: []bibledomain.StoryEntityCandidate{
			{
				EntityKey: "character:twin-a", Kind: "character", CanonicalName: "双生", NormalizedName: "双生",
				Aliases: []string{}, StableSpec: spec("白衣"), EpisodeNumbers: []int{1}, Evidence: []bibledomain.Evidence{evidence},
				States: []bibledomain.StoryEntityStateCandidate{}, Ambiguities: []string{},
			},
			{
				EntityKey: "character:twin-b", Kind: "character", CanonicalName: "双生", NormalizedName: "双生",
				Aliases: []string{}, StableSpec: spec("黑衣"), EpisodeNumbers: []int{1}, Evidence: []bibledomain.Evidence{evidence},
				States: []bibledomain.StoryEntityStateCandidate{{
					StateKey: "base", Label: "基础状态", StateSpec: spec("黑衣"), EpisodeNumbers: []int{1},
					Evidence: []bibledomain.Evidence{evidence}, Ambiguities: []string{},
				}}, Ambiguities: []string{},
			},
			{
				EntityKey: "location:tower", Kind: "location", CanonicalName: "高塔", NormalizedName: "高塔",
				Aliases: []string{}, StableSpec: spec("石塔"), EpisodeNumbers: []int{1}, Evidence: []bibledomain.Evidence{evidence},
				States: []bibledomain.StoryEntityStateCandidate{{
					StateKey: "night", Label: "夜间", StateSpec: spec("月光石塔"), EpisodeNumbers: []int{1},
					Evidence: []bibledomain.Evidence{evidence}, Ambiguities: []string{},
				}}, Ambiguities: []string{},
			},
			{
				EntityKey: "prop:mirror", Kind: "prop", CanonicalName: "古镜", NormalizedName: "古镜",
				Aliases: []string{}, StableSpec: spec("青铜古镜"), EpisodeNumbers: []int{1}, Evidence: []bibledomain.Evidence{evidence},
				States: []bibledomain.StoryEntityStateCandidate{}, Ambiguities: []string{},
			},
		},
		CanonicalWorldEntries: []bibledomain.StoryWorldEntryCandidate{},
		MergedClaims:          []bibledomain.StoryClaimCandidate{},
		MergedArcs:            []bibledomain.StoryArcCandidate{},
		Conflicts:             []bibledomain.ReviewIssue{},
		ReviewIssues:          []bibledomain.ReviewIssue{},
	}
	snapshot, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("encode materialization candidate: %v", err)
	}
	candidateHash, err := agentcontract.CanonicalHash(snapshot)
	if err != nil {
		t.Fatalf("hash materialization candidate: %v", err)
	}
	version, err := bibledomain.NewProductionBibleVersion(bibledomain.ProductionBibleVersionInput{
		ID: "00000000-0000-0000-0000-000000000001", WorkspaceID: "00000000-0000-0000-0000-000000000002",
		ProjectID: "00000000-0000-0000-0000-000000000003", DocumentRevisionID: "00000000-0000-0000-0000-000000000004",
		DocumentRevisionHash: "1" + fmt.Sprintf("%063d", 0), CandidateRevisionID: "00000000-0000-0000-0000-000000000005",
		CandidateRevisionHash: "2" + fmt.Sprintf("%063d", 0), CandidateContentHash: candidateHash,
		CandidateRevisionNo: 1, Version: 1, ReviewDecisionID: "00000000-0000-0000-0000-000000000006",
		Snapshot: snapshot, CreatedBy: "00000000-0000-0000-0000-000000000007",
		CreatedAt: time.Date(2026, 8, 28, 5, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("build materialization Bible Version: %v", err)
	}
	return version
}
