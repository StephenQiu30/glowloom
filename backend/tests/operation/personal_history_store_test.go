package operation_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type historySQLState struct {
	actor                  identityapp.Principal
	project, first, second uuid.UUID
	queries                []string
	args                   [][]driver.NamedValue
	authorized             []uuid.UUID
	denied                 bool
	outputReads            int
}
type historySQLDriver struct{ state *historySQLState }
type historySQLConn struct{ state *historySQLState }
type historySQLTx struct{}
type historySQLRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (d historySQLDriver) Open(string) (driver.Conn, error) { return &historySQLConn{d.state}, nil }
func (c *historySQLConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unsupported")
}
func (c *historySQLConn) Close() error              { return nil }
func (c *historySQLConn) Begin() (driver.Tx, error) { return historySQLTx{}, nil }
func (c *historySQLConn) BeginTx(_ context.Context, _ driver.TxOptions) (driver.Tx, error) {
	return historySQLTx{}, nil
}
func (historySQLTx) Commit() error          { return nil }
func (historySQLTx) Rollback() error        { return nil }
func (r *historySQLRows) Columns() []string { return r.columns }
func (r *historySQLRows) Close() error      { return nil }
func (r *historySQLRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
func (c *historySQLConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	s := c.state
	s.queries = append(s.queries, query)
	s.args = append(s.args, args)
	switch {
	case strings.Contains(query, "SELECT candidate.* FROM history_outputs"):
		s.outputReads++
		return &historySQLRows{columns: []string{"operation_id", "id", "sequence", "kind", "media_asset_id", "moderation_status", "create_time"}, values: [][]driver.Value{{s.first.String(), uuid.NewString(), int64(0), "media", uuid.NewString(), "passed", time.Unix(100, 0).UTC()}}}, nil
	case strings.Contains(query, "WITH personal_operation_ids"):
		at := time.Unix(100, 0).UTC()
		return &historySQLRows{columns: []string{"id", "project_id", "capability", "status", "create_time", "prompt_preview"}, values: [][]driver.Value{{s.first.String(), s.project.String(), "image.generate", "completed", at, "晨光"}, {s.second.String(), s.project.String(), "image.generate", "failed", at.Add(-time.Second), "细雨"}}}, nil
	case strings.HasPrefix(query, "SELECT 1 FROM workspace.project"):
		return &historySQLRows{columns: []string{"exists"}, values: [][]driver.Value{{int64(1)}}}, nil
	case strings.Contains(query, "FROM operation.operation_output"):
		s.outputReads++
		return &historySQLRows{columns: []string{"operation_id", "id", "sequence", "kind", "media_asset_id", "moderation_status", "create_time"}, values: [][]driver.Value{{s.first.String(), uuid.NewString(), int64(0), "image", uuid.NewString(), "passed", time.Unix(100, 0).UTC()}}}, nil
	default:
		return nil, errors.New("unexpected SQL: " + query)
	}
}

type historySQLAccess struct{ state *historySQLState }

func (a historySQLAccess) AuthorizeOperation(_ context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if actor != a.state.actor || write {
		return identityapp.ErrForbidden
	}
	a.state.authorized = append(a.state.authorized, project)
	if a.state.denied {
		return identityapp.ErrForbidden
	}
	return nil
}
func historyStore(t *testing.T, s *historySQLState) *pgoperation.Store {
	t.Helper()
	name := "personal-history-" + uuid.NewString()
	sql.Register(name, historySQLDriver{s})
	conn, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	gdb, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return pgoperation.NewScopedStore(gdb, func(*gorm.DB) operationapp.ProjectAccess { return historySQLAccess{s} }, nil)
}
func historySQLFixture() *historySQLState {
	return &historySQLState{actor: historyActor(), project: uuid.New(), first: uuid.New(), second: uuid.New()}
}
func TestPersonalHistoryStoreRequiresProvenInitiationAndBatchLoadsOutputs(t *testing.T) {
	s := historySQLFixture()
	from, before := time.Unix(1, 0).UTC(), time.Unix(200, 0).UTC()
	input := operationapp.PersonalHistoryInput{Kind: "image", Capability: "image.generate", Search: "晨光", CreatedFrom: &from, CreatedBefore: &before, Order: "desc", Limit: 2}
	page, err := historyStore(t, s).ListPersonalHistory(t.Context(), s.actor, input)
	if err != nil || len(page.Items) != 2 || len(page.Items[0].Outputs) != 1 || page.Items[1].Outputs == nil || s.outputReads != 1 || len(s.authorized) != 1 || s.authorized[0] != s.project {
		t.Fatalf("page=%+v err=%v outputReads=%d authorization=%v", page, err, s.outputReads, s.authorized)
	}
	query := s.queries[0]
	for _, required := range []string{"operation.prompt_submission", "ps.actor_id=", "operation.quote_request", "qr.actor_id=", "qr.org_id=", "jsonb_array_elements", "operation_id", "p.owner_scope_id=", "p.org_id=", "o.create_time>=", "o.create_time<", "m.display_name", "operation.operation_input", "ORDER BY o.create_time DESC,o.id DESC", "LIMIT"} {
		if !strings.Contains(query, required) {
			t.Errorf("query omitted required predicate %q", required)
		}
	}
	if strings.Contains(query, "confirmed_by") {
		t.Fatal("confirmer used as author")
	}
	values := s.args[0]
	hasActor, hasScope := false, false
	for _, arg := range values {
		hasActor = hasActor || arg.Value == s.actor.ID.String()
		hasScope = hasScope || arg.Value == s.actor.ScopeID.String()
	}
	if !hasActor || !hasScope {
		t.Fatalf("identity not bound to SQL: %+v", values)
	}
}
func TestPersonalHistoryStoreRevokedProjectCannotExposeCandidateOutputs(t *testing.T) {
	s := historySQLFixture()
	s.denied = true
	page, err := historyStore(t, s).ListPersonalHistory(t.Context(), s.actor, operationapp.PersonalHistoryInput{Limit: 2})
	if !errors.Is(err, identityapp.ErrForbidden) || s.outputReads != 0 || len(page.Items) != 0 {
		t.Fatalf("page=%+v err=%v outputReads=%d", page, err, s.outputReads)
	}
}
func TestPersonalHistoryStoreUsesLastReturnedTaskForCursor(t *testing.T) {
	s := historySQLFixture()
	page, err := historyStore(t, s).ListPersonalHistory(t.Context(), s.actor, operationapp.PersonalHistoryInput{Order: "asc", Limit: 1, After: &operationapp.TaskCursor{ID: uuid.New(), CreateTime: time.Unix(20, 0)}})
	if err != nil || len(page.Items) != 1 || page.Next == nil || page.Next.ID != s.first {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	q := s.queries[0]
	if !strings.Contains(q, "(o.create_time,o.id)>") || !strings.Contains(q, "ORDER BY o.create_time ASC,o.id ASC") {
		t.Fatal(q)
	}
}

func TestPersonalHistoryStoreMediaKindsUseCapabilityFamilies(t *testing.T) {
	for _, tc := range []struct{ kind, first, second string }{{"image", "image.%", ""}, {"video", "video.%", ""}, {"audio", "audio.%", ""}, {"document", "text.%", "document.%"}} {
		t.Run(tc.kind, func(t *testing.T) {
			s := historySQLFixture()
			_, err := historyStore(t, s).ListPersonalHistory(t.Context(), s.actor, operationapp.PersonalHistoryInput{Kind: tc.kind})
			if err != nil {
				t.Fatal(err)
			}
			q := s.queries[0]
			if !strings.Contains(q, "o.capability LIKE") {
				t.Fatal("media kind omitted from SQL")
			}
			hasFirst, hasSecond := false, tc.second == ""
			for _, arg := range s.args[0] {
				hasFirst = hasFirst || arg.Value == tc.first
				hasSecond = hasSecond || arg.Value == tc.second
			}
			if !hasFirst || !hasSecond {
				t.Fatalf("wrong capability family: %+v", s.args[0])
			}
		})
	}
}
