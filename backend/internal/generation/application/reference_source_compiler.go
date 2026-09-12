package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type ReferenceGenerationSourceCompilation struct {
	Payload                     json.RawMessage
	ContentHash                 string
	ProductionWorldOwnerSetHash string
}

type ReferenceBaseSource struct {
	ProductionBindingRef agentcontract.ReferencePlanOwnerRef   `json:"production_binding_ref"`
	OccurrenceRefs       []agentcontract.ReferencePlanOwnerRef `json:"occurrence_refs"`
	RequiredViewRoles    []string                              `json:"required_view_roles"`
}

type CharacterIdentityAnchorSource struct {
	ReferenceBaseSource
	agentcontract.CharacterIdentityAnchorBrief
	IdentityRef                 agentcontract.ReferencePlanOwnerRef `json:"identity_ref"`
	CharacterSpecificationRef   agentcontract.ReferencePlanOwnerRef `json:"character_specification_ref"`
	IdentityAnchorAssetStateRef agentcontract.ReferencePlanOwnerRef `json:"identity_anchor_asset_state_ref"`
}

type LocationBoardSource struct {
	ReferenceBaseSource
	agentcontract.LocationBoardBrief
	LocationIdentityRef      agentcontract.ReferencePlanOwnerRef `json:"location_identity_ref"`
	LocationSpecificationRef agentcontract.ReferencePlanOwnerRef `json:"location_specification_ref"`
	LocationAssetStateRef    agentcontract.ReferencePlanOwnerRef `json:"location_asset_state_ref"`
}

type PropSheetSource struct {
	ReferenceBaseSource
	agentcontract.PropSheetBrief
	PropIdentityRef      agentcontract.ReferencePlanOwnerRef `json:"prop_identity_ref"`
	PropSpecificationRef agentcontract.ReferencePlanOwnerRef `json:"prop_specification_ref"`
	PropAssetStateRef    agentcontract.ReferencePlanOwnerRef `json:"prop_asset_state_ref"`
}

// CompileBaseReferenceGenerationSource consumes a verified immutable projection.
// The caller must revalidate its Owner heads and approved Plan in the write transaction.
func CompileBaseReferenceGenerationSource(input agentcontract.ReferenceBriefInput, brief agentcontract.ReferenceBriefCandidate, world storygraph.Version) (ReferenceGenerationSourceCompilation, error) {
	if err := brief.ValidateFor(input); err != nil {
		return ReferenceGenerationSourceCompilation{}, fmt.Errorf("validate Reference generation source Brief: %w", err)
	}
	if world.WorkspaceID != input.WorkspaceID || world.ProjectID != input.ProjectID || world.SchemaVersion != storygraph.ProductionSchemaID {
		return ReferenceGenerationSourceCompilation{}, errors.New("Reference generation source facts have drifted")
	}
	if err := storygraph.ValidateProductionVersion(world); err != nil {
		return ReferenceGenerationSourceCompilation{}, fmt.Errorf("validate Reference generation source World: %w", err)
	}
	specificationKind := map[string]storygraph.NodeType{
		"character_identity_anchor": storygraph.NodeTypeCharacterSpecification,
		"location_board":            storygraph.NodeTypeLocationSpecification,
		"prop_sheet":                storygraph.NodeTypePropSpecification,
	}[input.TargetKind]
	refs := input.SourceRefs
	if specificationKind == "" || len(input.DependencySelections) != 0 || len(refs.Identity) != 1 || len(refs.Specification) != 1 || len(refs.State) != 1 || len(refs.Occurrence) == 0 || len(refs.Scene) == 0 || len(refs.Interaction) != 0 {
		return ReferenceGenerationSourceCompilation{}, errors.New("Reference generation requires an exact dependency-free base source")
	}
	binding, err := resolveReferenceBaseBinding(world.Nodes, refs, specificationKind)
	if err != nil {
		return ReferenceGenerationSourceCompilation{}, err
	}
	base := ReferenceBaseSource{ProductionBindingRef: referenceSourceContractRef(binding), OccurrenceRefs: refs.Occurrence, RequiredViewRoles: brief.RequiredViewRoles}
	var payload any
	switch input.TargetKind {
	case "character_identity_anchor":
		value := CharacterIdentityAnchorSource{ReferenceBaseSource: base, IdentityRef: refs.Identity[0], CharacterSpecificationRef: refs.Specification[0], IdentityAnchorAssetStateRef: refs.State[0]}
		if err = json.Unmarshal(brief.Brief, &value.CharacterIdentityAnchorBrief); err != nil {
			return ReferenceGenerationSourceCompilation{}, err
		}
		payload = value
	case "location_board":
		value := LocationBoardSource{ReferenceBaseSource: base, LocationIdentityRef: refs.Identity[0], LocationSpecificationRef: refs.Specification[0], LocationAssetStateRef: refs.State[0]}
		if err = json.Unmarshal(brief.Brief, &value.LocationBoardBrief); err != nil {
			return ReferenceGenerationSourceCompilation{}, err
		}
		payload = value
	case "prop_sheet":
		value := PropSheetSource{ReferenceBaseSource: base, PropIdentityRef: refs.Identity[0], PropSpecificationRef: refs.Specification[0], PropAssetStateRef: refs.State[0]}
		if err = json.Unmarshal(brief.Brief, &value.PropSheetBrief); err != nil {
			return ReferenceGenerationSourceCompilation{}, err
		}
		payload = value
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ReferenceGenerationSourceCompilation{}, err
	}
	raw, err = canonical.JSON(raw)
	if err != nil {
		return ReferenceGenerationSourceCompilation{}, err
	}
	hash, err := canonical.Hash(raw)
	if err != nil {
		return ReferenceGenerationSourceCompilation{}, err
	}
	return ReferenceGenerationSourceCompilation{Payload: raw, ContentHash: hash, ProductionWorldOwnerSetHash: world.OwnerSetHash}, nil
}

