package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type legacyObjectColumn struct{ table, key, location, size, sha, mime string }

// This privileged migration inventory is closed over reviewed owning columns.
// Business authorization and ordinary cross-module reads retain their own ports.
var legacyObjectColumns = []legacyObjectColumn{
	{"script.script_source", "original_key", "original_location_id", "original_bytes", "original_sha256", "original_mime"},
	{"script.script_source", "rich_key", "rich_location_id", "rich_bytes", "rich_sha256", "'application/json'"},
	{"script.script_version", "text_key", "text_location_id", "text_bytes", "text_sha256", "'text/plain; charset=utf-8'"},
	{"script.script_version", "rich_key", "rich_location_id", "rich_bytes", "rich_sha256", "'application/json'"},
	{"script.copy_object_intent", "source_key", "source_location_id", "byte_size", "sha256", "mime"},
	{"script.copy_object_intent", "target_key", "target_location_id", "byte_size", "sha256", "mime"},
	{"script.copy_object_state", "target_key", "target_location_id", "NULL::bigint", "NULL::text", "NULL::text"},
	{"media.media_asset", "object_key", "object_location_id", "byte_size", "sha256", "mime_type"},
	{"media.rendition", "object_key", "object_location_id", "byte_size", "NULL::text", "NULL::text"},
	{"media.package_object", "object_key", "object_location_id", "byte_size", "sha256", "content_type"},
	{"media.project_copy_object", "source_object_key", "source_location_id", "byte_size", "sha256", "content_type"},
	{"media.project_copy_object", "target_object_key", "target_location_id", "byte_size", "sha256", "content_type"},
	{"media.transfer_object", "source_object_key", "source_location_id", "byte_size", "sha256", "content_type"},
	{"media.transfer_object", "target_object_key", "target_location_id", "byte_size", "sha256", "content_type"},
	{"media.purge_object", "object_key", "object_location_id", "byte_size", "sha256", "NULL::text"},
	{"mediatool.depth_object", "object_key", "object_location_id", "byte_size", "sha256", "mime_type"},
	{"script.object_intent", "object_key", "object_location_id", "byte_size", "sha256", "mime"},
	{"script.object_state", "object_key", "object_location_id", "NULL::bigint", "NULL::text", "NULL::text"},
}

const legacyRowDigest = `encode(sha256(convert_to((to_jsonb(t)-'object_location_id'-'source_location_id'-'target_location_id'-'original_location_id'-'rich_location_id'-'text_location_id')::text,'UTF8')),'hex')`

type legacyObjectRow struct {
	Retired                  bool
	Source, Key, Fingerprint string
	ByteSize                 *int64
	SHA256, MIMEType         *string
}

type legacyBindingRow struct{ ProfileID, Bucket, ResourceIdentity, EvidenceSHA256 string }

// BindLegacyStorage records reviewed evidence for the exact original MinIO
// identity. Registration and activation alone never establish this proof.
func (s *StorageRoutes) BindLegacyStorage(ctx context.Context, p application.StorageProfile, evidence string) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	p = normalizeStorageProfile(p)
	if p.Provider != "minio" || !validStorageProfile(p) || !storageDigest(evidence) {
		return application.ErrStorageIdentityConflict
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identity string
		read := tx.Raw(`SELECT resource_identity FROM media.storage_profile WHERE id=? AND provider='minio' AND endpoint=? AND signing_endpoint=? AND region=? AND bucket=? AND addressing_style=?`, p.ID, p.Endpoint, p.SigningEndpoint, p.Region, p.Bucket, p.AddressingStyle).Scan(&identity)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrStorageIdentityConflict
		}
		if err := tx.Exec(`INSERT INTO media.storage_legacy_binding(singleton,profile_id,bucket,resource_identity,evidence_sha256) VALUES(1,?,?,?,?) ON CONFLICT(singleton) DO NOTHING`, p.ID, p.Bucket, identity, evidence).Error; err != nil {
			return err
		}
		binding, err := readLegacyBinding(tx)
		if err != nil {
			return err
		}
		if binding.ProfileID != p.ID || binding.Bucket != p.Bucket || binding.ResourceIdentity != identity || binding.EvidenceSHA256 != evidence {
			return application.ErrStorageIdentityConflict
		}
		return nil
	})
}

