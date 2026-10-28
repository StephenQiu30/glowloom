package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	bibledomain "github.com/StephenQiu30/glowloom-server/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// ValidateDraftSources rechecks pinned source facts inside the owning save/quote transaction.
// Pending slots are saveable. Resource readiness is derived exclusively from server rows.
func ValidateDraftSources(tx *gorm.DB, a identityapp.Principal, p uuid.UUID, d domain.Draft, ready bool, subjects ...application.SubjectSources) error {
	var sourceOwner application.SubjectSources
	if len(subjects) == 1 {
		sourceOwner = subjects[0]
	}
	for _, r := range d.References {
		s := r.Source
		switch s.Kind {
		case "pending_upload":
			if ready {
				return domain.ErrReferenceUnavailable
			}
			if s.UploadSessionID != "" {
				if s.UploadSessionID != s.UploadAttemptID {
					return domain.ErrReferenceUnavailable
				}
				id, err := uuid.Parse(s.UploadSessionID)
				if err != nil {
					return domain.ErrReferenceUnavailable
				}
				var found int
				result := tx.Raw(`SELECT 1 FROM media.upload_request WHERE request_key=? AND principal_id=? AND project_id=? FOR SHARE`, id, a.ID, p).Scan(&found)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return domain.ErrReferenceUnavailable
				}
			}
		case "media":
			if err := checkDraftMedia(tx, p, domain.SourceMedia{SourceID: s.SourceID, SourceVersion: s.SourceVersion, MediaKind: r.MediaKind}, ready); err != nil {
				return err
			}
		case "subject":
			var expected domain.ReferenceSource
			var found bool
			var err error
			if sourceOwner != nil {
				expected, found, err = sourceOwner.ResolveSubject(tx.Statement.Context, a, p, s.SourceID, s.SourceVersion)
				if err != nil {
					return err
				}
			}
			if !found {
				for _, m := range s.Media {
					if m.BindingID != "" {
						return domain.ErrReferenceUnavailable
					}
				}
				expected, err = subjectSource(tx, p, s.SourceID, s.SourceVersion)
			}
			if err != nil {
				return err
			}
			if expected.Description != s.Description || len(expected.Media) != len(s.Media) {
				return domain.ErrReferenceUnavailable
			}
			for i, m := range expected.Media {
				if m != s.Media[i] {
					return domain.ErrReferenceUnavailable
				}
				if m.BindingID != "" {
					if sourceOwner == nil {
						return domain.ErrReferenceUnavailable
					}
					if err := sourceOwner.ValidateBinding(tx.Statement.Context, a, p, m, ready); err != nil {
						return err
					}
				} else if err := checkDraftMedia(tx, p, m, ready); err != nil {
					return err
				}
			}
		default:
			return domain.ErrInvalidDraft
		}
	}
	return nil
}
func checkDraftMedia(tx *gorm.DB, p uuid.UUID, m domain.SourceMedia, ready bool) error {
	id, err := uuid.Parse(m.SourceID)
	if err != nil {
		return domain.ErrReferenceUnavailable
	}
	version, err := strconv.ParseInt(m.SourceVersion, 10, 64)
	if err != nil || version < 1 {
		return domain.ErrReferenceUnavailable
	}
	var row struct {
		Kind, Status, ModerationStatus string
		Revision                       int64
		ContainsRealPerson             bool
		ConsentRecordID                *uuid.UUID
	}
	r := tx.Raw(`SELECT kind,status,moderation_status,revision,contains_real_person,consent_record_id FROM media.media_asset WHERE id=? AND project_id=? AND NOT is_delete FOR SHARE`, id, p).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 || row.Kind != m.MediaKind || row.Revision != version {
		return domain.ErrReferenceUnavailable
	}
	if ready && (row.Status != "ready" || row.ModerationStatus != "passed" || (row.ContainsRealPerson && row.ConsentRecordID == nil)) {
		return domain.ErrReferenceUnavailable
	}
	return nil
}
func subjectSource(tx *gorm.DB, p uuid.UUID, sourceID, versionID string) (domain.ReferenceSource, error) {
	out := domain.ReferenceSource{Kind: "subject", SourceID: sourceID, SourceVersion: versionID, Media: []domain.SourceMedia{}}
	id, e1 := uuid.Parse(sourceID)
	v, e2 := uuid.Parse(versionID)
	if e1 != nil || e2 != nil {
		return out, domain.ErrReferenceUnavailable
	}
	// Table names are constants, never supplied by a client. Versions remain pinned after later edits.
	for _, kind := range []string{"character", "location", "prop"} {
		var row struct{ Content []byte }
		query := fmt.Sprintf(`SELECT v.content FROM bible.%s_version v JOIN bible.%s h ON h.id=v.entry_id WHERE v.id=? AND v.entry_id=? AND v.project_id=? AND NOT h.is_delete FOR SHARE OF h`, kind, kind)
		r := tx.Raw(query, v, id, p).Scan(&row)
		if r.Error != nil {
			return out, r.Error
		}
		if r.RowsAffected == 0 {
			continue
		}
		var content struct {
			Name, Description string
			Looks             []bibledomain.LookContent
		}
		if json.Unmarshal(row.Content, &content) != nil {
			return out, domain.ErrReferenceUnavailable
		}
		out.Description = content.Name
		if content.Description != "" {
			out.Description += "：" + content.Description
		}
		for _, look := range content.Looks {
			if !look.Default {
				continue
			}
			for _, ref := range look.References {
				out.Media = append(out.Media, domain.SourceMedia{SourceID: ref.Media.AssetID.String(), SourceVersion: strconv.FormatInt(ref.Media.Revision, 10), MediaKind: "image"})
			}
		}
		return out, nil
	}
	return out, domain.ErrReferenceUnavailable
}

// ResolveUpload returns only the current actor's original successful upload attempt.
func (s *DraftStore) ResolveUpload(ctx context.Context, a identityapp.Principal, p, attempt uuid.UUID) (application.UploadResolution, error) {
	var out application.UploadResolution
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, p, false); err != nil {
			return err
		}
		var row struct{ SourceID, SourceVersion, MediaKind string }
		result := tx.Raw(`SELECT m.id::text AS source_id,m.revision::text AS source_version,m.kind AS media_kind FROM media.upload_request u JOIN media.media_asset m ON m.id=u.asset_id AND m.project_id=u.project_id WHERE u.project_id=? AND u.principal_id=? AND u.request_key=? AND NOT m.is_delete FOR SHARE OF m`, p, a.ID, attempt).Scan(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := checkDraftMedia(tx, p, domain.SourceMedia{SourceID: row.SourceID, SourceVersion: row.SourceVersion, MediaKind: row.MediaKind}, true); err != nil {
			return err
		}
		out = application.UploadResolution{Found: true, MediaKind: row.MediaKind, Source: &domain.ReferenceSource{Kind: "media", SourceID: row.SourceID, SourceVersion: row.SourceVersion}}
		return nil
	})
	return out, err
}
