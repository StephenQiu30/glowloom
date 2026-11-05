package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type generatedAudioAssetRecord struct {
	ID               uuid.UUID
	PersonalOrgID    uuid.UUID
	PersonalActorID  uuid.UUID
	Kind             string
	Origin           string
	Status           string
	ObjectKey        string
	ObjectLocationID uuid.UUID
	FileName         string
	MimeType         string
	ByteSize         int64
	SHA256           *string
	DurationMS       *int32
	AudioChannels    *int32
	Codec            *string
	ModerationStatus string
	ModerationDetail jsonDocument
	AIGCMarked       bool
	Revision         int64
	CreateTime       time.Time
	UpdateTime       time.Time
}

func (generatedAudioAssetRecord) TableName() string { return "media.media_asset" }

type generatedAudioAuditRecord struct {
	ID, OrgID, ActorID uuid.UUID
	ActorKind          string
	Action             string
	ObjectType         string
	ObjectID           string
	After              jsonDocument
	RequestID          string
}

func (generatedAudioAuditRecord) TableName() string { return "audit.audit_log" }

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
		intent.ConfigRevision != r.Intent.ConfigRevision || intent.State != application.StorageWorkOpen && intent.State != application.StorageWorkAccepted {
		return application.LibraryAssetSummary{}, application.ErrStorageRouteConflict
	}
	if err := validateGeneratedAudioLocations(tx, prepared, intent.State); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	if err := requireNoNewMediaPurge(tx, []uuid.UUID{a.ID}); err != nil {
		return application.LibraryAssetSummary{}, err
	}
	var prior assetRow
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", a.ID).Take(&prior).Error
	if err == nil {
		if intent.State != application.StorageWorkAccepted {
			return application.LibraryAssetSummary{}, application.ErrStorageRouteConflict
		}
		if err := replayGeneratedAudio(tx, prepared, prior); err != nil {
			return application.LibraryAssetSummary{}, err
		}
		return application.PersonalUploadSummary(prior.domain()), nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return application.LibraryAssetSummary{}, fmt.Errorf("read generated personal audio replay: %w", err)
	}
	if intent.State != application.StorageWorkOpen {
		return application.LibraryAssetSummary{}, application.ErrStorageRouteConflict
	}
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	libraryID, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		return application.LibraryAssetSummary{}, err
	}
	libraryCandidate := libraryRow{ID: libraryID, Kind: string(domain.LibraryPersonal), OrgID: actor.OrgID, PersonalActorID: &actor.ID}
	if err := tx.Select("id", "kind", "org_id", "personal_actor_id").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&libraryCandidate).Error; err != nil {
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
	updatedAt := time.Now().UTC()
	if updatedAt.Before(library.UpdateTime) {
		updatedAt = library.UpdateTime
	}
	updated := tx.Model(&libraryRow{}).Where("id = ? AND revision = ?", libraryID, library.Revision).Updates(map[string]any{
		"revision":    library.Revision + 1,
		"update_time": updatedAt,
	})
	if updated.Error != nil {
		return application.LibraryAssetSummary{}, fmt.Errorf("persist media library change: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return application.LibraryAssetSummary{}, application.ErrLibraryConflict
	}
	after, err := json.Marshal(map[string]any{"source_job_id": r.SourceJobID, "receipt_sha256": r.Provenance.ReceiptSHA256, "kind": "audio", "aigc_marked": true})
	if err != nil {
		return application.LibraryAssetSummary{}, err
	}
	audit := generatedAudioAuditRecord{ID: uuid.New(), OrgID: actor.OrgID, ActorID: actor.ID, ActorKind: "user", Action: "media.generated_audio_accepted", ObjectType: "media_asset", ObjectID: a.ID.String(), After: jsonDocument(after), RequestID: r.SourceJobID.String()}
	if err := tx.Create(&audit).Error; err != nil {
		return application.LibraryAssetSummary{}, fmt.Errorf("record generated audio audit: %w", err)
	}
	return application.PersonalUploadSummary(a), nil
}

func validateGeneratedAudioLocations(tx *gorm.DB, p application.PreparedPersonalGeneratedAudio, intentState application.StorageWorkState) error {
	var rows []objectLocationRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner_intent_id = ?", p.Request.Intent.ID).Order("id").Limit(3).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != 2 {
		return application.ErrStorageRouteConflict
	}
	expected := map[uuid.UUID]domain.ObjectLocation{p.Asset.ObjectLocation.ID: *p.Asset.ObjectLocation, p.Renditions[0].ObjectLocation.ID: *p.Renditions[0].ObjectLocation}
	wantState := domain.ObjectLocationPresent
	if intentState == application.StorageWorkAccepted {
		wantState = domain.ObjectLocationAccepted
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
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("media_asset_id = ?", row.ID).Order("id").Limit(2).Find(&rows).Error; err != nil {
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
	asset := generatedAudioAssetRecord{ID: a.ID, PersonalOrgID: a.Personal.OrgID, PersonalActorID: a.Personal.ActorID, Kind: string(a.Kind), Origin: string(a.Origin), Status: string(a.Status), ObjectKey: a.ObjectKey, ObjectLocationID: a.ObjectLocation.ID, FileName: a.FileName, MimeType: a.MimeType, ByteSize: a.ByteSize, SHA256: a.SHA256, DurationMS: a.DurationMS, AudioChannels: a.AudioChannels, Codec: a.Codec, ModerationStatus: string(a.ModerationStatus), ModerationDetail: jsonDocument(a.ModerationDetail), AIGCMarked: a.AIGCMarked, Revision: a.Revision, CreateTime: a.CreateTime, UpdateTime: a.UpdateTime}
	if err := tx.Create(&asset).Error; err != nil {
		return err
	}
	for _, r := range prepared.Renditions {
		rendition := renditionRow{ID: r.ID, MediaAssetID: r.MediaAssetID, Kind: string(r.Kind), ObjectKey: r.ObjectKey, ObjectLocationID: &r.ObjectLocation.ID, Width: r.Width, Height: r.Height, ByteSize: r.ByteSize, CreateTime: r.CreateTime, UpdateTime: r.UpdateTime}
		if err := tx.Create(&rendition).Error; err != nil {
			return err
		}
	}
	return nil
}
