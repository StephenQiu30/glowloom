package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// OutputSettlement is an Execution-local adapter capability. Implementations use
// only the supplied transaction; they never contact another service for money.
type OutputSettlement interface {
	FinalizeOutputInTransaction(context.Context, *gorm.DB, application.OutputDeliveryRequest, bool) error
}

// OutputDeliveryStore persists original commands before network I/O and adopts
// complete Media ACKs with output binding, settlement, terminal state and Outbox.
type OutputDeliveryStore struct {
	db         *gorm.DB
	settlement OutputSettlement
}

// NewOutputDeliveryStore requires the same-owner atomic settlement writer.
func NewOutputDeliveryStore(db *gorm.DB, settlement OutputSettlement) *OutputDeliveryStore {
	return &OutputDeliveryStore{db: db, settlement: settlement}
}

var _ application.OutputDeliveries = (*OutputDeliveryStore)(nil)

type outputDeliveryRow struct {
	CommandID     uuid.UUID `gorm:"primaryKey"`
	OperationID   uuid.UUID
	SeqNo         int
	RequestSHA256 string          `gorm:"column:request_sha256"`
	PayloadSHA256 string          `gorm:"column:payload_sha256"`
	Payload       json.RawMessage `gorm:"type:jsonb"`
	Attempted     bool
	State         string
	MediaReceipt  json.RawMessage `gorm:"type:jsonb"`
}

func (outputDeliveryRow) TableName() string { return "operation.output_delivery" }

func (s *OutputDeliveryStore) ready(ctx context.Context) error {
	if s == nil || s.db == nil || s.db.Statement == nil || s.settlement == nil {
		return ErrUnavailable
	}
	if _, nested := s.db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return writefence.ErrTransaction
	}
	owner, err := pgfence.New(s.db).Owner(ctx)
	if err != nil {
		return err
	}
	if owner != writefence.WriterExecution {
		return contract.ErrOutputPublicationOwner
	}
	return nil
}
func outputDeliveryJSON(value any) []byte { raw, _ := json.Marshal(value); return raw }
func matchOutputDelivery(row outputDeliveryRow, req application.OutputDeliveryRequest) error {
	if row.CommandID != req.Command.ID || row.OperationID != req.Publication.Output.OperationID || row.SeqNo != req.Publication.Output.Sequence || row.RequestSHA256 != req.Command.SHA256 || row.PayloadSHA256 != req.Publication.Output.Digest() {
		return contract.ErrOutputPublicationConflict
	}
	original, err := writefence.CanonicalReceipt(row.Payload)
	if err != nil {
		return err
	}
	current, err := writefence.CanonicalReceipt(outputDeliveryJSON(req))
	if err != nil {
		return err
	}
	if !bytes.Equal(original, current) {
		return contract.ErrOutputPublicationConflict
	}
	return nil
}
func decodeOutputDelivery(row outputDeliveryRow) (application.OutputDeliveryWork, error) {
	var work application.OutputDeliveryWork
	if json.Unmarshal(row.Payload, &work.Request) != nil || application.ValidateOutputDelivery(work.Request) != nil || matchOutputDelivery(row, work.Request) != nil {
		return work, contract.ErrOutputPublicationConflict
	}
	work.Attempted = row.Attempted
	if row.State != "prepared" {
		var receipt contract.OperationOutputReceipt
		if json.Unmarshal(row.MediaReceipt, &receipt) != nil || work.Request.Publication.Matches(receipt) != nil {
			return work, contract.ErrOutputPublicationConflict
		}
		work.Receipt = &receipt
	}
	return work, nil
}

// ReadOutputDelivery loads original immutable evidence for a new worker epoch.
// It does not authorize new transfer, supplier work, output binding or settlement.
func (s *OutputDeliveryStore) ReadOutputDelivery(ctx context.Context, operation uuid.UUID, sequence int) (*application.OutputDeliveryWork, error) {
	if err := s.ready(ctx); err != nil {
		return nil, err
	}
	var row outputDeliveryRow
	err := s.db.WithContext(ctx).Where("operation_id=? AND seq_no=?", operation, sequence).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	work, err := decodeOutputDelivery(row)
	return &work, err
}

