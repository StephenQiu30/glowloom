package canvas_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	canvaswire "github.com/StephenQiu30/glowloom-server/backend/internal/app/canvas"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
)

func TestCanvasInformationScopedLedgerPaginationAndMetadata(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	store := canvasStore(runtime).WithCanvasUsage(canvaswire.NewInformationUsage)
	service := application.NewService(store)
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Information"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Information(t.Context(), actor, doc.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerName != "Canvas test" || got.ID != doc.ID || got.ProjectID != project || got.CreatedAt.IsZero() || got.UpdatedAt.Before(got.CreatedAt) || got.Usage.Currency != "CNY" || got.Usage.SettledMicros != "0" || got.Usage.TaskCount != "0" || len(got.Usage.Entries) != 0 {
		t.Fatalf("metadata or empty facts: %+v", got)
	}
	seed := func(canvas uuid.UUID, confirmed bool, charge int64) uuid.UUID {
		t.Helper()
		id := uuid.New()
		source, _ := json.Marshal(map[string]any{"canvas_id": canvas.String(), "node_id": uuid.NewString(), "revision": 1})
		var at *time.Time
		if confirmed {
			now := time.Now().UTC()
			at = &now
		}
		if err := owner.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,model_profile_version_id,input_hash,origin,status,region,confirmed_at,source_context) VALUES(?,?,'free','image.generate','text2image',?,'information-fixture','canvas','completed','domestic',?,?::jsonb)`, id, project, uuid.New(), at, string(source)).Error; err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"settle", "reserve", "release", "provider_overage"} {
			if err := owner.Exec(`INSERT INTO billing.ledger_entry(id,project_id,entry_type,amount_micros,operation_id,model_key) VALUES(?,?,?,?,?,'information-model')`, uuid.New(), project, kind, charge, id).Error; err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	for range 21 {
		seed(doc.ID, true, 1000001)
	}
	seed(doc.ID, false, 999999999)
	seed(uuid.New(), true, 999999999)
	got, err = service.Information(t.Context(), actor, doc.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Usage.SettledMicros != "21000021" || got.Usage.TaskCount != "21" || len(got.Usage.Entries) != 20 || got.Usage.NextPage == nil || *got.Usage.NextPage != 2 {
		t.Fatalf("scoped charges: %+v", got.Usage)
	}
	last, err := service.Information(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(last.Usage.Entries) != 1 || last.Usage.NextPage != nil || last.Usage.SettledMicros != got.Usage.SettledMicros {
		t.Fatalf("last page: %+v", last.Usage)
	}
	for _, entry := range got.Usage.Entries {
		if entry.ID == last.Usage.Entries[0].ID {
			t.Fatal("overlapping ledger pages")
		}
	}
	stranger, _ := seedCanvasActor(t, owner)
	if _, err = service.Information(t.Context(), stranger, doc.ID, 1); err == nil {
		t.Fatal("cross-scope read allowed")
	}
	if _, err = service.Information(t.Context(), actor, doc.ID, 0); err == nil {
		t.Fatal("invalid page accepted")
	}
	if _, err = service.Information(t.Context(), actor, uuid.New(), 1); err == nil {
		t.Fatal("missing canvas read allowed")
	}
	if err = owner.Exec(`UPDATE canvas.document SET is_delete=true WHERE id=?`, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Information(t.Context(), actor, doc.ID, 1); err == nil {
		t.Fatal("deleted canvas read allowed")
	}
}
