package postgres

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func quoteRequestFingerprint(input application.CreateFreeQuoteInput) ([sha256.Size]byte, error) {
	var params map[string]json.RawMessage
	if err := json.Unmarshal(input.Params, &params); err != nil {
		return [sha256.Size]byte{}, application.ErrInvalidFreeQuote
	}
	encoded, err := json.Marshal(struct {
		Kind            string                            `json:"kind"`
		ProjectID       uuid.UUID                         `json:"project_id"`
		ModelKey        string                            `json:"model_key"`
		Capability      string                            `json:"capability"`
		Mode            string                            `json:"mode"`
		Prompt          string                            `json:"prompt"`
		MediaInputs     []application.FreeQuoteMediaInput `json:"media_inputs"`
		Params          map[string]json.RawMessage        `json:"params"`
		OutputCount     int32                             `json:"output_count"`
		ForceRegenerate bool                              `json:"force_regenerate"`
		Source          *domain.CanvasSource              `json:"source"`
	}{
		Kind:      "free_single",
		ProjectID: input.ProjectID, ModelKey: input.ModelKey,
		Capability: input.Capability, Mode: input.Mode, Prompt: input.Prompt,
		MediaInputs: input.MediaInputs,
		Params:      params, OutputCount: input.OutputCount,
		ForceRegenerate: input.ForceRegenerate,
		Source:          input.Source,
	})
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode quote request fingerprint: %w", err)
	}
	return sha256.Sum256(encoded), nil
}

func batchQuoteRequestFingerprint(input application.CreateBatchFreeQuoteInput) ([sha256.Size]byte, error) {
	type item struct {
		ModelKey        string                            `json:"model_key"`
		Capability      string                            `json:"capability"`
		Mode            string                            `json:"mode"`
		Prompt          string                            `json:"prompt"`
		MediaInputs     []application.FreeQuoteMediaInput `json:"media_inputs"`
		Params          json.RawMessage                   `json:"params"`
		OutputCount     int32                             `json:"output_count"`
		ForceRegenerate bool                              `json:"force_regenerate"`
		Source          *domain.CanvasSource              `json:"source"`
	}
	items := make([]item, 0, len(input.Items))
	for _, requested := range input.Items {
		raw := requested.Params
		if len(raw) == 0 {
			raw = json.RawMessage(`{}`)
		}
		var params map[string]json.RawMessage
		if err := json.Unmarshal(raw, &params); err != nil || params == nil {
			return [sha256.Size]byte{}, application.ErrInvalidBatchFreeQuote
		}
		if requested.Validate() == nil {
			canonical, err := json.Marshal(params)
			if err != nil {
				return [sha256.Size]byte{}, fmt.Errorf("encode batch quote parameters: %w", err)
			}
			raw = canonical
		}
		items = append(items, item{
			ModelKey: requested.ModelKey, Capability: requested.Capability,
			Mode: requested.Mode, Prompt: requested.Prompt, MediaInputs: requested.MediaInputs, Params: raw,
			OutputCount: requested.OutputCount, ForceRegenerate: requested.ForceRegenerate,
			Source: requested.Source,
		})
	}
	encoded, err := json.Marshal(struct {
		Kind      string    `json:"kind"`
		ProjectID uuid.UUID `json:"project_id"`
		Items     []item    `json:"items"`
	}{Kind: "free_batch", ProjectID: input.ProjectID, Items: items})
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode batch quote fingerprint: %w", err)
	}
	return sha256.Sum256(encoded), nil
}

func lockQuoteRequest(tx *gorm.DB, actor identityapp.Principal, requestID uuid.UUID) error {
	digest := sha256.Sum256([]byte(actor.ID.String() + ":" + requestID.String()))
	first := int32(binary.BigEndian.Uint32(digest[:4]))
	second := int32(binary.BigEndian.Uint32(digest[4:8]))
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(?, ?)`, first, second).Error; err != nil {
		return fmt.Errorf("lock quote request: %w", err)
	}
	return nil
}

func readQuoteRequest(tx *gorm.DB, actor identityapp.Principal, requestID uuid.UUID, fingerprint [sha256.Size]byte) (json.RawMessage, bool, error) {
	var row struct {
		Fingerprint []byte
		Result      string
	}
	read := tx.Raw(`
		SELECT fingerprint, result::text AS result
		FROM operation.quote_request
		WHERE org_id = ?::uuid AND actor_id = ?::uuid AND request_id = ?::uuid
	`, actor.OrgID.String(), actor.ID.String(), requestID.String()).Scan(&row)
	if read.Error != nil {
		return nil, false, fmt.Errorf("read quote request: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return nil, false, nil
	}
	if !bytes.Equal(row.Fingerprint, fingerprint[:]) {
		return nil, true, application.ErrQuoteKeyReused
	}
	return json.RawMessage(row.Result), true, nil
}

func insertQuoteRequest(tx *gorm.DB, actor identityapp.Principal, requestID uuid.UUID, fingerprint [sha256.Size]byte, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode quote result: %w", err)
	}
	write := tx.Exec(`
		INSERT INTO operation.quote_request
		  (org_id, actor_id, request_id, fingerprint, result)
		VALUES (?::uuid, ?::uuid, ?::uuid, ?, ?::jsonb)
	`, actor.OrgID.String(), actor.ID.String(), requestID.String(), fingerprint[:], string(encoded))
	if write.Error != nil {
		return fmt.Errorf("insert quote request: %w", write.Error)
	}
	if write.RowsAffected != 1 {
		return fmt.Errorf("insert quote request: wrote %d rows", write.RowsAffected)
	}
	return nil
}
