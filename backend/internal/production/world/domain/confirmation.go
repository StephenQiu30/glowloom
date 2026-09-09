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

const (
	ProductionWorldCheckpointKey       = "gate_2_bible_continuity"
	ConfirmProductionWorldContract     = "confirm_production_world"
	ConfirmProductionWorldOperation    = "production_world.confirm"
	ProductionWorldConfirmedEvent      = "ProductionWorldConfirmed"
	PlanningStructureRebaseFamily      = "planning_structure_rebase_set"
	ProductionWorldCommandResultSchema = "production-world-confirmation-result-production"
)

type ReviewDecisionAuditRef struct {
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash"`
}

type CollectionMemberRef struct {
	OwnerKind   string `json:"owner_kind"`
	LogicalID   string `json:"logical_id"`
	VersionID   string `json:"version_id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash"`
}

type CollectionCommitReceipt struct {
	ID                        string                 `json:"collection_receipt_id"`
	CommandID                 string                 `json:"command_id"`
	IdempotencyKey            string                 `json:"idempotency_key"`
	WorkspaceID               string                 `json:"workspace_id"`
	ProjectID                 string                 `json:"project_id"`
	DecisionCheckpointID      string                 `json:"decision_checkpoint_id"`
	OwnerKind                 string                 `json:"owner_kind"`
	VersionFamily             string                 `json:"version_family"`
	ScopeKind                 string                 `json:"scope_kind"`
	ScopeKey                  string                 `json:"scope_key"`
	ScopeRevision             int64                  `json:"scope_revision"`
	ScopeContentHash          string                 `json:"scope_content_hash"`
	Members                   []CollectionMemberRef  `json:"members"`
	MemberCount               int                    `json:"member_count"`
	MembersHash               string                 `json:"members_hash"`
	CollectionRootHash        string                 `json:"collection_root_hash"`
	CoveredScopeKeys          []string               `json:"covered_scope_keys"`
	CommittedOwnerVersionRefs []CollectionMemberRef  `json:"committed_owner_version_refs"`
	ReviewDecisionAuditRef    ReviewDecisionAuditRef `json:"review_decision_audit_ref"`
	ReceiptContentHash        string                 `json:"receipt_content_hash"`
	CommittedAt               time.Time              `json:"committed_at"`
	CommittedBy               string                 `json:"committed_by"`
}

type CollectionCommitReceiptInput struct {
	ID, CommandID, IdempotencyKey, WorkspaceID, ProjectID string
	DecisionCheckpointID, OwnerKind, VersionFamily        string
	ScopeKind, ScopeKey                                   string
	ScopeRevision                                         int64
	ScopeContentHash, MembersHash, CollectionRootHash     string
	Members, CommittedOwnerVersionRefs                    []CollectionMemberRef
	CoveredScopeKeys                                      []string
	ReviewDecisionAuditRef                                ReviewDecisionAuditRef
	CommittedAt                                           time.Time
	CommittedBy                                           string
}

