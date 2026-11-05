package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
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

// Directory export states record admission, execution and read availability.
const (
	DirectoryExportPending   = "pending"
	DirectoryExportRunning   = "running"
	DirectoryExportCompleted = "completed"
	DirectoryExportFailed    = "failed"
	DirectoryExportExpired   = "expired"
	DirectoryExportRevoked   = "revoked"
)

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
	ObjectLocation                        *ExportObjectLocation
	ObjectLocationInfo                    *ExportObjectInfo
	ErrorCategory                         string
}

// ExportObjectLocation identifies one encrypted output in the shared immutable
// object-location ledger. A current route never reinterprets an older export.
type ExportObjectLocation struct {
	ID, IntentID           uuid.UUID
	ProfileID, Bucket, Key string
}

// ExportObjectInfo contains the metadata verified from a private export object.
type ExportObjectInfo struct {
	Size                int64
	ContentType, SHA256 string
}

// ExportTx holds identity/team authorization and the job under one transaction.
type ExportTx interface {
	Authorize(Principal, *uuid.UUID, int64, bool, func() time.Time) (int64, error)
	Request(uuid.UUID, uuid.UUID, uuid.UUID) (DirectoryExportJob, bool, error)
	Load(uuid.UUID) (DirectoryExportJob, error)
	Snapshot(*uuid.UUID, ExportFilter, time.Time) ([][]string, error)
	Save(DirectoryExportJob) error
}

