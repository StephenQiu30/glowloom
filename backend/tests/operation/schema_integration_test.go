package operation_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func operationSchemaDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LV_TEST_OPERATION_SCHEMA_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_OPERATION_SCHEMA_DB_DSN to an isolated PostgreSQL database with migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open operation test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.DB.WithContext(ctx)
}

func operationSchemaProject(t *testing.T, database *gorm.DB) string {
	t.Helper()
	orgID, projectID := uuid.NewString(), uuid.NewString()
	if err := database.Exec(`INSERT INTO workspace.organization (id, name) VALUES (?::uuid, ?)`, orgID, "operation-"+orgID).Error; err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO workspace.project (id, org_id, name, aspect_ratio, style_type)
		VALUES (?::uuid, ?::uuid, '操作测试', '16:9', 'realistic')
	`, projectID, orgID).Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return projectID
}

func operationSchemaBatch(t *testing.T, database *gorm.DB, projectID string) string {
	t.Helper()
	id := uuid.NewString()
	if err := database.Exec(`
		INSERT INTO operation.batch
		  (id, project_id, kind, scope, status, total_count, quote_total_micros)
		VALUES (?::uuid, ?::uuid, 'keyframe', '{}'::jsonb, 'quoted', 1, 300)
	`, id, projectID).Error; err != nil {
		t.Fatalf("insert batch: %v", err)
	}
	return id
}

func operationSchemaQuoted(t *testing.T, database *gorm.DB, projectID string, batchID any) string {
	t.Helper()
	id := uuid.NewString()
	if err := database.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, batch_id, target_type, target_id, capability, mode,
		   model_profile_version_id, price_rule_version_id, input_hash, origin, status,
		   quote_micros, quote_expires_at, region)
		VALUES (?::uuid, ?::uuid, ?::uuid, 'shot_frame', ?::uuid, 'image.generate',
		        'text2image', ?::uuid, ?::uuid, ?, 'batch', 'quoted', 300,
		        now() + interval '15 minutes', 'domestic')
	`, id, projectID, batchID, uuid.NewString(), uuid.NewString(), uuid.NewString(), "hash-"+id).Error; err != nil {
		t.Fatalf("insert quoted operation: %v", err)
	}
	return id
}

