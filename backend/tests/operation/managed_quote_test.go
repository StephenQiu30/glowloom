package operation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestManagedQuoteFreezesDefaultsAndPricesTokenBudget(t *testing.T) {
	now := time.Now().UTC()
	input := operationapp.CreateFreeQuoteInput{ProjectID: uuid.New(), RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{ModelKey: "fixture", Capability: "text.generate", Mode: "text_to_text", Prompt: "晚霞", Params: json.RawMessage(`{}`), OutputCount: 1}}
	catalog := operationapp.FreeQuoteCatalog{ReleaseID: uuid.New(), ModelVersionID: uuid.New(), ProviderModelID: "fixture", Region: "overseas", Modes: []string{"text_to_text"}, Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(`[{"field":"max_tokens","type":"integer","required":true,"default":100}]`), Price: catalogdomain.PriceRuleVersion{ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1, Unit: catalogdomain.PricePer1KTokens, Rule: json.RawMessage(`{"input_micros_per_1k":1000,"output_micros_per_1k":2000}`), Currency: "CNY", EffectiveFrom: now.Add(-time.Minute)}}
	quoted, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil)
	require.NoError(t, err)
	require.Equal(t, int64(206), *quoted.Operation.QuoteMicros)
	require.JSONEq(t, `{"max_tokens":100}`, string(quoted.Operation.Params))
	same, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil)
	require.NoError(t, err)
	require.Equal(t, quoted.Operation.InputHash, same.Operation.InputHash)
	catalog.ReleaseID = uuid.New()
	changed, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil)
	require.NoError(t, err)
	require.NotEqual(t, quoted.Operation.InputHash, changed.Operation.InputHash)
}
