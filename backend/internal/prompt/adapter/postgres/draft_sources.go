package postgres

import (
	"encoding/json"
	"fmt"
	"strconv"

	bibledomain "github.com/StephenQiu30/glowloom-server/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ValidateDraftSources rechecks pinned source facts inside the owning save/quote transaction.
// Pending slots are saveable. Resource readiness is derived exclusively from server rows.
func ValidateDraftSources(tx *gorm.DB, a identityapp.Principal, p uuid.UUID, d domain.Draft, ready bool) error {
	for _, r := range d.References {
		s := r.Source
		switch s.Kind {
		case "pending_upload":
			if ready {
				return domain.ErrReferenceUnavailable
			}
			if s.UploadSessionID != "" {
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
			expected, err := subjectSource(tx, p, s.SourceID, s.SourceVersion)
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
				if err := checkDraftMedia(tx, p, m, ready); err != nil {
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
