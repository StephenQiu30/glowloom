package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	catalogtask "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// RequestCredentialTest commits a safe test request without reading ciphertext.
func (s *Store) RequestCredentialTest(ctx context.Context, actor identityapp.Principal, request application.CredentialTestRequest) (application.CredentialTestAccepted, error) {
	result := application.CredentialTestAccepted{}
	if s == nil || s.db == nil || request.TestID == uuid.Nil || actor.ID != request.ActorID || actor.OrgID != request.OrgID {
		return result, application.ErrInvalidCredentialTest
	}
	requestID, err := uuid.Parse(request.RequestID)
	if err != nil || requestID == uuid.Nil {
		return result, application.ErrInvalidCredentialTest
	}
	if request.CredentialID == uuid.Nil && request.Kind != "reachability" {
		return result, application.ErrInvalidCredentialTest
	}
	identity := "credential-test/" + actor.OrgID.String() + "/" + request.ProviderID.String() + "/" + request.CredentialID.String()
	if request.ModelID != uuid.Nil {
		identity += "/model/" + request.ModelID.String()
	}
	if request.Kind == "reachability" || request.Kind == "discovery" {
		identity += "/" + request.Kind
	}
	if request.Kind == "model_query" {
		identity += "/query/" + request.ParentCheckID.String()
	}
	digest := sha256.Sum256([]byte(identity))
	hash := hex.EncodeToString(digest[:])
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, actor.ID.String()+"/"+requestID.String()).Error; err != nil {
			return fmt.Errorf("lock credential test request: %w", err)
		}
		var receipt struct {
			RequestHash, ResponseBody string
			StatusCode                int
		}
		row := tx.Raw(`SELECT request_hash,response_body::text AS response_body,status_code FROM infra.idempotency_record WHERE actor_id=?::uuid AND idem_key=?`, actor.ID, requestID.String()).Scan(&receipt)
		if row.Error != nil {
			return fmt.Errorf("read credential test receipt: %w", row.Error)
		}
		if row.RowsAffected == 1 {
			if receipt.RequestHash != hash || receipt.StatusCode != 202 {
				return application.ErrCredentialTestKeyReused
			}
			if err := json.Unmarshal([]byte(receipt.ResponseBody), &result); err != nil {
				return fmt.Errorf("decode credential test receipt: %w", err)
			}
			return nil
		}
		if err := requireCredentialProvider(tx, request.ProviderID); err != nil {
			return err
		}
		kind := "credential_auth"
		var credentialID *uuid.UUID
		if request.Kind == "reachability" {
			kind = "reachability"
		} else {
			credentialID = &request.CredentialID
			var found int
			row = tx.Raw(`SELECT 1 FROM catalog.provider p JOIN catalog.provider_credential c ON c.provider_id=p.id WHERE p.id=? AND c.id=? AND (?='model_query' OR p.status<>'archived') AND (p.status='active' OR p.current_config_version_id IS NOT NULL) AND (c.status='active' OR (?='model_query' AND c.status='draining')) AND NOT p.is_delete AND NOT c.is_delete FOR SHARE OF p,c`, request.ProviderID, request.CredentialID, request.Kind, request.Kind).Scan(&found)
			if row.Error != nil {
				return row.Error
			}
			if row.RowsAffected != 1 {
				return application.ErrCredentialNotFound
			}
		}
		if request.Kind == "discovery" {
			kind = "discovery"
		}
		if request.ModelID != uuid.Nil {
			kind = "model_access"
		}
		if request.Kind == "model_query" {
			kind = "model_query"
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, request.ProviderID.String()+"/test/"+kind).Error; err != nil {
			return err
		}
		var inFlight struct {
			ID      uuid.UUID
			ModelID *uuid.UUID
		}
		row = tx.Raw(`SELECT check_run.id,check_run.model_id FROM catalog.provider_check check_run JOIN catalog.provider p ON p.id=check_run.provider_id AND (p.current_config_version_id=check_run.config_version_id OR check_run.kind='model_query')
 WHERE check_run.provider_id=? AND check_run.credential_id IS NOT DISTINCT FROM ?::uuid AND check_run.kind=? AND (?<>'model_query' OR check_run.parent_check_id=?::uuid) AND
 (check_run.status IN ('pending','running') OR (check_run.kind='model_access' AND check_run.model_result IN ('unknown','accepted') AND check_run.tracking_ended_at IS NULL)) ORDER BY check_run.create_time DESC LIMIT 1`, request.ProviderID, credentialID, kind, kind, request.ParentCheckID).Scan(&inFlight)
		if row.Error != nil {
			return row.Error
		}
		saveReceipt := func() error {
			body, err := json.Marshal(struct {
				application.CredentialTestAccepted
				PlatformEpoch int64 `json:"platform_epoch"`
			}{result, actor.PlatformEpoch})
			if err != nil {
				return err
			}
			return tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at) VALUES(?,?,?,?,202,?::jsonb,?)`, uuid.New(), actor.ID, requestID.String(), hash, string(body), time.Now().UTC().Add(24*time.Hour)).Error
		}

		var parentID *uuid.UUID
		taskID := ""
		var pinned struct {
			ConfigVersionID, ModelVersionID uuid.UUID
			TaskID                          string
		}
		if kind == "model_query" {
			parentID = &request.ParentCheckID
			row = tx.Raw(`SELECT task_id,config_version_id,model_version_id FROM catalog.provider_check WHERE id=? AND provider_id=? AND credential_id=? AND model_id=? AND kind='model_access' AND task_id<>'' AND model_result IN ('accepted','unknown') FOR UPDATE`, request.ParentCheckID, request.ProviderID, request.CredentialID, request.ModelID).Scan(&pinned)
			if row.Error != nil {
				return row.Error
			}
			if row.RowsAffected != 1 {
				return application.ErrInvalidCredentialTest
			}
			if err := tx.Exec(`UPDATE catalog.provider_check SET status='running' WHERE id=?`, request.ParentCheckID).Error; err != nil {
				return err
			}
		}
		var modelID, versionID *uuid.UUID
		if kind == "model_query" {
			modelID, versionID = &request.ModelID, &pinned.ModelVersionID
			taskID = pinned.TaskID
		} else if request.ModelID != uuid.Nil {
			var version struct{ ID uuid.UUID }
			r := tx.Raw(`SELECT v.id FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.id=m.current_version_id
 JOIN catalog.model_release release ON release.model_id=m.id AND release.model_version_id=v.id
 JOIN catalog.provider p ON p.id=m.provider_id AND p.current_config_version_id=release.config_version_id
 WHERE m.id=? AND m.provider_id=? AND m.status<>'archived' AND NOT m.is_delete AND m.candidate_snapshot IS NOT NULL
 AND (release.status='draft' OR (release.status='active' AND m.active_release_id=release.id))
 AND release.upstream_cost='{}'::jsonb AND NOT v.is_delete FOR SHARE OF m,v,p,release`, request.ModelID, request.ProviderID).Scan(&version)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return application.ErrInvalidCredentialTest
			}
			modelID, versionID = &request.ModelID, &version.ID
		}
		if inFlight.ID != uuid.Nil {
			result = application.CredentialTestAccepted{Kind: kind, TestID: inFlight.ID, CheckID: inFlight.ID, EventID: uuid.NewSHA1(inFlight.ID, []byte("credential_test_requested")), ProviderID: request.ProviderID, CredentialID: request.CredentialID, Accepted: true, ModelID: inFlight.ModelID}
			return saveReceipt()
		}
		if kind == "model_access" {
			var previous struct{ ID uuid.UUID }
			if err := tx.Raw(`SELECT c.id FROM catalog.provider_check c JOIN catalog.provider p ON p.id=c.provider_id AND p.current_config_version_id=c.config_version_id
 WHERE c.provider_id=? AND c.credential_id=? AND c.model_id=? AND c.kind='model_access' AND c.model_result='unknown' AND c.tracking_ended_at IS NOT NULL
 ORDER BY c.create_time DESC,c.id DESC LIMIT 1`, request.ProviderID, request.CredentialID, request.ModelID).Scan(&previous).Error; err != nil {
				return err
			}
			if previous.ID != uuid.Nil {
				parentID = &previous.ID
			}
		}
		if kind != "model_query" {
			if s.checkUsage == nil {
				return application.ErrInvalidCredentialTest
			}
			var limits struct{ ConcurrencyLimit, RateLimitPerMin int64 }
			if err := tx.Raw(`SELECT concurrency_limit,rate_limit_per_min FROM catalog.provider WHERE id=?`, request.ProviderID).Scan(&limits).Error; err != nil {
				return err
			}
			active, recent, err := s.checkUsage(tx).ProviderUsage(ctx, request.ProviderID)
			if err != nil {
				return err
			}
			if active >= limits.ConcurrencyLimit || recent >= limits.RateLimitPerMin {
				return application.ErrProviderCheckCapacity
			}
		}
		if err := tx.Exec(`INSERT INTO catalog.provider_check(id,actor_id,provider_id,config_version_id,credential_id,kind,status,model_id,model_version_id,request_id,parent_check_id,task_id)
 SELECT ?,?,p.id,COALESCE(NULLIF(?::uuid,'00000000-0000-0000-0000-000000000000'::uuid),p.current_config_version_id),?,?,'pending',?,?,?,?,? FROM catalog.provider p WHERE p.id=? AND p.current_config_version_id IS NOT NULL`, request.TestID, actor.ID, pinned.ConfigVersionID, credentialID, kind, modelID, versionID, requestID, parentID, taskID, request.ProviderID).Error; err != nil {
			return err
		}
		eventID := uuid.NewSHA1(request.TestID, []byte("credential_test_requested"))
		now := time.Now().UTC()
		const topic = "glowloom.catalog.credential_test_requested.v1"
		aggregateType, aggregateID := "provider_credential", request.CredentialID
		if kind == "reachability" {
			aggregateType, aggregateID = "provider", request.ProviderID
		}
		payload, err := json.Marshal(map[string]any{"event_id": eventID, "event_type": topic, "occurred_at": now, "org_id": actor.OrgID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": aggregateType, "id": aggregateID}, "data": map[string]any{"test_id": request.TestID, "provider_id": request.ProviderID, "credential_id": request.CredentialID, "request_id": request.RequestID, "kind": kind}})
		if err != nil {
			return fmt.Errorf("encode credential test request: %w", err)
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?::uuid,?,?,?::jsonb)`, eventID, topic, actor.OrgID.String(), string(payload)).Error; err != nil {
			return fmt.Errorf("record credential test request: %w", err)
		}
		ref := catalogtask.Input{Kind: kind, TestID: request.TestID, ProviderID: request.ProviderID, CredentialID: request.CredentialID, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: request.RequestID}
		if err := taskruntime.EnqueueReference(ctx, tx, catalogtask.Credential, request.TestID.String(), now, ref); err != nil {
			return err
		}
		result = application.CredentialTestAccepted{Kind: kind, TestID: request.TestID, CheckID: request.TestID, EventID: eventID, ProviderID: request.ProviderID, CredentialID: request.CredentialID, Accepted: true, ModelID: modelID}
		return saveReceipt()
	})
	return result, err
}

