package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// acceptTranscript joins the real artifact receipt and the terminal job change.
// The DB draft is authoritative; the private object preserves exact recovery bytes.
func (s *TranscriptionStore) acceptTranscript(ctx context.Context, tx *gorm.DB, row transcriptionRow) error {
	if s.storage == nil {
		return nil
	}
	var frozen domain.FrozenTranscription
	if json.Unmarshal(row.Frozen, &frozen) != nil {
		return application.ErrInvalidTranscription
	}
	ledger := s.storage(tx)
	owner := application.StorageOwner("transcription", row.ProjectID, row.ID, row.Attempt)
	intent, err := ledger.ReadStorageWork(ctx, owner)
	if err != nil {
		return err
	}
	if intent.Owner != owner || intent.State != "open" {
		return media.ErrStorageIdentityConflict
	}
	key := path.Join("projects", row.ProjectID.String(), "transcriptions", row.ID.String(), "attempt-"+strconv.Itoa(row.Attempt)+".json")
	loc, found, err := ledger.FindWorkLocation(ctx, intent.ID, key)
	if err != nil {
		return err
	}
	if !found || loc.Ref.ProfileID != intent.ProfileID || loc.Ref.Bucket != intent.Bucket || loc.Ref.Key != key || loc.State != "present" || loc.ByteSize == nil || loc.SHA256 == nil || loc.MIMEType == nil || *loc.MIMEType != "application/json" {
		return media.ErrStorageIdentityConflict
	}
	// The application verified and decoded these exact bytes before Complete.
	var draft domain.Transcript
	if json.Unmarshal(row.Result, &draft) != nil {
		return application.ErrInvalidTranscription
	}
	raw, err := json.Marshal(struct {
		Version      int               `json:"version"`
		JobID        string            `json:"job_id"`
		Attempt      int               `json:"attempt"`
		SourceSHA256 string            `json:"source_sha256"`
		Draft        domain.Transcript `json:"draft"`
	}{1, row.ID.String(), row.Attempt, frozen.Input.SHA256, draft})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	if *loc.ByteSize != int64(len(raw)) || *loc.SHA256 != hex.EncodeToString(digest[:]) {
		return media.ErrStorageIdentityConflict
	}
	return ledger.AcceptStorageWork(ctx, intent.ID, []uuid.UUID{loc.ID})
}
