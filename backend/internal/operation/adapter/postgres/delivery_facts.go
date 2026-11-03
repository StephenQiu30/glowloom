package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// readDeliveryReady checks the complete original result set. Supplier manifests
// define actual outputs; planned output_count is only used for legacy results.
func readDeliveryReady(tx *gorm.DB, projectID, id uuid.UUID, status domain.Status) (ready, conflict bool, err error) {
	if !status.IsTerminal() {
		return false, false, nil
	}
	var op struct {
		OutputCount                int
		Model, Price               *uuid.UUID
		RequestKey                 string
		Result                     []byte
		ReusedFromID               *uuid.UUID
		InputHash                  string
		QuoteMicros, SettledMicros *int64
		ReuseCompleted             bool
		ManagedBinding             bool
		ProviderCalls              int64
	}
	if err = tx.Raw(`SELECT o.output_count,o.model_profile_version_id AS model,o.price_rule_version_id AS price,o.provider_request_key AS request_key,p.result,o.reused_from_id,o.input_hash,o.quote_micros,o.settled_micros,
 EXISTS(SELECT 1 FROM operation.provider_binding b WHERE b.operation_id=o.id) AS managed_binding,
 EXISTS(SELECT 1 FROM operation.operation_event e WHERE e.operation_id=o.id AND e.reason='reused_result' AND e.to_status='completed' AND NOT e.is_delete) AS reuse_completed,
 (SELECT count(*) FROM operation.provider_call c WHERE c.operation_id=o.id AND c.project_id=o.project_id AND NOT c.is_delete) AS provider_calls FROM operation.operation o LEFT JOIN operation.provider_result p ON p.operation_id=o.id WHERE o.id=? AND o.project_id=? AND NOT o.is_delete`, id, projectID).Scan(&op).Error; err != nil {
		return false, false, fmt.Errorf("read original delivery manifest: %w", err)
	}
	var manifest *application.ManagedResult
	expected := op.OutputCount
	if len(op.Result) > 0 {
		manifest = &application.ManagedResult{}
		if json.Unmarshal(op.Result, manifest) != nil || manifest.Validate() != nil || op.Model == nil || op.Price == nil || manifest.Identity != (application.ProviderDispatchIdentity{ProjectID: projectID, OperationID: id, ModelProfileVersionID: *op.Model, PriceRuleVersionID: *op.Price, RequestKey: op.RequestKey, Action: "submit", Attempt: 1}) {
			return false, true, nil
		}
		if manifest.State != "succeeded" {
			return false, false, nil
		}
		expected = len(manifest.Artifacts)
		if manifest.Text != "" {
			expected = 1
		}
	}
	if expected < 1 {
		return false, false, nil
	}
	outputs, err := readDeliveryOutputs(tx, projectID, id)
	if err != nil {
		return false, false, err
	}
	if manifest == nil && op.ReusedFromID != nil {
		if status != domain.StatusCompleted || !op.ReuseCompleted || op.QuoteMicros == nil || *op.QuoteMicros != 0 || op.SettledMicros == nil || *op.SettledMicros != 0 || op.ProviderCalls != 0 {
			return false, true, nil
		}
		ready, err := readReusedDeliveryReady(tx, reuseSourceIdentity{ID: id, ProjectID: projectID, SourceID: *op.ReusedFromID, InputHash: op.InputHash, OutputCount: int32(op.OutputCount)}, outputs)
		return ready, !ready, err
	}
	if manifest == nil {
		// This immutable binding is written only for managed-cloud execution.
		// A missing manifest cannot turn that task into a legacy planned set;
		// current catalog configuration or creation time is not historical proof.
		if op.ManagedBinding {
			return false, false, nil
		}
		base := 0
		if len(outputs) > 0 {
			base = outputs[0].Sequence
		}
		return validLegacyDelivery(outputs, expected, base, false), false, nil
	}
	if op.ReusedFromID != nil {
		return false, true, nil
	}
	if len(outputs) != expected {
		return false, false, nil
	}
	for index, out := range outputs {
		if out.Sequence != index+1 || out.Deleted || out.Moderation != "passed" {
			return false, false, nil
		}
		if manifest != nil && manifest.Text != "" {
			var value struct {
				Text string `json:"text"`
			}
			if out.Kind != "json" || json.Unmarshal(out.Payload, &value) != nil || value.Text != manifest.Text {
				return false, false, nil
			}
			continue
		}
		if out.Kind == "media" {
			if out.Unreadable || out.AssetID == nil || out.AssetDeleted == nil || *out.AssetDeleted || out.AssetStatus == nil || *out.AssetStatus != "ready" || out.AssetModeration == nil || *out.AssetModeration != "passed" || out.Source == nil || *out.Source != id {
				return false, false, nil
			}
			if manifest != nil {
				artifact := manifest.Artifacts[index]
				if out.Size == nil || *out.Size != artifact.Size || out.MIME == nil || *out.MIME != artifact.MIME || out.SHA == nil || *out.SHA != artifact.SHA256 {
					return false, false, nil
				}
			}
		} else if manifest != nil {
			return false, false, nil
		}
	}
	return true, false, nil
}

type deliveryOutput struct {
	Sequence                     int
	Kind, Moderation             string
	Deleted                      bool
	Payload                      []byte
	AssetID                      *uuid.UUID
	BoundLocationID              *uuid.UUID
	ObjectKey                    string
	Unreadable                   bool
	AssetDeleted                 *bool
	AssetStatus, AssetModeration *string
	MIME, SHA                    *string
	Size                         *int64
	Source                       *uuid.UUID
	ContainsRealPerson           bool
	ConsentRecordID              *uuid.UUID
}

