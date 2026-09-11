package domain

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

const AssetIdentityStateCollectionFamily = "asset_identity_state_set"

type IdentityStateMember struct {
	AssetID           string `json:"asset_id"`
	AssetStateID      string `json:"asset_state_id"`
	IdentityKey       string `json:"identity_key"`
	StateKey          string `json:"state_key"`
	AssetContentHash  string `json:"asset_content_hash"`
	StateContentHash  string `json:"state_content_hash"`
	MemberContentHash string `json:"member_content_hash"`
}

type IdentityStateCollectionHead struct {
	WorkspaceID        string                       `json:"workspace_id"`
	ProjectID          string                       `json:"project_id"`
	ScopeKey           string                       `json:"scope_key"`
	ScopeRevision      int64                        `json:"scope_revision"`
	ScopeContentHash   string                       `json:"scope_content_hash"`
	Members            []IdentityStateMember        `json:"members"`
	CurrentVersionRefs []ownercollection.VersionRef `json:"current_root_version_refs"`
	MemberCount        int                          `json:"member_count"`
	MembersHash        string                       `json:"members_hash"`
	CollectionRootHash string                       `json:"collection_root_hash"`
	HeadRevision       int64                        `json:"head_revision"`
	HeadContentHash    string                       `json:"head_content_hash"`
	UpdatedAt          time.Time                    `json:"updated_at"`
}

func BuildIdentityStateCollection(
	workspaceID, projectID string,
	revision int64,
	assets []Asset,
	states []AssetState,
) (ownercollection.Ref, error) {
	if _, err := uuid.Parse(workspaceID); err != nil {
		return ownercollection.Ref{}, errors.New("invalid Asset collection workspace")
	}
	if _, err := uuid.Parse(projectID); err != nil || revision < 1 || len(assets) == 0 || len(states) == 0 {
		return ownercollection.Ref{}, errors.New("invalid Asset collection input")
	}
	assets = append([]Asset(nil), assets...)
	slices.SortFunc(assets, func(left, right Asset) int {
		if compared := strings.Compare(left.IdentityKey, right.IdentityKey); compared != 0 {
			return compared
		}
		return strings.Compare(left.ID, right.ID)
	})
	assetByID := make(map[string]Asset, len(assets))
	stateCountByAsset := make(map[string]int, len(assets))
	logicalIDs := make(map[string]struct{}, len(assets)+len(states))
	members := make([]ownercollection.VersionRef, 0, len(assets)+len(states))
	for _, asset := range assets {
		if ValidateAsset(asset) != nil || asset.WorkspaceID != workspaceID || asset.ProjectID != projectID {
			return ownercollection.Ref{}, errors.New("invalid Asset collection identity set")
		}
		if _, exists := assetByID[asset.ID]; exists {
			return ownercollection.Ref{}, errors.New("duplicate Asset collection identity")
		}
		if _, exists := logicalIDs[asset.IdentityKey]; exists {
			return ownercollection.Ref{}, errors.New("duplicate Asset collection logical identity")
		}
		assetByID[asset.ID] = asset
		logicalIDs[asset.IdentityKey] = struct{}{}
		members = append(members, ownercollection.VersionRef{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "asset", VersionFamily: AssetIdentityStateCollectionFamily,
			OwnerLogicalID: asset.IdentityKey, OwnerVersionID: asset.ID,
			OwnerRevision: int64(asset.Revision), OwnerContentHash: asset.ContentHash,
		})
	}
	states = append([]AssetState(nil), states...)
	slices.SortFunc(states, func(left, right AssetState) int {
		if compared := strings.Compare(left.StateKey, right.StateKey); compared != 0 {
			return compared
		}
		return strings.Compare(left.ID, right.ID)
	})
	seenStateIDs := make(map[string]struct{}, len(states))
	for _, state := range states {
		asset, exists := assetByID[state.AssetID]
		if ValidateAssetState(state) != nil || !exists || state.WorkspaceID != workspaceID ||
			state.ProjectID != projectID || state.AssetID != asset.ID {
			return ownercollection.Ref{}, errors.New("invalid Asset collection state set")
		}
		if _, exists = seenStateIDs[state.ID]; exists {
			return ownercollection.Ref{}, errors.New("duplicate Asset collection state")
		}
		if _, exists = logicalIDs[state.StateKey]; exists {
			return ownercollection.Ref{}, errors.New("duplicate Asset collection logical identity")
		}
		seenStateIDs[state.ID] = struct{}{}
		logicalIDs[state.StateKey] = struct{}{}
		stateCountByAsset[state.AssetID]++
		members = append(members, ownercollection.VersionRef{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "asset", VersionFamily: AssetIdentityStateCollectionFamily,
			OwnerLogicalID: state.StateKey, OwnerVersionID: state.ID,
			OwnerRevision: int64(state.Revision), OwnerContentHash: state.ContentHash,
		})
	}
	for assetID := range assetByID {
		if stateCountByAsset[assetID] == 0 {
			return ownercollection.Ref{}, errors.New("Asset collection identity has no state")
		}
	}
	return ownercollection.Build(ownercollection.Scope{
		WorkspaceID: workspaceID, ProjectID: projectID,
		OwnerKind: "asset", VersionFamily: AssetIdentityStateCollectionFamily,
		ScopeKind: "project", ScopeKey: "project:" + projectID, ScopeRevision: revision,
	}, members)
}

