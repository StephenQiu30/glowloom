package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// ImportSourcesFactory retains media frozen-original locks in the owner transaction.
type ImportSourcesFactory func(*gorm.DB) application.SourceAssetReader

// ImportStore owns immutable original manifests and small state/attempt fences.
type ImportStore struct {
	db      *gorm.DB
	access  ProjectAccessFactory
	sources ImportSourcesFactory
}

// NewImportStore injects current project authorization and owning document sources.
func NewImportStore(db *gorm.DB, access ProjectAccessFactory, sources ImportSourcesFactory) *ImportStore {
	return &ImportStore{db: db, access: access, sources: sources}
}

func (s *ImportStore) transaction(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool, f func(*gorm.DB) error) error {
	return NewSourceStore(s.db, s.access).transaction(ctx, actor, project, write, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		return f(tx)
	})
}

func importHash(in any) (string, error) {
	data, err := json.Marshal(in)
	return domain.ContentSHA(data), err
}

func importReplay(tx *gorm.DB, actor identityapp.Principal, project, key uuid.UUID, action, hash string) (application.ImportJob, bool, error) {
	if err := checkRequestScope(tx, actor, project, key, "file_import_"+action, hash); err != nil {
		return application.ImportJob{}, false, err
	}
	var row struct{ Response string }
	read := tx.Raw(`SELECT response::text FROM script.import_command WHERE actor_id=? AND request_id=?`, actor.ID, key).Scan(&row)
	if read.Error != nil {
		return application.ImportJob{}, false, read.Error
	}
	if read.RowsAffected == 0 {
		return application.ImportJob{}, false, nil
	}
	var result application.ImportJob
	if err := json.Unmarshal([]byte(row.Response), &result); err != nil {
		return result, false, application.ErrUnavailable
	}
	return result, true, nil
}

// CreateImport commits all frozen inputs and the command outbox without content writes.
func (s *ImportStore) CreateImport(ctx context.Context, actor identityapp.Principal, in application.ImportCommand, now time.Time) (application.ImportJob, error) {
	var result application.ImportJob
	hashed := in
	hashed.RequestID = uuid.Nil
	hash, err := importHash(hashed)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, in.ProjectID, true, func(tx *gorm.DB) error {
		if err := commandLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		prior, found, err := importReplay(tx, actor, in.ProjectID, in.Key, "create", hash)
		if err != nil {
			return err
		}
		if found {
			result = prior
			return nil
		}
		if s.sources == nil {
			return application.ErrUnavailable
		}
		state, err := readState(tx, actor.OrgID, in.ProjectID, true)
		if err != nil {
			return err
		}
		if state.Revision != in.ExpectedRevision || !sameOptionalUUID(state.DraftVersionID, in.BaseVersionID) {
			return application.ErrConflict
		}
		if err := rejectPendingImport(tx, actor.OrgID, in.ProjectID, uuid.Nil); err != nil {
			return err
		}
		if err := rejectPendingWrite(tx, actor.OrgID, in.ProjectID); err != nil {
			return err
		}
		reader := s.sources(tx)
		if reader == nil {
			return application.ErrUnavailable
		}
		frozen, err := reader.Freeze(ctx, actor, in.ProjectID, in.AssetIDs)
		if err != nil {
			return err
		}
		if len(frozen) != len(in.AssetIDs) {
			return application.ErrUnavailable
		}
		id := uuid.NewSHA1(in.Key, []byte("file-import/"+actor.ID.String()+"/"+in.ProjectID.String()))
		if err := registerRequest(tx, actor, in.ProjectID, in.Key, "file_import_create", hash, now); err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`INSERT INTO script.import_job(id,org_id,project_id,actor_id,actor_role,request_id,request_hash,expected_script_revision,base_version_id,rights_confirmed_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, actor.OrgID, in.ProjectID, actor.ID, actor.Role, in.Key, hash, in.ExpectedRevision, in.BaseVersionID, now, now)); err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`INSERT INTO script.import_state(job_id,revision,attempt,status,stage,latest_script_revision,latest_version_id,io_state,updated_at) VALUES(?,1,1,'queued','queued',?,?,'idle',?)`, id, in.ExpectedRevision, in.BaseVersionID, now)); err != nil {
			return err
		}
		positions := make([]int, len(frozen))
		for i, file := range frozen {
			if file.AssetID != in.AssetIDs[i] || file.ProjectID != in.ProjectID {
				return application.ErrUnavailable
			}
			positions[i] = i
			sourceID := uuid.NewSHA1(id, []byte(fmt.Sprintf("file/%d", i)))
			if err := exactlyOne(tx.Exec(`INSERT INTO script.import_file(job_id,position,source_id,asset_id,project_id,media_revision,sha256,byte_size,mime,file_name) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, i, sourceID, file.AssetID, file.ProjectID, file.Revision, file.SHA256, file.ByteSize, file.MIME, file.FileName)); err != nil {
				return err
			}
		}
		if err := insertImportAttempt(tx, id, 1, in.ExpectedRevision, in.BaseVersionID, positions, now); err != nil {
			return err
		}
		r, err := readImport(tx, actor, in.ProjectID, id, false)
		if err != nil {
			return err
		}
		result = r.Job
		return recordImportCommand(tx, actor, in.Key, in.RequestID, "create", hash, 0, result, "start", now)
	})
	return result, err
}