func readLegacyBinding(tx *gorm.DB) (legacyBindingRow, error) {
	var row legacyBindingRow
	read := tx.Raw(`SELECT b.* FROM media.storage_legacy_binding b JOIN media.storage_profile p ON p.id=b.profile_id AND p.bucket=b.bucket AND p.resource_identity=b.resource_identity AND p.provider='minio' WHERE singleton=1`).Scan(&row)
	if read.Error != nil {
		return row, read.Error
	}
	if read.RowsAffected != 1 {
		return row, application.ErrStorageNotReady
	}
	return row, nil
}

// ResolveLegacyRef supports unchanged old JSON/Temporal keys only after the
// original resource has been proved. It never reads the active write route.
func (s *StorageRoutes) ResolveLegacyRef(ctx context.Context, key string) (domain.ObjectRef, error) {
	if s == nil || s.db == nil {
		return domain.ObjectRef{}, application.ErrUnavailable
	}
	binding, err := readLegacyBinding(s.db.WithContext(ctx))
	if err != nil {
		return domain.ObjectRef{}, err
	}
	ref := domain.ObjectRef{ProfileID: binding.ProfileID, Bucket: binding.Bucket, Key: key}
	return ref, ref.Validate()
}

func legacyRows(tx *gorm.DB, key string, lock bool) ([]legacyObjectRow, error) {
	rows := make([]legacyObjectRow, 0)
	for _, column := range legacyObjectColumns {
		source := column.table + "/" + column.location
		query := fmt.Sprintf(`SELECT ? AS source,t.%s AS key,%s AS fingerprint,%s AS byte_size,%s AS sha256,%s AS mime_type,(%s) AS retired FROM %s t WHERE %s IS NULL`, column.key, legacyRowDigest, column.size, column.sha, column.mime, legacyRetired(column), column.table, column.location)
		args := []any{source}
		if key != "" {
			query += fmt.Sprintf(` AND %s=?`, column.key)
			args = append(args, key)
		}
		query += fmt.Sprintf(` ORDER BY %s`, column.key)
		if lock {
			query += ` FOR UPDATE`
		}
		var found []legacyObjectRow
		if err := tx.Raw(query, args...).Scan(&found).Error; err != nil {
			return nil, fmt.Errorf("inventory legacy storage column: %w", err)
		}
		rows = append(rows, found...)
	}
	return rows, nil
}

func legacyCandidate(ref domain.ObjectRef, rows []legacyObjectRow) (application.LegacyObjectCandidate, error) {
	c := application.LegacyObjectCandidate{Ref: ref, References: len(rows), Retired: len(rows) > 0}
	digests := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Key != ref.Key {
			return c, application.ErrObjectMismatch
		}
		digests = append(digests, row.Source+":"+row.Fingerprint+":"+fmt.Sprint(row.Retired))
		c.Retired = c.Retired && row.Retired
		if row.ByteSize != nil {
			if c.ByteSize != nil && *c.ByteSize != *row.ByteSize {
				return c, application.ErrObjectMismatch
			}
			c.ByteSize = row.ByteSize
		}
		if row.SHA256 != nil {
			if c.SHA256 != nil && *c.SHA256 != *row.SHA256 {
				return c, application.ErrObjectMismatch
			}
			c.SHA256 = row.SHA256
		}
		if row.MIMEType != nil {
			if c.MIMEType != nil && *c.MIMEType != *row.MIMEType {
				return c, application.ErrObjectMismatch
			}
			c.MIMEType = row.MIMEType
		}
	}
	sort.Strings(digests)
	sum := sha256.Sum256([]byte(strings.Join(digests, "\n")))
	c.SnapshotSHA256 = hex.EncodeToString(sum[:])
	return c, nil
}

