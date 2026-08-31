package script_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	scripthttp "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/httpapi"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

const (
	sourceHTTPProjectID  = "41000000-0000-4000-8000-000000000001"
	sourceHTTPRevisionID = "41000000-0000-4000-8000-000000000002"
)

func TestScriptSourceHTTPAcceptsAndQueriesExactSourceVersion(t *testing.T) {
	sources := &sourceHTTPService{accepted: acceptedHTTPSource()}
	mux := http.NewServeMux()
	scripthttp.New(sourceDocumentHTTPService{}, sources, sourceHTTPAuthenticator{}).Register(mux)

	accepted := httptest.NewRecorder()
	mux.ServeHTTP(accepted, httptest.NewRequest(
		http.MethodPost,
		"/api/projects/"+sourceHTTPProjectID+"/script-sources",
		strings.NewReader(`{"document_revision_id":"`+sourceHTTPRevisionID+`","expected_head_revision":0,"idempotency_key":"accept-source"}`),
	))
	if accepted.Code != http.StatusCreated ||
		!strings.Contains(accepted.Body.String(), `"codepoint_index_rule":"unicode-code-point"`) ||
		sources.command.ProjectID != sourceHTTPProjectID ||
		sources.command.DocumentRevisionID != sourceHTTPRevisionID ||
		sources.actor.TokenVersion != 3 {
		t.Fatalf("accept response=%d %s actor=%#v command=%#v", accepted.Code, accepted.Body.String(), sources.actor, sources.command)
	}

	queried := httptest.NewRecorder()
	mux.ServeHTTP(queried, httptest.NewRequest(
		http.MethodGet,
		"/api/projects/"+sourceHTTPProjectID+"/script-sources/"+sourceHTTPRevisionID,
		nil,
	))
	if queried.Code != http.StatusOK || sources.queriedProjectID != sourceHTTPProjectID ||
		sources.queriedRevisionID != sourceHTTPRevisionID ||
		!strings.Contains(queried.Body.String(), `"version_id":"`+sourceHTTPRevisionID+`"`) {
		t.Fatalf("query response=%d %s service=%#v", queried.Code, queried.Body.String(), sources)
	}
}

func TestScriptSourceHTTPRejectsUnknownAcceptanceFields(t *testing.T) {
	sources := &sourceHTTPService{}
	mux := http.NewServeMux()
	scripthttp.New(sourceDocumentHTTPService{}, sources, sourceHTTPAuthenticator{}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/projects/"+sourceHTTPProjectID+"/script-sources",
		strings.NewReader(`{"document_revision_id":"`+sourceHTTPRevisionID+`","expected_head_revision":0,"idempotency_key":"accept-source","latest":true}`),
	))
	if response.Code != http.StatusUnprocessableEntity || sources.command.ProjectID != "" {
		t.Fatalf("response=%d %s command=%#v", response.Code, response.Body.String(), sources.command)
	}
}

func TestScriptSourceHTTPRequiresExpectedHeadRevision(t *testing.T) {
	sources := &sourceHTTPService{}
	mux := http.NewServeMux()
	scripthttp.New(sourceDocumentHTTPService{}, sources, sourceHTTPAuthenticator{}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/projects/"+sourceHTTPProjectID+"/script-sources",
		strings.NewReader(`{"document_revision_id":"`+sourceHTTPRevisionID+`","idempotency_key":"accept-source"}`),
	))
	if response.Code != http.StatusUnprocessableEntity || sources.command.ProjectID != "" {
		t.Fatalf("response=%d %s command=%#v", response.Code, response.Body.String(), sources.command)
	}
}

type sourceHTTPAuthenticator struct{}

func (sourceHTTPAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: "41000000-0000-4000-8000-000000000003", TokenVersion: 3}, nil
}

type sourceHTTPService struct {
	accepted                            scriptdomain.AcceptedSource
	actor                               scriptapp.Actor
	command                             scriptapp.AcceptSourceCommand
	queriedProjectID, queriedRevisionID string
}

func (service *sourceHTTPService) Accept(
	_ context.Context,
	actor scriptapp.Actor,
	command scriptapp.AcceptSourceCommand,
) (scriptdomain.AcceptedSource, error) {
	service.actor, service.command = actor, command
	return service.accepted, nil
}

func (service *sourceHTTPService) GetExact(
	_ context.Context,
	_ scriptapp.Actor,
	projectID, revisionID string,
) (scriptdomain.AcceptedSource, error) {
	service.queriedProjectID, service.queriedRevisionID = projectID, revisionID
	return service.accepted, nil
}

type sourceDocumentHTTPService struct{}

func (sourceDocumentHTTPService) Preview(context.Context, scriptapp.Actor, string, string) (scriptapp.Preview, error) {
	return scriptapp.Preview{}, errors.New("not implemented")
}
func (sourceDocumentHTTPService) Import(context.Context, scriptapp.Actor, scriptapp.ImportCommand) (scriptdomain.Analysis, error) {
	return scriptdomain.Analysis{}, errors.New("not implemented")
}
func (sourceDocumentHTTPService) GetRevision(context.Context, scriptapp.Actor, string) (scriptdomain.Analysis, error) {
	return scriptdomain.Analysis{}, errors.New("not implemented")
}
func (sourceDocumentHTTPService) GetCurrentAnalysis(context.Context, scriptapp.Actor, string) (scriptdomain.Analysis, error) {
	return scriptdomain.Analysis{}, errors.New("not implemented")
}
func (sourceDocumentHTTPService) ListDocuments(context.Context, scriptapp.Actor, string, int, int) ([]scriptdomain.Document, int, error) {
	return nil, 0, errors.New("not implemented")
}

func acceptedHTTPSource() scriptdomain.AcceptedSource {
	return scriptdomain.AcceptedSource{
		Identity: scriptdomain.SourceVersionIdentity{
			OwnerKind: "production/script", LogicalID: "41000000-0000-4000-8000-000000000004",
			VersionID: sourceHTTPRevisionID, Revision: 1, ContentHash: strings.Repeat("a", 64),
			CreatedAt: time.Unix(10, 0).UTC(),
		},
		SpanIndexID: "41000000-0000-4000-8000-000000000005", SpanIndexHash: strings.Repeat("b", 64),
		CodepointCount: 29, UTF8ByteCount: 75, NewlineNormalization: "lf", CodepointIndexRule: "unicode-code-point",
		HeadRevision: 1, HeadHash: strings.Repeat("c", 64), CollectionRootHash: strings.Repeat("d", 64),
		CollectionReceiptID: "41000000-0000-4000-8000-000000000006",
		CommandReceiptID:    "41000000-0000-4000-8000-000000000007",
	}
}
