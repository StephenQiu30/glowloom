// Package postgres persists the rebuilt AI facts and one funds lifecycle.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// Authority retains live Identity/Creation locks in the same transaction.
type Authority func(context.Context, *gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, bool) error

// Store never delegates model consumption to legacy operation or billing tables.
type Store struct {
	workerPermit WorkerPermit
	runLimit     RunLimit
	grant        uuid.UUID
	db           *gorm.DB
	actor        identity.Principal
	authorize    Authority
}

// New binds each business transaction to the initiating actor and authority.
func New(db *gorm.DB, actor identity.Principal, authorize Authority) *Store {
	return &Store{db: db, actor: actor, authorize: authorize}
}
func (s *Store) check(ctx context.Context, tx *gorm.DB, scope, project uuid.UUID, admin bool) error {
	if s.authorize == nil || s.actor.ID == uuid.Nil {
		return domain.ErrForbidden
	}
	return s.authorize(ctx, tx, s.actor, scope, project, admin)
}

// Model resolves the current published version for an exact name and protocol.
func (s *Store) Model(ctx context.Context, name, protocol string) (domain.Model, error) {
	var row struct{ Document []byte }
	r := s.db.WithContext(ctx).Raw("SELECT document FROM ai.model WHERE name=? AND document->>'protocol'=? AND enabled", name, protocol).Scan(&row)
	if r.Error != nil {
		return domain.Model{}, r.Error
	}
	var m domain.Model
	if r.RowsAffected != 1 || json.Unmarshal(row.Document, &m) != nil || m.Validate() != nil || m.Protocol != protocol {
		return m, domain.ErrUnavailable
	}
	return m, nil
}
func scanRecord(tx *gorm.DB, id uuid.UUID) (domain.Record, error) {
	var row struct {
		ID, Scope, ModelID      uuid.UUID
		State                   string
		Held, Charged, Released int64
		Receipt                 []byte
		Created                 time.Time
	}
	r := tx.Raw(`SELECT r.id,r.scope_id AS scope,r.model_id,r.state,h.amount AS held,h.charged,h.released,r.created_at AS created,
 (SELECT a.receipt FROM ai.attempt a WHERE a.request_id=r.id AND a.receipt IS NOT NULL ORDER BY number DESC LIMIT 1) AS receipt
 FROM ai.request r JOIN ai.hold h ON h.request_id=r.id WHERE r.id=?`, id).Scan(&row)
	if r.Error != nil {
		return domain.Record{}, r.Error
	}
	if r.RowsAffected != 1 {
		return domain.Record{}, domain.ErrUnavailable
	}
	result := domain.Record{ID: row.ID, Scope: row.Scope, ModelID: row.ModelID, State: row.State, Held: row.Held, Charged: row.Charged, Released: row.Released, Created: row.Created}
	if len(row.Receipt) > 0 {
		var receipt domain.Receipt
		if json.Unmarshal(row.Receipt, &receipt) != nil {
			return result, domain.ErrInvalid
		}
		result.Receipt = &receipt
	}
	return result, nil
}

// Read repeats live scope authorization; possession of a UUID gives no access.
func (s *Store) Read(ctx context.Context, scope, id uuid.UUID) (domain.Record, error) {
	var record domain.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, scope, uuid.Nil, false); err != nil {
			return err
		}
		var err error
		record, err = scanRecord(tx, id)
		if err == nil && record.Scope != scope {
			return domain.ErrForbidden
		}
		return err
	})
	return record, err
}

