package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// ArtifactStore owns durable bounded source bytes, never physical Media objects.
type ArtifactStore struct{ db *gorm.DB }

// NewArtifactStore requires an Execution-only runtime connection.
func NewArtifactStore(db *gorm.DB) *ArtifactStore { return &ArtifactStore{db: db} }

func (s *ArtifactStore) ready(ctx context.Context) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
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
		return writefence.ErrInvalid
	}
	return nil
}

func artifactWorkerFence(ctx context.Context, tx *gorm.DB, req contract.Request) error {
	job, ok := taskruntime.CurrentJob(ctx)
	if !ok || job.BusinessID != req.OperationID.String() || job.Kind != "operation.run" {
		return taskruntime.ErrLeaseLost
	}
	return taskruntime.Fence(ctx, tx, job)
}

func artifactCommand(r contract.Request) writefence.Command {
	return writefence.Command{ID: uuid.NewSHA1(r.DeliveryID, []byte("execution.provider_artifact.collect")), Action: "execution.provider_artifact.collect", SHA256: r.Digest(), Authorities: r.Command.Authorities}
}

// PrepareArtifact persists the first complete source envelope before any transfer.
func (s *ArtifactStore) PrepareArtifact(ctx context.Context, i application.ProviderDispatchIdentity, a application.ManagedArtifact) (application.ArtifactDeliveryRecord, error) {
	var record application.ArtifactDeliveryRecord
	if err := s.ready(ctx); err != nil {
		return record, err
	}
	if i.Validate() != nil || a.Sequence < 1 || a.Sequence > 8 || a.Size < 1 || a.Size > 512<<20 {
		return record, contract.ErrInvalid
	}
	job, ok := taskruntime.CurrentJob(ctx)
	if !ok || job.BusinessID != i.OperationID.String() || job.Kind != "operation.run" {
		return record, taskruntime.ErrLeaseLost
	}
	if prior, err := s.FindArtifact(ctx, i, a.Sequence); err == nil {
		if prior.Request.ByteSize != a.Size || prior.Request.SHA256 != a.SHA256 || prior.Request.MIME != a.MIME {
			return record, contract.ErrConflict
		}
		if _, err := pgfence.New(s.db).Admit(ctx, artifactCommand(prior.Request)); err != nil {
			return record, err
		}
		return prior, nil
	} else if !errors.Is(err, contract.ErrUnknown) {
		return record, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := taskruntime.Fence(ctx, tx, job); err != nil {
			return err
		}
		var row struct {
			ID                    uuid.UUID
			CreateTime            time.Time
			Snapshot, Observation json.RawMessage
			Capability            string
		}
		read := tx.Raw(`SELECT c.id,c.create_time,a.snapshot,o.capability,c.response_summary->'supplier_observation' AS observation FROM operation.operation o JOIN operation.frozen_input_acceptance a ON a.operation_id=o.id JOIN operation.provider_call c ON c.operation_id=o.id AND c.project_id=o.project_id WHERE o.id=? AND o.project_id=? AND o.model_profile_version_id=? AND o.price_rule_version_id=? AND o.provider_request_key=? AND c.request_key=o.provider_request_key AND c.model_profile_version_id=o.model_profile_version_id AND c.price_rule_version_id=o.price_rule_version_id AND c.response_summary#>>'{supplier_observation,original,state}'='succeeded' AND NOT o.is_delete AND NOT c.is_delete ORDER BY c.create_time DESC,c.attempt DESC LIMIT 1 FOR UPDATE OF o,c`, i.OperationID, i.ProjectID, i.ModelProfileVersionID, i.PriceRuleVersionID, i.RequestKey).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return contract.ErrConflict
		}
		var original supplierObservationRecord
		if json.Unmarshal(row.Observation, &original) != nil || original.Original.Identity != i || original.Original.Validate() != nil {
			return contract.ErrConflict
		}
		var snapshot map[string]json.RawMessage
		var command writefence.Command
		if json.Unmarshal(row.Snapshot, &snapshot) != nil || json.Unmarshal(snapshot["command"], &command) != nil {
			return application.ErrFrozenInputConflict
		}
		manifest, err := outputEvidenceHash(row.Observation)
		if err != nil {
			return err
		}
		delivery := uuid.NewSHA1(row.ID, []byte(fmt.Sprintf("provider-artifact/%s/%d", row.CreateTime.UTC().Format(time.RFC3339Nano), a.Sequence)))
		request := contract.Request{Command: writefence.Command{ID: uuid.NewSHA1(delivery, []byte("media.provider_artifact.stage")), Action: "media.provider_artifact.stage", Authorities: command.Authorities}, DeliveryID: delivery, ProviderCallID: row.ID, ProviderCallCreatedAt: row.CreateTime, OperationID: i.OperationID, ProjectID: i.ProjectID, CallAction: original.Original.CallAction, CallAttempt: original.Original.CallAttempt, RequestKey: i.RequestKey, Sequence: a.Sequence, ByteSize: a.Size, SHA256: a.SHA256, ManifestSHA256: manifest, MIME: a.MIME}
		request.Processing = "original"
		switch row.Capability {
		case "video.generate":
			request.Processing = "video_probe"
		case "audio.generate":
			if original.ReleaseID != nil {
				var config struct{ Adapter string }
				read := tx.Raw(`SELECT v.config->>'adapter_key' AS adapter FROM catalog.model_release r JOIN catalog.provider_config_version v ON v.id=r.config_version_id WHERE r.id=?`, *original.ReleaseID).Scan(&config)
				if read.Error != nil {
					return read.Error
				}
				if read.RowsAffected != 1 {
					return contract.ErrConflict
				}
				if config.Adapter == "volcengine_music" {
					request.Processing = "music_mp3"
				}
			}
		}
		request.Command.SHA256 = request.Digest()
		if request.Validate() != nil {
			return contract.ErrInvalid
		}
		raw, err := json.Marshal(request)
		if err != nil {
			return err
		}
		inserted := tx.Exec(`INSERT INTO operation.provider_artifact_delivery(delivery_id,media_command_id,provider_call_id,provider_call_created_at,operation_id,project_id,call_action,call_attempt,request_key,artifact_seq,byte_size,sha256,manifest_sha256,request_sha256,request,chunk_count) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb,?) ON CONFLICT DO NOTHING`, delivery, request.Command.ID, row.ID, row.CreateTime, i.OperationID, i.ProjectID, request.CallAction, request.CallAttempt, i.RequestKey, a.Sequence, a.Size, a.SHA256, manifest, request.Digest(), string(raw), (a.Size+contract.ChunkSize-1)/contract.ChunkSize)
		if inserted.Error != nil {
			return inserted.Error
		}
		record, err = readArtifactRecord(tx, request, true)
		return err
	})
	if err != nil {
		return record, err
	}
	if _, err = pgfence.New(s.db).Admit(ctx, artifactCommand(record.Request)); err != nil {
		return record, err
	}
	return record, nil
}