// VerifyCredentialTestRequest rejects injected events without an accepted receipt.
func (s *Store) VerifyCredentialTestRequest(ctx context.Context, request application.CredentialTestRequest, eventID uuid.UUID) error {
	if s == nil || s.db == nil || eventID == uuid.Nil {
		return application.ErrInvalidCredentialTest
	}
	requestID, err := uuid.Parse(request.RequestID)
	if err != nil || requestID.String() != request.RequestID || request.TestID != uuid.NewSHA1(requestID, []byte(request.ActorID.String()+request.OrgID.String()+request.ProviderID.String()+request.CredentialID.String())) || eventID != uuid.NewSHA1(request.TestID, []byte("credential_test_requested")) {
		return application.ErrInvalidCredentialTest
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ Body string }
		result := tx.Raw(`SELECT response_body::text AS body FROM infra.idempotency_record WHERE actor_id=?::uuid AND idem_key=? AND status_code=202 AND NOT is_delete FOR SHARE`, request.ActorID, request.RequestID).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read accepted credential test: %w", result.Error)
		}
		var accepted application.CredentialTestAccepted
		if result.RowsAffected != 1 || json.Unmarshal([]byte(row.Body), &accepted) != nil || !accepted.Accepted || accepted.EventID != eventID || accepted.TestID != request.TestID || accepted.ProviderID != request.ProviderID || accepted.CredentialID != request.CredentialID {
			return application.ErrInvalidCredentialTest
		}
		if s.actors == nil || s.actors(tx) == nil {
			return fmt.Errorf("catalog authority unavailable")
		}
		err := s.requireWorkerPlatform(tx, request)
		if errors.Is(err, identityapp.ErrForbidden) {
			return application.ErrCredentialTestRevoked
		}
		return err
	})
}
