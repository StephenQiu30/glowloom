package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// AcceptPersonalGeneratedAudio runs only on the caller's already-open result
// transaction. The voice owner verifies its immutable receipt and worker lease;
// this owner rechecks personal authority and accepts the exact storage output set.
func (s *Store) AcceptPersonalGeneratedAudio(ctx context.Context, actor identity.Principal, prepared application.PreparedPersonalGeneratedAudio) (application.LibraryAssetSummary, error) {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return application.LibraryAssetSummary{}, application.ErrUnavailable
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return application.LibraryAssetSummary{}, application.ErrUnavailable
	}
	r, a := prepared.Request, prepared.Asset
	if prepared.Validate() != nil || actor.ID != r.Personal.ActorID || actor.OrgID != r.Personal.OrgID {
		return application.LibraryAssetSummary{}, application.ErrInvalidGeneratedAudio
	}
	tx := s.db.WithContext(ctx)
	if err := s.requirePersonal(tx, actor, true); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	// Share the personal-library publication lock with ordinary uploads. This
	// serializes the library revision without giving generated work upload rights.
	if err := personalUploadLock(tx, r.Personal); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	routes := NewStorageRoutes(tx)
	intent, err := routes.ReadStorageWork(ctx, r.Intent.Owner)
	if err != nil {
		return application.LibraryAssetSummary{}, err
	}
	if intent.ID != r.Intent.ID || intent.ProfileID != r.Intent.ProfileID || intent.Bucket != r.Intent.Bucket ||
		intent.ConfigRevision != r.Intent.ConfigRevision || intent.State != "open" && intent.State != "accepted" {
		return application.LibraryAssetSummary{}, application.ErrStorageRouteConflict
	}
	if err := validateGeneratedAudioLocations(tx, prepared, intent.State); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	if err := requireNoNewMediaPurge(tx, []uuid.UUID{a.ID}); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	var prior assetRow
	read := tx.Raw(`SELECT * FROM media.media_asset WHERE id=? FOR UPDATE`, a.ID).Scan(&prior)
	if read.Error != nil {
		return application.LibraryAssetSummary{}, fmt.Errorf("read generated personal audio replay: %w", read.Error)
	}
	if read.RowsAffected == 1 {
		if intent.State != "accepted" {
			return application.LibraryAssetSummary{}, application.ErrStorageRouteConflict
		}
		if err := replayGeneratedAudio(tx, prepared, prior); err != nil {
			return application.LibraryAssetSummary{}, err
		}
		return application.PersonalUploadSummary(prior.domain()), nil
	}
	if intent.State != "open" {
		return application.LibraryAssetSummary{}, application.ErrStorageRouteConflict
	}
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	libraryID, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		return application.LibraryAssetSummary{}, err
	}
	if err := tx.Exec(`INSERT INTO media.library(id,kind,org_id,personal_actor_id) VALUES(?,'personal',?,?) ON CONFLICT(id) DO NOTHING`, libraryID, actor.OrgID, actor.ID).Error; err != nil {
		return application.LibraryAssetSummary{}, fmt.Errorf("ensure generated audio library: %w", err)
	}
	library, err := readLibrary(tx, actor, scope, true)
	if err != nil {
		return application.LibraryAssetSummary{}, err
	}
	locationIDs := []uuid.UUID{a.ObjectLocation.ID, prepared.Renditions[0].ObjectLocation.ID}
	if err := routes.AcceptStorageWork(ctx, intent.ID, locationIDs); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	if err := insertGeneratedAudio(tx, prepared); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	if err := libraryChanged(tx, `UPDATE media.library SET revision=revision+1,update_time=GREATEST(update_time,clock_timestamp()) WHERE id=? AND revision=?`, libraryID, library.Revision); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	after, err := json.Marshal(map[string]any{"source_job_id": r.SourceJobID, "receipt_sha256": r.Provenance.ReceiptSHA256, "kind": "audio", "aigc_marked": true})
	if err != nil {
		return application.LibraryAssetSummary{}, err
	}
	if err := libraryChanged(tx, `INSERT INTO audit.audit_log(id,org_id,actor_id,actor_kind,action,object_type,object_id,after,request_id) VALUES(?,?,?,'user','media.generated_audio_accepted','media_asset',?,?::jsonb,?)`, uuid.New(), actor.OrgID, actor.ID, a.ID.String(), string(after), r.SourceJobID.String()); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	return application.PersonalUploadSummary(a), nil
}

