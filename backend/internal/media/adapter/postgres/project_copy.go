package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// ProjectCopyStore owns media snapshots, immutable object intents and target registration.
// Workspace injects its admission/fenced transaction; no method reads its copy job.
type ProjectCopyStore struct {
	routed bool
	db     *gorm.DB
	scopeAuthority
}

// NewProjectCopyStore injects an owning coordinator's transaction.
func NewProjectCopyStore(tx *gorm.DB) *ProjectCopyStore { return &ProjectCopyStore{db: tx} }

func (s scopeAuthority) copyMediaTarget(tx *gorm.DB, actor identityapp.Principal, binding application.ProjectCopyBinding) error {
	if binding.Validate() != nil || binding.OrgID != actor.OrgID {
		return application.ErrProjectCopyMediaUnavailable
	}
	if err := s.requireCurrentActor(tx, actor); err != nil {
		return err
	}
	if err := s.requireCopyTarget(tx, actor, binding.TargetProjectID); err != nil {
		return err
	}
	var project struct{ ID uuid.UUID }
	read := tx.Table("workspace.project").Select("id").Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND org_id = ? AND status = ? AND is_delete = ?", binding.TargetProjectID, binding.OrgID, workspacedomain.ProjectStatusCopying, false).Take(&project)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	if read.Error != nil {
		return fmt.Errorf("read private media copy target: %w", read.Error)
	}
	return nil
}

func strictCopyMedia(body []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return application.ErrProjectCopyMediaUnavailable
	}
	return nil
}

func copyMediaDigest(content application.ProjectMediaCopy, mapping map[uuid.UUID]uuid.UUID) (string, []byte, []byte, error) {
	body, err := json.Marshal(content)
	if err != nil || len(body) > 32<<20 {
		return "", nil, nil, application.ErrProjectCopyMediaUnavailable
	}
	assetMap, err := json.Marshal(mapping)
	if err != nil {
		return "", nil, nil, err
	}
	hash := sha256.Sum256(append(append(bytes.Clone(body), '\n'), assetMap...))
	return hex.EncodeToString(hash[:]), body, assetMap, nil
}

// Freeze records all live assets or fails on an unavailable item; it never drops one.
func (s *ProjectCopyStore) Freeze(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, now time.Time) (application.ProjectCopySnapshot, error) {
	return s.freezeWithReferences(ctx, actor, binding, now, nil)
}

// FreezeWithReferences adds only explicitly referenced, retained historical
// documents. Public reads and ordinary project references remain unchanged.
func (s *ProjectCopyStore) FreezeWithReferences(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, now time.Time, referenced []uuid.UUID) (application.ProjectCopySnapshot, error) {
	if len(referenced) > 4096 {
		return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
	}
	seen := make(map[uuid.UUID]bool, len(referenced))
	for _, id := range referenced {
		if id == uuid.Nil || seen[id] {
			return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
		}
		seen[id] = true
	}
	return s.freezeWithReferences(ctx, actor, binding, now, referenced)
}

func (s *ProjectCopyStore) freezeWithReferences(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, now time.Time, referenced []uuid.UUID) (application.ProjectCopySnapshot, error) {
	return s.freezeWithReferenceFacts(ctx, actor, binding, now, referenced, nil)
}

// FreezeWithReferenceFacts adds trusted immutable image/audio bindings to the
// complete copy. It must run in the owning admission transaction: all validation
// failures roll back its snapshot/intents along with the outer command.
// External Script references remain restricted to documents.
func (s *ProjectCopyStore) FreezeWithReferenceFacts(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, now time.Time, referenced []uuid.UUID, facts []application.ReferenceFact) (application.ProjectCopySnapshot, error) {
	if len(referenced) > 4096 || len(facts) > 4096 {
		return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
	}
	seen := make(map[uuid.UUID]bool, len(referenced))
	for _, id := range referenced {
		if id == uuid.Nil || seen[id] {
			return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
		}
		seen[id] = true
	}
	for _, fact := range facts {
		if err := application.ValidateCopyReferenceFact(fact); err != nil {
			return application.ProjectCopySnapshot{}, err
		}
	}
	snapshot, err := s.freezeWithReferenceFacts(ctx, actor, binding, now, referenced, facts)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	if _, err := application.NewProjectCopyReferenceQuery(s).FreezeReferences(ctx, actor, binding, snapshot, facts); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	return snapshot, nil
}

