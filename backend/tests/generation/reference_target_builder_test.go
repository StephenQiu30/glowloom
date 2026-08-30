package generation_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

func TestReferenceTargetBuilderFreezesApprovedCharacterInputsAndReplays(t *testing.T) {
	now := time.Date(2026, time.August, 29, 2, 30, 0, 0, time.UTC)
	actor := generationapp.Actor{UserID: uuid.NewString(), TokenVersion: 1}
	source := referenceTargetSource(actor.UserID)
	store := &referenceTargetStore{source: source, targets: map[string]generationdomain.GenerationTarget{}}
	ids := []string{uuid.NewString(), uuid.NewString()}
	service := generationapp.NewReferenceTargetBuilderService(store, generationapp.ReferenceTargetBuilderConfig{
		Now: func() time.Time { return now },
		NewID: func() string {
			id := ids[0]
			ids = ids[1:]
			return id
		},
	})
	command := generationapp.BuildReferenceTargetsCommand{
		ApprovedIntentSetID: source.ApprovedIntentSetRef.ID,
		ExpectedContentHash: source.ApprovedIntentSetRef.ContentHash,
		IdempotencyKey:      "reference-targets:" + source.ApprovedIntentSetRef.ID,
	}

	result, err := service.BuildReferenceTargets(context.Background(), actor, command)
	if err != nil {
		t.Fatalf("build approved reference targets: %v", err)
	}
	if len(result.Targets) != 1 || len(store.targets) != 1 || result.Receipt.Operation != "generation.reference_targets.build" {
		t.Fatalf("reference target result = %#v, persisted=%d", result, len(store.targets))
	}
	target := result.Targets[0]
	if target.SourceOwnerRef != source.ApprovedIntentSetRef || target.PolicySnapshotRef != source.EffectiveStyleSnapshotRef ||
		target.Kind != generationdomain.GenerationTargetReferenceAsset || target.CreatedBy != actor.UserID ||
		target.ReferenceAsset == nil || target.ReferenceAsset.AssetID != source.Requirements[0].AssetID ||
		target.ReferenceAsset.OutputKind != "reference_sheet" || target.ReferenceAsset.PromptVersion != "character-reference-sheet" ||
		target.ReferenceAsset.Width != 1536 || target.ReferenceAsset.Height != 1024 ||
		target.ReferenceAsset.NumberResults != 4 || target.ReferenceAsset.OutputFormat != "PNG" ||
		!strings.Contains(target.ReferenceAsset.PositivePrompt, `"hair":"black"`) ||
		!strings.Contains(target.ReferenceAsset.PositivePrompt, `"outfit":"red coat"`) ||
		!strings.Contains(target.ReferenceAsset.PositivePrompt, source.VisualStyle) ||
		generationdomain.ValidateGenerationTarget(target) != nil {
		t.Fatalf("reference target did not freeze exact source facts: %#v", target)
	}
	if len(target.ReferenceAsset.RequiredViewRoles) != 3 || target.ReferenceAsset.RequiredViewRoles[0] != "front" ||
		target.ReferenceAsset.RequiredViewRoles[1] != "profile" || target.ReferenceAsset.RequiredViewRoles[2] != "back" {
		t.Fatalf("reference target view roles = %v", target.ReferenceAsset.RequiredViewRoles)
	}

	replayed, err := service.BuildReferenceTargets(context.Background(), actor, command)
	if err != nil || len(replayed.Targets) != 1 || replayed.Targets[0].ID != target.ID ||
		replayed.Targets[0].TargetHash != target.TargetHash || replayed.Receipt.ID != result.Receipt.ID || len(ids) != 0 {
		t.Fatalf("replay reference targets: result=%#v remaining_ids=%d err=%v", replayed, len(ids), err)
	}
}

func TestReferenceTargetBuilderFailsClosedBeforeWriting(t *testing.T) {
	now := time.Date(2026, time.August, 29, 2, 40, 0, 0, time.UTC)
	actor := generationapp.Actor{UserID: uuid.NewString(), TokenVersion: 1}
	for name, mutate := range map[string]func(*generationapp.ReferenceTargetSource, *generationapp.BuildReferenceTargetsCommand){
		"approved hash drift": func(source *generationapp.ReferenceTargetSource, command *generationapp.BuildReferenceTargetsCommand) {
			command.ExpectedContentHash = strings.Repeat("0", 64)
		},
		"unsupported missing location": func(source *generationapp.ReferenceTargetSource, _ *generationapp.BuildReferenceTargetsCommand) {
			for index := range source.Requirements {
				source.Requirements[index].AssetKind = "location"
				source.Requirements[index].RequiredViewRoles = []string{"environment"}
			}
		},
		"mixed immutable facts": func(source *generationapp.ReferenceTargetSource, _ *generationapp.BuildReferenceTargetsCommand) {
			drifted := source.Requirements[0]
			drifted.SpecificationVersionRef.ID = uuid.NewString()
			source.Requirements = append(source.Requirements, drifted)
		},
	} {
		t.Run(name, func(t *testing.T) {
			source := referenceTargetSource(actor.UserID)
			store := &referenceTargetStore{source: source, targets: map[string]generationdomain.GenerationTarget{}}
			command := generationapp.BuildReferenceTargetsCommand{
				ApprovedIntentSetID: source.ApprovedIntentSetRef.ID,
				ExpectedContentHash: source.ApprovedIntentSetRef.ContentHash,
				IdempotencyKey:      "reference-targets:" + source.ApprovedIntentSetRef.ID,
			}
			mutate(&store.source, &command)
			service := generationapp.NewReferenceTargetBuilderService(store, generationapp.ReferenceTargetBuilderConfig{
				Now: nowUTC(now), NewID: uuid.NewString,
			})
			if _, err := service.BuildReferenceTargets(context.Background(), actor, command); generationErrorCode(err) != "state_conflict" {
				t.Fatalf("drift was not rejected: %T %v", err, err)
			}
			if len(store.targets) != 0 || store.receipt.ID != "" {
				t.Fatalf("failed build wrote facts: targets=%d receipt=%#v", len(store.targets), store.receipt)
			}
		})
	}
}

