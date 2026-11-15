package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// ReferencePinStore owns independent Media transactions. It deliberately has no
// cross-domain SQL, physical object reader or deletion capability.
type ReferencePinStore struct {
	db       *gorm.DB
	verifier application.ReferencePinConsumerVerifier
	source   application.ReferencePinSourceAdmission
}

// NewReferencePinStore uses the actual restricted connection, never an owner label.
// New acquisitions require source admission; custody and original committed ACK
// recovery remain available when that source is offline or no longer grants work.
func NewReferencePinStore(db *gorm.DB, verifier application.ReferencePinConsumerVerifier, source application.ReferencePinSourceAdmission) *ReferencePinStore {
	return &ReferencePinStore{db: db, verifier: verifier, source: source}
}

var _ application.ReferencePins = (*ReferencePinStore)(nil)

type referencePinRow struct {
	domain.ReferencePinIdentity `gorm:"embedded"`
	AcquireCommandID            uuid.UUID
	Facts                       json.RawMessage `gorm:"type:jsonb"`
	State                       string
	AdoptionReceiptSHA256       string `gorm:"column:adoption_receipt_sha256"`
	ReleaseReceiptSHA256        string `gorm:"column:release_receipt_sha256"`
	ReleaseOutcome              string
}

func (referencePinRow) TableName() string { return "media.reference_pin" }

type referenceGateRow struct {
	AssetID        uuid.UUID
	Epoch          int64
	Closed         bool
	CloseCommandID *uuid.UUID
}

func (referenceGateRow) TableName() string { return "media.reference_pin_gate" }

// pinCommandReceipt retains action payload identity separately from the caller's
// original business digest. A reused command cannot address a different pin.
type pinCommandReceipt struct {
	RequestSHA256 string                       `json:"request_sha256"`
	Pin           *domain.ReferencePin         `json:"pin,omitempty"`
	Gate          *domain.ReferenceAcquisition `json:"gate,omitempty"`
}

func (s *ReferencePinStore) ready(ctx context.Context) error {
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
		return domain.ErrReferencePinOwner
	}
	return nil
}

func referenceRequestDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", domain.ErrInvalidReferencePin
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (s *ReferencePinStore) prepare(ctx context.Context, command writefence.Command, action string, payload any) (string, *pinCommandReceipt, error) {
	if err := s.ready(ctx); err != nil {
		return "", nil, err
	}
	if command.Action != "media.reference_pin."+action {
		return "", nil, writefence.ErrInvalid
	}
	digest, err := referenceRequestDigest(payload)
	if err != nil {
		return "", nil, err
	}
	record, err := pgfence.New(s.db).Admit(ctx, command)
	if err != nil {
		return "", nil, err
	}
	if record.State == writefence.Frozen {
		return "", nil, writefence.ErrFrozen
	}
	if record.State == writefence.Committed {
		receipt, err := decodePinReceipt(record, digest)
		return digest, &receipt, err
	}
	return digest, nil, nil
}

func decodePinReceipt(record writefence.Record, digest string) (pinCommandReceipt, error) {
	var receipt pinCommandReceipt
	if json.Unmarshal(record.Receipt, &receipt) != nil || receipt.RequestSHA256 != digest || (receipt.Pin == nil) == (receipt.Gate == nil) {
		return receipt, domain.ErrReferencePinConflict
	}
	return receipt, nil
}

// transact is the sole effect path: Guard, business locks/effect and Complete
// share one transaction, including recovery after a concurrent lost-ACK replay.
func (s *ReferencePinStore) transact(ctx context.Context, command writefence.Command, digest string, effect func(*gorm.DB) (pinCommandReceipt, error)) (pinCommandReceipt, error) {
	var receipt pinCommandReceipt
	fences := pgfence.New(s.db)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := fences.Guard(ctx, tx, command)
		if errors.Is(err, writefence.ErrCommitted) {
			saved, err := pgfence.New(tx).Load(ctx, command.ID)
			if err != nil {
				return err
			}
			receipt, err = decodePinReceipt(saved, digest)
			return err
		}
		if err != nil {
			return err
		}
		receipt, err = effect(tx)
		if err != nil {
			return err
		}
		receipt.RequestSHA256 = digest
		raw, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		if err := fences.Complete(ctx, tx, command, raw); err != nil {
			return err
		}
		canonical, err := writefence.CanonicalReceipt(raw)
		if err != nil {
			return err
		}
		// Return exactly the same normalized nested review JSON that Load will
		// recover after PostgreSQL JSONB has reordered keys and whitespace.
		return json.Unmarshal(canonical, &receipt)
	})
	return receipt, err
}