func (s *ProjectCopyStore) freezeWithReferenceFacts(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, now time.Time, referenced []uuid.UUID, facts []application.ReferenceFact) (application.ProjectCopySnapshot, error) {
	if s == nil || s.db == nil {
		return application.ProjectCopySnapshot{}, application.ErrUnavailable
	}
	tx := s.db.WithContext(ctx)
	if err := s.copyMediaTarget(tx, actor, binding); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	if err := s.requireProject(tx, actor, binding.SourceProjectID, false); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	librarySource, err := readCopySourceLibrary(tx, actor, binding)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	// Library declares every binary catalog row, including recycled/removed
	// originals. Script's caller references remain restricted to documents below.
	libraryRefs := make(map[uuid.UUID]bool, len(librarySource.items))
	scriptRefs := make(map[uuid.UUID]bool, len(referenced))
	selectedRefs := append([]uuid.UUID(nil), referenced...)
	seenRefs := make(map[uuid.UUID]bool, len(referenced))
	for _, id := range referenced {
		scriptRefs[id] = true
		seenRefs[id] = true
	}
	referenceRefs := make(map[uuid.UUID]bool, len(facts))
	for _, fact := range facts {
		referenceRefs[fact.AssetID] = true
		if !seenRefs[fact.AssetID] {
			selectedRefs = append(selectedRefs, fact.AssetID)
			seenRefs[fact.AssetID] = true
		}
	}
	for _, item := range librarySource.items {
		if item.AssetID == nil {
			continue
		}
		id := *item.AssetID
		libraryRefs[id] = true
		if !seenRefs[id] {
			selectedRefs = append(selectedRefs, id)
			seenRefs[id] = true
		}
	}
	if len(selectedRefs) > 4096 {
		return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
	}
	slices.SortFunc(selectedRefs, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	var rows []assetRow
	query := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("project_id = ? AND is_delete = ?", binding.SourceProjectID, false)
	if len(selectedRefs) != 0 {
		query = tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("project_id = ? AND (is_delete = ? OR id IN ?)", binding.SourceProjectID, false, selectedRefs)
	}
	if err := query.Order("id").Limit(4097).Find(&rows).Error; err != nil {
		return application.ProjectCopySnapshot{}, fmt.Errorf("freeze media assets: %w", err)
	}
	if len(rows) > 4096 {
		return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
	}
	selected := make(map[uuid.UUID]assetRow, len(rows))
	assetIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		selected[row.ID], assetIDs = row, append(assetIDs, row.ID)
	}
	if err := requireNoPurgeReservation(tx, assetIDs); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	for _, id := range referenced {
		row, present := selected[id]
		if !present || row.Kind != string(domain.KindDocument) {
			return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
		}
	}
	for id := range libraryRefs {
		if _, present := selected[id]; !present {
			return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
		}
	}
	for id := range referenceRefs {
		if _, present := selected[id]; !present {
			return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
		}
	}
	var renditions []renditionRow
	renditionQuery := tx.Table("media.rendition AS r").Select("r.*").Joins("JOIN media.media_asset AS a ON a.id = r.media_asset_id").Clauses(clause.Locking{Strength: "SHARE"}).Where("a.project_id = ? AND a.is_delete = ? AND r.is_delete = ?", binding.SourceProjectID, false, false)
	if len(selectedRefs) != 0 {
		renditionQuery = tx.Table("media.rendition AS r").Select("r.*").Joins("JOIN media.media_asset AS a ON a.id = r.media_asset_id").Clauses(clause.Locking{Strength: "SHARE"}).Where("a.project_id = ? AND a.id IN ? AND r.is_delete = ?", binding.SourceProjectID, assetIDs, false)
	}
	if err := renditionQuery.Order("r.media_asset_id").Order("r.kind").Order("r.id").Find(&renditions).Error; err != nil {
		return application.ProjectCopySnapshot{}, fmt.Errorf("freeze media renditions: %w", err)
	}
	byAsset := make(map[uuid.UUID][]domain.Rendition)
	for _, row := range renditions {
		if err := hydrateCopySourceRendition(tx, &row); err != nil {
			return application.ProjectCopySnapshot{}, err
		}
		byAsset[row.MediaAssetID] = append(byAsset[row.MediaAssetID], row.domain())
	}
	sources := make([]application.ProjectCopySourceAsset, 0, len(rows))
	for _, row := range rows {
		if err := hydrateCopySourceAsset(tx, &row); err != nil {
			return application.ProjectCopySnapshot{}, err
		}
		source := application.ProjectCopySourceAsset{Asset: row.domain(), Renditions: byAsset[row.ID]}
		if source.Asset.IsDelete {
			if source.Asset.DeleteTime == nil || source.Asset.PurgeAfter == nil {
				return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
			}
			source.RetainedHistory = &application.RetainedHistoryProof{AssetID: source.Asset.ID, Revision: source.Asset.Revision, DeletedAt: *source.Asset.DeleteTime, PurgeAfter: *source.Asset.PurgeAfter}
			switch {
			case libraryRefs[row.ID]:
				source.RetainedHistory.DeclaredBy = application.RetainedHistoryLibrary
			case referenceRefs[row.ID]:
				source.RetainedHistory.DeclaredBy = application.RetainedHistoryMediaReference
			case scriptRefs[row.ID]:
				source.RetainedHistory.DeclaredBy = application.RetainedHistoryScriptDocument
			default:
				return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
			}
		}
		sources = append(sources, source)
	}
	content, err := application.PrepareProjectMediaCopy(binding, sources, now)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	snapshotID := uuid.NewSHA1(binding.JobID, []byte("media-snapshot"))
	if s.routed {
		if err := bindCopyLocations(ctx, tx, projectCopyWorkOwner(binding.TargetProjectID, snapshotID), content.Objects); err != nil {
			return application.ProjectCopySnapshot{}, err
		}
		if err := attachCopyAssetLocations(content.Assets, content.Objects); err != nil {
			return application.ProjectCopySnapshot{}, err
		}
	}
	mapping := make(map[uuid.UUID]uuid.UUID, len(sources))
	for i, source := range sources {
		mapping[source.Asset.ID] = content.Assets[i].Asset.ID
	}
	content.Library, err = application.PrepareProjectLibraryCopy(binding, librarySource.row.ID, librarySource.row.Revision, librarySource.folders, librarySource.items, mapping, now)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	digest, body, assetMap, err := copyMediaDigest(content, mapping)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	snapshot := application.ProjectCopySnapshot{ID: snapshotID, ManifestSHA256: digest, Assets: len(sources), Renditions: len(renditions), AssetMapping: mapping}
	snapshotRecord := projectCopySnapshotRecord{ID: snapshot.ID, JobID: binding.JobID, OrgID: binding.OrgID, SourceProjectID: binding.SourceProjectID, TargetProjectID: binding.TargetProjectID, ManifestSHA256: digest, AssetCount: snapshot.Assets, RenditionCount: snapshot.Renditions, Content: jsonDocument(body), AssetMapping: jsonDocument(assetMap)}
	if err := tx.Create(&snapshotRecord).Error; err != nil {
		return application.ProjectCopySnapshot{}, fmt.Errorf("persist frozen media snapshot: %w", err)
	}
	for _, object := range content.Objects {
		objectRecord := copyObjectRow{SnapshotID: snapshot.ID, TargetAssetID: object.TargetAssetID, RenditionKind: object.RenditionKind, SourceObjectKey: object.SourceObjectKey, TargetObjectKey: object.TargetObjectKey, ByteSize: object.ByteSize, ContentType: object.ContentType, SHA256: object.SHA256, Status: application.ProjectCopyObjectPending, SourceLocationID: locationID(object.SourceLocation), TargetLocationID: locationID(object.TargetLocation)}
		if err := tx.Create(&objectRecord).Error; err != nil {
			return application.ProjectCopySnapshot{}, fmt.Errorf("persist private object identity: %w", err)
		}
	}
	return snapshot, nil
}

