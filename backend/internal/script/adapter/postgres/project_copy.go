package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// ProjectCopyAccessFactory binds the workspace-minted phase and worker to each own transaction.
type ProjectCopyAccessFactory func(*gorm.DB) application.ProjectCopyAccess

// ProjectCopyStore owns immutable full-history snapshots and exact object proof state.
type ProjectCopyStore struct {
	db         *gorm.DB
	access     ProjectCopyAccessFactory
	characters ProjectCopyCharacterFactory
	locations  SourceObjectLocationFactory
}

// NewProjectCopyStore injects caller-owned SQL for Freeze/Register or a pool for transfer.
func NewProjectCopyStore(db *gorm.DB, access ProjectCopyAccessFactory) *ProjectCopyStore {
	return &ProjectCopyStore{db: db, access: access}
}

// ProjectCopyCharacterFactory binds exact frozen Bible identities to caller-owned admission SQL.
type ProjectCopyCharacterFactory func(*gorm.DB) application.ProjectCopyCharacters

// NewProjectCopyStoreWithCharacters requires the real Bible owner for non-nil historical references.
func NewProjectCopyStoreWithCharacters(db *gorm.DB, access ProjectCopyAccessFactory, characters ProjectCopyCharacterFactory) *ProjectCopyStore {
	return &ProjectCopyStore{db: db, access: access, characters: characters}
}

func (s *ProjectCopyStore) authorize(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, b application.ProjectCopyBinding) error {
	if s.access == nil {
		return application.ErrUnavailable
	}
	access := s.access(tx)
	if access == nil {
		return application.ErrUnavailable
	}
	if err := access.Authorize(ctx, actor, b, false); err != nil {
		return err
	}
	return access.Authorize(ctx, actor, b, true)
}
func (s *ProjectCopyStore) ownTransaction(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, caller bool, f func(*gorm.DB) error) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return application.ErrUnavailable
	}
	_, existing := s.db.Statement.ConnPool.(gorm.TxCommitter)
	run := func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, b); err != nil {
			return err
		}
		return f(tx)
	}
	if caller {
		if !existing {
			return application.ErrUnavailable
		}
		return run(s.db.WithContext(ctx))
	}
	if existing {
		return run(s.db.WithContext(ctx))
	}
	return s.db.WithContext(ctx).Transaction(run)
}
func copyManifestJSON(m application.ProjectCopyManifest) ([]byte, error) { return json.Marshal(m) }
func copySnapshot(m application.ProjectCopyManifest) (application.ProjectCopySnapshot, error) {
	data, err := copyManifestJSON(m)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	return application.ProjectCopySnapshot{ID: uuid.NewSHA1(m.Binding.JobID, []byte("script-history")), ManifestSHA256: domain.ContentSHA(data), ContentSHA256: m.ContentSHA256, Counts: m.Counts}, nil
}

