package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// ProjectCopyAccessFactory binds the workspace-minted phase and worker to each own transaction.
type ProjectCopyAccessFactory func(*gorm.DB) application.ProjectCopyAccess

// ProjectCopyStore owns immutable full-history snapshots and exact object proof state.
type ProjectCopyStore struct {
	db     *gorm.DB
	access ProjectCopyAccessFactory
}

// NewProjectCopyStore injects caller-owned SQL for Freeze/Register or a pool for transfer.
func NewProjectCopyStore(db *gorm.DB, access ProjectCopyAccessFactory) *ProjectCopyStore {
	return &ProjectCopyStore{db: db, access: access}
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
		manifest, err := application.RemapProjectHistory(b, history, assets)
		if err != nil {
			return err
		}
		result, err = copySnapshot(manifest)
		if err != nil {
			return err
		}
		data, err := copyManifestJSON(manifest)
		if err != nil {
			return err
		}
		if err := copyInsert(tx, "script.copy_snapshot", map[string]any{"id": result.ID, "job_id": b.JobID, "org_id": b.OrgID, "source_project_id": b.SourceProjectID, "target_project_id": b.TargetProjectID, "manifest": gorm.Expr("?::jsonb", string(data)), "manifest_sha256": result.ManifestSHA256, "content_sha256": result.ContentSHA256, "created_at": at.UTC()}); err != nil {
			return fmt.Errorf("freeze complete script history: %w", err)
		}
		for _, object := range manifest.Objects {
			if err := copyInsert(tx, "script.copy_object_intent", map[string]any{"snapshot_id": result.ID, "source_key": object.Source.Key, "target_key": object.Target.Key, "sha256": object.Source.SHA256, "byte_size": object.Source.ByteSize, "mime": object.Source.MIME}); err != nil {
				return err
			}
			if err := copyInsert(tx, "script.copy_object_state", map[string]any{"target_key": object.Target.Key, "put_started": false, "confirmed": false, "removed": false, "updated_at": at.UTC()}); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func readCopyManifest(tx *gorm.DB, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.ProjectCopyManifest, error) {
	var row struct {
		JobID, OrgID, SourceProjectID, TargetProjectID uuid.UUID
		Manifest, ManifestSHA256, ContentSHA256        string
	}
	read := tx.Raw(`SELECT job_id,org_id,source_project_id,target_project_id,manifest::text,manifest_sha256,content_sha256 FROM script.copy_snapshot WHERE id=?`, snapshot.ID).Scan(&row)
	if read.Error != nil {
		return application.ProjectCopyManifest{}, read.Error
	}
	if read.RowsAffected != 1 {
		return application.ProjectCopyManifest{}, application.ErrNotFound
	}
	if row.JobID != b.JobID || row.OrgID != b.OrgID || row.SourceProjectID != b.SourceProjectID || row.TargetProjectID != b.TargetProjectID || row.ManifestSHA256 != snapshot.ManifestSHA256 || row.ContentSHA256 != snapshot.ContentSHA256 {
		return application.ProjectCopyManifest{}, application.ErrObjectMismatch
	}
	var m application.ProjectCopyManifest
	if err := json.Unmarshal([]byte(row.Manifest), &m); err != nil {
		return m, application.ErrUnavailable
	}
	proof, err := copySnapshot(m)
	if err != nil {
		return m, err
	}
	if m.Binding != b || proof != snapshot {
		return m, application.ErrObjectMismatch
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
	var row struct {
		SHA256, MIME                   string
		ByteSize                       int64
		PutStarted, Confirmed, Removed bool
	}
	read := tx.Raw(`SELECT i.sha256,i.byte_size,i.mime,s.put_started,s.confirmed,s.removed FROM script.copy_object_intent i JOIN script.copy_object_state s ON s.target_key=i.target_key WHERE i.snapshot_id=? AND i.target_key=? FOR UPDATE OF s`, snapshot.ID, fact.Key).Scan(&row)
	if read.Error != nil {
		return copyObjectState{}, read.Error
	}
	if read.RowsAffected != 1 || row.SHA256 != fact.SHA256 || row.ByteSize != fact.ByteSize || row.MIME != fact.MIME {
		return copyObjectState{}, application.ErrObjectMismatch
	}
	return copyObjectState{row.PutStarted, row.Confirmed, row.Removed}, nil
}
func exactCopyUpdate(write *gorm.DB) error {
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return application.ErrConflict
	}
	return nil
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
		return exactCopyUpdate(tx.Exec(`UPDATE script.copy_object_state SET put_started=true,updated_at=? WHERE target_key=?`, time.Now().UTC(), fact.Key))
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
		return exactCopyUpdate(tx.Exec(`UPDATE script.copy_object_state SET confirmed=true,updated_at=? WHERE target_key=?`, time.Now().UTC(), fact.Key))
	})
}
func copyObjectsComplete(tx *gorm.DB, snapshot application.ProjectCopySnapshot, removed bool) error {
	var row struct{ Total, Complete int }
	predicate := "confirmed AND NOT removed"
	if removed {
		predicate = "removed"
	}
	err := tx.Raw(`SELECT count(*) AS total,count(*) FILTER(WHERE `+predicate+`) AS complete FROM script.copy_object_intent i JOIN script.copy_object_state s ON s.target_key=i.target_key WHERE i.snapshot_id=?`, snapshot.ID).Scan(&row).Error
	if err != nil {
		return err
	}
	if row.Total != snapshot.Counts.Objects || row.Complete != row.Total {
		return application.ErrNeedsReconciliation
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
		var old struct{ Receipt string }
		read := tx.Raw(`SELECT receipt::text FROM script.copy_receipt WHERE snapshot_id=?`, snapshot.ID).Scan(&old)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 0 {
			actual, err := readCopyHistory(tx, b.OrgID, b.TargetProjectID)
			if err != nil {
				return err
			}
			if actual.State != nil || len(actual.Sources)+len(actual.Versions)+len(actual.Episodes)+len(actual.Structures) != 0 {
				return application.ErrConflict
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
		if read.RowsAffected != 0 {
			var prior application.ProjectCopyReceipt
			if json.Unmarshal([]byte(old.Receipt), &prior) != nil || prior != receipt {
				return application.ErrObjectMismatch
			}
			return nil
		}
		data, err := copyJSONValue(receipt)
		if err != nil {
			return err
		}
		return copyInsert(tx, "script.copy_receipt", map[string]any{"snapshot_id": snapshot.ID, "receipt": data, "created_at": time.Now().UTC()})
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
	return s.ownTransaction(ctx, actor, b, false, func(tx *gorm.DB) error {
		if _, err := exactCopyObject(tx, b, snapshot, fact); err != nil {
			return err
		}
		return exactCopyUpdate(tx.Exec(`UPDATE script.copy_object_state SET confirmed=true,updated_at=? WHERE target_key=?`, time.Now().UTC(), fact.Key))
	})
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
		return exactCopyUpdate(tx.Exec(`UPDATE script.copy_object_state SET removed=true,updated_at=? WHERE target_key=?`, time.Now().UTC(), fact.Key))
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