func resolveReferenceBaseBinding(nodes []storygraph.Node, refs agentcontract.ReferencePlanTargetOwnerRefs, specificationKind storygraph.NodeType) (storygraph.OwnerRef, error) {
	byRef := make(map[storygraph.OwnerRef]storygraph.Node, len(nodes))
	for _, node := range nodes {
		if _, duplicate := byRef[node.OwnerRef]; duplicate {
			return storygraph.OwnerRef{}, errors.New("ambiguous Reference source Owner ref")
		}
		byRef[node.OwnerRef] = node
	}
	for _, group := range []struct {
		refs []agentcontract.ReferencePlanOwnerRef
		kind storygraph.NodeType
	}{{refs.Identity, storygraph.NodeTypeAssetIdentity}, {refs.Specification, specificationKind}, {refs.State, storygraph.NodeTypeAssetState}, {refs.Scene, storygraph.NodeTypeScene}, {refs.Occurrence, storygraph.NodeTypeOccurrence}} {
		for _, ref := range group.refs {
			if node, found := byRef[referenceSourceGraphRef(ref)]; !found || node.NodeType != group.kind {
				return storygraph.OwnerRef{}, errors.New("Reference source Owner ref is missing or has drifted")
			}
		}
	}
	identity, specification, state := referenceSourceGraphRef(refs.Identity[0]), referenceSourceGraphRef(refs.Specification[0]), referenceSourceGraphRef(refs.State[0])
	var binding storygraph.OwnerRef
	bindings := 0
	scenes := make(map[storygraph.OwnerRef]bool, len(refs.Scene))
	occurrences := make(map[storygraph.OwnerRef]bool, len(refs.Occurrence))
	for _, ref := range refs.Scene {
		scenes[referenceSourceGraphRef(ref)] = false
	}
	for _, ref := range refs.Occurrence {
		occurrences[referenceSourceGraphRef(ref)] = false
	}
	for _, node := range nodes {
		// Full payload schemas and graph relations were validated by ValidateProductionVersion.
		switch node.NodeType {
		case storygraph.NodeTypeProductionBinding:
			var value struct {
				Identity      storygraph.OwnerRef   `json:"asset_identity_ref"`
				Specification storygraph.OwnerRef   `json:"specification_ref"`
				States        []storygraph.OwnerRef `json:"state_refs"`
			}
			if err := json.Unmarshal(node.Payload, &value); err != nil {
				return storygraph.OwnerRef{}, err
			}
			if value.Identity == identity && value.Specification == specification && slices.Contains(value.States, state) {
				binding, bindings = node.OwnerRef, bindings+1
			}
		case storygraph.NodeTypeOccurrence:
			var value struct {
				Identity storygraph.OwnerRef `json:"asset_identity_ref"`
				State    storygraph.OwnerRef `json:"asset_state_ref"`
				Scene    storygraph.OwnerRef `json:"scene_ref"`
			}
			if err := json.Unmarshal(node.Payload, &value); err != nil {
				return storygraph.OwnerRef{}, err
			}
			_, inScope := scenes[value.Scene]
			_, requested := occurrences[node.OwnerRef]
			matches := inScope && value.Identity == identity && value.State == state
			if matches != requested {
				return storygraph.OwnerRef{}, errors.New("Reference source Occurrence closure has drifted")
			}
			if matches {
				scenes[value.Scene], occurrences[node.OwnerRef] = true, true
			}
		}
	}
	if bindings != 1 {
		return storygraph.OwnerRef{}, errors.New("Reference source requires one exact Production Binding")
	}
	for _, set := range []map[storygraph.OwnerRef]bool{scenes, occurrences} {
		for _, found := range set {
			if !found {
				return storygraph.OwnerRef{}, errors.New("Reference source contains an unrelated Scene or Occurrence")
			}
		}
	}
	return binding, nil
}

func referenceSourceGraphRef(ref agentcontract.ReferencePlanOwnerRef) storygraph.OwnerRef {
	value := storygraph.OwnerRef{WorkspaceID: ref.WorkspaceID, ProjectID: ref.ProjectID, OwnerKind: ref.OwnerKind, VersionFamily: ref.VersionFamily, OwnerLogicalID: ref.OwnerLogicalID, OwnerVersionID: ref.OwnerVersionID, OwnerRevision: ref.OwnerRevision, OwnerContentHash: ref.OwnerContentHash}
	if ref.FragmentKey != nil {
		value.FragmentKey = *ref.FragmentKey
	}
	if ref.FragmentContentHash != nil {
		value.FragmentContentHash = *ref.FragmentContentHash
	}
	return value
}

func referenceSourceContractRef(ref storygraph.OwnerRef) agentcontract.ReferencePlanOwnerRef {
	value := agentcontract.ReferencePlanOwnerRef{WorkspaceID: ref.WorkspaceID, ProjectID: ref.ProjectID, OwnerKind: ref.OwnerKind, VersionFamily: ref.VersionFamily, OwnerLogicalID: ref.OwnerLogicalID, OwnerVersionID: ref.OwnerVersionID, OwnerRevision: ref.OwnerRevision, OwnerContentHash: ref.OwnerContentHash}
	if ref.FragmentKey != "" {
		value.FragmentKey = &ref.FragmentKey
	}
	if ref.FragmentContentHash != "" {
		value.FragmentContentHash = &ref.FragmentContentHash
	}
	return value
}
