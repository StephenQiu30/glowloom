package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	domain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

type copyPageLockTrace struct {
	logger.Interface
	projects []string
}

func (l *copyPageLockTrace) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sql, _ := query()
	if strings.Contains(sql, "FROM workspace.project WHERE") && strings.Contains(sql, "FOR SHARE") {
		l.projects = append(l.projects, sql)
	}
}

func TestSecondaryCopyListLockOrder(t *testing.T) {
	for _, size := range []int{1, 2} {
		t.Run(fmt.Sprintf("page_%d", size), func(t *testing.T) {
			verifySecondaryRollback(t)
			tx, actor, team, scope, _, requests := projectConsumerFixture(t)
			personal := actor.ScopeID
			actor.ScopeID = scope
			source, low, high := uuid.New(), uuid.New(), uuid.New()
			source[0], low[0], high[0] = 0x80, 0x10, 0xf0
			for _, id := range []uuid.UUID{source, low, high} {
				if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Copy target','16:9','realistic')`, id, actor.OrgID, actor.ID, scope).Error; err != nil {
					t.Fatal(err)
				}
			}
			// The highest target is deliberately first in the multi-job page: sorting
			// each individual pair would still acquire source before the later low target.
			targets := []uuid.UUID{low}
			if size == 2 {
				targets = []uuid.UUID{high, low}
			}
			ids := make([]uuid.UUID, size)
			for i, target := range targets {
				digest := strings.Repeat("a", 64)
				job := domain.ProjectCopyJob{ID: uuid.New(), OrgID: actor.OrgID, ActorID: actor.ID, SourceProjectID: source, TargetProjectID: target, SourceRevision: 1, Revision: 1, TargetName: "Copy target", Status: "queued", Stage: "media", RequestID: uuid.NewString(), Manifest: domain.ProjectCopyManifest{WorkspaceSHA256: digest, MediaSnapshotID: uuid.New(), MediaSHA256: digest, CanvasSnapshotID: uuid.New(), CanvasSHA256: digest}}
				ids[i] = job.ID
				manifest, _ := json.Marshal(job.Manifest)
				response, _ := json.Marshal(job)
				if err := tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,owner_scope_id,actor_id,request_id,idem_key,request_sha256,admission_response,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,create_time) VALUES(?,?,?,?,?,?,?,?::jsonb,?,1,?,?,'queued','media',?::jsonb,'{}',?)`, job.ID, actor.OrgID, scope, actor.ID, job.RequestID, uuid.New(), digest, string(response), source, target, job.TargetName, string(manifest), time.Now().Add(-time.Duration(i)*time.Hour)).Error; err != nil {
					t.Fatal(err)
				}
			}
			trace := &copyPageLockTrace{Interface: logger.Default.LogMode(logger.Silent)}
			store := pgworkspace.NewScopedProjectCopyStore(tx.Session(&gorm.Session{Logger: trace}), requests, secondaryWorkers, nil, nil)
			page, err := store.List(t.Context(), actor, workspace.ProjectCopyListInput{SourceProjectID: source, Limit: size})
			if err != nil || len(page.Jobs) != size {
				t.Fatalf("actual List: count=%d err=%v", len(page.Jobs), err)
			}
			for i, job := range page.Jobs {
				if job.ID != ids[i] {
					t.Error("page order changed")
				}
			}
			captured := append([]string(nil), trace.projects...)
			t.Logf("actual List project lock queries: %v", captured)
			if len(captured) != 1 {
				t.Errorf("page must acquire all project locks once, got %d acquisitions", len(captured))
			} else {
				for _, id := range append([]uuid.UUID{source}, targets...) {
					if !strings.Contains(captured[0], id.String()) {
						t.Errorf("page lock omitted %s", id)
					}
				}
				if !strings.Contains(captured[0], "ORDER BY id FOR SHARE") {
					t.Error("page locks lack global UUID order")
				}
			}
			exerciseCopyPageLocks(t, captured, actor.OrgID, scope, source, low, high, size)
			// The page-wide path must retain current scope/session gates, including empty pages.
			other := actor
			other.ScopeID = personal
			if _, err := store.List(t.Context(), other, workspace.ProjectCopyListInput{SourceProjectID: source, Limit: 2}); !errors.Is(err, workspace.ErrProjectNotFound) {
				t.Errorf("wrong scope empty page: %v", err)
			}
			empty, err := store.List(t.Context(), actor, workspace.ProjectCopyListInput{SourceProjectID: high, Limit: 2})
			if err != nil || len(empty.Jobs) != 0 || empty.Next != nil {
				t.Fatalf("authorized empty source: %+v %v", empty, err)
			}
			first, err := store.List(t.Context(), actor, workspace.ProjectCopyListInput{SourceProjectID: source, Limit: 1})
			if err != nil || len(first.Jobs) != 1 || first.Jobs[0].ID != ids[0] {
				t.Fatalf("first page: %v", err)
			}
			if size == 2 {
				if first.Next == nil {
					t.Fatal("next cursor absent")
				}
				next, err := store.List(t.Context(), actor, workspace.ProjectCopyListInput{SourceProjectID: source, Limit: 1, After: first.Next})
				if err != nil || len(next.Jobs) != 1 || next.Jobs[0].ID != ids[1] || next.Next != nil {
					t.Fatalf("second page: %+v %v", next, err)
				}
			}
			if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := store.List(t.Context(), actor, workspace.ProjectCopyListInput{SourceProjectID: source, Limit: 2}); !errors.Is(err, workspace.ErrScopeNotFound) {
				t.Errorf("revoked membership: %v", err)
			}
			if err := tx.Exec(`UPDATE workspace.team_member SET status='active' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := store.List(t.Context(), actor, workspace.ProjectCopyListInput{SourceProjectID: source, Limit: 2}); !errors.Is(err, identity.ErrSessionInvalid) {
				t.Errorf("revoked session: %v", err)
			}
		})
	}
}