func readDeliveryOutputs(tx *gorm.DB, projectID, id uuid.UUID) ([]deliveryOutput, error) {
	var outputs []deliveryOutput
	err := tx.Raw(`SELECT o.seq_no AS sequence,o.kind,o.moderation_status AS moderation,o.is_delete AS deleted,o.json_payload AS payload,
 a.id AS asset_id,(to_jsonb(a)->>'object_location_id')::uuid AS bound_location_id,a.object_key,a.is_delete AS asset_deleted,a.status AS asset_status,a.moderation_status AS asset_moderation,a.mime_type AS mime,a.sha256 AS sha,a.byte_size AS size,a.source_operation_id AS source,a.contains_real_person,a.consent_record_id
 FROM operation.operation_output o LEFT JOIN media.media_asset a ON a.id=o.media_asset_id AND a.project_id=o.project_id WHERE o.operation_id=? AND o.project_id=? ORDER BY o.seq_no`, id, projectID).Scan(&outputs).Error
	if err != nil {
		return nil, fmt.Errorf("read original delivery outputs: %w", err)
	}
	// Legacy archives have no physical binding. If a binding exists, resolve
	// its exact identity just as a media read would, without object I/O.
	var ids []uuid.UUID
	for _, out := range outputs {
		if out.BoundLocationID != nil {
			ids = append(ids, *out.BoundLocationID)
		}
	}
	if len(ids) == 0 {
		return outputs, nil
	}
	var locations []struct {
		ID                                  uuid.UUID
		ProfileID, Bucket, ObjectKey, State string
	}
	if err := tx.Raw(`SELECT id,profile_id,bucket,object_key,state FROM media.object_location WHERE id IN ?`, ids).Scan(&locations).Error; err != nil {
		return nil, fmt.Errorf("read archived object locations: %w", err)
	}
	byID := make(map[uuid.UUID]*media.ObjectLocation, len(locations))
	for _, l := range locations {
		byID[l.ID] = &media.ObjectLocation{ID: l.ID, State: l.State, Ref: media.ObjectRef{ProfileID: l.ProfileID, Bucket: l.Bucket, Key: l.ObjectKey}}
	}
	for index, out := range outputs {
		if out.BoundLocationID != nil {
			outputs[index].Unreadable = !byID[*out.BoundLocationID].Readable(out.ObjectKey)
		}
	}
	return outputs, nil
}

// Legacy archives accept their established complete zero- or one-based set.
// Reuse only accepts the zero-based set validated by lockReusableSourceOutputs.
// The project-owned output link is authority for historical assets; an absent
// source_operation_id is valid under that established contract.
func validLegacyDelivery(outputs []deliveryOutput, expected, base int, reusable bool) bool {
	if expected < 1 || len(outputs) != expected || (base != 0 && base != 1) || (reusable && base != 0) {
		return false
	}
	for index, out := range outputs {
		if out.Sequence != index+base || out.Deleted || !reusableModeration(out.Moderation) {
			return false
		}
		switch out.Kind {
		case "json":
			if len(out.Payload) == 0 || out.AssetID != nil {
				return false
			}
		case "media":
			if out.Unreadable || out.AssetID == nil || out.AssetDeleted == nil || *out.AssetDeleted || out.AssetStatus == nil || *out.AssetStatus != "ready" || out.AssetModeration == nil || !reusableModeration(*out.AssetModeration) || len(out.Payload) != 0 {
				return false
			}
			if out.ContainsRealPerson || out.ConsentRecordID != nil {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Completion records independent copied outputs. The source's frozen identity
// and exact payload/asset relationship remain evidence, while hiding its history
// or disabling an old catalog does not revoke the completed copy's access.
func readReusedDeliveryReady(tx *gorm.DB, identity reuseSourceIdentity, outputs []deliveryOutput) (bool, error) {
	if identity.ID == identity.SourceID || identity.InputHash == "" || identity.OutputCount < 1 || !validLegacyDelivery(outputs, int(identity.OutputCount), 0, true) {
		return false, nil
	}
	var source struct{ OutputCount int32 }
	result := tx.Raw(`SELECT output_count FROM operation.operation WHERE id=? AND project_id=? AND input_hash=? AND origin<>'agent' AND status='completed'`, identity.SourceID, identity.ProjectID, identity.InputHash).Scan(&source)
	if result.Error != nil {
		return false, fmt.Errorf("read frozen reuse source: %w", result.Error)
	}
	if result.RowsAffected != 1 || source.OutputCount != identity.OutputCount {
		return false, nil
	}
	original, err := readDeliveryOutputs(tx, identity.ProjectID, identity.SourceID)
	if err != nil {
		return false, err
	}
	if !validLegacyDelivery(original, int(identity.OutputCount), 0, true) {
		return false, nil
	}
	for index, out := range outputs {
		previous := original[index]
		if out.Sequence != previous.Sequence || out.Kind != previous.Kind {
			return false, nil
		}
		if out.Kind == "media" {
			if *out.AssetID != *previous.AssetID {
				return false, nil
			}
		} else if !jsonEqual(out.Payload, previous.Payload) {
			return false, nil
		}
	}
	return true, nil
}
