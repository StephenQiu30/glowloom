package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// ProjectCopyAccessFactory binds one workspace-minted admission or actual claimed worker to own SQL.
type ProjectCopyAccessFactory func(*gorm.DB) application.ProjectCopyAccess

// ProjectCopyMediaFactory binds the media owner's exact frozen plan to the caller transaction.
type ProjectCopyMediaFactory func(*gorm.DB) application.ProjectCopyMedia

// ProjectCopyScopesFactory binds the script owner's exact historic identity plan to caller SQL.
type ProjectCopyScopesFactory func(*gorm.DB) application.ProjectCopyScopes

// ProjectCopyStore preserves complete Bible history and permanent exact transfer/publication proofs.
type ProjectCopyStore struct {
	db     *gorm.DB
	access ProjectCopyAccessFactory
	media  ProjectCopyMediaFactory
	scopes ProjectCopyScopesFactory
}

// NewProjectCopyStore injects explicit owning authority and typed foreign-owner planning ports.
func NewProjectCopyStore(db *gorm.DB, access ProjectCopyAccessFactory, media ProjectCopyMediaFactory, scopes ProjectCopyScopesFactory) *ProjectCopyStore {
	return &ProjectCopyStore{db: db, access: access, media: media, scopes: scopes}
}

func (s *ProjectCopyStore) transaction(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, caller bool, f func(*gorm.DB) error) error {
	if s == nil || s.db == nil || s.db.Statement == nil || s.access == nil {
		return application.ErrUnavailable
	}
	_, existing := s.db.Statement.ConnPool.(gorm.TxCommitter)
	run := func(tx *gorm.DB) error {
		access := s.access(tx)
		if access == nil {
			return application.ErrUnavailable
		}
		if err := access.Authorize(ctx, actor, b, false); err != nil {
			return err
		}
		if err := access.Authorize(ctx, actor, b, true); err != nil {
			return err
		}
		return f(tx)
	}
	if caller {
		if !existing {
			return application.ErrUnavailable
		}
		return run(s.db.WithContext(ctx))
	}
	if existing {
		return run(s.db.WithContext(ctx))
	}
	return s.db.WithContext(ctx).Transaction(run)
}
func bibleCopySHA(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func bibleCopySnapshot(m application.CopyManifest) (application.ProjectCopySnapshot, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	return application.ProjectCopySnapshot{ID: uuid.NewSHA1(m.Binding.JobID, []byte("bible-history")), ManifestSHA256: bibleCopySHA(data), ContentSHA256: m.ContentSHA256, Counts: m.Counts, Identities: m.Identities, Versions: m.Versions}, nil
}
func equalBibleSnapshot(a, b application.ProjectCopySnapshot) bool {
	return a.ID == b.ID && a.ManifestSHA256 == b.ManifestSHA256 && a.ContentSHA256 == b.ContentSHA256 && a.Counts == b.Counts && slices.Equal(a.Identities, b.Identities) && slices.Equal(a.Versions, b.Versions)
}

// ReferencedMediaFacts reads every exact historical binding under trusted caller-owned admission SQL.
// No current Library visibility filter or physical object I/O can drop retained historical evidence.
func (s *ProjectCopyStore) ReferencedMediaFacts(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding) ([]domain.MediaFact, error) {
	var facts []domain.MediaFact
	err := s.transaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		h, err := readBibleHistory(ctx, tx, actor, b.SourceProjectID)
		if err != nil {
			return err
		}
		facts = application.HistoryMediaFacts(h)
		return nil
	})
	return facts, err
}

// ReferencedMedia returns the deterministic unique asset identities required by all history bindings.
func (s *ProjectCopyStore) ReferencedMedia(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding) ([]uuid.UUID, error) {
	facts, err := s.ReferencedMediaFacts(ctx, actor, b)
	if err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]bool)
	ids := make([]uuid.UUID, 0, len(facts))
	for _, fact := range facts {
		if !seen[fact.AssetID] {
			seen[fact.AssetID] = true
			ids = append(ids, fact.AssetID)
		}
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return ids, nil
}