// Execute only the actual List's project-lock SELECTs against existing IDs. No
// business content is read and no rows are inserted/updated on these connections.
// Rollback fixtures cannot be seen by a second connection, so this demonstrates
// PostgreSQL lock interleaving, not a full production copy/worker workflow.
func exerciseCopyPageLocks(t *testing.T, queries []string, org, scope, source, low, high uuid.UUID, size int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, os.Getenv("GLOWLOOM_TEST_EXISTING_DB_DSN"), noop.NewTracerProvider())
	if err != nil {
		t.Fatal("open existing lock harness database")
	}
	defer func() { _ = conn.Close() }()
	var rows []struct {
		ID, OrgID    uuid.UUID
		OwnerScopeID *uuid.UUID
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT id,org_id,owner_scope_id FROM workspace.project WHERE org_id=(SELECT org_id FROM workspace.project WHERE NOT is_delete GROUP BY org_id HAVING count(*)>=2 ORDER BY org_id LIMIT 1) AND NOT is_delete ORDER BY id LIMIT 3`).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatal("two existing same-org project IDs unavailable; lock interleaving unverified")
	}
	if size == 2 && len(rows) < 3 {
		t.Log("only two existing IDs: multi-job order checked by actual fixture SQL; two-connection interleaving covered by page_1")
		return
	}
	mappedSource := rows[1].ID
	mappedHigh := rows[len(rows)-1].ID
	scopeTerms := make([]string, 0, len(rows))
	for _, row := range rows {
		predicate := "owner_scope_id IS NULL"
		if row.OwnerScopeID != nil {
			predicate = "owner_scope_id='" + row.OwnerScopeID.String() + "'"
		}
		scopeTerms = append(scopeTerms, "(id='"+row.ID.String()+"' AND "+predicate+")")
	}
	// Only the fixture predicates are mapped to real IDs/scope values. ORDER BY
	// and the sequence of lock statements are the actual captured List SQL.
	replace := strings.NewReplacer("owner_scope_id='"+scope.String()+"'", "("+strings.Join(scopeTerms, " OR ")+")", org.String(), rows[0].OrgID.String(), source.String(), mappedSource.String(), low.String(), rows[0].ID.String(), high.String(), mappedHigh.String())
	t.Logf("lock harness uses %d existing same-org IDs; scope predicates mapped, content unread", len(rows))
	reader := conn.DB.WithContext(ctx).Begin()
	if reader.Error != nil {
		t.Fatal(reader.Error)
	}
	defer func() { _ = reader.Rollback().Error }()
	worker := conn.DB.WithContext(ctx).Begin()
	if worker.Error != nil {
		t.Fatal(worker.Error)
	}
	defer func() { _ = worker.Rollback().Error }()
	for _, tx := range []*gorm.DB{reader, worker} {
		if err := tx.Exec(`SET LOCAL lock_timeout='1500ms'; SET LOCAL statement_timeout='2500ms'`).Error; err != nil {
			t.Fatal(err)
		}
	}
	var pid int
	if err := reader.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error; err != nil {
		t.Fatal(err)
	}
	if err := worker.Exec(`SELECT id FROM workspace.project WHERE id=? FOR UPDATE`, rows[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		for _, query := range queries {
			if err := reader.Exec(replace.Replace(query)).Error; err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	// On every exit release the blocker, cancel the reader, and join its goroutine.
	joined := false
	defer func() {
		_ = worker.Rollback().Error
		cancel()
		if !joined {
			<-done
		}
	}()
	deadline := time.Now().Add(time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err := conn.DB.WithContext(ctx).Raw(`SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=?`, pid).Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("reader did not reach the target lock barrier")
	}
	// If List retained source before waiting for low target, this NOWAIT fails.
	// This proves the cycle's two edges without making PostgreSQL kill a victim.
	probe := worker.Exec(`SELECT id FROM workspace.project WHERE id=? FOR UPDATE NOWAIT`, mappedSource).Error
	if probe != nil {
		t.Errorf("worker cannot advance from low target to source while List waits: %v", probe)
	} else {
		t.Log("two-connection barrier: List waiting at low target; worker acquired source without waiting")
	}
	if err := worker.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		joined = true
		if err != nil {
			t.Errorf("List project locks after worker rollback: %v", err)
		}
	case <-ctx.Done():
		t.Error("reader did not finish after worker rollback")
	}
}