// Grant is an explicit audited administrator event, never implicit account funding.
func (s *Store) Grant(ctx context.Context, scope uuid.UUID, key string, credits int64) error {
	if scope == uuid.Nil || key == "" || len(key) > 120 || credits < 1 || credits > 1000000000 {
		return domain.ErrInvalid
	}
	hash := domain.Digest([]byte(fmt.Sprintf("%s/%d", scope, credits)))
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, scope, uuid.Nil, true); err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO ai.account(scope_id) VALUES(?) ON CONFLICT DO NOTHING", scope).Error; err != nil {
			return err
		}
		var version int64
		if err := tx.Raw("SELECT version FROM ai.account WHERE scope_id=? FOR UPDATE", scope).Row().Scan(&version); err != nil {
			return err
		}
		var prior string
		r := tx.Raw("SELECT source_hash FROM ai.money_event WHERE scope_id=? AND event_key=?", scope, "grant/"+key).Scan(&prior)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 {
			if prior != hash {
				return domain.ErrConflict
			}
			return nil
		}
		if err := tx.Exec(`INSERT INTO ai.money_event(id,scope_id,actor_id,source_reference,event_key,kind,available_delta,held_delta,spent_delta,source_hash) VALUES(?,?,?,?,?,'grant',?,0,0,?)`, uuid.New(), scope, s.actor.ID, key, "grant/"+key, credits, hash).Error; err != nil {
			return err
		}
		r = tx.Exec("UPDATE ai.account SET available=available+?,version=version+1 WHERE scope_id=? AND version=? AND available+held<=92233720368547-?", credits, scope, version, credits)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrConflict
		}
		return nil
	})
}