func NewCollectionCommitReceipt(input CollectionCommitReceiptInput) (CollectionCommitReceipt, error) {
	for _, value := range []string{input.ID, input.CommandID, input.WorkspaceID, input.ProjectID,
		input.DecisionCheckpointID, input.ReviewDecisionAuditRef.ID, input.CommittedBy} {
		if _, err := uuid.Parse(value); err != nil {
			return CollectionCommitReceipt{}, errors.New("invalid Production World collection receipt identity")
		}
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" || input.OwnerKind == "" || input.VersionFamily == "" ||
		input.ScopeKind == "" || input.ScopeKey == "" || input.ScopeRevision < 1 || input.CommittedAt.IsZero() ||
		!productionWorldContentHashPattern.MatchString(input.ScopeContentHash) ||
		!productionWorldContentHashPattern.MatchString(input.MembersHash) ||
		!productionWorldContentHashPattern.MatchString(input.CollectionRootHash) ||
		!productionWorldContentHashPattern.MatchString(input.ReviewDecisionAuditRef.ContentHash) ||
		input.ReviewDecisionAuditRef.Revision < 1 || len(input.Members) == 0 || len(input.CoveredScopeKeys) == 0 {
		return CollectionCommitReceipt{}, errors.New("invalid Production World collection receipt")
	}
	members := append([]CollectionMemberRef(nil), input.Members...)
	refs := append([]CollectionMemberRef(nil), input.CommittedOwnerVersionRefs...)
	scopes := append([]string(nil), input.CoveredScopeKeys...)
	if !canonicalCollectionMemberRefs(members) || !canonicalCollectionMemberRefs(refs) ||
		!slices.IsSorted(scopes) || !uniqueNonempty(scopes) {
		return CollectionCommitReceipt{}, errors.New("Production World collection receipt set is not canonical")
	}
	value := CollectionCommitReceipt{
		ID: input.ID, CommandID: input.CommandID, IdempotencyKey: input.IdempotencyKey,
		WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID,
		DecisionCheckpointID: input.DecisionCheckpointID, OwnerKind: input.OwnerKind,
		VersionFamily: input.VersionFamily, ScopeKind: input.ScopeKind, ScopeKey: input.ScopeKey,
		ScopeRevision: input.ScopeRevision, ScopeContentHash: input.ScopeContentHash,
		Members: members, MemberCount: len(members), MembersHash: input.MembersHash,
		CollectionRootHash: input.CollectionRootHash, CoveredScopeKeys: scopes,
		CommittedOwnerVersionRefs: refs, ReviewDecisionAuditRef: input.ReviewDecisionAuditRef,
		CommittedAt: input.CommittedAt.UTC(), CommittedBy: input.CommittedBy,
	}
	var err error
	value.ReceiptContentHash, err = confirmationHash(collectionReceiptHashMaterial(value))
	return value, err
}

type CollectionReceiptRef struct {
	ID                 string `json:"collection_receipt_id"`
	OwnerKind          string `json:"owner_kind"`
	VersionFamily      string `json:"version_family"`
	ScopeKind          string `json:"scope_kind"`
	ScopeKey           string `json:"scope_key"`
	CollectionRootHash string `json:"collection_root_hash"`
	ReceiptContentHash string `json:"receipt_content_hash"`
}

type DomainEventDescriptor struct {
	EventType            string              `json:"event_type"`
	AggregateOwnerRef    CollectionMemberRef `json:"aggregate_owner_ref"`
	AggregateRevision    int64               `json:"aggregate_revision"`
	AggregateContentHash string              `json:"aggregate_content_hash"`
	PayloadContractID    string              `json:"payload_contract_id"`
	PayloadContentHash   string              `json:"payload_content_hash"`
}

type ConfirmProductionWorldResult struct {
	SchemaVersion                string                  `json:"schema_version"`
	CommandID                    string                  `json:"command_id"`
	CommandContractID            string                  `json:"command_contract_id"`
	CommandContentHash           string                  `json:"command_content_hash"`
	CommandReceiptID             string                  `json:"command_receipt_id"`
	CoordinatorKind              string                  `json:"coordinator_kind"`
	SubjectRef                   CollectionMemberRef     `json:"subject_ref"`
	OrderedCollectionReceiptRefs []CollectionReceiptRef  `json:"ordered_collection_receipt_refs"`
	CommittedOwnerVersionRefs    []CollectionMemberRef   `json:"committed_owner_version_refs"`
	DomainEventDescriptors       []DomainEventDescriptor `json:"domain_event_descriptors"`
	ResultContentHash            string                  `json:"result_content_hash"`
	ReceiptContentHash           string                  `json:"receipt_content_hash"`
	CommittedAt                  time.Time               `json:"committed_at"`
	CommittedBy                  string                  `json:"committed_by"`
}

func CompleteConfirmProductionWorldResult(value ConfirmProductionWorldResult) (ConfirmProductionWorldResult, error) {
	for _, identifier := range []string{value.CommandID, value.CommandReceiptID, value.SubjectRef.VersionID, value.CommittedBy} {
		if _, err := uuid.Parse(identifier); err != nil {
			return ConfirmProductionWorldResult{}, errors.New("invalid Production World command result identity")
		}
	}
	if value.SchemaVersion != ProductionWorldCommandResultSchema || value.CommandContractID != ConfirmProductionWorldContract ||
		value.CoordinatorKind != "production/world" || value.CommittedAt.IsZero() ||
		!productionWorldContentHashPattern.MatchString(value.CommandContentHash) ||
		!canonicalCollectionMemberRefs(value.CommittedOwnerVersionRefs) || len(value.OrderedCollectionReceiptRefs) == 0 ||
		len(value.DomainEventDescriptors) != 1 {
		return ConfirmProductionWorldResult{}, errors.New("invalid Production World command result")
	}
	resultHash, err := confirmationHash(struct {
		CollectionReceipts []CollectionReceiptRef  `json:"collection_receipts"`
		OwnerVersions      []CollectionMemberRef   `json:"owner_versions"`
		Events             []DomainEventDescriptor `json:"events"`
	}{value.OrderedCollectionReceiptRefs, value.CommittedOwnerVersionRefs, value.DomainEventDescriptors})
	if err != nil {
		return ConfirmProductionWorldResult{}, err
	}
	value.ResultContentHash = resultHash
	value.ReceiptContentHash, err = confirmationHash(struct {
		Schema, CommandReceiptID, CommandID, CommandContractID, CommandContentHash, CoordinatorKind string
		SubjectRef                                                                                  CollectionMemberRef
		ResultContentHash                                                                           string
	}{"production-world-command-receipt", value.CommandReceiptID, value.CommandID, value.CommandContractID,
		value.CommandContentHash, value.CoordinatorKind, value.SubjectRef, resultHash})
	return value, err
}

func collectionReceiptHashMaterial(value CollectionCommitReceipt) any {
	return struct {
		Schema                                                                                 string
		ID, CommandID, IdempotencyKey, WorkspaceID, ProjectID, DecisionCheckpointID            string
		OwnerKind, VersionFamily, ScopeKind, ScopeKey, ScopeContentHash, MembersHash, RootHash string
		ScopeRevision, MemberCount                                                             int64
		Members, OwnerRefs                                                                     []CollectionMemberRef
		CoveredScopeKeys                                                                       []string
		ReviewDecision                                                                         ReviewDecisionAuditRef
	}{"production-world-collection-receipt", value.ID, value.CommandID, value.IdempotencyKey,
		value.WorkspaceID, value.ProjectID, value.DecisionCheckpointID, value.OwnerKind,
		value.VersionFamily, value.ScopeKind, value.ScopeKey, value.ScopeContentHash,
		value.MembersHash, value.CollectionRootHash, value.ScopeRevision, int64(value.MemberCount),
		value.Members, value.CommittedOwnerVersionRefs, value.CoveredScopeKeys, value.ReviewDecisionAuditRef}
}

func canonicalCollectionMemberRefs(values []CollectionMemberRef) bool {
	if len(values) == 0 {
		return false
	}
	previous := ""
	for _, value := range values {
		if _, err := uuid.Parse(value.VersionID); err != nil {
			return false
		}
		key := value.OwnerKind + "\x00" + value.LogicalID + "\x00" + value.VersionID
		if value.OwnerKind == "" || value.LogicalID == "" || key <= previous || value.Revision < 1 ||
			!productionWorldContentHashPattern.MatchString(value.ContentHash) {
			return false
		}
		previous = key
	}
	return true
}

func confirmationHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return canonical.Hash(raw)
}