func validateGeneratedAudioLocations(tx *gorm.DB, p application.PreparedPersonalGeneratedAudio, intentState string) error {
	var rows []objectLocationRow
	if err := tx.Raw(`SELECT * FROM media.object_location WHERE owner_intent_id=? ORDER BY id LIMIT 3 FOR UPDATE`, p.Request.Intent.ID).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) != 2 {
		return application.ErrStorageRouteConflict
	}
	expected := map[uuid.UUID]domain.ObjectLocation{p.Asset.ObjectLocation.ID: *p.Asset.ObjectLocation, p.Renditions[0].ObjectLocation.ID: *p.Renditions[0].ObjectLocation}
	wantState := "present"
	if intentState == "accepted" {
		wantState = "accepted"
	}
	for _, row := range rows {
		want, ok := expected[row.ID]
		got := row.domain()
		if !ok || row.OwnerIntentID == nil || *row.OwnerIntentID != p.Request.Intent.ID || row.OwnerSessionID != nil ||
			got.Ref != want.Ref || got.State != wantState ||
			!generatedAudioValueEqual(got.ByteSize, want.ByteSize) || !generatedAudioValueEqual(got.SHA256, want.SHA256) || !generatedAudioValueEqual(got.MIMEType, want.MIMEType) {
			return application.ErrObjectMismatch
		}
	}
	return nil
}

func replayGeneratedAudio(tx *gorm.DB, expected application.PreparedPersonalGeneratedAudio, row assetRow) error {
	if err := row.loadLocation(tx); err != nil {
		return err
	}
	var rows []renditionRow
	if err := tx.Raw(`SELECT * FROM media.rendition WHERE media_asset_id=? ORDER BY id LIMIT 2 FOR SHARE`, row.ID).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) != 1 {
		return application.ErrObjectMismatch
	}
	if err := rows[0].loadLocation(tx); err != nil {
		return err
	}
	stored := application.PreparedPersonalGeneratedAudio{Request: expected.Request, Asset: row.domain(), Renditions: []domain.Rendition{rows[0].domain()}}
	if stored.Validate() != nil || !generatedAudioValueEqual(stored.Asset.DurationMS, expected.Asset.DurationMS) ||
		!generatedAudioValueEqual(stored.Asset.AudioChannels, expected.Asset.AudioChannels) ||
		!generatedAudioValueEqual(stored.Asset.Codec, expected.Asset.Codec) {
		return application.ErrObjectMismatch
	}
	return nil
}

func generatedAudioValueEqual[T comparable](a, b *T) bool {
	return a != nil && b != nil && *a == *b
}

func insertGeneratedAudio(tx *gorm.DB, prepared application.PreparedPersonalGeneratedAudio) error {
	a := prepared.Asset
	if err := libraryChanged(tx, `INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,duration_ms,audio_channels,codec,moderation_status,moderation_detail,aigc_marked,revision,create_time,update_time) VALUES(?,?,?,'audio','system','ready',?,?,?,?,?,?,?,?,?,'passed',?::jsonb,true,1,?,?)`,
		a.ID, a.Personal.OrgID, a.Personal.ActorID, a.ObjectKey, a.ObjectLocation.ID, a.FileName, a.MimeType, a.ByteSize, a.SHA256, a.DurationMS, a.AudioChannels, a.Codec, string(a.ModerationDetail), a.CreateTime, a.UpdateTime); err != nil {
		return err
	}
	for _, r := range prepared.Renditions {
		if err := libraryChanged(tx, `INSERT INTO media.rendition(id,media_asset_id,kind,object_key,object_location_id,width,height,byte_size,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?,?)`, r.ID, r.MediaAssetID, string(r.Kind), r.ObjectKey, r.ObjectLocation.ID, r.Width, r.Height, r.ByteSize, r.CreateTime, r.UpdateTime); err != nil {
			return err
		}
	}
	return nil
}
