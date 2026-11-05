package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// DirectoryExportStore uses the existing identity database and private export table.
type DirectoryExportStore struct{ db *gorm.DB }

// NewDirectoryExportStore does not migrate or open any external connection.
func NewDirectoryExportStore(db *gorm.DB) *DirectoryExportStore { return &DirectoryExportStore{db: db} }

type exportTx struct{ db *gorm.DB }

type platformSettingsRecord struct{ Singleton bool }

func (platformSettingsRecord) TableName() string { return "identity.platform_settings" }

type exportTeamRecord struct {
	ID                uuid.UUID
	Status            string
	GovernanceBlocked bool
	Revision          int64
}

func (exportTeamRecord) TableName() string { return "workspace.team" }

type exportTeamMemberRecord struct {
	TeamID, UserID uuid.UUID
	Role, Status   string
}

func (exportTeamMemberRecord) TableName() string { return "workspace.team_member" }

type exportJSON []byte

func (j *exportJSON) Scan(value any) error {
	switch value := value.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], value...)
	case string:
		*j = append((*j)[:0], value...)
	default:
		return fmt.Errorf("scan export JSON from %T", value)
	}
	return nil
}

func (j exportJSON) Value() (driver.Value, error) { return string(j), nil }
func (exportJSON) GormDataType() string           { return "jsonb" }

// Transaction takes the governance lock before authority and receipt reads.
// READ COMMITTED gives a waiter a fresh receipt snapshot after the lock holder
// commits. Snapshot materialization itself remains one consistent SQL statement.
func (s *DirectoryExportStore) Transaction(ctx context.Context, run func(application.ExportTx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var settings platformSettingsRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("singleton = ?", true).Take(&settings).Error; err != nil {
			return err
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
	var teamRow exportTeamRecord
	err := t.db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status IN ? AND NOT governance_blocked", *team, []string{"active", "archived"}).Take(&teamRow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, application.ErrForbidden
	}
	if err != nil {
		return 0, err
	}
	var member exportTeamMemberRecord
	err = t.db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("team_id = ? AND user_id = ? AND role = ? AND status = ?", *team, actor.ID, "team_admin", "active").Take(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || revision != 0 && revision != teamRow.Revision {
		return 0, application.ErrForbidden
	}
	if err != nil {
		return 0, err
	}
	return teamRow.Revision, nil
}

type exportRecord struct {
	ID, ActorID, ScopeID, RequestKey             uuid.UUID
	TeamID                                       *uuid.UUID
	ScopeKind                                    string
	ActorSessionID                               uuid.UUID
	CredentialEpoch, PlatformEpoch, TeamRevision int64
	FilterSnapshot                               exportJSON
	Status                                       string
	SnapshotPayload                              []byte
	SnapshotKeyID                                string
	RowCount                                     int
	AsOf, CreatedAt, UpdatedAt, ExpiresAt        time.Time
	PrivateObjectID                              *uuid.UUID
	StorageLocationID, StorageIntentID           *uuid.UUID
	StorageProfileID, StorageBucket              string
	ErrorCategory                                string
}

func (exportRecord) TableName() string { return "identity.directory_export" }

func exportJob(r exportRecord) (application.DirectoryExportJob, error) {
	j := application.DirectoryExportJob{ID: r.ID, ActorID: r.ActorID, ScopeID: r.ScopeID, RequestKey: r.RequestKey, TeamID: r.TeamID, Actor: application.Principal{ID: r.ActorID, SessionID: r.ActorSessionID, CredentialEpoch: r.CredentialEpoch, PlatformEpoch: r.PlatformEpoch}, TeamRevision: r.TeamRevision, Status: r.Status, Snapshot: r.SnapshotPayload, KeyID: r.SnapshotKeyID, RowCount: r.RowCount, AsOf: r.AsOf, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, ExpiresAt: r.ExpiresAt, ObjectID: r.PrivateObjectID, ErrorCategory: r.ErrorCategory}
	if r.StorageLocationID != nil && r.StorageProfileID != "" && r.StorageBucket != "" && r.PrivateObjectID != nil {
		j.ObjectLocation = &application.ExportObjectLocation{ID: *r.StorageLocationID, ProfileID: r.StorageProfileID, Bucket: r.StorageBucket, Key: exportObjectKey(*r.PrivateObjectID)}
		if r.StorageIntentID != nil {
			j.ObjectLocation.IntentID = *r.StorageIntentID
		}
	}
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
	err := t.db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.DirectoryExportJob{}, application.ErrExportNotFound
	}
	if err != nil {
		return application.DirectoryExportJob{}, err
	}
	job, err := exportJob(r)
	if err != nil {
		return job, err
	}
	return job, nil
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
	var locationID, intentID *uuid.UUID
	var profileID, bucket string
	if j.ObjectLocation != nil {
		locationID, profileID, bucket = &j.ObjectLocation.ID, j.ObjectLocation.ProfileID, j.ObjectLocation.Bucket
		if j.ObjectLocation.IntentID != uuid.Nil {
			intentID = &j.ObjectLocation.IntentID
		}
	}
	record := exportRecord{ID: j.ID, ActorID: j.ActorID, ScopeID: j.ScopeID, RequestKey: j.RequestKey, TeamID: j.TeamID, ScopeKind: scopeKind, ActorSessionID: j.Actor.SessionID, CredentialEpoch: j.Actor.CredentialEpoch, PlatformEpoch: j.Actor.PlatformEpoch, TeamRevision: j.TeamRevision, FilterSnapshot: exportJSON(filter), Status: j.Status, SnapshotPayload: j.Snapshot, SnapshotKeyID: j.KeyID, RowCount: j.RowCount, AsOf: j.AsOf, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, ExpiresAt: j.ExpiresAt, PrivateObjectID: j.ObjectID, StorageLocationID: locationID, StorageIntentID: intentID, StorageProfileID: profileID, StorageBucket: bucket, ErrorCategory: j.ErrorCategory}
	return t.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"status", "snapshot_payload", "updated_at", "expires_at", "private_object_id", "storage_location_id", "storage_intent_id", "storage_profile_id", "storage_bucket", "error_category"})}).Create(&record).Error
}