func sameOptionalUUID(a, b *uuid.UUID) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func rejectPendingImport(tx *gorm.DB, org, project, except uuid.UUID) error {
	var found bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.import_job j JOIN script.import_state s ON s.job_id=j.id WHERE j.org_id=? AND j.project_id=? AND j.id<>? AND (s.status IN ('queued','running','cancel_requested') OR s.needs_reconciliation OR s.io_owner_id IS NOT NULL OR s.io_state='unknown'))`, org, project, except).Scan(&found).Error; err != nil {
		return err
	}
	if found {
		return application.ErrNeedsReconciliation
	}
	return nil
}

func insertImportAttempt(tx *gorm.DB, id uuid.UUID, attempt int, revision int64, version *uuid.UUID, positions []int, now time.Time) error {
	data, err := json.Marshal(positions)
	if err != nil {
		return err
	}
	key := uuid.NewSHA1(id, []byte(fmt.Sprintf("publication/%d", attempt)))
	return exactlyOne(tx.Exec(`INSERT INTO script.import_attempt(job_id,attempt,expected_script_revision,base_version_id,publication_key,positions,created_at) VALUES(?,?,?,?,?,ARRAY(SELECT jsonb_array_elements_text(?::jsonb)::integer),?)`, id, attempt, revision, version, key, string(data), now))
}

// ListImports provides a current scoped recovery path after unknown acceptance.
func (s *ImportStore) ListImports(ctx context.Context, actor identityapp.Principal, project uuid.UUID, after int64, limit int) (application.ImportPage, error) {
	page := application.ImportPage{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, Items: []application.ImportJob{}}
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB) error {
		var ids []uuid.UUID
		if err := tx.Raw(`SELECT id FROM script.import_job WHERE org_id=? AND project_id=? ORDER BY created_at DESC,id DESC OFFSET ? LIMIT ?`, actor.OrgID, project, after, limit+1).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) > limit {
			ids = ids[:limit]
			next := after + int64(limit)
			page.NextAfter = &next
		}
		for _, id := range ids {
			r, err := readImport(tx, actor, project, id, false)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, r.Job)
		}
		return nil
	})
	return page, err
}

// Import exposes actual file outcomes, excluding object keys and editable bodies.
func (s *ImportStore) Import(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (application.ImportJob, error) {
	var result application.ImportJob
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB) error {
		r, err := readImport(tx, actor, project, id, false)
		result = r.Job
		return err
	})
	return result, err
}

// ControlImport admits exact current controller intent without publishing old content.
func (s *ImportStore) ControlImport(ctx context.Context, actor identityapp.Principal, in application.ImportControl, now time.Time) (application.ImportJob, error) {
	var result application.ImportJob
	hashed := in
	hashed.RequestID = uuid.Nil
	hash, err := importHash(hashed)
	if err != nil {
		return result, err
	}
	err = s.transaction(ctx, actor, in.ProjectID, true, func(tx *gorm.DB) error {
		if err := commandLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		prior, found, err := importReplay(tx, actor, in.ProjectID, in.Key, in.Action, hash)
		if err != nil {
			return err
		}
		if found {
			result = prior
			return nil
		}
		r, err := readImport(tx, actor, in.ProjectID, in.JobID, true)
		if err != nil {
			return err
		}
		if actor.ID != r.Actor.ID && actor.Role != identitydomain.RoleAdmin || in.Action == "retry" && actor.ID != r.Actor.ID {
			return identityapp.ErrForbidden
		}
		if r.Job.Revision != in.ExpectedRevision {
			return application.ErrConflict
		}
		j := r.Job
		switch in.Action {
		case "cancel":
			if j.Status == "succeeded" || j.Status == "cancelled" {
				return application.ErrConflict
			}
			j.CancellationRequested = true
			j.Status = "cancel_requested"
			j.Stage = "cancelling"
		case "reconcile":
			if !j.NeedsReconciliation && !j.CancellationRequested || j.ReconciliationRequested {
				return application.ErrConflict
			}
			j.ReconciliationRequested = true
		case "retry":
			if s.sources == nil {
				return application.ErrUnavailable
			}
			if !j.Retryable || j.Attempt >= 100 {
				return application.ErrConflict
			}
			state, err := readState(tx, actor.OrgID, in.ProjectID, true)
			if err != nil {
				return err
			}
			if state.Revision != j.LatestScriptRevision || !sameOptionalUUID(state.DraftVersionID, j.LatestVersionID) {
				return application.ErrConflict
			}
			if err := rejectPendingImport(tx, actor.OrgID, in.ProjectID, j.ID); err != nil {
				return err
			}
			if err := rejectPendingSourceExcept(tx, actor.OrgID, in.ProjectID, uuid.Nil); err != nil {
				return err
			}
			positions := []int{}
			for _, file := range j.Files {
				if file.Status == "failed" {
					positions = append(positions, file.Position)
				}
			}
			j.Attempt++
			j.Status = "queued"
			j.Stage = "queued"
			j.FailureCode = ""
			j.CancellationRequested = false
			j.ReconciliationRequested = false
			if err := insertImportAttempt(tx, j.ID, j.Attempt, j.LatestScriptRevision, j.LatestVersionID, positions, now); err != nil {
				return err
			}
		default:
			return domain.ErrInvalidSource
		}
		j.Revision++
		j.UpdatedAt = now
		if err := updateImportState(tx, j, r.OwnerID, r.IOState); err != nil {
			return err
		}
		if err := registerRequest(tx, actor, in.ProjectID, in.Key, "file_import_"+in.Action, hash, now); err != nil {
			return err
		}
		refreshed, err := readImport(tx, actor, in.ProjectID, in.JobID, false)
		if err != nil {
			return err
		}
		result = refreshed.Job
		emit := in.Action
		if emit == "retry" {
			emit = "start"
		}
		return recordImportCommand(tx, actor, in.Key, in.RequestID, in.Action, hash, in.ExpectedRevision, result, emit, now)
	})
	return result, err
}

func (s *ImportStore) configured() error {
	if s == nil || s.db == nil || s.access == nil {
		return application.ErrUnavailable
	}
	return nil
}

var _ application.ImportStore = (*ImportStore)(nil)
