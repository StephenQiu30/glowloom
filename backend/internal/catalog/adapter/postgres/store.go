// Package postgres persists platform-level providers and encrypted credentials.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

var (
	// ErrProviderKeyExists means another provider already uses the stable key.
	ErrProviderKeyExists = application.ErrProviderKeyExists
	// ErrProviderNotFound means no non-deleted provider has the requested ID.
	ErrProviderNotFound = application.ErrProviderNotFound
	// ErrProviderUnavailable means a provider does not accept new credentials.
	ErrProviderUnavailable = application.ErrProviderUnavailable
	// ErrCredentialNotFound means no current credential matches the provider.
	ErrCredentialNotFound = application.ErrCredentialNotFound
	// ErrRevisionConflict means provider settings changed after they were read.
	ErrRevisionConflict = domain.ErrProviderRevisionConflict
)

// Store keeps the provider tables behind an explicitly injected database handle.
// Administrator authorization belongs to the application command or query.
type Store struct {
	db *gorm.DB
}

// NewStore injects the database handle for catalog persistence.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// CreateProvider inserts a provider with its initial revision.
func (s *Store) CreateProvider(ctx context.Context, provider domain.Provider) error {
	if err := provider.Validate(); err != nil {
		return err
	}
	if provider.Revision != 1 || provider.Status != domain.ProviderActive {
		return domain.ErrInvalidProvider
	}
	result := s.db.WithContext(ctx).Exec(`
		INSERT INTO catalog.provider
		  (id, key, name, adapter_key, region, status, concurrency_limit, rate_limit_per_min)
		VALUES (?::uuid, ?, ?, ?, ?, ?, ?, ?)
	`, provider.ID.String(), provider.Key, provider.Name, provider.AdapterKey,
		string(provider.Region), string(provider.Status), provider.ConcurrencyLimit, provider.RateLimitPerMin)
	if result.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(result.Error, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "provider_key_key" {
			return fmt.Errorf("create provider: %w", ErrProviderKeyExists)
		}
		return fmt.Errorf("create provider: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("create provider: inserted %d rows", result.RowsAffected)
	}
	return nil
}

// FindProvider reads a non-deleted provider by ID.
func (s *Store) FindProvider(ctx context.Context, providerID uuid.UUID) (domain.Provider, error) {
	if providerID == uuid.Nil {
		return domain.Provider{}, ErrProviderNotFound
	}
	var row providerRow
	result := s.db.WithContext(ctx).Raw(`
		SELECT id, current_config_version_id, key, name, adapter_key, region, status, concurrency_limit,
		       rate_limit_per_min, revision, create_time, update_time
		FROM catalog.provider WHERE id = ?::uuid AND NOT is_delete
	`, providerID.String()).Scan(&row)
	if result.Error != nil {
		return domain.Provider{}, fmt.Errorf("find provider: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.Provider{}, ErrProviderNotFound
	}
	return row.provider(), nil
}

// UpdateProvider changes operational limits and availability with optimistic locking.
func (s *Store) UpdateProvider(ctx context.Context, provider domain.Provider, expectedRevision int64) error {
	if err := provider.Validate(); err != nil {
		return err
	}
	if expectedRevision < 1 || provider.Revision != expectedRevision+1 {
		return domain.ErrInvalidProvider
	}
	result := s.db.WithContext(ctx).Exec(`
		UPDATE catalog.provider
		SET status = ?, concurrency_limit = ?, rate_limit_per_min = ?,
		    revision = revision + 1, update_time = now()
		WHERE id = ?::uuid AND revision = ? AND NOT is_delete
	`, string(provider.Status), provider.ConcurrencyLimit, provider.RateLimitPerMin,
		provider.ID.String(), expectedRevision)
	if result.Error != nil {
		return fmt.Errorf("update provider: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

// ReplaceCredential atomically disables the old active record and inserts a new one.
// Locking the provider row serializes concurrent replacements for one provider.
func (s *Store) ReplaceCredential(ctx context.Context, credential domain.Credential) error {
	if err := credential.Validate(); err != nil {
		return err
	}
	if credential.Status != domain.CredentialActive || credential.LastTestResult != "" {
		return domain.ErrInvalidCredential
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return replaceCredentialInTx(tx, credential)
	})
}

func replaceCredentialInTx(tx *gorm.DB, credential domain.Credential) error {
	if err := requireCredentialProvider(tx, credential.ProviderID); err != nil {
		return err
	}
	result := tx.Exec(`
			UPDATE catalog.provider_credential
			SET status = 'draining', update_time = now()
			WHERE provider_id = ?::uuid AND status = 'active' AND NOT is_delete
		`, credential.ProviderID.String())
	if result.Error != nil {
		return fmt.Errorf("disable previous credential: %w", result.Error)
	}
	result = tx.Exec(`
			INSERT INTO catalog.provider_credential
			  (id, provider_id, label, ciphertext, key_id, last4, status)
			VALUES (?::uuid, ?::uuid, ?, ?::bytea, ?, ?, 'active')
		`, credential.ID.String(), credential.ProviderID.String(), credential.Label,
		credential.Ciphertext, credential.KeyID, credential.Last4)
	if result.Error != nil {
		return fmt.Errorf("insert encrypted credential: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("insert encrypted credential: inserted %d rows", result.RowsAffected)
	}
	return nil
}

// FindActiveCredential returns only the current credential of an active provider.
func (s *Store) FindActiveCredential(ctx context.Context, providerID uuid.UUID) (domain.Credential, error) {
	if providerID == uuid.Nil {
		return domain.Credential{}, ErrCredentialNotFound
	}
	var row credentialRow
	result := s.db.WithContext(ctx).Raw(`
		SELECT c.id, c.provider_id, c.label, c.ciphertext, c.key_id, c.last4,
		       c.status, c.last_tested_at, c.last_test_result, c.create_time, c.update_time
		FROM catalog.provider_credential AS c
		JOIN catalog.provider AS p ON p.id = c.provider_id
		WHERE c.provider_id = ?::uuid AND c.status = 'active' AND NOT c.is_delete
		  AND p.status = 'active' AND NOT p.is_delete
	`, providerID.String()).Scan(&row)
	if result.Error != nil {
		return domain.Credential{}, fmt.Errorf("find active credential: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.Credential{}, ErrCredentialNotFound
	}
	return row.credential(), nil
}

// DisableCredential stops new use of one current credential without deleting it.
func (s *Store) DisableCredential(ctx context.Context, providerID, credentialID uuid.UUID) error {
	if providerID == uuid.Nil || credentialID == uuid.Nil {
		return ErrCredentialNotFound
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked int
		result := tx.Raw(`
			SELECT 1 FROM catalog.provider WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, providerID.String()).Scan(&locked)
		if result.Error != nil {
			return fmt.Errorf("lock provider for credential disable: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrProviderNotFound
		}
		result = tx.Exec(`
			UPDATE catalog.provider_credential
			SET status = 'disabled', update_time = now()
			WHERE id = ?::uuid AND provider_id = ?::uuid
			  AND status = 'active' AND NOT is_delete
		`, credentialID.String(), providerID.String())
		if result.Error != nil {
			return fmt.Errorf("disable credential: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrCredentialNotFound
		}
		return nil
	})
}

type providerRow struct {
	CurrentConfigVersionID uuid.UUID
	ID                     uuid.UUID
	Key                    string
	Name                   string
	AdapterKey             string
	Region                 string
	Status                 string
	ConcurrencyLimit       int
	RateLimitPerMin        int
	Revision               int64
	CreateTime             time.Time
	UpdateTime             time.Time
}

func (row providerRow) provider() domain.Provider {
	return domain.Provider{
		ID: row.ID, CurrentConfigVersionID: row.CurrentConfigVersionID, Key: row.Key, Name: row.Name, AdapterKey: row.AdapterKey,
		Region: domain.Region(row.Region), Status: domain.ProviderStatus(row.Status),
		ConcurrencyLimit: row.ConcurrencyLimit, RateLimitPerMin: row.RateLimitPerMin,
		Revision: row.Revision, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
	}
}

type credentialRow struct {
	ID             uuid.UUID
	ProviderID     uuid.UUID
	Label          string
	Ciphertext     []byte
	KeyID          string
	Last4          string
	Status         string
	LastTestedAt   *time.Time
	LastTestResult *string
	CreateTime     time.Time
	UpdateTime     time.Time
}

func (row credentialRow) credential() domain.Credential {
	credential := domain.Credential{
		ID: row.ID, ProviderID: row.ProviderID, Label: row.Label,
		Ciphertext: row.Ciphertext, KeyID: row.KeyID, Last4: row.Last4,
		Status:     domain.CredentialStatus(row.Status),
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
	}
	if row.LastTestedAt != nil {
		credential.LastTestedAt = *row.LastTestedAt
	}
	if row.LastTestResult != nil {
		credential.LastTestResult = domain.TestResult(*row.LastTestResult)
	}
	return credential
}

// requireCredentialProvider permits onboarding while disabled, but never a
// preset emergency stop. Lock order matches configuration and dispatch changes.
func requireCredentialProvider(tx *gorm.DB, providerID uuid.UUID) error {
	var preset struct {
		ID                uuid.UUID
		EmergencyDisabled bool
	}
	result := tx.Raw(`SELECT preset.id,preset.emergency_disabled FROM catalog.provider p
 JOIN catalog.provider_config_version c ON c.id=p.current_config_version_id
 JOIN catalog.provider_preset_version v ON v.id=c.preset_version_id
 JOIN catalog.provider_preset preset ON preset.id=v.preset_id
 WHERE p.id=? FOR SHARE OF preset`, providerID).Scan(&preset)
	if result.Error != nil {
		return result.Error
	}
	if preset.EmergencyDisabled {
		return application.ErrPresetUnavailable
	}
	var provider struct {
		Status                 string
		CurrentConfigVersionID *uuid.UUID
	}
	result = tx.Raw(`SELECT status,current_config_version_id FROM catalog.provider WHERE id=? AND NOT is_delete FOR UPDATE`, providerID).Scan(&provider)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrProviderNotFound
	}
	if provider.Status != "active" && provider.CurrentConfigVersionID == nil {
		return ErrProviderUnavailable
	}
	return nil
}
