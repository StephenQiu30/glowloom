package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
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
	digest := sha256.Sum256([]byte("credential-test/" + actor.OrgID.String() + "/" + request.ProviderID.String() + "/" + request.CredentialID.String()))
	hash := hex.EncodeToString(digest[:])
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actor.ID, actor.OrgID); err != nil {
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
		var found int
		row = tx.Raw(`SELECT 1 FROM catalog.provider p JOIN catalog.provider_credential c ON c.provider_id=p.id WHERE p.id=?::uuid AND c.id=?::uuid AND p.status='active' AND c.status='active' AND NOT p.is_delete AND NOT c.is_delete FOR SHARE OF p,c`, request.ProviderID, request.CredentialID).Scan(&found)
		if row.Error != nil {
			return fmt.Errorf("authorize credential test target: %w", row.Error)
		}
		if row.RowsAffected != 1 {
			return application.ErrCredentialNotFound
		}
		eventID := uuid.NewSHA1(request.TestID, []byte("credential_test_requested"))
		now := time.Now().UTC()
		const topic = "lanverse.catalog.credential_test_requested.v1"
		payload, err := json.Marshal(map[string]any{"event_id": eventID, "event_type": topic, "occurred_at": now, "org_id": actor.OrgID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "provider_credential", "id": request.CredentialID}, "data": map[string]any{"test_id": request.TestID, "provider_id": request.ProviderID, "credential_id": request.CredentialID, "request_id": request.RequestID}})
		if err != nil {
			return fmt.Errorf("encode credential test request: %w", err)
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?::uuid,?,?,?::jsonb)`, eventID, topic, actor.OrgID.String(), string(payload)).Error; err != nil {
			return fmt.Errorf("record credential test request: %w", err)
		}
		result = application.CredentialTestAccepted{TestID: request.TestID, EventID: eventID, ProviderID: request.ProviderID, CredentialID: request.CredentialID, Accepted: true}
		body, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("encode credential test receipt: %w", err)
		}
		if err := tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at) VALUES(?::uuid,?::uuid,?,?,202,?::jsonb,?)`, uuid.New(), actor.ID, requestID.String(), hash, string(body), now.Add(24*time.Hour)).Error; err != nil {
			return fmt.Errorf("store credential test receipt: %w", err)
		}
		return nil
	})
	return result, err
}

// VerifyCredentialTestRequest rejects injected events without an accepted receipt.
func (s *Store) VerifyCredentialTestRequest(ctx context.Context, request application.CredentialTestRequest, eventID uuid.UUID) error {
	if s == nil || s.db == nil || eventID == uuid.Nil {
		return application.ErrInvalidCredentialTest
	}
	var row struct{ Body string }
	result := s.db.WithContext(ctx).Raw(`SELECT response_body::text AS body FROM infra.idempotency_record WHERE actor_id=?::uuid AND idem_key=? AND status_code=202`, request.ActorID, request.RequestID).Scan(&row)
	if result.Error != nil {
		return fmt.Errorf("read accepted credential test: %w", result.Error)
	}
	var accepted application.CredentialTestAccepted
	if result.RowsAffected != 1 || json.Unmarshal([]byte(row.Body), &accepted) != nil || !accepted.Accepted || accepted.EventID != eventID || accepted.TestID != request.TestID || accepted.ProviderID != request.ProviderID || accepted.CredentialID != request.CredentialID {
		return application.ErrInvalidCredentialTest
	}
	return nil
}