// Admit commits request identity, one hold and one independently audited attempt.
func (s *Store) Admit(ctx context.Context, call domain.Call) (application.Dispatch, error) {
	var result application.Dispatch
	if call.ID == uuid.Nil || call.Actor != s.actor.ID || call.Scope == uuid.Nil || call.Key == "" || len(call.Key) > 160 || len(call.Hash) != 64 || call.Limit < 0 || call.Deadline.Before(time.Now()) || call.Model.Validate() != nil {
		return result, domain.ErrInvalid
	}
	validated, validateErr := application.Prepare(call.Model, call.Actor, call.Scope, call.Project, call.Run, call.Key, call.Origin, call.Raw)
	if validateErr != nil || validated.Hash != call.Hash || validated.Limit != call.Limit || validated.Stream != call.Stream {
		return result, domain.ErrInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, call.Scope, call.Project, false); err != nil {
			return err
		}
		if s.grant != uuid.Nil {
			var present int
			r := tx.Raw(`SELECT 1 FROM ai.integration_grant WHERE id=? AND scope_id=? AND NOT revoked AND expires_at>clock_timestamp() AND model_ids @> ?::jsonb FOR SHARE`, s.grant, call.Scope, string(mustJSON([]uuid.UUID{call.Model.ID}))).Scan(&present)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 || call.Run != s.grant {
				return domain.ErrForbidden
			}
		}

		if err := tx.Exec("INSERT INTO ai.account(scope_id) VALUES(?) ON CONFLICT DO NOTHING", call.Scope).Error; err != nil {
			return err
		}
		var account struct{ Available, Held, Version int64 }
		if err := tx.Raw("SELECT available,held,version FROM ai.account WHERE scope_id=? FOR UPDATE", call.Scope).Scan(&account).Error; err != nil {
			return err
		}
		var prior struct {
			ID        uuid.UUID
			InputHash string
		}
		r := tx.Raw("SELECT id,input_hash FROM ai.request WHERE scope_id=? AND idempotency_key=?", call.Scope, call.Key).Scan(&prior)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			if prior.InputHash != call.Hash {
				return domain.ErrConflict
			}
			record, err := scanRecord(tx, prior.ID)
			result = application.Dispatch{Call: call, Record: record}
			result.Call.ID = prior.ID
			return err
		}
		var current struct{ Document []byte }
		r = tx.Raw("SELECT document FROM ai.model m WHERE id=? AND (enabled OR EXISTS(SELECT 1 FROM ai.quote q WHERE q.id=? AND q.scope_id=? AND q.input_hash=? AND q.contract->'Model'=m.document AND q.expires_at>clock_timestamp())) FOR SHARE", call.Model.ID, call.ID, call.Scope, call.Hash).Scan(&current)
		if r.Error != nil {
			return r.Error
		}
		actual, _ := json.Marshal(call.Model)
		var a, b any
		if r.RowsAffected != 1 || json.Unmarshal(current.Document, &a) != nil || json.Unmarshal(actual, &b) != nil || domain.Digest(mustJSON(a)) != domain.Digest(mustJSON(b)) {
			return domain.ErrUnavailable
		}
		if account.Available < call.Limit {
			return domain.ErrFunds
		}
		contract, err := json.Marshal(call.Model)
		if err != nil {
			return err
		}
		if err := s.syncRunLimit(ctx, tx, call); err != nil {
			return err
		}
		if err := budgetChange(tx, call, call.Limit, 0); err != nil {
			return err
		}
		r = tx.Exec("UPDATE ai.account SET available=?,held=?,version=version+1 WHERE scope_id=? AND version=?", account.Available-call.Limit, account.Held+call.Limit, call.Scope, account.Version)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrConflict
		}
		if err := tx.Exec(`INSERT INTO ai.request(id,scope_id,actor_id,project_id,run_id,origin,idempotency_key,input_hash,model_id,contract,stream,deadline,state) VALUES(?,?,?,?,?,?,?,?,?,?::jsonb,?,?,'sending')`, call.ID, call.Scope, call.Actor, nullable(call.Project), nullable(call.Run), call.Origin, call.Key, call.Hash, call.Model.ID, string(contract), call.Stream, call.Deadline).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO ai.hold(request_id,amount) VALUES(?,?)", call.ID, call.Limit).Error; err != nil {
			return err
		}
		if err := money(tx, call, "freeze", -call.Limit, call.Limit, 0); err != nil {
			return err
		}
		attempt, route, err := selectRoute(tx, call, uuid.Nil, 1)
		if err != nil {
			return err
		}
		result = application.Dispatch{Call: call, Attempt: attempt, Route: route, Send: true, Record: domain.Record{ID: call.ID, Scope: call.Scope, ModelID: call.Model.ID, State: "sending", Held: call.Limit}}
		return nil
	})
	return result, err
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func nullable(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
func money(tx *gorm.DB, call domain.Call, kind string, available, held, spent int64) error {
	return tx.Exec(`INSERT INTO ai.money_event(id,scope_id,actor_id,request_id,event_key,kind,available_delta,held_delta,spent_delta,source_hash) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.New(), call.Scope, call.Actor, call.ID, kind+"/"+call.ID.String(), kind, available, held, spent, call.Hash).Error
}
func budgetChange(tx *gorm.DB, call domain.Call, hold, spent int64) error {
	ids := []uuid.UUID{}
	if call.Project != uuid.Nil {
		ids = append(ids, call.Project)
	}
	if call.Run != uuid.Nil && call.Run != call.Project {
		ids = append(ids, call.Run)
	}
	if len(ids) == 0 {
		return nil
	}
	var rows []struct {
		ID                        uuid.UUID
		LimitCredits, Held, Spent int64
	}
	if err := tx.Raw("SELECT id,limit_credits,held,spent FROM ai.budget WHERE scope_id=? AND id IN ? ORDER BY id FOR UPDATE", call.Scope, ids).Scan(&rows).Error; err != nil {
		return err
	}
	if call.Run != uuid.Nil {
		found := false
		for _, row := range rows {
			if row.ID == call.Run {
				found = true
			}
		}
		if !found {
			return domain.ErrBudget
		}
	}
	for _, row := range rows {
		if row.Held+hold < 0 || row.Spent+spent < 0 || row.Held+hold+row.Spent+spent > row.LimitCredits {
			return domain.ErrBudget
		}
		if err := tx.Exec("UPDATE ai.budget SET held=?,spent=? WHERE scope_id=? AND id=?", row.Held+hold, row.Spent+spent, call.Scope, row.ID).Error; err != nil {
			return err
		}
	}
	return nil
}
func selectRoute(tx *gorm.DB, call domain.Call, excluded uuid.UUID, number int) (uuid.UUID, domain.Route, error) {
	var rows []struct {
		ID, CredentialID uuid.UUID
		QuotaGroup       string
		Document         []byte
		KeyID            string
		SupplierID       uuid.UUID
		Ciphertext       []byte
	}
	if err := tx.Exec(`SELECT q.name FROM ai.quota q WHERE q.name IN(SELECT quota_group FROM ai.route WHERE model_id=?) ORDER BY q.name FOR UPDATE`, call.Model.ID).Error; err != nil {
		return uuid.Nil, domain.Route{}, err
	}
	err := tx.Raw(`SELECT r.id,r.credential_id,r.quota_group,r.document,c.key_id,c.supplier_id,c.ciphertext FROM ai.route r
 JOIN ai.credential c ON c.id=r.credential_id AND c.state='active' JOIN ai.quota q ON q.name=r.quota_group
 WHERE r.model_id=? AND r.enabled AND r.id<>? AND (q.cooldown_until IS NULL OR q.cooldown_until<clock_timestamp())
 AND (SELECT count(*) FROM ai.attempt a WHERE a.quota_group=q.name AND a.state IN ('ready','sending','pending','unknown'))<q.concurrency
 AND (SELECT count(*) FROM ai.attempt a WHERE a.quota_group=q.name AND a.started_at>clock_timestamp()-interval '1 minute')<q.requests_per_minute
 ORDER BY r.priority,r.id FOR SHARE OF r,c`, call.Model.ID, excluded).Scan(&rows).Error
	if err != nil {
		return uuid.Nil, domain.Route{}, err
	}
	for _, row := range rows {
		var route domain.Route
		if json.Unmarshal(row.Document, &route) != nil || route.ID != row.ID || route.ModelID != call.Model.ID || route.Credential != row.CredentialID || route.Supplier != row.SupplierID || route.Group != row.QuotaGroup || route.Protocol != call.Model.Protocol || route.Purchase.Validate() != nil {
			continue
		}
		// Reject a route before freezing dispatch if its published worst case exceeds procurement policy.
		highest := route.Purchase.Input
		if route.Purchase.CacheRead > highest {
			highest = route.Purchase.CacheRead
		}
		if route.Purchase.CacheWrite > highest {
			highest = route.Purchase.CacheWrite
		}
		bound := route.Purchase
		bound.Input = highest
		cost, err := bound.Cost(domain.Usage{Input: call.Model.MaxInput, Output: call.Model.MaxOutput})
		if err != nil || cost > call.Model.PurchaseCap {
			continue
		}
		route.KeyID = row.KeyID
		route.Ciphertext = row.Ciphertext
		raw, _ := json.Marshal(route)
		id := uuid.New()
		if err := tx.Exec(`INSERT INTO ai.attempt(id,request_id,number,route_id,credential_id,quota_group,contract,state) VALUES(?,?,?,?,?,?,?::jsonb,'sending')`, id, call.ID, number, route.ID, route.Credential, route.Group, string(raw)).Error; err != nil {
			return uuid.Nil, domain.Route{}, err
		}
		return id, route, nil
	}
	return uuid.Nil, domain.Route{}, domain.ErrUnavailable
}

// Unknown never frees credits or reopens a send, including client cancellation.
func (s *Store) Unknown(ctx context.Context, request, attempt uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE ai.request SET state='unknown',updated_at=clock_timestamp() WHERE id=? AND state IN ('sending','pending')", request).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE ai.attempt SET state='unknown' WHERE id=? AND request_id=? AND state IN ('sending','pending')", attempt, request).Error
	})
}

// SaveReceipt is separate from settlement: a money failure must not lose provider evidence.
func (s *Store) SaveReceipt(ctx context.Context, request, attempt uuid.UUID, receipt domain.Receipt) error {
	if receipt.Usage.Validate() != nil || receipt.PurchaseMicros < 0 {
		return domain.ErrInvalid
	}
	raw, err := json.Marshal(receipt)
	if err != nil || len(raw) > 32<<20 {
		return domain.ErrInvalid
	}
	hash := domain.Digest(raw)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT id FROM ai.request WHERE id=? FOR UPDATE", request).Error; err != nil {
			return err
		}
		var row struct {
			Receipt  []byte
			Digest   string
			State    string
			Contract []byte
		}
		r := tx.Raw("SELECT receipt,digest,state,contract FROM ai.attempt WHERE id=? AND request_id=? FOR UPDATE", attempt, request).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrConflict
		}
		if len(row.Receipt) > 0 {
			if row.Digest != hash {
				return domain.ErrConflict
			}
			return nil
		}
		if row.State != "sending" && row.State != "pending" && row.State != "unknown" {
			return domain.ErrConflict
		}
		var route domain.Route
		if json.Unmarshal(row.Contract, &route) != nil {
			return domain.ErrInvalid
		}
		cost, err := route.Purchase.Cost(receipt.Usage)
		if err != nil || cost != receipt.PurchaseMicros {
			return domain.ErrInvalid
		}
		if err := tx.Exec("UPDATE ai.attempt SET receipt=?::jsonb,digest=?,state='received' WHERE id=?", string(raw), hash, attempt).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE ai.request SET state='received',updated_at=clock_timestamp() WHERE id=? AND state IN ('sending','pending','unknown')", request).Error; err != nil {
			return err
		}
		return taskruntime.Enqueue(ctx, tx, "ai.settle", request.String(), time.Now())
	})
}

// Settle uses the frozen sale, one hold and one settlement identity.
func (s *Store) Settle(ctx context.Context, id uuid.UUID) (domain.Record, error) {
	var record domain.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			ScopeID, ActorID uuid.UUID
			ProjectID, RunID *uuid.UUID
			InputHash        string
			Contract         []byte
		}
		r := tx.Raw("SELECT scope_id,actor_id,project_id,run_id,input_hash,contract FROM ai.request WHERE id=?", id).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrUnavailable
		}
		var account struct{ Available, Held, Version int64 }
		if err := tx.Raw("SELECT available,held,version FROM ai.account WHERE scope_id=? FOR UPDATE", row.ScopeID).Scan(&account).Error; err != nil {
			return err
		}
		if err := tx.Exec("SELECT id FROM ai.request WHERE id=? FOR UPDATE", id).Error; err != nil {
			return err
		}
		var err error
		record, err = scanRecord(tx, id)
		if err != nil {
			return err
		}
		if record.State == "settled" || record.State == "released" {
			return nil
		}
		if record.Receipt == nil {
			return domain.ErrUnknown
		}
		var model domain.Model
		if json.Unmarshal(row.Contract, &model) != nil {
			return domain.ErrInvalid
		}
		micros, err := model.Price.Cost(record.Receipt.Usage)
		if err != nil {
			return err
		}
		charge, err := domain.Credits(micros)
		if err != nil {
			return err
		}
		if charge > record.Held {
			charge = record.Held
		}
		released := record.Held - charge
		call := domain.Call{ID: id, Scope: row.ScopeID, Actor: row.ActorID, Hash: row.InputHash}
		if row.ProjectID != nil {
			call.Project = *row.ProjectID
		}
		if row.RunID != nil {
			call.Run = *row.RunID
		}
		if err := budgetChange(tx, call, -record.Held, charge); err != nil {
			return err
		}
		if account.Held < record.Held {
			return domain.ErrConflict
		}
		if err := money(tx, call, "settle", 0, -charge, charge); err != nil {
			return err
		}
		if err := money(tx, call, "release", released, -released, 0); err != nil {
			return err
		}
		r = tx.Exec("UPDATE ai.account SET available=?,held=?,version=version+1 WHERE scope_id=? AND version=?", account.Available+released, account.Held-record.Held, call.Scope, account.Version)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrConflict
		}
		if err := tx.Exec("UPDATE ai.hold SET charged=?,released=?,closed=true WHERE request_id=? AND NOT closed", charge, released, id).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE ai.request SET state='settled',updated_at=clock_timestamp() WHERE id=?", id).Error; err != nil {
			return err
		}
		if model.Protocol == "media" {
			var flags struct {
				ReconciliationOnly bool `json:"reconciliation_only"`
			}
			_ = json.Unmarshal(record.Receipt.Body, &flags)
			if !flags.ReconciliationOnly {
				if err := taskruntime.Enqueue(ctx, tx, "ai.deliver", id.String(), time.Now()); err != nil {
					return err
				}
			}
		}
		record.State = "settled"
		record.Charged = charge
		record.Released = released
		return nil
	})
	return record, err
}

// Backup is reachable only after affirmative not-sent evidence from our provider.
func (s *Store) Backup(ctx context.Context, failed application.Dispatch) (application.Dispatch, error) {
	next := failed
	next.Send = false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := verifyCallIdentity(tx, failed.Call); err != nil {
			return err
		}
		var state string
		r := tx.Raw("SELECT state FROM ai.request WHERE id=? AND deadline>clock_timestamp() FOR UPDATE", failed.Call.ID).Scan(&state)
		if r.Error != nil {
			return r.Error
		}
		if state != "sending" {
			return domain.ErrUnknown
		}
		r = tx.Exec("UPDATE ai.attempt SET state='not_sent' WHERE id=? AND request_id=? AND number=1 AND state='sending' AND receipt IS NULL", failed.Attempt, failed.Call.ID)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrUnknown
		}
		if err := s.check(ctx, tx, failed.Call.Scope, failed.Call.Project, false); err != nil {
			return err
		}
		id, route, err := selectRoute(tx, failed.Call, failed.Route.ID, 2)
		if err != nil {
			return err
		}
		next.Attempt = id
		next.Route = route
		next.Send = true
		return nil
	})
	return next, err
}

var _ application.Store = (*Store)(nil)

// ReleaseNotSent is used only when our transport proves no request was sent.
func (s *Store) ReleaseNotSent(ctx context.Context, d application.Dispatch) (domain.Record, error) {
	var out domain.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := verifyCallIdentity(tx, d.Call); err != nil {
			return err
		}
		var account struct{ Available, Held, Version int64 }
		if err := tx.Raw("SELECT available,held,version FROM ai.account WHERE scope_id=? FOR UPDATE", d.Call.Scope).Scan(&account).Error; err != nil {
			return err
		}
		if err := tx.Exec("SELECT id FROM ai.request WHERE id=? FOR UPDATE", d.Call.ID).Error; err != nil {
			return err
		}
		r := tx.Exec("UPDATE ai.attempt SET state='not_sent' WHERE id=? AND request_id=? AND state='sending' AND receipt IS NULL", d.Attempt, d.Call.ID)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrUnknown
		}
		var pending int64
		if err := tx.Raw("SELECT count(*) FROM ai.attempt WHERE request_id=? AND state<>'not_sent'", d.Call.ID).Scan(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return domain.ErrUnknown
		}
		var err error
		out, err = scanRecord(tx, d.Call.ID)
		if err != nil {
			return err
		}
		if account.Held < out.Held {
			return domain.ErrConflict
		}
		if err := budgetChange(tx, d.Call, -out.Held, 0); err != nil {
			return err
		}
		if err := money(tx, d.Call, "release", out.Held, -out.Held, 0); err != nil {
			return err
		}
		if err := tx.Exec("UPDATE ai.account SET available=?,held=?,version=version+1 WHERE scope_id=? AND version=?", account.Available+out.Held, account.Held-out.Held, d.Call.Scope, account.Version).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE ai.hold SET released=amount,closed=true WHERE request_id=?", d.Call.ID).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE ai.request SET state='released',updated_at=clock_timestamp() WHERE id=?", d.Call.ID).Error; err != nil {
			return err
		}
		out.Released = out.Held
		out.State = "released"
		return nil
	})
	return out, err
}
