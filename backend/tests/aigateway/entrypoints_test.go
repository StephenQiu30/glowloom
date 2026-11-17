package aigateway_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

func entrypointProject(t *testing.T, db *gorm.DB, a identity.Principal) uuid.UUID {
	t.Helper()
	p := uuid.New()
	require.NoError(t, db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=clock_timestamp() WHERE id=?`, a.ID.String()+"@entrypoint.invalid", a.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Entrypoint acceptance','16:9','realistic')`, p, a.OrgID, a.ID, a.ScopeID).Error)
	return p
}

func entrypointSession(t *testing.T, db *gorm.DB, a identity.Principal) {
	t.Helper()
	now := time.Now().UTC()
	digest := sha256.Sum256(a.SessionID[:])
	require.NoError(t, db.Exec(`UPDATE identity."user" SET session_epoch=1,platform_epoch=1 WHERE id=?`, a.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent) VALUES(?,?,?,1,?,?,?,false)`, a.SessionID, a.ID, digest[:], now, now, now.Add(time.Hour)).Error)
}

func entrypointTextInput(m domain.Model) operationhttp.QuoteItemRequest {
	return operationhttp.QuoteItemRequest{ModelKey: m.Name, Capability: "text.generate", Mode: "text_to_text", Prompt: "entrypoint acceptance", Params: json.RawMessage(`{"max_tokens":100}`), OutputCount: 1}
}

func TestEntrypointMediaWorkbenchQueueExecuteDeliverActualPNG(t *testing.T) {
	db, store, _, a := fixture(t)
	p := entrypointProject(t, db, a)
	m := mediaModel(t, store, "image.generate", "provider_success_v1")
	body := pngBytes(t)
	objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}}
	var sends atomic.Int64
	r := &aigateway.Runtime{DB: db, Objects: fixtureMediaStorage(t, db), MediaArchive: mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{})}
	r.Provider = providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		sends.Add(1)
		require.Equal(t, p, d.Call.Project)
		require.Equal(t, "creation", d.Call.Origin)
		result := operation.GenerationResult{Outcome: operation.ProviderOutcomeCompleted, State: operation.ProviderTaskSucceeded, TaskID: "entrypoint-physical-png", Images: []operation.GeneratedImage{{Bytes: body, MediaType: "image/png"}}}
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		cost, err := d.Route.Purchase.Cost(domain.Usage{})
		require.NoError(t, err)
		return domain.Receipt{Body: raw, UpstreamID: result.TaskID, PurchaseMicros: cost}, nil
	})
	c := aigateway.Creation{Runtime: r, Authorize: fixtureAuthority}
	q, err := c.Quote(t.Context(), a, p, uuid.NewString(), operationhttp.QuoteItemRequest{ModelKey: m.Name, Capability: m.Capability, Mode: "text2image", Prompt: "a real PNG", Params: json.RawMessage(`{}`), OutputCount: 1})
	require.NoError(t, err)
	for range 2 {
		_, err = c.Confirm(t.Context(), a, p, q.OperationID, "entrypoint-confirm")
		require.NoError(t, err)
	}
	var pin uuid.UUID
	require.NoError(t, db.Raw("SELECT id FROM media.storage_work_intent WHERE kind='operation' AND owner_id=? AND work_id=? AND state='open'", p, q.OperationID).Row().Scan(&pin))
	require.NotEqual(t, uuid.Nil, pin, "confirm must durably pin before exposing execute")
	var count int64
	require.NoError(t, db.Raw("SELECT count(*) FROM infra.task_work WHERE kind='ai.execute' AND business_id=?", q.OperationID.String()).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	for range 2 {
		next, err := c.Execute(t.Context(), taskruntime.Job{Kind: pg.ExecutionKind, BusinessID: q.OperationID.String()})
		require.NoError(t, err)
		require.True(t, next.IsZero())
	}
	require.EqualValues(t, 1, sends.Load())
	require.NoError(t, db.Raw("SELECT count(*) FROM infra.task_work WHERE kind='ai.deliver' AND business_id=?", q.OperationID.String()).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	before, err := c.Task(t.Context(), a, p, q.OperationID)
	require.NoError(t, err)
	require.Equal(t, "ingesting", string(before.Status))
	handlers := map[string]taskruntime.Handler{}
	r.RegisterWorker(handlers)
	next, err := handlers["ai.deliver"](t.Context(), taskruntime.Job{Kind: "ai.deliver", BusinessID: q.OperationID.String()})
	require.NoError(t, err)
	require.True(t, next.IsZero())
	task, err := c.Task(t.Context(), a, p, q.OperationID)
	require.NoError(t, err)
	require.Equal(t, "completed", string(task.Status))
	require.Equal(t, "ready", task.BillingOutcome.DeliveryState)
	require.Len(t, task.Outputs, 1)
	require.NotNil(t, task.Outputs[0].MediaAssetID)
	assetID := *task.Outputs[0].MediaAssetID
	type assetSnapshot struct {
		ID, SourceAIRequestID     uuid.UUID
		SourceOperationID         *uuid.UUID
		Status, ObjectKey, SHA256 string
	}
	var asset assetSnapshot
	require.NoError(t, db.Raw("SELECT id,source_ai_request_id,source_operation_id,status,object_key,sha256 FROM media.media_asset WHERE id=?", assetID).Scan(&asset).Error)
	require.Equal(t, assetID, asset.ID)
	require.Equal(t, q.OperationID, asset.SourceAIRequestID)
	require.Nil(t, asset.SourceOperationID)
	require.Equal(t, "ready", asset.Status)
	require.Equal(t, body, objects.files[asset.ObjectKey])
	require.Equal(t, domain.Digest(body), asset.SHA256)
	require.Equal(t, 3, objects.writes)
	writesAfterFirstDelivery := objects.writes
	var settlementsAfterFirstDelivery int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'", q.OperationID).Scan(&settlementsAfterFirstDelivery).Error)
	require.EqualValues(t, 1, settlementsAfterFirstDelivery)

	// Snapshot the first successful delivery before replaying its durable handler.
	// A second read without an intervening replay would miss replaced asset identity.
	next, err = handlers["ai.deliver"](t.Context(), taskruntime.Job{Kind: "ai.deliver", BusinessID: q.OperationID.String()})
	require.NoError(t, err)
	require.True(t, next.IsZero())
	again, err := c.Task(t.Context(), a, p, q.OperationID)
	require.NoError(t, err)
	require.Equal(t, task.Outputs, again.Outputs)
	require.Equal(t, assetID, *again.Outputs[0].MediaAssetID)
	var replayedAsset assetSnapshot
	require.NoError(t, db.Raw("SELECT id,source_ai_request_id,source_operation_id,status,object_key,sha256 FROM media.media_asset WHERE id=?", *again.Outputs[0].MediaAssetID).Scan(&replayedAsset).Error)
	require.Equal(t, asset, replayedAsset)
	require.Equal(t, asset.SHA256, domain.Digest(objects.files[replayedAsset.ObjectKey]))
	require.Equal(t, writesAfterFirstDelivery, objects.writes)
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'", q.OperationID).Scan(&count).Error)
	require.Equal(t, settlementsAfterFirstDelivery, count)
	require.EqualValues(t, 1, sends.Load())
	for _, query := range []string{
		"SELECT count(*) FROM ai.hold WHERE request_id=? AND closed AND charged=1",
		"SELECT count(*) FROM ai.attempt WHERE request_id=? AND receipt IS NOT NULL",
		"SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='freeze'",
		"SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'",
		"SELECT count(*) FROM media.media_asset WHERE source_ai_request_id=?",
	} {
		require.NoError(t, db.Raw(query, q.OperationID).Scan(&count).Error)
		require.EqualValues(t, 1, count, query)
	}
	for _, query := range []string{
		"SELECT count(*) FROM operation.operation WHERE id=?",
		"SELECT count(*) FROM billing.reservation WHERE operation_id=?",
	} {
		require.NoError(t, db.Raw(query, q.OperationID).Scan(&count).Error)
		require.Zero(t, count, query)
	}
}

func TestEntrypointAgentReadyGrantRevocationReleasesWithoutSend(t *testing.T) {
	for _, revoke := range []string{"barrier", "owner_epoch", "grant_expired"} {
		t.Run(revoke, func(t *testing.T) {
			db, _, m, a := fixture(t)
			p := entrypointProject(t, db, a)
			a.SessionID = uuid.Nil
			root, submission, grant := uuid.New(), uuid.New(), uuid.New()
			scope, err := json.Marshal(agent.Scope{ActorID: a.ID, OrgID: a.OrgID, ScopeID: a.ScopeID, ProjectID: p})
			require.NoError(t, err)
			require.NoError(t, db.Exec(`INSERT INTO agent_business.run_authorization(root_run_id,session_id,submission_id,request_id,content_hash,scope,budget_micros,owner_epoch) VALUES(?,?,?,?,?,?::jsonb,2000000,1)`, root, uuid.New(), submission, uuid.New(), domain.Digest([]byte("entrypoint")), string(scope)).Error)
			require.NoError(t, db.Exec(`INSERT INTO agent_business.execution_grant(grant_id,root_run_id,submission_id,owner_epoch,authorization_revision,expires_at) VALUES(?,?,?,1,1,clock_timestamp()+interval '1 hour')`, grant, root, submission).Error)
			var sends atomic.Int64
			r := &aigateway.Runtime{DB: db, Provider: providerFunc(func(context.Context, application.Dispatch, func(json.RawMessage) error) (domain.Receipt, error) {
				sends.Add(1)
				return domain.Receipt{}, domain.ErrUnknown
			})}
			prepare := aigateway.Creation{Runtime: r, Authorize: fixtureAuthority, Run: root, Origin: "agent", RunLimit: aigateway.ApprovedRunLimit, Permit: aigateway.AgentPermit}
			q, err := prepare.Quote(t.Context(), a, p, uuid.NewString(), entrypointTextInput(m))
			require.NoError(t, err)
			_, err = prepare.Confirm(t.Context(), a, p, q.OperationID, "approved")
			require.NoError(t, err)
			var proof []byte
			require.NoError(t, db.Raw("SELECT worker_proof FROM ai.job WHERE request_id=?", q.OperationID).Row().Scan(&proof))
			require.Contains(t, string(proof), grant.String())
			_, err = pg.New(db, a, aigateway.AgentWorkerAuthority(proof)).LoadQuote(t.Context(), a.ScopeID, q.OperationID)
			require.NoError(t, err, "the original frozen grant must authorize the ready job")
			var ready int64
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.attempt WHERE request_id=? AND state='ready'", q.OperationID).Scan(&ready).Error)
			require.EqualValues(t, 1, ready)
			switch revoke {
			case "barrier":
				require.NoError(t, db.Exec("UPDATE agent_business.run_authorization SET barrier=true WHERE root_run_id=?", root).Error)
			case "owner_epoch":
				require.NoError(t, db.Exec("UPDATE agent_business.run_authorization SET owner_epoch=owner_epoch+1 WHERE root_run_id=?", root).Error)
			default:
				require.NoError(t, db.Exec("UPDATE agent_business.execution_grant SET expires_at=clock_timestamp()-interval '1 second' WHERE grant_id=?", grant).Error)
			}
			worker := aigateway.Creation{Runtime: r}
			for range 2 {
				next, err := worker.Execute(t.Context(), taskruntime.Job{Kind: pg.ExecutionKind, BusinessID: q.OperationID.String()})
				require.NoError(t, err)
				require.True(t, next.IsZero())
			}
			require.Zero(t, sends.Load())
			var actual struct {
				State                                            string
				Held, Charged, Released, ReleaseEvents, Attempts int64
			}
			require.NoError(t, db.Raw(`SELECT r.state,a.held,h.charged,h.released,(SELECT count(*) FROM ai.money_event WHERE request_id=r.id AND kind='release') AS release_events,(SELECT count(*) FROM ai.attempt WHERE request_id=r.id) AS attempts FROM ai.request r JOIN ai.account a ON a.scope_id=r.scope_id JOIN ai.hold h ON h.request_id=r.id WHERE r.id=?`, q.OperationID).Scan(&actual).Error)
			require.Equal(t, "released", actual.State)
			require.Zero(t, actual.Held)
			require.Zero(t, actual.Charged)
			require.Positive(t, actual.Released)
			require.EqualValues(t, 1, actual.ReleaseEvents)
			require.EqualValues(t, 1, actual.Attempts)
		})
	}
}

func TestEntrypointSessionReadyRevocationReleasesWithoutSend(t *testing.T) {
	db, _, m, a := fixture(t)
	p := entrypointProject(t, db, a)
	entrypointSession(t, db, a)
	var sends atomic.Int64
	r := &aigateway.Runtime{DB: db, Provider: providerFunc(func(context.Context, application.Dispatch, func(json.RawMessage) error) (domain.Receipt, error) {
		sends.Add(1)
		return domain.Receipt{}, domain.ErrUnknown
	})}
	c := aigateway.Creation{Runtime: r}
	q, err := c.Quote(t.Context(), a, p, uuid.NewString(), entrypointTextInput(m))
	require.NoError(t, err)
	_, err = c.Confirm(t.Context(), a, p, q.OperationID, "session-confirm")
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE identity.user_session SET revoked_at=clock_timestamp() WHERE id=?", a.SessionID).Error)
	for range 2 {
		next, err := c.Execute(t.Context(), taskruntime.Job{Kind: pg.ExecutionKind, BusinessID: q.OperationID.String()})
		require.NoError(t, err)
		require.True(t, next.IsZero())
	}
	require.Zero(t, sends.Load())
	var actual struct {
		State         string
		Held, Charged int64
	}
	require.NoError(t, db.Raw("SELECT r.state,a.held,h.charged FROM ai.request r JOIN ai.account a ON a.scope_id=r.scope_id JOIN ai.hold h ON h.request_id=r.id WHERE r.id=?", q.OperationID).Scan(&actual).Error)
	require.Equal(t, "released", actual.State)
	require.Zero(t, actual.Held)
	require.Zero(t, actual.Charged)
}

func TestEntrypointSDKReplayKeepsFrozenModelAfterAliasAndPurchaseChange(t *testing.T) {
	db, store, old, a := fixture(t)
	token, err := store.IssueGrant(t.Context(), a.ScopeID, []uuid.UUID{old.ID}, 20, 3600)
	require.NoError(t, err)
	grant, err := pg.ResolveGrant(t.Context(), db, token)
	require.NoError(t, err)
	bound := store.WithGrant(grant.ID)
	var sends atomic.Int64
	provider := providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		sends.Add(1)
		u := domain.Usage{Input: 100, Output: 20}
		cost, err := d.Route.Purchase.Cost(u)
		require.NoError(t, err)
		return domain.Receipt{Body: json.RawMessage(`{"choices":[{"message":{"content":"frozen"},"finish_reason":"stop"}]}`), Usage: u, PurchaseMicros: cost}, nil
	})
	raw := json.RawMessage(`{"model":"` + old.Name + `","max_tokens":100,"messages":[{"role":"user","content":"frozen request"}]}`)
	original, err := application.Prepare(old, a.ID, a.ScopeID, uuid.Nil, grant.ID, "original-sdk-key", "sdk", raw)
	require.NoError(t, err)
	first, err := application.New(bound, provider).Execute(t.Context(), original, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, first.Charged)
	newer := old
	newer.ID = uuid.New()
	newer.Price.Input *= 1000
	newer.Price.Output *= 1000
	require.NoError(t, store.PublishModel(t.Context(), newer))
	purchase := old.Price
	purchase.Request = 20000
	require.NoError(t, store.PublishRoute(t.Context(), domain.Route{ID: uuid.New(), ModelID: newer.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: newer.Protocol, BaseURL: "https://8.8.8.8/v1", UpstreamModel: "newer-purchase", Purchase: purchase}, []byte("synthetic"), "fixture", 2, 10000))
	var enabled bool
	require.NoError(t, db.Raw("SELECT enabled FROM ai.model WHERE id=?", old.ID).Row().Scan(&enabled))
	require.False(t, enabled)
	frozen, err := bound.CallModel(t.Context(), a.ScopeID, grant.ID, original.Key, original.Origin, old.Name, old.Protocol)
	require.NoError(t, err)
	require.Equal(t, old, frozen)
	replay, err := application.Prepare(frozen, a.ID, a.ScopeID, uuid.Nil, grant.ID, original.Key, original.Origin, raw)
	require.NoError(t, err)
	replayed, err := application.New(bound, provider).Execute(t.Context(), replay, nil)
	require.NoError(t, err)
	require.Equal(t, first, replayed)
	require.EqualValues(t, 1, sends.Load())
	current, err := bound.CallModel(t.Context(), a.ScopeID, grant.ID, "new-sdk-key", "sdk", old.Name, old.Protocol)
	require.NoError(t, err)
	require.Equal(t, newer, current)
	forbidden, err := application.Prepare(current, a.ID, a.ScopeID, uuid.Nil, grant.ID, "new-sdk-key", "sdk", raw)
	require.NoError(t, err)
	_, err = bound.Admit(t.Context(), forbidden)
	require.ErrorIs(t, err, domain.ErrForbidden)
	token, err = store.IssueGrant(t.Context(), a.ScopeID, []uuid.UUID{old.ID, newer.ID}, 20, 3600)
	require.NoError(t, err)
	allowed, err := pg.ResolveGrant(t.Context(), db, token)
	require.NoError(t, err)
	fresh, err := application.Prepare(current, a.ID, a.ScopeID, uuid.Nil, allowed.ID, "allowed-new-sdk-key", "sdk", raw)
	require.NoError(t, err)
	charged, err := application.New(store.WithGrant(allowed.ID), provider).Execute(t.Context(), fresh, nil)
	require.NoError(t, err)
	want, err := newer.Price.Cost(domain.Usage{Input: 100, Output: 20})
	require.NoError(t, err)
	wantCredits, err := domain.Credits(want)
	require.NoError(t, err)
	require.Equal(t, wantCredits, charged.Charged)
	require.Greater(t, charged.Charged, first.Charged)
	require.Equal(t, newer.ID, charged.ModelID)
	require.NotEqual(t, want, charged.Receipt.PurchaseMicros)
	require.EqualValues(t, 2, sends.Load())
	var requests int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.request WHERE scope_id=?", a.ScopeID).Scan(&requests).Error)
	require.EqualValues(t, 2, requests)
}

func TestEntrypointFabricatedCrossScopeCannotMutateSendOrFunds(t *testing.T) {
	for _, action := range []string{"release_not_sent", "cancel_queued", "claim_queued"} {
		t.Run(action, func(t *testing.T) {
			db, owner, m, a := fixture(t)
			_, outsider, _, b := fixture(t)
			original := call(t, m, a)
			var attempt uuid.UUID
			if action == "release_not_sent" {
				d, err := owner.Admit(t.Context(), original)
				require.NoError(t, err)
				attempt = d.Attempt
			} else {
				_, err := owner.Queue(t.Context(), original)
				require.NoError(t, err)
				require.NoError(t, db.Raw("SELECT id FROM ai.attempt WHERE request_id=?", original.ID).Row().Scan(&attempt))
			}
			type snapshot struct {
				Available, Held, Charged, Released, Events int64
				AttemptState, RequestState                 string
			}
			read := func() snapshot {
				var s snapshot
				require.NoError(t, db.Raw(`SELECT a.available,a.held,h.charged,h.released,r.state AS request_state,x.state AS attempt_state,(SELECT count(*) FROM ai.money_event WHERE request_id=r.id) AS events FROM ai.request r JOIN ai.account a ON a.scope_id=r.scope_id JOIN ai.hold h ON h.request_id=r.id JOIN ai.attempt x ON x.request_id=r.id WHERE r.id=?`, original.ID).Scan(&s).Error)
				return s
			}
			before := read()
			forged := original
			forged.Scope, forged.Actor = b.ScopeID, b.ID
			switch action {
			case "release_not_sent":
				_, err := outsider.ReleaseNotSent(t.Context(), application.Dispatch{Call: forged, Attempt: attempt})
				require.Error(t, err)
			case "cancel_queued":
				_, err := outsider.CancelQueued(t.Context(), forged)
				require.Error(t, err)
			default:
				d, err := outsider.ClaimQueued(t.Context(), forged)
				require.Error(t, err)
				require.False(t, d.Send)
			}
			require.Equal(t, before, read(), "fabricated identity must leave the original attempt, hold and ledger untouched")
			if action == "release_not_sent" {
				released, err := owner.ReleaseNotSent(t.Context(), application.Dispatch{Call: original, Attempt: attempt})
				require.NoError(t, err)
				require.Equal(t, "released", released.State)
			} else {
				d, err := owner.ClaimQueued(t.Context(), original)
				require.NoError(t, err)
				require.True(t, d.Send, "the original send permission remains available exactly once")
				require.Equal(t, attempt, d.Attempt)
				again, err := owner.ClaimQueued(t.Context(), original)
				require.NoError(t, err)
				require.False(t, again.Send)
			}
		})
	}
}