func TestOperationSchemaEnforcesSameProjectRelationships(t *testing.T) {
	database := operationSchemaDB(t)
	projectA, projectB := operationSchemaProject(t, database), operationSchemaProject(t, database)
	batchA, batchB := operationSchemaBatch(t, database, projectA), operationSchemaBatch(t, database, projectB)
	opA, opB := operationSchemaQuoted(t, database, projectA, batchA), operationSchemaQuoted(t, database, projectB, batchB)

	if err := database.Exec(`
		INSERT INTO operation.operation_input (id, operation_id, seq_no, role, ref_type, text_value)
		VALUES (?::uuid, ?::uuid, 0, 'prompt', 'text', '一只猫')
	`, uuid.NewString(), opA).Error; err != nil {
		t.Fatalf("insert frozen input: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO operation.operation_input (id, operation_id, seq_no, role, ref_type)
		VALUES (?::uuid, ?::uuid, 0, 'prompt', 'text')
	`, uuid.NewString(), opA).Error; err == nil {
		t.Fatal("duplicate operation input sequence was accepted")
	}
	if err := database.Exec(`
		INSERT INTO operation.operation_input (id, operation_id, seq_no, role, ref_type)
		VALUES (?::uuid, ?::uuid, 0, 'prompt', 'text')
	`, uuid.NewString(), uuid.NewString()).Error; err == nil {
		t.Fatal("input without parent operation was accepted")
	}
	if err := database.Exec(`UPDATE operation.operation SET batch_id = ?::uuid WHERE id = ?::uuid`, batchB, opA).Error; err == nil {
		t.Fatal("cross-project batch membership was accepted")
	}
	if err := database.Exec(`UPDATE operation.operation SET reused_from_id = ?::uuid WHERE id = ?::uuid`, opB, opA).Error; err == nil {
		t.Fatal("cross-project reuse was accepted")
	}

	reservationA := uuid.NewString()
	if err := database.Exec(`
		INSERT INTO billing.reservation (id, project_id, operation_id, amount_micros, status)
		VALUES (?::uuid, ?::uuid, ?::uuid, 300, 'held')
	`, reservationA, projectA, opA).Error; err != nil {
		t.Fatalf("insert held reservation: %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation SET reservation_id = ?::uuid WHERE id = ?::uuid`, reservationA, opA).Error; err != nil {
		t.Fatalf("link own reservation: %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation SET reservation_id = ?::uuid WHERE id = ?::uuid`, reservationA, opB).Error; err == nil {
		t.Fatal("cross-project reservation link was accepted")
	}
	peerID := operationSchemaQuoted(t, database, projectA, batchA)
	if err := database.Exec(`UPDATE operation.operation SET reservation_id = ?::uuid WHERE id = ?::uuid`, reservationA, peerID).Error; err == nil {
		t.Fatal("another operation in same project linked this reservation")
	}
	if err := database.Exec(`
		INSERT INTO billing.reservation (id, project_id, operation_id, amount_micros, status)
		VALUES (?::uuid, ?::uuid, ?::uuid, 300, 'held')
	`, uuid.NewString(), projectB, opA).Error; err == nil {
		t.Fatal("reservation for another project's operation was accepted")
	}
	if err := database.Exec(`
		INSERT INTO billing.reservation (id, project_id, operation_id, amount_micros, status)
		VALUES (?::uuid, ?::uuid, ?::uuid, 300, 'held')
	`, uuid.NewString(), projectA, opA).Error; err == nil {
		t.Fatal("second reservation for one operation was accepted")
	}
	if err := database.Exec(`
		INSERT INTO billing.reservation (id, project_id, operation_id, amount_micros, status)
		VALUES (?::uuid, ?::uuid, ?::uuid, -1, 'held')
	`, uuid.NewString(), projectB, opB).Error; err == nil {
		t.Fatal("negative reservation amount was accepted")
	}
}

func TestOperationSchemaRejectsInvalidFinancialAndLifecycleValues(t *testing.T) {
	database := operationSchemaDB(t)
	projectID := operationSchemaProject(t, database)
	batchID := operationSchemaBatch(t, database, projectID)
	opID := operationSchemaQuoted(t, database, projectID, batchID)

	for name, statement := range map[string]string{
		"negative quote":          `UPDATE operation.operation SET quote_micros = -1 WHERE id = ?::uuid`,
		"negative settlement":     `UPDATE operation.operation SET settled_micros = -1 WHERE id = ?::uuid`,
		"invalid status":          `UPDATE operation.operation SET status = 'paid' WHERE id = ?::uuid`,
		"zero outputs":            `UPDATE operation.operation SET output_count = 0 WHERE id = ?::uuid`,
		"missing quote expiry":    `UPDATE operation.operation SET quote_expires_at = NULL WHERE id = ?::uuid`,
		"quote before creation":   `UPDATE operation.operation SET quote_expires_at = create_time - interval '1 second' WHERE id = ?::uuid`,
		"agent session no region": `UPDATE operation.operation SET target_type = 'agent_session', model_profile_version_id = NULL, price_rule_version_id = NULL, region = NULL WHERE id = ?::uuid`,
		"generated without model": `UPDATE operation.operation SET model_profile_version_id = NULL WHERE id = ?::uuid`,
		"invalid provider status": `UPDATE operation.operation SET status = 'processing' WHERE id = ?::uuid`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := database.Exec(statement, opID).Error; err == nil {
				t.Fatalf("invalid operation %q was accepted", name)
			}
		})
	}
	for name, statement := range map[string]string{
		"negative total":     `UPDATE operation.batch SET quote_total_micros = -1 WHERE id = ?::uuid`,
		"count beyond total": `UPDATE operation.batch SET succeeded_count = 2 WHERE id = ?::uuid`,
		"unsupported status": `UPDATE operation.batch SET status = 'submitting' WHERE id = ?::uuid`,
		"unsupported kind":   `UPDATE operation.batch SET kind = 'unknown' WHERE id = ?::uuid`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := database.Exec(statement, batchID).Error; err == nil {
				t.Fatalf("invalid batch %q was accepted", name)
			}
		})
	}

	uploadID := uuid.NewString()
	if err := database.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, target_type, capability, mode, input_hash, origin, status)
		VALUES (?::uuid, ?::uuid, 'shot_frame', 'video.upload', 'upload', ?,
		        'upload', 'draft')
	`, uploadID, projectID, "hash-"+uploadID).Error; err != nil {
		t.Fatalf("insert upload without provider model: %v", err)
	}

	agentID := uuid.NewString()
	if err := database.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, target_type, capability, mode, input_hash, origin,
		   status, quote_micros, quote_expires_at, region)
		VALUES (?::uuid, ?::uuid, 'agent_session', 'text.chat', 'chat', ?,
		        'agent', 'quoted', 1000, now() + interval '15 minutes', 'domestic')
	`, agentID, projectID, "hash-"+agentID).Error; err != nil {
		t.Fatalf("insert agent session without single model version: %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation SET model_profile_version_id = ?::uuid WHERE id = ?::uuid`, uuid.NewString(), agentID).Error; err == nil {
		t.Fatal("agent session tied to one model version was accepted")
	}
}