// Freeze plans all typed history in the coordinator's admission transaction without remote I/O.
func (s *ProjectCopyStore) Freeze(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, assets map[uuid.UUID]uuid.UUID, at time.Time) (application.ProjectCopySnapshot, error) {
	var result application.ProjectCopySnapshot
	err := s.transaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		history, err := readBibleHistory(ctx, tx, actor, b.SourceProjectID)
		if err != nil {
			return err
		}
		facts, lookScopes := application.HistoryMediaFacts(history), application.HistoryLookScopes(history)
		var refs []application.ReferenceMapping
		var scopes []application.ScopeMapping
		if len(facts) > 0 {
			if s.media == nil || s.media(tx) == nil {
				return application.ErrUnavailable
			}
			refs, err = s.media(tx).FreezeReferences(ctx, actor, b, facts, assets)
			if err != nil {
				return err
			}
		}
		if len(lookScopes) > 0 {
			if s.scopes == nil || s.scopes(tx) == nil {
				return application.ErrUnavailable
			}
			scopes, err = s.scopes(tx).RemapLookScopes(ctx, actor, b, lookScopes)
			if err != nil {
				return err
			}
		}
		manifest, err := application.RemapProjectHistory(b, history, refs, scopes)
		if err != nil {
			return err
		}
		result, err = bibleCopySnapshot(manifest)
		if err != nil {
			return err
		}
		data, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		return exactlyOne(tx.Exec(`INSERT INTO bible.copy_snapshot(id,job_id,org_id,source_project_id,target_project_id,manifest,manifest_sha256,content_sha256,created_at) VALUES(?,?,?,?,?,?::jsonb,?,?,?)`, result.ID, b.JobID, b.OrgID, b.SourceProjectID, b.TargetProjectID, string(data), result.ManifestSHA256, result.ContentSHA256, at.UTC()))
	})
	return result, err
}

func readBibleManifest(tx *gorm.DB, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.CopyManifest, error) {
	var row struct {
		JobID, OrgID, SourceProjectID, TargetProjectID uuid.UUID
		Manifest, ManifestSHA256, ContentSHA256        string
	}
	read := tx.Raw(`SELECT job_id,org_id,source_project_id,target_project_id,manifest::text,manifest_sha256,content_sha256 FROM bible.copy_snapshot WHERE id=?`, snapshot.ID).Scan(&row)
	if read.Error != nil {
		return application.CopyManifest{}, read.Error
	}
	if read.RowsAffected != 1 || row.JobID != b.JobID || row.OrgID != b.OrgID || row.SourceProjectID != b.SourceProjectID || row.TargetProjectID != b.TargetProjectID || row.ManifestSHA256 != snapshot.ManifestSHA256 || row.ContentSHA256 != snapshot.ContentSHA256 {
		return application.CopyManifest{}, domain.ErrCorruptHistory
	}
	var m application.CopyManifest
	if json.Unmarshal([]byte(row.Manifest), &m) != nil {
		return m, domain.ErrCorruptHistory
	}
	proof, err := bibleCopySnapshot(m)
	if err != nil || m.Binding != b || !equalBibleSnapshot(proof, snapshot) {
		return m, domain.ErrCorruptHistory
	}
	// Rebuild from original source rather than accepting self-consistent tampered target facts.
	expected, err := application.RemapProjectHistory(b, m.Source, m.References, m.Scopes)
	if err != nil {
		return m, err
	}
	a, err := json.Marshal(expected)
	if err != nil {
		return m, err
	}
	actual, err := json.Marshal(m)
	if err != nil {
		return m, err
	}
	if string(a) != string(actual) {
		return m, domain.ErrCorruptHistory
	}
	return m, nil
}

// Manifest rechecks the exact current claimed worker before returning private frozen content.
func (s *ProjectCopyStore) Manifest(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.CopyManifest, error) {
	var manifest application.CopyManifest
	err := s.transaction(ctx, actor, b, false, func(tx *gorm.DB) error { var err error; manifest, err = readBibleManifest(tx, b, snapshot); return err })
	return manifest, err
}

