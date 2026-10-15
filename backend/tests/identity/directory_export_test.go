package identity_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type exportStatisticsFixture struct {
	identity.DirectoryStatisticsStore
}

func (exportStatisticsFixture) UserStatistics(_ context.Context, _ identity.Principal, f identity.DirectoryFilter, _ func() time.Time) (identity.UserStatistics, error) {
	return identity.UserStatistics{Filter: identity.ExportFilter{Status: f.Status}}, nil
}
func (exportStatisticsFixture) MemberStatistics(_ context.Context, _ identity.Principal, _ uuid.UUID, f identity.ExportFilter, _ func() time.Time) (identity.MemberStatistics, error) {
	return identity.MemberStatistics{Filter: f}, nil
}
func TestDirectoryExportFilterBoundaries(t *testing.T) {
	stats := identity.NewDirectoryStatistics(exportStatisticsFixture{}, time.Now)
	for _, status := range []string{"active", "disabled", "deleted"} {
		if _, err := stats.Users(t.Context(), identity.Principal{}, identity.ExportFilter{Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := stats.Users(t.Context(), identity.Principal{}, identity.ExportFilter{Status: "pending_deletion"}); !errors.Is(err, identity.ErrInvalidDirectoryFilter) {
		t.Fatal("pending_deletion accepted")
	}
	if _, err := stats.Users(t.Context(), identity.Principal{}, identity.ExportFilter{Role: "team_admin"}); err == nil {
		t.Fatal("team role accepted in platform scope")
	}
	f, err := stats.Members(t.Context(), identity.Principal{}, uuid.New(), identity.ExportFilter{Role: "team_admin", Status: "removed"})
	if err != nil || f.Filter.Role != "team_admin" {
		t.Fatal("lost team filter", err)
	}
	if _, err := stats.Members(t.Context(), identity.Principal{}, uuid.New(), identity.ExportFilter{PlatformRole: "platform_admin"}); err == nil {
		t.Fatal("platform filter accepted in team scope")
	}
}
func TestDirectoryExportCSVFormulaSafety(t *testing.T) {
	service, store, _, _, actor := exportFixture(t)
	store.rows = [][]string{{"email", "name"}, {"=HYPERLINK(1)", "\t+cmd"}, {" \r@SUM(1)", "-2"}, {"normal", "a,b\n\"quoted\""}}
	job, err := service.Create(t.Context(), actor, nil, identity.ExportFilter{}, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.RunOnce(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	if err = service.Stream(t.Context(), actor, nil, job.ID, func(reader io.Reader) error { _, err := io.Copy(&data, reader); return err }); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"'=HYPERLINK(1)", "'\t+cmd", "' \r@SUM(1)", "'-2"} {
		if !strings.Contains(data.String(), want) {
			t.Fatalf("unsafe CSV: missing %q", want)
		}
	}
}

type exportMemoryStore struct {
	mu       sync.Mutex
	jobs     map[uuid.UUID]identity.DirectoryExportJob
	rows     [][]string
	denied   bool
	revision int64
}

func (f *exportMemoryStore) Transaction(_ context.Context, run func(identity.ExportTx) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return run(f)
}
func (f *exportMemoryStore) Authorize(_ identity.Principal, _ *uuid.UUID, revision int64, _ bool, _ func() time.Time) (int64, error) {
	if f.denied || revision != 0 && revision != f.revision {
		return 0, identity.ErrForbidden
	}
	return f.revision, nil
}
func (f *exportMemoryStore) Request(actor, scope, key uuid.UUID) (identity.DirectoryExportJob, bool, error) {
	for _, j := range f.jobs {
		if j.ActorID == actor && j.ScopeID == scope && j.RequestKey == key {
			return j, true, nil
		}
	}
	return identity.DirectoryExportJob{}, false, nil
}
func (f *exportMemoryStore) Load(id uuid.UUID) (identity.DirectoryExportJob, error) {
	j, ok := f.jobs[id]
	if !ok {
		return j, identity.ErrExportNotFound
	}
	return j, nil
}
func (f *exportMemoryStore) Snapshot(_ *uuid.UUID, _ identity.ExportFilter, _ time.Time) ([][]string, error) {
	return f.rows, nil
}
func (f *exportMemoryStore) Save(j identity.DirectoryExportJob) error { f.jobs[j.ID] = j; return nil }
func (f *exportMemoryStore) Pending(_ context.Context, limit int, now time.Time) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	for id, j := range f.jobs {
		if j.Status == "pending" && now.Before(j.ExpiresAt) && len(ids) < limit {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
func (f *exportMemoryStore) Cleanup(ctx context.Context, _ int, now time.Time, remove func(context.Context, identity.DirectoryExportJob) error) (int, error) {
	n := 0
	for id, j := range f.jobs {
		if !now.Before(j.ExpiresAt) && j.Status != "expired" {
			if err := remove(ctx, j); err != nil {
				return n, err
			}
			j.Status = "expired"
			j.Snapshot = nil
			j.ObjectID = nil
			f.jobs[id] = j
			n++
		}
	}
	return n, nil
}

type exportMemoryObjects struct {
	values       map[string][]byte
	onPut, onGet func()
	failPut      bool
}

func (f *exportMemoryObjects) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.values[key] = data
	if f.onPut != nil {
		f.onPut()
	}
	if f.failPut {
		return errors.New("simulated uncertain PUT")
	}
	return nil
}
func (f *exportMemoryObjects) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if f.onGet != nil {
		f.onGet()
	}
	return io.NopCloser(bytes.NewReader(f.values[key])), nil
}
func (f *exportMemoryObjects) Remove(_ context.Context, key string) error {
	delete(f.values, key)
	return nil
}
func exportFixture(t *testing.T) (*identity.DirectoryExport, *exportMemoryStore, *exportMemoryObjects, *time.Time, identity.Principal) {
	t.Helper()
	now := time.Now().UTC()
	store := &exportMemoryStore{jobs: map[uuid.UUID]identity.DirectoryExportJob{}, rows: [][]string{{"name"}, {"=danger"}}, revision: 1}
	objects := &exportMemoryObjects{values: map[string][]byte{}}
	protector, err := credentialproof.New("test", bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return identity.NewDirectoryExport(store, protector, objects, func() time.Time { return now }), store, objects, &now, identity.Principal{ID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1, PlatformEpoch: 1}
}
func TestDirectoryExportMaterializedEncryptedSnapshotAndExpiry(t *testing.T) {
	service, store, objects, now, actor := exportFixture(t)
	request := uuid.New()
	job, err := service.Create(t.Context(), actor, nil, identity.ExportFilter{}, request)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(job.Snapshot, []byte("danger")) {
		t.Fatal("plaintext snapshot")
	}
	replay, err := service.Create(t.Context(), actor, nil, identity.ExportFilter{}, request)
	if err != nil || replay.ID != job.ID {
		t.Fatal("lost idempotency", err)
	}
	if _, err := service.Create(t.Context(), actor, nil, identity.ExportFilter{Status: "active"}, request); !errors.Is(err, identity.ErrExportConflict) {
		t.Fatal("changed filter reused request key", err)
	}
	store.rows = [][]string{{"name"}, {"changed later"}}
	if _, err := service.RunOnce(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	complete := store.jobs[job.ID]
	if complete.Status != "completed" || len(complete.Snapshot) != 0 || !complete.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatal("completion contract")
	}
	for _, value := range objects.values {
		if bytes.Contains(value, []byte("danger")) {
			t.Fatal("plaintext private object")
		}
	}
	var output bytes.Buffer
	if err := service.Stream(t.Context(), actor, nil, job.ID, func(r io.Reader) error { _, err := io.Copy(&output, r); return err }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "'=danger") || strings.Contains(output.String(), "changed later") {
		t.Fatal("worker reread live rows or unsafe CSV")
	}
	*now = complete.ExpiresAt
	output.Reset()
	if err := service.Stream(t.Context(), actor, nil, job.ID, func(r io.Reader) error { _, err := io.Copy(&output, r); return err }); !errors.Is(err, identity.ErrExportNotFound) || output.Len() != 0 {
		t.Fatal("expiry accepted", err)
	}
	if _, err := service.RunOnce(t.Context(), 2); err != nil || len(objects.values) != 0 || len(store.jobs[job.ID].Snapshot) != 0 {
		t.Fatal("expiry cleanup", err)
	}
}
func TestDirectoryExportRevocationDuringObjectIO(t *testing.T) {
	for _, phase := range []string{"before_generation", "during_put", "during_get"} {
		t.Run(phase, func(t *testing.T) {
			service, store, objects, _, actor := exportFixture(t)
			job, err := service.Create(t.Context(), actor, nil, identity.ExportFilter{}, uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			switch phase {
			case "before_generation":
				store.denied = true
			case "during_put":
				objects.onPut = func() { store.denied = true }
			}
			if _, err := service.RunOnce(t.Context(), 2); err != nil {
				t.Fatal(err)
			}
			if phase != "during_get" && store.jobs[job.ID].Status != "revoked" {
				t.Fatal("revoked generation completed")
			}
			if phase == "during_get" {
				objects.onGet = func() { store.denied = true }
			}
			bytesWritten := 0
			err = service.Stream(t.Context(), actor, nil, job.ID, func(r io.Reader) error { data, _ := io.ReadAll(r); bytesWritten = len(data); return nil })
			if !errors.Is(err, identity.ErrForbidden) || bytesWritten != 0 {
				t.Fatal("revoked bytes escaped", err, bytesWritten)
			}
		})
	}
}
func TestDirectoryExportTenThousandBoundary(t *testing.T) {
	for _, n := range []int{10000, 10001} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			service, store, _, _, actor := exportFixture(t)
			store.rows = make([][]string, n+1)
			for i := range store.rows {
				store.rows[i] = []string{"safe"}
			}
			job, err := service.Create(t.Context(), actor, nil, identity.ExportFilter{}, uuid.New())
			if n == 10000 {
				if err != nil || job.RowCount != n {
					t.Fatal("10000 rejected", err)
				}
			} else if !errors.Is(err, identity.ErrExportLimit) || len(store.jobs) != 0 {
				t.Fatal("10001 not rejected atomically", err)
			}
		})
	}
}

type cleanupContextStore struct {
	*exportMemoryStore
	job identity.DirectoryExportJob
}

func (s cleanupContextStore) Cleanup(ctx context.Context, _ int, _ time.Time, remove func(context.Context, identity.DirectoryExportJob) error) (int, error) {
	bounded, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	return 0, remove(bounded, s.job)
}

type cleanupBlockingObjects struct {
	*exportMemoryObjects
	sawDeadline bool
}

func (o *cleanupBlockingObjects) Remove(ctx context.Context, _ string) error {
	_, o.sawDeadline = ctx.Deadline()
	if !o.sawDeadline {
		return errors.New("Remove received unbounded parent context")
	}
	<-ctx.Done()
	return ctx.Err()
}
func TestDirectoryExportCleanupDeadlineReachesObjectClient(t *testing.T) {
	_, store, objects, _, _ := exportFixture(t)
	object := uuid.New()
	blocking := &cleanupBlockingObjects{exportMemoryObjects: objects}
	protector, err := credentialproof.New("test", bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := identity.NewDirectoryExport(cleanupContextStore{exportMemoryStore: store, job: identity.DirectoryExportJob{ObjectID: &object}}, protector, blocking, time.Now)
	_, err = service.RunOnce(t.Context(), 1)
	if !blocking.sawDeadline || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("cleanup discarded its I/O deadline", err)
	}
}
func TestDirectoryExportConcurrentRequestBinding(t *testing.T) {
	for _, different := range []bool{false, true} {
		t.Run(strconv.FormatBool(different), func(t *testing.T) {
			service, store, _, _, actor := exportFixture(t)
			key := uuid.New()
			start := make(chan struct{})
			type outcome struct {
				job identity.DirectoryExportJob
				err error
			}
			results := make(chan outcome, 2)
			for i := 0; i < 2; i++ {
				go func(index int) {
					<-start
					filter := identity.ExportFilter{}
					if different && index == 1 {
						filter.Status = "deleted"
					}
					job, err := service.Create(t.Context(), actor, nil, filter, key)
					results <- outcome{job, err}
				}(i)
			}
			close(start)
			a, b := <-results, <-results
			if len(store.jobs) != 1 {
				t.Fatal("concurrent requests created multiple jobs")
			}
			if !different {
				if a.err != nil || b.err != nil || a.job.ID != b.job.ID {
					t.Fatal("equal concurrent requests did not replay", a.err, b.err)
				}
			} else if (a.err != nil || !errors.Is(b.err, identity.ErrExportConflict)) && (b.err != nil || !errors.Is(a.err, identity.ErrExportConflict)) {
				t.Fatal("changed concurrent request did not conflict", a.err, b.err)
			}
		})
	}
}
