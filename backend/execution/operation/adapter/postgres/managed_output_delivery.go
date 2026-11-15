package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ReadOutputManifest reads the original durable provider evidence, never a supplier
// URL, another owner's asset row or a replacement provider request.
func (s *OutputDeliveryStore) ReadOutputManifest(ctx context.Context, operation uuid.UUID) (application.ManagedResult, error) {
	var result application.ManagedResult
	if err := s.ready(ctx); err != nil {
		return result, err
	}
	var row struct{ Result json.RawMessage }
	r := s.db.WithContext(ctx).Raw(`SELECT pr.result FROM operation.provider_result pr JOIN operation.operation op ON op.id=pr.operation_id WHERE op.id=? AND op.status IN ('succeeded','ingesting') AND NOT op.is_delete`, operation).Scan(&row)
	if r.Error != nil {
		return result, r.Error
	}
	if r.RowsAffected != 1 {
		return result, application.ErrProviderCallConflict
	}
	if json.Unmarshal(row.Result, &result) != nil || result.Validate() != nil || result.Identity.OperationID != operation || result.State != application.ManagedResultSucceeded {
		return result, application.ErrProviderCallConflict
	}
	return result, nil
}

// PrepareManagedOutputDelivery reconstructs private publication intent from
// authoritative local snapshots and original paid evidence, retaining the first
// full request across new worker epochs. No caller can choose its own scope,
// supplier cost, model, worker lease, output identity or required sequence set.
func (s *OutputDeliveryStore) PrepareManagedOutputDelivery(ctx context.Context, result application.ManagedResult, artifact application.ManagedArtifact) (application.OutputDeliveryRequest, error) {
	var req application.OutputDeliveryRequest
	if err := s.ready(ctx); err != nil {
		return req, err
	}
	if result.Validate() != nil || result.State != application.ManagedResultSucceeded || result.Text != "" {
		return req, application.ErrProviderCallConflict
	}
	if prior, err := s.ReadOutputDelivery(ctx, result.Identity.OperationID, artifact.Sequence); err != nil {
		return req, err
	} else if prior != nil {
		return prior.Request, nil
	}
	job, ok := taskruntime.CurrentJob(ctx)
	if !ok || job.BusinessID != result.Identity.OperationID.String() || (job.Kind != "operation.ingest" && job.Kind != "operation.run") {
		return req, taskruntime.ErrLeaseLost
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			Snapshot                                  json.RawMessage
			Params                                    json.RawMessage
			ProviderKey, ModelKey, Region, Moderation string
		}
		r := tx.Raw(`SELECT a.snapshot,op.params,p.key AS provider_key,m.model_key,op.region,v.moderation FROM operation.frozen_input_acceptance a JOIN operation.operation op ON op.id=a.operation_id JOIN catalog.model_profile_version v ON v.id=op.model_profile_version_id JOIN catalog.model_profile m ON m.id=v.model_profile_id JOIN catalog.provider p ON p.id=m.provider_id WHERE op.id=? AND op.project_id=? AND NOT op.is_delete FOR UPDATE OF op`, result.Identity.OperationID, result.Identity.ProjectID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || row.Moderation != "provider" {
			return application.ErrProviderCallConflict
		}
		var source struct {
			Command            writefence.Command `json:"command"`
			ScopeID, ProjectID uuid.UUID
			AspectRatio        string `json:"aspect_ratio"`
		}
		// UUID fields use explicit JSON names; metadata stays private immutable input.
		var snapshot map[string]json.RawMessage
		if json.Unmarshal(row.Snapshot, &snapshot) != nil || json.Unmarshal(snapshot["command"], &source.Command) != nil || json.Unmarshal(snapshot["scope_id"], &source.ScopeID) != nil || json.Unmarshal(snapshot["project_id"], &source.ProjectID) != nil || source.ProjectID != result.Identity.ProjectID {
			return application.ErrFrozenInputConflict
		}
		if value := snapshot["aspect_ratio"]; len(value) > 0 {
			if json.Unmarshal(value, &source.AspectRatio) != nil {
				return application.ErrFrozenInputConflict
			}
		}
		var params struct {
			AspectRatio string `json:"aspect_ratio"`
		}
		if json.Unmarshal(row.Params, &params) != nil {
			return application.ErrProviderCallConflict
		}
		if source.AspectRatio == "" {
			source.AspectRatio = params.AspectRatio
		} else if params.AspectRatio != "" && params.AspectRatio != source.AspectRatio {
			return application.ErrFrozenInputConflict
		}
		if source.AspectRatio != "16:9" && source.AspectRatio != "9:16" && source.AspectRatio != "1:1" {
			return application.ErrWorkflowInputNotReady
		}
		cost, err := NewStore(s.db).ProviderCostInTransaction(ctx, tx, result.Identity.OperationID)
		if err != nil {
			return err
		}
		manifest, err := outputEvidenceHash(outputDeliveryJSON(result))
		if err != nil {
			return err
		}
		usage, err := outputEvidenceHash(result.Usage)
		if err != nil {
			return err
		}
		supplier, err := outputEvidenceHash(result.SupplierUsage)
		if err != nil {
			return err
		}
		sequences := make([]int, len(result.Artifacts))
		found := false
		for i, a := range result.Artifacts {
			sequences[i] = a.Sequence
			if a == artifact {
				found = true
			}
		}
		if !found {
			return application.ErrProviderCallConflict
		}
		kind := ""
		switch {
		case strings.HasPrefix(artifact.MIME, "image/"):
			kind = "image"
		case strings.HasPrefix(artifact.MIME, "audio/"):
			kind = "audio"
		case artifact.MIME == "video/mp4":
			kind = "video"
		default:
			return application.ErrProviderCallConflict
		}
		output := contract.OperationOutputIdentity{OperationID: result.Identity.OperationID, OutputID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("operation-output/%s/%d", result.Identity.OperationID, artifact.Sequence))), ProjectID: result.Identity.ProjectID, ScopeID: source.ScopeID, Sequence: artifact.Sequence, Sequences: sequences, ByteSize: artifact.Size, SHA256: artifact.SHA256, MIMEType: artifact.MIME, Kind: kind, ProviderKey: row.ProviderKey, ModelKey: row.ModelKey, Region: row.Region, AspectRatio: source.AspectRatio, Provider: contract.OutputProviderEvidence{RequestKey: result.Identity.RequestKey, Attempt: result.Identity.Attempt, ManifestSHA256: manifest, UsageSHA256: usage, SupplierUsageSHA256: supplier, ActualCostMicros: cost.ActualCostMicros}, Worker: contract.OutputWorkerEvidence{Kind: job.Kind, TaskID: result.Identity.OperationID, Owner: job.Owner, Epoch: job.Epoch}}
		suffix := fmt.Sprintf("%d/%s", artifact.Sequence, artifact.SHA256)
		req = application.OutputDeliveryRequest{Command: writefence.Command{ID: uuid.NewSHA1(result.Identity.OperationID, []byte("execution.operation_output.adopt/"+suffix)), Action: "execution.operation_output.adopt", SHA256: strings.Repeat("0", 64), Authorities: source.Command.Authorities}, Publication: contract.Request{Command: writefence.Command{ID: uuid.NewSHA1(result.Identity.OperationID, []byte("media.operation_output.publish/"+suffix)), Action: "media.operation_output.publish", SHA256: output.Digest(), Authorities: source.Command.Authorities}, Output: output}}
		req.Command.SHA256 = req.Digest()
		if err := application.ValidateOutputDelivery(req); err != nil {
			return err
		}
		return verifyOutputDeliveryEvidence(ctx, tx, req)
	})
	if err != nil {
		return req, err
	}
	_, err = s.PrepareOutputDelivery(ctx, req)
	return req, err
}