// Freeze runs on the admission transaction after the trusted target has been minted.
func (s *ProjectCopyStore) Freeze(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, assets map[uuid.UUID]uuid.UUID, at time.Time) (application.ProjectCopySnapshot, error) {
	var result application.ProjectCopySnapshot
	err := s.ownTransaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		if err := rejectPendingWrite(tx, b.OrgID, b.SourceProjectID); err != nil {
			return err
		}
		history, err := readCopyHistory(tx, b.OrgID, b.SourceProjectID)
		if err != nil {
			return err
		}
		var characters []application.ProjectCopyCharacterMapping
		if refs := application.ProjectHistoryCharacterReferences(history); len(refs) > 0 {
			if s.characters == nil || s.characters(tx) == nil {
				return application.ErrContextUnavailable
			}
			characters, err = s.characters(tx).FreezeCharacters(ctx, actor, b, refs)
			if err != nil {
				return err
			}
		}
		manifest, err := application.RemapProjectHistoryWithCharacters(b, history, assets, characters)
		if err != nil {
			return err
		}
		result, err = copySnapshot(manifest)
		if err != nil {
			return err
		}
		var storageID *uuid.UUID
		if len(manifest.Objects) > 0 {
			if s.locations == nil || s.locations(tx) == nil {
				return application.ErrUnavailable
			}
			facts := make([]domain.ObjectFact, len(manifest.Objects))
			for i, o := range manifest.Objects {
				facts[i] = o.Target
				if o.Source.LocationID == uuid.Nil {
					return application.ErrObjectMismatch
				}
			}
			id, bound, err := s.locations(tx).Admit(ctx, b.TargetProjectID, result.ID, facts)
			if err != nil {
				return err
			}
			if id == uuid.Nil || len(bound) != len(facts) {
				return application.ErrObjectMismatch
			}
			for _, fact := range bound {
				if fact.LocationID == uuid.Nil {
					return application.ErrObjectMismatch
				}
			}
			storageID = &id
			bindCopyTargets(&manifest, bound)
		}
		data, err := copyManifestJSON(manifest)
		if err != nil {
			return err
		}
		snapshotRow := scriptCopySnapshotRow{ID: result.ID, JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID, Manifest: scriptJSON(data), ManifestSHA256: result.ManifestSHA256, ContentSHA256: result.ContentSHA256, CreatedAt: at.UTC(), StorageIntentID: storageID}
		if err := exactlyOne(tx.Create(&snapshotRow)); err != nil {
			return fmt.Errorf("freeze complete script history: %w", err)
		}
		for _, object := range manifest.Objects {
			if object.Source.LocationID == uuid.Nil || object.Target.LocationID == uuid.Nil {
				return application.ErrObjectMismatch
			}
			intent := scriptCopyObjectIntentRow{SnapshotID: result.ID, SourceKey: object.Source.Key, TargetKey: object.Target.Key, SHA256: object.Source.SHA256, ByteSize: object.Source.ByteSize, MIME: object.Source.MIME, SourceLocationID: object.Source.LocationID, TargetLocationID: object.Target.LocationID}
			if err := exactlyOne(tx.Create(&intent)); err != nil {
				return err
			}
			state := scriptCopyObjectStateRow{TargetKey: object.Target.Key, UpdatedAt: at.UTC(), TargetLocationID: object.Target.LocationID}
			if err := exactlyOne(tx.Create(&state)); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func readCopyManifest(tx *gorm.DB, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.ProjectCopyManifest, error) {
	var row scriptCopySnapshotRow
	if err := tx.Where("id = ?", snapshot.ID).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ProjectCopyManifest{}, application.ErrNotFound
	} else if err != nil {
		return application.ProjectCopyManifest{}, err
	}
	if row.JobID != b.JobID || row.OrgID != b.OrgID || row.SourceProjectID != b.SourceProjectID || row.TargetProjectID != b.TargetProjectID || row.ManifestSHA256 != snapshot.ManifestSHA256 || row.ContentSHA256 != snapshot.ContentSHA256 {
		return application.ProjectCopyManifest{}, application.ErrObjectMismatch
	}
	var m application.ProjectCopyManifest
	if err := json.Unmarshal(row.Manifest, &m); err != nil {
		return m, application.ErrUnavailable
	}
	proof, err := copySnapshot(m)
	if err != nil {
		return m, err
	}
	if m.Binding != b || proof != snapshot {
		return m, application.ErrObjectMismatch
	}
	if err := hydrateCopyLocations(tx, &m, snapshot); err != nil {
		return m, err
	}
	return m, nil
}

// Manifest rechecks the current actual worker before exposing private frozen facts.
func (s *ProjectCopyStore) Manifest(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.ProjectCopyManifest, error) {
	var m application.ProjectCopyManifest
	err := s.ownTransaction(ctx, actor, b, false, func(tx *gorm.DB) error { var err error; m, err = readCopyManifest(tx, b, snapshot); return err })
	return m, err
}

type copyObjectState struct{ PutStarted, Confirmed, Removed bool }

func exactCopyObject(tx *gorm.DB, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, fact domain.ObjectFact) (copyObjectState, error) {
	if _, err := readCopyManifest(tx, b, snapshot); err != nil {
		return copyObjectState{}, err
	}
	if fact.LocationID == uuid.Nil {
		return copyObjectState{}, application.ErrObjectMismatch
	}
	var intent scriptCopyObjectIntentRow
	if err := tx.Where("snapshot_id = ? AND target_key = ? AND target_location_id = ?", snapshot.ID, fact.Key, fact.LocationID).Take(&intent).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return copyObjectState{}, application.ErrObjectMismatch
	} else if err != nil {
		return copyObjectState{}, err
	}
	if intent.SHA256 != fact.SHA256 || intent.ByteSize != fact.ByteSize || intent.MIME != fact.MIME {
		return copyObjectState{}, application.ErrObjectMismatch
	}
	var state scriptCopyObjectStateRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("target_key = ? AND target_location_id = ?", fact.Key, fact.LocationID).Take(&state).Error; err != nil {
		return copyObjectState{}, err
	}
	return copyObjectState{state.PutStarted, state.Confirmed, state.Removed}, nil
}

