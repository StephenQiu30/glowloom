package operation_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestManagedPlatformDefaultOnlyFillsAbsentProjectChoice(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	actor, project := operationStoreProject(t, db)
	require.NoError(t, db.Exec(`UPDATE workspace.project SET allow_overseas_models=true,default_models='{}' WHERE id=?`, project).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],param_schema='[]',limits='{"max_outputs":1}' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":1}' WHERE id=?`, f.identity.PriceRuleVersionID).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.capability_default(capability,release_id) VALUES('text.generate',?) ON CONFLICT(capability) DO UPDATE SET release_id=EXCLUDED.release_id`, f.release).Error)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second*5)
		defer cancel()
		require.NoError(t, db.WithContext(ctx).Exec(`DELETE FROM catalog.capability_default WHERE release_id=?`, f.release).Error)
	})
	input := application.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: application.FreeQuoteItemInput{Capability: "text.generate", Mode: "text_to_text", Prompt: "fixture", Params: json.RawMessage(`{}`), OutputCount: 1}}
	quote, err := f.store.CreateFreeQuote(t.Context(), actor, input)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, quote.OperationID)
	require.NoError(t, db.Exec(`UPDATE workspace.project SET default_models='{"text.generate":"unavailable-explicit-model"}' WHERE id=?`, project).Error)
	input.RequestID = uuid.NewString()
	_, err = f.store.CreateFreeQuote(t.Context(), actor, input)
	require.ErrorIs(t, err, application.ErrFreeQuoteModelMissing)
}
