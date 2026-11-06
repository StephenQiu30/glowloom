package catalog_test

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/providerprobe"
	catalogflow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/task"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestOnboardingRequiresFormalEvidenceForEveryModelAndMode(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires disposable catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	org := insertModelCatalogOrganization(t.Context(), t, conn.DB)
	actor := insertModelCatalogActor(t.Context(), t, conn.DB, org)
	require.NoError(t, conn.DB.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	project := insertModelCatalogProject(t.Context(), t, conn.DB, actor, false)
	require.NoError(t, conn.DB.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros) VALUES(?,?,0)`, uuid.New(), project).Error)
	store := app.NewCatalogStore(conn.DB)
	presets := catalogapp.NewProviderPresets(store)
	items, err := presets.List(t.Context(), actor)
	require.NoError(t, err)
	var preset domain.Preset
	for _, item := range items {
		if item.Slug == "volcengine-ark" {
			preset = item
		}
	}
	input := catalogapp.ConnectPresetInput{RequestID: uuid.New(), VersionID: preset.VersionID, CatalogVersion: preset.ModelCatalog.Version, CandidateKeys: []string{"ark-chat", "ark-image", "ark-video"}, DirectUse: true}
	connection, err := presets.Connect(t.Context(), actor, preset.ID, input)
	require.NoError(t, err)
	require.Len(t, connection.ModelIDs, 3)
	seedReviewedOnboardingPrices(t, conn.DB, actor, connection, false)
	onboarding := catalogapp.NewProviderOnboarding(store)
	state, err := onboarding.Read(t.Context(), actor, connection.ProviderID)
	require.NoError(t, err)
	require.Len(t, state.Models, 3)
	require.Equal(t, "disabled", state.Status)
	_, err = onboarding.Activate(t.Context(), actor, connection.ProviderID, catalogapp.ActivateOnboardingInput{RequestID: uuid.New(), CheckID: uuid.New(), ExpectedRevision: state.Revision})
	require.Error(t, err)
	private, public := testCredentialKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	unsealer, err := credentialseal.NewUnsealer("onboarding-fixture", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	require.NoError(t, err)
	sealer, err := credentialseal.NewSealer("onboarding-fixture", public)
	require.NoError(t, err)
	credential := uuid.New()
	ciphertext, err := sealer.Seal(connection.ProviderID, credential, json.RawMessage(`{"api_key":"synthetic-onboarding-key"}`))
	require.NoError(t, err)
	require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,ciphertext,key_id,last4) VALUES(?,?,'fixture',?,'onboarding-fixture','-key')`, credential, connection.ProviderID, ciphertext).Error)
	command := catalogapp.NewRequestCredentialTestCommand(store)
	requestID := uuid.NewString()
	accepted, err := command.ExecuteModel(t.Context(), actor, connection.ProviderID, credential, connection.ModelIDs[0], requestID)
	require.NoError(t, err)
	again, err := command.ExecuteModel(t.Context(), actor, connection.ProviderID, credential, connection.ModelIDs[0], requestID)
	require.NoError(t, err)
	require.Equal(t, accepted, again)
	_, err = command.ExecuteModel(t.Context(), actor, connection.ProviderID, credential, connection.ModelIDs[1], requestID)
	require.ErrorIs(t, err, catalogapp.ErrCredentialTestKeyReused)
	request := catalogapp.CredentialTestRequest{TestID: accepted.TestID, ProviderID: connection.ProviderID, CredentialID: credential, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: requestID}
	service := catalogapp.NewCredentialTestService(store)
	_, _, err = service.Load(t.Context(), request)
	require.NoError(t, err)
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "/api/v3/chat/completions", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "doubao-seed-2-1-pro-260915", body["model"])
		return providerResponse(200, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`), nil
	})})
	worker := providerprobe.NewScopedActivities(conn.DB, unsealer, client, store)
	workerInput := catalogflow.AgentTestInput{TestID: accepted.TestID, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: requestID, ProviderID: connection.ProviderID, AdapterKey: "volcengine_ark", Credential: catalogflow.AgentCredential{ID: credential, KeyID: "onboarding-fixture", Ciphertext: base64.StdEncoding.EncodeToString(ciphertext)}}
	evidence, err := worker.TestCredential(t.Context(), workerInput)
	require.NoError(t, err)
	require.Equal(t, "completed", evidence.ModelResult)
	repeated, err := worker.TestCredential(t.Context(), workerInput)
	require.NoError(t, err)
	require.Equal(t, "unknown", repeated.ModelResult)
	require.Equal(t, 1, calls)
	require.NoError(t, service.Record(t.Context(), catalogapp.CredentialTestObservation{CredentialTestRequest: request, Result: evidence.Result, TestedAt: time.Now().UTC(), Last4: "-key", AuthResult: evidence.AuthResult, AdapterResult: evidence.AdapterResult, DiscoveryResult: evidence.DiscoveryResult, ModelResult: evidence.ModelResult}))
	state, err = onboarding.Read(t.Context(), actor, connection.ProviderID)
	require.NoError(t, err)
	require.Equal(t, "completed", state.Check.ModelResult)
	activation := catalogapp.ActivateOnboardingInput{RequestID: uuid.New(), CheckID: accepted.CheckID, ExpectedRevision: state.Revision}
	// A replaced credential cannot reuse the old successful check.
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider_credential SET status='disabled' WHERE id=?`, credential).Error)
	_, err = onboarding.Activate(t.Context(), actor, connection.ProviderID, activation)
	require.Error(t, err)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider_credential SET status='active' WHERE id=?`, credential).Error)
	_, err = onboarding.Activate(t.Context(), actor, connection.ProviderID, activation)
	require.ErrorIs(t, err, catalogapp.ErrReleaseUnverified)
	require.Len(t, state.ReleaseReviews, 3)
	for _, review := range state.ReleaseReviews[:2] {
		for _, mode := range review.RequiredModes {
			seedOnboardingModeEvidence(t, conn.DB, project, credential, review, mode)
		}
	}
	last := state.ReleaseReviews[2]
	require.NotEmpty(t, last.RequiredModes)
	for _, mode := range last.RequiredModes[1:] {
		seedOnboardingModeEvidence(t, conn.DB, project, credential, last, mode)
	}
	_, err = onboarding.Activate(t.Context(), actor, connection.ProviderID, activation)
	require.ErrorIs(t, err, catalogapp.ErrReleaseUnverified)
	seedOnboardingModeEvidence(t, conn.DB, project, credential, last, last.RequiredModes[0])
	mismatched := activation
	mismatched.ReleaseIDs = []uuid.UUID{last.ID}
	_, err = onboarding.Activate(t.Context(), actor, connection.ProviderID, mismatched)
	require.ErrorIs(t, err, catalogapp.ErrPresetConflict)
	for _, review := range state.ReleaseReviews {
		activation.ReleaseIDs = append(activation.ReleaseIDs, review.ID)
	}

	enabled, err := onboarding.Activate(t.Context(), actor, connection.ProviderID, activation)
	require.NoError(t, err)
	require.Equal(t, "active", enabled.Status)
	replay, err := onboarding.Activate(t.Context(), actor, connection.ProviderID, activation)
	require.NoError(t, err)
	firstJSON, err := json.Marshal(enabled)
	require.NoError(t, err)
	replayJSON, err := json.Marshal(replay)
	require.NoError(t, err)
	require.JSONEq(t, string(firstJSON), string(replayJSON))
	var evidenceCount int
	require.NoError(t, conn.DB.Raw(`SELECT count(*) FROM catalog.model_validation validation JOIN catalog.model_release release ON release.id=validation.release_id WHERE release.model_id IN ?`, connection.ModelIDs).Scan(&evidenceCount).Error)
	require.GreaterOrEqual(t, evidenceCount, 3)
	// The shared catalog may exceed one page. Resolve this test's three
	// connected model keys directly rather than relying on page order.
	var keys []string
	require.NoError(t, conn.DB.Raw(`SELECT model_key FROM catalog.model_profile WHERE id IN ? ORDER BY model_key`, connection.ModelIDs).Scan(&keys).Error)
	require.Len(t, keys, 3)
	found := 0
	for _, key := range keys {
		catalog, err := store.ListModelsForProject(t.Context(), actor, catalogapp.ListModelsInput{ProjectID: project, ModelKey: key, Limit: 1})
		require.NoError(t, err)
		require.Len(t, catalog.Models, 1)
		model := catalog.Models[0]
		require.Equal(t, connection.ProviderID, model.ProviderID)
		found++
		require.Equal(t, domain.ModelActive, model.Status)
		require.NotNil(t, model.CurrentVersion)
		require.NotNil(t, model.CurrentPrice)
		mode := "text_to_text"
		if model.Capability == "image.generate" {
			mode = "text2image"
		}
		if model.Capability == "video.generate" {
			mode = "text2video"
		}
		quote, err := operationwire.ProvideOperationStore(conn.DB).CreateFreeQuote(t.Context(), actor, operationapp.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{ModelKey: model.Key, Capability: model.Capability, Mode: mode, Prompt: "A blue circle.", Params: json.RawMessage(`{}`), OutputCount: 1}})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, quote.OperationID)
		require.Zero(t, quote.QuoteMicros)
	}
	require.Equal(t, 3, found)
	// Restoring the connection leaves it disabled until the current release
	// is explicitly re-enabled using current evidence for all of its modes.
	updateProvider := catalogapp.NewUpdateProviderCommand(store, time.Now)
	archivedStatus := domain.ProviderArchived
	archivedProvider, err := updateProvider.Execute(t.Context(), actor, catalogapp.UpdateProviderInput{
		ProviderID: connection.ProviderID, ExpectedRevision: enabled.Revision,
		Status: &archivedStatus, RequestID: uuid.NewString(),
	})
	require.NoError(t, err)
	visible, err := store.ListModelsForProject(t.Context(), actor, catalogapp.ListModelsInput{ProjectID: project, ModelKey: keys[0], Limit: 1})
	require.NoError(t, err)
	require.Empty(t, visible.Models)
	model, err := store.FindModelForAdmin(t.Context(), actor, connection.ModelIDs[0])
	require.NoError(t, err)
	releases, err := store.ListModelReleases(t.Context(), actor, model.ID)
	require.NoError(t, err)
	require.Len(t, releases, 1)
	release := releases[0]
	activate := catalogapp.ActivateReleaseInput{
		RequestID: uuid.New(), ExpectedRevision: release.Revision, ExpectedModelRevision: model.Revision,
		ExpectedActiveReleaseID: &release.ID, EnableProvider: true,
	}
	_, err = store.ActivateModelRelease(t.Context(), actor, release.ID, activate)
	require.ErrorIs(t, err, catalogapp.ErrProviderUnavailable)
	disabledStatus := domain.ProviderDisabled
	restored, err := updateProvider.Execute(t.Context(), actor, catalogapp.UpdateProviderInput{
		ProviderID: connection.ProviderID, ExpectedRevision: archivedProvider.Revision,
		Status: &disabledStatus, RequestID: uuid.NewString(),
	})
	require.NoError(t, err)
	require.Equal(t, disabledStatus, restored.Status)
	require.NoError(t, conn.DB.Exec("UPDATE catalog.model_release SET onboarding_check_id=? WHERE id=?", accepted.CheckID, release.ID).Error)
	require.NoError(t, conn.DB.Exec("UPDATE catalog.model_validation SET status='failed' WHERE release_id=? AND mode=?", release.ID, release.RequiredModes[0]).Error)
	reviewed, err := onboarding.Read(t.Context(), actor, connection.ProviderID)
	require.NoError(t, err)
	require.Len(t, reviewed.ReleaseReviews, 1)
	require.Equal(t, &accepted.CheckID, reviewed.ReleaseReviews[0].OnboardingCheckID)
	require.NotContains(t, reviewed.ReleaseReviews[0].ValidatedModes, release.RequiredModes[0])
	activeStatus := domain.ProviderActive
	_, err = updateProvider.Execute(t.Context(), actor, catalogapp.UpdateProviderInput{
		ProviderID: connection.ProviderID, ExpectedRevision: restored.Revision,
		Status: &activeStatus, RequestID: uuid.NewString(),
	})
	require.ErrorIs(t, err, catalogapp.ErrReleaseUnverified)

	_, err = store.ActivateModelRelease(t.Context(), actor, release.ID, activate)
	require.ErrorIs(t, err, catalogapp.ErrReleaseUnverified)
	require.NoError(t, conn.DB.Exec("UPDATE catalog.model_validation SET status='succeeded' WHERE release_id=?", release.ID).Error)
	_, err = store.ActivateModelRelease(t.Context(), actor, release.ID, activate)
	require.NoError(t, err)

	// Management can verify an active model again without reactivating releases.
	recheck, err := command.ExecuteModel(t.Context(), actor, connection.ProviderID, credential, connection.ModelIDs[0], uuid.NewString())
	require.NoError(t, err)
	require.NotEqual(t, accepted.CheckID, recheck.CheckID)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider_credential SET status='disabled' WHERE id=?`, credential).Error)
	_, err = command.ExecuteModel(t.Context(), actor, connection.ProviderID, credential, connection.ModelIDs[0], uuid.NewString())
	require.Error(t, err)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider_credential SET status='active' WHERE id=?`, credential).Error)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.model_release SET status='retired' WHERE model_id=?`, connection.ModelIDs[0]).Error)
	_, err = command.ExecuteModel(t.Context(), actor, connection.ProviderID, credential, connection.ModelIDs[0], uuid.NewString())
	require.Error(t, err)
	actor.PlatformRole = "creator"
	_, err = onboarding.Read(t.Context(), actor, connection.ProviderID)
	require.Error(t, err)
}