func readArtifactRecord(tx *gorm.DB, req contract.Request, lock bool) (application.ArtifactDeliveryRecord, error) {
	var record application.ArtifactDeliveryRecord
	var row struct {
		Request, MediaReceipt json.RawMessage
		State                 string
		Attempted             bool
	}
	query := `SELECT request,media_receipt,state,attempted FROM operation.provider_artifact_delivery WHERE delivery_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	read := tx.Raw(query, req.DeliveryID).Scan(&row)
	if read.Error != nil {
		return record, read.Error
	}
	if read.RowsAffected != 1 {
		return record, contract.ErrUnknown
	}
	if json.Unmarshal(row.Request, &record.Request) != nil || !record.Request.Equal(req) {
		return record, contract.ErrConflict
	}
	record.State, record.Attempted = row.State, row.Attempted
	if len(row.MediaReceipt) > 0 && string(row.MediaReceipt) != "null" {
		record.Receipt = new(contract.Receipt)
		if json.Unmarshal(row.MediaReceipt, record.Receipt) != nil || !record.Receipt.Matches(req) {
			return record, contract.ErrConflict
		}
	}
	return record, nil
}

// PutArtifactChunk inserts one exact chunk; replay can only match existing bytes.
func (s *ArtifactStore) PutArtifactChunk(ctx context.Context, req contract.Request, seq int, data []byte) error {
	if err := s.ready(ctx); err != nil {
		return err
	}
	if req.Validate() != nil || seq < 0 || len(data) < 1 || len(data) > contract.ChunkSize {
		return contract.ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := artifactWorkerFence(ctx, tx, req); err != nil {
			return err
		}
		if err := pgfence.New(s.db).Guard(ctx, tx, artifactCommand(req)); err != nil {
			return err
		}
		record, err := readArtifactRecord(tx, req, true)
		if err != nil {
			return err
		}
		if record.State != "collecting" {
			return contract.ErrConflict
		}
		var existing struct{ Data []byte }
		read := tx.Raw(`SELECT data FROM operation.provider_artifact_chunk WHERE delivery_id=? AND seq_no=?`, req.DeliveryID, seq).Scan(&existing)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
			if !bytes.Equal(data, existing.Data) {
				return contract.ErrConflict
			}
			return nil
		}
		return tx.Exec(`INSERT INTO operation.provider_artifact_chunk(delivery_id,seq_no,data) VALUES(?,?,?)`, req.DeliveryID, seq, data).Error
	})
}

// SealArtifact hashes rows in bounded memory before publishing an immutable source.
func (s *ArtifactStore) SealArtifact(ctx context.Context, req contract.Request) error {
	if err := s.ready(ctx); err != nil {
		return err
	}
	if req.Validate() != nil {
		return contract.ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := artifactWorkerFence(ctx, tx, req); err != nil {
			return err
		}
		if err := pgfence.New(s.db).Guard(ctx, tx, artifactCommand(req)); err != nil {
			return err
		}
		record, err := readArtifactRecord(tx, req, true)
		if err != nil {
			return err
		}
		if record.State != "collecting" {
			return nil
		}
		rows, err := tx.Raw(`SELECT seq_no,data FROM operation.provider_artifact_chunk WHERE delivery_id=? ORDER BY seq_no`, req.DeliveryID).Rows()
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		hash := sha256.New()
		var size int64
		count := 0
		for rows.Next() {
			var seq int
			var data []byte
			if err := rows.Scan(&seq, &data); err != nil {
				return err
			}
			expected := int64(contract.ChunkSize)
			if req.ByteSize-size < expected {
				expected = req.ByteSize - size
			}
			if seq != count || int64(len(data)) != expected {
				return contract.ErrConflict
			}
			hash.Write(data)
			size += int64(len(data))
			count++
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if size != req.ByteSize || hex.EncodeToString(hash.Sum(nil)) != req.SHA256 {
			return contract.ErrConflict
		}
		return tx.Exec(`UPDATE operation.provider_artifact_delivery SET state='prepared',sealed_at=CURRENT_TIMESTAMP,update_time=CURRENT_TIMESTAMP WHERE delivery_id=?`, req.DeliveryID).Error
	})
}

// StartArtifact permanently records transfer before the first external call.
func (s *ArtifactStore) StartArtifact(ctx context.Context, req contract.Request) (bool, error) {
	if err := s.ready(ctx); err != nil {
		return false, err
	}
	if req.Validate() != nil {
		return false, contract.ErrInvalid
	}
	started := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := artifactWorkerFence(ctx, tx, req); err != nil {
			return err
		}
		if err := pgfence.New(s.db).Guard(ctx, tx, artifactCommand(req)); err != nil {
			return err
		}
		record, err := readArtifactRecord(tx, req, true)
		if err != nil {
			return err
		}
		if record.Attempted {
			return nil
		}
		if record.State != "prepared" {
			return contract.ErrConflict
		}
		update := tx.Exec(`UPDATE operation.provider_artifact_delivery SET attempted=true,update_time=CURRENT_TIMESTAMP WHERE delivery_id=?`, req.DeliveryID)
		if update.Error != nil {
			return update.Error
		}
		started = true
		return nil
	})
	return started, err
}

// AdoptArtifact binds the exact Media ACK and removes source chunks in one commit.
func (s *ArtifactStore) AdoptArtifact(ctx context.Context, req contract.Request, ack contract.Receipt) error {
	if err := s.ready(ctx); err != nil {
		return err
	}
	if !ack.Matches(req) {
		return contract.ErrConflict
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := artifactWorkerFence(ctx, tx, req); err != nil {
			return err
		}
		record, err := readArtifactRecord(tx, req, true)
		if err != nil {
			return err
		}
		if record.Receipt != nil {
			return nil
		}
		if !record.Attempted || record.State != "prepared" {
			return contract.ErrConflict
		}
		raw, err := json.Marshal(ack)
		if err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE operation.provider_artifact_delivery SET state='present',media_receipt=?::jsonb,update_time=CURRENT_TIMESTAMP WHERE delivery_id=?`, string(raw), req.DeliveryID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM operation.provider_artifact_chunk WHERE delivery_id=?`, req.DeliveryID).Error; err != nil {
			return err
		}
		// This is transfer responsibility, not an operation output or financial effect.
		return pgfence.New(s.db).Complete(ctx, tx, artifactCommand(req), raw)
	})
}

// FindArtifact reads an opaque receipt only through the original full dispatch.
func (s *ArtifactStore) FindArtifact(ctx context.Context, i application.ProviderDispatchIdentity, sequence int) (application.ArtifactDeliveryRecord, error) {
	var record application.ArtifactDeliveryRecord
	if err := s.ready(ctx); err != nil {
		return record, err
	}
	if i.Validate() != nil || sequence < 1 || sequence > 8 {
		return record, contract.ErrInvalid
	}
	var rows []struct{ Request json.RawMessage }
	read := s.db.WithContext(ctx).Raw(`SELECT d.request FROM operation.provider_artifact_delivery d JOIN operation.operation o ON o.id=d.operation_id WHERE d.operation_id=? AND d.project_id=? AND d.request_key=? AND d.artifact_seq=? AND o.model_profile_version_id=? AND o.price_rule_version_id=? ORDER BY d.create_time LIMIT 2`, i.OperationID, i.ProjectID, i.RequestKey, sequence, i.ModelProfileVersionID, i.PriceRuleVersionID).Scan(&rows)
	if read.Error != nil {
		return record, read.Error
	}
	if len(rows) == 0 {
		return record, contract.ErrUnknown
	}
	if len(rows) != 1 {
		return record, contract.ErrConflict
	}
	var req contract.Request
	if json.Unmarshal(rows[0].Request, &req) != nil {
		return record, contract.ErrConflict
	}
	return readArtifactRecord(s.db.WithContext(ctx), req, false)
}

// ArtifactDeliveries reads original bounded recovery records, without opening bytes.
func (s *ArtifactStore) ArtifactDeliveries(ctx context.Context, i application.ProviderDispatchIdentity) ([]application.ArtifactDeliveryRecord, error) {
	if err := s.ready(ctx); err != nil {
		return nil, err
	}
	if i.Validate() != nil {
		return nil, contract.ErrInvalid
	}
	var rows []struct{ Request json.RawMessage }
	read := s.db.WithContext(ctx).Raw(`SELECT d.request FROM operation.provider_artifact_delivery d JOIN operation.operation o ON o.id=d.operation_id WHERE d.operation_id=? AND d.project_id=? AND d.request_key=? AND o.model_profile_version_id=? AND o.price_rule_version_id=? ORDER BY d.create_time,d.artifact_seq LIMIT 129`, i.OperationID, i.ProjectID, i.RequestKey, i.ModelProfileVersionID, i.PriceRuleVersionID).Scan(&rows)
	if read.Error != nil {
		return nil, read.Error
	}
	if len(rows) > 128 {
		return nil, contract.ErrConflict
	}
	records := make([]application.ArtifactDeliveryRecord, 0, len(rows))
	for _, row := range rows {
		var req contract.Request
		if json.Unmarshal(row.Request, &req) != nil {
			return nil, contract.ErrConflict
		}
		record, err := readArtifactRecord(s.db.WithContext(ctx), req, false)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// VerifyArtifactSource permits only the exact durable attempted source request.
func (s *ArtifactStore) VerifyArtifactSource(ctx context.Context, req contract.Request) error {
	if err := s.ready(ctx); err != nil {
		return err
	}
	if req.Validate() != nil {
		return contract.ErrInvalid
	}
	record, err := readArtifactRecord(s.db.WithContext(ctx), req, false)
	if err != nil {
		return err
	}
	if !record.Attempted || record.State != "prepared" {
		return contract.ErrUnknown
	}
	return nil
}

// OpenArtifactSource streams one bounded chunk query at a time without a SQL lock.
func (s *ArtifactStore) OpenArtifactSource(ctx context.Context, req contract.Request) (io.ReadCloser, error) {
	if err := s.VerifyArtifactSource(ctx, req); err != nil {
		return nil, err
	}
	return &artifactChunkReader{ctx: ctx, db: s.db, req: req}, nil
}

type artifactChunkReader struct {
	ctx    context.Context
	db     *gorm.DB
	req    contract.Request
	seq    int
	data   *bytes.Reader
	read   int64
	closed bool
}

func (r *artifactChunkReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, errors.New("artifact source closed")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.data == nil || r.data.Len() == 0 {
		if r.read == r.req.ByteSize {
			return 0, io.EOF
		}
		var row struct{ Data []byte }
		q := r.db.WithContext(r.ctx).Raw(`SELECT data FROM operation.provider_artifact_chunk WHERE delivery_id=? AND seq_no=?`, r.req.DeliveryID, r.seq).Scan(&row)
		if q.Error != nil {
			return 0, q.Error
		}
		if q.RowsAffected != 1 {
			return 0, contract.ErrUnknown
		}
		expected := int64(contract.ChunkSize)
		if r.req.ByteSize-r.read < expected {
			expected = r.req.ByteSize - r.read
		}
		if int64(len(row.Data)) != expected {
			return 0, contract.ErrConflict
		}
		r.data = bytes.NewReader(row.Data)
		r.seq++
	}
	n, err := r.data.Read(p)
	r.read += int64(n)
	return n, err
}
func (r *artifactChunkReader) Close() error { r.closed = true; r.data = nil; return nil }

var _ application.ArtifactDeliveryRepository = (*ArtifactStore)(nil)
