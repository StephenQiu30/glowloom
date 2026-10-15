package application

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrExportLimit rejects snapshots exceeding the accepted row or byte bound.
	ErrExportLimit = errors.New("directory export exceeds limit")
	// ErrExportNotFound hides unavailable, expired and foreign export jobs.
	ErrExportNotFound = errors.New("directory export not found")
	// ErrExportConflict rejects reused request keys with different filters.
	ErrExportConflict = errors.New("directory export request conflict")
	// ErrExportUnavailable means the existing private storage is unavailable.
	ErrExportUnavailable = errors.New("directory export unavailable")
)

const exportRowsLimit = 10000
const exportBytesLimit = 32 << 20

// ExportFilter has separate platform-account and team-membership whitelists.
type ExportFilter struct {
	Search       string     `json:"search,omitempty"`
	Status       string     `json:"status,omitempty"`
	Role         string     `json:"role,omitempty"`
	PlatformRole string     `json:"platform_role,omitempty"`
	Order        string     `json:"order,omitempty"`
	Verified     *bool      `json:"verified,omitempty"`
	InTeam       *bool      `json:"in_team,omitempty"`
	From         *time.Time `json:"from,omitempty"`
	To           *time.Time `json:"to,omitempty"`
}

func normalizeExportFilter(team bool, f ExportFilter) (ExportFilter, error) {
	f.Search = strings.TrimSpace(f.Search)
	if !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > 100 || (f.From != nil && f.To != nil && !f.From.Before(*f.To)) {
		return f, ErrInvalidDirectoryFilter
	}
	if f.From != nil {
		v := f.From.UTC()
		f.From = &v
	}
	if f.To != nil {
		v := f.To.UTC()
		f.To = &v
	}
	if team {
		if f.PlatformRole != "" || f.Verified != nil || f.InTeam != nil || f.Order != "" || (f.Role != "" && f.Role != "member" && f.Role != "team_admin") || (f.Status != "" && f.Status != "active" && f.Status != "suspended" && f.Status != "removed") {
			return f, ErrInvalidDirectoryFilter
		}
	} else {
		if f.Role != "" {
			return f, ErrInvalidDirectoryFilter
		}
		d, err := normalizeDirectoryFilter(DirectoryFilter{Search: f.Search, Status: f.Status, PlatformRole: f.PlatformRole, Order: f.Order, Verified: f.Verified, InTeam: f.InTeam, From: f.From, To: f.To})
		if err != nil {
			return f, err
		}
		f.Order = d.Order
	}
	return f, nil
}

// DirectoryExportJob contains server-owned encrypted temporary material.
// Only the HTTP projection may cross the public API boundary.
type DirectoryExportJob struct {
	ID, ActorID, ScopeID, RequestKey      uuid.UUID
	TeamID                                *uuid.UUID
	Actor                                 Principal
	TeamRevision                          int64
	Filter                                ExportFilter
	Status                                string
	Snapshot                              []byte
	KeyID                                 string
	RowCount                              int
	AsOf, CreatedAt, UpdatedAt, ExpiresAt time.Time
	ObjectID                              *uuid.UUID
	ErrorCategory                         string
}

// ExportTx holds identity/team authorization and the job under one transaction.
type ExportTx interface {
	Authorize(Principal, *uuid.UUID, int64, bool, time.Time) (int64, error)
	Request(uuid.UUID, uuid.UUID, uuid.UUID) (DirectoryExportJob, bool, error)
	Load(uuid.UUID) (DirectoryExportJob, error)
	Snapshot(*uuid.UUID, ExportFilter, time.Time) ([][]string, error)
	Save(DirectoryExportJob) error
}

// DirectoryExportStore persists bounded encrypted snapshots and job receipts.
type DirectoryExportStore interface {
	Transaction(context.Context, func(ExportTx) error) error
	Pending(context.Context, int, time.Time) ([]uuid.UUID, error)
	Cleanup(context.Context, int, time.Time, func(DirectoryExportJob) error) (int, error)
}

// ExportProtector authenticates encrypted snapshots and private CSV objects.
type ExportProtector interface {
	KeyID() string
	Seal(uuid.UUID, []byte) ([]byte, error)
	Open(uuid.UUID, string, []byte) ([]byte, error)
}

// ExportObjects uses the already configured private object-storage connection.
type ExportObjects interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Remove(context.Context, string) error
}

// DirectoryExport owns bounded materialization, generation and authorized downloads.
type DirectoryExport struct {
	store     DirectoryExportStore
	protector ExportProtector
	objects   ExportObjects
	now       func() time.Time
}

// NewDirectoryExport injects the existing database, identity encryption and storage.
func NewDirectoryExport(store DirectoryExportStore, protector ExportProtector, objects ExportObjects, now func() time.Time) *DirectoryExport {
	return &DirectoryExport{store: store, protector: protector, objects: objects, now: now}
}