type referenceTargetStore struct {
	source  generationapp.ReferenceTargetSource
	targets map[string]generationdomain.GenerationTarget
	receipt platformcommand.Receipt
}

func (store *referenceTargetStore) WithinReferenceTargetTransaction(
	_ context.Context,
	operation func(generationapp.ReferenceTargetBuilderRepository) error,
) error {
	beforeTargets := make(map[string]generationdomain.GenerationTarget, len(store.targets))
	for id, target := range store.targets {
		beforeTargets[id] = target
	}
	beforeReceipt := store.receipt
	if err := operation(store); err != nil {
		store.targets, store.receipt = beforeTargets, beforeReceipt
		return err
	}
	return nil
}

func (store *referenceTargetStore) LoadReferenceTargetSource(
	_ context.Context,
	_ generationapp.Actor,
	approvedIntentSetID string,
) (generationapp.ReferenceTargetSource, error) {
	if approvedIntentSetID != store.source.ApprovedIntentSetRef.ID {
		return generationapp.ReferenceTargetSource{}, generationapp.ErrGenerationTargetNotFound
	}
	return store.source, nil
}

func (store *referenceTargetStore) FindReferenceTargetBuildReceipt(
	_ context.Context,
	workspaceID, key string,
) (platformcommand.Receipt, error) {
	if store.receipt.ID == "" || store.receipt.WorkspaceID != workspaceID || store.receipt.IdempotencyKey != key {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	return store.receipt, nil
}

func (store *referenceTargetStore) EnsureReferenceTargetBuildReceipt(
	_ context.Context,
	receipt platformcommand.Receipt,
) (platformcommand.Receipt, error) {
	if store.receipt.ID == "" {
		store.receipt = receipt
	}
	if store.receipt.InputHash != receipt.InputHash {
		return platformcommand.Receipt{}, platformcommand.ErrInputMismatch
	}
	return store.receipt, nil
}

func (store *referenceTargetStore) EnsureGenerationTarget(
	_ context.Context,
	target generationdomain.GenerationTarget,
) (generationdomain.GenerationTarget, error) {
	if persisted, found := store.targets[target.ID]; found {
		if !generationdomain.SameGenerationTarget(persisted, target) {
			return generationdomain.GenerationTarget{}, errors.New("target identity drift")
		}
		return persisted, nil
	}
	store.targets[target.ID] = target
	return target, nil
}

func (store *referenceTargetStore) FindGenerationTarget(
	_ context.Context,
	targetID string,
) (generationdomain.GenerationTarget, error) {
	target, found := store.targets[targetID]
	if !found {
		return generationdomain.GenerationTarget{}, generationapp.ErrGenerationTargetNotFound
	}
	return target, nil
}

func referenceTargetSource(createdBy string) generationapp.ReferenceTargetSource {
	approved := generationdomain.FrozenOwnerReference{
		Owner: "storyboard", Resource: "approved_storyboard_intents", ID: uuid.NewString(),
		Revision: 1, ContentHash: strings.Repeat("a", 64),
	}
	style := generationdomain.FrozenOwnerReference{
		Owner: "preset", Resource: "effective_style_snapshot", ID: uuid.NewString(),
		Revision: 3, ContentHash: strings.Repeat("b", 64),
	}
	requirement := generationapp.ReferenceTargetRequirement{
		AssetID: uuid.NewString(), AssetKind: "character", RequiredViewRoles: []string{"front", "profile", "back"},
		SpecificationVersionRef: generationdomain.FrozenOwnerReference{
			Owner: "production", Resource: "production_bible_specification_version", ID: uuid.NewString(),
			Revision: 2, ContentHash: strings.Repeat("c", 64),
		},
		SpecificationSnapshot: json.RawMessage(`{"hair":"black","identity":"Lin"}`),
		AssetStateRef: generationdomain.FrozenOwnerReference{
			Owner: "asset", Resource: "asset_state", ID: uuid.NewString(),
			Revision: 1, ContentHash: strings.Repeat("d", 64),
		},
		AssetStateSnapshot: json.RawMessage(`{"outfit":"red coat","timeline":"episode-1"}`),
	}
	return generationapp.ReferenceTargetSource{
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), ApprovedIntentSetRef: approved,
		EffectiveStyleSnapshotRef: style, VisualStyle: "cinematic ink animation", AspectRatio: "9:16",
		Requirements: []generationapp.ReferenceTargetRequirement{requirement, requirement}, CreatedBy: createdBy,
	}
}

func nowUTC(value time.Time) func() time.Time { return func() time.Time { return value } }