func exportObjectKey(id uuid.UUID) string {
	return "private/identity/directory-exports/" + id.String() + ".enc"
}

func exportStorageOwner(job application.DirectoryExportJob) mediaapp.StorageWorkOwner {
	return mediaapp.StorageWorkOwner{Kind: mediaapp.StorageWorkKindExport, OwnerID: job.ActorID, WorkID: job.ID}
}

func exportLocationRef(location application.ExportObjectLocation) mediadomain.ObjectRef {
	return mediadomain.ObjectRef{ProfileID: location.ProfileID, Bucket: location.Bucket, Key: location.Key}
}

func exportDomainLocation(location application.ExportObjectLocation) mediadomain.ObjectLocation {
	return mediadomain.ObjectLocation{ID: location.ID, Ref: exportLocationRef(location)}
}

func (t *exportTx) PinExportObject(ctx context.Context, job application.DirectoryExportJob) (application.ExportObjectLocation, error) {
	if job.ObjectID == nil || job.ID == uuid.Nil || job.ActorID == uuid.Nil {
		return application.ExportObjectLocation{}, application.ErrExportUnavailable
	}
	routes := pgmedia.NewStorageRoutes(t.db)
	intent, err := routes.PinStorageWork(ctx, exportStorageOwner(job))
	if err != nil {
		return application.ExportObjectLocation{}, err
	}
	location, err := routes.ReserveWorkLocation(ctx, intent.ID, exportObjectKey(*job.ObjectID))
	if err != nil {
		return application.ExportObjectLocation{}, err
	}
	return application.ExportObjectLocation{ID: location.ID, IntentID: intent.ID, ProfileID: location.Ref.ProfileID, Bucket: location.Ref.Bucket, Key: location.Ref.Key}, nil
}

