package agentbusiness_test

import (
	"encoding/json"
	"testing"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	gatewayapp "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	gateway "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
)

// Metadata-only consumer fixture: real frozen quote and ledger admission, then
// controlled original receipt/output rows. Provider/archival execution belongs
// to the separate actual shot-pipeline regression.
func gatewayCreation(t *testing.T, db *gorm.DB, actor identity.Principal, g domain.Grant, input operationhttp.QuoteItemRequest) (*costGateway, gatewayapp.Dispatch) {
	t.Helper()
	ledger := newCostGateway(t, db, g.Scope, g.RootRunID)
	if input.Capability == "video.generate" {
		model := ledger.model
		model.ID = uuid.New()
		model.Name = "shot-" + uuid.NewString()
		model.Protocol = "media"
		model.Capability = "video.generate"
		model.Modes = []string{"text2video"}
		model.Price.Output = 0
		model.Price.Request = creditMicros
		require.NoError(t, ledger.store.PublishModel(t.Context(), model))
		ledger.model = model
		config := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"video"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 60}
		configRaw, err := json.Marshal(config)
		require.NoError(t, err)
		route := gateway.Route{ID: uuid.New(), ModelID: model.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: "media", BaseURL: config.BaseURL, UpstreamModel: "fixture", Purchase: model.Price, DriverConfig: configRaw}
		require.NoError(t, ledger.store.PublishRoute(t.Context(), route, []byte("synthetic"), "fixture", 100, 10000))
	}
	input.ModelKey = ledger.model.Name
	creation := aigateway.Creation{Runtime: &aigateway.Runtime{DB: db}, Run: g.RootRunID, Origin: "agent"}
	quote, err := creation.Quote(t.Context(), actor, g.Scope.ProjectID, uuid.NewString(), input)
	require.NoError(t, err)
	call, err := ledger.store.LoadQuote(t.Context(), g.Scope.ScopeID, quote.OperationID)
	require.NoError(t, err)
	d, err := ledger.store.Admit(t.Context(), call)
	require.NoError(t, err)
	require.True(t, d.Send)
	return ledger, d
}
