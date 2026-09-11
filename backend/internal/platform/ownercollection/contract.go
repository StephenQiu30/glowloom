package ownercollection

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

const maximumSafeInteger int64 = 9007199254740991

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type VersionRef struct {
	WorkspaceID      string `json:"workspace_id"`
	ProjectID        string `json:"project_id"`
	OwnerKind        string `json:"owner_kind"`
	VersionFamily    string `json:"version_family"`
	OwnerLogicalID   string `json:"owner_logical_id"`
	OwnerVersionID   string `json:"owner_version_id"`
	OwnerRevision    int64  `json:"owner_revision"`
	OwnerContentHash string `json:"owner_content_hash"`
}

type Scope struct {
	WorkspaceID   string
	ProjectID     string
	OwnerKind     string
	VersionFamily string
	ScopeKind     string
	ScopeKey      string
	ScopeRevision int64
}

type Ref struct {
	WorkspaceID        string       `json:"workspace_id"`
	ProjectID          string       `json:"project_id"`
	OwnerKind          string       `json:"owner_kind"`
	VersionFamily      string       `json:"version_family"`
	ScopeKind          string       `json:"scope_kind"`
	ScopeKey           string       `json:"scope_key"`
	ScopeRevision      int64        `json:"scope_revision"`
	ScopeContentHash   string       `json:"scope_content_hash"`
	Members            []VersionRef `json:"members"`
	MemberCount        int64        `json:"member_count"`
	MembersHash        string       `json:"members_hash"`
	CollectionRootHash string       `json:"collection_root_hash"`
}

func Build(scope Scope, members []VersionRef) (Ref, error) {
	if err := validateScope(scope); err != nil {
		return Ref{}, err
	}
	ordered := make([]VersionRef, len(members))
	copy(ordered, members)
	slices.SortFunc(ordered, func(left, right VersionRef) int {
		return strings.Compare(memberKey(left), memberKey(right))
	})
	for index, member := range ordered {
		if err := validateMember(scope, member); err != nil {
			return Ref{}, err
		}
		if index > 0 && memberKey(ordered[index-1]) == memberKey(member) {
			return Ref{}, errors.New("duplicate Owner Collection member")
		}
	}

	scopeContentHash, err := hash(struct {
		WorkspaceID            string       `json:"workspace_id"`
		ProjectID              string       `json:"project_id"`
		OwnerKind              string       `json:"owner_kind"`
		VersionFamily          string       `json:"version_family"`
		ScopeKind              string       `json:"scope_kind"`
		ScopeKey               string       `json:"scope_key"`
		ScopeRevision          int64        `json:"scope_revision"`
		CurrentRootVersionRefs []VersionRef `json:"current_root_version_refs"`
	}{
		WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID,
		OwnerKind: scope.OwnerKind, VersionFamily: scope.VersionFamily,
		ScopeKind: scope.ScopeKind, ScopeKey: scope.ScopeKey, ScopeRevision: scope.ScopeRevision,
		CurrentRootVersionRefs: ordered,
	})
	if err != nil {
		return Ref{}, err
	}
	membersHash, err := hash(ordered)
	if err != nil {
		return Ref{}, err
	}
	value := Ref{
		WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID,
		OwnerKind: scope.OwnerKind, VersionFamily: scope.VersionFamily,
		ScopeKind: scope.ScopeKind, ScopeKey: scope.ScopeKey, ScopeRevision: scope.ScopeRevision,
		ScopeContentHash: scopeContentHash, Members: ordered, MemberCount: int64(len(ordered)), MembersHash: membersHash,
	}
	value.CollectionRootHash, err = hash(struct {
		WorkspaceID      string       `json:"workspace_id"`
		ProjectID        string       `json:"project_id"`
		OwnerKind        string       `json:"owner_kind"`
		VersionFamily    string       `json:"version_family"`
		ScopeKind        string       `json:"scope_kind"`
		ScopeKey         string       `json:"scope_key"`
		ScopeRevision    int64        `json:"scope_revision"`
		ScopeContentHash string       `json:"scope_content_hash"`
		Members          []VersionRef `json:"members"`
		MemberCount      int64        `json:"member_count"`
		MembersHash      string       `json:"members_hash"`
	}{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		OwnerKind: value.OwnerKind, VersionFamily: value.VersionFamily,
		ScopeKind: value.ScopeKind, ScopeKey: value.ScopeKey, ScopeRevision: value.ScopeRevision,
		ScopeContentHash: value.ScopeContentHash, Members: value.Members,
		MemberCount: value.MemberCount, MembersHash: value.MembersHash,
	})
	if err != nil {
		return Ref{}, err
	}
	return value, nil
}

func validateScope(value Scope) error {
	if !validUUID(value.WorkspaceID) || !validUUID(value.ProjectID) ||
		!validText(value.OwnerKind) || !validText(value.VersionFamily) || !validText(value.ScopeKey) ||
		!slices.Contains([]string{"project", "episode", "scene", "reference_plan"}, value.ScopeKind) ||
		value.ScopeRevision < 1 || value.ScopeRevision > maximumSafeInteger {
		return errors.New("invalid Owner Collection scope")
	}
	return nil
}

func validateMember(scope Scope, value VersionRef) error {
	if value.WorkspaceID != scope.WorkspaceID || value.ProjectID != scope.ProjectID ||
		value.OwnerKind != scope.OwnerKind || value.VersionFamily != scope.VersionFamily ||
		!validText(value.OwnerLogicalID) || !validUUID(value.OwnerVersionID) ||
		value.OwnerRevision < 1 || value.OwnerRevision > maximumSafeInteger ||
		!hashPattern.MatchString(value.OwnerContentHash) {
		return errors.New("invalid Owner Collection member")
	}
	return nil
}

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func validText(value string) bool {
	return strings.TrimSpace(value) != "" && norm.NFC.IsNormalString(value)
}

func memberKey(value VersionRef) string {
	return value.OwnerKind + "\x00" + value.VersionFamily + "\x00" + value.OwnerLogicalID + "\x00" + value.OwnerVersionID
}

func hash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return canonical.Hash(encoded)
}