// PrepareOutputDelivery verifies the original paid result and persists it before RPC.
func (s *OutputDeliveryStore) PrepareOutputDelivery(ctx context.Context, req application.OutputDeliveryRequest) (application.OutputDeliveryWork, error) {
	var work application.OutputDeliveryWork
	if err := application.ValidateOutputDelivery(req); err != nil {
		return work, err
	}
	if err := s.ready(ctx); err != nil {
		return work, err
	}
	if prior, err := s.ReadOutputDelivery(ctx, req.Publication.Output.OperationID, req.Publication.Output.Sequence); err != nil {
		return work, err
	} else if prior != nil {
		if matchOutputDelivery(outputDeliveryRow{CommandID: prior.Request.Command.ID, OperationID: prior.Request.Publication.Output.OperationID, SeqNo: prior.Request.Publication.Output.Sequence, RequestSHA256: prior.Request.Command.SHA256, PayloadSHA256: prior.Request.Publication.Output.Digest(), Payload: outputDeliveryJSON(prior.Request)}, req) != nil {
			return work, contract.ErrOutputPublicationConflict
		}
		return *prior, s.admitOriginalOutput(ctx, *prior)
	}
	// Retain the complete original payload before admitting its effect command.
	// A crash cannot leave an admitted digest whose worker evidence was lost.
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := NewStore(s.db).checkAcceptedOperationFunding(ctx, tx, req.Publication.Output.OperationID); err != nil {
			return err
		}
		if err := verifyOutputDeliveryEvidence(ctx, tx, req); err != nil {
			return err
		}
		if err := fenceOutputWindow(ctx, tx, req); err != nil {
			return err
		}
		row := outputDeliveryRow{CommandID: req.Command.ID, OperationID: req.Publication.Output.OperationID, SeqNo: req.Publication.Output.Sequence, RequestSHA256: req.Command.SHA256, PayloadSHA256: req.Publication.Output.Digest(), Payload: outputDeliveryJSON(req), State: "prepared"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var stored outputDeliveryRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("command_id=?", req.Command.ID).Take(&stored).Error; err != nil {
			return err
		}
		if err := matchOutputDelivery(stored, req); err != nil {
			return err
		}
		var err error
		work, err = decodeOutputDelivery(stored)
		return err
	})
	if err == nil {
		err = s.admitOriginalOutput(ctx, work)
	}
	return work, err
}

func (s *OutputDeliveryStore) admitOriginalOutput(ctx context.Context, work application.OutputDeliveryWork) error {
	if work.Receipt != nil {
		return nil
	}
	record, err := pgfence.New(s.db).Admit(ctx, work.Request.Command)
	if err != nil {
		return err
	}
	if record.State != writefence.Admitted {
		return writefence.ErrFrozen
	}
	return nil
}

// StartOutputDelivery permanently records a possible network transfer before it starts.
func (s *OutputDeliveryStore) StartOutputDelivery(ctx context.Context, req application.OutputDeliveryRequest) (bool, error) {
	if err := application.ValidateOutputDelivery(req); err != nil {
		return false, err
	}
	if err := s.ready(ctx); err != nil {
		return false, err
	}
	var started bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := pgfence.New(s.db).Guard(ctx, tx, req.Command); err != nil {
			return err
		}
		var row outputDeliveryRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("command_id=?", req.Command.ID).Take(&row).Error; err != nil {
			return err
		}
		if err := matchOutputDelivery(row, req); err != nil {
			return err
		}
		if row.Attempted || row.State != "prepared" {
			return nil
		}
		if err := verifyOutputDeliveryEvidence(ctx, tx, req); err != nil {
			return err
		}
		if err := fenceOutputWindow(ctx, tx, req); err != nil {
			return err
		}
		if err := recordOutputAttemptWindow(ctx, tx, req); err != nil {
			return err
		}
		result := tx.Model(&outputDeliveryRow{}).Where("command_id=? AND NOT attempted AND state='prepared'", req.Command.ID).Updates(map[string]any{"attempted": true, "update_time": gorm.Expr("CURRENT_TIMESTAMP")})
		if result.Error != nil {
			return result.Error
		}
		started = result.RowsAffected == 1
		return nil
	})
	return started, err
}

