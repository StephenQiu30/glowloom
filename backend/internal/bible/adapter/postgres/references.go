package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// References resolves confirmed character facts within another owner's exact transaction.
type References struct {
	tx     *gorm.DB
	access application.ProjectAccess
}

// NewReferences injects the caller transaction and its currently authorized project access.
func NewReferences(tx *gorm.DB, access application.ProjectAccess) *References {
	return &References{tx: tx, access: access}
}

// Reference resolves current redirects or verifies a pinned historically confirmed version.
func (r *References) Reference(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, pinned *uuid.UUID) (application.CharacterReference, error) {
	if r == nil || r.tx == nil || r.access == nil || project == uuid.Nil || id == uuid.Nil || (pinned != nil && *pinned == uuid.Nil) {
		return application.CharacterReference{}, application.ErrUnavailable
	}
	if _, ok := r.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return application.CharacterReference{}, application.ErrUnavailable
	}
	facts, err := r.access.Authorize(ctx, actor, project, false)
	if err != nil {
		return application.CharacterReference{}, err
	}
	if facts.OrgID != actor.OrgID || facts.ProjectID != project {
		return application.CharacterReference{}, application.ErrUnavailable
	}
	b := &bound{tx: r.tx.WithContext(ctx), actor: actor, project: facts, access: r.access}
	h, err := b.loadHead(ctx, domain.KindCharacter, id)
	if err != nil {
		return application.CharacterReference{}, err
	}
	if h.Deleted {
		return application.CharacterReference{}, application.ErrNotFound
	}
	version := h.ConfirmedVersionID
	if pinned == nil {
		seen := map[uuid.UUID]bool{h.ID: true}
		for depth := 0; h.RedirectID != nil; depth++ {
			if depth >= 32 || seen[*h.RedirectID] {
				return application.CharacterReference{}, domain.ErrCorruptHistory
			}
			seen[*h.RedirectID] = true
			h, err = b.loadHead(ctx, domain.KindCharacter, *h.RedirectID)
			if err != nil {
				return application.CharacterReference{}, err
			}
			if h.Deleted {
				return application.CharacterReference{}, application.ErrNotFound
			}
			version = h.ConfirmedVersionID
		}
	} else {
		version = pinned
	}
	if version == nil {
		return application.CharacterReference{}, application.ErrConflict
	}
	v, err := b.LoadVersion(ctx, domain.KindCharacter, *version)
	if err != nil {
		return application.CharacterReference{}, err
	}
	if v.EntryID != h.ID {
		return application.CharacterReference{}, application.ErrNotFound
	}
	var confirmations int64
	if err := b.tx.Raw(`SELECT count(*) FROM bible.character_confirmation WHERE org_id=? AND project_id=? AND entry_id=? AND version_id=?`, actor.OrgID, project, h.ID, v.ID).Scan(&confirmations).Error; err != nil {
		return application.CharacterReference{}, err
	}
	if confirmations < 1 {
		return application.CharacterReference{}, errors.Join(application.ErrConflict, domain.ErrInvalidContent)
	}
	return application.CharacterReference{CharacterID: h.ID, VersionID: v.ID, Revision: h.Revision}, nil
}
