package operation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestProviderTrialQuoteRequiresAdminAndBudgetAndCanConfirmDraft(t *testing.T) {
	database := operationStoreDB(t)
	f := seedManagedFixture(t, database)
	i := f.identity
	require.NoError(t, database.Exec(`UPDATE catalog.model_release SET status='draft',upstream_cost='{"currency":"USD","max_micros":100}' WHERE id=?`, f.release).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.provider SET status='disabled' WHERE id=?`, f.provider).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET status='disabled',active_release_id=NULL,capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],limits='{"max_outputs":1}',supports_cancel=false,queue='provider',moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}' WHERE id=?`, i.PriceRuleVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE workspace.project SET allow_overseas_models=true WHERE id=?`, i.ProjectID).Error)
	var actor identityapp.Principal
	require.NoError(t, database.Raw(`SELECT actor.id,actor.org_id,actor.role FROM identity."user" actor JOIN workspace.project p ON p.org_id=actor.org_id WHERE p.id=?`, i.ProjectID).Scan(&actor).Error)
	input := application.ProviderTrialQuote{ProjectID: i.ProjectID, Mode: "text_to_text", Prompt: "fixture prompt", Params: json.RawMessage(`{}`), MaxChargeMicros: 10, SupplierMaxMicros: 100, SupplierCurrency: "USD", RequestID: uuid.New()}
	_, err := f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, input)
	require.ErrorIs(t, err, identityapp.ErrForbidden)
	actor.PlatformRole = "platform_admin"
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	low := input
	low.SupplierMaxMicros = 99
	_, err = f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, low)
	require.Error(t, err)
	quote, err := f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, input)
	require.NoError(t, err)
	require.Equal(t, int64(10), quote.QuoteMicros)
	replay, err := f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, input)
	require.NoError(t, err)
	require.Equal(t, quote.OperationID, replay.OperationID)
	input.Prompt = "different intent"
	_, err = f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, input)
	require.ErrorIs(t, err, application.ErrQuoteKeyReused)
	loaded, err := f.store.LoadWorkflowOperation(t.Context(), quote.OperationID)
	require.NoError(t, err)
	model, price, err := f.store.ReadCurrentQuoteCatalog(t.Context(), actor, loaded.Operation, time.Now())
	require.NoError(t, err)
	require.NotNil(t, model)
	require.NotNil(t, price)
	confirmed, err := f.store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: i.ProjectID, OperationID: quote.OperationID, RequestID: uuid.NewString()})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, confirmed.ReservationID)
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, actor.ID).Error)
	// Revoking platform authority invalidates a still-open trial before dispatch.
	model, price, err = f.store.ReadCurrentQuoteCatalog(t.Context(), actor, loaded.Operation, time.Now())
	require.NoError(t, err)
	require.Nil(t, model)
	require.Nil(t, price)
}