func lockReferenceGate(tx *gorm.DB, asset uuid.UUID) (referenceGateRow, error) {
	// The INSERT conflict waits on a concurrent first acquisition/closure before
	// either can lock the same permanent asset gate. Closed rows are never reset.
	if err := tx.Exec(`INSERT INTO media.reference_pin_gate(asset_id) VALUES (?) ON CONFLICT (asset_id) DO NOTHING`, asset).Error; err != nil {
		return referenceGateRow{}, err
	}
	var row referenceGateRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("asset_id = ?", asset).Take(&row).Error
	return row, err
}

func loadReferencePin(tx *gorm.DB, identity domain.ReferencePinIdentity, lock bool) (domain.ReferencePin, error) {
	var row referencePinRow
	query := tx.Where("pin_id = ?", identity.PinID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ReferencePin{}, domain.ErrReferencePinNotFound
	}
	if err != nil {
		return domain.ReferencePin{}, err
	}
	if row.ReferencePinIdentity != identity {
		return domain.ReferencePin{}, domain.ErrReferencePinConflict
	}
	// Use exactly the receipt's decimal normalization before any consumer sees
	// immutable Facts. JSONB can retain decimal scale that differs from the
	// original Acquire ACK (for example 1.2300 versus 1.23). Compact first so
	// PostgreSQL's display whitespace does not consume the receipt size budget.
	var compact bytes.Buffer
	if json.Compact(&compact, row.Facts) != nil {
		return domain.ReferencePin{}, domain.ErrReferencePinConflict
	}
	canonical, err := writefence.CanonicalReceipt(compact.Bytes())
	if err != nil {
		return domain.ReferencePin{}, domain.ErrReferencePinConflict
	}
	var pin domain.ReferencePin
	if json.Unmarshal(canonical, &pin) != nil || pin.Identity != identity || pin.AcquireCommandID != row.AcquireCommandID {
		return pin, domain.ErrReferencePinConflict
	}
	pin.State = row.State
	pin.AdoptionReceiptSHA256 = row.AdoptionReceiptSHA256
	pin.ReleaseReceiptSHA256 = row.ReleaseReceiptSHA256
	pin.ReleaseOutcome = row.ReleaseOutcome
	return pin, nil
}

