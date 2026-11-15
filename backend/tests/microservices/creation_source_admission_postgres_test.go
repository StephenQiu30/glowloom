package microservices_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func sourceRequest(f *sourcePolicyFixture) (workspacesource.Identity, workspacesource.AdmissionRequest) {
	return workspacesource.Identity{AccountID: f.request.ActorID, SessionID: f.request.SessionID, CredentialEpoch: 1}, workspacesource.AdmissionRequest{Command: f.request.Command, Target: writefence.WriterExecution, ScopeID: f.request.ScopeID, ProjectID: f.request.ProjectID}
}

func TestCreationSourcePostgresChecksCurrentPolicyBeforeExactAdmission(t *testing.T) {
	for _, change := range []string{"", "foreign-owner", "archived-project", "deleted-project", "copying-project", "disabled-organization", "closed-scope", "pending-project", "stale-scope"} {
		t.Run(change, func(t *testing.T) {
			f := newSourcePolicyFixture(t)
			id, request := sourceRequest(f)
			switch change {
			case "foreign-owner":
				id.AccountID = uuid.New()
				request.Command.Authorities[0].ID = id.AccountID
			case "archived-project":
				if err := f.admin.Exec(`UPDATE workspace.project SET status='archived',archived_at=now() WHERE id=?`, request.ProjectID).Error; err != nil {
					t.Fatal(err)
				}
			case "deleted-project":
				if err := f.admin.Exec(`UPDATE workspace.project SET is_delete=true,delete_time=now(),purge_after=now()+interval '30 days' WHERE id=?`, request.ProjectID).Error; err != nil {
					t.Fatal(err)
				}
			case "copying-project":
				if err := f.admin.Exec(`UPDATE workspace.project SET status='copying' WHERE id=?`, request.ProjectID).Error; err != nil {
					t.Fatal(err)
				}
			case "disabled-organization":
				if err := f.admin.Exec(`UPDATE workspace.organization SET status='disabled' WHERE id=(SELECT physical_scope_id FROM workspace.creation_scope WHERE id=?)`, request.ScopeID).Error; err != nil {
					t.Fatal(err)
				}
			case "closed-scope", "stale-scope":
				for _, a := range request.Command.Authorities {
					if a.Issuer == writefence.Creation && a.Kind == writefence.Scope {
						completedAuthority(t, pgfence.NewAuthorityStore(f.creation), a, change == "closed-scope")
					}
				}
			case "pending-project":
				for _, a := range request.Command.Authorities {
					if a.Issuer == writefence.Creation && a.Kind == writefence.Project {
						if _, err := pgfence.NewAuthorityStore(f.creation).Begin(t.Context(), a, uuid.New(), true); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			store := pgsource.New(f.creation)
			permit, err := store.RegisterSourceCommand(t.Context(), id, request)
			if change == "" {
				if err != nil || permit.Command.ID != request.Command.ID || permit.PhysicalScopeID == uuid.Nil {
					t.Fatalf("source admission: %+v %v", permit, err)
				}
				if _, err := store.RegisterSourceCommand(t.Context(), id, request); err != nil {
					t.Fatal("exact replay", err)
				}
				changed := request
				changed.Command.SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
				if _, err := store.RegisterSourceCommand(t.Context(), id, changed); !errors.Is(err, writefence.ErrConflict) {
					t.Fatalf("rebound command: %v", err)
				}
			} else if err == nil {
				t.Fatal("closed policy registered a command")
			}
			var count int
			if err := f.admin.Raw(`SELECT count(*) FROM infra.authority_admission WHERE service_owner='creation' AND id=?`, request.Command.ID).Scan(&count).Error; err != nil {
				t.Fatal(err)
			}
			want := 0
			if change == "" {
				want = 1
			}
			if count != want {
				t.Fatalf("admissions = %d, want %d", count, want)
			}
		})
	}
}

func TestCreationSourcePostgresRequiresActualOwnerAndRefusesOuterTransaction(t *testing.T) {
	f := newSourcePolicyFixture(t)
	id, request := sourceRequest(f)
	for _, db := range []*gorm.DB{f.identity, f.media, f.execution, f.admin} {
		if _, err := pgsource.New(db).RegisterSourceCommand(t.Context(), id, request); !errors.Is(err, writefence.ErrInvalid) {
			t.Fatalf("wrong actual owner accepted: %v", err)
		}
	}
	if err := f.creation.Transaction(func(tx *gorm.DB) error {
		_, err := pgsource.New(tx).RegisterSourceCommand(t.Context(), id, request)
		if !errors.Is(err, writefence.ErrTransaction) {
			t.Fatalf("outer SQL transaction accepted: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`SELECT 1 FROM identity.user_session LIMIT 1`, `SELECT 1 FROM billing.budget LIMIT 1`, `SELECT 1 FROM media.asset LIMIT 1`} {
		if err := f.creation.Exec(query).Error; err == nil {
			t.Fatal("Creation source role gained another owner's table")
		}
	}
}

func makeSourceTeam(t *testing.T, f *sourcePolicyFixture) uuid.UUID {
	t.Helper()
	team := uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.team(id,name,created_by) VALUES (?,'source team',?)`, []any{team, f.request.ActorID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES (?,?,'member','active',now(),'created')`, []any{team, f.request.ActorID}},
		{`UPDATE workspace.creation_scope SET kind='team',owner_user_id=NULL,team_id=? WHERE id=?`, []any{team, f.request.ScopeID}},
	} {
		if err := f.admin.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	return team
}

func TestCreationSourcePostgresUsesCurrentTeamFacts(t *testing.T) {
	for _, change := range []string{"", "removed", "suspended", "archived", "governance"} {
		t.Run(change, func(t *testing.T) {
			f := newSourcePolicyFixture(t)
			team := makeSourceTeam(t, f)
			id, request := sourceRequest(f)
			switch change {
			case "removed", "suspended":
				if err := f.admin.Exec(`UPDATE workspace.team_member SET status=? WHERE team_id=? AND user_id=?`, change, team, id.AccountID).Error; err != nil {
					t.Fatal(err)
				}
			case "archived":
				if err := f.admin.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team).Error; err != nil {
					t.Fatal(err)
				}
			case "governance":
				if err := f.admin.Exec(`UPDATE workspace.team SET governance_blocked=true WHERE id=?`, team).Error; err != nil {
					t.Fatal(err)
				}
			}
			_, err := pgsource.New(f.creation).RegisterSourceCommand(t.Context(), id, request)
			if (change == "" && err != nil) || (change != "" && !errors.Is(err, workspacesource.ErrScopeNotFound)) {
				t.Fatalf("team policy %s: %v", change, err)
			}
		})
	}
}

func TestCreationSourcePostgresAdmissionSerializesWithClosureAtOriginalResourceLocks(t *testing.T) {
	for _, resource := range []string{"project", "member"} {
		for _, admitFirst := range []bool{true, false} {
			name := resource + "-closure-first"
			if admitFirst {
				name = resource + "-admission-first"
			}
			t.Run(name, func(t *testing.T) { testSourcePolicyRace(t, resource, admitFirst) })
		}
	}
}

func testSourcePolicyRace(t *testing.T, resource string, admitFirst bool) {
	t.Helper()
	f := newSourcePolicyFixture(t)
	id, request := sourceRequest(f)
	var team uuid.UUID
	if resource == "member" {
		team = makeSourceTeam(t, f)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	var workers sync.WaitGroup
	hold := f.admin.WithContext(ctx).Begin()
	if hold.Error != nil {
		t.Fatal(hold.Error)
	}
	defer func() { cancel(); _ = hold.Rollback().Error; workers.Wait() }()
	closeQuery := `UPDATE workspace.project SET is_delete=true,delete_time=now(),purge_after=now()+interval '30 days' WHERE id=?`
	args := []any{request.ProjectID}
	if resource == "member" {
		closeQuery = `UPDATE workspace.team_member SET status='removed',auth_epoch=auth_epoch+1 WHERE team_id=? AND user_id=?`
		args = []any{team, id.AccountID}
	}
	if admitFirst {
		if err := hold.Exec(`SELECT 1 FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='project' AND authority_id=? FOR UPDATE`, request.ProjectID).Error; err != nil {
			t.Fatal(err)
		}
	} else {
		if err := hold.Exec(closeQuery, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	admitted := make(chan error, 1)
	workers.Go(func() { _, err := pgsource.New(f.creation).RegisterSourceCommand(ctx, id, request); admitted <- err })
	needle := "workspace.project"
	if resource == "member" {
		needle = "workspace.team_member"
	}
	if admitFirst {
		needle = "authority_head"
	}
	waitSourcePolicyBlock(ctx, t, f.admin, "owner_b_creation_test", needle)
	var closed chan error
	if admitFirst {
		closed = make(chan error, 1)
		workers.Go(func() { closed <- f.admin.WithContext(ctx).Exec(closeQuery, args...).Error })
		waitSourcePolicyBlock(ctx, t, f.admin, "postgres", map[string]string{"project": "UPDATE workspace.project", "member": "UPDATE workspace.team_member"}[resource])
	}
	if err := hold.Commit().Error; err != nil {
		t.Fatal(err)
	}
	err := <-admitted
	if admitFirst {
		if err != nil {
			t.Fatal("first admission lost", err)
		}
		if err := <-closed; err != nil {
			t.Fatal(err)
		}
	} else if err == nil {
		t.Fatal("closure-first admitted new work")
	}
	var count int
	if err := f.admin.Raw(`SELECT count(*) FROM infra.authority_admission WHERE service_owner='creation' AND id=?`, request.Command.ID).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	want := 0
	if admitFirst {
		want = 1
	}
	if count != want {
		t.Fatalf("source ledger crossed policy closure: %d want %d", count, want)
	}
}

func waitSourcePolicyBlock(ctx context.Context, t *testing.T, db *gorm.DB, user, query string) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=? AND query LIKE ? AND cardinality(pg_blocking_pids(pid))>0)`, user, "%"+query+"%").Scan(&blocked).Error; err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("no real resource-lock contention observed: %s %s", user, query)
		}
	}
}
