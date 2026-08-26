package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const OwnerSnapshotOriginConfirmed = "confirmed_owner_facts"

type OwnerHeadRef struct {
	OwnerKind      string `json:"owner_kind"`
	OwnerLogicalID string `json:"owner_logical_id"`
	OwnerVersionID string `json:"owner_version_id"`
	OwnerRevision  int64  `json:"owner_revision"`
	ContentHash    string `json:"content_hash"`
}

type OwnerSnapshot struct {
	Origin             string         `json:"origin"`
	WorkspaceID        string         `json:"workspace_id"`
	ProjectID          string         `json:"project_id"`
	SourceRevisionID   string         `json:"source_revision_id"`
	SourceRevisionHash string         `json:"source_revision_hash"`
	OwnerHeads         []OwnerHeadRef `json:"owner_heads"`
	Graph              Snapshot       `json:"graph"`
}

type CompiledOwnerSnapshot struct {
	WorkspaceID, ProjectID, SourceRevisionID, SourceRevisionHash string
	OwnerHeads                                                   []OwnerHeadRef
	OwnerSetHash                                                 string
	Graph                                                        CanonicalSnapshot
}

func OwnerHeadRefFrom(owner OwnerRef) OwnerHeadRef {
	return OwnerHeadRef{
		OwnerKind: owner.OwnerKind, OwnerLogicalID: owner.OwnerLogicalID,
		OwnerVersionID: owner.OwnerVersionID, OwnerRevision: owner.OwnerRevision,
		ContentHash: owner.ContentHash,
	}
}

func CompileOwnerSnapshot(snapshot OwnerSnapshot) (CompiledOwnerSnapshot, error) {
	if snapshot.Origin != OwnerSnapshotOriginConfirmed {
		return CompiledOwnerSnapshot{}, errors.New("StoryGraph compiler only accepts confirmed Owner facts")
	}
	if _, err := uuid.Parse(snapshot.WorkspaceID); err != nil {
		return CompiledOwnerSnapshot{}, errors.New("invalid StoryGraph workspace")
	}
	if _, err := uuid.Parse(snapshot.ProjectID); err != nil {
		return CompiledOwnerSnapshot{}, errors.New("invalid StoryGraph project")
	}
	if _, err := uuid.Parse(snapshot.SourceRevisionID); err != nil || !hashPattern.MatchString(snapshot.SourceRevisionHash) {
		return CompiledOwnerSnapshot{}, errors.New("invalid StoryGraph source revision")
	}
	heads, ownerSetHash, err := CanonicalOwnerHeadRefs(snapshot.OwnerHeads)
	if err != nil {
		return CompiledOwnerSnapshot{}, err
	}
	headByLogicalOwner := make(map[string]OwnerHeadRef, len(heads))
	usedHeads := make(map[string]struct{}, len(heads))
	foundSource := false
	for _, head := range heads {
		key := ownerHeadKey(head.OwnerKind, head.OwnerLogicalID)
		headByLogicalOwner[key] = head
		if head.OwnerKind == "production/script" && head.OwnerVersionID == snapshot.SourceRevisionID && head.ContentHash == snapshot.SourceRevisionHash {
			foundSource = true
		}
	}
	if !foundSource {
		return CompiledOwnerSnapshot{}, errors.New("source revision is outside the frozen Owner Set")
	}
	for _, node := range snapshot.Graph.Nodes {
		key := ownerHeadKey(node.OwnerRef.OwnerKind, node.OwnerRef.OwnerLogicalID)
		head, ok := headByLogicalOwner[key]
		if !ok || head.OwnerVersionID != node.OwnerRef.OwnerVersionID || head.OwnerRevision != node.OwnerRef.OwnerRevision || head.ContentHash != node.OwnerRef.ContentHash {
			return CompiledOwnerSnapshot{}, fmt.Errorf("StoryGraph node %s is outside the frozen Owner Set", node.StoryNodeKey)
		}
		usedHeads[key] = struct{}{}
		if nodeRequiresEvidence(node.NodeType) && len(node.EvidenceRefs) == 0 {
			return CompiledOwnerSnapshot{}, fmt.Errorf("StoryGraph node %s requires source evidence", node.StoryNodeKey)
		}
	}
	if len(usedHeads) != len(heads) {
		return CompiledOwnerSnapshot{}, errors.New("frozen Owner Set contains an unprojected Owner")
	}
	graph, err := Canonicalize(snapshot.Graph)
	if err != nil {
		return CompiledOwnerSnapshot{}, err
	}
	return CompiledOwnerSnapshot{
		WorkspaceID: snapshot.WorkspaceID, ProjectID: snapshot.ProjectID,
		SourceRevisionID: snapshot.SourceRevisionID, SourceRevisionHash: snapshot.SourceRevisionHash,
		OwnerHeads: heads, OwnerSetHash: ownerSetHash, Graph: graph,
	}, nil
}

func CanonicalOwnerHeadRefs(values []OwnerHeadRef) ([]OwnerHeadRef, string, error) {
	if len(values) == 0 {
		return nil, "", errors.New("StoryGraph Owner Set must not be empty")
	}
	result := append(make([]OwnerHeadRef, 0, len(values)), values...)
	for _, value := range result {
		if !knownOwnerKind(value.OwnerKind) || strings.TrimSpace(value.OwnerLogicalID) == "" || value.OwnerRevision < 1 || !hashPattern.MatchString(value.ContentHash) {
			return nil, "", errors.New("invalid StoryGraph Owner Head")
		}
		if _, err := uuid.Parse(value.OwnerVersionID); err != nil {
			return nil, "", errors.New("invalid StoryGraph Owner Head")
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return ownerHeadKey(result[i].OwnerKind, result[i].OwnerLogicalID) < ownerHeadKey(result[j].OwnerKind, result[j].OwnerLogicalID)
	})
	for index := 1; index < len(result); index++ {
		if ownerHeadKey(result[index-1].OwnerKind, result[index-1].OwnerLogicalID) == ownerHeadKey(result[index].OwnerKind, result[index].OwnerLogicalID) {
			return nil, "", errors.New("duplicate StoryGraph Owner Head")
		}
	}
	hash, err := canonicalValueHash(result)
	return result, hash, err
}

func HashCanonicalValue(value any) (string, error) {
	return canonicalValueHash(value)
}

func knownOwnerKind(value string) bool {
	for _, owner := range nodeOwners {
		if owner == value {
			return true
		}
	}
	return false
}

func ownerHeadKey(kind, logicalID string) string {
	return kind + "\x00" + logicalID
}

func nodeRequiresEvidence(nodeType NodeType) bool {
	switch nodeType {
	case NodeTypeScene, NodeTypeDialogue, NodeTypeNarrativeBeat, NodeTypeOccurrence,
		NodeTypeRelationshipClaim, NodeTypeForeshadowingClaim, NodeTypePayoffClaim,
		NodeTypeContinuityClaim, NodeTypeCausalClaim, NodeTypeShot, NodeTypeShotContinuityClaim:
		return true
	default:
		return false
	}
}