// Acquire atomically retains precisely the accepted bytes and review facts. A
// replay returns its original held receipt, even after binding/release/closure.
func (s *ReferencePinStore) Acquire(ctx context.Context, req application.ReferencePinRequest) (domain.ReferencePin, error) {
	if err := req.Validate(); err != nil {
		return domain.ReferencePin{}, err
	}
	digest, replay, err := s.prepare(ctx, req.Command, "acquire", req.Identity)
	if err != nil {
		return domain.ReferencePin{}, err
	}
	if replay != nil {
		if replay.Pin == nil {
			return domain.ReferencePin{}, domain.ErrReferencePinConflict
		}
		return *replay.Pin, nil
	}
	if s.source == nil {
		return domain.ReferencePin{}, domain.ErrReferencePinProof
	}
	proof, err := s.source.AdmitReferencePinSource(ctx, req)
	if err != nil {
		return domain.ReferencePin{}, fmt.Errorf("admit current pin acquisition source: %w", err)
	}
	if err := proof.Validate(req); err != nil {
		return domain.ReferencePin{}, err
	}
	receipt, err := s.transact(ctx, req.Command, digest, func(tx *gorm.DB) (pinCommandReceipt, error) {
		gate, err := lockReferenceGate(tx, req.Identity.AssetID)
		if err != nil {
			return pinCommandReceipt{}, err
		}
		if gate.Closed || gate.Epoch != req.Identity.GateEpoch {
			return pinCommandReceipt{}, domain.ErrReferencePinClosed
		}
		pin, err := acquireReferenceFacts(tx, req)
		if err != nil {
			return pinCommandReceipt{}, err
		}
		existing, err := loadReferencePin(tx, req.Identity, true)
		if err == nil {
			_ = existing
			return pinCommandReceipt{}, domain.ErrReferencePinConflict
		}
		if !errors.Is(err, domain.ErrReferencePinNotFound) {
			return pinCommandReceipt{}, err
		}
		raw, err := json.Marshal(pin)
		if err != nil {
			return pinCommandReceipt{}, err
		}
		row := referencePinRow{ReferencePinIdentity: req.Identity, AcquireCommandID: req.Command.ID, Facts: raw, State: "held"}
		if err := tx.Create(&row).Error; err != nil {
			return pinCommandReceipt{}, fmt.Errorf("retain media reference pin: %w", err)
		}
		return pinCommandReceipt{Pin: &pin}, nil
	})
	if err != nil {
		return domain.ReferencePin{}, err
	}
	return *receipt.Pin, nil
}

func acquireReferenceFacts(tx *gorm.DB, req application.ReferencePinRequest) (domain.ReferencePin, error) {
	i := req.Identity
	// Match the established catalog -> asset lock order used by recycling and
	// purge. Committed original acquisitions never re-enter this new-binding gate.
	var catalog []struct {
		CatalogState string
		PurgedAt     *time.Time
	}
	if err := tx.Table("media.library_item").Select("catalog_state", "purged_at").Clauses(clause.Locking{Strength: "SHARE"}).Where("asset_id = ?", i.AssetID).Find(&catalog).Error; err != nil {
		return domain.ReferencePin{}, err
	}
	for _, item := range catalog {
		if item.CatalogState != "active" || item.PurgedAt != nil {
			return domain.ReferencePin{}, domain.ErrReferencePinConflict
		}
	}
	var asset struct {
		ID, ProjectID, ObjectLocationID          uuid.UUID
		Status, Kind, ModerationStatus, SHA256   string
		ModerationDetail                         json.RawMessage
		ByteSize, Revision                       int64
		IsDelete, AIGCMarked, ContainsRealPerson bool
		ConsentRecordID                          *uuid.UUID
	}
	result := tx.Table("media.media_asset").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", i.AssetID).Take(&asset)
	if result.Error != nil {
		return domain.ReferencePin{}, result.Error
	}
	if asset.IsDelete || asset.Status != "ready" || asset.ModerationStatus != "passed" ||
		asset.ContainsRealPerson && asset.ConsentRecordID == nil || asset.ProjectID != i.ProjectID || asset.ObjectLocationID != i.LocationID || asset.ByteSize != i.ByteSize || asset.SHA256 != i.SHA256 || asset.Revision != i.AssetRevision {
		return domain.ReferencePin{}, domain.ErrReferencePinConflict
	}
	if err := requireNoPurgeReservation(tx, []uuid.UUID{i.AssetID}); err != nil {
		return domain.ReferencePin{}, fmt.Errorf("%w: %w", domain.ErrReferencePinConflict, err)
	}
	var location struct {
		State, SHA256 string
		ByteSize      *int64
	}
	if err := tx.Table("media.object_location").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", i.LocationID).Take(&location).Error; err != nil {
		return domain.ReferencePin{}, err
	}
	if location.State != "accepted" || location.ByteSize == nil || *location.ByteSize != i.ByteSize || location.SHA256 != i.SHA256 {
		return domain.ReferencePin{}, domain.ErrReferencePinConflict
	}
	if len(asset.ModerationDetail) == 0 {
		asset.ModerationDetail = json.RawMessage("null")
	}
	return domain.ReferencePin{Identity: i, AcquireCommandID: req.Command.ID, Kind: asset.Kind, ModerationStatus: asset.ModerationStatus, ModerationDetail: asset.ModerationDetail, AIGCMarked: asset.AIGCMarked, ContainsRealPerson: asset.ContainsRealPerson, ConsentRecordID: asset.ConsentRecordID, State: "held"}, nil
}

