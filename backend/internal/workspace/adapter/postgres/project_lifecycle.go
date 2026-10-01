package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectWorkFactory binds owning modules' work evidence to this store's transaction.
type ProjectWorkFactory func(*gorm.DB) application.ProjectWorkGuard

// NewStoreWithProjectWorkGuard enables archival and deletion with explicit work evidence.
func NewStoreWithProjectWorkGuard(db *gorm.DB, work ProjectWorkFactory) *Store {
	return &Store{db: db, work: work}
}

type lifecycleProjectRow struct {
	ID, OrgID                                                     uuid.UUID
	Name, Description, AspectRatio, StyleType, Resolution, Status string
	StyleSubtype                                                  *string
	StylePresetID                                                 *uuid.UUID
	AllowOverseasModels, IsDelete                                 bool
	ArchivedAt, DeleteTime, PurgeAfter                            *time.Time
	Revision                                                      int64
	CreateTime, UpdateTime                                        time.Time
	DefaultModels                                                 string
}

func (r lifecycleProjectRow) snapshot() (application.ProjectSnapshot, error) {
	p := domain.Project{ID: r.ID, OrgID: r.OrgID, Name: r.Name, Description: r.Description, AspectRatio: r.AspectRatio, StyleType: r.StyleType, Resolution: r.Resolution, Status: r.Status, AllowOverseasModels: r.AllowOverseasModels, IsDelete: r.IsDelete, ArchivedAt: r.ArchivedAt, DeleteTime: r.DeleteTime, PurgeAfter: r.PurgeAfter, Revision: r.Revision, CreateTime: r.CreateTime, UpdateTime: r.UpdateTime}
	if r.StyleSubtype != nil {
		p.StyleSubtype = *r.StyleSubtype
	}
	if r.StylePresetID != nil {
		p.StylePresetID = *r.StylePresetID
	}
	result := application.ProjectSnapshot{Project: p}
	if err := json.Unmarshal([]byte(r.DefaultModels), &result.DefaultModels); err != nil || result.DefaultModels == nil {
		return application.ProjectSnapshot{}, application.ErrProjectDependencyUnavailable
	}
	if err := result.Validate(); err != nil {
		return application.ProjectSnapshot{}, err
	}
	return result, nil
}

func readLifecycleProject(tx *gorm.DB, org, id uuid.UUID, write bool) (application.ProjectSnapshot, error) {
	query := `SELECT id,org_id,name,description,aspect_ratio,style_type,style_subtype,style_preset_id,resolution,allow_overseas_models,status,is_delete,archived_at,delete_time,purge_after,revision,create_time,update_time,default_models::text AS default_models FROM workspace.project WHERE id=? AND org_id=?`
	if write {
		query += ` FOR UPDATE`
	} else {
		query += ` AND NOT is_delete FOR SHARE`
	}
	var row lifecycleProjectRow
	read := tx.Raw(query, id, org).Scan(&row)
	if read.Error != nil {
		return application.ProjectSnapshot{}, fmt.Errorf("read project lifecycle row: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return application.ProjectSnapshot{}, ErrProjectNotFound
	}
	return row.snapshot()
}

// ReadProjectSnapshot returns complete safe settings while holding the project read lock.
func (s *Store) ReadProjectSnapshot(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.ProjectSnapshot, error) {
	if s == nil || s.db == nil {
		return application.ProjectSnapshot{}, ErrUnavailable
	}
	var saved application.ProjectSnapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var err error
		saved, err = readLifecycleProject(tx, actor.OrgID, id, false)
		return err
	})
	if err != nil {
		return application.ProjectSnapshot{}, fmt.Errorf("read project snapshot transaction: %w", err)
	}
	return saved, nil
}

