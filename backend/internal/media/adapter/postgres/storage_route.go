package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// StorageRoutes persists deployment-owned identity and serializes every route
// change in PostgreSQL. Construction and profile registration do not activate it.
type StorageRoutes struct{ db *gorm.DB }

// NewStorageRoutes injects the transaction owner; construction performs no activation.
func NewStorageRoutes(db *gorm.DB) *StorageRoutes { return &StorageRoutes{db: db} }

// RegisterProfiles is idempotent for identical identities and rejects renamed resources.
func (s *StorageRoutes) RegisterProfiles(ctx context.Context, profiles []application.StorageProfile) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, p := range profiles {
			p = normalizeStorageProfile(p)
			if !validStorageProfile(p) {
				return application.ErrStorageIdentityConflict
			}
			identityProfile := config.StorageProfile{Provider: p.Provider, Endpoint: p.Endpoint, Region: p.Region, Bucket: p.Bucket}
			sum := sha256.Sum256([]byte(identityProfile.ResourceIdentity()))
			identity := hex.EncodeToString(sum[:])
			if err := tx.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, p.ID, p.Provider, p.Endpoint, p.SigningEndpoint, p.Region, p.Bucket, p.AddressingStyle, identity).Error; err != nil {
				var pg *pgconn.PgError
				if errors.As(err, &pg) && pg.Code == "23505" {
					return application.ErrStorageIdentityConflict
				}
				return fmt.Errorf("register storage profile: %w", err)
			}
			var saved string
			read := tx.Raw(`SELECT id FROM media.storage_profile WHERE id=? AND provider=? AND endpoint=? AND signing_endpoint=? AND region=? AND bucket=? AND addressing_style=? AND resource_identity=? FOR SHARE`, p.ID, p.Provider, p.Endpoint, p.SigningEndpoint, p.Region, p.Bucket, p.AddressingStyle, identity).Scan(&saved)
			if read.Error != nil {
				return fmt.Errorf("read registered storage profile: %w", read.Error)
			}
			if read.RowsAffected != 1 || saved != p.ID {
				return application.ErrStorageIdentityConflict
			}
		}
		return nil
	})
}

func validStorageProfile(p application.StorageProfile) bool {
	if (domain.ObjectRef{ProfileID: p.ID, Bucket: p.Bucket, Key: "validation"}).Validate() != nil || p.Region == "" {
		return false
	}
	if p.Provider != "minio" && p.Provider != "cos" && p.Provider != "tos" {
		return false
	}
	for _, raw := range []string{p.Endpoint, p.SigningEndpoint} {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
			return false
		}
	}
	if p.Provider == "cos" && config.ValidateCOSOrigin(p.Endpoint, p.Bucket, p.Region, true) != nil {
		return false
	}
	return p.Provider == "minio" && (p.AddressingStyle == "path" || p.AddressingStyle == "virtual") || p.Provider != "minio" && p.AddressingStyle == ""
}

func normalizeStorageProfile(p application.StorageProfile) application.StorageProfile {
	for _, value := range []*string{&p.Endpoint, &p.SigningEndpoint} {
		u, err := url.Parse(*value)
		if err != nil {
			continue
		}
		u.Host = strings.ToLower(u.Host)
		if u.Port() == "443" && u.Scheme == "https" || u.Port() == "80" && u.Scheme == "http" {
			u.Host = u.Hostname()
			if strings.Contains(u.Host, ":") {
				u.Host = "[" + u.Host + "]"
			}
		}
		*value = u.String()
	}
	return p
}

// ReadActive requires an explicitly initialized database route.
func (s *StorageRoutes) ReadActive(ctx context.Context) (application.StorageRoute, error) {
	if s == nil || s.db == nil {
		return application.StorageRoute{}, application.ErrUnavailable
	}
	return readStorageRoute(s.db.WithContext(ctx), false)
}

func readStorageRoute(tx *gorm.DB, lock bool) (application.StorageRoute, error) {
	query := `SELECT r.profile_id,p.bucket,r.revision FROM media.storage_route r JOIN media.storage_profile p ON p.id=r.profile_id WHERE r.singleton=1`
	if lock {
		query += ` FOR UPDATE OF r`
	}
	var route application.StorageRoute
	read := tx.Raw(query).Scan(&route)
	if read.Error != nil {
		return route, fmt.Errorf("read active storage route: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return route, application.ErrStorageNotReady
	}
	return route, nil
}

// Activate is the low-level CAS, called only after the operator's admission
// gates. Replaying an identical expected/new/target receipt repairs a lost reply.
func (s *StorageRoutes) Activate(ctx context.Context, expected, newRevision, profileID string) (application.StorageRoute, error) {
	var route application.StorageRoute
	if s == nil || s.db == nil {
		return route, application.ErrUnavailable
	}
	if newRevision == "" || profileID == "" || newRevision == expected {
		return route, application.ErrStorageRouteConflict
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('media.storage_route',0))`).Error; err != nil {
			return err
		}
		current, err := readStorageRoute(tx, true)
		if err != nil && !errors.Is(err, application.ErrStorageNotReady) {
			return err
		}
		var prior struct{ ExpectedRevision, ProfileID string }
		replay := tx.Raw(`SELECT expected_revision,profile_id FROM media.storage_activation WHERE revision=?`, newRevision).Scan(&prior)
		if replay.Error != nil {
			return replay.Error
		}
		if replay.RowsAffected == 1 {
			if prior.ExpectedRevision == expected && prior.ProfileID == profileID && current.Revision == newRevision && current.ProfileID == profileID {
				route = current
				return nil
			}
			return application.ErrStorageRouteConflict
		}
		if current.Revision != expected {
			return application.ErrStorageRouteConflict
		}
		var bucket string
		read := tx.Raw(`SELECT bucket FROM media.storage_profile WHERE id=? FOR SHARE`, profileID).Scan(&bucket)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrStorageNotReady
		}
		if err := tx.Exec(`INSERT INTO media.storage_activation(revision,expected_revision,profile_id) VALUES(?,?,?)`, newRevision, expected, profileID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id) VALUES(1,?,?) ON CONFLICT(singleton) DO UPDATE SET revision=excluded.revision,profile_id=excluded.profile_id,updated_at=now()`, newRevision, profileID).Error; err != nil {
			return err
		}
		route = application.StorageRoute{ProfileID: profileID, Bucket: bucket, Revision: newRevision}
		return nil
	})
	return route, err
}
