package aigateway_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	op "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func TestRebuiltHistoryReadsNewCallsAndPersonalMetadataWithoutMoneyMutations(t *testing.T) {
	db, _, m, a := fixture(t)
	pid := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'History','16:9','realistic')`, pid, a.OrgID, a.ID, a.ScopeID).Error)
	runtime := &aigateway.Runtime{DB: db, Provider: providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		u := domain.Usage{Input: 10, Output: 1}
		cost, err := d.Route.Purchase.Cost(u)
		require.NoError(t, err)
		return domain.Receipt{Body: json.RawMessage(`{"choices":[{"message":{"content":"new output"},"finish_reason":"stop"}]}`), Usage: u, PurchaseMicros: cost}, nil
	})}
	c := aigateway.Creation{Runtime: runtime, Authorize: fixtureAuthority}
	history := aigateway.History{Creation: c}
	quote, err := c.Quote(t.Context(), a, pid, "history", operationhttp.QuoteItemRequest{ModelKey: m.Name, Capability: "text.generate", Mode: "text_to_text", Prompt: "literal %_ prompt", Params: json.RawMessage(`{"max_tokens":100}`), OutputCount: 1})
	require.NoError(t, err)
	_, err = c.Confirm(t.Context(), a, pid, quote.OperationID, "confirmed")
	require.NoError(t, err)
	_, err = c.Execute(t.Context(), taskruntime.Job{BusinessID: quote.OperationID.String()})
	require.NoError(t, err)
	query := op.PersonalHistoryInput{Search: "%_", Kind: "document"}
	page, err := history.ListPersonalHistory(t.Context(), a, query)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Len(t, page.Items[0].Outputs, 1)
	require.True(t, page.Items[0].Outputs[0].CanDownload)
	require.False(t, page.Items[0].Outputs[0].CanPublish)
	output := page.Items[0].Outputs[0].ID
	favorite := true
	key := uuid.New()
	in := op.PersonalHistoryCommand{Action: "favorite", Favorited: &favorite, Targets: []op.PersonalHistoryTarget{{OperationID: quote.OperationID, OutputID: &output, ExpectedRevision: 0}}}
	first, err := history.ApplyPersonalHistoryCommand(t.Context(), a, key, in)
	require.NoError(t, err)
	require.True(t, first.Results[0].OK)
	replay, err := history.ApplyPersonalHistoryCommand(t.Context(), a, key, in)
	require.NoError(t, err)
	require.Equal(t, first, replay)
	query.FavoriteOnly = true
	page, err = history.ListPersonalHistory(t.Context(), a, query)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.True(t, page.Items[0].Outputs[0].Favorite)
	stale, err := history.ApplyPersonalHistoryCommand(t.Context(), a, uuid.New(), in)
	require.NoError(t, err)
	require.False(t, stale.Results[0].OK)
	require.Equal(t, "revision_conflict", stale.Results[0].Code)
	_, err = history.ApplyPersonalHistoryCommand(t.Context(), a, uuid.New(), op.PersonalHistoryCommand{Action: "delete", Targets: []op.PersonalHistoryTarget{{OperationID: quote.OperationID, ExpectedRevision: 1}}})
	require.NoError(t, err)
	page, err = history.ListPersonalHistory(t.Context(), a, op.PersonalHistoryInput{})
	require.NoError(t, err)
	require.Empty(t, page.Items)
	var settled, held int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'", quote.OperationID).Scan(&settled).Error)
	require.EqualValues(t, 1, settled)
	require.NoError(t, db.Raw("SELECT held FROM ai.account WHERE scope_id=?", a.ScopeID).Scan(&held).Error)
	require.Zero(t, held)
	_, _, _, other := fixture(t)
	page, err = history.ListPersonalHistory(t.Context(), other, op.PersonalHistoryInput{})
	require.NoError(t, err)
	require.Empty(t, page.Items)
}

func TestBatchConfirmationSelectionIsImmutableAndInsufficientFundsRollBackAllChildren(t *testing.T) {
	for _, balance := range []int64{1, 100} {
		t.Run(map[int64]string{1: "rollback", 100: "selection"}[balance], func(t *testing.T) {
			db, _, m, a := fixture(t, balance)
			pid := uuid.New()
			require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Batch','16:9','realistic')`, pid, a.OrgID, a.ID, a.ScopeID).Error)
			c := aigateway.Creation{Runtime: &aigateway.Runtime{DB: db, Provider: providerFunc(func(context.Context, application.Dispatch, func(json.RawMessage) error) (domain.Receipt, error) {
				t.Fatal("quotes must not call provider")
				return domain.Receipt{}, nil
			})}, Authorize: fixtureAuthority}
			item := operationhttp.QuoteItemRequest{ModelKey: m.Name, Mode: "text_to_text", Prompt: "batch", Params: json.RawMessage(`{"max_tokens":100}`), OutputCount: 1}
			batch, err := c.BatchQuote(t.Context(), a, pid, "batch", []operationhttp.QuoteItemRequest{item, item, item})
			require.NoError(t, err)
			var exclude []uuid.UUID
			if balance > 1 {
				exclude = []uuid.UUID{*batch.Items[2].OperationID}
			}
			response, err := c.ConfirmBatch(t.Context(), a, pid, *batch.BatchID, "decision", exclude)
			var count int64
			if balance == 1 {
				require.ErrorIs(t, err, domain.ErrFunds)
				require.NoError(t, db.Raw("SELECT count(*) FROM ai.request WHERE scope_id=?", a.ScopeID).Scan(&count).Error)
				require.Zero(t, count)
				return
			}
			require.NoError(t, err)
			again, err := c.ConfirmBatch(t.Context(), a, pid, *batch.BatchID, "decision", exclude)
			require.NoError(t, err)
			require.JSONEq(t, string(response), string(again))
			_, err = c.ConfirmBatch(t.Context(), a, pid, *batch.BatchID, "decision", nil)
			require.ErrorIs(t, err, domain.ErrConflict)
			_, err = c.ConfirmBatch(t.Context(), a, pid, *batch.BatchID, "different", exclude)
			require.ErrorIs(t, err, domain.ErrConflict)
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.request WHERE scope_id=?", a.ScopeID).Scan(&count).Error)
			require.EqualValues(t, 2, count)
		})
	}
}

