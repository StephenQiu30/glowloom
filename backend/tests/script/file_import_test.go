package script_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func TestScriptFileImportClosedBudgetAndPartialRetry(t *testing.T) {
	files := []uuid.UUID{uuid.New(), uuid.New()}
	if err := domain.ValidateImportFiles(files, true); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]uuid.UUID{nil, {uuid.Nil}, {files[0], files[0]}, make([]uuid.UUID, 201)} {
		if err := domain.ValidateImportFiles(bad, true); err == nil {
			t.Fatal("invalid file admission", bad)
		}
	}
	if err := domain.ValidateImportFiles(files, false); err == nil {
		t.Fatal("unconfirmed rights accepted")
	}
	if got := domain.ImportOutcome(2, 1); got != "partial" {
		t.Fatal("failed file hidden", got)
	}
	if got := domain.ImportOutcome(0, 2); got != "failed" {
		t.Fatal("empty version accepted", got)
	}
	if got := domain.ImportOutcome(2, 0); got != "succeeded" {
		t.Fatal(got)
	}
	if !domain.CanRetryImport("partial", false, false, false, 1) || domain.CanRetryImport("partial", true, false, false, 1) || domain.CanRetryImport("failed", false, true, false, 1) || domain.CanRetryImport("failed", false, false, true, 1) || domain.CanRetryImport("succeeded", false, false, false, 0) {
		t.Fatal("retry crossed an execution or object fence")
	}
}

func TestScriptFileImportWarningsAreClosedFormalFactsAndNilIsByteCompatible(t *testing.T) {
	for _, warnings := range [][]domain.SourceExtractionWarning{{{Code: "invented", Count: 1}}, {{Code: "embedded_media_omitted", Count: 0}}, {{Code: "embedded_media_omitted", Count: 200001}}, {{Code: "embedded_media_omitted", Count: 1}, {Code: "embedded_media_omitted", Count: 2}}} {
		if err := domain.ValidateSourceWarnings(warnings); err == nil {
			t.Fatal("invalid parser fact", warnings)
		}
	}
	if err := domain.ValidateSourceWarnings([]domain.SourceExtractionWarning{{Code: "embedded_media_omitted", Count: 1}, {Code: "table_layout_flattened", Count: 2}}); err != nil {
		t.Fatal(err)
	}
	nilJSON, err := json.Marshal(domain.SourceProvenance{})
	if err != nil || string(nilJSON) != "{}" {
		t.Fatal("legacy provenance bytes changed", string(nilJSON), err)
	}
	actor, input := sourceCommand()
	input.Sources[0].Provenance.Warnings = []domain.SourceExtractionWarning{{Code: "embedded_media_omitted", Count: 1}}
	store := &sourcePersistence{}
	objects := &sourceObjects{data: make(map[string][]byte)}
	if _, err := app.NewSourceService(store, objects, time.Now).Write(t.Context(), actor, input); err == nil || store.begins != 0 || objects.puts != 0 {
		t.Fatal("client invented provenance", err)
	}
}
