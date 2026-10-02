package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ProjectCopyStore owns media snapshots, immutable object intents and target registration.
// Workspace injects its admission/fenced transaction; no method reads its copy job.
type ProjectCopyStore struct{ db *gorm.DB }

// NewProjectCopyStore injects an owning coordinator's transaction.
func NewProjectCopyStore(tx *gorm.DB) *ProjectCopyStore { return &ProjectCopyStore{db: tx} }

func copyMediaTarget(tx *gorm.DB, actor identityapp.Principal, binding application.ProjectCopyBinding) error {
	if binding.Validate() != nil || binding.OrgID != actor.OrgID {
		return application.ErrProjectCopyMediaUnavailable
	}
	if err := requireCurrentActor(tx, actor); err != nil {
		return err
	}
	var present int
	read := tx.Raw(`SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND status='copying' AND NOT is_delete FOR SHARE`, binding.TargetProjectID, binding.OrgID).Scan(&present)
	if read.Error != nil {
		return fmt.Errorf("read private media copy target: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return application.ErrNotFound
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
	if _, err := application.NewProjectCopyReferenceQuery(s, nil).FreezeReferences(ctx, actor, binding, snapshot, facts); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	return snapshot, nil
}

func (s *ProjectCopyStore) freezeWithReferenceFacts(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, now time.Time, referenced []uuid.UUID, facts []application.ReferenceFact) (application.ProjectCopySnapshot, error) {
	if s == nil || s.db == nil {
		return application.ProjectCopySnapshot{}, application.ErrUnavailable
	}
	tx := s.db.WithContext(ctx)
	if err := copyMediaTarget(tx, actor, binding); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	if err := requireProject(tx, actor, binding.SourceProjectID, false); err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	librarySource, err := readCopySourceLibrary(tx, actor, binding)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
	}
	// Library declares every binary catalog row, including recycled/removed
	// originals. Script's caller references remain restricted to documents below.
	libraryRefs := make(map[uuid.UUID]bool, len(librarySource.items))
	selectedRefs := append([]uuid.UUID(nil), referenced...)
	seenRefs := make(map[uuid.UUID]bool, len(referenced))
	for _, id := range referenced {
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
	query := tx.Raw(`SELECT * FROM media.media_asset WHERE project_id=? AND NOT is_delete ORDER BY id LIMIT 4097 FOR SHARE`, binding.SourceProjectID)
	if len(selectedRefs) != 0 {
		query = tx.Raw(`SELECT * FROM media.media_asset WHERE project_id=? AND (NOT is_delete OR id IN ?) ORDER BY id LIMIT 4097 FOR SHARE`, binding.SourceProjectID, selectedRefs)
	}
	if err := query.Scan(&rows).Error; err != nil {
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
	renditionQuery := tx.Raw(`SELECT r.* FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE a.project_id=? AND NOT a.is_delete AND NOT r.is_delete ORDER BY r.media_asset_id,r.kind,r.id FOR SHARE OF a,r`, binding.SourceProjectID)
	if len(selectedRefs) != 0 {
		renditionQuery = tx.Raw(`SELECT r.* FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE a.project_id=? AND a.id IN ? AND NOT r.is_delete ORDER BY r.media_asset_id,r.kind,r.id FOR SHARE OF a,r`, binding.SourceProjectID, assetIDs)
	}
	if err := renditionQuery.Scan(&renditions).Error; err != nil {
		return application.ProjectCopySnapshot{}, fmt.Errorf("freeze media renditions: %w", err)
	}
	byAsset := make(map[uuid.UUID][]domain.Rendition)
	for _, row := range renditions {
		byAsset[row.MediaAssetID] = append(byAsset[row.MediaAssetID], row.domain())
	}
	sources := make([]application.ProjectCopySourceAsset, 0, len(rows))
	for _, row := range rows {
		source := application.ProjectCopySourceAsset{Asset: row.domain(), Renditions: byAsset[row.ID]}
		if source.Asset.IsDelete {
			if source.Asset.DeleteTime == nil || source.Asset.PurgeAfter == nil {
				return application.ProjectCopySnapshot{}, application.ErrProjectCopyMediaUnavailable
			}
			source.RetainedHistory = &application.RetainedHistoryProof{AssetID: source.Asset.ID, Revision: source.Asset.Revision, DeletedAt: *source.Asset.DeleteTime, PurgeAfter: *source.Asset.PurgeAfter}
			if libraryRefs[row.ID] {
				source.RetainedHistory.DeclaredBy = "library"
			} else if referenceRefs[row.ID] {
				source.RetainedHistory.DeclaredBy = "reference"
			}
		}
		sources = append(sources, source)
	}
	content, err := application.PrepareProjectMediaCopy(binding, sources, now)
	if err != nil {
		return application.ProjectCopySnapshot{}, err
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
	snapshot := application.ProjectCopySnapshot{ID: uuid.NewSHA1(binding.JobID, []byte("media-snapshot")), ManifestSHA256: digest, Assets: len(sources), Renditions: len(renditions), AssetMapping: mapping}
	if err := tx.Exec(`INSERT INTO media.project_copy_snapshot(id,job_id,org_id,source_project_id,target_project_id,manifest_sha256,asset_count,rendition_count,content,asset_mapping) VALUES(?,?,?,?,?,?,?,?,?::jsonb,?::jsonb)`, snapshot.ID, binding.JobID, binding.OrgID, binding.SourceProjectID, binding.TargetProjectID, digest, snapshot.Assets, snapshot.Renditions, string(body), string(assetMap)).Error; err != nil {
		return application.ProjectCopySnapshot{}, fmt.Errorf("persist frozen media snapshot: %w", err)
	}
	for _, object := range content.Objects {
		if err := tx.Exec(`INSERT INTO media.project_copy_object(snapshot_id,target_asset_id,rendition_kind,source_object_key,target_object_key,byte_size,content_type,sha256) VALUES(?,?,?,?,?,?,?,?)`, snapshot.ID, object.TargetAssetID, object.RenditionKind, object.SourceObjectKey, object.TargetObjectKey, object.ByteSize, object.ContentType, object.SHA256).Error; err != nil {
			return application.ProjectCopySnapshot{}, fmt.Errorf("persist private object identity: %w", err)
		}
	}
	return snapshot, nil
}

type copyObjectRow struct {
	TargetAssetID   uuid.UUID
	RenditionKind   string
	SourceObjectKey string
	TargetObjectKey string
	ByteSize        *int64
	ContentType     string
	SHA256          *string
	Status          string
	SourceVerified  bool
	WriteStarted    bool
}

func (r copyObjectRow) object() application.ProjectCopyObject {
	return application.ProjectCopyObject{TargetAssetID: r.TargetAssetID, RenditionKind: r.RenditionKind, SourceObjectKey: r.SourceObjectKey, TargetObjectKey: r.TargetObjectKey, ByteSize: r.ByteSize, ContentType: r.ContentType, SHA256: r.SHA256, Status: r.Status, SourceVerified: r.SourceVerified, WriteStarted: r.WriteStarted}
}

func (s *ProjectCopyStore) load(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot) (application.ProjectMediaCopy, error) {
	tx := s.db.WithContext(ctx)
	if err := copyMediaTarget(tx, actor, binding); err != nil {
		return application.ProjectMediaCopy{}, err
	}
	var row struct {
		ManifestSHA256             string
		AssetCount, RenditionCount int
		Content, AssetMapping      []byte
	}
	read := tx.Raw(`SELECT manifest_sha256,asset_count,rendition_count,content,asset_mapping FROM media.project_copy_snapshot WHERE id=? AND job_id=? AND org_id=? AND source_project_id=? AND target_project_id=?`, snapshot.ID, binding.JobID, binding.OrgID, binding.SourceProjectID, binding.TargetProjectID).Scan(&row)
	if read.Error != nil {
		return application.ProjectMediaCopy{}, read.Error
	}
	if read.RowsAffected != 1 || row.ManifestSHA256 != snapshot.ManifestSHA256 || row.AssetCount != snapshot.Assets || row.RenditionCount != snapshot.Renditions {
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
	if err := s.db.WithContext(ctx).Raw(`SELECT * FROM media.project_copy_object WHERE snapshot_id=? ORDER BY target_asset_id,rendition_kind FOR SHARE`, snapshot.ID).Scan(&rows).Error; err != nil {
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
		result = append(result, row.object())
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
			if object.Status != "pending" || object.SHA256 != nil && *object.SHA256 != digest || object.ByteSize != nil && *object.ByteSize != size {
				return application.ErrObjectMismatch
			}
		}
	}
	if !found {
		return application.ErrProjectCopyMediaUnavailable
	}
	update := s.db.WithContext(ctx).Exec(`UPDATE media.project_copy_object SET sha256=?,byte_size=?,source_verified=true,update_time=statement_timestamp() WHERE snapshot_id=? AND target_asset_id=? AND rendition_kind=? AND status='pending' AND (sha256 IS NULL OR sha256=?) AND (byte_size IS NULL OR byte_size=?)`, digest, size, snapshot.ID, asset, kind, digest, size)
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
	update := s.db.WithContext(ctx).Exec(`UPDATE media.project_copy_object SET status='verified',update_time=statement_timestamp() WHERE snapshot_id=? AND target_asset_id=? AND rendition_kind=? AND status IN ('pending','verified') AND source_verified AND write_started AND sha256=? AND byte_size=?`, snapshot.ID, asset, kind, digest, size)
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
		if object.Status != "verified" || !object.SourceVerified || object.SHA256 == nil || object.ByteSize == nil {
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
	var receipt struct {
		ContentSHA256              string
		AssetCount, RenditionCount int
	}
	replayed := s.db.WithContext(ctx).Raw(`SELECT content_sha256,asset_count,rendition_count FROM media.project_copy_receipt WHERE snapshot_id=?`, snapshot.ID).Scan(&receipt)
	if replayed.Error != nil {
		return application.ProjectCopyReceipt{}, replayed.Error
	}
	if replayed.RowsAffected == 1 {
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
	for _, item := range content.Assets {
		a := item.Asset
		if a.Validate() != nil {
			return application.ProjectCopyReceipt{}, application.ErrProjectCopyMediaUnavailable
		}
		if err := tx.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,duration_ms,fps,audio_channels,codec,moderation_status,moderation_detail,aigc_marked,contains_real_person,revision,create_time,update_time) VALUES(?,?,?,?,'ready',?,?,?,?,?,?,?,?,?,?,?,'passed',?::jsonb,?,false,1,?,?)`, a.ID, a.ProjectID, a.Kind, a.Origin, a.ObjectKey, a.FileName, a.MimeType, a.ByteSize, a.SHA256, a.Width, a.Height, a.DurationMS, a.FPS, a.AudioChannels, a.Codec, nullableJSON(a.ModerationDetail), a.AIGCMarked, a.CreateTime, a.UpdateTime).Error; err != nil {
			return application.ProjectCopyReceipt{}, fmt.Errorf("register copied asset: %w", err)
		}
		for _, r := range item.Renditions {
			if r.Validate() != nil {
				return application.ProjectCopyReceipt{}, application.ErrProjectCopyMediaUnavailable
			}
			if err := tx.Exec(`INSERT INTO media.rendition(id,media_asset_id,kind,object_key,width,height,byte_size,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?)`, r.ID, r.MediaAssetID, r.Kind, r.ObjectKey, r.Width, r.Height, r.ByteSize, r.CreateTime, r.UpdateTime).Error; err != nil {
				return application.ProjectCopyReceipt{}, fmt.Errorf("register copied rendition: %w", err)
			}
		}
	}
	if err := registerCopiedLibrary(tx, actor, binding, content.Library); err != nil {
		return application.ProjectCopyReceipt{}, err
	}
	if err := tx.Exec(`INSERT INTO media.project_copy_receipt(snapshot_id,content_sha256,asset_count,rendition_count) VALUES(?,?,?,?)`, snapshot.ID, result.ContentSHA256, result.Assets, result.Renditions).Error; err != nil {
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
	if err := copyMediaTarget(s.db.WithContext(ctx), actor, binding); err != nil {
		return application.AssetSummary{}, err
	}
	var row assetRow
	read := s.db.WithContext(ctx).Raw(`SELECT a.* FROM media.media_asset a JOIN media.project_copy_object o ON o.target_asset_id=a.id AND o.target_object_key=a.object_key AND o.rendition_kind='' JOIN media.project_copy_snapshot s ON s.id=o.snapshot_id JOIN media.project_copy_receipt r ON r.snapshot_id=s.id WHERE s.job_id=? AND s.org_id=? AND s.source_project_id=? AND s.target_project_id=? AND a.id=? AND a.project_id=s.target_project_id AND a.status='ready' AND a.moderation_status='passed' AND NOT a.is_delete AND o.status='verified' AND o.source_verified AND a.sha256=o.sha256 AND a.byte_size=o.byte_size FOR SHARE OF a,o`, binding.JobID, binding.OrgID, binding.SourceProjectID, binding.TargetProjectID, asset).Scan(&row)
	if read.Error != nil {
		return application.AssetSummary{}, read.Error
	}
	if read.RowsAffected != 1 {
		return application.AssetSummary{}, application.ErrNotFound
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
	var conflict bool
	query := `SELECT EXISTS(SELECT 1 FROM media.media_asset WHERE object_key=? AND (id<>? OR project_id<>?))`
	args := []any{key, asset, binding.TargetProjectID}
	if kind != "" {
		query = `SELECT EXISTS(SELECT 1 FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE r.object_key=? AND (r.media_asset_id<>? OR r.kind<>? OR a.project_id<>?))`
		args = []any{key, asset, kind, binding.TargetProjectID}
	}
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&conflict).Error; err != nil {
		return err
	}
	if conflict {
		return application.ErrObjectMismatch
	}
	return nil
}

// ConfirmObjectRemoved records an absence checked after deletion by the worker.
func (s *ProjectCopyStore) ConfirmObjectRemoved(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, asset uuid.UUID, kind string) error {
	if err := s.AuthorizeObjectRemoval(ctx, actor, binding, snapshot, asset, kind); err != nil {
		return err
	}
	update := s.db.WithContext(ctx).Exec(`UPDATE media.project_copy_object SET status='removed',update_time=statement_timestamp() WHERE snapshot_id=? AND target_asset_id=? AND rendition_kind=?`, snapshot.ID, asset, kind)
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
		if object.Status != "removed" {
			return application.ErrObjectMismatch
		}
	}
	for _, item := range content.Assets {
		if err := s.db.WithContext(ctx).Exec(`UPDATE media.rendition SET is_delete=true,update_time=statement_timestamp() WHERE media_asset_id=? AND NOT is_delete`, item.Asset.ID).Error; err != nil {
			return err
		}
		if err := s.db.WithContext(ctx).Exec(`UPDATE media.media_asset SET is_delete=true,delete_time=statement_timestamp(),purge_after=statement_timestamp()+interval '30 days',revision=revision+1,update_time=statement_timestamp() WHERE id=? AND project_id=? AND object_key=? AND NOT is_delete`, item.Asset.ID, binding.TargetProjectID, item.Asset.ObjectKey).Error; err != nil {
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
	update := s.db.WithContext(ctx).Exec(`UPDATE media.project_copy_object SET write_started=true,update_time=statement_timestamp() WHERE snapshot_id=? AND target_asset_id=? AND rendition_kind=? AND status='pending' AND source_verified AND sha256 IS NOT NULL AND byte_size IS NOT NULL AND NOT write_started`, snapshot.ID, asset, kind)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrObjectMismatch
	}
	return nil
}