func NewIdentityStateMember(asset Asset, state AssetState) (IdentityStateMember, error) {
	if ValidateAsset(asset) != nil || ValidateAssetState(state) != nil ||
		asset.WorkspaceID != state.WorkspaceID || asset.ProjectID != state.ProjectID || asset.ID != state.AssetID {
		return IdentityStateMember{}, errors.New("invalid Asset identity-state member")
	}
	value := IdentityStateMember{
		AssetID: asset.ID, AssetStateID: state.ID, IdentityKey: asset.IdentityKey, StateKey: state.StateKey,
		AssetContentHash: asset.ContentHash, StateContentHash: state.ContentHash,
	}
	hash, err := identityStateHash("asset-identity-state-member", value)
	if err != nil {
		return IdentityStateMember{}, err
	}
	value.MemberContentHash = hash
	return value, nil
}

func ValidateIdentityStateMember(value IdentityStateMember) error {
	for _, identifier := range []string{value.AssetID, value.AssetStateID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Asset identity-state member identity")
		}
	}
	if strings.TrimSpace(value.IdentityKey) == "" || strings.TrimSpace(value.StateKey) == "" ||
		!ValidContentHash(value.AssetContentHash) || !ValidContentHash(value.StateContentHash) ||
		!ValidContentHash(value.MemberContentHash) {
		return errors.New("invalid Asset identity-state member")
	}
	originalHash := value.MemberContentHash
	value.MemberContentHash = ""
	expectedHash, err := identityStateHash("asset-identity-state-member", value)
	if err != nil || expectedHash != originalHash {
		return errors.New("Asset identity-state member has drifted")
	}
	return nil
}

func NewIdentityStateCollectionHead(
	workspaceID, projectID string,
	revision int64,
	assets []Asset,
	states []AssetState,
	updatedAt time.Time,
) (IdentityStateCollectionHead, error) {
	collection, err := BuildIdentityStateCollection(workspaceID, projectID, revision, assets, states)
	if err != nil || updatedAt.IsZero() {
		return IdentityStateCollectionHead{}, errors.New("invalid Asset collection input")
	}
	assetByID := make(map[string]Asset, len(assets))
	for _, asset := range assets {
		assetByID[asset.ID] = asset
	}
	members := make([]IdentityStateMember, len(states))
	for index, state := range states {
		member, memberErr := NewIdentityStateMember(assetByID[state.AssetID], state)
		if memberErr != nil {
			return IdentityStateCollectionHead{}, memberErr
		}
		members[index] = member
	}
	slices.SortFunc(members, func(left, right IdentityStateMember) int {
		if result := strings.Compare(left.IdentityKey, right.IdentityKey); result != 0 {
			return result
		}
		return strings.Compare(left.StateKey, right.StateKey)
	})
	for index, member := range members {
		if ValidateIdentityStateMember(member) != nil ||
			(index > 0 && members[index-1].IdentityKey == member.IdentityKey && members[index-1].StateKey == member.StateKey) {
			return IdentityStateCollectionHead{}, errors.New("invalid Asset collection member set")
		}
	}
	value := IdentityStateCollectionHead{
		WorkspaceID: workspaceID, ProjectID: projectID, ScopeKey: collection.ScopeKey,
		ScopeRevision: collection.ScopeRevision, ScopeContentHash: collection.ScopeContentHash, Members: members,
		CurrentVersionRefs: append([]ownercollection.VersionRef(nil), collection.Members...),
		MemberCount:        int(collection.MemberCount), MembersHash: collection.MembersHash,
		CollectionRootHash: collection.CollectionRootHash,
		HeadRevision:       revision, UpdatedAt: updatedAt.UTC(),
	}
	value.HeadContentHash, err = identityStateHeadHash(value)
	if err != nil {
		return IdentityStateCollectionHead{}, err
	}
	return value, nil
}