type copyObjectRow struct {
	SnapshotID                         uuid.UUID
	SourceLocationID, TargetLocationID *uuid.UUID
	TargetAssetID                      uuid.UUID
	RenditionKind                      string
	SourceObjectKey                    string
	TargetObjectKey                    string
	ByteSize                           *int64
	ContentType                        string
	SHA256                             *string
	Status                             application.ProjectCopyObjectStatus
	SourceVerified                     bool
	WriteStarted                       bool
}

func (copyObjectRow) TableName() string { return "media.project_copy_object" }

func (r copyObjectRow) object() application.ProjectCopyObject {
	return application.ProjectCopyObject{TargetAssetID: r.TargetAssetID, RenditionKind: r.RenditionKind, SourceObjectKey: r.SourceObjectKey, TargetObjectKey: r.TargetObjectKey, ByteSize: r.ByteSize, ContentType: r.ContentType, SHA256: r.SHA256, Status: r.Status, SourceVerified: r.SourceVerified, WriteStarted: r.WriteStarted}
}

func (s *ProjectCopyStore) load(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.ProjectMediaCopy, error) {
	tx := s.db.WithContext(ctx)
	if err := s.copyMediaTarget(tx, actor, binding); err != nil {
		return application.ProjectMediaCopy{}, err
	}
	var row struct {
		ManifestSHA256             string
		AssetCount, RenditionCount int
		Content, AssetMapping      jsonDocument
	}
	read := tx.Model(&projectCopySnapshotRecord{}).Select("manifest_sha256", "asset_count", "rendition_count", "content", "asset_mapping").Where("id = ? AND job_id = ? AND org_id = ? AND source_project_id = ? AND target_project_id = ?", snapshot.ID, binding.JobID, binding.OrgID, binding.SourceProjectID, binding.TargetProjectID).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.ProjectMediaCopy{}, application.ErrProjectCopyMediaUnavailable
	}
	if read.Error != nil {
		return application.ProjectMediaCopy{}, read.Error
	}
	if row.ManifestSHA256 != snapshot.ManifestSHA256 || row.AssetCount != snapshot.Assets || row.RenditionCount != snapshot.Renditions {
		return application.ProjectMediaCopy{}, application.ErrProjectCopyMediaUnavailable
	}
	var content application.ProjectMediaCopy
	var mapping map[uuid.UUID]uuid.UUID
	if strictCopyMedia(row.Content, &content) != nil || strictCopyMedia(row.AssetMapping, &mapping) != nil {
		return application.ProjectMediaCopy{}, application.ErrProjectCopyMediaUnavailable
	}
	if content.Library != nil && content.Library.Validate(binding) != nil {
		return application.ProjectMediaCopy{}, application.ErrProjectCopyMediaUnavailable
	}
	digest, _, _, err := copyMediaDigest(content, mapping)
	if err != nil || digest != snapshot.ManifestSHA256 || len(content.Assets) != snapshot.Assets || len(content.Objects) != snapshot.Assets+snapshot.Renditions {
		return application.ProjectMediaCopy{}, application.ErrProjectCopyMediaUnavailable
	}
	return content, nil
}

