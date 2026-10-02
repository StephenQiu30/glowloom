package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

const (
	maxMediaReferenceFacts        = 50000
	maxMediaReferencePayloadBytes = 1 << 20
	maxMediaReferenceHistoryBytes = 64 << 20
)

// MediaReferenceGuard retains the caller's current project lock while reading own history.
type MediaReferenceGuard struct {
	tx     *gorm.DB
	access application.ProjectAccess
}

// NewMediaReferenceGuard injects an existing content transaction and live workspace authority.
func NewMediaReferenceGuard(tx *gorm.DB, access application.ProjectAccess) *MediaReferenceGuard {
	return &MediaReferenceGuard{tx: tx, access: access}
}

// HasMediaReferences includes retained immutable appearances and sample voices.
// It performs no media reads, object I/O or content mutation.
func (g *MediaReferenceGuard) HasMediaReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	if g == nil || g.tx == nil || g.tx.Statement == nil || g.access == nil || project == uuid.Nil || asset == uuid.Nil {
		return false, application.ErrUnavailable
	}
	if _, ok := g.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, application.ErrUnavailable
	}
	facts, err := g.access.Authorize(ctx, actor, project, false)
	if err != nil {
		return false, err
	}
	if facts.ProjectID != project || facts.OrgID != actor.OrgID || facts.Revision < 1 {
		return false, application.ErrUnavailable
	}
	// Read immutable versions even when their current head, appearance or voice
	// has been removed. LoadVersion also checks the typed materialized rows;
	// corrupt or missing sample evidence cannot be interpreted as no reference.
	tx := g.tx.WithContext(ctx)
	// Check every fact LoadVersion can materialize before loading any JSON or
	// text. The current project lock serializes owning history writers. Keep
	// nested LoadVersion reads on a free connection, rather than an open Rows.
	var history struct{ Facts, Bytes, MaxBytes int64 }
	budget := tx.Raw(`SELECT count(*) AS facts,coalesce(sum(payload_bytes),0) AS bytes,coalesce(max(payload_bytes),0) AS max_bytes FROM (
 SELECT octet_length(content::text)::bigint+coalesce(octet_length(result_source::text),0) AS payload_bytes FROM bible.character_version WHERE org_id=? AND project_id=?
 UNION ALL SELECT octet_length(name)::bigint+octet_length(description)+coalesce(octet_length(applies_to::text),0) FROM bible.look_version WHERE org_id=? AND project_id=?
 UNION ALL SELECT octet_length(row_to_json(r)::text)::bigint FROM bible.reference_version r WHERE org_id=? AND project_id=?
 UNION ALL SELECT octet_length(content::text)::bigint+octet_length(source_kind) FROM bible.voice_version WHERE org_id=? AND project_id=?
 LIMIT ?
) retained`, actor.OrgID, project, actor.OrgID, project, actor.OrgID, project, actor.OrgID, project, maxMediaReferenceFacts+1).Scan(&history)
	if budget.Error != nil {
		return false, budget.Error
	}
	if budget.RowsAffected != 1 || history.Facts > maxMediaReferenceFacts || history.Bytes > maxMediaReferenceHistoryBytes || history.MaxBytes > maxMediaReferencePayloadBytes {
		return false, application.ErrUnavailable
	}
	var versions []struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM bible.character_version WHERE org_id=? AND project_id=? ORDER BY id LIMIT ?`, actor.OrgID, project, maxMediaReferenceFacts+1).Scan(&versions).Error; err != nil {
		return false, err
	}
	if len(versions) > maxMediaReferenceFacts {
		return false, application.ErrUnavailable
	}
	reader := &bound{tx: tx, actor: actor, project: facts}
	for _, row := range versions {
		version, err := reader.LoadVersion(ctx, domain.KindCharacter, row.ID)
		if err != nil {
			return false, err
		}
		for _, look := range version.Character.Looks {
			for _, ref := range look.References {
				if ref.Media.AssetID == asset {
					return true, nil
				}
			}
		}
		voice := version.Character.Voice
		if voice != nil && voice.Kind == domain.VoiceSample && voice.Sample.Media.AssetID == asset {
			return true, nil
		}
	}
	return false, nil
}
