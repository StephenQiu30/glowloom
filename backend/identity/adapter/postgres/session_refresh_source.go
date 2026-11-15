package postgres

import (
	"context"
	"crypto/subtle"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// LoadRefreshReuseRequest recovers only the accepted internal reuse action. The
// target comes exclusively from immutable evidence, even after token cleanup.
func (s *SessionRevocationStore) LoadRefreshReuseRequest(ctx context.Context, proof application.RefreshReuseRequestProof) (application.SessionRevocationRequest, error) {
	if err := proof.Validate(); err != nil {
		return application.SessionRevocationRequest{}, err
	}
	var result application.SessionRevocationRequest
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		result, err = findSessionRevocationProof(tx, proof.Key, []application.SessionRevocationProof{proof.Proof})
		if err != nil {
			return err
		}
		if result.Key != proof.Key || result.Action != application.RevokeRefreshReuseRequest || result.TargetID != result.RequestSessionID {
			return application.ErrForbidden
		}
		return nil
	})
	return result, err
}

func findSessionRevocationProof(tx *gorm.DB, key uuid.UUID, proofs []application.SessionRevocationProof) (application.SessionRevocationRequest, error) {
	var matched uuid.UUID
	knownProof := false
	for _, proof := range proofs {
		var rows []struct {
			ID       uuid.UUID
			CSRFHash []byte
		}
		if err := tx.Raw(`SELECT r.id,p.csrf_hash FROM identity.session_revocation_request r JOIN identity.session_revocation_request_proof p ON p.request_id=r.id WHERE r.idem_key=? AND p.kind=? AND p.token_digest=?`, key, proof.Kind, proof.TokenDigest).Scan(&rows).Error; err != nil {
			return application.SessionRevocationRequest{}, fmt.Errorf("find original session revocation proof: %w", err)
		}
		for _, row := range rows {
			knownProof = true
			if subtle.ConstantTimeCompare(row.CSRFHash, proof.CSRFHash) == 1 {
				if matched != uuid.Nil && matched != row.ID {
					return application.SessionRevocationRequest{}, application.ErrForbidden
				}
				matched = row.ID
			}
		}
	}
	if matched == uuid.Nil {
		if knownProof {
			return application.SessionRevocationRequest{}, application.ErrSessionCSRFInvalid
		}
		return application.SessionRevocationRequest{}, writefence.ErrNotFound
	}
	return readSessionRevocationRequest(tx, matched, false)
}

var _ application.RefreshReuseRequestReader = (*SessionRevocationStore)(nil)