// Objects returns only snapshot-bound object intents; it accepts no caller object keys.
func (s *ProjectCopyStore) Objects(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) ([]application.ProjectCopyObject, error) {
	if s == nil || s.db == nil {
		return nil, application.ErrUnavailable
	}
	content, err := s.load(ctx, actor, binding, snapshot)
	if err != nil {
		return nil, err
	}
	var rows []copyObjectRow
	if err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("snapshot_id = ?", snapshot.ID).Order("target_asset_id").Order("rendition_kind").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(content.Objects) {
		return nil, application.ErrProjectCopyMediaUnavailable
	}
	frozen := make(map[string]application.ProjectCopyObject, len(content.Objects))
	for _, object := range content.Objects {
		frozen[object.TargetObjectKey] = object
	}
	result := make([]application.ProjectCopyObject, 0, len(rows))
	for _, row := range rows {
		original, found := frozen[row.TargetObjectKey]
		if !found || row.TargetAssetID != original.TargetAssetID || row.RenditionKind != original.RenditionKind || row.SourceObjectKey != original.SourceObjectKey || row.ContentType != original.ContentType || original.ByteSize != nil && (row.ByteSize == nil || *row.ByteSize != *original.ByteSize) || original.SHA256 != nil && (row.SHA256 == nil || *row.SHA256 != *original.SHA256) {
			return nil, application.ErrProjectCopyMediaUnavailable
		}
		value := row.object()
		source, err := readRowLocation(s.db.WithContext(ctx), row.SourceLocationID, row.SourceObjectKey)
		if err != nil {
			return nil, err
		}
		target, err := readRowLocation(s.db.WithContext(ctx), row.TargetLocationID, row.TargetObjectKey)
		if err != nil {
			return nil, err
		}
		if original.SourceLocation != nil && (source == nil || original.SourceLocation.ID != source.ID || original.SourceLocation.Ref != source.Ref) || original.TargetLocation != nil && (target == nil || original.TargetLocation.ID != target.ID || original.TargetLocation.Ref != target.Ref) {
			return nil, application.ErrObjectMismatch
		}
		value.SourceLocation, value.TargetLocation = source, target
		result = append(result, value)
	}
	return result, nil
}