// Resolve saves a fresh, independently fenced snapshot without changing Acquire.
func (s *ReferencePinStore) Resolve(ctx context.Context, req application.ReferencePinRequest) (domain.ReferencePin, error) {
	return s.pinAction(ctx, req, "resolve")
}

// Bind retains an exact adoption receipt verified by the original consumer.
func (s *ReferencePinStore) Bind(ctx context.Context, req application.ReferencePinRequest) (domain.ReferencePin, error) {
	return s.pinAction(ctx, req, "bind")
}

// Release accepts only terminal non-adoption or durable exact reference removal.
func (s *ReferencePinStore) Release(ctx context.Context, req application.ReferencePinRequest) (domain.ReferencePin, error) {
	return s.pinAction(ctx, req, "release")
}

func (s *ReferencePinStore) pinAction(ctx context.Context, req application.ReferencePinRequest, action string) (domain.ReferencePin, error) {
	if err := req.Validate(); err != nil {
		return domain.ReferencePin{}, err
	}
	digest, replay, err := s.prepare(ctx, req.Command, action, req.Identity)
	if err != nil {
		return domain.ReferencePin{}, err
	}
	if replay != nil {
		if replay.Pin == nil {
			return domain.ReferencePin{}, domain.ErrReferencePinConflict
		}
		return *replay.Pin, nil
	}
	var proof domain.ReferencePinProof
	var proofDigest string
	if action != "resolve" {
		if s.verifier == nil {
			return domain.ReferencePin{}, domain.ErrReferencePinProof
		}
		pin, err := loadReferencePin(s.db.WithContext(ctx), req.Identity, false)
		if err != nil {
			return domain.ReferencePin{}, err
		}
		if action == "bind" {
			proof, err = s.verifier.VerifyAdoption(ctx, pin)
		} else {
			proof, err = s.verifier.VerifyRelease(ctx, pin)
		}
		if err != nil {
			return domain.ReferencePin{}, fmt.Errorf("verify consumer pin evidence: %w", err)
		}
		proofDigest, err = verifiedReferenceProof(pin, proof, action)
		if err != nil {
			return domain.ReferencePin{}, err
		}
	}
	receipt, err := s.transact(ctx, req.Command, digest, func(tx *gorm.DB) (pinCommandReceipt, error) {
		pin, err := loadReferencePin(tx, req.Identity, true)
		if err != nil {
			return pinCommandReceipt{}, err
		}
		if action == "resolve" {
			return pinCommandReceipt{Pin: &pin}, nil
		}
		// Recheck immutable proof binding after acquiring the local lock. The
		// consuming adapter guarantees its terminal/adoption facts survive this gap.
		if _, err := verifiedReferenceProof(pin, proof, action); err != nil {
			return pinCommandReceipt{}, err
		}
		if action == "bind" {
			if pin.State == "released" || pin.State == "bound" && pin.AdoptionReceiptSHA256 != proofDigest {
				return pinCommandReceipt{}, domain.ErrReferencePinConflict
			}
			pin.State, pin.AdoptionReceiptSHA256 = "bound", proofDigest
		} else {
			if pin.State == "bound" && proof.Outcome != "removed" || pin.State == "released" && (pin.ReleaseReceiptSHA256 != proofDigest || pin.ReleaseOutcome != proof.Outcome) {
				return pinCommandReceipt{}, domain.ErrReferencePinConflict
			}
			pin.State, pin.ReleaseReceiptSHA256, pin.ReleaseOutcome = "released", proofDigest, proof.Outcome
		}
		update := tx.Model(&referencePinRow{}).Where("pin_id = ?", pin.Identity.PinID).Updates(map[string]any{"state": pin.State, "adoption_receipt_sha256": pin.AdoptionReceiptSHA256, "release_receipt_sha256": pin.ReleaseReceiptSHA256, "release_outcome": pin.ReleaseOutcome})
		if update.Error != nil {
			return pinCommandReceipt{}, update.Error
		}
		if update.RowsAffected != 1 {
			return pinCommandReceipt{}, domain.ErrReferencePinConflict
		}
		return pinCommandReceipt{Pin: &pin}, nil
	})
	if err != nil {
		return domain.ReferencePin{}, err
	}
	return *receipt.Pin, nil
}

