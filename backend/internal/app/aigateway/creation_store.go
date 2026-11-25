package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/domain"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/adapter/http"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// Creation is a caller of the new gateway; it owns saved editor intent, never money.
type Creation struct {
	Runtime   *Runtime
	Authorize pg.Authority
	Run       uuid.UUID
	Origin    string
	RunLimit  pg.RunLimit
	Permit    pg.WorkerPermit
}

func (c Creation) origin() string {
	if c.Origin != "" {
		return c.Origin
	}
	return "creation"
}
func (c Creation) auth() pg.Authority {
	if c.Authorize != nil {
		return c.Authorize
	}
	return Authority
}
func (c Creation) store(tx *gorm.DB, a identity.Principal) *pg.Store {
	return pg.New(tx, a, c.auth()).WithRunLimit(c.RunLimit).WithWorkerPermit(c.Permit)
}
func canonical(v any) []byte {
	raw, _ := json.Marshal(v)
	normalized, err := domain.CanonicalJSON(raw)
	if err != nil {
		return nil
	}
	return normalized
}

type creationRow struct {
	ID, ScopeID, ActorID, ProjectID, ConfirmKey uuid.UUID
	OwnerKey                                    *string
	Epoch                                       int64
	Source, Draft, Presentation, Actor          []byte
	InputHash, State                            string
}

func (c Creation) row(tx *gorm.DB, a identity.Principal, p, id uuid.UUID) (creationRow, error) {
	var row creationRow
	q := tx.Raw("SELECT * FROM ai.creation WHERE id=? AND scope_id=? AND project_id=?", id, a.ScopeID, p).Scan(&row)
	if q.Error != nil {
		return row, q.Error
	}
	if q.RowsAffected != 1 {
		return row, domain.ErrForbidden
	}
	return row, nil
}

// Quote freezes the approved sale and source, without creating a hold or task wakeup.
func (c Creation) Quote(ctx context.Context, a identity.Principal, p uuid.UUID, key string, in operationhttp.QuoteItemRequest) (operation.CreateFreeQuoteResult, error) {
	var out operation.CreateFreeQuoteResult
	if p == uuid.Nil || key == "" || len(key) > 120 {
		return out, domain.ErrInvalid
	}
	fingerprint := domain.Digest(canonical(in))
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.auth()(ctx, tx, a, a.ScopeID, p, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", a.ScopeID.String()+"/creation/"+key).Error; err != nil {
			return err
		}
		var prior creationRow
		q := tx.Raw("SELECT c.* FROM ai.creation c JOIN ai.quote q ON q.id=c.id WHERE q.scope_id=? AND q.idempotency_key=?", a.ScopeID, "creation/"+key).Scan(&prior)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if prior.InputHash != fingerprint || prior.ActorID != a.ID || prior.ProjectID != p {
				return domain.ErrConflict
			}
			return json.Unmarshal(prior.Presentation, &out)
		}
		epoch := int64(0)
		owner := any(nil)
		if in.InputSource != nil {
			if !in.InputSource.Valid() || in.ExpectedSubmissionEpoch == nil {
				return domain.ErrInvalid
			}
			owner = in.InputSource.OwnerKey()
			if err := tx.Exec("INSERT INTO ai.creation_slot(actor_id,project_id,owner_key) VALUES(?,?,?) ON CONFLICT DO NOTHING", a.ID, p, owner).Error; err != nil {
				return err
			}
			var slot struct {
				Epoch      int64
				CreationID *uuid.UUID
			}
			if err := tx.Raw("SELECT epoch,creation_id FROM ai.creation_slot WHERE actor_id=? AND project_id=? AND owner_key=? FOR UPDATE", a.ID, p, owner).Scan(&slot).Error; err != nil {
				return err
			}
			if slot.Epoch != *in.ExpectedSubmissionEpoch {
				return operation.ErrSubmissionEpoch
			}
			if slot.CreationID != nil {
				return operation.ErrSubmissionUnresolved
			}
			epoch = slot.Epoch + 1
		}
		source := creationSource(in)
		if in.Source != nil && in.Source.ShotID != nil {
			if !in.Source.Valid() {
				return domain.ErrInvalid
			}
			if err := checkStoryboardQuoteInput(tx, p, in); err != nil {
				return err
			}
			prompt, memoryVersion, err := storyboardMemoryPrompt(tx, p, in.Source.ShotID, in.Prompt)
			if err != nil {
				return err
			}
			in.Prompt = prompt
			source = canonical(storyboardQuoteSource{CanvasSource: *in.Source, MemoryVersion: memoryVersion})
		}
		model, raw, draft, err := c.quoteInput(ctx, tx, a, p, in)
		if err != nil {
			return err
		}
		call, err := application.Prepare(model, a.ID, a.ScopeID, p, c.Run, "creation/"+key, c.origin(), raw)
		if err != nil {
			return err
		}
		quote, err := c.store(tx, a).Quote(ctx, call)
		if err != nil {
			return err
		}
		confirm := uuid.New()
		details := map[string]any{"version": "gateway.v1", "unit": "credits", "maximum_credits": quote.Credits, "exact_credits": strconv.FormatInt(quote.Credits, 10), "model_version": model.ID}
		if call.EstimatedVideoTokens > 0 {
			price := call.Model.Price
			credits, err := call.VideoEstimatedCredits()
			if err != nil {
				return err
			}
			details["unit"] = "per_million_video_tokens"
			details["currency"] = price.Currency
			details["unit_price_micros"] = price.Output
			details["estimated_tokens"] = call.EstimatedVideoTokens
			details["estimated_credits"] = credits
			details["exact_credits"] = strconv.FormatInt(credits, 10)
			details["max_charge_credits"] = strconv.FormatInt(quote.Credits, 10)
		}
		detail := canonical(details)
		out = operation.CreateFreeQuoteResult{OperationID: quote.ID, QuoteMicros: quote.Credits * 100000, QuoteDetail: detail, AvailableMicros: quote.Available * 100000, ExpiresAt: quote.Expires, Confirmable: quote.Available >= quote.Credits, EditorDraft: draft, ConfirmKey: &confirm, FinalPrompt: nativePrompt(model, raw)}
		if draft != nil {
			compiled, _ := draft.Compile()
			out.FinalPrompt = compiled.Prompt
			out.CompilerVersion = promptdomain.CompilerVersion
			out.ReferenceMapping = compiled.Mapping
			out.SubmissionID = &quote.ID
			out.SubmissionEpoch = epoch
		}
		if err := tx.Exec(`INSERT INTO ai.creation(id,scope_id,actor_id,project_id,owner_key,epoch,source,draft,presentation,input_hash,actor,confirm_key) VALUES(?,?,?,?,?,?,?::jsonb,?::jsonb,?::jsonb,?,?::jsonb,?)`, quote.ID, a.ScopeID, a.ID, p, owner, epoch, string(source), string(canonical(draft)), string(canonical(out)), fingerprint, string(canonical(a)), confirm).Error; err != nil {
			return err
		}
		if owner != nil {
			return tx.Exec("UPDATE ai.creation_slot SET epoch=?,creation_id=? WHERE actor_id=? AND project_id=? AND owner_key=?", epoch, quote.ID, a.ID, p, owner).Error
		}
		return nil
	})
	return out, err
}

