package operation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestProviderTrialQuoteRequiresAdminAndBudgetAndCanConfirmDraft(t *testing.T) {
	database := operationStoreDB(t)
	f := seedManagedFixture(t, database)
	i := f.identity
	require.NoError(t, database.Exec(`UPDATE catalog.model_release SET status='draft',upstream_cost='{"currency":"USD","max_micros":100}' WHERE id=?`, f.release).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.provider SET status='disabled' WHERE id=?`, f.provider).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET status='disabled',active_release_id=NULL,capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],limits='{"max_outputs":1}',supports_cancel=false,queue='provider',moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}',source_url='https://provider.example/pricing',reviewed_at='2026-11-03T00:00:00Z' WHERE id=?`, i.PriceRuleVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE workspace.project SET allow_overseas_models=true WHERE id=?`, i.ProjectID).Error)
	var actor identityapp.Principal
	require.NoError(t, database.Raw(`SELECT actor.id,actor.org_id,actor.role FROM identity."user" actor JOIN workspace.project p ON p.org_id=actor.org_id WHERE p.id=?`, i.ProjectID).Scan(&actor).Error)
	actor = identityfixture.Session(t, database, actor)
	input := application.ProviderTrialQuote{ProjectID: i.ProjectID, Mode: "text_to_text", Prompt: "fixture prompt", Params: json.RawMessage(`{}`), MaxChargeMicros: 10, SupplierMaxMicros: 100, SupplierCurrency: "USD", RequestID: uuid.New()}
	_, err := f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, input)
	require.ErrorIs(t, err, identityapp.ErrForbidden)
	actor.PlatformRole = "platform_admin"
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, database, actor)
	low := input
	low.SupplierMaxMicros = 99
	_, err = f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, low)
	require.Error(t, err)
	var scope, lot uuid.UUID
	require.NoError(t, database.Raw(`SELECT billing.bind_project_wallet(?)`, i.ProjectID).Row().Scan(&scope))
	require.NoError(t, database.Raw(`SELECT billing.issue_promotional_credit(?,?,100,now()+interval '1 hour',NULL,?)`,
		scope, uuid.New(), "isolated trial funding").Row().Scan(&lot))
	quote, err := f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, input)
	require.NoError(t, err)
	require.Equal(t, int64(10), quote.QuoteMicros)
	var detail application.QuoteCostDetail
	require.NoError(t, json.Unmarshal(quote.QuoteDetail, &detail))
	require.Equal(t, "https://provider.example/pricing", detail.SourceURL)
	require.NotNil(t, detail.ReviewedAt)
	require.Equal(t, "2026-11-03T00:00:00Z", detail.ReviewedAt.UTC().Format(time.RFC3339))
	recovery, err := f.store.ReadProviderTrialRecovery(t.Context(), actor, f.release)
	require.NoError(t, err)
	require.True(t, recovery.Found)
	require.Equal(t, quote.OperationID, recovery.OperationID)
	require.Equal(t, input.ProjectID, recovery.ProjectID)
	require.Equal(t, input.RequestID, recovery.RequestID)
	require.NotEqual(t, uuid.Nil, recovery.ConfirmRequestID)
	replay, err := f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, input)
	require.NoError(t, err)
	require.Equal(t, quote.OperationID, replay.OperationID)
	input.Prompt = "different intent"
	_, err = f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, input)
	require.ErrorIs(t, err, application.ErrQuoteKeyReused)
	loaded, err := f.store.FindOperation(t.Context(), actor, i.ProjectID, quote.OperationID)
	require.NoError(t, err)
	model, price, err := f.store.ReadCurrentQuoteCatalog(t.Context(), actor, loaded, time.Now())
	require.NoError(t, err)
	require.NotNil(t, model)
	require.NotNil(t, price)
	confirmed, err := f.store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: i.ProjectID, OperationID: quote.OperationID, RequestID: uuid.NewString()})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, confirmed.ReservationID)
	require.NoError(t, database.Exec(`UPDATE catalog.model_release SET status='active' WHERE id=?`, f.release).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET status='active',active_release_id=? WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.release, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.provider SET status='active' WHERE id=?`, f.provider).Error)
	input.RequestID = uuid.New()
	activeQuote, err := f.store.CreateProviderTrialQuote(t.Context(), actor, f.release, input)
	require.NoError(t, err)
	activeOperation, err := f.store.FindOperation(t.Context(), actor, i.ProjectID, activeQuote.OperationID)
	require.NoError(t, err)
	activeModel, _, err := f.store.ReadCurrentQuoteCatalog(t.Context(), actor, activeOperation, time.Now())
	require.NoError(t, err)
	require.NotNil(t, activeModel)
	_, err = f.store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{ProjectID: i.ProjectID, OperationID: activeQuote.OperationID, RequestID: uuid.NewString()})
	require.NoError(t, err)
	var requestKey string
	require.NoError(t, database.Raw(`SELECT provider_request_key FROM operation.operation WHERE id=?`, activeQuote.OperationID).Scan(&requestKey).Error)
	_, err = f.store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: activeQuote.OperationID, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting, ProviderRequestKey: &requestKey})
	require.NoError(t, err)
	require.NoError(t, f.store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{OperationID: activeQuote.OperationID, Action: "submit", Attempt: 1, DispatchRequired: true}))
	dispatch := i
	dispatch.OperationID = activeQuote.OperationID
	dispatch.RequestKey = requestKey
	claimed, err := f.store.ClaimProviderDispatch(t.Context(), dispatch)
	require.NoError(t, err)
	require.True(t, claimed.Claimed)

	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, actor.ID).Error)
	// Revoking platform authority invalidates a still-open trial before dispatch.
	model, price, err = f.store.ReadCurrentQuoteCatalog(t.Context(), actor, loaded, time.Now())
	require.NoError(t, err)
	require.Nil(t, model)
	require.Nil(t, price)
}
