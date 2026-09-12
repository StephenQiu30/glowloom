package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/google/uuid"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformowner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

const (
	ConfirmVisualFoundationOperation = "visual_foundation_reference.confirm"
	VisualFoundationConfirmedEvent   = "VisualFoundationReferenceConfirmed"
	VisualFoundationCheckpointKey    = "gate_3_visual_foundation_scope"
)

type VisualScopeCollectionReceipt struct {
	ID                 string            `json:"id"`
	CommandID          string            `json:"command_id"`
	WorkspaceID        string            `json:"workspace_id"`
	ProjectID          string            `json:"project_id"`
	GateInputID        string            `json:"gate_input_id"`
	ReviewDecisionID   string            `json:"review_decision_id"`
	CheckpointKey      string            `json:"checkpoint_key"`
	Collection         platformowner.Ref `json:"collection"`
	ReceiptContentHash string            `json:"receipt_content_hash"`
	CommittedBy        string            `json:"committed_by"`
	CommittedAt        time.Time         `json:"committed_at"`
}

type ConfirmVisualFoundationResult struct {
	CommandID                  string                       `json:"command_id"`
	CommandReceiptID           string                       `json:"command_receipt_id"`
	PresetCollectionReceipt    VisualScopeCollectionReceipt `json:"preset_collection_receipt"`
	ReferenceCollectionReceipt VisualScopeCollectionReceipt `json:"reference_collection_receipt"`
	PlanVersionID              string                       `json:"plan_version_id"`
	PlanRevision               int64                        `json:"plan_revision"`
	PlanContentHash            string                       `json:"plan_content_hash"`
	ResultContentHash          string                       `json:"result_content_hash"`
	ReceiptContentHash         string                       `json:"receipt_content_hash"`
	CommittedBy                string                       `json:"committed_by"`
	CommittedAt                time.Time                    `json:"committed_at"`
}

func NewVisualScopeCollectionReceipt(
	id, commandID, gateInputID, reviewDecisionID, committedBy string,
	collection platformowner.Ref,
	committedAt time.Time,
) (VisualScopeCollectionReceipt, error) {
	for _, identifier := range []string{
		id, commandID, gateInputID, reviewDecisionID, committedBy,
		collection.WorkspaceID, collection.ProjectID,
	} {
		if parsed, err := uuid.Parse(identifier); err != nil || parsed == uuid.Nil {
			return VisualScopeCollectionReceipt{}, errors.New("invalid Visual Foundation collection receipt identity")
		}
	}
	if committedAt.IsZero() || committedAt.Location() != time.UTC ||
		!((collection.OwnerKind == "preset" && collection.VersionFamily == "preset_effective_set") ||
			(collection.OwnerKind == "production/reference" && collection.VersionFamily == "reference_plan_set")) {
		return VisualScopeCollectionReceipt{}, errors.New("invalid Visual Foundation collection receipt")
	}
	rebuilt, err := platformowner.Build(platformowner.Scope{
		WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID,
		OwnerKind: collection.OwnerKind, VersionFamily: collection.VersionFamily,
		ScopeKind: collection.ScopeKind, ScopeKey: collection.ScopeKey, ScopeRevision: collection.ScopeRevision,
	}, collection.Members)
	if err != nil || !reflect.DeepEqual(rebuilt, collection) {
		return VisualScopeCollectionReceipt{}, errors.New("Visual Foundation collection has drifted")
	}
	value := VisualScopeCollectionReceipt{
		ID: id, CommandID: commandID, WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID,
		GateInputID: gateInputID, ReviewDecisionID: reviewDecisionID,
		CheckpointKey: VisualFoundationCheckpointKey, Collection: collection,
		CommittedBy: committedBy, CommittedAt: committedAt,
	}
	value.ReceiptContentHash, err = visualReferenceHash(value)
	return value, err
}

func CompleteConfirmVisualFoundationResult(value ConfirmVisualFoundationResult) (ConfirmVisualFoundationResult, error) {
	for _, identifier := range []string{
		value.CommandID, value.CommandReceiptID, value.PlanVersionID, value.CommittedBy,
	} {
		if parsed, err := uuid.Parse(identifier); err != nil || parsed == uuid.Nil {
			return ConfirmVisualFoundationResult{}, errors.New("invalid Visual Foundation confirmation result identity")
		}
	}
	if value.PlanRevision < 1 || !referencePlanHashPattern.MatchString(value.PlanContentHash) ||
		value.CommittedAt.IsZero() || value.CommittedAt.Location() != time.UTC ||
		value.PresetCollectionReceipt.CommandID != value.CommandID ||
		value.ReferenceCollectionReceipt.CommandID != value.CommandID ||
		value.PresetCollectionReceipt.CommittedBy != value.CommittedBy ||
		value.ReferenceCollectionReceipt.CommittedBy != value.CommittedBy {
		return ConfirmVisualFoundationResult{}, errors.New("invalid Visual Foundation confirmation result")
	}
	var err error
	value.ResultContentHash, err = visualReferenceHash(struct {
		PresetReceiptHash, ReferenceReceiptHash string
		PlanVersionID                           string
		PlanRevision                            int64
		PlanContentHash                         string
	}{
		value.PresetCollectionReceipt.ReceiptContentHash,
		value.ReferenceCollectionReceipt.ReceiptContentHash,
		value.PlanVersionID, value.PlanRevision, value.PlanContentHash,
	})
	if err != nil {
		return ConfirmVisualFoundationResult{}, err
	}
	value.ReceiptContentHash, err = visualReferenceHash(struct {
		CommandID, CommandReceiptID, ResultContentHash string
	}{value.CommandID, value.CommandReceiptID, value.ResultContentHash})
	return value, err
}

func visualReferenceHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}
