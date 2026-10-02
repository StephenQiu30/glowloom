package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

func mergeCharacter(ctx context.Context, tx Transaction, actor identityapp.Principal, c Command, h *domain.Head, v domain.Version, now time.Time) error {
	if h.ConfirmedVersionID == nil || *h.ConfirmedVersionID != v.ID {
		return ErrConflict
	}
	target, version, err := tx.Load(ctx, domain.KindCharacter, *c.TargetID)
	if err != nil {
		return err
	}
	if target.Revision != *c.ExpectedTargetRevision || target.Deleted || target.RedirectID != nil || target.ConfirmedVersionID == nil || *target.ConfirmedVersionID != version.ID {
		return ErrConflict
	}
	if err := validateDependencies(ctx, tx, actor, c.ProjectID, v); err != nil {
		return err
	}
	if err := validateDependencies(ctx, tx, actor, c.ProjectID, version); err != nil {
		return err
	}
	if err := applyImpact(ctx, tx, actor, c, v); err != nil {
		return err
	}
	id := target.ID
	h.RedirectID = &id
	return tx.Redirect(ctx, domain.Redirect{ID: uuid.New(), SourceID: h.ID, TargetID: target.ID, SourceVersionID: v.ID, TargetVersionID: version.ID, ActorID: actor.ID, CreatedAt: now})
}

func splitCharacter(ctx context.Context, tx Transaction, actor identityapp.Principal, c Command, parent domain.Version, now time.Time) (domain.Head, domain.Version, error) {
	child := domain.Head{ID: uuid.New(), OrgID: actor.OrgID, ProjectID: c.ProjectID, Kind: domain.KindCharacter, Revision: 1, CreatedAt: now, UpdatedAt: now}
	v := domain.Version{ID: uuid.New(), EntryID: child.ID, OrgID: child.OrgID, ProjectID: child.ProjectID, Kind: child.Kind, Number: 1, ActorID: actor.ID, CreatedAt: now, Origin: domain.OriginManual}
	if err := setContent(&v, c); err != nil {
		return child, v, err
	}
	_, hash, err := v.Content()
	if err != nil {
		return child, v, err
	}
	v.ContentSHA256 = hash
	child.CurrentVersionID = v.ID
	if err := tx.InsertHead(ctx, child); err != nil {
		return child, v, err
	}
	if err := tx.InsertVersion(ctx, v); err != nil {
		return child, v, err
	}
	if err := tx.Split(ctx, domain.Split{ID: uuid.New(), SourceID: parent.EntryID, SourceVersionID: parent.ID, TargetID: child.ID, TargetVersionID: v.ID, ActorID: actor.ID, CreatedAt: now}); err != nil {
		return child, v, err
	}
	return child, v, nil
}
