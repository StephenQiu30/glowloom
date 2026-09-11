package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

var structureIdentityOwnerHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type StructureIdentityScopeHead struct {
	WorkspaceID        string
	ProjectID          string
	ScopeKey           string
	ScopeRevision      int64
	ScopeContentHash   string
	MemberCount        int64
	MembersHash        string
	CollectionRootHash string
	CurrentVersionRefs []ownercollection.VersionRef
	CurrentVersionID   string
	HeadRevision       int64
	HeadContentHash    string
	UpdatedAt          time.Time
}

type StructureIdentityCollectionReceipt struct {
	ID                        string                       `json:"collection_receipt_id"`
	CommandID                 string                       `json:"command_id"`
	IdempotencyKey            string                       `json:"idempotency_key"`
	WorkspaceID               string                       `json:"workspace_id"`
	ProjectID                 string                       `json:"project_id"`
	CheckpointKey             string                       `json:"decision_checkpoint_id"`
	OwnerKind                 string                       `json:"owner_kind"`
	CollectionFamily          string                       `json:"version_family"`
	ScopeKind                 string                       `json:"scope_kind"`
	ScopeKey                  string                       `json:"scope_key"`
	ScopeRevision             int64                        `json:"scope_revision"`
	ScopeContentHash          string                       `json:"scope_content_hash"`
	Members                   []ownercollection.VersionRef `json:"members"`
	MemberCount               int64                        `json:"member_count"`
	MembersHash               string                       `json:"members_hash"`
	CollectionRootHash        string                       `json:"collection_root_hash"`
	CoveredScopeKeys          []string                     `json:"covered_scope_keys"`
	CommittedOwnerVersionRefs []ownercollection.VersionRef `json:"committed_owner_version_refs"`
	ReviewDecisionID          string                       `json:"review_decision_id"`
	ReceiptContentHash        string                       `json:"receipt_content_hash"`
	CommittedAt               time.Time                    `json:"committed_at"`
	CommittedBy               string                       `json:"committed_by"`
}

func BuildStructureIdentityCollection(version StructureIdentitySetVersion) (ownercollection.Ref, error) {
	if version.SchemaVersion != StructureIdentitySetSchemaVersion || !structureIdentityOwnerUUIDs(
		version.ID, version.WorkspaceID, version.ProjectID,
	) || version.Version < 1 || !structureIdentityOwnerHashPattern.MatchString(version.ContentHash) {
		return ownercollection.Ref{}, errors.New("invalid Structure Identity Owner Version")
	}
	return ownercollection.Build(ownercollection.Scope{
		WorkspaceID: version.WorkspaceID, ProjectID: version.ProjectID,
		OwnerKind: "production/bible", VersionFamily: StructureIdentityCollectionFamily,
		ScopeKind: "project", ScopeKey: "project:" + version.ProjectID, ScopeRevision: int64(version.Version),
	}, []ownercollection.VersionRef{{
		WorkspaceID: version.WorkspaceID, ProjectID: version.ProjectID,
		OwnerKind: "production/bible", VersionFamily: StructureIdentityCollectionFamily,
		OwnerLogicalID: version.ProjectID, OwnerVersionID: version.ID,
		OwnerRevision: int64(version.Version), OwnerContentHash: version.ContentHash,
	}})
}

func NewStructureIdentityScopeHead(
	collection ownercollection.Ref,
	currentVersionID string,
	headRevision int64,
	updatedAt time.Time,
) (StructureIdentityScopeHead, error) {
	rebuilt, err := rebuildStructureIdentityCollection(collection)
	if err != nil || !reflect.DeepEqual(rebuilt, collection) || collection.MemberCount != 1 ||
		!structureIdentityOwnerUUIDs(currentVersionID) || collection.Members[0].OwnerVersionID != currentVersionID ||
		headRevision < 1 || updatedAt.IsZero() {
		return StructureIdentityScopeHead{}, errors.New("invalid Structure Identity Scope Head")
	}
	value := StructureIdentityScopeHead{
		WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID, ScopeKey: collection.ScopeKey,
		ScopeRevision: collection.ScopeRevision, ScopeContentHash: collection.ScopeContentHash,
		MemberCount: collection.MemberCount, MembersHash: collection.MembersHash,
		CollectionRootHash: collection.CollectionRootHash,
		CurrentVersionRefs: append([]ownercollection.VersionRef(nil), collection.Members...),
		CurrentVersionID:   currentVersionID, HeadRevision: headRevision, UpdatedAt: updatedAt.UTC(),
	}
	value.HeadContentHash, err = structureIdentityOwnerHash(struct {
		ContractID         string                       `json:"contract_id"`
		WorkspaceID        string                       `json:"workspace_id"`
		ProjectID          string                       `json:"project_id"`
		ScopeKey           string                       `json:"scope_key"`
		ScopeRevision      int64                        `json:"scope_revision"`
		ScopeContentHash   string                       `json:"scope_content_hash"`
		MemberCount        int64                        `json:"member_count"`
		MembersHash        string                       `json:"members_hash"`
		CollectionRootHash string                       `json:"collection_root_hash"`
		CurrentVersionRefs []ownercollection.VersionRef `json:"current_root_version_refs"`
		CurrentVersionID   string                       `json:"current_version_id"`
		HeadRevision       int64                        `json:"head_revision"`
	}{
		"structure-identity-scope-head", value.WorkspaceID, value.ProjectID, value.ScopeKey,
		value.ScopeRevision, value.ScopeContentHash, value.MemberCount, value.MembersHash,
		value.CollectionRootHash, value.CurrentVersionRefs, value.CurrentVersionID, value.HeadRevision,
	})
	return value, err
}