// RecordObjectDigest durably records checked source bytes before the external write.
// Workspace holds its current worker fence before invoking this transaction method.
func (s *ProjectCopyStore) RecordObjectDigest(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, asset uuid.UUID, kind, digest string, size int64) error {
	objects, err := s.Objects(ctx, actor, binding, snapshot)
	if err != nil {
		return err
	}
	if len(digest) != 64 || size < 1 || size > 2<<30 {
		return application.ErrObjectMismatch
	}
	found := false
	for _, object := range objects {
		if object.TargetAssetID == asset && object.RenditionKind == kind {
			found = true
			if object.Status != application.ProjectCopyObjectPending || object.SHA256 != nil && *object.SHA256 != digest || object.ByteSize != nil && *object.ByteSize != size {
				return application.ErrObjectMismatch
			}
		}
	}
	if !found {
		return application.ErrProjectCopyMediaUnavailable
	}
	update := s.db.WithContext(ctx).Model(&copyObjectRow{}).Where("snapshot_id = ? AND target_asset_id = ? AND rendition_kind = ? AND status = ? AND (sha256 IS NULL OR sha256 = ?) AND (byte_size IS NULL OR byte_size = ?)", snapshot.ID, asset, kind, application.ProjectCopyObjectPending, digest, size).Updates(map[string]any{"sha256": digest, "byte_size": size, "source_verified": true, "update_time": time.Now().UTC()})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrObjectMismatch
	}
	return nil
}

// ConfirmObject records a target readback verified against its durable source digest.
func (s *ProjectCopyStore) ConfirmObject(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, asset uuid.UUID, kind, digest string, size int64) error {
	if _, err := s.load(ctx, actor, binding, snapshot); err != nil {
		return err
	}
	var object copyObjectRow
	read := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("snapshot_id = ? AND target_asset_id = ? AND rendition_kind = ?", snapshot.ID, asset, kind).Take(&object)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.ErrProjectCopyMediaUnavailable
	}
	if read.Error != nil {
		return read.Error
	}
	if object.TargetLocationID != nil {
		if err := confirmCopyLocation(ctx, s.db.WithContext(ctx), projectCopyWorkOwner(binding.TargetProjectID, snapshot.ID), object.TargetLocationID, object.TargetObjectKey, object.ContentType, digest, size); err != nil {
			return err
		}
	}
	update := s.db.WithContext(ctx).Model(&copyObjectRow{}).Where("snapshot_id = ? AND target_asset_id = ? AND rendition_kind = ? AND status IN ? AND source_verified = ? AND write_started = ? AND sha256 = ? AND byte_size = ?", snapshot.ID, asset, kind, []application.ProjectCopyObjectStatus{application.ProjectCopyObjectPending, application.ProjectCopyObjectVerified}, true, true, digest, size).Updates(map[string]any{"status": application.ProjectCopyObjectVerified, "update_time": time.Now().UTC()})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrObjectMismatch
	}
	return nil
}