func ValidateIdentityStateCollectionHead(value IdentityStateCollectionHead) error {
	collection, err := ownercollection.Build(ownercollection.Scope{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		OwnerKind: "asset", VersionFamily: AssetIdentityStateCollectionFamily,
		ScopeKind: "project", ScopeKey: value.ScopeKey, ScopeRevision: value.ScopeRevision,
	}, value.CurrentVersionRefs)
	if err != nil || value.ScopeKey != "project:"+value.ProjectID || value.HeadRevision != value.ScopeRevision ||
		value.MemberCount != int(collection.MemberCount) || value.ScopeContentHash != collection.ScopeContentHash ||
		value.MembersHash != collection.MembersHash || value.CollectionRootHash != collection.CollectionRootHash ||
		value.UpdatedAt.IsZero() {
		return errors.New("Asset identity-state Head has drifted")
	}
	refsByID := make(map[string]ownercollection.VersionRef, len(value.CurrentVersionRefs))
	for _, ref := range value.CurrentVersionRefs {
		if _, exists := refsByID[ref.OwnerVersionID]; exists {
			return errors.New("Asset identity-state Head has duplicate Version identity")
		}
		refsByID[ref.OwnerVersionID] = ref
	}
	assetIDs := make(map[string]struct{}, len(value.Members))
	stateIDs := make(map[string]struct{}, len(value.Members))
	for _, member := range value.Members {
		assetRef, assetExists := refsByID[member.AssetID]
		stateRef, stateExists := refsByID[member.AssetStateID]
		if ValidateIdentityStateMember(member) != nil || !assetExists || !stateExists ||
			assetRef.OwnerLogicalID != member.IdentityKey || assetRef.OwnerContentHash != member.AssetContentHash ||
			stateRef.OwnerLogicalID != member.StateKey || stateRef.OwnerContentHash != member.StateContentHash {
			return errors.New("Asset identity-state Head membership has drifted")
		}
		assetIDs[member.AssetID] = struct{}{}
		if _, exists := stateIDs[member.AssetStateID]; exists {
			return errors.New("Asset identity-state Head has duplicate State membership")
		}
		stateIDs[member.AssetStateID] = struct{}{}
	}
	if len(refsByID) != len(assetIDs)+len(stateIDs) {
		return errors.New("Asset identity-state Head Version coverage has drifted")
	}
	headHash, err := identityStateHeadHash(value)
	if err != nil || headHash != value.HeadContentHash {
		return errors.New("Asset identity-state Head content has drifted")
	}
	return nil
}

func identityStateHeadHash(value IdentityStateCollectionHead) (string, error) {
	return identityStateHash("asset-identity-state-scope-head", struct {
		WorkspaceID        string                       `json:"workspace_id"`
		ProjectID          string                       `json:"project_id"`
		ScopeKey           string                       `json:"scope_key"`
		ScopeRevision      int64                        `json:"scope_revision"`
		ScopeContentHash   string                       `json:"scope_content_hash"`
		MemberCount        int                          `json:"member_count"`
		MembersHash        string                       `json:"members_hash"`
		CollectionRootHash string                       `json:"collection_root_hash"`
		CurrentVersionRefs []ownercollection.VersionRef `json:"current_root_version_refs"`
		HeadRevision       int64                        `json:"head_revision"`
	}{
		value.WorkspaceID, value.ProjectID, value.ScopeKey, value.ScopeRevision,
		value.ScopeContentHash, value.MemberCount, value.MembersHash, value.CollectionRootHash,
		value.CurrentVersionRefs, value.HeadRevision,
	})
}

func identityStateHash(schema string, value any) (string, error) {
	raw, err := json.Marshal(struct {
		Schema string `json:"schema"`
		Value  any    `json:"value"`
	}{schema, value})
	if err != nil {
		return "", err
	}
	return canonical.Hash(raw)
}
