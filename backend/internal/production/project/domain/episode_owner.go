package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	"github.com/google/uuid"
)

const ProjectEpisodeCollectionFamily = "project_episode_set"

const ProjectEpisodeCheckpoint = "project_episode_lifecycle_confirmed"

var episodeOwnerHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type EpisodeOwnerVersion struct {
	ID                string
	WorkspaceID       string
	ProjectID         string
	EpisodeID         string
	Revision          int64
	ParentVersionID   *string
	ParentContentHash *string
	Status            string
	Position          int
	SequenceKey       string
	Name              string
	TargetDurationMS  int
	SourceVersionID   string
	ScriptVersionID   string
	SourceStart       int
	SourceEnd         int
	ScriptContentHash string
	ContentHash       string
	CreatedBy         string
	CreatedAt         time.Time
}

type ProjectEpisodeMembership struct {
	ID                 string
	WorkspaceID        string
	ProjectID          string
	ScopeRevision      int64
	Position           int
	EpisodeID          string
	EpisodeVersionID   string
	VersionContentHash string
	CreatedAt          time.Time
}

type ProjectEpisodeScopeHead struct {
	WorkspaceID        string
	ProjectID          string
	ScopeKey           string
	ScopeRevision      int64
	ScopeContentHash   string
	MemberCount        int64
	MembersHash        string
	CollectionRootHash string
	CurrentVersionRefs []ownercollection.VersionRef
	HeadRevision       int64
	HeadContentHash    string
	UpdatedAt          time.Time
}

type ProjectEpisodeCollectionReceipt struct {
	ID                        string
	CommandID                 string
	IdempotencyKey            string
	WorkspaceID               string
	ProjectID                 string
	ReviewDecisionID          string
	ScopeRevision             int64
	ScopeContentHash          string
	Members                   []ownercollection.VersionRef
	MemberCount               int64
	MembersHash               string
	CollectionRootHash        string
	CoveredScopeKeys          []string
	CommittedOwnerVersionRefs []ownercollection.VersionRef
	ReceiptContentHash        string
	CommittedAt               time.Time
	CommittedBy               string
}

func EpisodeSequenceKey(position int) string {
	return fmt.Sprintf("%012d", position)
}

func NewEpisodeOwnerVersion(value EpisodeOwnerVersion) (EpisodeOwnerVersion, error) {
	parentEmpty := value.ParentVersionID == nil && value.ParentContentHash == nil
	parentComplete := value.ParentVersionID != nil && value.ParentContentHash != nil
	if !episodeOwnerUUIDs(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, value.SourceVersionID, value.ScriptVersionID, value.CreatedBy) ||
		value.Revision < 1 || value.Position < 1 || value.SequenceKey != EpisodeSequenceKey(value.Position) ||
		!slices.Contains([]string{"active", "archived"}, value.Status) || strings.TrimSpace(value.Name) == "" ||
		value.TargetDurationMS < 1 || value.SourceStart < 0 || value.SourceEnd <= value.SourceStart ||
		!episodeOwnerHashPattern.MatchString(value.ScriptContentHash) || value.CreatedAt.IsZero() ||
		(value.Revision == 1 && !parentEmpty) || (value.Revision > 1 && !parentComplete) {
		return EpisodeOwnerVersion{}, errors.New("invalid Project Episode Version")
	}
	if value.Revision > 1 && (!episodeOwnerUUIDs(*value.ParentVersionID) || !episodeOwnerHashPattern.MatchString(*value.ParentContentHash)) {
		return EpisodeOwnerVersion{}, errors.New("invalid Project Episode parent Version")
	}
	value.CreatedAt = value.CreatedAt.UTC()
	value.ContentHash = ""
	encoded, err := json.Marshal(struct {
		ContractID        string  `json:"contract_id"`
		WorkspaceID       string  `json:"workspace_id"`
		ProjectID         string  `json:"project_id"`
		EpisodeID         string  `json:"episode_id"`
		Revision          int64   `json:"revision"`
		ParentVersionID   *string `json:"parent_owner_version_id"`
		ParentContentHash *string `json:"parent_owner_content_hash"`
		Status            string  `json:"status"`
		Position          int     `json:"position"`
		SequenceKey       string  `json:"sequence_key"`
		Name              string  `json:"name"`
		TargetDurationMS  int     `json:"target_duration_ms"`
		SourceVersionID   string  `json:"source_version_id"`
		ScriptVersionID   string  `json:"script_version_id"`
		SourceStart       int     `json:"source_start"`
		SourceEnd         int     `json:"source_end"`
		ScriptContentHash string  `json:"script_content_hash"`
	}{
		ContractID: "project-episode-version", WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		EpisodeID: value.EpisodeID, Revision: value.Revision,
		ParentVersionID: value.ParentVersionID, ParentContentHash: value.ParentContentHash,
		Status: value.Status, Position: value.Position, SequenceKey: value.SequenceKey,
		Name: value.Name, TargetDurationMS: value.TargetDurationMS,
		SourceVersionID: value.SourceVersionID, ScriptVersionID: value.ScriptVersionID,
		SourceStart: value.SourceStart, SourceEnd: value.SourceEnd, ScriptContentHash: value.ScriptContentHash,
	})
	if err != nil {
		return EpisodeOwnerVersion{}, err
	}
	value.ContentHash, err = platformcanonical.Hash(encoded)
	return value, err
}

