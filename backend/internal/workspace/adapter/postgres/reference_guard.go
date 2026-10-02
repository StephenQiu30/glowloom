package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// MediaReferenceGuard reads only workspace-owned current and permanent historical facts.
type MediaReferenceGuard struct{ tx *gorm.DB }

// NewMediaReferenceGuard keeps authority and reference locks in the caller's transaction.
func NewMediaReferenceGuard(tx *gorm.DB) *MediaReferenceGuard { return &MediaReferenceGuard{tx: tx} }

// HasMediaReferences includes recycled directories, old covers and retained style presets.
func (g *MediaReferenceGuard) HasMediaReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	if g == nil || g.tx == nil || asset == uuid.Nil {
		return false, application.ErrProjectDependencyUnavailable
	}
	if _, err := NewProjectContentAccessStore(g.tx).Authorize(ctx, actor, project, false); err != nil {
		return false, err
	}
	tx := g.tx.WithContext(ctx)
	var present bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND cover_asset_id=?) OR EXISTS(SELECT 1 FROM workspace.style_preset WHERE org_id=? AND (project_id=? OR project_id IS NULL) AND ?::uuid=ANY(reference_asset_ids)) OR EXISTS(SELECT 1 FROM workspace.project_folder WHERE org_id=? AND cover_project_id=? AND cover_asset_id=?)`, project, actor.OrgID, asset, actor.OrgID, project, asset, actor.OrgID, project, asset).Scan(&present).Error; err != nil {
		return false, fmt.Errorf("read current workspace media references: %w", err)
	}
	if present {
		return true, nil
	}
	const maxFacts = 100000
	// Preserve the existing per-family row, combined history and single-body
	// budgets, but check them in SQL before any payload reaches this process.
	var budget struct{ ProjectFacts, FolderFacts, Bytes, Largest int64 }
	if err := tx.Raw(`SELECT count(*) FILTER(WHERE family='project') AS project_facts,count(*) FILTER(WHERE family='folder') AS folder_facts,coalesce(sum(size),0) AS bytes,coalesce(max(size),0) AS largest FROM (
 (SELECT 'project' AS family,octet_length(response_body::text) AS size FROM workspace.project_change_command WHERE org_id=? AND project_id=? LIMIT ?)
 UNION ALL
 (SELECT 'folder' AS family,octet_length(response_body::text) AS size FROM workspace.project_folder_command WHERE org_id=? LIMIT ?)
) retained`, actor.OrgID, project, maxFacts+1, actor.OrgID, maxFacts+1).Scan(&budget).Error; err != nil {
		return false, fmt.Errorf("bound permanent workspace reference history: %w", err)
	}
	if budget.ProjectFacts > maxFacts || budget.FolderFacts > maxFacts || budget.Bytes > 64<<20 || budget.Largest > 1<<20 {
		return false, application.ErrProjectDependencyUnavailable
	}
	// A concurrent directory writer may append after preflight. Each body is
	// capped in SQL and each row is consumed immediately through the same
	// budgets, without retaining an unvalidated array on the caller connection.
	rows, err := tx.Raw(`SELECT family,actor_id,action,body FROM (
 (SELECT 'project' AS family,id,actor_id,action,CASE WHEN octet_length(response_body::text) BETWEEN 1 AND 1048576 THEN response_body ELSE NULL END AS body FROM workspace.project_change_command WHERE org_id=? AND project_id=? ORDER BY id LIMIT ?)
 UNION ALL
 (SELECT 'folder' AS family,id,actor_id,action,CASE WHEN octet_length(response_body::text) BETWEEN 1 AND 1048576 THEN response_body ELSE NULL END AS body FROM workspace.project_folder_command WHERE org_id=? ORDER BY id LIMIT ?)
) retained ORDER BY family DESC,id`, actor.OrgID, project, maxFacts+1, actor.OrgID, maxFacts+1).Rows()
	if err != nil {
		return false, fmt.Errorf("read permanent workspace reference history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	projectFacts, folderFacts, totalBytes := 0, 0, 0
	for rows.Next() {
		var family, action string
		var actorID uuid.UUID
		var body []byte
		if err := rows.Scan(&family, &actorID, &action, &body); err != nil {
			return false, fmt.Errorf("scan permanent workspace reference history: %w", err)
		}
		totalBytes += len(body)
		switch family {
		case "project":
			projectFacts++
		case "folder":
			folderFacts++
		default:
			return false, application.ErrProjectDependencyUnavailable
		}
		if projectFacts > maxFacts || folderFacts > maxFacts || totalBytes > 64<<20 || len(body) == 0 {
			return false, application.ErrProjectDependencyUnavailable
		}
		if family == "project" {
			var saved application.ProjectSnapshot
			if decodeWorkspaceReferenceHistory(body, &saved) != nil || saved.Validate() != nil || saved.Project.ID != project || saved.Project.OrgID != actor.OrgID {
				return false, application.ErrProjectDependencyUnavailable
			}
			if saved.Project.CoverAssetID != nil && *saved.Project.CoverAssetID == asset {
				return true, nil
			}
			continue
		}
		switch action {
		case "create", "patch", "move", "recycle":
		default:
			return false, application.ErrProjectDependencyUnavailable
		}
		var saved application.FolderChangeResult
		if decodeWorkspaceReferenceHistory(body, &saved) != nil {
			return false, application.ErrProjectDependencyUnavailable
		}
		if saved.Folder != nil {
			if saved.Folder.Validate() != nil || saved.Folder.OrgID != actor.OrgID || saved.Folder.ActorID != actorID {
				return false, application.ErrProjectDependencyUnavailable
			}
			if saved.Folder.Cover != nil && saved.Folder.Cover.ProjectID == project && saved.Folder.Cover.AssetID == asset {
				return true, nil
			}
		} else if action != "move" {
			return false, application.ErrProjectDependencyUnavailable
		}
		if saved.Placement != nil && (saved.Placement.Validate() != nil || saved.Placement.OrgID != actor.OrgID || saved.Placement.ActorID != actorID) {
			return false, application.ErrProjectDependencyUnavailable
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read permanent workspace reference history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return false, fmt.Errorf("close permanent workspace reference history: %w", err)
	}
	return false, nil
}
func decodeWorkspaceReferenceHistory(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > 1<<20 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return application.ErrProjectDependencyUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return application.ErrProjectDependencyUnavailable
	}
	return nil
}