func TestRebuiltMediaHistoryPublicationAndSubjectPickerUseFormalNewOutput(t *testing.T) {
	db, store, _, a := fixture(t)
	pid := entrypointProject(t, db, a)
	entrypointSession(t, db, a)
	m := mediaModel(t, store, "image.generate", "provider_success_v1")
	body := pngBytes(t)
	objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}}
	runtime := &aigateway.Runtime{DB: db, Objects: fixtureMediaStorage(t, db), MediaArchive: mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{})}
	runtime.Provider = providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		cost, err := d.Route.Purchase.Cost(domain.Usage{})
		require.NoError(t, err)
		raw, _ := json.Marshal(op.GenerationResult{Outcome: op.ProviderOutcomeCompleted, State: op.ProviderTaskSucceeded, TaskID: "published-source", Images: []op.GeneratedImage{{Bytes: body, MediaType: "image/png"}}})
		return domain.Receipt{Body: raw, PurchaseMicros: cost}, nil
	})
	c := aigateway.Creation{Runtime: runtime}
	q, err := c.Quote(t.Context(), a, pid, "publication", operationhttp.QuoteItemRequest{ModelKey: m.Name, Capability: m.Capability, Mode: "text2image", Prompt: "private prompt", Params: json.RawMessage(`{}`), OutputCount: 1})
	require.NoError(t, err)
	_, err = c.Confirm(t.Context(), a, pid, q.OperationID, "confirm")
	require.NoError(t, err)
	_, err = c.Execute(t.Context(), taskruntime.Job{BusinessID: q.OperationID.String()})
	require.NoError(t, err)
	require.NoError(t, runtime.Deliver(t.Context(), q.OperationID))
	task, err := c.Task(t.Context(), a, pid, q.OperationID)
	require.NoError(t, err)
	require.Len(t, task.Outputs, 1)
	h := aigateway.History{Creation: c}
	key := uuid.New()
	input := op.HistoryPublicationInput{Title: "明确公开的结果", Targets: []op.HistoryPublicationTarget{{OperationID: q.OperationID, OutputID: task.Outputs[0].ID}}}
	published, err := op.NewHistoryPublisher(h).Publish(t.Context(), a, key, input)
	require.NoError(t, err)
	require.Equal(t, "published", published.Items[0].Status)
	replay, err := op.NewHistoryPublisher(h).Publish(t.Context(), a, key, input)
	require.NoError(t, err)
	require.Equal(t, published, replay)
	file, err := h.FindPublishedWork(t.Context(), *published.Items[0].WorkID)
	require.NoError(t, err)
	require.Nil(t, file.Work.Prompt)
	require.Equal(t, *task.Outputs[0].MediaAssetID, file.AssetID)
	require.NotEqual(t, uuid.Nil, file.LocationID)
	actor := mediaapp.Actor{ID: a.ID, OrgID: a.OrgID, ScopeID: a.ScopeID, SessionID: a.SessionID, CredentialEpoch: a.CredentialEpoch, PlatformEpoch: a.PlatformEpoch}
	picker := subjectwire.NewSourceLibrary(db, subjectwire.Objects{}, time.Minute)
	page, err := picker.List(t.Context(), actor, mediaapp.SubjectCatalogueQuery{Kind: "image", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, file.AssetID, page.Items[0].Media.ID)
	_, err = h.ApplyPersonalHistoryCommand(t.Context(), a, uuid.New(), op.PersonalHistoryCommand{Action: "delete", Targets: []op.PersonalHistoryTarget{{OperationID: q.OperationID, ExpectedRevision: 0}}})
	require.NoError(t, err)
	page, err = picker.List(t.Context(), actor, mediaapp.SubjectCatalogueQuery{Kind: "image", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Empty(t, page.Items)
	require.NoError(t, db.Exec("UPDATE media.media_asset SET is_delete=true WHERE id=?", file.AssetID).Error)
	_, err = h.FindPublishedWork(t.Context(), *published.Items[0].WorkID)
	require.ErrorIs(t, err, op.ErrPublicNotFound)
	unavailable, err := c.Task(t.Context(), a, pid, q.OperationID)
	require.NoError(t, err)
	require.Equal(t, "unrecoverable", unavailable.BillingOutcome.DeliveryState)
	require.Nil(t, unavailable.Outputs[0].MediaAssetID)
}
