package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// ProviderArtifactStore binds opaque byte receipts to Media-local physical storage.
type ProviderArtifactStore struct{ db *gorm.DB }

// NewProviderArtifactStore requires the restricted Media runtime database.
func NewProviderArtifactStore(db *gorm.DB) *ProviderArtifactStore {
	return &ProviderArtifactStore{db: db}
}

func (s *ProviderArtifactStore) ready(ctx context.Context) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return application.ErrUnavailable
	}
	if _, nested := s.db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return writefence.ErrTransaction
	}
	owner, err := pgfence.New(s.db).Owner(ctx)
	if err != nil {
		return err
	}
	if owner != writefence.WriterMedia {
		return writefence.ErrInvalid
	}
	return nil
}

// PrepareProviderArtifact never selects a new storage route during generation.
func (s *ProviderArtifactStore) PrepareProviderArtifact(ctx context.Context, req contract.Request) (application.ProviderArtifactWork, error) {
	var work application.ProviderArtifactWork
	if err := s.ready(ctx); err != nil {
		return work, err
	}
	if application.ValidateProviderArtifact(req) != nil {
		return work, contract.ErrInvalid
	}
	if prior, err := s.ReadProviderArtifact(ctx, req); err == nil {
		if prior.Receipt != nil {
			return prior, nil
		}
	} else if !errors.Is(err, contract.ErrUnknown) {
		return work, err
	}
	if _, err := pgfence.New(s.db).Admit(ctx, req.Command); err != nil {
		return work, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := pgfence.New(s.db).Guard(ctx, tx, req.Command); err != nil {
			return err
		}
		owner := application.StorageWorkOwner{Kind: application.StorageWorkKindOperation, OwnerID: req.ProjectID, WorkID: req.OperationID}
		intent, found, err := readStorageWork(tx, owner)
		if err != nil {
			return err
		}
		if !found || intent.State != application.StorageWorkOpen {
			return application.ErrStorageRouteConflict
		}
		raw, err := json.Marshal(req)
		if err != nil {
			return err
		}
		insert := tx.Exec(`INSERT INTO media.provider_artifact_receipt(command_id,delivery_id,provider_call_id,provider_call_created_at,operation_id,project_id,call_action,call_attempt,request_key,artifact_seq,byte_size,sha256,manifest_sha256,request_sha256,request,storage_work_intent_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb,?) ON CONFLICT DO NOTHING`, req.Command.ID, req.DeliveryID, req.ProviderCallID, req.ProviderCallCreatedAt, req.OperationID, req.ProjectID, req.CallAction, req.CallAttempt, req.RequestKey, req.Sequence, req.ByteSize, req.SHA256, req.ManifestSHA256, req.Digest(), string(raw), intent.ID)
		if insert.Error != nil {
			return insert.Error
		}
		work, err = readProviderArtifact(tx, req, true)
		return err
	})
	return work, err
}

type providerArtifactStorageReceipt struct {
	Ack                 contract.Receipt `json:"ack"`
	CanonicalLocationID *uuid.UUID       `json:"canonical_location_id,omitempty"`
}