// BeginObject durably records the exact owned key before any remote put attempt.
func (s *ProjectCopyStore) BeginObject(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, fact domain.ObjectFact) error {
	return s.ownTransaction(ctx, actor, b, false, func(tx *gorm.DB) error {
		state, err := exactCopyObject(tx, b, snapshot, fact)
		if err != nil {
			return err
		}
		if state.Removed {
			return application.ErrConflict
		}
		if _, err := s.copyLocationAction(ctx, tx, snapshot, fact, copyObjectBegin); err != nil {
			return err
		}
		updated := tx.Model(&scriptCopyObjectStateRow{}).Where("target_key = ? AND target_location_id = ?", fact.Key, fact.LocationID).
			Updates(map[string]any{"put_started": true, "updated_at": time.Now().UTC()})
		return exactlyOne(updated)
	})
}

// CompleteObject records a real full read proof; absent/removed objects cannot be published.
func (s *ProjectCopyStore) CompleteObject(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, fact domain.ObjectFact) error {
	return s.ownTransaction(ctx, actor, b, false, func(tx *gorm.DB) error {
		state, err := exactCopyObject(tx, b, snapshot, fact)
		if err != nil {
			return err
		}
		if !state.PutStarted || state.Removed {
			return application.ErrConflict
		}
		if _, err := s.copyLocationAction(ctx, tx, snapshot, fact, copyObjectConfirm); err != nil {
			return err
		}
		updated := tx.Model(&scriptCopyObjectStateRow{}).Where("target_key = ? AND target_location_id = ?", fact.Key, fact.LocationID).
			Updates(map[string]any{"confirmed": true, "updated_at": time.Now().UTC()})
		return exactlyOne(updated)
	})
}
func copyObjectsComplete(tx *gorm.DB, snapshot application.ProjectCopySnapshot, removed bool) error {
	var intents []scriptCopyObjectIntentRow
	if err := tx.Where("snapshot_id = ?", snapshot.ID).Find(&intents).Error; err != nil {
		return err
	}
	if len(intents) != snapshot.Counts.Objects {
		return application.ErrNeedsReconciliation
	}
	for _, intent := range intents {
		var state scriptCopyObjectStateRow
		if err := tx.Where("target_key = ? AND target_location_id = ?", intent.TargetKey, intent.TargetLocationID).Take(&state).Error; err != nil {
			return err
		}
		if removed && !state.Removed || !removed && (!state.Confirmed || state.Removed) {
			return application.ErrNeedsReconciliation
		}
	}
	return nil
}

// Register preserves every immutable history row and verifies actual replayed target facts.
func (s *ProjectCopyStore) Register(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.ProjectCopyReceipt, error) {
	var receipt application.ProjectCopyReceipt
	err := s.ownTransaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		m, err := readCopyManifest(tx, b, snapshot)
		if err != nil {
			return err
		}
		if err := copyObjectsComplete(tx, snapshot, false); err != nil {
			return err
		}
		var old scriptCopyReceiptRow
		read := tx.Where("snapshot_id = ?", snapshot.ID).Take(&old)
		receiptExists := read.Error == nil
		if read.Error != nil && !errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return read.Error
		}
		if !receiptExists {
			actual, err := readCopyHistory(tx, b.OrgID, b.TargetProjectID)
			if err != nil {
				return err
			}
			if actual.State != nil || len(actual.Sources)+len(actual.Versions)+len(actual.Episodes)+len(actual.Structures) != 0 {
				return application.ErrConflict
			}
			var storageRow scriptCopySnapshotRow
			if err := tx.Model(&scriptCopySnapshotRow{}).Select("storage_intent_id").Where("id = ?", snapshot.ID).Take(&storageRow).Error; err != nil {
				return err
			}
			storageID := storageRow.StorageIntentID
			if len(m.Objects) > 0 {
				if storageID == nil || *storageID == uuid.Nil || s.locations == nil || s.locations(tx) == nil {
					return application.ErrUnavailable
				}
				facts := make([]domain.ObjectFact, len(m.Objects))
				for i, o := range m.Objects {
					facts[i] = o.Target
				}
				if err := s.locations(tx).Accept(ctx, *storageID, facts); err != nil {
					return err
				}
			}
			if err := insertCopyHistory(tx, m.Target); err != nil {
				return err
			}
		}
		actual, err := readCopyHistory(tx, b.OrgID, b.TargetProjectID)
		if err != nil {
			return err
		}
		alignCopyHistory(&actual, m.Target)
		expectedJSON, err := json.Marshal(m.Target)
		if err != nil {
			return err
		}
		actualJSON, err := json.Marshal(actual)
		if err != nil {
			return err
		}
		if string(expectedJSON) != string(actualJSON) {
			return fmt.Errorf("registered script history differs: %w", application.ErrObjectMismatch)
		}
		content, err := application.ProjectHistoryContentSHA(actual)
		if err != nil {
			return err
		}
		if content != snapshot.ContentSHA256 {
			return fmt.Errorf("registered script semantic history differs: %w", application.ErrObjectMismatch)
		}
		receipt = application.ProjectCopyReceipt{ManifestSHA256: snapshot.ManifestSHA256, ContentSHA256: snapshot.ContentSHA256, Counts: snapshot.Counts}
		if receiptExists {
			var prior application.ProjectCopyReceipt
			if json.Unmarshal(old.Receipt, &prior) != nil || prior != receipt {
				return application.ErrObjectMismatch
			}
			return nil
		}
		data, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		return exactlyOne(tx.Create(&scriptCopyReceiptRow{SnapshotID: snapshot.ID, Receipt: scriptJSON(data), CreatedAt: time.Now().UTC()}))
	})
	return receipt, err
}

