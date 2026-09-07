package storyboard_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	api "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
)

type intentAuth struct {
	actor app.Actor
	err   error
}

func (a intentAuth) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: a.actor.UserID, TokenVersion: a.actor.TokenVersion}, a.err
}
func TestTextIntentHTTPAcceptAndRecover(t *testing.T) {
	service, store, actor, cmd := intentServiceFixture()
	mux := http.NewServeMux()
	api.NewIntentHandler(service, intentAuth{actor: actor}).Register(mux)
	body := `{"workspace_id":"` + cmd.WorkspaceID + `","candidate_revision_id":"` + cmd.CandidateRevisionID + `","candidate_revision_hash":"` + cmd.CandidateRevisionHash + `","expected_candidate_revision":1,"review_decision_id":"` + cmd.ReviewDecisionID + `","idempotency_key":"` + cmd.IdempotencyKey + `"}`
	postURL := "/api/projects/" + cmd.ProjectID + "/storyboard-intent-acceptances"
	for _, suffix := range []string{"garbage", ` {}`} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, postURL, strings.NewReader(body+suffix)))
		if recorder.Code != 422 || store.saves != 0 {
			t.Fatalf("invalid JSON accepted: %d", recorder.Code)
		}
	}
	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, postURL, strings.NewReader(body)))
		if recorder.Code != 200 {
			t.Fatalf("accept: %d %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Data struct {
				Approved struct {
					ContentHash string `json:"content_hash"`
				}
				Receipt struct{ ID string }
			}
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Data.Approved.ContentHash) != 64 || response.Data.Receipt.ID != store.receipt.ID {
			t.Fatal("missing stable receipt")
		}
	}
	for _, path := range []string{"/api/storyboard-draft-sets/" + store.source.Set.ID, "/api/storyboard-draft-sets/" + store.source.Set.ID + "/approved-intents"} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), `"status":"intent_frozen"`) {
			t.Fatalf("recover: %d %s", recorder.Code, recorder.Body.String())
		}
	}
	if store.saves != 1 || store.receipts != 1 {
		t.Fatal("repeated acceptance wrote twice")
	}
	store.denied = true
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/storyboard-draft-sets/"+store.source.Set.ID+"/approved-intents", nil))
	if recorder.Code != 404 {
		t.Fatalf("denied recovery: %d", recorder.Code)
	}
}
func TestTextIntentHTTPRequiresAuthentication(t *testing.T) {
	service, store, actor, _ := intentServiceFixture()
	mux := http.NewServeMux()
	api.NewIntentHandler(service, intentAuth{actor: actor, err: errors.New("expired")}).Register(mux)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/storyboard-draft-sets/"+store.source.Set.ID, nil))
	if recorder.Code != 401 {
		t.Fatalf("unauthenticated: %d", recorder.Code)
	}
}