// LegacyCandidates selects a bounded set of keys first, then gathers every
// unresolved owning row for each key. Total inventory size never blocks a batch.
func (s *StorageRoutes) LegacyCandidates(ctx context.Context, limits ...int) ([]application.LegacyObjectCandidate, error) {
	if s == nil || s.db == nil {
		return nil, application.ErrUnavailable
	}
	limit := 1000
	if len(limits) > 1 {
		return nil, application.ErrUnavailable
	}
	if len(limits) == 1 {
		limit = limits[0]
	}
	if limit < 1 || limit > 1000 {
		return nil, application.ErrUnavailable
	}
	var result []application.LegacyObjectCandidate
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		binding, err := readLegacyBinding(tx)
		if err != nil {
			return err
		}
		parts := make([]string, 0, len(legacyObjectColumns))
		args := make([]any, 0, len(legacyObjectColumns)+1)
		for _, column := range legacyObjectColumns {
			parts = append(parts, fmt.Sprintf(`(SELECT DISTINCT %s AS object_key FROM %s WHERE %s IS NULL ORDER BY %s LIMIT ?)`, column.key, column.table, column.location, column.key))
			args = append(args, limit)
		}
		args = append(args, limit)
		var keys []string
		if err := tx.Raw(`SELECT DISTINCT object_key FROM (`+strings.Join(parts, ` UNION ALL `)+`) keys ORDER BY object_key LIMIT ?`, args...).Scan(&keys).Error; err != nil {
			return err
		}
		for _, key := range keys {
			ref := domain.ObjectRef{ProfileID: binding.ProfileID, Bucket: binding.Bucket, Key: key}
			if ref.Validate() != nil {
				return application.ErrStorageNotReady
			}
			rows, err := legacyRows(tx, key, false)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				continue
			}
			candidate, err := legacyCandidate(ref, rows)
			if err != nil {
				return err
			}
			result = append(result, candidate)
		}
		return nil
	})
	return result, err
}

// LegacyCandidateCount counts remaining physical keys without materializing all
// reference rows. It is progress only; the per-key CAS decides acceptance.
func (s *StorageRoutes) LegacyCandidateCount(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, application.ErrUnavailable
	}
	parts := make([]string, 0, len(legacyObjectColumns))
	for _, column := range legacyObjectColumns {
		parts = append(parts, fmt.Sprintf(`SELECT %s AS object_key FROM %s WHERE %s IS NULL`, column.key, column.table, column.location))
	}
	var count int
	err := s.db.WithContext(ctx).Raw(`SELECT count(DISTINCT object_key) FROM (` + strings.Join(parts, ` UNION ALL `) + `) keys`).Scan(&count).Error
	return count, err
}

func storageDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}
func legacyInfoMatches(c application.LegacyObjectCandidate, info application.ObjectInfo) bool {
	return info.Size >= 0 && info.Size <= 2<<30 && storageDigest(info.SHA256) && info.ContentType != "" && (c.ByteSize == nil || *c.ByteSize == info.Size) && (c.SHA256 == nil || *c.SHA256 == info.SHA256) && (c.MIMEType == nil || *c.MIMEType == info.ContentType)
}