// BeginCleanup selects only this unpublished target's frozen owned object intents.
func (s *ProjectCopyStore) BeginCleanup(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) ([]application.ProjectCopyObject, error) {
	var result []application.ProjectCopyObject
	err := s.ownTransaction(ctx, actor, b, false, func(tx *gorm.DB) error {
		m, err := readCopyManifest(tx, b, snapshot)
		if err != nil {
			return err
		}
		result = m.Objects
		for i := range result {
			state, err := exactCopyObject(tx, b, snapshot, result[i].Target)
			if err != nil {
				return err
			}
			result[i].Copied = state.Confirmed
			result[i].PutStarted = state.PutStarted
			result[i].Removed = state.Removed
		}
		return nil
	})
	return result, err
}

// BeginCleanupObject freezes a matching actual digest before attempting removal.
func (s *ProjectCopyStore) BeginCleanupObject(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, fact domain.ObjectFact) error {
	ready := true
	err := s.ownTransaction(ctx, actor, b, false, func(tx *gorm.DB) error {
		if _, err := exactCopyObject(tx, b, snapshot, fact); err != nil {
			return err
		}
		var err error
		ready, err = s.copyLocationAction(ctx, tx, snapshot, fact, copyObjectBeginRemoval)
		if err != nil {
			return err
		}
		if !ready {
			return nil
		}
		updated := tx.Model(&scriptCopyObjectStateRow{}).Where("target_key = ? AND target_location_id = ?", fact.Key, fact.LocationID).
			Updates(map[string]any{"confirmed": true, "updated_at": time.Now().UTC()})
		return exactlyOne(updated)
	})
	if err != nil {
		return err
	}
	if !ready {
		return application.ErrNeedsReconciliation
	}
	return nil
}

// CompleteCleanupObject retains absence proof while leaving historical SQL immutable.
func (s *ProjectCopyStore) CompleteCleanupObject(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, fact domain.ObjectFact) error {
	return s.ownTransaction(ctx, actor, b, false, func(tx *gorm.DB) error {
		state, err := exactCopyObject(tx, b, snapshot, fact)
		if err != nil {
			return err
		}
		if state.PutStarted && !state.Confirmed {
			return application.ErrNeedsReconciliation
		}
		if _, err := s.copyLocationAction(ctx, tx, snapshot, fact, copyObjectConfirmRemoval); err != nil {
			return err
		}
		updated := tx.Model(&scriptCopyObjectStateRow{}).Where("target_key = ? AND target_location_id = ?", fact.Key, fact.LocationID).
			Updates(map[string]any{"removed": true, "updated_at": time.Now().UTC()})
		return exactlyOne(updated)
	})
}

// FinishCleanup runs in the coordinator transaction only after every actual absence proof.
func (s *ProjectCopyStore) FinishCleanup(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) error {
	return s.ownTransaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		if _, err := readCopyManifest(tx, b, snapshot); err != nil {
			return err
		}
		return copyObjectsComplete(tx, snapshot, true)
	})
}