// ConfirmTransfer records actual foreign media verification under a fresh current worker transaction.
func (s *ProjectCopyStore) ConfirmTransfer(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) error {
	return s.transaction(ctx, actor, b, false, func(tx *gorm.DB) error {
		if _, err := readBibleManifest(tx, b, snapshot); err != nil {
			return err
		}
		return writeBibleCopyProof(tx, "bible.copy_transfer_receipt", snapshot)
	})
}
func writeBibleCopyProof(tx *gorm.DB, table string, snapshot application.ProjectCopySnapshot) error {
	var prior struct{ ManifestSHA256, ContentSHA256 string }
	read := tx.Table(table).Select("manifest_sha256,content_sha256").Where("snapshot_id=?", snapshot.ID).Scan(&prior)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected == 1 {
		if prior.ManifestSHA256 != snapshot.ManifestSHA256 || prior.ContentSHA256 != snapshot.ContentSHA256 {
			return domain.ErrCorruptHistory
		}
		return nil
	}
	if read.RowsAffected != 0 {
		return domain.ErrCorruptHistory
	}
	return exactlyOne(tx.Table(table).Create(map[string]any{"snapshot_id": snapshot.ID, "manifest_sha256": snapshot.ManifestSHA256, "content_sha256": snapshot.ContentSHA256, "created_at": time.Now().UTC()}))
}
func requireBibleCopyProof(tx *gorm.DB, table string, snapshot application.ProjectCopySnapshot) error {
	var n int64
	if err := tx.Table(table).Where("snapshot_id=? AND manifest_sha256=? AND content_sha256=?", snapshot.ID, snapshot.ManifestSHA256, snapshot.ContentSHA256).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return application.ErrUnavailable
	}
	return nil
}

func verifyBibleTarget(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, b application.ProjectCopyBinding, m application.CopyManifest) error {
	actual, err := readBibleHistory(ctx, tx, actor, b.TargetProjectID)
	if err != nil {
		return err
	}
	expected, err := json.Marshal(m.Target)
	if err != nil {
		return err
	}
	data, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	if string(expected) != string(data) {
		return fmt.Errorf("copied Bible history differs: %w", domain.ErrCorruptHistory)
	}
	return nil
}

// Register inserts and independently rereads all target histories in the coordinator's transaction.
func (s *ProjectCopyStore) Register(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.ProjectCopyReceipt, error) {
	var receipt application.ProjectCopyReceipt
	err := s.transaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		m, err := readBibleManifest(tx, b, snapshot)
		if err != nil {
			return err
		}
		if err := requireBibleCopyProof(tx, "bible.copy_transfer_receipt", snapshot); err != nil {
			return err
		}
		var count int64
		if err := tx.Table("bible.copy_receipt").Where("snapshot_id=?", snapshot.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			existing, err := readBibleHistory(ctx, tx, actor, b.TargetProjectID)
			if err != nil {
				return err
			}
			if len(existing.Heads) != 0 {
				return application.ErrConflict
			}
			if err := insertBibleHistory(ctx, tx, actor, b.TargetProjectID, m.Target); err != nil {
				return err
			}
		}
		if err := verifyBibleTarget(ctx, tx, actor, b, m); err != nil {
			return err
		}
		if err := writeBibleCopyProof(tx, "bible.copy_receipt", snapshot); err != nil {
			return err
		}
		receipt = application.ProjectCopyReceipt{ManifestSHA256: snapshot.ManifestSHA256, ContentSHA256: snapshot.ContentSHA256, Counts: snapshot.Counts}
		return nil
	})
	return receipt, err
}

// Verify rereads complete published-to-private-target facts before the coordinator can activate it.
func (s *ProjectCopyStore) Verify(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.ProjectCopyReceipt, error) {
	var receipt application.ProjectCopyReceipt
	err := s.transaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		m, err := readBibleManifest(tx, b, snapshot)
		if err != nil {
			return err
		}
		if err := requireBibleCopyProof(tx, "bible.copy_receipt", snapshot); err != nil {
			return err
		}
		if err := verifyBibleTarget(ctx, tx, actor, b, m); err != nil {
			return err
		}
		receipt = application.ProjectCopyReceipt{ManifestSHA256: snapshot.ManifestSHA256, ContentSHA256: snapshot.ContentSHA256, Counts: snapshot.Counts}
		return nil
	})
	return receipt, err
}

// Cleanup seals only this job's hidden target history; immutable rows are retained and never published.
// Physical object cleanup belongs to media. Current authority must prove this target remains copying.
func (s *ProjectCopyStore) Cleanup(ctx context.Context, actor identityapp.Principal, b application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) error {
	return s.transaction(ctx, actor, b, true, func(tx *gorm.DB) error {
		m, err := readBibleManifest(tx, b, snapshot)
		if err != nil {
			return err
		}
		h, err := readBibleHistory(ctx, tx, actor, b.TargetProjectID)
		if err != nil {
			return err
		}
		if len(h.Heads) > 0 {
			if err := verifyBibleTarget(ctx, tx, actor, b, m); err != nil {
				return err
			}
		}
		return writeBibleCopyProof(tx, "bible.copy_cleanup_receipt", snapshot)
	})
}
