package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestOwnerIsolationCanonicalContract(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	schema := string(content)
	for _, required := range []string{
		"CREATE FUNCTION infra.runtime_owner() RETURNS text",
		"STABLE SECURITY INVOKER",
		"FORCE ROW LEVEL SECURITY",
		"service_owner text NOT NULL DEFAULT infra.runtime_owner()",
		"PRIMARY KEY (service_owner, kind, business_id)",
		"UNIQUE (service_owner, consumer, event_id)",
		"UNIQUE (service_owner, actor_id, idem_key)",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("canonical schema missing owner isolation contract %q", required)
		}
	}
}

func TestOwnerRuntimeIsolation(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_DB_DSN to a disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	pool, err := conn.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	execOwnerSQL(ctx, t, tx, "SET LOCAL row_security = on")

	// Session authorization, not an application GUC, determines the real LOGIN.
	// All fixture roles and rows disappear with this transaction.
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	owners := []string{"identity", "creation", "media", "execution", "legacy"}
	logins := make(map[string]string)
	for _, owner := range owners {
		capability := "glowloom_" + owner
		if owner == "legacy" {
			capability = "glowloom_app"
		}
		login := "owner_probe_" + owner + "_" + suffix
		logins[owner] = login
		execOwnerSQL(ctx, t, tx, "CREATE ROLE "+login+" LOGIN INHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS")
		execOwnerSQL(ctx, t, tx, "GRANT "+capability+" TO "+login)
	}
	var stable, invoker bool
	if err := tx.QueryRowContext(ctx, `SELECT provolatile = 's', NOT prosecdef FROM pg_proc WHERE oid = 'infra.runtime_owner()'::regprocedure`).Scan(&stable, &invoker); err != nil || !stable || !invoker {
		t.Fatalf("runtime_owner stable/invoker = %t/%t, error = %v", stable, invoker, err)
	}
	for _, owner := range owners[:4] {
		var safe bool
		if err := tx.QueryRowContext(ctx, `SELECT NOT (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls) AND NOT pg_has_role(oid,'glowloom_app','MEMBER') FROM pg_roles WHERE rolname=$1`, "glowloom_"+owner).Scan(&safe); err != nil || !safe {
			t.Fatalf("%s restricted capability = %t, error = %v", owner, safe, err)
		}
	}

	id, actor, event := uuid.NewString(), uuid.NewString(), uuid.NewString()
	// Identical primary/natural identities are independent in all five owners.
	inserts := []struct {
		table  string
		insert string
		filter string
	}{
		{"infra.task_work", fmt.Sprintf(`INSERT INTO infra.task_work(kind,business_id,next_run_at) VALUES ('owner.probe','%s',now()) ON CONFLICT (service_owner,kind,business_id) DO UPDATE SET next_run_at=excluded.next_run_at`, id), fmt.Sprintf("business_id='%s'", id)},
		{"infra.idempotency_record", fmt.Sprintf(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,expires_at) VALUES ('%s','%s','%s','hash',now()+interval '1 hour') ON CONFLICT(service_owner,actor_id,idem_key) DO UPDATE SET request_hash=excluded.request_hash`, id, actor, id), fmt.Sprintf("id='%s'", id)},
		{"infra.processed_event", fmt.Sprintf(`INSERT INTO infra.processed_event(id,consumer,event_id) VALUES ('%s','owner.probe','%s') ON CONFLICT(service_owner,consumer,event_id) DO NOTHING`, id, event), fmt.Sprintf("id='%s'", id)},
		{"infra.outbox", fmt.Sprintf(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES ('%s','owner.probe','key','{}') ON CONFLICT(service_owner,id,create_time) DO NOTHING`, id), fmt.Sprintf("id='%s'", id)},
		{"infra.outbox_dead_letter", fmt.Sprintf(`INSERT INTO infra.outbox_dead_letter(event_id,event_create_time,topic,partition_key,payload,attempt_count,error_class,failed_at) VALUES ('%s',now(),'owner.probe','key','{}',1,'probe',now()) ON CONFLICT(service_owner,event_id,event_create_time) DO UPDATE SET replayed_at=now()`, event), fmt.Sprintf("event_id='%s'", event)},
		{"infra.outbox_dead_letter_audit", fmt.Sprintf(`INSERT INTO infra.outbox_dead_letter_audit(id,event_id,event_create_time,operator_id,reason) VALUES ('%s','%s',now(),'%s','probe') ON CONFLICT(service_owner,id) DO NOTHING`, id, event, actor), fmt.Sprintf("id='%s'", id)},
		{"audit.audit_log", fmt.Sprintf(`INSERT INTO audit.audit_log(id,org_id,actor_kind,action,object_type,object_id) VALUES ('%s','%s','system','owner.probe','test','%s') ON CONFLICT(service_owner,id,create_time) DO NOTHING`, id, actor, id), fmt.Sprintf("id='%s'", id)},
	}
	business := map[string]string{"identity": "identity.user_session", "creation": "canvas.node", "media": "media.media_asset", "execution": "operation.operation"}
	for _, owner := range owners {
		t.Run(owner, func(t *testing.T) {
			execOwnerSQL(ctx, t, tx, "SET LOCAL SESSION AUTHORIZATION "+logins[owner])
			var got string
			if err := tx.QueryRowContext(ctx, "SELECT infra.runtime_owner()").Scan(&got); err != nil || got != owner {
				t.Fatalf("runtime_owner = %q, error = %v, want %q", got, err, owner)
			}
			execOwnerSQL(ctx, t, tx, "SET LOCAL glowloom.owner = 'execution'")
			execOwnerSQL(ctx, t, tx, "SET LOCAL glowloom.service_owner = 'execution'")
			for _, fixture := range inserts {
				execOwnerSQL(ctx, t, tx, fixture.insert)
				execOwnerSQL(ctx, t, tx, fixture.insert)
				var count int
				if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+fixture.table+" WHERE "+fixture.filter+" AND service_owner=infra.runtime_owner()").Scan(&count); err != nil || count != 1 {
					t.Fatalf("%s own count = %d, error = %v", fixture.table, count, err)
				}
				if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+fixture.table+" WHERE service_owner<>infra.runtime_owner()").Scan(&count); err != nil || count != 0 {
					t.Fatalf("%s cross-owner count = %d, error = %v", fixture.table, count, err)
				}
				assertAuditACLPermissionDenied(ctx, t, tx, "UPDATE "+fixture.table+" SET service_owner='"+otherOwner(owner)+"' WHERE "+fixture.filter)
				forgedInsert := strings.Replace(fixture.insert, "(", "(service_owner,", 1)
				forgedInsert = strings.Replace(forgedInsert, "VALUES (", "VALUES ('"+otherOwner(owner)+"',", 1)
				assertAuditACLPermissionDenied(ctx, t, tx, forgedInsert)
				assertAuditACLPermissionDenied(ctx, t, tx, "TRUNCATE "+fixture.table)
			}
			assertAuditACLPermissionDenied(ctx, t, tx, `INSERT INTO infra.task_work(service_owner,kind,business_id,next_run_at) VALUES ('`+otherOwner(owner)+`','owner.probe','forged',now()) ON CONFLICT(service_owner,kind,business_id) DO NOTHING`)
			assertAuditACLPermissionDenied(ctx, t, tx, `INSERT INTO infra.processed_event(service_owner,id,consumer,event_id) VALUES ('`+otherOwner(owner)+`','`+id+`','owner.probe','`+event+`') ON CONFLICT(service_owner,consumer,event_id) DO NOTHING`)
			assertAuditACLPermissionDenied(ctx, t, tx, "SET LOCAL row_security = off; SELECT * FROM infra.task_work")
			if owner != "legacy" {
				for domain, table := range business {
					if domain != owner {
						assertAuditACLPermissionDenied(ctx, t, tx, "SELECT * FROM "+table+" LIMIT 0")
						assertAuditACLPermissionDenied(ctx, t, tx, "DELETE FROM "+table+" WHERE false")
					}
				}
				assertAuditACLPermissionDenied(ctx, t, tx, "SET ROLE glowloom_app")
				assertAuditACLPermissionDenied(ctx, t, tx, "SELECT billing.ensure_creation_wallet(NULL)")
			}
			// Claim is the existing SKIP LOCKED mechanism, bounded by database RLS.
			var claimed int
			if err := tx.QueryRowContext(ctx, `WITH due AS (SELECT service_owner,kind,business_id FROM infra.task_work WHERE kind='owner.probe' FOR UPDATE SKIP LOCKED) UPDATE infra.task_work task SET owner='probe',lease_until=now()+interval '1 minute',epoch=task.epoch+1 FROM due WHERE task.service_owner=due.service_owner AND task.kind=due.kind AND task.business_id=due.business_id RETURNING task.epoch`).Scan(&claimed); err != nil || claimed != 1 {
				t.Fatalf("own task claim epoch = %d, error = %v", claimed, err)
			}
			execOwnerSQL(ctx, t, tx, "RESET SESSION AUTHORIZATION")
		})
	}

	// The ordinary account remains ambiguous even after SET ROLE selects one
	// capability. NOINHERIT must not turn a multi-owner LOGIN into a valid one.
	unknown, ambiguous := "owner_probe_unknown_"+suffix, "owner_probe_multi_"+suffix
	for _, login := range []string{unknown, ambiguous} {
		execOwnerSQL(ctx, t, tx, "CREATE ROLE "+login+" LOGIN NOINHERIT NOSUPERUSER NOBYPASSRLS")
		execOwnerSQL(ctx, t, tx, "GRANT USAGE ON SCHEMA infra TO "+login)
		execOwnerSQL(ctx, t, tx, "GRANT EXECUTE ON FUNCTION infra.runtime_owner() TO "+login)
	}
	execOwnerSQL(ctx, t, tx, "GRANT glowloom_identity,glowloom_execution TO "+ambiguous)
	for _, login := range []string{unknown, ambiguous} {
		execOwnerSQL(ctx, t, tx, "SET LOCAL SESSION AUTHORIZATION "+login)
		assertAuditACLPermissionDenied(ctx, t, tx, "SELECT infra.runtime_owner()")
		if login == ambiguous {
			execOwnerSQL(ctx, t, tx, "SET LOCAL ROLE glowloom_identity")
			assertAuditACLPermissionDenied(ctx, t, tx, "SELECT infra.runtime_owner()")
			assertAuditACLPermissionDenied(ctx, t, tx, "SELECT * FROM infra.task_work")
			execOwnerSQL(ctx, t, tx, "RESET ROLE")
		}
		execOwnerSQL(ctx, t, tx, "RESET SESSION AUTHORIZATION")
	}

	// Future offline partitions do not inherit the parent's runtime ACL.
	future := time.Date(time.Now().UTC().Year()+4, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, parent := range []string{"audit.audit_log", "infra.outbox"} {
		partition := parent + "_owner_probe_" + suffix
		execOwnerSQL(ctx, t, tx, fmt.Sprintf("CREATE TABLE %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')", partition, parent, future.Format(time.RFC3339), future.AddDate(0, 1, 0).Format(time.RFC3339)))
	}
	// Existing children also force RLS so an accidental direct grant cannot
	// bypass ownership. Future children remain inaccessible without explicit ACL.
	rows, err := tx.QueryContext(ctx, `SELECT child.oid::regclass::text,child.relrowsecurity,child.relforcerowsecurity FROM pg_inherits JOIN pg_class child ON child.oid=inhrelid WHERE inhparent IN ('audit.audit_log'::regclass,'infra.outbox'::regclass)`)
	if err != nil {
		t.Fatal(err)
	}
	var partitions []string
	for rows.Next() {
		var name string
		var enabled, forced bool
		if err := rows.Scan(&name, &enabled, &forced); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(name, "_owner_probe_"+suffix) && (!enabled || !forced) {
			t.Errorf("partition %s RLS enabled/forced = %t/%t", name, enabled, forced)
		}
		partitions = append(partitions, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	for _, owner := range owners {
		execOwnerSQL(ctx, t, tx, "SET LOCAL SESSION AUTHORIZATION "+logins[owner])
		for _, partition := range partitions {
			assertAuditACLPermissionDenied(ctx, t, tx, "SELECT * FROM "+partition+" LIMIT 0")
			assertAuditACLPermissionDenied(ctx, t, tx, "DELETE FROM "+partition+" WHERE false")
		}
		execOwnerSQL(ctx, t, tx, "RESET SESSION AUTHORIZATION")
	}
	// Even an offline accidental grant on an existing child still enforces RLS.
	currentOutbox := "infra.outbox_" + time.Now().UTC().Format("200601")
	execOwnerSQL(ctx, t, tx, "GRANT SELECT ON "+currentOutbox+" TO glowloom_identity")
	execOwnerSQL(ctx, t, tx, "SET LOCAL SESSION AUTHORIZATION "+logins["identity"])
	var ownRows, foreignRows int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FILTER (WHERE service_owner='identity'),count(*) FILTER (WHERE service_owner<>'identity') FROM "+currentOutbox+" WHERE id='"+id+"'").Scan(&ownRows, &foreignRows); err != nil || ownRows != 1 || foreignRows != 0 {
		t.Fatalf("direct child own/foreign rows = %d/%d, error = %v", ownRows, foreignRows, err)
	}
	execOwnerSQL(ctx, t, tx, "RESET SESSION AUTHORIZATION")
	var runtimeOwners int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pg_class WHERE relowner IN ('glowloom_identity'::regrole,'glowloom_creation'::regrole,'glowloom_media'::regrole,'glowloom_execution'::regrole)`).Scan(&runtimeOwners); err != nil || runtimeOwners != 0 {
		t.Fatalf("runtime-owned objects = %d, error = %v", runtimeOwners, err)
	}
	// Column-level business mutation contracts must not broaden with the split.
	for _, probe := range []struct{ role, table, column string }{
		{"glowloom_creation", "bible.character", "org_id"},
		{"glowloom_media", "media.library", "org_id"},
		{"glowloom_execution", "billing.reservation", "project_id"},
	} {
		var allowed bool
		if err := tx.QueryRowContext(ctx, `SELECT has_column_privilege($1,$2,$3,'UPDATE')`, probe.role, probe.table, probe.column).Scan(&allowed); err != nil || allowed {
			t.Errorf("%s UPDATE %s.%s = %t, error = %v", probe.role, probe.table, probe.column, allowed, err)
		}
	}
	var creationCommitment, executionCommitment bool
	if err := tx.QueryRowContext(ctx, `SELECT has_table_privilege('glowloom_creation','agent_business.operation_commitment','SELECT'),has_table_privilege('glowloom_execution','agent_business.operation_commitment','SELECT')`).Scan(&creationCommitment, &executionCommitment); err != nil || creationCommitment || !executionCommitment {
		t.Fatalf("operation_commitment creation/execution SELECT = %t/%t, error = %v", creationCommitment, executionCommitment, err)
	}
}

func execOwnerSQL(ctx context.Context, t *testing.T, tx *sql.Tx, statement string) {
	t.Helper()
	if _, err := tx.ExecContext(ctx, statement); err != nil {
		t.Fatalf("owner fixture %q: %v", statement, err)
	}
}

func otherOwner(owner string) string {
	if owner == "execution" {
		return "creation"
	}
	return "execution"
}