// Register commits ready target metadata only after every object has a verified receipt.
func (s *ProjectCopyStore) Register(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.ProjectCopyReceipt, error) {
	content, err := s.load(ctx, actor, binding, snapshot)
	if err != nil {
		return application.ProjectCopyReceipt{}, err
	}
	objects, err := s.Objects(ctx, actor, binding, snapshot)
	if err != nil {
		return application.ProjectCopyReceipt{}, err
	}
	byKey := make(map[string]application.ProjectCopyObject, len(objects))
	for _, object := range objects {
		if object.Status != application.ProjectCopyObjectVerified || !object.SourceVerified || object.SHA256 == nil || object.ByteSize == nil {
			return application.ProjectCopyReceipt{}, application.ErrObjectMismatch
		}
		byKey[object.TargetObjectKey] = object
	}
	for i := range content.Assets {
		item := &content.Assets[i]
		object := byKey[item.Asset.ObjectKey]
		if object.TargetAssetID != item.Asset.ID {
			return application.ProjectCopyReceipt{}, application.ErrObjectMismatch
		}
		item.Asset.SHA256 = object.SHA256
		for j := range item.Renditions {
			r := &item.Renditions[j]
			object := byKey[r.ObjectKey]
			if object.TargetAssetID != item.Asset.ID || object.RenditionKind != string(r.Kind) {
				return application.ProjectCopyReceipt{}, application.ErrObjectMismatch
			}
			r.ByteSize = object.ByteSize
		}
	}
	body, err := copiedMediaContentBytes(content)
	if err != nil {
		return application.ProjectCopyReceipt{}, err
	}
	hash := sha256.Sum256(body)
	result := application.ProjectCopyReceipt{ManifestSHA256: snapshot.ManifestSHA256, ContentSHA256: hex.EncodeToString(hash[:]), Assets: snapshot.Assets, Renditions: snapshot.Renditions}
	var receipt projectCopyReceiptRecord
	replayed := s.db.WithContext(ctx).Where("snapshot_id = ?", snapshot.ID).Take(&receipt)
	if errors.Is(replayed.Error, gorm.ErrRecordNotFound) {
		replayed.Error = nil
	}
	if replayed.Error != nil {
		return application.ProjectCopyReceipt{}, replayed.Error
	}
	if receipt.SnapshotID != uuid.Nil {
		if receipt.ContentSHA256 != result.ContentSHA256 || receipt.AssetCount != result.Assets || receipt.RenditionCount != result.Renditions {
			return application.ProjectCopyReceipt{}, application.ErrObjectMismatch
		}
		if err := verifyCopiedMedia(s.db.WithContext(ctx), content.Assets, binding.TargetProjectID); err != nil {
			return application.ProjectCopyReceipt{}, err
		}
		if err := verifyCopiedLibrary(s.db.WithContext(ctx), actor, binding, content.Library); err != nil {
			return application.ProjectCopyReceipt{}, err
		}
		return result, nil
	}
	tx := s.db.WithContext(ctx)
	if len(objects) > 0 && objects[0].TargetLocation != nil {
		if err := acceptCopyLocations(ctx, tx, projectCopyWorkOwner(binding.TargetProjectID, snapshot.ID), objects); err != nil {
			return application.ProjectCopyReceipt{}, err
		}
	}
	for _, item := range content.Assets {
		a := item.Asset
		if a.Validate() != nil {
			return application.ProjectCopyReceipt{}, application.ErrProjectCopyMediaUnavailable
		}
		assetRecord := mediaAssetRecordFromDomain(a)
		if err := tx.Create(&assetRecord).Error; err != nil {
			return application.ProjectCopyReceipt{}, fmt.Errorf("register copied asset: %w", err)
		}
		for _, r := range item.Renditions {
			if r.Validate() != nil {
				return application.ProjectCopyReceipt{}, application.ErrProjectCopyMediaUnavailable
			}
			renditionRecord := mediaRenditionRecordFromDomain(r)
			if err := tx.Create(&renditionRecord).Error; err != nil {
				return application.ProjectCopyReceipt{}, fmt.Errorf("register copied rendition: %w", err)
			}
		}
	}
	if err := registerCopiedLibrary(tx, actor, binding, content.Library); err != nil {
		return application.ProjectCopyReceipt{}, err
	}
	receipt = projectCopyReceiptRecord{SnapshotID: snapshot.ID, ContentSHA256: result.ContentSHA256, AssetCount: result.Assets, RenditionCount: result.Renditions}
	if err := tx.Create(&receipt).Error; err != nil {
		return application.ProjectCopyReceipt{}, fmt.Errorf("record complete copied media: %w", err)
	}
	return result, nil
}

