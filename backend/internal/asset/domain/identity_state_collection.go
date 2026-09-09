package domain

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
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
	WorkspaceID        string                `json:"workspace_id"`
	ProjectID          string                `json:"project_id"`
	ScopeKey           string                `json:"scope_key"`
	ScopeRevision      int64                 `json:"scope_revision"`
	ScopeContentHash   string                `json:"scope_content_hash"`
	Members            []IdentityStateMember `json:"members"`
	MemberCount        int                   `json:"member_count"`
	MembersHash        string                `json:"members_hash"`
	CollectionRootHash string                `json:"collection_root_hash"`
	HeadRevision       int64                 `json:"head_revision"`
	HeadContentHash    string                `json:"head_content_hash"`
	UpdatedAt          time.Time             `json:"updated_at"`
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
	members []IdentityStateMember,
	updatedAt time.Time,
) (IdentityStateCollectionHead, error) {
	if _, err := uuid.Parse(workspaceID); err != nil {
		return IdentityStateCollectionHead{}, errors.New("invalid Asset collection workspace")
	}
	if _, err := uuid.Parse(projectID); err != nil || revision < 1 || len(members) == 0 || updatedAt.IsZero() {
		return IdentityStateCollectionHead{}, errors.New("invalid Asset collection input")
	}
	members = append([]IdentityStateMember(nil), members...)
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
	scopeKey := "project:" + projectID
	scopeHash, err := identityStateHash("asset-identity-state-scope", struct {
		WorkspaceID, ProjectID, Family, ScopeKey string
		ScopeRevision                            int64
		Members                                  []IdentityStateMember
	}{workspaceID, projectID, AssetIdentityStateCollectionFamily, scopeKey, revision, members})
	if err != nil {
		return IdentityStateCollectionHead{}, err
	}
	membersHash, err := identityStateHash("asset-identity-state-members", members)
	if err != nil {
		return IdentityStateCollectionHead{}, err
	}
	collectionRootHash, err := identityStateHash("asset-identity-state-collection", struct {
		ScopeKey, ScopeContentHash, MembersHash string
		ScopeRevision                           int64
		MemberCount                             int
	}{scopeKey, scopeHash, membersHash, revision, len(members)})
	if err != nil {
		return IdentityStateCollectionHead{}, err
	}
	value := IdentityStateCollectionHead{
		WorkspaceID: workspaceID, ProjectID: projectID, ScopeKey: scopeKey,
		ScopeRevision: revision, ScopeContentHash: scopeHash, Members: members,
		MemberCount: len(members), MembersHash: membersHash, CollectionRootHash: collectionRootHash,
		HeadRevision: revision, UpdatedAt: updatedAt.UTC(),
	}
	headHash, err := identityStateHash("asset-identity-state-head", struct {
		WorkspaceID, ProjectID, ScopeKey, ScopeContentHash, MembersHash, CollectionRootHash string
		ScopeRevision, HeadRevision                                                         int64
		MemberCount                                                                         int
	}{value.WorkspaceID, value.ProjectID, value.ScopeKey, value.ScopeContentHash, value.MembersHash,
		value.CollectionRootHash, value.ScopeRevision, value.HeadRevision, value.MemberCount})
	if err != nil {
		return IdentityStateCollectionHead{}, err
	}
	value.HeadContentHash = headHash
	return value, nil
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