func (t *exportTx) exportWork(ctx context.Context, job application.DirectoryExportJob) (mediaapp.StorageWorkIntent, mediadomain.ObjectLocation, error) {
	if job.ObjectLocation == nil || job.ObjectLocation.ID == uuid.Nil || job.ObjectLocation.IntentID == uuid.Nil {
		return mediaapp.StorageWorkIntent{}, mediadomain.ObjectLocation{}, application.ErrExportUnavailable
	}
	routes := pgmedia.NewStorageRoutes(t.db)
	intent, err := routes.ReadStorageWork(ctx, exportStorageOwner(job))
	if err != nil {
		return intent, mediadomain.ObjectLocation{}, err
	}
	if intent.ID != job.ObjectLocation.IntentID || intent.ProfileID != job.ObjectLocation.ProfileID || intent.Bucket != job.ObjectLocation.Bucket {
		return intent, mediadomain.ObjectLocation{}, application.ErrExportUnavailable
	}
	location, err := routes.ReadLocation(ctx, job.ObjectLocation.ID)
	if err != nil {
		return intent, location, err
	}
	if location.Ref != exportLocationRef(*job.ObjectLocation) {
		return intent, location, application.ErrExportUnavailable
	}
	return intent, location, nil
}

func (t *exportTx) BeginExportObjectWrite(ctx context.Context, job application.DirectoryExportJob, deadline time.Time) error {
	intent, location, err := t.exportWork(ctx, job)
	if err != nil {
		return err
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	return pgmedia.NewStorageRoutes(t.db).BeginWorkWrite(bounded, intent.ID, location)
}

func (t *exportTx) ConfirmExportObject(ctx context.Context, job application.DirectoryExportJob, info application.ExportObjectInfo) error {
	intent, location, err := t.exportWork(ctx, job)
	if err != nil {
		return err
	}
	return pgmedia.NewStorageRoutes(t.db).ConfirmWorkLocation(ctx, intent.ID, location, mediaapp.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256})
}

func (t *exportTx) AcceptExportObject(ctx context.Context, job application.DirectoryExportJob) error {
	intent, location, err := t.exportWork(ctx, job)
	if err != nil {
		return err
	}
	return pgmedia.NewStorageRoutes(t.db).AcceptStorageWork(ctx, intent.ID, []uuid.UUID{location.ID})
}

func (t *exportTx) CancelExportObject(ctx context.Context, job application.DirectoryExportJob) error {
	if job.ObjectLocation == nil || job.ObjectLocation.IntentID == uuid.Nil {
		return nil
	}
	routes := pgmedia.NewStorageRoutes(t.db)
	intent, err := routes.ReadStorageWork(ctx, exportStorageOwner(job))
	if err != nil {
		return err
	}
	if intent.ID != job.ObjectLocation.IntentID {
		return application.ErrExportUnavailable
	}
	return routes.CancelStorageWork(ctx, intent.ID)
}