// DirectoryExportStore persists bounded encrypted snapshots and job receipts.
type DirectoryExportStore interface {
	Transaction(context.Context, func(ExportTx) error) error
	Pending(context.Context, int, time.Time) ([]uuid.UUID, error)
	Cleanup(context.Context, int, time.Time, func(context.Context, DirectoryExportJob) error) (int, error)
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

// RoutedExportTx extends an authorized identity transaction with storage
// admission and result publication operations.
type RoutedExportTx interface {
	PinExportObject(context.Context, DirectoryExportJob) (ExportObjectLocation, error)
	BeginExportObjectWrite(context.Context, DirectoryExportJob, time.Time) error
	ConfirmExportObject(context.Context, DirectoryExportJob, ExportObjectInfo) error
	AcceptExportObject(context.Context, DirectoryExportJob) error
	CancelExportObject(context.Context, DirectoryExportJob) error
}

// RoutedExportLocationStore fences cleanup against a running export and old
// provider writes before granting deletion of the exact saved location.
type RoutedExportLocationStore interface {
	BeginExportObjectRemoval(context.Context, DirectoryExportJob) (ExportObjectLocation, bool, error)
	ConfirmExportObjectRemoval(context.Context, DirectoryExportJob, ExportObjectLocation) error
}

// RoutedExportObjects implements provider I/O against immutable refs supplied
// by the transactional location ledger.
type RoutedExportObjects interface {
	PutIfAbsent(context.Context, ExportObjectLocation, io.Reader, int64, string, string) error
	Stat(context.Context, ExportObjectLocation) (ExportObjectInfo, error)
	Get(context.Context, ExportObjectLocation) (io.ReadCloser, error)
	Remove(context.Context, DirectoryExportJob) error
}

// DirectoryExport owns bounded materialization, generation and authorized downloads.
type DirectoryExport struct {
	store     DirectoryExportStore
	protector ExportProtector
	objects   ExportObjects
	routed    RoutedExportObjects
	now       func() time.Time
}

// NewDirectoryExport injects the existing database, identity encryption and storage.
func NewDirectoryExport(store DirectoryExportStore, protector ExportProtector, objects ExportObjects, now func() time.Time) *DirectoryExport {
	return &DirectoryExport{store: store, protector: protector, objects: objects, now: now}
}

// NewRoutedDirectoryExport pins every temporary output to the database's
// accepted route and uses provider adapters only through exact references.
func NewRoutedDirectoryExport(store DirectoryExportStore, protector ExportProtector, objects RoutedExportObjects, now func() time.Time) *DirectoryExport {
	return &DirectoryExport{store: store, protector: protector, routed: objects, now: now}
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
	if s.objects == nil && s.routed == nil || s.protector == nil {
		return DirectoryExportJob{}, ErrExportUnavailable
	}
	scope := uuid.Nil
	if team != nil {
		scope = *team
	}
	var job DirectoryExportJob
	err = s.store.Transaction(ctx, func(tx ExportTx) error {
		revision, err := tx.Authorize(actor, team, 0, true, s.now)
		if err != nil {
			return err
		}
		now := s.now().UTC()
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
		job = DirectoryExportJob{ID: uuid.New(), ActorID: actor.ID, ScopeID: scope, RequestKey: key, TeamID: team, Actor: actor, TeamRevision: revision, Filter: f, Status: DirectoryExportPending, KeyID: s.protector.KeyID(), RowCount: len(rows) - 1, AsOf: now, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
		objectID := uuid.NewSHA1(job.ID, []byte("directory-export-csv"))
		job.ObjectID = &objectID
		if s.routed != nil {
			routedTx, ok := tx.(RoutedExportTx)
			if !ok {
				return ErrExportUnavailable
			}
			location, err := routedTx.PinExportObject(ctx, job)
			if err != nil {
				return err
			}
			job.ObjectLocation = &location
		}
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
		if _, err := tx.Authorize(actor, team, 0, false, s.now); err != nil {
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
		if _, err := tx.Authorize(actor, team, job.TeamRevision, false, s.now); err != nil {
			return err
		}
		if job.Status == DirectoryExportExpired || job.Status == DirectoryExportRevoked || !s.now().UTC().Before(job.ExpiresAt) {
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
	if s.objects == nil && s.routed == nil || s.protector == nil {
		return ErrExportUnavailable
	}
	job, err := s.Read(ctx, actor, team, id)
	if err != nil {
		return err
	}
	if job.Status != DirectoryExportCompleted || job.ObjectID == nil {
		return ErrExportNotFound
	}
	var reader io.ReadCloser
	if s.routed != nil {
		if job.ObjectLocation == nil {
			return ErrExportUnavailable
		}
		reader, err = s.routed.Get(ctx, *job.ObjectLocation)
	} else {
		reader, err = s.objects.Get(ctx, exportObjectKey(*job.ObjectID))
	}
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
		if current.Status != DirectoryExportCompleted || current.ObjectID == nil || *current.ObjectID != *job.ObjectID || !s.now().UTC().Before(current.ExpiresAt) {
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
	if s.objects == nil && s.routed == nil || s.protector == nil {
		return 0, ErrExportUnavailable
	}
	if limit < 1 || limit > 20 {
		limit = 20
	}
	count, err := s.store.Cleanup(ctx, limit, s.now().UTC(), func(cleanupCtx context.Context, job DirectoryExportJob) error {
		if job.ObjectID != nil {
			if s.routed != nil {
				return s.routed.Remove(cleanupCtx, job)
			}
			return s.objects.Remove(cleanupCtx, exportObjectKey(*job.ObjectID))
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
		if job.Status != DirectoryExportPending {
			return nil
		}
		now := s.now().UTC()
		if !now.Before(job.ExpiresAt) {
			if s.routed != nil {
				routedTx, ok := tx.(RoutedExportTx)
				if !ok || routedTx.CancelExportObject(ctx, job) != nil {
					return ErrExportUnavailable
				}
			}
			job.Status = DirectoryExportExpired
			job.Snapshot = nil
			job.UpdatedAt = now
			return tx.Save(job)
		}
		if _, err := tx.Authorize(job.Actor, job.TeamID, job.TeamRevision, false, s.now); err != nil {
			if !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrSessionInvalid) {
				return err
			}
			if s.routed != nil {
				routedTx, ok := tx.(RoutedExportTx)
				if !ok || routedTx.CancelExportObject(ctx, job) != nil {
					return ErrExportUnavailable
				}
			}
			job.Status = DirectoryExportRevoked
			job.Snapshot = nil
			job.ErrorCategory = "authority_revoked"
			job.UpdatedAt = now
			return tx.Save(job)
		}
		if s.routed != nil && job.ObjectLocation == nil {
			routedTx, ok := tx.(RoutedExportTx)
			if !ok {
				return ErrExportUnavailable
			}
			location, err := routedTx.PinExportObject(ctx, job)
			if err != nil {
				return err
			}
			job.ObjectLocation = &location
		}
		job.Status = DirectoryExportRunning
		job.UpdatedAt = now
		if s.routed != nil {
			routedTx, ok := tx.(RoutedExportTx)
			if !ok || job.ObjectLocation == nil {
				return ErrExportUnavailable
			}
			deadline, bounded := ctx.Deadline()
			if !bounded {
				return ErrExportUnavailable
			}
			if err := routedTx.BeginExportObjectWrite(ctx, job, deadline); err != nil {
				return err
			}
		}
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
		if s.routed != nil {
			if job.ObjectLocation == nil {
				category = "storage_failed"
			} else {
				digest := sha256.Sum256(ciphertext)
				hash := hex.EncodeToString(digest[:])
				if err := s.routed.PutIfAbsent(ctx, *job.ObjectLocation, bytes.NewReader(ciphertext), int64(len(ciphertext)), "application/octet-stream", hash); err != nil {
					category = "storage_failed"
				} else if info, statErr := s.routed.Stat(ctx, *job.ObjectLocation); statErr != nil || info.Size != int64(len(ciphertext)) || info.ContentType != "application/octet-stream" || info.SHA256 != hash {
					category = "storage_failed"
				} else {
					job.ObjectLocationInfo = &info
				}
			}
		} else if err := s.objects.Put(ctx, exportObjectKey(*job.ObjectID), bytes.NewReader(ciphertext), int64(len(ciphertext)), "application/octet-stream"); err != nil {
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
		if current.Status != DirectoryExportRunning {
			return nil
		}
		var routedTx RoutedExportTx
		if s.routed != nil {
			var ok bool
			routedTx, ok = tx.(RoutedExportTx)
			if !ok || current.ObjectLocation == nil {
				return ErrExportUnavailable
			}
		}
		now := s.now().UTC()
		if !now.Before(current.ExpiresAt) {
			if routedTx != nil {
				if err := routedTx.CancelExportObject(ctx, current); err != nil {
					return err
				}
			}
			current.Status = DirectoryExportExpired
			current.Snapshot = nil
			current.UpdatedAt = now
			return tx.Save(current)
		}
		if _, err := tx.Authorize(current.Actor, current.TeamID, current.TeamRevision, false, s.now); err != nil {
			if !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrSessionInvalid) {
				return err
			}
			if routedTx != nil {
				if cancelErr := routedTx.CancelExportObject(ctx, current); cancelErr != nil {
					return cancelErr
				}
			}
			current.Status = DirectoryExportRevoked
			current.Snapshot = nil
			current.ErrorCategory = "authority_revoked"
			current.UpdatedAt = now
			return tx.Save(current)
		}
		if category != "" {
			if routedTx != nil {
				if err := routedTx.CancelExportObject(ctx, current); err != nil {
					return err
				}
			}
			return s.fail(tx, current, category)
		}
		if routedTx != nil {
			if job.ObjectLocationInfo == nil {
				return ErrExportUnavailable
			}
			if err := routedTx.ConfirmExportObject(ctx, current, *job.ObjectLocationInfo); err != nil {
				return err
			}
			if err := routedTx.AcceptExportObject(ctx, current); err != nil {
				return err
			}
		}
		current.Status = DirectoryExportCompleted
		current.Snapshot = nil
		current.UpdatedAt = now
		current.ExpiresAt = now.Add(15 * time.Minute)
		return tx.Save(current)
	})
}

func (s *DirectoryExport) fail(tx ExportTx, job DirectoryExportJob, category string) error {
	job.Status = DirectoryExportFailed
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
