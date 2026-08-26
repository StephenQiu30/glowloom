package script_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

type fakeStore struct {
	receipts          map[string]platformcommand.Receipt
	analyses          map[string]domain.Analysis
	currentRevisionID string
	lastProjectID     string
	lastProjectWrite  bool
}

func (store *fakeStore) WithinTransaction(_ context.Context, operation func(scriptapp.Repository) error) error {
	return operation(store)
}
func (store *fakeStore) ProjectWorkspace(_ context.Context, _ scriptapp.Actor, projectID string, write bool) (string, error) {
	store.lastProjectID = projectID
	store.lastProjectWrite = write
	return "00000000-0000-0000-0000-000000000002", nil
}
func (store *fakeStore) FindReceipt(_ context.Context, _, operation, key string) (platformcommand.Receipt, error) {
	receipt, ok := store.receipts[operation+":"+key]
	if !ok {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	return receipt, nil
}
func (store *fakeStore) CreateReceipt(_ context.Context, receipt platformcommand.Receipt) error {
	store.receipts[receipt.Operation+":"+receipt.IdempotencyKey] = receipt
	return nil
}
func (store *fakeStore) CreateAnalysis(_ context.Context, analysis domain.Analysis) error {
	store.analyses[analysis.Revision.ID] = analysis
	return nil
}
func (store *fakeStore) GetAnalysis(_ context.Context, revisionID string) (domain.Analysis, error) {
	analysis, ok := store.analyses[revisionID]
	if !ok {
		return domain.Analysis{}, scriptapp.ErrNotFound
	}
	return analysis, nil
}
func (store *fakeStore) GetCurrentAnalysis(_ context.Context, _ string) (domain.Analysis, error) {
	return store.GetAnalysis(context.Background(), store.currentRevisionID)
}
func (store *fakeStore) ListDocuments(context.Context, string, int, int) ([]domain.Document, int, error) {
	return nil, 0, nil
}

type fakeMedia struct{ workspaceID, mimeType, text string }

func (media fakeMedia) Read(context.Context, scriptapp.Actor, string) (scriptapp.MediaContent, []byte, error) {
	return scriptapp.MediaContent{WorkspaceID: media.workspaceID, MIMEType: media.mimeType}, []byte(media.text), nil
}

func TestImportReplaysReceiptAndRejectsChangedInput(t *testing.T) {
	store := &fakeStore{receipts: map[string]platformcommand.Receipt{}, analyses: map[string]domain.Analysis{}}
	sequence := 0
	service := scriptapp.NewService(store, fakeMedia{workspaceID: "00000000-0000-0000-0000-000000000002", mimeType: "text/markdown", text: "第一集\n内容\n第二集\n内容\n"}, scriptapp.Config{
		Now: func() time.Time { return time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) },
		NewID: func() string {
			sequence++
			return fmt.Sprintf("00000000-0000-0000-0000-%012d", sequence)
		},
	})
	mediaID := "00000000-0000-0000-0000-000000000003"
	command := scriptapp.ImportCommand{ProjectID: "00000000-0000-0000-0000-000000000001", InputType: "media", Title: "剧本.md", MediaVersionID: &mediaID, Language: "zh-CN", RightsDeclaration: "我确认拥有使用权", IdempotencyKey: "script-import-1"}
	actor := scriptapp.Actor{UserID: "00000000-0000-0000-0000-000000000004", TokenVersion: 1}

	first, err := service.Import(context.Background(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Import(context.Background(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision.ID != second.Revision.ID || len(store.analyses) != 1 {
		t.Fatalf("replay created a second revision: first=%q second=%q analyses=%d", first.Revision.ID, second.Revision.ID, len(store.analyses))
	}
	if first.Revision.AnalysisStatus != "deterministic" || len(first.Revision.Blocks) != 4 {
		t.Fatalf("analysis = %#v", first.Revision)
	}

	command.Title = "另一份剧本.md"
	_, err = service.Import(context.Background(), actor, command)
	var apiError *scriptapp.Error
	if !errors.As(err, &apiError) || apiError.Code != "resource_conflict" || apiError.Status != 409 {
		t.Fatalf("changed input error = %#v", err)
	}
}

func TestPreviewRejectsMediaFromAnotherWorkspace(t *testing.T) {
	store := &fakeStore{receipts: map[string]platformcommand.Receipt{}, analyses: map[string]domain.Analysis{}}
	service := scriptapp.NewService(store, fakeMedia{workspaceID: "00000000-0000-0000-0000-000000000099", mimeType: "text/markdown", text: "第一集\n内容"}, scriptapp.Config{})

	_, err := service.Preview(context.Background(), scriptapp.Actor{}, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000003")
	var apiError *scriptapp.Error
	if !errors.As(err, &apiError) || apiError.Code != "not_found" {
		t.Fatalf("cross-workspace preview error = %#v", err)
	}
}

func TestGetCurrentAnalysisRestoresLatestProjectScript(t *testing.T) {
	const projectID = "00000000-0000-0000-0000-000000000001"
	const revisionID = "00000000-0000-0000-0000-000000000011"
	analysis := domain.Analysis{
		Document: domain.Document{ProjectID: projectID, Title: "雾港倒计时.md"},
		Revision: domain.Revision{ID: revisionID, VersionNo: 1, AnalysisStatus: "deterministic"},
	}
	store := &fakeStore{
		receipts:          map[string]platformcommand.Receipt{},
		analyses:          map[string]domain.Analysis{revisionID: analysis},
		currentRevisionID: revisionID,
	}
	service := scriptapp.NewService(store, fakeMedia{}, scriptapp.Config{})

	current, err := service.GetCurrentAnalysis(
		context.Background(),
		scriptapp.Actor{UserID: "00000000-0000-0000-0000-000000000004", TokenVersion: 1},
		projectID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision.ID != revisionID || current.Document.Title != "雾港倒计时.md" {
		t.Fatalf("current analysis = %#v", current)
	}
	if store.lastProjectID != projectID || store.lastProjectWrite {
		t.Fatalf("authorization = project %q write=%t", store.lastProjectID, store.lastProjectWrite)
	}
}