// BackfillLegacyObject CAS-binds only the exact owning rows whose bytes were
// checked. It preserves source keys and old JSON/Temporal payload bytes.
func (s *StorageRoutes) BackfillLegacyObject(ctx context.Context, c application.LegacyObjectCandidate, info application.ObjectInfo) (domain.ObjectLocation, error) {
	var location domain.ObjectLocation
	if s == nil || s.db == nil {
		return location, application.ErrUnavailable
	}
	if c.Ref.Validate() != nil || !storageDigest(c.SnapshotSHA256) || c.References < 1 || !legacyInfoMatches(c, info) {
		return location, application.ErrObjectMismatch
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('media.storage_route',0))`).Error; err != nil {
			return err
		}
		binding, err := readLegacyBinding(tx)
		if err != nil {
			return err
		}
		if binding.ProfileID != c.Ref.ProfileID || binding.Bucket != c.Ref.Bucket {
			return application.ErrStorageIdentityConflict
		}
		var prior struct {
			LocationID       uuid.UUID
			ByteSize         int64
			SHA256, MIMEType string
		}
		read := tx.Raw(`SELECT location_id,byte_size,sha256,mime_type FROM media.storage_legacy_receipt WHERE profile_id=? AND bucket=? AND object_key=? AND snapshot_sha256=?`, c.Ref.ProfileID, c.Ref.Bucket, c.Ref.Key, c.SnapshotSHA256).Scan(&prior)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
			if prior.ByteSize != info.Size || prior.SHA256 != info.SHA256 || prior.MIMEType != info.ContentType {
				return application.ErrObjectMismatch
			}
			location, err = NewStorageRoutes(tx).ReadLocation(ctx, prior.LocationID)
			return err
		}
		rows, err := legacyRows(tx, c.Ref.Key, true)
		if err != nil {
			return err
		}
		current, err := legacyCandidate(c.Ref, rows)
		if err != nil {
			return err
		}
		if current.SnapshotSHA256 != c.SnapshotSHA256 || current.References != c.References {
			return application.ErrStorageRouteConflict
		}
		if !legacyInfoMatches(current, info) {
			return application.ErrObjectMismatch
		}
		location, err = NewStorageRoutes(tx).ReserveLocation(ctx, c.Ref)
		if err != nil {
			return err
		}
		var owned objectLocationRow
		if err := tx.Raw(`SELECT * FROM media.object_location WHERE id=? FOR UPDATE`, location.ID).Scan(&owned).Error; err != nil {
			return err
		}
		if owned.OwnerSessionID != nil || owned.OwnerIntentID != nil || owned.State != "reserved" && owned.State != "present" && owned.State != "accepted" || owned.ByteSize != nil && *owned.ByteSize != info.Size || owned.SHA256 != nil && *owned.SHA256 != info.SHA256 || owned.MIMEType != nil && *owned.MIMEType != info.ContentType {
			return application.ErrStorageRouteConflict
		}
		if err := tx.Exec(`UPDATE media.object_location SET state='accepted',byte_size=?,sha256=?,mime_type=? WHERE id=?`, info.Size, info.SHA256, info.ContentType, location.ID).Error; err != nil {
			return err
		}
		for _, column := range legacyObjectColumns {
			digests := make([]string, 0)
			for _, row := range rows {
				if row.Source == column.table+"/"+column.location {
					digests = append(digests, row.Fingerprint)
				}
			}
			if len(digests) == 0 {
				continue
			}
			query := fmt.Sprintf(`UPDATE %s t SET %s=? WHERE %s=? AND %s IS NULL AND %s IN ?`, column.table, column.location, column.key, column.location, legacyRowDigest)
			update := tx.Exec(query, location.ID, c.Ref.Key, digests)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != int64(len(digests)) {
				return application.ErrStorageRouteConflict
			}
		}
		if err := tx.Exec(`INSERT INTO media.storage_legacy_receipt(profile_id,bucket,object_key,snapshot_sha256,location_id,byte_size,sha256,mime_type,reference_count) VALUES(?,?,?,?,?,?,?,?,?)`, c.Ref.ProfileID, c.Ref.Bucket, c.Ref.Key, c.SnapshotSHA256, location.ID, info.Size, info.SHA256, info.ContentType, c.References).Error; err != nil {
			return err
		}
		location.State = "accepted"
		location.ByteSize = &info.Size
		location.SHA256 = &info.SHA256
		location.MIMEType = &info.ContentType
		return nil
	})
	return location, err
}

func legacyRetired(c legacyObjectColumn) string {
	switch c.table {
	case "media.media_asset", "media.rendition":
		return "t.is_delete AND EXISTS(SELECT 1 FROM media.purge_object p WHERE p.object_key=t.object_key AND p.object_location_id IS NOT DISTINCT FROM t.object_location_id AND p.removed)"
	case "media.package_object", "mediatool.depth_object":
		return "t.status='removed'"
	case "media.project_copy_object", "media.transfer_object":
		if c.location == "target_location_id" {
			return "t.status='removed'"
		}
		return "EXISTS(SELECT 1 FROM media.purge_object p WHERE p.object_key=t.source_object_key AND p.object_location_id IS NOT DISTINCT FROM t.source_location_id AND p.removed)"
	case "media.purge_object":
		return "t.removed"
	case "script.object_intent":
		return "EXISTS(SELECT 1 FROM script.object_state s WHERE s.object_key=t.object_key AND s.object_location_id IS NOT DISTINCT FROM t.object_location_id AND s.removed)"
	case "script.object_state", "script.copy_object_state":
		return "t.removed"
	case "script.copy_object_intent":
		if c.location == "target_location_id" {
			return "EXISTS(SELECT 1 FROM script.copy_object_state s WHERE s.target_key=t.target_key AND s.target_location_id IS NOT DISTINCT FROM t.target_location_id AND s.removed)"
		}
		return "false"
	default:
		return "false"
	}
}

// BackfillMissingLegacyObject records only a confirmed absent object whose
// every owning reference proves intentional retirement. Missing live bytes fail.
func (s *StorageRoutes) BackfillMissingLegacyObject(ctx context.Context, c application.LegacyObjectCandidate) (domain.ObjectLocation, error) {
	var location domain.ObjectLocation
	if s == nil || s.db == nil {
		return location, application.ErrUnavailable
	}
	if !c.Retired || c.Ref.Validate() != nil || !storageDigest(c.SnapshotSHA256) || c.References < 1 {
		return location, application.ErrObjectMismatch
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('media.storage_route',0))`).Error; err != nil {
			return err
		}
		binding, err := readLegacyBinding(tx)
		if err != nil {
			return err
		}
		if binding.ProfileID != c.Ref.ProfileID || binding.Bucket != c.Ref.Bucket {
			return application.ErrStorageIdentityConflict
		}
		var prior struct{ LocationID uuid.UUID }
		read := tx.Raw(`SELECT location_id FROM media.storage_legacy_tombstone WHERE profile_id=? AND bucket=? AND object_key=? AND snapshot_sha256=?`, c.Ref.ProfileID, c.Ref.Bucket, c.Ref.Key, c.SnapshotSHA256).Scan(&prior)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
			location, err = NewStorageRoutes(tx).ReadLocation(ctx, prior.LocationID)
			return err
		}
		rows, err := legacyRows(tx, c.Ref.Key, true)
		if err != nil {
			return err
		}
		current, err := legacyCandidate(c.Ref, rows)
		if err != nil {
			return err
		}
		if current.SnapshotSHA256 != c.SnapshotSHA256 || current.References != c.References {
			return application.ErrStorageRouteConflict
		}
		if !current.Retired {
			return application.ErrObjectMismatch
		}
		location, err = NewStorageRoutes(tx).ReserveLocation(ctx, c.Ref)
		if err != nil {
			return err
		}
		var owned objectLocationRow
		if err := tx.Raw(`SELECT * FROM media.object_location WHERE id=? FOR UPDATE`, location.ID).Scan(&owned).Error; err != nil {
			return err
		}
		if owned.OwnerSessionID != nil || owned.OwnerIntentID != nil {
			return application.ErrStorageRouteConflict
		}
		for _, column := range legacyObjectColumns {
			var live bool
			query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s t WHERE %s=? AND %s=? AND NOT(%s))`, column.table, column.key, column.location, legacyRetired(column))
			if err := tx.Raw(query, c.Ref.Key, location.ID).Scan(&live).Error; err != nil {
				return err
			}
			if live {
				return application.ErrObjectMismatch
			}
		}
		if err := tx.Exec(`UPDATE media.object_location SET state='deleted' WHERE id=?`, location.ID).Error; err != nil {
			return err
		}
		for _, column := range legacyObjectColumns {
			digests := make([]string, 0)
			for _, row := range rows {
				if row.Source == column.table+"/"+column.location {
					digests = append(digests, row.Fingerprint)
				}
			}
			if len(digests) == 0 {
				continue
			}
			query := fmt.Sprintf(`UPDATE %s t SET %s=? WHERE %s=? AND %s IS NULL AND %s IN ?`, column.table, column.location, column.key, column.location, legacyRowDigest)
			update := tx.Exec(query, location.ID, c.Ref.Key, digests)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != int64(len(digests)) {
				return application.ErrStorageRouteConflict
			}
		}
		if err := tx.Exec(`INSERT INTO media.storage_legacy_tombstone(profile_id,bucket,object_key,snapshot_sha256,location_id,reference_count) VALUES(?,?,?,?,?,?)`, c.Ref.ProfileID, c.Ref.Bucket, c.Ref.Key, c.SnapshotSHA256, location.ID, c.References).Error; err != nil {
			return err
		}
		location.State = "deleted"
		return nil
	})
	return location, err
}