func readProviderArtifact(tx *gorm.DB, req contract.Request, lock bool) (application.ProviderArtifactWork, error) {
	var work application.ProviderArtifactWork
	var row struct {
		Request, Receipt    json.RawMessage
		StorageWorkIntentID uuid.UUID
		LocationID          *uuid.UUID
		CreateTime          time.Time
	}
	query := `SELECT request,receipt,storage_work_intent_id,location_id,create_time FROM media.provider_artifact_receipt WHERE command_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	read := tx.Raw(query, req.Command.ID).Scan(&row)
	if read.Error != nil {
		return work, read.Error
	}
	if read.RowsAffected != 1 {
		return work, contract.ErrUnknown
	}
	if json.Unmarshal(row.Request, &work.Request) != nil || !work.Request.Equal(req) {
		return work, contract.ErrConflict
	}
	intent, found, err := readStorageWork(tx, application.StorageWorkOwner{Kind: application.StorageWorkKindOperation, OwnerID: req.ProjectID, WorkID: req.OperationID})
	if err != nil {
		return work, err
	}
	if !found || intent.ID != row.StorageWorkIntentID {
		return work, application.ErrStorageRouteConflict
	}
	work.Intent, work.PreparedAt = intent, row.CreateTime
	if len(row.Receipt) > 0 && string(row.Receipt) != "null" {
		var stored providerArtifactStorageReceipt
		if json.Unmarshal(row.Receipt, &stored) != nil || row.LocationID == nil {
			return work, contract.ErrConflict
		}
		// Earlier original-only rows contain the public ACK directly. They remain
		// immutable; canonical rows always use the private location envelope.
		if stored.Ack.ArtifactID == uuid.Nil {
			if json.Unmarshal(row.Receipt, &stored.Ack) != nil {
				return work, contract.ErrConflict
			}
		}
		if err := application.ValidateProviderArtifactReceipt(req, stored.Ack); err != nil {
			return work, err
		}
		work.Receipt = &stored.Ack
		work.Canonical = stored.Ack.Canonical
		if stored.CanonicalLocationID != nil {
			var canonical objectLocationRow
			if err := tx.Where("id=?", *stored.CanonicalLocationID).Take(&canonical).Error; err != nil {
				return work, err
			}
			value := canonical.domain()
			work.CanonicalLocation = &value
		} else if stored.Ack.Canonical != nil {
			return work, contract.ErrConflict
		}
		var location objectLocationRow
		if err := tx.Where("id=?", *row.LocationID).Take(&location).Error; err != nil {
			return work, err
		}
		value := location.domain()
		work.Location = &value
	}
	return work, nil
}

// ReadProviderArtifact resolves an exact original responsibility without current routing.
func (s *ProviderArtifactStore) ReadProviderArtifact(ctx context.Context, req contract.Request) (application.ProviderArtifactWork, error) {
	if err := s.ready(ctx); err != nil {
		return application.ProviderArtifactWork{}, err
	}
	if application.ValidateProviderArtifact(req) != nil {
		return application.ProviderArtifactWork{}, contract.ErrInvalid
	}
	return readProviderArtifact(s.db.WithContext(ctx), req, false)
}

// CommitProviderArtifact binds the actual verified location and receipt atomically.
func (s *ProviderArtifactStore) CommitProviderArtifact(ctx context.Context, work application.ProviderArtifactWork, loc domain.ObjectLocation) (contract.Receipt, error) {
	var ack contract.Receipt
	req := work.Request
	if err := s.ready(ctx); err != nil {
		return ack, err
	}
	if application.ValidateProviderArtifact(req) != nil {
		return ack, contract.ErrInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := pgfence.New(s.db).Guard(ctx, tx, req.Command); err != nil {
			return err
		}
		original, err := readProviderArtifact(tx, req, true)
		if err != nil {
			return err
		}
		if original.Receipt != nil {
			ack = *original.Receipt
			return nil
		}
		if original.Intent.ID != work.Intent.ID || original.Intent.State != application.StorageWorkOpen || loc.ID == uuid.Nil {
			return application.ErrStorageRouteConflict
		}
		var row objectLocationRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", loc.ID).Take(&row).Error; err != nil {
			return err
		}
		actual := row.domain()
		if row.OwnerIntentID == nil || *row.OwnerIntentID != work.Intent.ID || row.OwnerSessionID != nil || actual.Ref != loc.Ref || actual.Ref.ProfileID != work.Intent.ProfileID || actual.Ref.Bucket != work.Intent.Bucket || (actual.State != domain.ObjectLocationPresent && actual.State != domain.ObjectLocationAccepted) || actual.ByteSize == nil || *actual.ByteSize != req.ByteSize || actual.MIMEType == nil || *actual.MIMEType != req.MIME || actual.SHA256 == nil || *actual.SHA256 != req.SHA256 {
			return application.ErrStorageIdentityConflict
		}
		ack = contract.Receipt{Request: req, ArtifactID: req.DeliveryID, State: "present", Canonical: work.Canonical}
		if err := application.ValidateProviderArtifactReceipt(req, ack); err != nil {
			return err
		}
		stored := providerArtifactStorageReceipt{Ack: ack}
		if work.Canonical != nil {
			if work.CanonicalLocation == nil {
				return application.ErrStorageIdentityConflict
			}
			canonical := work.CanonicalLocation
			var saved objectLocationRow
			if canonical.ID == loc.ID {
				saved = row
			} else if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", canonical.ID).Take(&saved).Error; err != nil {
				return err
			}
			value := saved.domain()
			d := work.Canonical
			if saved.OwnerIntentID == nil || *saved.OwnerIntentID != work.Intent.ID || saved.OwnerSessionID != nil || value.Ref != canonical.Ref || value.Ref.ProfileID != work.Intent.ProfileID || value.Ref.Bucket != work.Intent.Bucket || (value.State != domain.ObjectLocationPresent && value.State != domain.ObjectLocationAccepted) || value.ByteSize == nil || *value.ByteSize != d.ByteSize || value.MIMEType == nil || *value.MIMEType != d.MIME || value.SHA256 == nil || *value.SHA256 != d.SHA256 {
				return application.ErrStorageIdentityConflict
			}
			stored.CanonicalLocationID = &canonical.ID
			if err := tx.Model(&objectLocationRow{}).Where("id=?", canonical.ID).Update("state", domain.ObjectLocationAccepted).Error; err != nil {
				return err
			}
		}
		raw, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		if err := tx.Model(&objectLocationRow{}).Where("id=?", loc.ID).Update("state", domain.ObjectLocationAccepted).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE media.provider_artifact_receipt SET state='present',location_id=?,receipt=?::jsonb,update_time=CURRENT_TIMESTAMP WHERE command_id=?`, loc.ID, string(raw), req.Command.ID).Error; err != nil {
			return err
		}
		return pgfence.New(s.db).Complete(ctx, tx, req.Command, raw)
	})
	return ack, err
}

var _ application.ProviderArtifactRepository = (*ProviderArtifactStore)(nil)