// AdoptOutputDelivery performs no external IO. Prior ACKs remain private in the
// delivery ledger until all required sequences can bind and settle in this Tx.
func (s *OutputDeliveryStore) AdoptOutputDelivery(ctx context.Context, req application.OutputDeliveryRequest, receipt contract.OperationOutputReceipt) error {
	if err := application.ValidateOutputDelivery(req); err != nil {
		return err
	}
	if err := req.Publication.Matches(receipt); err != nil {
		return err
	}
	if err := s.ready(ctx); err != nil {
		return err
	}
	if (receipt.State == "published" && (receipt.ModerationStatus != "passed" && receipt.ModerationStatus != "skipped")) || (receipt.State == "rejected" && receipt.ModerationStatus != "rejected") {
		return contract.ErrOutputPublicationConflict
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fence := pgfence.New(s.db)
		guard := fence.Guard(ctx, tx, req.Command)
		if guard != nil && !errors.Is(guard, writefence.ErrCommitted) {
			return guard
		}
		// Serialize the whole manifest before any individual delivery row to avoid
		// last-two-sequence deadlocks and make completeness a local transaction fact.
		var present int
		locked := tx.Raw(`SELECT 1 FROM operation.operation WHERE id=? AND project_id=? AND NOT is_delete FOR UPDATE`, req.Publication.Output.OperationID, req.Publication.Output.ProjectID).Scan(&present)
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return ErrNotFound
		}
		var row outputDeliveryRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("command_id=?", req.Command.ID).Take(&row).Error; err != nil {
			return err
		}
		if err := matchOutputDelivery(row, req); err != nil {
			return err
		}
		raw, err := writefence.CanonicalReceipt(outputDeliveryJSON(receipt))
		if err != nil {
			return err
		}
		if row.State != "prepared" {
			old, err := writefence.CanonicalReceipt(row.MediaReceipt)
			if err != nil || !bytes.Equal(old, raw) {
				return contract.ErrOutputPublicationConflict
			}
			return nil
		}
		if errors.Is(guard, writefence.ErrCommitted) || !row.Attempted {
			return contract.ErrOutputPublicationConflict
		}
		if err := verifyOutputDeliveryEvidence(ctx, tx, req); err != nil {
			return err
		}
		if err := fenceOutputWindow(ctx, tx, req); err != nil {
			return err
		}
		state := "adopted"
		if receipt.State == "rejected" {
			state = "rejected"
		}
		updated := tx.Model(&outputDeliveryRow{}).Where("command_id=? AND state='prepared' AND attempted", req.Command.ID).Updates(map[string]any{"state": state, "media_receipt": string(raw), "update_time": gorm.Expr("CURRENT_TIMESTAMP")})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return contract.ErrOutputPublicationConflict
		}
		rows := []outputDeliveryRow{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id=?", req.Publication.Output.OperationID).Order("seq_no").Find(&rows).Error; err != nil {
			return err
		}
		complete := len(rows) == len(req.Publication.Output.Sequences)
		rejected := false
		for i, saved := range rows {
			if !complete {
				break
			}
			if saved.SeqNo != req.Publication.Output.Sequences[i] || saved.State == "prepared" {
				complete = false
				break
			}
			work, err := decodeOutputDelivery(saved)
			if err != nil {
				return err
			}
			o := work.Request.Publication.Output
			if o.Provider != req.Publication.Output.Provider || o.ProjectID != req.Publication.Output.ProjectID || o.ScopeID != req.Publication.Output.ScopeID || !equalOutputSequences(o.Sequences, req.Publication.Output.Sequences) {
				return contract.ErrOutputPublicationConflict
			}
			rejected = rejected || saved.State == "rejected"
		}
		if complete {
			for _, saved := range rows {
				work, err := decodeOutputDelivery(saved)
				if err != nil {
					return err
				}
				ack := work.Receipt
				o := work.Request.Publication.Output
				result := tx.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status) VALUES(?,?,?,?,?,?,?) ON CONFLICT ON CONSTRAINT uq_operation_output DO NOTHING`, o.OutputID, o.ProjectID, o.OperationID, o.Sequence, "media", ack.AssetID, ack.ModerationStatus)
				if result.Error != nil {
					return result.Error
				}
				var exact int
				r := tx.Raw(`SELECT 1 FROM operation.operation_output WHERE id=? AND project_id=? AND operation_id=? AND seq_no=? AND kind=? AND media_asset_id=? AND moderation_status=? AND NOT is_delete`, o.OutputID, o.ProjectID, o.OperationID, o.Sequence, "media", ack.AssetID, ack.ModerationStatus).Scan(&exact)
				if r.Error != nil {
					return r.Error
				}
				if r.RowsAffected != 1 {
					return contract.ErrOutputPublicationConflict
				}
			}
			if err := s.settlement.FinalizeOutputInTransaction(ctx, tx, req, rejected); err != nil {
				return err
			}
		}
		return fence.Complete(ctx, tx, req.Command, raw)
	})
}

func equalOutputSequences(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if b[i] != v {
			return false
		}
	}
	return true
}

func fenceOutputWindow(ctx context.Context, tx *gorm.DB, req application.OutputDeliveryRequest) error {
	w := req.Publication.Output.Worker
	job, ok := taskruntime.CurrentJob(ctx)
	if !ok {
		job = taskruntime.Job{Kind: w.Kind, BusinessID: w.TaskID.String(), Owner: w.Owner, Epoch: w.Epoch}
	}
	if (job.Kind != "operation.run" && job.Kind != "operation.ingest") || job.BusinessID != req.Publication.Output.OperationID.String() {
		return taskruntime.ErrLeaseLost
	}
	return taskruntime.Fence(ctx, tx, job)
}

func outputEvidenceHash(raw json.RawMessage) (string, error) {
	canonical, err := writefence.CanonicalReceipt(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// verifyOutputDeliveryEvidence binds real immutable provider result/usage/cost and
// actual sequence/bytes to this operation. No supplier evidence is supplied by Media.
func verifyOutputDeliveryEvidence(ctx context.Context, tx *gorm.DB, req application.OutputDeliveryRequest) error {
	o := req.Publication.Output
	var stored struct {
		Result                                    json.RawMessage
		OutputCount                               int
		Status                                    string
		RequestKey                                string
		ProviderKey, ModelKey, Region, Moderation string
		ModelProfileVersionID, PriceRuleVersionID uuid.UUID
	}
	r := tx.Raw(`SELECT pr.result,op.output_count,op.status,op.provider_request_key AS request_key,op.model_profile_version_id,op.price_rule_version_id,p.key AS provider_key,m.model_key,op.region,v.moderation FROM operation.operation op JOIN operation.provider_result pr ON pr.operation_id=op.id JOIN catalog.model_profile_version v ON v.id=op.model_profile_version_id JOIN catalog.model_profile m ON m.id=v.model_profile_id JOIN catalog.provider p ON p.id=m.provider_id WHERE op.id=? AND op.project_id=? AND NOT op.is_delete FOR UPDATE OF op`, o.OperationID, o.ProjectID).Scan(&stored)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 || stored.Moderation != "provider" || stored.ProviderKey != o.ProviderKey || stored.ModelKey != o.ModelKey || stored.Region != o.Region {
		return application.ErrProviderCallConflict
	}
	var result application.ManagedResult
	if json.Unmarshal(stored.Result, &result) != nil || result.Validate() != nil || result.State != application.ManagedResultSucceeded || result.Identity.OperationID != o.OperationID || result.Identity.ProjectID != o.ProjectID || result.Identity.RequestKey != o.Provider.RequestKey || result.Identity.RequestKey != stored.RequestKey || result.Identity.Attempt != o.Provider.Attempt || result.Identity.ModelProfileVersionID != stored.ModelProfileVersionID || result.Identity.PriceRuleVersionID != stored.PriceRuleVersionID || stored.OutputCount != len(result.Artifacts) || stored.OutputCount != len(o.Sequences) {
		return application.ErrProviderCallConflict
	}
	if stored.Status != "succeeded" && stored.Status != "ingesting" {
		return application.ErrTransitionConflict
	}
	hash, err := outputEvidenceHash(outputDeliveryJSON(result))
	if err != nil || hash != o.Provider.ManifestSHA256 {
		return application.ErrProviderCallConflict
	}
	hash, err = outputEvidenceHash(result.Usage)
	if err != nil || hash != o.Provider.UsageSHA256 {
		return application.ErrProviderCallConflict
	}
	hash, err = outputEvidenceHash(result.SupplierUsage)
	if err != nil || hash != o.Provider.SupplierUsageSHA256 {
		return application.ErrProviderCallConflict
	}
	found := false
	for i, artifact := range result.Artifacts {
		if artifact.Sequence != o.Sequences[i] {
			return application.ErrProviderCallConflict
		}
		if artifact.Sequence == o.Sequence {
			found = artifact.Size == o.ByteSize && artifact.MIME == o.MIMEType && artifact.SHA256 == o.SHA256
		}
	}
	if !found {
		return application.ErrProviderCallConflict
	}
	cost, err := NewStore(tx).ProviderCostInTransaction(ctx, tx, o.OperationID)
	if err != nil {
		return err
	}
	if cost.ActualCostMicros != o.Provider.ActualCostMicros {
		return fmt.Errorf("original provider cost changed: %w", application.ErrProviderCallConflict)
	}
	return nil
}
