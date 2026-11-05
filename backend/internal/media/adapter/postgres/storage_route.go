package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// StorageRoutes persists deployment-owned identity and serializes environment
// route projections in PostgreSQL.
type StorageRoutes struct{ db *gorm.DB }

const storageRouteSingleton = 1

type storageProfileRecord struct {
	ID, Provider, Endpoint, SigningEndpoint, Region, Bucket, AddressingStyle string
	ResourceIdentity                                                         string
	CreatedAt                                                                time.Time
}

func (storageProfileRecord) TableName() string { return "media.storage_profile" }

type storageRouteRecord struct {
	Singleton int `gorm:"primaryKey"`
	Revision  string
	ProfileID string
	UpdatedAt time.Time
}

func (storageRouteRecord) TableName() string { return "media.storage_route" }

type storageRouteLockRecord struct {
	Singleton int `gorm:"primaryKey"`
}

func (storageRouteLockRecord) TableName() string { return "media.storage_route_lock" }

// NewStorageRoutes injects the transaction owner.
func NewStorageRoutes(db *gorm.DB) *StorageRoutes { return &StorageRoutes{db: db} }

// RegisterProfiles is idempotent for identical identities and rejects renamed resources.
func (s *StorageRoutes) RegisterProfiles(ctx context.Context, profiles []application.StorageProfile) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return registerStorageProfiles(tx, profiles)
	})
}

func registerStorageProfiles(tx *gorm.DB, profiles []application.StorageProfile) error {
	ordered := slices.Clone(profiles)
	slices.SortFunc(ordered, func(a, b application.StorageProfile) int {
		return strings.Compare(a.ID, b.ID)
	})
	for _, p := range ordered {
		p = normalizeStorageProfile(p)
		if !validStorageProfile(p) {
			return application.ErrStorageIdentityConflict
		}
		identityProfile := config.StorageProfile{Provider: config.StorageProvider(p.Provider), Endpoint: p.Endpoint, Region: p.Region, Bucket: p.Bucket}
		sum := sha256.Sum256([]byte(identityProfile.ResourceIdentity()))
		identity := hex.EncodeToString(sum[:])
		profile := storageProfileRecord{ID: p.ID, Provider: p.Provider, Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Region: p.Region, Bucket: p.Bucket, AddressingStyle: p.AddressingStyle, ResourceIdentity: identity}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&profile).Error; err != nil {
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "23505" {
				return application.ErrStorageIdentityConflict
			}
			return fmt.Errorf("register storage profile: %w", err)
		}
		var saved storageProfileRecord
		read := tx.Where(map[string]any{"id": p.ID, "provider": p.Provider, "endpoint": p.Endpoint, "signing_endpoint": p.SigningEndpoint, "region": p.Region, "bucket": p.Bucket, "addressing_style": p.AddressingStyle, "resource_identity": identity}).Take(&saved)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrStorageIdentityConflict
			}
			return fmt.Errorf("read registered storage profile: %w", read.Error)
		}
		if read.RowsAffected != 1 || saved.ID != p.ID {
			return application.ErrStorageIdentityConflict
		}
	}
	return nil
}

func validStorageProfile(p application.StorageProfile) bool {
	if (domain.ObjectRef{ProfileID: p.ID, Bucket: p.Bucket, Key: "validation"}).Validate() != nil || p.Region == "" {
		return false
	}
	if p.Provider != string(config.StorageProviderMinIO) && p.Provider != string(config.StorageProviderCOS) && p.Provider != string(config.StorageProviderTOS) {
		return false
	}
	for _, raw := range []string{p.Endpoint, p.SigningEndpoint} {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
			return false
		}
	}
	if p.Provider == string(config.StorageProviderCOS) && config.ValidateCOSOrigin(p.Endpoint, p.Bucket, p.Region, true) != nil {
		return false
	}
	return p.Provider == string(config.StorageProviderMinIO) && (p.AddressingStyle == string(config.StorageAddressingPath) || p.AddressingStyle == string(config.StorageAddressingVirtual)) || p.Provider != string(config.StorageProviderMinIO) && p.AddressingStyle == ""
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

// ReadConfigured returns the route projection applied from the process environment.
func (s *StorageRoutes) ReadConfigured(ctx context.Context) (application.StorageRoute, error) {
	if s == nil || s.db == nil {
		return application.StorageRoute{}, application.ErrUnavailable
	}
	return readStorageRoute(s.db.WithContext(ctx), false)
}

func readStorageRoute(tx *gorm.DB, lock bool) (application.StorageRoute, error) {
	if lock {
		if err := lockStorageRoute(tx); err != nil {
			return application.StorageRoute{}, err
		}
	}
	var record storageRouteRecord
	query := tx.Model(&storageRouteRecord{})
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	read := query.Where("singleton = ?", storageRouteSingleton).Take(&record)
	if read.Error != nil {
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return application.StorageRoute{}, application.ErrStorageNotReady
		}
		return application.StorageRoute{}, fmt.Errorf("read configured storage route: %w", read.Error)
	}
	var profile storageProfileRecord
	if err := tx.Where("id = ?", record.ProfileID).Take(&profile).Error; err != nil {
		return application.StorageRoute{}, err
	}
	return application.StorageRoute{ProfileID: record.ProfileID, Bucket: profile.Bucket, Revision: record.Revision}, nil
}

func lockStorageRoute(tx *gorm.DB) error {
	var lock storageRouteLockRecord
	result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("singleton = ?", storageRouteSingleton).Take(&lock)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return application.ErrStorageNotReady
	}
	return result.Error
}
