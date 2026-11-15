package identityregistration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgcreation "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	creation "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func governanceLegacyDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := os.Getenv("GLOWLOOM_TEST_REGISTRATION_DB_NAME")
	// The owning fixture already validates this exact isolated database name.
	if name == "" {
		t.Skip("requires dedicated registration database")
	}
	connection, err := db.Open(t.Context(), fmt.Sprintf("postgres://owner_b_legacy_test@127.0.0.1:52080/%s?sslmode=disable", name), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	var user struct{ SessionUser, CurrentUser string }
	if err := connection.DB.Raw(`SELECT session_user,current_user`).Scan(&user).Error; err != nil || user.SessionUser != "owner_b_legacy_test" || user.CurrentUser != "owner_b_legacy_test" {
		t.Fatal("latch test requires actual legacy login, no SET ROLE substitute")
	}
	var private bool
	if err := connection.DB.Raw(`SELECT has_table_privilege(current_user,'workspace.account_governance_preparation','SELECT')`).Scan(&private).Error; err != nil || private {
		t.Fatal("legacy role gained private governance table SELECT")
	}
	return connection.DB
}

func latchFixture(t *testing.T) (*registrationFixture, creation.AccountGovernanceIntent, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	f, _, r := acceptedTeamQualificationFixture(t)
	p, err := identity.NewTeamQualifications(pgidentity.NewTeamQualificationStore(f.idb)).Intent(t.Context(), r.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	team, otherTeam, scope, organization := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, entry := range []struct{ team, owner uuid.UUID }{{team, p.AccountID}, {otherTeam, r.CandidateID}} {
		if err := f.cdb.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'management latch fixture',?)`, entry.team, entry.owner).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.cdb.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'management latch fixture')`, organization).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.cdb.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, scope, team, organization).Error; err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		team, user uuid.UUID
		role       string
	}{{team, p.AccountID, "team_admin"}, {team, r.CandidateID, "member"}, {otherTeam, r.CandidateID, "team_admin"}} {
		if err := f.cdb.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,?,'active',statement_timestamp(),'created')`, entry.team, entry.user, entry.role).Error; err != nil {
			t.Fatal(err)
		}
	}
	return f, creationIntent(p), team, otherTeam, r.CandidateID
}
func requireLatchError(t *testing.T, err error) {
	t.Helper()
	var state interface{ SQLState() string }
	if !errors.As(err, &state) || state.SQLState() != "55P03" {
		t.Fatalf("management latch expected 55P03: %v", err)
	}
}

func TestTeamGovernanceLatchActualLegacyCannotForgeMarker(t *testing.T) {
	f, intent, team, otherTeam, member := latchFixture(t)
	legacy := governanceLegacyDB(t)
	if err := legacy.Exec(`UPDATE workspace.team SET name='normal legacy management before pending' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcreation.NewAccountGovernanceStore(f.cdb)
	if _, err := store.AcceptAccountGovernance(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []struct {
		name, sql string
		args      []any
	}{
		{"team update", `UPDATE workspace.team SET name='forged management' WHERE id=?`, []any{team}},
		{"member update", `UPDATE workspace.team_member SET role='team_admin' WHERE team_id=? AND user_id=?`, []any{team, member}},
		{"member delete", `DELETE FROM workspace.team_member WHERE team_id=? AND user_id=?`, []any{team, member}},
		{"new membership", `INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',statement_timestamp(),'created')`, []any{otherTeam, intent.AccountID}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			err := legacy.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec(`SELECT set_config('glowloom.creation_governance_request',?,true)`, intent.RequestID.String()).Error; err != nil {
					return err
				}
				return tx.Exec(operation.sql, operation.args...).Error
			})
			if operation.name == "member delete" {
				var state interface{ SQLState() string }
				if !errors.As(err, &state) || state.SQLState() != "42501" {
					t.Fatalf("legacy physical deletion lost its original permission denial: %v", err)
				}
			} else {
				requireLatchError(t, err)
			}
		})
	}
	service, err := creation.NewAccountGovernance(store, directGovernanceQualification{source: identity.NewTeamQualifications(pgidentity.NewTeamQualificationStore(f.idb))}, f.creationWriters)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := service.Resolve(t.Context(), intent.RequestID); err != nil || !receipt.RejectedDuties {
		t.Fatalf("owned rejection did not finish: %v", err)
	}
	if err := legacy.Exec(`UPDATE workspace.team SET name='normal legacy management after rejection' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
}

func TestTeamGovernanceLatchStatementBeforePrepareSeesCommittedPending(t *testing.T) {
	f, intent, team, _, _ := latchFixture(t)
	legacy := governanceLegacyDB(t)
	preparer := registrationDB(t, "creation")
	pool, err := preparer.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	var preparePID int
	if err := preparer.Raw(`SELECT pg_backend_pid()`).Scan(&preparePID).Error; err != nil {
		t.Fatal(err)
	}
	legacyPool, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}
	legacyPool.SetMaxOpenConns(1)
	var legacyPID int
	if err := legacy.Raw(`SELECT pg_backend_pid()`).Scan(&legacyPID).Error; err != nil {
		t.Fatal(err)
	}
	blocker := f.cdb.Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	defer func() { _ = blocker.Rollback().Error }()
	if err := blocker.Exec(`SELECT id FROM workspace.team WHERE id=? FOR UPDATE`, team).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	prepared := make(chan error, 1)
	go func() {
		_, err := pgcreation.NewAccountGovernanceStore(preparer).AcceptAccountGovernance(ctx, intent)
		prepared <- err
	}()
	waitAdvisory(t, f.cdb, preparePID, true)
	updated := make(chan error, 1)
	go func() {
		updated <- legacy.WithContext(ctx).Exec(`UPDATE workspace.team SET name='started before pending commit' WHERE id=?`, team).Error
	}()
	waitAdvisory(t, f.cdb, legacyPID, false)
	if err := blocker.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-prepared; err != nil {
		t.Fatal(err)
	}
	requireLatchError(t, <-updated)
}

func waitAdvisory(t *testing.T, database *gorm.DB, pid int, granted bool) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var found bool
		if err := database.Raw(`SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=? AND locktype='advisory' AND granted=?)`, pid, granted).Scan(&found).Error; err != nil {
			t.Fatal(err)
		}
		if found {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("management statement did not reach its expected durable lock wait")
		case <-tick.C:
		}
	}
}