func verifiedReferenceProof(pin domain.ReferencePin, proof domain.ReferencePinProof, action string) (string, error) {
	if proof.Identity != pin.Identity || proof.AcquireCommandID != pin.AcquireCommandID || proof.SnapshotSHA256 == "" || proof.SnapshotSHA256 != pin.SnapshotSHA256() {
		return "", domain.ErrReferencePinProof
	}
	if action == "bind" && proof.Outcome != "adopted" || action == "release" && proof.Outcome != "not_adopted_frozen" && proof.Outcome != "removed" {
		return "", domain.ErrReferencePinProof
	}
	raw, err := writefence.CanonicalReceipt(proof.Receipt)
	if err != nil {
		return "", domain.ErrReferencePinProof
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// CloseAcquisition permanently stops new pins; it does not assert that old
// references have migrated or that any physical object can safely be deleted.
func (s *ReferencePinStore) CloseAcquisition(ctx context.Context, req application.CloseReferenceAcquisitionRequest) (domain.ReferenceAcquisition, error) {
	if req.AssetID == uuid.Nil || req.Epoch < 1 {
		return domain.ReferenceAcquisition{}, domain.ErrInvalidReferencePin
	}
	payload := struct {
		AssetID uuid.UUID
		Epoch   int64
	}{req.AssetID, req.Epoch}
	digest, replay, err := s.prepare(ctx, req.Command, "close", payload)
	if err != nil {
		return domain.ReferenceAcquisition{}, err
	}
	if replay != nil {
		if replay.Gate == nil {
			return domain.ReferenceAcquisition{}, domain.ErrReferencePinConflict
		}
		return *replay.Gate, nil
	}
	receipt, err := s.transact(ctx, req.Command, digest, func(tx *gorm.DB) (pinCommandReceipt, error) {
		gate, err := lockReferenceGate(tx, req.AssetID)
		if err != nil {
			return pinCommandReceipt{}, err
		}
		if gate.Closed || gate.Epoch != req.Epoch {
			return pinCommandReceipt{}, domain.ErrReferencePinClosed
		}
		var asset struct{ ProjectID uuid.UUID }
		if err := tx.Table("media.media_asset").Clauses(clause.Locking{Strength: "SHARE"}).Select("project_id").Where("id = ?", req.AssetID).Take(&asset).Error; err != nil {
			return pinCommandReceipt{}, err
		}
		var sameProject bool
		for _, authority := range req.Command.Authorities {
			sameProject = sameProject || authority.Issuer == writefence.Creation && authority.Kind == writefence.Project && authority.ID == asset.ProjectID
		}
		if !sameProject || asset.ProjectID == uuid.Nil {
			return pinCommandReceipt{}, domain.ErrInvalidReferencePin
		}
		if err := tx.Model(&referenceGateRow{}).Where("asset_id = ?", req.AssetID).Updates(map[string]any{"closed": true, "epoch": gate.Epoch + 1, "close_command_id": req.Command.ID}).Error; err != nil {
			return pinCommandReceipt{}, err
		}
		closed := domain.ReferenceAcquisition{AssetID: req.AssetID, Epoch: gate.Epoch + 1, Closed: true, CloseCommandID: &req.Command.ID}
		return pinCommandReceipt{Gate: &closed}, nil
	})
	if err != nil {
		return domain.ReferenceAcquisition{}, err
	}
	return *receipt.Gate, nil
}