// BeginExportObjectRemoval confirms the job has expired, then grants provider
// deletion only for its exact ledger location after any write deadline drains.
func (s *DirectoryExportStore) BeginExportObjectRemoval(ctx context.Context, claimed application.DirectoryExportJob) (application.ExportObjectLocation, bool, error) {
	var location application.ExportObjectLocation
	var removable bool
	if s == nil || s.db == nil || claimed.ID == uuid.Nil || claimed.ObjectID == nil {
		return location, false, application.ErrExportUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row exportRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ? AND private_object_id = ?", claimed.ID, application.DirectoryExportExpired, claimed.ObjectID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ErrExportNotFound
		}
		if err != nil {
			return err
		}
		job, err := exportJob(row)
		if err != nil {
			return err
		}
		if job.ObjectLocation == nil || job.ObjectLocation.IntentID == uuid.Nil {
			return application.ErrExportUnavailable
		}
		location = *job.ObjectLocation
		routes := pgmedia.NewStorageRoutes(tx)
		physical, err := routes.ReadLocation(ctx, location.ID)
		if err != nil {
			return err
		}
		if physical.Ref != exportLocationRef(location) {
			return application.ErrExportUnavailable
		}
		if physical.State == mediadomain.ObjectLocationDeleted {
			removable = true
			return nil
		}
		intent, err := routes.ReadStorageWork(ctx, exportStorageOwner(job))
		if err != nil {
			return err
		}
		if intent.ID != location.IntentID {
			return application.ErrExportUnavailable
		}
		switch intent.State {
		case mediaapp.StorageWorkOpen:
			if err := routes.CancelStorageWork(ctx, intent.ID); err != nil {
				return err
			}
			removable, err = routes.BeginWorkRemoval(ctx, intent.ID, physical)
			return err
		case mediaapp.StorageWorkCancelled:
			removable, err = routes.BeginWorkRemoval(ctx, intent.ID, physical)
			return err
		case mediaapp.StorageWorkAccepted:
			removable, err = routes.BeginAcceptedWorkRemoval(ctx, intent.ID, physical)
			return err
		default:
			return application.ErrExportUnavailable
		}
	})
	return location, removable, err
}

// ConfirmExportObjectRemoval retires the exact row only after provider deletion.
func (s *DirectoryExportStore) ConfirmExportObjectRemoval(ctx context.Context, _ application.DirectoryExportJob, location application.ExportObjectLocation) error {
	if s == nil || s.db == nil || location.ID == uuid.Nil || location.IntentID == uuid.Nil {
		return application.ErrExportUnavailable
	}
	routes := pgmedia.NewStorageRoutes(s.db.WithContext(ctx))
	return routes.ConfirmWorkRemoval(ctx, location.IntentID, exportDomainLocation(location))
}

// Pending lists a bounded batch without retaining locks across object I/O.
func (s *DirectoryExportStore) Pending(ctx context.Context, limit int, now time.Time) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := s.db.WithContext(ctx).Table("identity.directory_export").Where("status = ? AND expires_at > ?", application.DirectoryExportPending, now).Order("created_at,id").Limit(limit).Pluck("id", &ids).Error
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
		retryBefore := now.Add(-time.Minute)
		claim := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("expires_at <= ?", now).
			Where("(status <> ? AND (status <> ? OR updated_at <= ?)) OR (status = ? AND (private_object_id IS NOT NULL OR snapshot_payload IS NOT NULL) AND updated_at <= ?)", application.DirectoryExportExpired, application.DirectoryExportRunning, retryBefore, application.DirectoryExportExpired, retryBefore).
			Order("expires_at,id").Limit(limit)
		if err := claim.Find(&records).Error; err != nil {
			return err
		}
		for _, r := range records {
			j, err := exportJob(r)
			if err != nil {
				return err
			}
			if err := tx.Model(&exportRecord{}).Where("id = ?", j.ID).Updates(map[string]any{"status": application.DirectoryExportExpired, "snapshot_payload": nil, "updated_at": now}).Error; err != nil {
				return err
			}
			j.Status = application.DirectoryExportExpired
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
		result := s.db.WithContext(ctx).Model(&exportRecord{}).
			Where("id = ? AND status = ? AND updated_at = ? AND private_object_id IS NOT DISTINCT FROM ? AND expires_at <= ?", job.ID, application.DirectoryExportExpired, job.UpdatedAt, job.ObjectID, now).
			Updates(map[string]any{"private_object_id": nil, "storage_location_id": nil, "storage_intent_id": nil, "storage_profile_id": nil, "storage_bucket": nil, "snapshot_payload": nil})
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