// Create materializes at most 10000 rows before committing an encrypted snapshot.
func (s *DirectoryExport) Create(ctx context.Context, actor Principal, team *uuid.UUID, filter ExportFilter, key uuid.UUID) (DirectoryExportJob, error) {
	f, err := normalizeExportFilter(team != nil, filter)
	if err != nil {
		return DirectoryExportJob{}, err
	}
	if key == uuid.Nil || team != nil && *team == uuid.Nil {
		return DirectoryExportJob{}, ErrInvalidDirectoryFilter
	}
	if s.objects == nil || s.protector == nil {
		return DirectoryExportJob{}, ErrExportUnavailable
	}
	now := s.now().UTC()
	scope := uuid.Nil
	if team != nil {
		scope = *team
	}
	var job DirectoryExportJob
	err = s.store.Transaction(ctx, func(tx ExportTx) error {
		revision, err := tx.Authorize(actor, team, 0, true, now)
		if err != nil {
			return err
		}
		prior, found, err := tx.Request(actor.ID, scope, key)
		if err != nil {
			return err
		}
		if found {
			a, _ := json.Marshal(prior.Filter)
			b, _ := json.Marshal(f)
			if !bytes.Equal(a, b) {
				return ErrExportConflict
			}
			job = prior
			return nil
		}
		rows, err := tx.Snapshot(team, f, now)
		if err != nil {
			return err
		}
		if len(rows) == 0 || len(rows)-1 > exportRowsLimit {
			return ErrExportLimit
		}
		raw, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if len(raw) > exportBytesLimit {
			return ErrExportLimit
		}
		job = DirectoryExportJob{ID: uuid.New(), ActorID: actor.ID, ScopeID: scope, RequestKey: key, TeamID: team, Actor: actor, TeamRevision: revision, Filter: f, Status: "pending", KeyID: s.protector.KeyID(), RowCount: len(rows) - 1, AsOf: now, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
		objectID := uuid.NewSHA1(job.ID, []byte("directory-export-csv"))
		job.ObjectID = &objectID
		job.Snapshot, err = s.protector.Seal(job.ID, raw)
		if err != nil {
			return err
		}
		return tx.Save(job)
	})
	return job, err
}

// Read requires current authority and the original caller and scope.
func (s *DirectoryExport) Read(ctx context.Context, actor Principal, team *uuid.UUID, id uuid.UUID) (DirectoryExportJob, error) {
	var job DirectoryExportJob
	err := s.withJob(ctx, actor, team, id, func(j DirectoryExportJob) error { job = j; return nil })
	return job, err
}

func (s *DirectoryExport) withJob(ctx context.Context, actor Principal, team *uuid.UUID, id uuid.UUID, run func(DirectoryExportJob) error) error {
	return s.store.Transaction(ctx, func(tx ExportTx) error {
		if _, err := tx.Authorize(actor, team, 0, false, s.now().UTC()); err != nil {
			return err
		}
		job, err := tx.Load(id)
		if err != nil {
			return err
		}
		if job.ActorID != actor.ID || (job.TeamID == nil) != (team == nil) || team != nil && *job.TeamID != *team {
			return ErrExportNotFound
		}
		if actor.CredentialEpoch != job.Actor.CredentialEpoch || team == nil && actor.PlatformEpoch != job.Actor.PlatformEpoch {
			return ErrForbidden
		}
		if _, err := tx.Authorize(actor, team, job.TeamRevision, false, s.now().UTC()); err != nil {
			return err
		}
		if job.Status == "expired" || job.Status == "revoked" || !s.now().UTC().Before(job.ExpiresAt) {
			return ErrExportNotFound
		}
		return run(job)
	})
}

// Stream performs bounded private I/O outside database locks, then reauthorizes
// immediately before the bounded HTTP callback under current authority locks.
func (s *DirectoryExport) Stream(ctx context.Context, actor Principal, team *uuid.UUID, id uuid.UUID, write func(io.Reader) error) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s.objects == nil || s.protector == nil {
		return ErrExportUnavailable
	}
	job, err := s.Read(ctx, actor, team, id)
	if err != nil {
		return err
	}
	if job.Status != "completed" || job.ObjectID == nil {
		return ErrExportNotFound
	}
	reader, err := s.objects.Get(ctx, exportObjectKey(*job.ObjectID))
	if err != nil {
		return ErrExportUnavailable
	}
	raw, readErr := io.ReadAll(io.LimitReader(reader, exportBytesLimit+1025))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || len(raw) > exportBytesLimit+1024 {
		return ErrExportUnavailable
	}
	data, err := s.protector.Open(*job.ObjectID, job.KeyID, raw)
	if err != nil {
		return ErrExportUnavailable
	}
	return s.withJob(ctx, actor, team, id, func(current DirectoryExportJob) error {
		if current.Status != "completed" || current.ObjectID == nil || *current.ObjectID != *job.ObjectID || !s.now().UTC().Before(current.ExpiresAt) {
			return ErrExportNotFound
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return write(&exportContextReader{ctx: ctx, reader: bytes.NewReader(data)})
	})
}

func exportObjectKey(id uuid.UUID) string {
	return "private/identity/directory-exports/" + id.String() + ".enc"
}

// RunOnce performs bounded work in the existing API process, independent of SMTP.
func (s *DirectoryExport) RunOnce(ctx context.Context, limit int) (int, error) {
	if s.objects == nil || s.protector == nil {
		return 0, ErrExportUnavailable
	}
	if limit < 1 || limit > 20 {
		limit = 20
	}
	count, err := s.store.Cleanup(ctx, limit, s.now().UTC(), func(job DirectoryExportJob) error {
		if job.ObjectID != nil {
			return s.objects.Remove(ctx, exportObjectKey(*job.ObjectID))
		}
		return nil
	})
	if err != nil {
		return count, err
	}
	ids, err := s.store.Pending(ctx, limit, s.now().UTC())
	if err != nil {
		return count, err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if err := s.process(ctx, id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *DirectoryExport) process(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var job DirectoryExportJob
	claimed := false
	err := s.store.Transaction(ctx, func(tx ExportTx) error {
		var err error
		job, err = tx.Load(id)
		if err != nil {
			return err
		}
		if job.Status != "pending" {
			return nil
		}
		now := s.now().UTC()
		if !now.Before(job.ExpiresAt) {
			job.Status = "expired"
			job.Snapshot = nil
			job.UpdatedAt = now
			return tx.Save(job)
		}
		if _, err := tx.Authorize(job.Actor, job.TeamID, job.TeamRevision, false, now); err != nil {
			if !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrSessionInvalid) {
				return err
			}
			job.Status = "revoked"
			job.Snapshot = nil
			job.ErrorCategory = "authority_revoked"
			job.UpdatedAt = now
			return tx.Save(job)
		}
		job.Status = "running"
		job.UpdatedAt = now
		claimed = true
		return tx.Save(job)
	})
	if err != nil || !claimed {
		return err
	}
	// Materialization is immutable; no directory query runs after admission.
	raw, err := s.protector.Open(job.ID, job.KeyID, job.Snapshot)
	var rows [][]string
	if err == nil {
		err = json.Unmarshal(raw, &rows)
	}
	category := ""
	if err != nil || len(rows) != job.RowCount+1 || job.RowCount > exportRowsLimit {
		category = "snapshot_invalid"
	}
	var data []byte
	if category == "" {
		data, err = directoryCSV(rows)
		if err != nil {
			category = "format_failed"
		}
	}
	if category == "" {
		ciphertext, err := s.protector.Seal(*job.ObjectID, data)
		if err != nil {
			return err
		}
		if err := s.objects.Put(ctx, exportObjectKey(*job.ObjectID), bytes.NewReader(ciphertext), int64(len(ciphertext)), "application/octet-stream"); err != nil {
			category = "storage_failed"
		}
	}
	// The job already owns the deterministic object ID, even if the process dies
	// after a successful PUT. Expiry cleanup can always remove that exact object.
	return s.store.Transaction(ctx, func(tx ExportTx) error {
		current, err := tx.Load(id)
		if err != nil {
			return err
		}
		if current.Status != "running" {
			return nil
		}
		now := s.now().UTC()
		if !now.Before(current.ExpiresAt) {
			current.Status = "expired"
			current.Snapshot = nil
			current.UpdatedAt = now
			return tx.Save(current)
		}
		if _, err := tx.Authorize(current.Actor, current.TeamID, current.TeamRevision, false, now); err != nil {
			if !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrSessionInvalid) {
				return err
			}
			current.Status = "revoked"
			current.Snapshot = nil
			current.ErrorCategory = "authority_revoked"
			current.UpdatedAt = now
			return tx.Save(current)
		}
		if category != "" {
			return s.fail(tx, current, category)
		}
		current.Status = "completed"
		current.Snapshot = nil
		current.UpdatedAt = now
		current.ExpiresAt = now.Add(15 * time.Minute)
		return tx.Save(current)
	})
}

func (s *DirectoryExport) fail(tx ExportTx, job DirectoryExportJob, category string) error {
	job.Status = "failed"
	job.Snapshot = nil
	job.ErrorCategory = category
	job.UpdatedAt = s.now().UTC()
	return tx.Save(job)
}

func directoryCSV(rows [][]string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	for _, row := range rows {
		safe := make([]string, len(row))
		for i, value := range row {
			trimmed := strings.TrimLeftFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
			if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) || strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") {
				value = "'" + value
			}
			safe[i] = value
		}
		if err := writer.Write(safe); err != nil {
			return nil, err
		}
		if buffer.Len() > exportBytesLimit {
			return nil, ErrExportLimit
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	if buffer.Len() > exportBytesLimit {
		return nil, ErrExportLimit
	}
	return buffer.Bytes(), nil
}

// exportContextReader stops response copies when the bounded request is cancelled.
type exportContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *exportContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
