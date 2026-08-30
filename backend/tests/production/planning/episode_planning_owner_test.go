package planning_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

type episodePlanningOwnerStore struct {
	source       planningapp.EpisodePlanningCandidateSource
	receipts     []platformcommand.Receipt
	structures   []domain.Structure
	saveAttempts int
	failSave     bool
}

func (store *episodePlanningOwnerStore) WithinEpisodePlanningTransaction(
	_ context.Context,
	operation func(planningapp.EpisodePlanningRepository) error,
) error {
	receipts := append([]platformcommand.Receipt(nil), store.receipts...)
	structures := append([]domain.Structure(nil), store.structures...)
	attempts := store.saveAttempts
	if err := operation(store); err != nil {
		store.receipts, store.structures, store.saveAttempts = receipts, structures, attempts
		return err
	}
	return nil
}

func (store *episodePlanningOwnerStore) GetEpisodePlanningCandidate(
	context.Context,
	planningapp.Actor,
	string,
	bool,
) (planningapp.EpisodePlanningCandidateSource, error) {
	return store.source, nil
}

func (store *episodePlanningOwnerStore) FindReceipt(
	_ context.Context,
	_ string,
	operation string,
	key string,
) (platformcommand.Receipt, error) {
	for _, receipt := range store.receipts {
		if receipt.Operation == operation && receipt.IdempotencyKey == key {
			return receipt, nil
		}
	}
	return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
}

func (store *episodePlanningOwnerStore) CreateReceipt(
	_ context.Context,
	receipt platformcommand.Receipt,
) error {
	store.receipts = append(store.receipts, receipt)
	return nil
}

func (store *episodePlanningOwnerStore) GetPlanningOwnerSet(
	context.Context,
	planningapp.Actor,
	planningapp.PlanningOwnerSetReference,
) ([]domain.Structure, error) {
	return append([]domain.Structure(nil), store.structures...), nil
}

func (store *episodePlanningOwnerStore) CreatePlanningOwnerSet(
	_ context.Context,
	structures []domain.Structure,
) error {
	store.saveAttempts++
	store.structures = append(store.structures, structures...)
	if store.failSave {
		return errors.New("injected Planning owner batch failure")
	}
	return nil
}

