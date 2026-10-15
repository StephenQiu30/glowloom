package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// DirectoryExportStore uses the existing identity database and private export table.
type DirectoryExportStore struct{ db *gorm.DB }

// NewDirectoryExportStore does not migrate or open any external connection.
func NewDirectoryExportStore(db *gorm.DB) *DirectoryExportStore { return &DirectoryExportStore{db: db} }

type exportTx struct{ db *gorm.DB }

// Transaction takes the governance lock before authority and receipt reads.
// READ COMMITTED gives a waiter a fresh receipt snapshot after the lock holder
// commits. Snapshot materialization itself remains one consistent SQL statement.
func (s *DirectoryExportStore) Transaction(ctx context.Context, run func(application.ExportTx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL statement_timeout = '30s'").Error; err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL idle_in_transaction_session_timeout = '35s'").Error; err != nil {
			return err
		}
		var exists bool
		if err := tx.Raw(`SELECT singleton FROM identity.platform_settings WHERE singleton FOR UPDATE`).Scan(&exists).Error; err != nil {
			return err
		}
		if !exists {
			return application.ErrSessionInvalid
		}
		return run(&exportTx{db: tx})
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}
func (t *exportTx) Authorize(actor application.Principal, team *uuid.UUID, revision int64, recent bool, clock func() time.Time) (int64, error) {
	if team == nil {
		if err := lockPlatformReader(t.db, actor, clock); err != nil {
			return 0, err
		}
	} else {
		if _, err := lockSelfActor(t.db, actor, clock); err != nil {
			return 0, err
		}
	}
	if recent {
		if err := NewStore(t.db).CheckRecentSession(t.db.Statement.Context, actor, clock); err != nil {
			return 0, err
		}
	}
	if team == nil {
		return 0, nil
	}
	var row struct{ Revision int64 }
	r := t.db.Raw(`SELECT t.revision FROM workspace.team t JOIN workspace.team_member m ON m.team_id=t.id WHERE t.id=? AND m.user_id=? AND m.role='team_admin' AND m.status='active' AND t.status IN ('active','archived') AND NOT t.governance_blocked FOR UPDATE OF t,m`, *team, actor.ID).Scan(&row)
	if r.Error != nil {
		return 0, r.Error
	}
	if r.RowsAffected != 1 || revision != 0 && revision != row.Revision {
		return 0, application.ErrForbidden
	}
	return row.Revision, nil
}

type exportRecord struct {
	ID, ActorID, ScopeID, RequestKey             uuid.UUID
	TeamID                                       *uuid.UUID
	ActorSessionID                               uuid.UUID
	CredentialEpoch, PlatformEpoch, TeamRevision int64
	FilterSnapshot                               []byte
	Status                                       string
	SnapshotPayload                              []byte
	SnapshotKeyID                                string
	RowCount                                     int
	AsOf, CreatedAt, UpdatedAt, ExpiresAt        time.Time
	PrivateObjectID                              *uuid.UUID
	ErrorCategory                                string
}

func exportJob(r exportRecord) (application.DirectoryExportJob, error) {
	j := application.DirectoryExportJob{ID: r.ID, ActorID: r.ActorID, ScopeID: r.ScopeID, RequestKey: r.RequestKey, TeamID: r.TeamID, Actor: application.Principal{ID: r.ActorID, SessionID: r.ActorSessionID, CredentialEpoch: r.CredentialEpoch, PlatformEpoch: r.PlatformEpoch}, TeamRevision: r.TeamRevision, Status: r.Status, Snapshot: r.SnapshotPayload, KeyID: r.SnapshotKeyID, RowCount: r.RowCount, AsOf: r.AsOf, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, ExpiresAt: r.ExpiresAt, ObjectID: r.PrivateObjectID, ErrorCategory: r.ErrorCategory}
	err := json.Unmarshal(r.FilterSnapshot, &j.Filter)
	return j, err
}
func (t *exportTx) Request(actor, scope, key uuid.UUID) (application.DirectoryExportJob, bool, error) {
	var r exportRecord
	result := t.db.Table("identity.directory_export").Where("actor_id=? AND scope_id=? AND request_key=?", actor, scope, key).Take(&r)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return application.DirectoryExportJob{}, false, nil
	}
	if result.Error != nil {
		return application.DirectoryExportJob{}, false, result.Error
	}
	j, err := exportJob(r)
	return j, true, err
}
func (t *exportTx) Load(id uuid.UUID) (application.DirectoryExportJob, error) {
	var r exportRecord
	result := t.db.Raw(`SELECT * FROM identity.directory_export WHERE id=? FOR UPDATE`, id).Scan(&r)
	if result.Error != nil {
		return application.DirectoryExportJob{}, result.Error
	}
	if result.RowsAffected != 1 {
		return application.DirectoryExportJob{}, application.ErrExportNotFound
	}
	return exportJob(r)
}
func (t *exportTx) Save(j application.DirectoryExportJob) error {
	filter, err := json.Marshal(j.Filter)
	if err != nil {
		return err
	}
	scopeKind := "platform"
	if j.TeamID != nil {
		scopeKind = "team"
	}
	return t.db.Exec(`INSERT INTO identity.directory_export (id,actor_id,scope_id,request_key,team_id,scope_kind,actor_session_id,credential_epoch,platform_epoch,team_revision,filter_snapshot,status,snapshot_payload,snapshot_key_id,row_count,as_of,created_at,updated_at,expires_at,private_object_id,error_category)
 VALUES (?,?,?,?,?,?,?,?,?,?,?::jsonb,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status,snapshot_payload=EXCLUDED.snapshot_payload,updated_at=EXCLUDED.updated_at,expires_at=EXCLUDED.expires_at,private_object_id=EXCLUDED.private_object_id,error_category=EXCLUDED.error_category`, j.ID, j.ActorID, j.ScopeID, j.RequestKey, j.TeamID, scopeKind, j.Actor.SessionID, j.Actor.CredentialEpoch, j.Actor.PlatformEpoch, j.TeamRevision, string(filter), j.Status, j.Snapshot, j.KeyID, j.RowCount, j.AsOf, j.CreatedAt, j.UpdatedAt, j.ExpiresAt, j.ObjectID, j.ErrorCategory).Error
}

