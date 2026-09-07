package gormdb_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	database "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	agenthttp "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/agenthttp"
	creationgorm "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/gormdb"
	creationhttp "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/httpapi"
	creation "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	"github.com/google/uuid"
)

type creationJourneyAuth struct{ userID string }

func (a creationJourneyAuth) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: a.userID, TokenVersion: 1}, nil
}

func TestCreationPublicCommandRecoversPeerReceiptAfterResponseLoss(t *testing.T) {
	db := creationDatabase(t)
	tx := db
	now := time.Now().UTC()
	ctx := context.Background()
	fixture := seedSourceAcceptanceProject(t, func(v any) error { return tx.Create(v).Error }, now, "合成原稿：门外有人敲门。")
	registerSourceAcceptanceFixtureCleanup(t, db, fixture, []string{"creation-source"})
	t.Cleanup(func() {
		ids := db.Model(&model.CreationRun{}).Select("id").Where("project_id = ?", fixture.projectID)
		if err := db.Where("run_id IN (?)", ids).Delete(&model.CreationCommandOutbox{}).Error; err != nil {
			t.Error(err)
		}
		if err := db.Where("project_id = ?", fixture.projectID).Delete(&model.CreationRun{}).Error; err != nil {
			t.Error(err)
		}
	})
	acceptCreationSource(t, tx, fixture)
	var accepted *domain.Acceptance
	posts := 0
	// The peer is a contract fixture, not a Python orchestration implementation.
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if accepted == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(accepted)
			return
		}
		posts++
		var input domain.Command
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		hash, err := creation.PayloadHash(input)
		if err != nil {
			t.Error(err)
			return
		}
		accepted = &domain.Acceptance{Schema: "creation-acceptance-production", CommandID: input.CommandID, RunID: input.RunID, PayloadHash: hash, FlowType: input.FlowType, WorkflowID: input.WorkflowID, ReceiptID: uuid.NewString(), AcceptedAt: now}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"response_was_interrupted":`))
	}))
	defer peer.Close()
	config := creation.Config{Endpoint: peer.URL, Now: func() time.Time { return now }, NewID: uuid.NewString}
	service := creation.NewService(creationgorm.New(tx), config)
	mux := http.NewServeMux()
	creationhttp.New(service, creationJourneyAuth{userID: fixture.userID.String()}).Register(mux)
	payload := `{"document_revision_id":"` + fixture.revisionID.String() + `","source_hash":"` + fixture.normalizedHash + `","idempotency_key":"http-journey"}`
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/projects/"+fixture.projectID.String()+"/creation-runs", strings.NewReader(payload)))
	if response.Code != 202 {
		t.Fatalf("create HTTP: %d %s", response.Code, response.Body.String())
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	client, err := agenthttp.New(strings.Repeat("x", 32), nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err = creation.NewDispatcher(creationgorm.New(tx), client, func() time.Time { return now }).DispatchOne(ctx); err != nil {
		t.Fatal(err)
	}
	unknown, err := service.Get(ctx, creation.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, created.Data.ID)
	if err != nil || unknown.Status != domain.Unknown {
		t.Fatalf("response loss: %v %s", err, unknown.Status)
	}
	now = now.Add(time.Minute)
	// Reopen an independent database pool; recovery cannot see an uncommitted transaction.
	reopened, err := database.Open(ctx, os.Getenv("LANVERSE_TEST_DATABASE_URL"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close(reopened) }()
	restored := creation.NewService(creationgorm.New(reopened), config)
	if err = creation.NewDispatcher(creationgorm.New(reopened), client, func() time.Time { return now }).DispatchOne(ctx); err != nil {
		t.Fatal(err)
	}
	read, err := restored.Get(ctx, creation.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, created.Data.ID)
	if err != nil || read.Status != domain.Accepted || read.Acceptance == nil || read.Acceptance.ReceiptID != accepted.ReceiptID || posts != 1 {
		t.Fatalf("recovery failed: status=%s posts=%d err=%v", read.Status, posts, err)
	}
}
