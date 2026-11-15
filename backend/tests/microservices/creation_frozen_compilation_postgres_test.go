package microservices_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// This fixture installs source heads explicitly; it tests restricted source
// transactions, not public registration or the complete quote entry point.
func frozenCompilationFixture(t *testing.T, selection *prompt.TemplateRequest) (*frozenFixture, prompt.FrozenCatalogContext) {
	t.Helper()
	f := newFrozenFixture(t)
	model, mode := uuid.New(), "text"
	modelVersion := model.String()
	f.draft.ModelVersion, f.draft.Mode = &modelVersion, &mode
	f.draft.Params["output_count"] = float64(2)
	raw, _ := json.Marshal(f.draft)
	dh, ih := f.draft.Hashes()
	if err := f.creation.Exec(`UPDATE workspace.prompt_draft SET draft=?::jsonb,draft_hash=?,input_hash=? WHERE id=?`, string(raw), dh, ih, f.request.Source.DraftID).Error; err != nil {
		t.Fatal(err)
	}
	if selection != nil {
		// The original fixture has no effect or handoff yet; bind the selected
		// template under its own original source command identity before freezing.
		f.request.HandoffID, f.request.TemplateSelection = uuid.New(), selection
		f.bind(t)
	}
	return f, prompt.FrozenCatalogContext{ModelVersionID: model, ModelKey: "immutable-test-model", Capability: "text.structured", Mode: mode}
}

func frozenSkillSelection() *prompt.TemplateRequest {
	definition, _ := domain.DefinitionFor("skill_draft")
	revision := int64(0)
	return &prompt.TemplateRequest{Operation: definition.Operation, ExpectedTemplateID: definition.TemplateID, ExpectedCustomizationRevision: &revision}
}

func insertCompilationPreference(t *testing.T, f *frozenFixture, tx interface{ Exec(string, ...any) *gorm.DB }) {
	t.Helper()
	var organization struct{ OrgID uuid.UUID }
	if err := f.creation.Raw(`SELECT org_id FROM workspace.project WHERE id=?`, f.request.ProjectID).Scan(&organization).Error; err != nil {
		t.Fatal(err)
	}
	definition, _ := domain.DefinitionFor("skill_draft")
	if err := tx.Exec(`INSERT INTO workspace.prompt_customization(id,org_id,owner_id,operation,mode,content,base_template_id,revision) VALUES(?,?,?,?,'append','首次个人偏好',?,1)`, uuid.New(), organization.OrgID, f.request.ActorID, definition.Operation, definition.TemplateID).Error; err != nil {
		t.Fatal(err)
	}
}

