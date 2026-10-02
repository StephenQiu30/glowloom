package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func readBibleHistory(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, project uuid.UUID) (application.CopyHistory, error) {
	history := application.CopyHistory{}
	b := &bound{tx: tx.WithContext(ctx), actor: actor, project: workspaceapp.ProjectContentAccess{ProjectID: project, OrgID: actor.OrgID}}
	for _, kind := range []domain.Kind{domain.KindCharacter, domain.KindLocation, domain.KindProp} {
		headTable, versionTable, confirmationTable, err := tables(kind)
		if err != nil {
			return history, err
		}
		var headIDs, versionIDs []struct{ ID uuid.UUID }
		if err := tx.Raw("SELECT id FROM "+headTable+" WHERE org_id=? AND project_id=? ORDER BY id FOR SHARE", actor.OrgID, project).Scan(&headIDs).Error; err != nil {
			return history, err
		}
		for _, row := range headIDs {
			h, err := b.loadHead(ctx, kind, row.ID)
			if err != nil {
				return history, err
			}
			history.Heads = append(history.Heads, h)
		}
		if err := tx.Raw("SELECT id FROM "+versionTable+" WHERE org_id=? AND project_id=? ORDER BY entry_id,version_no", actor.OrgID, project).Scan(&versionIDs).Error; err != nil {
			return history, err
		}
		for _, row := range versionIDs {
			v, err := b.LoadVersion(ctx, kind, row.ID)
			if err != nil {
				return history, err
			}
			history.Versions = append(history.Versions, v)
		}
		var confirmations []domain.Confirmation
		if err := tx.Raw("SELECT id,entry_id,version_id,revision,actor_id,created_at FROM "+confirmationTable+" WHERE org_id=? AND project_id=? ORDER BY id", actor.OrgID, project).Scan(&confirmations).Error; err != nil {
			return history, err
		}
		for _, c := range confirmations {
			history.Confirmations = append(history.Confirmations, application.CopyConfirmation{Kind: kind, Confirmation: c})
		}
	}
	for _, query := range []struct {
		sql string
		out any
	}{
		{`SELECT id,character_id,created_at FROM bible.look WHERE org_id=? AND project_id=? ORDER BY id`, &history.Looks},
		{`SELECT id,source_id,target_id,source_version_id,target_version_id,actor_id,created_at FROM bible.character_redirect WHERE org_id=? AND project_id=? ORDER BY id`, &history.Redirects},
		{`SELECT id,source_id,target_id,source_version_id,target_version_id,actor_id,created_at FROM bible.character_split WHERE org_id=? AND project_id=? ORDER BY id`, &history.Splits},
	} {
		if err := tx.Raw(query.sql, actor.OrgID, project).Scan(query.out).Error; err != nil {
			return history, err
		}
	}
	counts, err := application.ValidateCopyHistory(history, actor.OrgID, project)
	if err != nil {
		return history, err
	}
	for _, check := range []struct {
		table    string
		expected int
	}{{"bible.look_version", counts.LookVersions}, {"bible.reference_version", counts.References}, {"bible.voice_version", counts.Voices}} {
		var n int64
		if err := tx.Table(check.table).Where("org_id=? AND project_id=?", actor.OrgID, project).Count(&n).Error; err != nil {
			return history, err
		}
		if n != int64(check.expected) {
			return history, fmt.Errorf("unexpected historical rows in %s: %w", check.table, domain.ErrCorruptHistory)
		}
	}
	return application.CanonicalCopyHistory(history)
}

func insertBibleHistory(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, h application.CopyHistory) error {
	if _, err := application.ValidateCopyHistory(h, actor.OrgID, project); err != nil {
		return err
	}
	b := &bound{tx: tx.WithContext(ctx), actor: actor, project: workspaceapp.ProjectContentAccess{ProjectID: project, OrgID: actor.OrgID}}
	for _, head := range h.Heads {
		table, _, _, err := tables(head.Kind)
		if err != nil {
			return err
		}
		fields := map[string]any{"id": head.ID, "org_id": head.OrgID, "project_id": head.ProjectID, "revision": head.Revision, "current_version_id": head.CurrentVersionID, "confirmed_version_id": head.ConfirmedVersionID, "is_delete": head.Deleted, "created_at": head.CreatedAt, "updated_at": head.UpdatedAt}
		if head.Kind == domain.KindCharacter {
			fields["redirect_id"] = head.RedirectID
		}
		if err := exactlyOne(tx.Table(table).Create(fields)); err != nil {
			return err
		}
	}
	for _, look := range h.Looks {
		if err := exactlyOne(tx.Exec(`INSERT INTO bible.look(id,org_id,project_id,character_id,created_at) VALUES(?,?,?,?,?)`, look.ID, actor.OrgID, project, look.CharacterID, look.CreatedAt)); err != nil {
			return err
		}
	}
	for _, version := range h.Versions {
		if err := b.InsertVersion(ctx, version); err != nil {
			return err
		}
	}
	for _, confirmation := range h.Confirmations {
		if err := b.Confirm(ctx, confirmation.Kind, confirmation.Confirmation); err != nil {
			return err
		}
	}
	for _, redirect := range h.Redirects {
		if err := b.Redirect(ctx, redirect); err != nil {
			return err
		}
	}
	for _, split := range h.Splits {
		if err := b.Split(ctx, split); err != nil {
			return err
		}
	}
	return nil
}