func BuildProjectEpisodeCollection(
	workspaceID string,
	projectID string,
	scopeRevision int64,
	versions []EpisodeOwnerVersion,
) (ownercollection.Ref, error) {
	ordered := append([]EpisodeOwnerVersion(nil), versions...)
	slices.SortFunc(ordered, func(left, right EpisodeOwnerVersion) int { return left.Position - right.Position })
	members := make([]ownercollection.VersionRef, len(ordered))
	for index, version := range ordered {
		rebuilt, err := NewEpisodeOwnerVersion(version)
		if err != nil || rebuilt.ContentHash != version.ContentHash || version.WorkspaceID != workspaceID ||
			version.ProjectID != projectID || version.Status != "active" || version.Position != index+1 {
			return ownercollection.Ref{}, errors.New("invalid Project Episode Collection member set")
		}
		members[index] = ownercollection.VersionRef{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "production/project", VersionFamily: ProjectEpisodeCollectionFamily,
			OwnerLogicalID: version.EpisodeID, OwnerVersionID: version.ID,
			OwnerRevision: version.Revision, OwnerContentHash: version.ContentHash,
		}
	}
	if len(members) == 0 {
		return ownercollection.Ref{}, errors.New("Project Episode Collection cannot be empty")
	}
	return ownercollection.Build(ownercollection.Scope{
		WorkspaceID: workspaceID, ProjectID: projectID,
		OwnerKind: "production/project", VersionFamily: ProjectEpisodeCollectionFamily,
		ScopeKind: "project", ScopeKey: "project:" + projectID, ScopeRevision: scopeRevision,
	}, members)
}

func NewProjectEpisodeScopeHead(
	collection ownercollection.Ref,
	headRevision int64,
	updatedAt time.Time,
) (ProjectEpisodeScopeHead, error) {
	rebuilt, buildErr := ownercollection.Build(ownercollection.Scope{
		WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID,
		OwnerKind: collection.OwnerKind, VersionFamily: collection.VersionFamily,
		ScopeKind: collection.ScopeKind, ScopeKey: collection.ScopeKey, ScopeRevision: collection.ScopeRevision,
	}, collection.Members)
	if buildErr != nil || !reflect.DeepEqual(rebuilt, collection) ||
		collection.OwnerKind != "production/project" || collection.VersionFamily != ProjectEpisodeCollectionFamily ||
		collection.ScopeKind != "project" || collection.ScopeKey != "project:"+collection.ProjectID ||
		collection.MemberCount < 1 || headRevision < 1 || updatedAt.IsZero() {
		return ProjectEpisodeScopeHead{}, errors.New("invalid Project Episode Scope Head")
	}
	value := ProjectEpisodeScopeHead{
		WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID,
		ScopeKey: collection.ScopeKey, ScopeRevision: collection.ScopeRevision,
		ScopeContentHash: collection.ScopeContentHash, MemberCount: collection.MemberCount,
		MembersHash: collection.MembersHash, CollectionRootHash: collection.CollectionRootHash,
		CurrentVersionRefs: append([]ownercollection.VersionRef(nil), collection.Members...),
		HeadRevision:       headRevision, UpdatedAt: updatedAt.UTC(),
	}
	var err error
	value.HeadContentHash, err = episodeOwnerValueHash(struct {
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
		HeadRevision       int64                        `json:"head_revision"`
	}{
		ContractID: "project-episode-scope-head", WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		ScopeKey: value.ScopeKey, ScopeRevision: value.ScopeRevision, ScopeContentHash: value.ScopeContentHash,
		MemberCount: value.MemberCount, MembersHash: value.MembersHash,
		CollectionRootHash: value.CollectionRootHash, CurrentVersionRefs: value.CurrentVersionRefs,
		HeadRevision: value.HeadRevision,
	})
	return value, err
}

