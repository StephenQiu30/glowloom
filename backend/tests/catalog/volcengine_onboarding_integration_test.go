package catalog_test

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/providerprobe"
	catalogflow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/workflow"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestVolcengineWholeCatalogAndSpeechOnboardingUseRealStores(t *testing.T) {
	for _, slug := range []string{"volcengine-ark", "volcengine-speech"} {
		t.Run(slug, func(t *testing.T) {
			ctx, database := modelCatalogDB(t)
			org := insertModelCatalogOrganization(ctx, t, database)
			actor := insertModelCatalogActor(ctx, t, database, org)
			require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
			actor = identityfixture.Session(t, database, actor)
			project := insertModelCatalogProject(ctx, t, database, actor, false)
			require.NoError(t, database.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros) VALUES(?,?,0)`, uuid.New(), project).Error)
			store := app.NewCatalogStore(database)
			presets := catalogapp.NewProviderPresets(store)
			list, err := presets.List(ctx, actor)
			require.NoError(t, err)
			var preset domain.Preset
			for _, p := range list {
				if p.Slug == slug {
					preset = p
				}
			}
			require.NotEqual(t, uuid.Nil, preset.ID)
			keys := []string{}
			for _, m := range preset.ModelCatalog.Models {
				keys = append(keys, m.Key)
			}
			if slug == "volcengine-ark" {
				require.Greater(t, len(keys), 16)
			}
			connected, err := presets.Connect(ctx, actor, preset.ID, catalogapp.ConnectPresetInput{RequestID: uuid.New(), VersionID: preset.VersionID, CatalogVersion: preset.ModelCatalog.Version, CandidateKeys: keys, DirectUse: true})
			require.NoError(t, err)
			onboarding := catalogapp.NewProviderOnboarding(store)
			state, err := onboarding.Read(ctx, actor, connected.ProviderID)
			require.NoError(t, err)
			require.Len(t, state.Models, len(keys))
			require.Equal(t, "disabled", state.Status)
			private, public := testCredentialKey(t)
			der, err := x509.MarshalPKCS8PrivateKey(private)
			require.NoError(t, err)
			unsealer, err := credentialseal.NewUnsealer("volc-fixture", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
			require.NoError(t, err)
			sealer, err := credentialseal.NewSealer("volc-fixture", public)
			require.NoError(t, err)
			credential := uuid.New()
			ciphertext, err := sealer.Seal(connected.ProviderID, credential, json.RawMessage(`{"api_key":"synthetic-volc-key"}`))
			require.NoError(t, err)
			require.NoError(t, database.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,ciphertext,key_id,last4) VALUES(?,?,'fixture',?,'volc-fixture','-key')`, credential, connected.ProviderID, ciphertext).Error)
			model := state.Models[0]
			for _, m := range state.Models {
				if m.Capability == "text.generate" {
					model = m
					break
				}
			}
			requestID := uuid.NewString()
			accepted, err := catalogapp.NewRequestCredentialTestCommand(store).ExecuteModel(ctx, actor, connected.ProviderID, credential, model.ID, requestID)
			require.NoError(t, err)
			request := catalogapp.CredentialTestRequest{TestID: accepted.TestID, ProviderID: connected.ProviderID, CredentialID: credential, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: requestID}
			service := catalogapp.NewCredentialTestService(store)
			_, _, err = service.Load(ctx, request)
			require.NoError(t, err)
			calls := 0
			client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if slug == "volcengine-speech" {
					require.Equal(t, "seed-tts-2.0", r.Header.Get("X-Api-Resource-Id"))
					return providerResponse(200, `{"code":0,"data":"SUQzAAAAAAAAAAAAAAAAAAAAAA==","usage":{"text_words":3}}`), nil
				}
				return providerResponse(200, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`), nil
			})})
			worker := providerprobe.NewScopedActivities(database, unsealer, client, store)
			result, err := worker.TestCredential(ctx, catalogflow.AgentTestInput{TestID: accepted.TestID, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: requestID, ProviderID: connected.ProviderID, AdapterKey: preset.Document.Config.AdapterKey, Credential: catalogflow.AgentCredential{ID: credential, KeyID: "volc-fixture", Ciphertext: base64.StdEncoding.EncodeToString(ciphertext)}})
			require.NoError(t, err)
			require.Equal(t, "completed", result.ModelResult)
			require.Equal(t, 1, calls)
			require.NoError(t, service.Record(ctx, catalogapp.CredentialTestObservation{CredentialTestRequest: request, Result: result.Result, TestedAt: time.Now().UTC(), Last4: "-key", AuthResult: result.AuthResult, AdapterResult: result.AdapterResult, DiscoveryResult: result.DiscoveryResult, ModelResult: result.ModelResult}))
			state, err = onboarding.Read(ctx, actor, connected.ProviderID)
			require.NoError(t, err)
			enabled, err := onboarding.Activate(ctx, actor, connected.ProviderID, catalogapp.ActivateOnboardingInput{RequestID: uuid.New(), CheckID: accepted.CheckID, ExpectedRevision: state.Revision})
			require.NoError(t, err)
			require.Len(t, enabled.Models, len(keys))
			count := 0
			modelKey := ""
			query := catalogapp.ListModelsInput{ProjectID: project, Limit: 200}
			for {
				catalog, err := store.ListModelsForProject(ctx, actor, query)
				require.NoError(t, err)
				for _, m := range catalog.Models {
					if m.ProviderID == connected.ProviderID {
						count++
						modelKey = m.Key
						require.Equal(t, domain.ModelActive, m.Status)
					}
				}
				if catalog.Next == nil {
					break
				}
				query.After = catalog.Next
			}
			require.Equal(t, len(keys), count)
			if slug == "volcengine-speech" {
				quote, err := operationwire.ProvideOperationStore(database).CreateFreeQuote(ctx, actor, operationapp.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{ModelKey: modelKey, Capability: "audio.generate", Mode: "text2audio", Prompt: "你好", Params: json.RawMessage(`{}`), OutputCount: 1}})
				require.NoError(t, err)
				require.NotEqual(t, uuid.Nil, quote.OperationID)
			}
		})
	}
}