func NewStructureIdentityCollectionReceipt(
	id string,
	commandID string,
	idempotencyKey string,
	reviewDecisionID string,
	collection ownercollection.Ref,
	coveredScopeKeys []string,
	committedAt time.Time,
	committedBy string,
) (StructureIdentityCollectionReceipt, error) {
	rebuilt, err := rebuildStructureIdentityCollection(collection)
	scopes := append([]string(nil), coveredScopeKeys...)
	if err != nil || !reflect.DeepEqual(rebuilt, collection) || collection.MemberCount != 1 ||
		!structureIdentityOwnerUUIDs(id, commandID, reviewDecisionID, committedBy) ||
		strings.TrimSpace(idempotencyKey) == "" || committedAt.IsZero() || !canonicalSceneScopes(scopes) {
		return StructureIdentityCollectionReceipt{}, errors.New("invalid Structure Identity Collection Receipt")
	}
	value := StructureIdentityCollectionReceipt{
		ID: id, CommandID: commandID, IdempotencyKey: idempotencyKey,
		WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID,
		CheckpointKey: StructureIdentityCheckpointKey, OwnerKind: "production/bible",
		CollectionFamily: StructureIdentityCollectionFamily,
		ScopeKind:        "project", ScopeKey: collection.ScopeKey, ScopeRevision: collection.ScopeRevision,
		ScopeContentHash: collection.ScopeContentHash,
		Members:          append([]ownercollection.VersionRef(nil), collection.Members...), MemberCount: collection.MemberCount,
		MembersHash: collection.MembersHash, CollectionRootHash: collection.CollectionRootHash,
		CoveredScopeKeys: scopes, CommittedOwnerVersionRefs: append([]ownercollection.VersionRef(nil), collection.Members...),
		ReviewDecisionID: reviewDecisionID, CommittedAt: committedAt.UTC(), CommittedBy: committedBy,
	}
	value.ReceiptContentHash, err = structureIdentityOwnerHash(struct {
		ContractID                string                       `json:"contract_id"`
		CollectionReceiptID       string                       `json:"collection_receipt_id"`
		CommandID                 string                       `json:"command_id"`
		IdempotencyKey            string                       `json:"idempotency_key"`
		WorkspaceID               string                       `json:"workspace_id"`
		ProjectID                 string                       `json:"project_id"`
		DecisionCheckpointID      string                       `json:"decision_checkpoint_id"`
		ReviewDecisionID          string                       `json:"review_decision_id"`
		OwnerKind                 string                       `json:"owner_kind"`
		VersionFamily             string                       `json:"version_family"`
		ScopeKind                 string                       `json:"scope_kind"`
		ScopeKey                  string                       `json:"scope_key"`
		ScopeRevision             int64                        `json:"scope_revision"`
		ScopeContentHash          string                       `json:"scope_content_hash"`
		Members                   []ownercollection.VersionRef `json:"members"`
		MemberCount               int64                        `json:"member_count"`
		MembersHash               string                       `json:"members_hash"`
		CollectionRootHash        string                       `json:"collection_root_hash"`
		CoveredScopeKeys          []string                     `json:"covered_scope_keys"`
		CommittedOwnerVersionRefs []ownercollection.VersionRef `json:"committed_owner_version_refs"`
	}{
		"structure-identity-collection-receipt", value.ID, value.CommandID, value.IdempotencyKey,
		value.WorkspaceID, value.ProjectID, value.CheckpointKey, value.ReviewDecisionID,
		value.OwnerKind, value.CollectionFamily, value.ScopeKind, value.ScopeKey, value.ScopeRevision,
		value.ScopeContentHash, value.Members, value.MemberCount, value.MembersHash,
		value.CollectionRootHash, value.CoveredScopeKeys, value.CommittedOwnerVersionRefs,
	})
	return value, err
}

func rebuildStructureIdentityCollection(value ownercollection.Ref) (ownercollection.Ref, error) {
	if value.OwnerKind != "production/bible" || value.VersionFamily != StructureIdentityCollectionFamily ||
		value.ScopeKind != "project" || value.ScopeKey != "project:"+value.ProjectID {
		return ownercollection.Ref{}, errors.New("invalid Structure Identity Collection")
	}
	return ownercollection.Build(ownercollection.Scope{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, OwnerKind: value.OwnerKind,
		VersionFamily: value.VersionFamily, ScopeKind: value.ScopeKind, ScopeKey: value.ScopeKey,
		ScopeRevision: value.ScopeRevision,
	}, value.Members)
}

func canonicalSceneScopes(values []string) bool {
	if len(values) == 0 || !slices.IsSorted(values) {
		return false
	}
	for index, value := range values {
		if !strings.HasPrefix(value, "scene:") || len(value) == len("scene:") ||
			(index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

func structureIdentityOwnerHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return canonical.Hash(raw)
}

func structureIdentityOwnerUUIDs(values ...string) bool {
	for _, value := range values {
		if _, err := uuid.Parse(value); err != nil {
			return false
		}
	}
	return true
}