// ReferenceCopiedAsset exposes only receipt-verified assets belonging to this private job.
// It implements the existing canvas reference shape without enabling public previews.
func (s *ProjectCopyStore) ReferenceCopiedAsset(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, asset uuid.UUID) (application.AssetSummary, error) {
	if s == nil || s.db == nil {
		return application.AssetSummary{}, application.ErrUnavailable
	}
	if err := s.copyMediaTarget(s.db.WithContext(ctx), actor, binding); err != nil {
		return application.AssetSummary{}, err
	}
	var row assetRow
	read := s.db.WithContext(ctx).Table("media.media_asset AS a").Select("a.*").
		Joins("JOIN media.project_copy_object AS o ON o.target_asset_id = a.id AND o.target_object_key = a.object_key AND o.rendition_kind = ?", "").
		Joins("JOIN media.project_copy_snapshot AS s ON s.id = o.snapshot_id").
		Joins("JOIN media.project_copy_receipt AS r ON r.snapshot_id = s.id").
		Clauses(clause.Locking{Strength: "SHARE", Table: clause.Table{Name: "a"}}).
		Where("s.job_id = ? AND s.org_id = ? AND s.source_project_id = ? AND s.target_project_id = ? AND a.id = ? AND a.project_id = s.target_project_id", binding.JobID, binding.OrgID, binding.SourceProjectID, binding.TargetProjectID, asset).
		Where(map[string]any{"a.status": domain.StatusReady, "a.moderation_status": domain.ModerationPassed, "a.is_delete": false, "o.status": application.ProjectCopyObjectVerified, "o.source_verified": true}).
		Where("a.sha256 = o.sha256 AND a.byte_size = o.byte_size").Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.AssetSummary{}, application.ErrNotFound
	}
	if read.Error != nil {
		return application.AssetSummary{}, read.Error
	}
	if err := row.loadLocation(s.db.WithContext(ctx)); err != nil {
		return application.AssetSummary{}, err
	}
	a := row.domain()
	if a.Validate() != nil || a.ContainsRealPerson || a.ConsentRecordID != nil || (a.Kind != domain.KindImage && a.Kind != domain.KindVideo && a.Kind != domain.KindAudio && a.Kind != domain.KindModel) {
		return application.AssetSummary{}, application.ErrNotFound
	}
	return application.AssetSummary{ID: a.ID, ProjectID: a.ProjectID, Kind: string(a.Kind), FileName: a.FileName, MIMEType: a.MimeType, ByteSize: a.ByteSize, Width: a.Width, Height: a.Height, DurationMS: a.DurationMS, Revision: a.Revision}, nil
}

// AuthorizeObjectRemoval checks exact frozen key ownership and all potential metadata conflicts.
func (s *ProjectCopyStore) AuthorizeObjectRemoval(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	objects, err := s.Objects(ctx, actor, binding, snapshot)
	if err != nil {
		return err
	}
	var key string
	for _, object := range objects {
		if object.TargetAssetID == asset && object.RenditionKind == kind {
			key = object.TargetObjectKey
		}
	}
	if key == "" {
		return application.ErrNotFound
	}
	query := s.db.WithContext(ctx).Model(&assetRow{}).Where("object_key = ? AND (id <> ? OR project_id <> ?)", key, asset, binding.TargetProjectID)
	if kind != "" {
		query = s.db.WithContext(ctx).Table("media.rendition AS r").Joins("JOIN media.media_asset AS a ON a.id = r.media_asset_id").Where("r.object_key = ? AND (r.media_asset_id <> ? OR r.kind <> ? OR a.project_id <> ?)", key, asset, kind, binding.TargetProjectID)
	}
	conflict, err := storageExists(query)
	if err != nil {
		return err
	}
	if conflict {
		return application.ErrObjectMismatch
	}
	for _, object := range objects {
		if object.TargetAssetID == asset && object.RenditionKind == kind && object.TargetLocation != nil {
			return beginCopyRemoval(ctx, s.db.WithContext(ctx), projectCopyWorkOwner(binding.TargetProjectID, snapshot.ID), locationID(object.TargetLocation), object.TargetObjectKey)
		}
	}
	return nil
}

// ConfirmObjectRemoved records an absence checked after deletion by the worker.
func (s *ProjectCopyStore) ConfirmObjectRemoved(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	if err := s.AuthorizeObjectRemoval(ctx, actor, binding, snapshot, asset, kind); err != nil {
		return err
	}
	var object copyObjectRow
	read := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("snapshot_id = ? AND target_asset_id = ? AND rendition_kind = ?", snapshot.ID, asset, kind).Take(&object)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	if read.Error != nil {
		return read.Error
	}
	if object.TargetLocationID != nil {
		if err := confirmCopyRemoval(ctx, s.db.WithContext(ctx), projectCopyWorkOwner(binding.TargetProjectID, snapshot.ID), object.TargetLocationID, object.TargetObjectKey); err != nil {
			return err
		}
	}
	update := s.db.WithContext(ctx).Model(&copyObjectRow{}).Where("snapshot_id = ? AND target_asset_id = ? AND rendition_kind = ?", snapshot.ID, asset, kind).Updates(map[string]any{"status": application.ProjectCopyObjectRemoved, "update_time": time.Now().UTC()})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrNotFound
	}
	return nil
}

