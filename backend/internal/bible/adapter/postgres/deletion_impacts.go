package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/bible/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// CharacterUsageReader is the downstream projection required by soft deletion.
type CharacterUsageReader interface {
	Read(context.Context, identityapp.Principal, uuid.UUID, []uuid.UUID) (application.CharacterUsage, error)
}

// DeletionImpacts checks deletion only; it must not silently implement merge or look changes.
type DeletionImpacts struct {
	tx     *gorm.DB
	access application.ProjectAccess
	usage  CharacterUsageReader
}

// NewDeletionImpacts binds Bible identity facts and the actual script owner to one transaction.
func NewDeletionImpacts(tx *gorm.DB, access application.ProjectAccess, usage CharacterUsageReader) *DeletionImpacts {
	return &DeletionImpacts{tx: tx, access: access, usage: usage}
}

// Read freezes the complete current reference evidence without modifying history.
func (d *DeletionImpacts) Read(ctx context.Context, actor identityapp.Principal, project uuid.UUID, input application.ImpactInput) (application.ImpactProof, error) {
	result := application.ImpactProof{}
	if d == nil || d.tx == nil || d.access == nil || d.usage == nil || input.Action != "delete" || !input.Kind.Valid() || input.EntryID == uuid.Nil || input.VersionID == uuid.Nil || input.LookID != nil {
		return result, application.ErrUnavailable
	}
	if _, ok := d.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return result, application.ErrUnavailable
	}
	facts, err := d.access.Authorize(ctx, actor, project, true)
	if err != nil {
		return result, err
	}
	if facts.ProjectID != project || facts.OrgID != actor.OrgID {
		return result, application.ErrUnavailable
	}
	type identity struct {
		ID       uuid.UUID
		Revision int64
	}
	identities := []identity{}
	usage := application.CharacterUsage{}
	if input.Kind == domain.KindCharacter {
		// UNION, not UNION ALL, terminates even if a corrupt redirect cycle exists.
		err = d.tx.WithContext(ctx).Raw(`WITH RECURSIVE aliases AS (
   SELECT id,revision FROM bible.character WHERE org_id=? AND project_id=? AND id=?
   UNION SELECT c.id,c.revision FROM bible.character c JOIN aliases a ON c.redirect_id=a.id WHERE c.org_id=? AND c.project_id=?
  ) SELECT id,revision FROM aliases ORDER BY id`, actor.OrgID, project, input.EntryID, actor.OrgID, project).Scan(&identities).Error
		if err != nil {
			return result, err
		}
		if len(identities) == 0 {
			return result, application.ErrNotFound
		}
		ids := make([]uuid.UUID, len(identities))
		for i, item := range identities {
			ids[i] = item.ID
		}
		usage, err = d.usage.Read(ctx, actor, project, ids)
		if err != nil {
			return result, errors.Join(application.ErrUnavailable, err)
		}
		decoded, decodeErr := hex.DecodeString(usage.SHA256)
		if decodeErr != nil || len(decoded) != sha256.Size || usage.CurrentLines < 0 || usage.HistoricalLines < 0 {
			return result, application.ErrUnavailable
		}
		result.Aliases = int64(len(identities) - 1)
	}
	// Location and prop have no identity-bearing downstream contract in the current schema.
	data, err := json.Marshal(struct {
		Org, Project uuid.UUID
		Input        application.ImpactInput
		Revision     int64
		Identities   []identity
		Usage        application.CharacterUsage
	}{actor.OrgID, project, input, facts.Revision, identities, usage})
	if err != nil {
		return result, err
	}
	hash := sha256.Sum256(data)
	result.SHA256 = hex.EncodeToString(hash[:])
	result.Revision = facts.Revision
	result.CurrentLines = usage.CurrentLines
	result.HistoricalLines = usage.HistoricalLines
	return result, nil
}

// Apply rechecks the accepted facts while the project write lock is still held.
// Historical records need no mutation: the Bible head controls future admission.
func (d *DeletionImpacts) Apply(ctx context.Context, actor identityapp.Principal, project uuid.UUID, input application.ImpactInput, proof application.ImpactProof) error {
	current, err := d.Read(ctx, actor, project, input)
	if err != nil {
		return err
	}
	if current != proof {
		return &application.ImpactConflict{Proof: current}
	}
	return nil
}