// Confirm freezes the single hold and enqueues the original request in one transaction.
func (c Creation) Confirm(ctx context.Context, a identity.Principal, p, id uuid.UUID, key string) (domain.Record, error) {
	var out domain.Record
	if key == "" || len(key) > 160 {
		return out, domain.ErrInvalid
	}
	if c.Runtime.Provider == nil {
		return out, domain.ErrUnavailable
	}
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.auth()(ctx, tx, a, a.ScopeID, p, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT id FROM ai.creation WHERE id=? FOR UPDATE", id).Error; err != nil {
			return err
		}
		row, err := c.row(tx, a, p, id)
		if err != nil {
			return err
		}
		if row.ActorID != a.ID {
			return domain.ErrForbidden
		}
		var source storyboardQuoteSource
		decodeErr := json.Unmarshal(row.Source, &source)
		storyboard := source.ShotID != nil || source.ShotRevision != nil
		if storyboard {
			if decodeErr != nil || !source.Valid() || row.OwnerKey != nil {
				return domain.ErrInvalid
			}
			if key != row.ConfirmKey.String() {
				return domain.ErrConflict
			}
			if row.State == "accepted" {
				out, err = c.store(tx, a).Read(ctx, a.ScopeID, id)
				return err
			}
		}
		if row.State == "closed" {
			var n int64
			if err := tx.Raw("SELECT count(*) FROM ai.request WHERE id=?", id).Scan(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				return domain.ErrConflict
			}
			out, err = c.store(tx, a).Read(ctx, a.ScopeID, id)
			return err
		}
		if row.OwnerKey != nil && key != row.ConfirmKey.String() {
			return domain.ErrConflict
		}
		if row.State != "accepted" && row.OwnerKey != nil {
			var source promptdomain.InputSource
			var frozen promptdomain.Draft
			if json.Unmarshal(row.Source, &source) != nil || json.Unmarshal(row.Draft, &frozen) != nil {
				return domain.ErrInvalid
			}
			current, err := operationwire.ProvidePromptEditor(tx).ReadInput(ctx, a, p, source, true)
			if err != nil {
				return err
			}
			if domain.Digest(canonical(current)) != domain.Digest(canonical(frozen)) {
				return domain.ErrConflict
			}
		}
		call, err := c.store(tx, a).LoadQuote(ctx, a.ScopeID, id)
		if err != nil {
			if storyboard && errors.Is(err, domain.ErrInvalid) {
				return operation.ErrQuoteStale
			}
			return err
		}
		if storyboard {
			if err := checkStoryboardConfirmation(tx, call, source); err != nil {
				return err
			}
		}
		if row.State != "accepted" {
			if err := c.Runtime.PinMediaOutput(ctx, tx, call); err != nil {
				return err
			}
		}
		call.Deadline = time.Now().Add(15 * time.Minute)
		out, err = c.store(tx, a).Queue(ctx, call)
		if err != nil {
			return err
		}
		if storyboard {
			var expired bool
			if err := tx.Raw("SELECT expires_at<=clock_timestamp() FROM ai.quote WHERE id=?", id).Row().Scan(&expired); err != nil {
				return err
			}
			if expired {
				return operation.ErrQuoteStale
			}
		}
		return tx.Exec("UPDATE ai.creation SET state='accepted' WHERE id=?", id).Error
	})
	return out, err
}

