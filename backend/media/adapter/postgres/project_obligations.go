package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// ProjectWorkReader reads only Media's own committed responsibility tables.
// It never consults another owner's tables or turns missing evidence into idle.
type ProjectWorkReader struct{ db *gorm.DB }

// NewProjectWorkReader requires an independent restricted Media database pool.
func NewProjectWorkReader(db *gorm.DB) *ProjectWorkReader { return &ProjectWorkReader{db: db} }

// ReadProjectWork observes a consistent snapshot without changing any fence,
// command, task, activity, receipt or pin. A closure manifest page is not a grant.
func (s *ProjectWorkReader) ReadProjectWork(ctx context.Context, request application.ProjectWorkRequest) (application.ProjectWorkSnapshot, error) {
	var snapshot application.ProjectWorkSnapshot
	if err := request.Validate(); err != nil {
		return snapshot, err
	}
	if s == nil || s.db == nil || s.db.Statement == nil || s.db.Statement.ConnPool == nil {
		return snapshot, application.ErrUnavailable
	}
	if _, nested := s.db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return snapshot, writefence.ErrTransaction
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterMedia {
			return writefence.ErrInvalid
		}
		var head struct {
			Epoch   int64
			Blocked bool
		}
		read := tx.Table("infra.writer_fence").Select("epoch", "blocked").Where("service_owner = infra.runtime_owner() AND issuer = ? AND authority_kind = ? AND authority_id = ?", writefence.Creation, writefence.Project, request.ProjectID).Take(&head)
		if read.Error != nil {
			return read.Error
		}
		if head.Epoch != request.ProjectEpoch || head.Blocked {
			return writefence.ErrFenced
		}
		result := application.ProjectWorkSnapshot{Request: request, BusinessState: application.ProjectWorkIdle, Commands: make([]application.ProjectCommandObservation, 0, len(request.Commands))}
		busy, err := readProjectBusinessWork(tx, request)
		if err != nil {
			return err
		}
		if busy {
			result.BusinessState = application.ProjectWorkBusy
		} else {
			var held bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.reference_pin WHERE project_id=? AND state='held')`, request.ProjectID).Scan(&held).Error; err != nil {
				return err
			}
			if held {
				result.BusinessState = application.ProjectWorkUnknown
			}
		}
		for _, command := range request.Commands {
			observation := application.ProjectCommandObservation{Command: command, State: "unknown"}
			record, err := pgfence.New(tx).Load(ctx, command.ID)
			if errors.Is(err, writefence.ErrNotFound) {
				result.Commands = append(result.Commands, observation)
				continue
			}
			if err != nil {
				return err
			}
			want, err := command.AuthorityDigest()
			if err != nil {
				return err
			}
			got, err := record.Command.AuthorityDigest()
			if err != nil || got != want || record.Command.ID != command.ID || record.Command.SHA256 != command.SHA256 || record.Command.Action != command.Action {
				return writefence.ErrConflict
			}
			observation.State = string(record.State)
			if record.State == writefence.Committed {
				// Load returns the canonical original receipt, preserving JSONB's
				// exact numbers instead of hashing its display whitespace.
				sum := sha256.Sum256(record.Receipt)
				observation.ReceiptSHA256 = hex.EncodeToString(sum[:])
				known, err := projectCommandHasBusinessReceipt(tx, record, request)
				if err != nil {
					return err
				}
				if !known && result.BusinessState != application.ProjectWorkBusy {
					result.BusinessState = application.ProjectWorkUnknown
				}
			}
			result.Commands = append(result.Commands, observation)
		}
		snapshot = result
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return application.ProjectWorkSnapshot{}, err
	}
	return snapshot, nil
}

// A technical committed marker alone cannot settle a custody responsibility.
// Unsupported business receipt contracts remain unknown during incremental cutover.
func projectCommandHasBusinessReceipt(tx *gorm.DB, record writefence.Record, request application.ProjectWorkRequest) (bool, error) {
	if record.Command.Action != "media.reference_pin.acquire" {
		return false, nil
	}
	var row referencePinRow
	err := tx.Where("acquire_command_id=? AND project_id=? AND scope_id=?", record.Command.ID, request.ProjectID, request.ScopeID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	pin, err := loadReferencePin(tx, row.ReferencePinIdentity, false)
	if err != nil {
		return false, err
	}
	digest, err := referenceRequestDigest(row.ReferencePinIdentity)
	if err != nil {
		return false, err
	}
	receipt, err := decodePinReceipt(record, digest)
	if err != nil || receipt.Pin == nil {
		return false, nil
	}
	return receipt.Pin.Identity == pin.Identity && receipt.Pin.AcquireCommandID == record.Command.ID && receipt.Pin.State == "held" && receipt.Pin.Kind != "" && receipt.Pin.SnapshotSHA256() == pin.SnapshotSHA256(), nil
}

func readProjectBusinessWork(tx *gorm.DB, request application.ProjectWorkRequest) (bool, error) {
	var busy bool
	err := tx.Raw(`SELECT
		EXISTS(SELECT 1 FROM media.media_asset WHERE project_id=@project AND NOT is_delete AND status IN ('uploading','processing')) OR
		EXISTS(SELECT 1 FROM media.transfer_job WHERE org_id=@org AND (source_project_id=@project OR target_project_id=@project) AND (status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended)) OR
		EXISTS(SELECT 1 FROM media.purge_job WHERE org_id=@org AND project_id=@project AND (status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended)) OR
		EXISTS(SELECT 1 FROM media.package_job WHERE org_id=@org AND project_id=@project AND status='needs_reconciliation') OR
		EXISTS(SELECT 1 FROM mediatool.depth_job WHERE org_id=@org AND project_id=@project AND (status IN ('queued','running','cancel_requested') OR needs_reconciliation OR execution_unconfirmed OR process_state IN ('started','unknown') OR active_worker IS NOT NULL)) OR
		EXISTS(SELECT 1 FROM mediatool.transcription_job WHERE org_id=@org AND project_id=@project AND (status IN ('queued','running','cancel_requested') OR inference_state IN ('submitted','unknown') OR active_worker IS NOT NULL)) OR
		EXISTS(SELECT 1 FROM mediatool.export_job WHERE org_id=@org AND project_id=@project AND (status IN ('queued','running','cancel_requested') OR active_worker IS NOT NULL)) OR
		EXISTS(SELECT 1 FROM media.provider_artifact_receipt WHERE project_id=@project AND state='prepared') OR
		EXISTS(SELECT 1 FROM media.operation_output_receipt WHERE payload->'output'->>'project_id'=@project_text AND state='prepared') OR
		EXISTS(SELECT 1 FROM infra.writer_command WHERE service_owner=infra.runtime_owner() AND state='admitted' AND authorities @> CAST(@authority AS jsonb))`,
		sql.Named("project", request.ProjectID), sql.Named("org", request.OrganizationID), sql.Named("project_text", request.ProjectID.String()), sql.Named("authority", projectAuthorityJSON(request))).Scan(&busy).Error
	return busy, err
}

func projectAuthorityJSON(request application.ProjectWorkRequest) string {
	body, _ := json.Marshal([]map[string]string{{"issuer": string(writefence.Creation), "kind": string(writefence.Project), "id": request.ProjectID.String()}})
	return string(body)
}

var _ application.ProjectWorkReader = (*ProjectWorkReader)(nil)
