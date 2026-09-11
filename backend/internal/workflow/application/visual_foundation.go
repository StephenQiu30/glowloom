package application

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

var visualFoundationHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type FaithfulVisualFoundationInputCommand struct {
	World         storygraphdomain.VisualFoundationWorldReadSet
	Source        worlddomain.ConfirmedVisualFoundationSource
	Selection     presetdomain.ProjectSelection
	PresetRelease presetdomain.Release
}

// CompileFaithfulVisualFoundationInput builds the first safe Visual
// Foundation execution path. It deliberately accepts no overrides, external
// references, inferred world adaptations, or unresolved Design Gaps.
func CompileFaithfulVisualFoundationInput(
	command FaithfulVisualFoundationInputCommand,
) (agentcontract.VisualFoundationInput, json.RawMessage, error) {
	if err := validateFaithfulVisualFoundationSources(command); err != nil {
		return agentcontract.VisualFoundationInput{}, nil, err
	}
	roots := make([]agentcontract.ConfirmedWorldRoot, len(command.World.ConfirmedWorldRoots))
	for index, root := range command.World.ConfirmedWorldRoots {
		roots[index] = agentcontract.ConfirmedWorldRoot{
			OwnerFamily: root.OwnerFamily, ScopeKey: root.ScopeKey,
			CollectionRootHash: root.CollectionRootHash,
		}
	}
	rules := make([]agentcontract.PresetAdaptationRuleSnapshot, len(command.PresetRelease.WorldAdaptationRules))
	for index, rule := range command.PresetRelease.WorldAdaptationRules {
		rules[index] = agentcontract.PresetAdaptationRuleSnapshot{
			RuleKey: rule.RuleKey, SourceFactKind: rule.SourceFactKind,
			DesignDomain: rule.DesignDomain, Directive: rule.Directive,
			PreservedInvariantKeys: append([]string(nil), rule.PreservedInvariantKeys...),
			ImpactScopeKinds:       append([]string(nil), rule.ImpactScopeKinds...),
		}
	}
	overrides := []agentcontract.TypedVisualOverride{}
	attachments := []agentcontract.VisualReferenceAttachment{}
	overridesHash, err := visualFoundationInputSetHash(overrides)
	if err != nil {
		return agentcontract.VisualFoundationInput{}, nil, err
	}
	attachmentsHash, err := visualFoundationInputSetHash(attachments)
	if err != nil {
		return agentcontract.VisualFoundationInput{}, nil, err
	}
	input := agentcontract.VisualFoundationInput{
		WorkspaceID: command.World.WorkspaceID, ProjectID: command.World.ProjectID,
		ProductionWorldOwnerSetHash: command.World.ProductionWorldOwnerSetHash,
		ConfirmedWorldRoots:         roots,
		PresetRelease: agentcontract.VisualFoundationPresetReleaseIdentity{
			Key: command.PresetRelease.Key, Release: command.PresetRelease.Release,
			ContentHash: command.PresetRelease.ContentHash,
		},
		ApplicationMode:    "faithful",
		FidelityInvariants: append([]string(nil), command.PresetRelease.FidelityInvariants...),
		AdaptationRules:    rules,
		VisualGrammar: agentcontract.VisualGrammarSnapshot{
			Palette:           command.PresetRelease.VisualGrammar.Palette,
			MaterialRendering: command.PresetRelease.VisualGrammar.MaterialRendering,
			Lighting:          command.PresetRelease.VisualGrammar.Lighting,
			Camera:            command.PresetRelease.VisualGrammar.Camera,
			NegativeConstraints: append(
				[]string(nil), command.PresetRelease.VisualGrammar.NegativeConstraints...,
			),
		},
		TypedOverrides: overrides, TypedOverridesHash: overridesHash,
		ReferenceAttachments: attachments, ReferenceAttachmentsHash: attachmentsHash,
		ConfirmedWorldFacts: []agentcontract.ConfirmedWorldFact{},
		DesignGaps:          []agentcontract.VisualDesignGap{},
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return agentcontract.VisualFoundationInput{}, nil, err
	}
	decoded, canonical, err := agentcontract.DecodeVisualFoundationInput(encoded)
	if err != nil {
		return agentcontract.VisualFoundationInput{}, nil, err
	}
	return decoded, canonical, nil
}

func validateFaithfulVisualFoundationSources(command FaithfulVisualFoundationInputCommand) error {
	world := command.World
	for _, identifier := range []string{world.WorkspaceID, world.ProjectID, world.StoryGraphVersionID} {
		parsed, err := uuid.Parse(identifier)
		if err != nil || parsed == uuid.Nil {
			return errors.New("invalid Visual Foundation world proof")
		}
	}
	if !visualFoundationHashPattern.MatchString(world.StoryGraphContentHash) ||
		!visualFoundationHashPattern.MatchString(world.ProductionWorldOwnerSetHash) ||
		command.Source.Validate() != nil || command.Source.WorkspaceID != world.WorkspaceID ||
		command.Source.ProjectID != world.ProjectID {
		return errors.New("Visual Foundation world proof has drifted")
	}
	bibleRoots := 0
	for _, root := range world.ConfirmedWorldRoots {
		if root.OwnerFamily == "bible_production_world_set" {
			bibleRoots++
			if root.CollectionRootHash != command.Source.BibleCollectionRootHash {
				return errors.New("Visual Foundation Bible root has drifted")
			}
		}
	}
	if bibleRoots != 1 {
		return errors.New("Visual Foundation Bible root is incomplete")
	}
	if len(command.Source.Candidate.SharedProof.DesignGaps) != 0 {
		return errors.New("Visual Foundation Design Gaps require an explicit visual-domain mapping")
	}
	selectionJSON, err := json.Marshal(command.Selection)
	if err != nil {
		return errors.New("invalid Project Preset selection for Visual Foundation")
	}
	selection, _, err := presetdomain.DecodeProjectSelection(selectionJSON)
	if err != nil || !reflect.DeepEqual(selection, command.Selection) ||
		selection.WorkspaceID != world.WorkspaceID || selection.ProjectID != world.ProjectID ||
		selection.ApplicationMode != "faithful" ||
		selection.PresetRelease.Key != command.PresetRelease.Key ||
		selection.PresetRelease.Release != command.PresetRelease.Release ||
		selection.PresetRelease.ContentHash != command.PresetRelease.ContentHash {
		return errors.New("Project Preset selection has drifted before Visual Foundation input compilation")
	}
	rebuiltRelease, _, err := presetdomain.NewRelease(command.PresetRelease.ReleaseInput)
	if err != nil || !reflect.DeepEqual(rebuiltRelease, command.PresetRelease) || command.PresetRelease.DefaultMode != "faithful" {
		return errors.New("invalid faithful Visual Foundation Preset release")
	}
	return nil
}

func visualFoundationInputSetHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}