// ApplyProjectChange commits project CAS, safe events, and a stable receipt together.
// The project lock also serializes generation, canvas, and media-tool admission.
func (s *Store) ApplyProjectChange(ctx context.Context, actor identityapp.Principal, input application.ProjectChangeInput, now time.Time) (application.ProjectSnapshot, error) {
	if s == nil || s.db == nil {
		return application.ProjectSnapshot{}, ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return application.ProjectSnapshot{}, err
	}
	if now.IsZero() {
		return application.ProjectSnapshot{}, application.ErrInvalidProjectChange
	}
	fingerprint, err := projectChangeFingerprint(actor, input)
	if err != nil {
		return application.ProjectSnapshot{}, err
	}
	var saved application.ProjectSnapshot
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var locked int
		if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.ID.String()+":"+input.IdempotencyKey.String()).Scan(&locked).Error; err != nil {
			return fmt.Errorf("lock project lifecycle request: %w", err)
		}
		current, err := readLifecycleProject(tx, actor.OrgID, input.Patch.ProjectID, true)
		if err != nil {
			return err
		}
		found, err := replayProjectChange(tx, actor, input.IdempotencyKey, fingerprint, &saved)
		if err != nil || found {
			return err
		}
		if current.Project.Revision != input.Patch.ExpectedRevision {
			return domain.ErrProjectRevisionConflict
		}
		// Read database time after acquiring the project lock. A restore that
		// waited behind another transaction cannot reuse an earlier deadline check.
		var lockedAt time.Time
		if err := tx.Raw(`SELECT clock_timestamp()`).Scan(&lockedAt).Error; err != nil {
			return fmt.Errorf("read locked project time: %w", err)
		}
		if lockedAt.After(now) {
			now = lockedAt.UTC()
		}
		blocking := false
		if input.Action == "archive" || input.Action == "delete" || input.Action == "unarchive" || input.Action == "restore" {
			if current.Project.IsDelete && input.Action != "restore" {
				return domain.ErrProjectStateConflict
			}
			if input.Action == "archive" {
				if err := current.Project.CanWrite(); err != nil {
					return err
				}
			}
			if s.work == nil {
				return application.ErrProjectDependencyUnavailable
			}
			guard := s.work(tx)
			if guard == nil {
				return application.ErrProjectDependencyUnavailable
			}
			blocking, err = guard.HasInflightWork(ctx, actor, current.Project.ID)
			if err != nil {
				return fmt.Errorf("read project inflight work: %w", err)
			}
		}
		after, events, err := application.PrepareProjectChange(actor, input, current.Project, now, blocking)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			saved = current
			return recordProjectChange(tx, actor, input.IdempotencyKey, fingerprint, saved)
		}
		if input.Action == "patch" {
			if err := requireUsablePreset(tx, after); err != nil {
				return err
			}
		}
		var preset any
		if after.StylePresetID != uuid.Nil {
			preset = after.StylePresetID
		}
		write := tx.Exec(`UPDATE workspace.project SET name=?,description=?,style_preset_id=?,allow_overseas_models=?,status=?,is_delete=?,archived_at=?,delete_time=?,purge_after=?,revision=? WHERE id=? AND org_id=? AND revision=?`, after.Name, after.Description, preset, after.AllowOverseasModels, after.Status, after.IsDelete, after.ArchivedAt, after.DeleteTime, after.PurgeAfter, after.Revision, after.ID, actor.OrgID, current.Project.Revision)
		if write.Error != nil {
			return fmt.Errorf("persist project lifecycle: %w", write.Error)
		}
		if write.RowsAffected != 1 {
			return domain.ErrProjectRevisionConflict
		}
		for _, event := range events {
			result := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, event.ID, event.Topic, event.PartitionKey, string(event.Payload))
			if result.Error != nil {
				return fmt.Errorf("persist project lifecycle event: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return application.ErrProjectDependencyUnavailable
			}
		}
		saved, err = readLifecycleProject(tx, actor.OrgID, after.ID, true)
		if err != nil {
			return err
		}
		return recordProjectChange(tx, actor, input.IdempotencyKey, fingerprint, saved)
	})
	if err != nil {
		return application.ProjectSnapshot{}, fmt.Errorf("project lifecycle transaction: %w", err)
	}
	return saved, nil
}

func projectChangeFingerprint(actor identityapp.Principal, input application.ProjectChangeInput) (string, error) {
	p := input.Patch
	body, err := json.Marshal(struct {
		Contract          string
		Org, Project      uuid.UUID
		Action            string
		Revision          int64
		Name, Description *string
		Preset            *uuid.UUID
		Overseas          *bool
	}{"project.lifecycle.v1", actor.OrgID, p.ProjectID, input.Action, p.ExpectedRevision, p.Name, p.Description, p.StylePresetID, p.AllowOverseasModels})
	if err != nil {
		return "", fmt.Errorf("encode project change fingerprint: %w", err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func replayProjectChange(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, fingerprint string, saved *application.ProjectSnapshot) (bool, error) {
	var row struct {
		RequestHash  string
		StatusCode   int
		ResponseBody []byte
	}
	read := tx.Raw(`SELECT request_hash,status_code,response_body FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND NOT is_delete AND expires_at>statement_timestamp()`, actor.ID, key.String()).Scan(&row)
	if read.Error != nil {
		return false, fmt.Errorf("read project lifecycle receipt: %w", read.Error)
	}
	if read.RowsAffected == 0 {
		return false, nil
	}
	if row.RequestHash != fingerprint || row.StatusCode != 200 {
		return false, application.ErrIdempotencyConflict
	}
	if err := json.Unmarshal(row.ResponseBody, saved); err != nil {
		return false, fmt.Errorf("decode project lifecycle receipt: %w", err)
	}
	if saved.Project.OrgID != actor.OrgID || saved.Project.ID == uuid.Nil || saved.DefaultModels == nil {
		return false, application.ErrProjectDependencyUnavailable
	}
	return true, nil
}

func recordProjectChange(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, fingerprint string, saved application.ProjectSnapshot) error {
	body, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("encode project lifecycle receipt: %w", err)
	}
	result := tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at)
 VALUES(?,?,?,?,200,?::jsonb,statement_timestamp()+interval '24 hours')
 ON CONFLICT(actor_id,idem_key) DO UPDATE SET request_hash=excluded.request_hash,status_code=excluded.status_code,response_body=excluded.response_body,expires_at=excluded.expires_at,is_delete=false,update_time=statement_timestamp()
 WHERE infra.idempotency_record.expires_at<=statement_timestamp() OR infra.idempotency_record.is_delete`, uuid.New(), actor.ID, key.String(), fingerprint, string(body))
	if result.Error != nil {
		return fmt.Errorf("persist project lifecycle receipt: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrIdempotencyConflict
	}
	return nil
}

var _ application.ProjectLifecycleStore = (*Store)(nil)