func TestApplyEpisodePlanningCandidateCreatesEveryFormalFactAndReplaysReceipt(t *testing.T) {
	now := time.Date(2026, time.August, 28, 15, 0, 0, 0, time.UTC)
	store := &episodePlanningOwnerStore{source: episodePlanningOwnerFixture()}
	service := planningapp.NewEpisodePlanningService(store, planningapp.Config{
		Now: func() time.Time { return now }, NewID: sequenceIDs(),
	})
	command := planningapp.ApplyEpisodePlanningCandidateCommand{
		WorkspaceID: store.source.WorkspaceID, ProjectID: store.source.ProjectID,
		CandidateRevisionID:       store.source.CandidateRevisionID,
		CandidateRevisionHash:     store.source.CandidateRevisionHash,
		ExpectedCandidateRevision: store.source.CandidateRevision,
		ReviewDecisionID:          "76000000-0000-0000-0000-000000000091",
		IdempotencyKey:            "workflow-review:76000000-0000-0000-0000-000000000091",
	}
	actor := planningapp.Actor{UserID: "76000000-0000-0000-0000-000000000092", TokenVersion: 1}

	result, err := service.ApplyEpisodePlanningCandidate(context.Background(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Set.ID != result.Receipt.ID || result.Receipt.Operation != "episode_planning.apply" ||
		result.Receipt.ResourceID != command.CandidateRevisionID || result.Set.ReviewDecisionID != command.ReviewDecisionID ||
		result.Set.ContentHash == "" || len(result.Set.Structures) != 2 || len(store.structures) != 2 ||
		store.saveAttempts != 1 {
		t.Fatalf("Planning owner set is incomplete: result=%#v structures=%#v", result, store.structures)
	}
	first := store.structures[0]
	if first.Status != "confirmed" || first.ConfirmedBy == nil || *first.ConfirmedBy != actor.UserID ||
		len(first.Scenes) != 1 || len(first.Scenes[0].Dialogues) != 1 ||
		len(first.Scenes[0].NarrativeUnits) != 2 || len(first.Scenes[0].Occurrences) != 2 ||
		len(first.Scenes[0].Claims) != 1 {
		t.Fatalf("formal Episode structure is incomplete: %#v", first)
	}
	scene := first.Scenes[0]
	if scene.Dialogues[0].SpeakerIdentity == nil || scene.Dialogues[0].SpeakerIdentity.AssetID != "76000000-0000-0000-0000-000000000041" ||
		scene.Occurrences[0].State.StateKey != "base" || scene.Occurrences[1].Identity.Kind != "location" ||
		len(scene.Claims[0].Participants) != 1 || scene.Claims[0].Participants[0].Role != "subject" ||
		len(scene.Claims[0].Anchors) != 1 || scene.Claims[0].Anchors[0].FragmentID != scene.NarrativeUnits[0].ID {
		t.Fatalf("formal identity/state/claim refs are incomplete: %#v", scene)
	}
	if len(result.Set.Structures[0].Fragments) != 7 {
		t.Fatalf("temporary-to-owner mapping is incomplete: %#v", result.Set.Structures[0].Fragments)
	}

	replayed, err := service.ApplyEpisodePlanningCandidate(context.Background(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Receipt.ID != result.Receipt.ID || replayed.Set.ContentHash != result.Set.ContentHash ||
		store.saveAttempts != 1 || len(store.structures) != 2 {
		t.Fatalf("Planning owner replay created a second effect: first=%#v replay=%#v", result, replayed)
	}
}

func TestApplyEpisodePlanningCandidateRejectsStateFromAnotherIdentityWithoutWrites(t *testing.T) {
	store := &episodePlanningOwnerStore{source: episodePlanningOwnerFixture()}
	store.source.Candidate.Episodes[0].Candidate.OrderedFragments[3].Attributes.StateKey = stringPointer("night")
	service := planningapp.NewEpisodePlanningService(store, planningapp.Config{Now: time.Now, NewID: sequenceIDs()})

	_, err := service.ApplyEpisodePlanningCandidate(context.Background(), planningapp.Actor{
		UserID: "76000000-0000-0000-0000-000000000092", TokenVersion: 1,
	}, planningapp.ApplyEpisodePlanningCandidateCommand{
		WorkspaceID: store.source.WorkspaceID, ProjectID: store.source.ProjectID,
		CandidateRevisionID:       store.source.CandidateRevisionID,
		CandidateRevisionHash:     store.source.CandidateRevisionHash,
		ExpectedCandidateRevision: store.source.CandidateRevision,
		ReviewDecisionID:          "76000000-0000-0000-0000-000000000093",
		IdempotencyKey:            "workflow-review:76000000-0000-0000-0000-000000000093",
	})
	if err == nil {
		t.Fatal("Planning owner accepted a state belonging to another identity")
	}
	if len(store.structures) != 0 || len(store.receipts) != 0 || store.saveAttempts != 0 {
		t.Fatalf("invalid identity/state produced owner writes: %#v", store)
	}
}

func TestApplyEpisodePlanningCandidateRollsBackTheWholeBatch(t *testing.T) {
	store := &episodePlanningOwnerStore{source: episodePlanningOwnerFixture(), failSave: true}
	service := planningapp.NewEpisodePlanningService(store, planningapp.Config{Now: time.Now, NewID: sequenceIDs()})
	_, err := service.ApplyEpisodePlanningCandidate(context.Background(), planningapp.Actor{
		UserID: "76000000-0000-0000-0000-000000000092", TokenVersion: 1,
	}, planningapp.ApplyEpisodePlanningCandidateCommand{
		WorkspaceID: store.source.WorkspaceID, ProjectID: store.source.ProjectID,
		CandidateRevisionID:       store.source.CandidateRevisionID,
		CandidateRevisionHash:     store.source.CandidateRevisionHash,
		ExpectedCandidateRevision: store.source.CandidateRevision,
		ReviewDecisionID:          "76000000-0000-0000-0000-000000000094",
		IdempotencyKey:            "workflow-review:76000000-0000-0000-0000-000000000094",
	})
	if err == nil {
		t.Fatal("injected Planning owner failure was ignored")
	}
	if len(store.structures) != 0 || len(store.receipts) != 0 || store.saveAttempts != 0 {
		t.Fatalf("failed Planning batch was not rolled back: %#v", store)
	}
}

func episodePlanningOwnerFixture() planningapp.EpisodePlanningCandidateSource {
	firstText := "内景 客厅 日\n林一：门开了。\n林一走向门口。"
	secondText := "外景 河岸 夜\n林一停下。"
	firstEnd := len([]rune(firstText))
	secondEnd := firstEnd + len([]rune(secondText))
	firstEpisode, secondEpisode := 1, 2
	firstEvidence := bibledomain.Evidence{
		SourceStart: 0, SourceEnd: firstEnd, TextHash: bibledomain.SourceTextHash(firstText),
		ExactAnchor: firstText, EpisodeNumber: &firstEpisode,
	}
	secondEvidence := bibledomain.Evidence{
		SourceStart: firstEnd, SourceEnd: secondEnd, TextHash: bibledomain.SourceTextHash(secondText),
		ExactAnchor: secondText, EpisodeNumber: &secondEpisode,
	}
	character := planningapp.PlanningIdentitySource{
		EntityKey: "character:lin-yi",
		Asset: bibledomain.MaterializedAsset{
			ID: "76000000-0000-0000-0000-000000000041", IdentityKey: "character:lin-yi",
			Kind: "character", Revision: 1, ContentHash: strings.Repeat("4", 64),
		},
		Specification: bibledomain.MaterializedSpecification{
			ID: "76000000-0000-0000-0000-000000000042", AssetID: "76000000-0000-0000-0000-000000000041",
			Kind: "character", EntityKey: "character:lin-yi", Version: 1, ContentHash: strings.Repeat("5", 64),
		},
		States: []bibledomain.MaterializedState{{
			ID: "76000000-0000-0000-0000-000000000043", AssetID: "76000000-0000-0000-0000-000000000041",
			StateKey: "base", Revision: 1, ContentHash: strings.Repeat("6", 64),
		}},
	}
	location := planningapp.PlanningIdentitySource{
		EntityKey: "location:living-room",
		Asset: bibledomain.MaterializedAsset{
			ID: "76000000-0000-0000-0000-000000000051", IdentityKey: "location:living-room",
			Kind: "location", Revision: 1, ContentHash: strings.Repeat("7", 64),
		},
		Specification: bibledomain.MaterializedSpecification{
			ID: "76000000-0000-0000-0000-000000000052", AssetID: "76000000-0000-0000-0000-000000000051",
			Kind: "location", EntityKey: "location:living-room", Version: 1, ContentHash: strings.Repeat("8", 64),
		},
		States: []bibledomain.MaterializedState{{
			ID: "76000000-0000-0000-0000-000000000053", AssetID: "76000000-0000-0000-0000-000000000051",
			StateKey: "night", Revision: 1, ContentHash: strings.Repeat("9", 64),
		}},
	}
	firstCandidate := domain.EpisodeReconciliationCandidate{
		EpisodeID:       "76000000-0000-0000-0000-000000000011",
		ScriptVersionID: "76000000-0000-0000-0000-000000000012", SourceStart: 0, SourceEnd: firstEnd,
		OrderedFragments: []domain.EpisodeStructureFragment{
			planningFragment("beat:first", "beat", stringPointer("scene:first"), 0, firstEnd, firstEvidence, domain.EpisodeStructureAttributes{
				ParticipantKeys: []string{"character:lin-yi"}, Action: stringPointer("林一走向门口"), ContinuityNotes: []string{},
			}),
			planningFragment("beat:second", "beat", stringPointer("scene:first"), 0, firstEnd, firstEvidence, domain.EpisodeStructureAttributes{
				ParticipantKeys: []string{"character:lin-yi"}, Action: stringPointer("林一停在门边"), ContinuityNotes: []string{},
			}),
			planningFragment("dialogue:first", "dialogue", stringPointer("scene:first"), 0, firstEnd, firstEvidence, domain.EpisodeStructureAttributes{
				SpeakerKey: stringPointer("character:lin-yi"), ParticipantKeys: []string{"character:lin-yi"},
				DialogueText: stringPointer("门开了。"), ContinuityNotes: []string{},
			}),
			planningFragment("occurrence:character", "occurrence", stringPointer("scene:first"), 0, firstEnd, firstEvidence, domain.EpisodeStructureAttributes{
				ParticipantKeys: []string{}, OccurrenceEntityKey: stringPointer("character:lin-yi"),
				StateKey: stringPointer("base"), ContinuityNotes: []string{},
			}),
			planningFragment("occurrence:location", "occurrence", stringPointer("scene:first"), 0, firstEnd, firstEvidence, domain.EpisodeStructureAttributes{
				ParticipantKeys: []string{}, OccurrenceEntityKey: stringPointer("location:living-room"),
				StateKey: stringPointer("night"), ContinuityNotes: []string{},
			}),
			planningFragment("scene:first", "scene", nil, 0, firstEnd, firstEvidence, domain.EpisodeStructureAttributes{
				ParticipantKeys: []string{"character:lin-yi"}, LocationKey: stringPointer("location:living-room"),
				TimeHint: stringPointer("日"), ContinuityNotes: []string{},
			}),
		},
		Claims: []domain.EpisodeClaimCandidate{{
			ClaimKey: "claim:causal-door", ClaimType: "causal", ParticipantKeys: []string{"character:lin-yi"},
			AnchorKeys: []string{"beat:first"}, Scope: "episode:76000000-0000-0000-0000-000000000011",
			Polarity: "positive", Status: "proposed", Evidence: []bibledomain.Evidence{firstEvidence},
		}},
		Conflicts: []bibledomain.ReviewIssue{}, ReviewIssues: []bibledomain.ReviewIssue{},
	}
	secondCandidate := domain.EpisodeReconciliationCandidate{
		EpisodeID:       "76000000-0000-0000-0000-000000000021",
		ScriptVersionID: "76000000-0000-0000-0000-000000000022", SourceStart: firstEnd, SourceEnd: secondEnd,
		OrderedFragments: []domain.EpisodeStructureFragment{
			planningFragment("occurrence:second", "occurrence", stringPointer("scene:second"), firstEnd, secondEnd, secondEvidence, domain.EpisodeStructureAttributes{
				ParticipantKeys: []string{}, OccurrenceEntityKey: stringPointer("character:lin-yi"),
				StateKey: stringPointer("base"), ContinuityNotes: []string{},
			}),
			planningFragment("scene:second", "scene", nil, firstEnd, secondEnd, secondEvidence, domain.EpisodeStructureAttributes{
				ParticipantKeys: []string{"character:lin-yi"}, ContinuityNotes: []string{},
			}),
		},
		Claims: []domain.EpisodeClaimCandidate{}, Conflicts: []bibledomain.ReviewIssue{}, ReviewIssues: []bibledomain.ReviewIssue{},
	}
	return planningapp.EpisodePlanningCandidateSource{
		CandidateRevisionID:   "76000000-0000-0000-0000-000000000001",
		CandidateRevisionHash: strings.Repeat("a", 64), CandidateRevision: 1,
		WorkspaceID:    "76000000-0000-0000-0000-000000000002",
		ProjectID:      "76000000-0000-0000-0000-000000000003",
		BibleVersionID: "76000000-0000-0000-0000-000000000004", BibleVersion: 1,
		BibleContentHash: strings.Repeat("b", 64), MaterializationHash: strings.Repeat("c", 64),
		Candidate: domain.EpisodePlanningCandidateSet{
			SchemaVersion:  "episode-planning-candidate-set",
			BibleVersionID: "76000000-0000-0000-0000-000000000004", BibleVersion: 1,
			BibleContentHash: strings.Repeat("b", 64), MaterializationHash: strings.Repeat("c", 64),
			Episodes: []domain.EpisodePlanningCandidateRoot{
				{EpisodeID: firstCandidate.EpisodeID, EpisodePosition: 1, ScriptVersionID: firstCandidate.ScriptVersionID,
					ShardKey: "episode:0001", StageInstanceKey: strings.Repeat("d", 64),
					CandidateRevisionID:   "76000000-0000-0000-0000-000000000013",
					CandidateRevisionHash: strings.Repeat("e", 64), Candidate: firstCandidate},
				{EpisodeID: secondCandidate.EpisodeID, EpisodePosition: 2, ScriptVersionID: secondCandidate.ScriptVersionID,
					ShardKey: "episode:0002", StageInstanceKey: strings.Repeat("f", 64),
					CandidateRevisionID:   "76000000-0000-0000-0000-000000000023",
					CandidateRevisionHash: strings.Repeat("1", 64), Candidate: secondCandidate},
			},
		},
		Episodes: []planningapp.PlanningEpisodeSource{
			{EpisodeID: firstCandidate.EpisodeID, EpisodePosition: 1, ScriptVersionID: firstCandidate.ScriptVersionID,
				ScriptVersion: 1, DocumentRevisionID: "76000000-0000-0000-0000-000000000031",
				SourceStart: 0, SourceEnd: firstEnd, Content: firstText, ContentHash: bibledomain.SourceTextHash(firstText)},
			{EpisodeID: secondCandidate.EpisodeID, EpisodePosition: 2, ScriptVersionID: secondCandidate.ScriptVersionID,
				ScriptVersion: 1, DocumentRevisionID: "76000000-0000-0000-0000-000000000031",
				SourceStart: firstEnd, SourceEnd: secondEnd, Content: secondText, ContentHash: bibledomain.SourceTextHash(secondText)},
		},
		Identities: []planningapp.PlanningIdentitySource{character, location},
	}
}

func planningFragment(
	key string,
	kind string,
	sceneKey *string,
	start int,
	end int,
	evidence bibledomain.Evidence,
	attributes domain.EpisodeStructureAttributes,
) domain.EpisodeStructureFragment {
	attributes.SceneKey = sceneKey
	return domain.EpisodeStructureFragment{
		TemporaryKey: key, Kind: kind, SourceKeys: []string{"source:" + key},
		SourceStart: start, SourceEnd: end, Summary: key, Evidence: []bibledomain.Evidence{evidence}, Attributes: attributes,
	}
}

func stringPointer(value string) *string { return &value }