func NewProjectEpisodeCollectionReceipt(
	id string,
	commandID string,
	idempotencyKey string,
	reviewDecisionID string,
	collection ownercollection.Ref,
	committedAt time.Time,
	committedBy string,
) (ProjectEpisodeCollectionReceipt, error) {
	rebuilt, buildErr := ownercollection.Build(ownercollection.Scope{
		WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID,
		OwnerKind: collection.OwnerKind, VersionFamily: collection.VersionFamily,
		ScopeKind: collection.ScopeKind, ScopeKey: collection.ScopeKey, ScopeRevision: collection.ScopeRevision,
	}, collection.Members)
	if buildErr != nil || !reflect.DeepEqual(rebuilt, collection) ||
		!episodeOwnerUUIDs(id, commandID, reviewDecisionID, committedBy) || strings.TrimSpace(idempotencyKey) == "" ||
		collection.OwnerKind != "production/project" || collection.VersionFamily != ProjectEpisodeCollectionFamily ||
		collection.ScopeKind != "project" || collection.MemberCount < 1 || committedAt.IsZero() {
		return ProjectEpisodeCollectionReceipt{}, errors.New("invalid Project Episode Collection Receipt")
	}
	value := ProjectEpisodeCollectionReceipt{
		ID: id, CommandID: commandID, IdempotencyKey: idempotencyKey,
		WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID,
		ReviewDecisionID: reviewDecisionID, ScopeRevision: collection.ScopeRevision,
		ScopeContentHash: collection.ScopeContentHash,
		Members:          append([]ownercollection.VersionRef(nil), collection.Members...), MemberCount: collection.MemberCount,
		MembersHash: collection.MembersHash, CollectionRootHash: collection.CollectionRootHash,
		CoveredScopeKeys:          []string{},
		CommittedOwnerVersionRefs: append([]ownercollection.VersionRef(nil), collection.Members...),
		CommittedAt:               committedAt.UTC(), CommittedBy: committedBy,
	}
	var err error
	value.ReceiptContentHash, err = episodeOwnerValueHash(struct {
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
		ContractID: "project-episode-collection-receipt", CollectionReceiptID: value.ID,
		CommandID: value.CommandID, IdempotencyKey: value.IdempotencyKey,
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		DecisionCheckpointID: ProjectEpisodeCheckpoint, ReviewDecisionID: value.ReviewDecisionID,
		OwnerKind: "production/project", VersionFamily: ProjectEpisodeCollectionFamily,
		ScopeKind: "project", ScopeKey: "project:" + value.ProjectID,
		ScopeRevision: value.ScopeRevision, ScopeContentHash: value.ScopeContentHash,
		Members: value.Members, MemberCount: value.MemberCount, MembersHash: value.MembersHash,
		CollectionRootHash: value.CollectionRootHash, CoveredScopeKeys: value.CoveredScopeKeys,
		CommittedOwnerVersionRefs: value.CommittedOwnerVersionRefs,
	})
	return value, err
}

func episodeOwnerValueHash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(encoded)
}

func episodeOwnerUUIDs(values ...string) bool {
	for _, value := range values {
		if _, err := uuid.Parse(value); err != nil {
			return false
		}
	}
	return true
}