// Pending lists a bounded batch without retaining locks across object I/O.
func (s *DirectoryExportStore) Pending(ctx context.Context, limit int, now time.Time) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := s.db.WithContext(ctx).Table("identity.directory_export").Where("status='pending' AND expires_at>?", now).Order("created_at,id").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

// Cleanup claims immutable expired objects, deletes outside SQL transactions,
// then clears the reference only if the same claim and object still own the row.
// An uncertain DELETE retains its object ID and is retryable after a minute.
func (s *DirectoryExportStore) Cleanup(ctx context.Context, limit int, now time.Time, remove func(context.Context, application.DirectoryExportJob) error) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if limit < 1 || limit > 20 {
		limit = 20
	}
	var claimed []application.DirectoryExportJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var records []exportRecord
		if err := tx.Raw(`SELECT * FROM identity.directory_export WHERE expires_at<=? AND ((status<>'expired' AND (status<>'running' OR updated_at<=?::timestamptz - interval '1 minute')) OR (status='expired' AND (private_object_id IS NOT NULL OR snapshot_payload IS NOT NULL) AND updated_at<=?::timestamptz - interval '1 minute')) ORDER BY expires_at,id LIMIT ? FOR UPDATE SKIP LOCKED`, now, now, now, limit).Scan(&records).Error; err != nil {
			return err
		}
		for _, r := range records {
			j, err := exportJob(r)
			if err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE identity.directory_export SET status='expired',snapshot_payload=NULL,updated_at=? WHERE id=?`, now, j.ID).Error; err != nil {
				return err
			}
			j.Status = "expired"
			j.Snapshot = nil
			j.UpdatedAt = now
			claimed = append(claimed, j)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, job := range claimed {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if err := remove(ctx, job); err != nil {
			return count, err
		}
		// This single statement owns its short implicit transaction. No object call
		// runs with its row lock held. A newer claim cannot be finalized by this one.
		result := s.db.WithContext(ctx).Exec(`UPDATE identity.directory_export SET private_object_id=NULL,snapshot_payload=NULL WHERE id=? AND status='expired' AND updated_at=? AND private_object_id IS NOT DISTINCT FROM ?::uuid AND expires_at<=?`, job.ID, job.UpdatedAt, job.ObjectID, now)
		if result.Error != nil {
			return count, result.Error
		}
		count += int(result.RowsAffected)
	}
	return count, nil
}

func memberExportQuery(tx *gorm.DB, team uuid.UUID, f application.ExportFilter, asOf time.Time) *gorm.DB {
	q := tx.Table(`workspace.team_member m JOIN identity."user" u ON u.id=m.user_id`).Where("m.team_id=? AND m.joined_at<=?", team, asOf)
	if f.Role != "" {
		q = q.Where("m.role=?", f.Role)
	}
	if f.Status != "" {
		q = q.Where("m.status=?", f.Status)
	}
	if f.From != nil {
		q = q.Where("m.joined_at>=?", *f.From)
	}
	if f.To != nil {
		q = q.Where("m.joined_at<?", *f.To)
	}
	if f.Search != "" {
		q = q.Where("(strpos(lower(u.display_name),lower(?))>0 OR strpos(lower(u.email_key),lower(?))>0)", f.Search, f.Search)
	}
	return q
}
func (t *exportTx) Snapshot(team *uuid.UUID, f application.ExportFilter, asOf time.Time) ([][]string, error) {
	if team == nil {
		var users []application.UserSummary
		query := filteredDirectory(t.db, application.DirectoryFilter{Search: f.Search, Status: f.Status, PlatformRole: f.PlatformRole, Verified: f.Verified, InTeam: f.InTeam, From: f.From, To: f.To}, asOf)
		order := "u.create_time DESC,u.id DESC"
		if f.Order == "created_asc" {
			order = "u.create_time ASC,u.id ASC"
		}
		if err := query.Order(order).Limit(10001).Scan(&users).Error; err != nil {
			return nil, err
		}
		rows := [][]string{{"id", "email", "display_name", "status", "platform_role", "email_verified", "active_team_count", "created_at"}}
		for _, u := range users {
			rows = append(rows, []string{u.ID.String(), u.Email, u.DisplayName, u.Status, u.PlatformRole, strconv.FormatBool(u.Verified), strconv.Itoa(u.ActiveTeamCount), u.CreatedAt.Format(time.RFC3339Nano)})
		}
		return rows, nil
	}
	var members []struct {
		UserID                           uuid.UUID
		Email, DisplayName, Role, Status string
		JoinedAt                         time.Time
	}
	if err := memberExportQuery(t.db, *team, f, asOf).Select("m.user_id,coalesce(u.email_key,'') AS email,u.display_name,m.role,m.status,m.joined_at").Order("m.joined_at,m.user_id").Limit(10001).Scan(&members).Error; err != nil {
		return nil, err
	}
	rows := [][]string{{"user_id", "email", "display_name", "role", "status", "joined_at"}}
	for _, m := range members {
		rows = append(rows, []string{m.UserID.String(), m.Email, m.DisplayName, m.Role, m.Status, m.JoinedAt.Format(time.RFC3339Nano)})
	}
	return rows, nil
}