// FinishCleanup soft-deletes only registered target facts after every object is absent.
func (s *ProjectCopyStore) FinishCleanup(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) error {
	content, err := s.load(ctx, actor, binding, snapshot)
	if err != nil {
		return err
	}
	objects, err := s.Objects(ctx, actor, binding, snapshot)
	if err != nil {
		return err
	}
	for _, object := range objects {
		if object.Status != application.ProjectCopyObjectRemoved {
			return application.ErrObjectMismatch
		}
	}
	var registered int64
	if err := s.db.WithContext(ctx).Model(&projectCopyReceiptRecord{}).Where("snapshot_id = ?", snapshot.ID).Count(&registered).Error; err != nil {
		return err
	}
	if registered == 0 {
		ids := make([]uuid.UUID, 0, len(content.Assets))
		for _, item := range content.Assets {
			ids = append(ids, item.Asset.ID)
		}
		if len(ids) > 0 {
			var present int64
			if err := s.db.WithContext(ctx).Model(&assetRow{}).Where("id IN ?", ids).Count(&present).Error; err != nil {
				return err
			}
			if present != 0 {
				return application.ErrObjectMismatch
			}
		}
		return nil
	}
	for _, item := range content.Assets {
		now := time.Now().UTC()
		if err := s.db.WithContext(ctx).Model(&renditionRow{}).Where("media_asset_id = ? AND is_delete = ?", item.Asset.ID, false).Updates(map[string]any{"is_delete": true, "update_time": now}).Error; err != nil {
			return err
		}
		deleted, err := item.Asset.Delete(now)
		if err != nil {
			return err
		}
		if err := updateOne(s.db.WithContext(ctx), "media.media_asset", map[string]any{"id": item.Asset.ID, "project_id": binding.TargetProjectID, "object_key": item.Asset.ObjectKey, "is_delete": false}, map[string]any{"is_delete": true, "delete_time": deleted.DeleteTime, "purge_after": deleted.PurgeAfter, "revision": deleted.Revision, "update_time": deleted.UpdateTime}, application.ErrLibraryConflict); err != nil {
			return err
		}
	}
	return nil
}

var _ application.ProjectCopyObjectRepository = (*ProjectCopyStore)(nil)

// BeginObjectWrite commits one conditional-write intent before any external request.
func (s *ProjectCopyStore) BeginObjectWrite(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	if _, err := s.load(ctx, actor, binding, snapshot); err != nil {
		return err
	}
	var object copyObjectRow
	read := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("snapshot_id = ? AND target_asset_id = ? AND rendition_kind = ?", snapshot.ID, asset, kind).Take(&object)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	if read.Error != nil {
		return read.Error
	}
	if object.TargetLocationID != nil {
		if err := beginCopyLocation(ctx, s.db.WithContext(ctx), projectCopyWorkOwner(binding.TargetProjectID, snapshot.ID), object.TargetLocationID, object.TargetObjectKey); err != nil {
			return err
		}
	}
	update := s.db.WithContext(ctx).Model(&copyObjectRow{}).Where("snapshot_id = ? AND target_asset_id = ? AND rendition_kind = ? AND status = ? AND source_verified = ? AND sha256 IS NOT NULL AND byte_size IS NOT NULL AND write_started = ?", snapshot.ID, asset, kind, application.ProjectCopyObjectPending, true, false).Updates(map[string]any{"write_started": true, "update_time": time.Now().UTC()})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrObjectMismatch
	}
	return nil
}

// NewScopedProjectCopyStore binds coordinator authority without replacing its lease.
func NewScopedProjectCopyStore(tx *gorm.DB, scopes ScopeAccessFactory) *ProjectCopyStore {
	return &ProjectCopyStore{db: tx, scopeAuthority: scopeAuthority{scopes: scopes}}
}

// WithRoutedStorage binds source and target locations before freezing new copy jobs.
func (s *ProjectCopyStore) WithRoutedStorage() *ProjectCopyStore {
	clone := *s
	clone.routed = true
	return &clone
}