// Execute consumes one durable claim. A restarted worker can settle a receipt, never resend it.
func (c Creation) Execute(ctx context.Context, job taskruntime.Job) (time.Time, error) {
	id, err := uuid.Parse(job.BusinessID)
	if err != nil {
		return time.Time{}, err
	}
	var row struct {
		ID, ScopeID uuid.UUID
		Actor       []byte
		WorkerProof []byte
	}
	if err = c.Runtime.DB.WithContext(ctx).Raw("SELECT j.request_id AS id,r.scope_id,j.actor,j.worker_proof FROM ai.job j JOIN ai.request r ON r.id=j.request_id WHERE j.request_id=?", id).Scan(&row).Error; err != nil {
		return time.Now().Add(time.Second), err
	}
	if row.ID == uuid.Nil {
		return time.Time{}, nil
	}
	var a identity.Principal
	if json.Unmarshal(row.Actor, &a) != nil {
		return time.Time{}, domain.ErrInvalid
	}
	store := c.store(c.Runtime.DB, a)
	if a.SessionID == uuid.Nil && c.Authorize == nil {
		store = pg.New(c.Runtime.DB, a, AgentWorkerAuthority(row.WorkerProof))
	}
	call, err := store.LoadQuote(ctx, row.ScopeID, id)
	if err != nil {
		// Revoked sessions cannot buy a model. The durable ready claim is proof of no send.
		trusted := pg.New(c.Runtime.DB, a, func(context.Context, *gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, bool) error { return nil })
		var original struct{ Contract []byte }
		if e := c.Runtime.DB.WithContext(ctx).Raw("SELECT contract FROM ai.quote WHERE id=?", id).Scan(&original).Error; e != nil {
			return time.Now().Add(time.Second), e
		}
		if json.Unmarshal(original.Contract, &call) != nil {
			return time.Time{}, domain.ErrInvalid
		}
		_, e := trusted.CancelQueued(ctx, call)
		if e != nil && !errors.Is(e, domain.ErrUnknown) {
			return time.Now().Add(time.Second), e
		}
		if errors.Is(e, domain.ErrUnknown) {
			var attempt struct{ ID uuid.UUID }
			q := c.Runtime.DB.WithContext(ctx).Raw("SELECT id FROM ai.attempt WHERE request_id=? AND state='sending' ORDER BY number DESC LIMIT 1", id).Scan(&attempt)
			if q.Error != nil {
				return time.Now().Add(time.Second), q.Error
			}
			if q.RowsAffected == 1 {
				if err := trusted.Unknown(ctx, id, attempt.ID); err != nil {
					return time.Now().Add(time.Second), err
				}
			}
		}
		return time.Time{}, nil
	}
	// The request's persisted deadline is immutable; worker send duration remains bounded.
	if err = c.Runtime.DB.WithContext(ctx).Raw("SELECT deadline FROM ai.request WHERE id=?", id).Row().Scan(&call.Deadline); err != nil {
		return time.Now().Add(time.Second), err
	}
	d, err := store.ClaimQueued(ctx, call)
	if err != nil {
		if errors.Is(err, domain.ErrUnavailable) {
			_, e := store.CancelQueued(ctx, call)
			return time.Time{}, e
		}
		return time.Now().Add(time.Second), err
	}
	if !d.Send {
		if d.Record.State == "sending" && d.Attempt != uuid.Nil {
			if err := store.Unknown(ctx, id, d.Attempt); err != nil {
				return time.Now().Add(time.Second), err
			}
		}
		if d.Record.State == "received" {
			_, err = store.Settle(ctx, id)
			if err != nil {
				return time.Now().Add(time.Second), err
			}
		}
		return time.Time{}, nil
	}
	_, err = application.New(store, c.Runtime.Provider).ExecuteDispatch(ctx, d, nil)
	if err != nil && !errors.Is(err, domain.ErrUnknown) && !errors.Is(err, domain.ErrUnavailable) {
		return time.Now().Add(time.Second), err
	}
	return time.Time{}, nil
}
