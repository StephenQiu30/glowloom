package bible_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	biblehttp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	"github.com/google/uuid"
)

type structureIdentityHTTPQuery struct {
	result app.StructureIdentitySnapshot
}

func (query structureIdentityHTTPQuery) GetCurrent(
	context.Context,
	app.Actor,
	string,
) (app.StructureIdentitySnapshot, error) {
	return query.result, nil
}

type structureIdentityQueryStore struct {
	value            domain.StructureIdentitySetVersion
	receipt          domain.StructureIdentityCollectionReceipt
	commandReceiptID string
	err              error
	calls            int
}

func (store *structureIdentityQueryStore) ReadCurrentStructureIdentity(
	context.Context,
	string,
	string,
) (domain.StructureIdentitySetVersion, domain.StructureIdentityCollectionReceipt, string, error) {
	store.calls++
	return store.value, store.receipt, store.commandReceiptID, store.err
}

func (store *structureIdentityQueryStore) ReadExactStructureIdentity(
	_ context.Context,
	_, _, versionID string,
) (domain.StructureIdentitySetVersion, error) {
	store.calls++
	if store.err != nil {
		return domain.StructureIdentitySetVersion{}, store.err
	}
	if store.value.ID != versionID {
		return domain.StructureIdentitySetVersion{}, app.ErrNotFound
	}
	return store.value, nil
}

func TestStructureIdentityQueryReadsOnlyRequestedFormalVersion(t *testing.T) {
	workspaceID, projectID, versionID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := &structureIdentityQueryStore{value: domain.StructureIdentitySetVersion{
		SchemaVersion: domain.StructureIdentitySetSchemaVersion,
		ID:            versionID, WorkspaceID: workspaceID, ProjectID: projectID, Version: 3,
		ContentHash: structureIdentityHash("formal-version"), CreatedAt: time.Now().UTC(),
	}}
	query := app.NewStructureIdentityQuery(
		store,
		textQueryProject{value: projectdomain.Project{ID: projectID, WorkspaceID: workspaceID}},
	)
	got, err := query.GetExact(context.Background(), app.Actor{}, projectID, versionID)
	if err != nil || got.ID != versionID || store.calls != 1 {
		t.Fatalf("exact Structure Identity=%#v calls=%d err=%v", got, store.calls, err)
	}
	if _, err = query.GetExact(context.Background(), app.Actor{}, projectID, uuid.NewString()); err == nil {
		t.Fatal("missing exact Structure Identity version reported as success")
	}
}

func TestStructureIdentityQueryRequiresCurrentAccessAndExactReceipt(t *testing.T) {
	workspaceID, projectID, versionID, decisionID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	value := domain.StructureIdentitySetVersion{
		SchemaVersion: domain.StructureIdentitySetSchemaVersion,
		ID:            versionID,
		WorkspaceID:   workspaceID,
		ProjectID:     projectID,
		Version:       1,
		ContentHash:   structureIdentityHash("formal-version"),
		CreatedAt:     time.Now().UTC(),
	}
	receipt := domain.StructureIdentityCollectionReceipt{
		ID:                 uuid.NewString(),
		CheckpointKey:      domain.StructureIdentityCheckpointKey,
		CollectionFamily:   domain.StructureIdentityCollectionFamily,
		VersionID:          versionID,
		VersionContentHash: value.ContentHash,
		ReviewDecisionID:   decisionID,
		CollectionRootHash: structureIdentityHash("collection-root"),
		ReceiptContentHash: structureIdentityHash("receipt"),
	}
	commandReceiptID := uuid.NewString()
	store := &structureIdentityQueryStore{value: value, receipt: receipt, commandReceiptID: commandReceiptID}
	projects := textQueryProject{value: projectdomain.Project{ID: projectID, WorkspaceID: workspaceID}}
	query := app.NewStructureIdentityQuery(store, projects)

	got, err := query.GetCurrent(context.Background(), app.Actor{}, projectID)
	if err != nil || got.Version.ID != versionID || got.Receipt.ID != receipt.ID || got.CommandReceiptID != commandReceiptID {
		t.Fatalf("query=%+v err=%v", got, err)
	}

	for _, mode := range []string{"scope", "version", "receipt-version", "receipt-hash", "checkpoint", "command-receipt"} {
		t.Run(mode, func(t *testing.T) {
			store.value, store.receipt = value, receipt
			switch mode {
			case "scope":
				store.value.ProjectID = uuid.NewString()
			case "version":
				store.value.Version = 0
			case "receipt-version":
				store.receipt.VersionID = uuid.NewString()
			case "receipt-hash":
				store.receipt.VersionContentHash = structureIdentityHash("other-version")
			case "checkpoint":
				store.receipt.CheckpointKey = "other_checkpoint"
			case "command-receipt":
				store.commandReceiptID = "not-a-receipt"
			}
			if _, queryErr := query.GetCurrent(context.Background(), app.Actor{}, projectID); queryErr == nil {
				t.Fatal("drifted formal Structure Identity result returned")
			}
		})
	}

	denied := errors.New("current permission revoked")
	projects.err = denied
	store.calls = 0
	query = app.NewStructureIdentityQuery(store, projects)
	if _, err = query.GetCurrent(context.Background(), app.Actor{}, projectID); !errors.Is(err, denied) || store.calls != 0 {
		t.Fatal("read Structure Identity result before current authorization")
	}

	projects.err = nil
	store.err = app.ErrNotFound
	query = app.NewStructureIdentityQuery(store, projects)
	if _, err = query.GetCurrent(context.Background(), app.Actor{}, projectID); err == nil {
		t.Fatal("missing current Structure Identity result reported as success")
	}

	if _, err = query.GetCurrent(context.Background(), app.Actor{}, "not-an-id"); err == nil || store.calls != 1 {
		t.Fatal("invalid Project identity reached the formal version reader")
	}
}

func TestStructureIdentityHTTPReturnsFrozenFormalResult(t *testing.T) {
	projectID, versionID := uuid.NewString(), uuid.NewString()
	result := app.StructureIdentitySnapshot{
		Version: domain.StructureIdentitySetVersion{
			SchemaVersion: domain.StructureIdentitySetSchemaVersion,
			ID:            versionID, ProjectID: projectID, WorkspaceID: uuid.NewString(), Version: 1,
			ContentHash: structureIdentityHash("formal-version"), CreatedAt: time.Now().UTC(),
		},
		Receipt: domain.StructureIdentityCollectionReceipt{
			ID: uuid.NewString(), CheckpointKey: domain.StructureIdentityCheckpointKey,
			CollectionFamily: domain.StructureIdentityCollectionFamily, VersionID: versionID,
			VersionContentHash: structureIdentityHash("formal-version"), ReviewDecisionID: uuid.NewString(),
			CollectionRootHash: structureIdentityHash("root"), ReceiptContentHash: structureIdentityHash("receipt"),
		},
		CommandReceiptID: uuid.NewString(),
	}
	mux := http.NewServeMux()
	biblehttp.NewStructureIdentityHandler(
		structureIdentityHTTPQuery{result: result},
		bibleHTTPAuthenticator{},
	).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/projects/"+projectID+"/structure-identity",
		nil,
	))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var payload struct {
		Data app.StructureIdentitySnapshot `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Version.ID != versionID || payload.Data.Receipt.VersionID != versionID {
		t.Fatalf("response=%+v", payload.Data)
	}

	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(
		http.MethodGet,
		"/api/projects/"+projectID+"/structure-identity?latest=true",
		nil,
	))
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("query parameter status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}