func TestCreationFrozenCompilationPostgresAbsenceLockOrdersFirstPreferenceBothWays(t *testing.T) {
	for _, freezeFirst := range []bool{false, true} {
		name := "preference-first"
		if freezeFirst {
			name = "freeze-first"
		}
		t.Run(name, func(t *testing.T) {
			f, catalog := frozenCompilationFixture(t, frozenSkillSelection())
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			blocker := f.creation.WithContext(ctx).Begin()
			if blocker.Error != nil {
				t.Fatal(blocker.Error)
			}
			defer func() { _ = blocker.Rollback().Error }()
			var currentUser string
			if err := f.creation.Raw(`SELECT current_user`).Scan(&currentUser).Error; err != nil {
				t.Fatal(err)
			}
			var organization struct{ OrgID uuid.UUID }
			if err := f.creation.Raw(`SELECT org_id FROM workspace.project WHERE id=?`, f.request.ProjectID).Scan(&organization).Error; err != nil {
				t.Fatal(err)
			}
			lockKey := organization.OrgID.String() + "/" + f.request.ActorID.String() + "/skill_draft"
			if freezeFirst {
				if err := blocker.Exec(`SELECT id FROM workspace.prompt_draft WHERE id=? FOR UPDATE`, f.request.Source.DraftID).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := blocker.Exec(`SELECT pg_advisory_xact_lock(69361,hashtext(?))`, lockKey).Error; err != nil {
					t.Fatal(err)
				}
			}
			type freezeResult struct {
				envelope prompt.FrozenInputEnvelope
				err      error
			}
			frozen := make(chan freezeResult, 1)
			go func() { envelope, err := f.store.FreezeInput(ctx, f.request); frozen <- freezeResult{envelope, err} }()
			needle := "pg_advisory_xact_lock(69361"
			if freezeFirst {
				needle = "FROM workspace.prompt_draft"
			}
			waitSourcePolicyBlock(ctx, t, f.admin, currentUser, needle)
			if !freezeFirst {
				insertCompilationPreference(t, f, blocker)
				if err := blocker.Commit().Error; err != nil {
					t.Fatal(err)
				}
				result := <-frozen
				if !errors.Is(result.err, prompt.ErrTemplateChanged) {
					t.Fatal("absent preference raced first INSERT and froze stale revision", result.err)
				}
				var count int
				if err := f.creation.Raw(`SELECT count(*) FROM workspace.frozen_input_handoff WHERE handoff_id=?`, f.request.HandoffID).Scan(&count).Error; err != nil || count != 0 {
					t.Fatal("failed selection committed a handoff", count, err)
				}
				return
			}
			preferenceDone := make(chan error, 1)
			go func() {
				tx := f.creation.WithContext(ctx).Begin()
				if tx.Error != nil {
					preferenceDone <- tx.Error
					return
				}
				defer func() { _ = tx.Rollback().Error }()
				if err := tx.Exec(`SELECT pg_advisory_xact_lock(69361,hashtext(?))`, lockKey).Error; err != nil {
					preferenceDone <- err
					return
				}
				definition, _ := domain.DefinitionFor("skill_draft")
				if err := tx.Exec(`INSERT INTO workspace.prompt_customization(id,org_id,owner_id,operation,mode,content,base_template_id,revision) VALUES(?,?,?,?,'append','首次个人偏好',?,1)`, uuid.New(), organization.OrgID, f.request.ActorID, definition.Operation, definition.TemplateID).Error; err != nil {
					preferenceDone <- err
					return
				}
				preferenceDone <- tx.Commit().Error
			}()
			waitSourcePolicyBlock(ctx, t, f.admin, currentUser, "pg_advisory_xact_lock(69361")
			if err := blocker.Commit().Error; err != nil {
				t.Fatal(err)
			}
			result := <-frozen
			if result.err != nil {
				t.Fatal(result.err)
			}
			if err := <-preferenceDone; err != nil {
				t.Fatal(err)
			}
			if result.envelope.Input.CompilationInputs.Template.Customization != nil {
				t.Fatal("source observed preference inserted after its own freeze boundary")
			}
			request := prompt.CompileFrozenInputRequest{Key: result.envelope.Key(), CatalogContext: catalog}
			command, err := prompt.FrozenCompilationCommand(result.envelope, request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pgfence.New(f.creation).Admit(ctx, command); err != nil {
				t.Fatal(err)
			}
			compiled, err := f.store.CompileFrozenInput(ctx, request, command)
			if err != nil || strings.Contains(compiled.FinalPrompt, "首次个人偏好") || compiled.TemplateEvidence == nil || compiled.TemplateEvidence.CustomizationID != nil {
				t.Fatal("compilation reloaded today's newly inserted preference", compiled, err)
			}
		})
	}
}

func TestCreationFrozenCompilationPostgresFinalGuardAndOutboxRollback(t *testing.T) {
	for _, failure := range []string{"fenced", "archived-project", "outbox-invalid-json"} {
		t.Run(failure, func(t *testing.T) {
			f, catalog := frozenCompilationFixture(t, nil)
			envelope := f.freeze(t)
			request := prompt.CompileFrozenInputRequest{Key: envelope.Key(), CatalogContext: catalog}
			command, err := prompt.FrozenCompilationCommand(envelope, request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pgfence.New(f.creation).Admit(t.Context(), command); err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "fenced":
				authority := command.Authorities[0]
				if err := pgfence.New(f.creation).Install(t.Context(), writefence.Control{Authority: writefence.Authority{Issuer: authority.Issuer, Kind: authority.Kind, ID: authority.ID, Epoch: authority.Epoch + 1}, Blocked: true, CoordinationID: uuid.New()}); err != nil {
					t.Fatal(err)
				}
			case "archived-project":
				if err := f.creation.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, f.request.ProjectID).Error; err != nil {
					t.Fatal(err)
				}
			case "outbox-invalid-json":
				// Fault only the real compile Outbox INSERT parameter. PostgreSQL's
				// actual JSONB decoder aborts the business transaction; no fixture DDL.
				const callback = "test.compile.invalid-outbox-json"
				if err := f.creation.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
					if strings.Contains(tx.Statement.SQL.String(), "INSERT INTO infra.outbox") && len(tx.Statement.Vars) == 4 && tx.Statement.Vars[1] == "glowloom.creation.frozen_input.compiled" {
						tx.Statement.Vars[3] = "{"
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = f.creation.Callback().Raw().Remove(callback) })
			}
			_, compileErr := f.store.CompileFrozenInput(t.Context(), request, command)
			if compileErr == nil {
				t.Fatal("invalid final transaction compiled")
			}
			switch failure {
			case "fenced":
				if !errors.Is(compileErr, writefence.ErrFenced) {
					t.Fatal("final writer fence was not the rejecting guard", compileErr)
				}
			case "archived-project":
				if !errors.Is(compileErr, domain.ErrReferenceUnavailable) {
					t.Fatal("current project policy was not rechecked", compileErr)
				}
			case "outbox-invalid-json":
				var databaseErr *pgconn.PgError
				if !errors.As(compileErr, &databaseErr) || databaseErr.Code != "22P02" {
					t.Fatal("actual PostgreSQL JSONB insertion was not the failure boundary", compileErr)
				}
			}
			record, err := pgfence.New(f.creation).Load(t.Context(), command.ID)
			if err != nil || record.State != writefence.Admitted || len(record.Receipt) != 0 {
				t.Fatal("failure split final writer receipt from business transaction", record, err)
			}
			if _, err := f.store.ReadFrozenCompilation(t.Context(), request); !errors.Is(err, domain.ErrFrozenInputNotFound) {
				t.Fatal("failed transaction became immutable compilation", err)
			}
		})
	}
}
