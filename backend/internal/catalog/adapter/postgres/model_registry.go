package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
)

var (
	// ErrCapabilityKeyExists means the stable capability key is already used.
	ErrCapabilityKeyExists = errors.New("capability key already exists")
	// ErrModelKeyExists means the stable model key is already used.
	ErrModelKeyExists = errors.New("model key already exists")
	// ErrModelNotFound means no non-deleted model has the requested ID.
	ErrModelNotFound = errors.New("model not found")
	// ErrModelSourceUnavailable means the provider or capability was deleted.
	ErrModelSourceUnavailable = errors.New("model provider or capability unavailable")
	// ErrModelVersionConflict means the next model version number was not supplied.
	ErrModelVersionConflict = errors.New("model version conflict")
	// ErrPriceVersionConflict means the next price version number was not supplied.
	ErrPriceVersionConflict = errors.New("price version conflict")
)

// CreateCapabilityForAdmin adds a platform capability after checking the live administrator.
func (s *Store) CreateCapabilityForAdmin(ctx context.Context, actorID, orgID uuid.UUID, capability domain.Capability) error {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || capability.Validate() != nil {
		return domain.ErrInvalidCapability
	}
	modes, err := json.Marshal(capability.Modes)
	if err != nil {
		return fmt.Errorf("encode capability modes: %w", err)
	}
	roles, err := json.Marshal(append([]string{}, capability.InputRoles...))
	if err != nil {
		return fmt.Errorf("encode capability input roles: %w", err)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		result := tx.Exec(`
			INSERT INTO catalog.capability (id, key, output_type, modes, input_roles)
			VALUES (?::uuid, ?, ?, ARRAY(SELECT jsonb_array_elements_text(?::jsonb)),
			        ARRAY(SELECT jsonb_array_elements_text(?::jsonb)))
		`, capability.ID.String(), capability.Key, string(capability.OutputType),
			string(modes), string(roles))
		if isUniqueViolation(result.Error, "capability_key_key") {
			return ErrCapabilityKeyExists
		}
		if result.Error != nil {
			return fmt.Errorf("insert capability: %w", result.Error)
		}
		return requireOneRow(result, "insert capability")
	})
}

// CreateModelForAdmin registers a disabled model with an initial revision.
func (s *Store) CreateModelForAdmin(ctx context.Context, actorID, orgID uuid.UUID, model domain.ModelProfile) error {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil ||
		model.Validate() != nil || model.Status != domain.ModelDisabled ||
		model.CurrentVersionID != uuid.Nil || model.Revision != 1 {
		return domain.ErrInvalidModel
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		return createModelInTx(tx, model)
	})
}

func createModelInTx(tx *gorm.DB, model domain.ModelProfile) error {
	result := tx.Exec(`
			INSERT INTO catalog.model_profile
			  (id, model_key, provider_id, capability, display_name)
			SELECT ?::uuid, ?, ?::uuid, ?, ?
			WHERE EXISTS (
			  SELECT 1 FROM catalog.provider WHERE id = ?::uuid AND NOT is_delete
			) AND EXISTS (
			  SELECT 1 FROM catalog.capability WHERE key = ? AND NOT is_delete
			)
		`, model.ID.String(), model.Key, model.ProviderID.String(), model.Capability,
		model.DisplayName, model.ProviderID.String(), model.Capability)
	if isUniqueViolation(result.Error, "model_profile_model_key_key") {
		return ErrModelKeyExists
	}
	if result.Error != nil {
		return fmt.Errorf("insert model profile: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrModelSourceUnavailable
	}
	return nil
}

// FindModelForAdmin checks live rights before reading a model identity and status.
func (s *Store) FindModelForAdmin(ctx context.Context, actorID, orgID, modelID uuid.UUID) (domain.ModelProfile, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || modelID == uuid.Nil {
		return domain.ModelProfile{}, ErrModelNotFound
	}
	var model domain.ModelProfile
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actorID, orgID); err != nil {
			return err
		}
		var row modelProfileRow
		result := tx.Raw(`
			SELECT id, model_key, provider_id, capability, display_name, status,
			       current_version_id, revision, create_time, update_time
			FROM catalog.model_profile WHERE id = ?::uuid AND NOT is_delete
		`, modelID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read model profile: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrModelNotFound
		}
		model = row.model()
		return nil
	})
	if err != nil {
		return domain.ModelProfile{}, fmt.Errorf("find model as administrator: %w", err)
	}
	return model, nil
}

// appendModelVersionInTx is only called by the audited publication transaction.
func appendModelVersionInTx(tx *gorm.DB, version domain.ModelVersion, expectedRevision int64) error {
	modes, err := json.Marshal(version.Modes)
	if err != nil {
		return fmt.Errorf("encode model modes: %w", err)
	}
	var limits struct {
		Roles map[string]json.RawMessage `json:"roles"`
	}
	if err := json.Unmarshal(version.Limits, &limits); err != nil {
		return domain.ErrInvalidModelVersion
	}
	roleNames := make([]string, 0, len(limits.Roles))
	for name := range limits.Roles {
		switch name {
		case "_total_images", "_total_videos", "_total_audios":
		default:
			roleNames = append(roleNames, name)
		}
	}
	roles, err := json.Marshal(roleNames)
	if err != nil {
		return fmt.Errorf("encode model input roles: %w", err)
	}
	row, err := lockModelProfile(tx, version.ModelID)
	if err != nil {
		return err
	}
	model := row.model()
	if err := model.AttachVersion(version, expectedRevision); err != nil {
		return err
	}
	var count int64
	if err := tx.Raw(`
			SELECT count(*) FROM catalog.model_profile_version
			WHERE model_profile_id = ?::uuid
		`, version.ModelID.String()).Scan(&count).Error; err != nil {
		return fmt.Errorf("count model versions: %w", err)
	}
	if int64(version.VersionNo) != count+1 {
		return ErrModelVersionConflict
	}
	var supported struct {
		ModesSupported bool
		RolesSupported bool
	}
	check := tx.Raw(`
		SELECT ARRAY(SELECT jsonb_array_elements_text(?::jsonb)) <@ c.modes AS modes_supported,
		       ARRAY(SELECT jsonb_array_elements_text(?::jsonb)) <@ c.input_roles AS roles_supported
		FROM catalog.capability AS c
		JOIN catalog.provider AS p ON p.id = ?::uuid AND NOT p.is_delete
		WHERE c.key = ? AND NOT c.is_delete
	`, string(modes), string(roles), row.ProviderID.String(), row.Capability).Scan(&supported)
	if check.Error != nil {
		return fmt.Errorf("check model capabilities: %w", check.Error)
	}
	if check.RowsAffected != 1 {
		return ErrModelSourceUnavailable
	}
	if !supported.ModesSupported || !supported.RolesSupported {
		return domain.ErrInvalidModelVersion
	}
	result := tx.Exec(`
			INSERT INTO catalog.model_profile_version
			  (id, model_profile_id, version_no, provider_model_id, modes, limits,
			   param_schema, supports_query, supports_cancel, supports_callback,
			   expected_max_ms, moderation, queue, create_by)
			VALUES (?::uuid, ?::uuid, ?, ?, ARRAY(SELECT jsonb_array_elements_text(?::jsonb)), ?::jsonb, ?::jsonb,
			        ?, ?, ?, ?, ?, ?, ?::uuid)
	`, version.ID.String(), version.ModelID.String(), version.VersionNo,
		version.ProviderModelID, string(modes), string(version.Limits),
		string(version.ParamSchema), version.SupportsQuery, version.SupportsCancel,
		version.SupportsCallback, version.ExpectedMaxMS, string(version.Moderation),
		version.Queue, version.CreateBy.String())
	if isUniqueViolation(result.Error, "uq_model_profile_version") {
		return ErrModelVersionConflict
	}
	if result.Error != nil {
		return fmt.Errorf("insert model version: %w", result.Error)
	}
	if err := requireOneRow(result, "insert model version"); err != nil {
		return err
	}
	result = tx.Exec(`
			UPDATE catalog.model_profile
			SET current_version_id = ?::uuid, revision = revision + 1, update_time = now()
			WHERE id = ?::uuid AND revision = ? AND NOT is_delete
	`, version.ID.String(), version.ModelID.String(), expectedRevision)
	if result.Error != nil {
		return fmt.Errorf("advance model version: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.ErrModelRevisionConflict
	}
	return nil
}

// setModelStatusInTx only runs inside the audited status transaction.
func setModelStatusInTx(tx *gorm.DB, modelID uuid.UUID, status domain.ModelStatus, expectedRevision int64) error {
	row, err := lockModelProfile(tx, modelID)
	if err != nil {
		return err
	}
	model := row.model()
	if model.Revision != expectedRevision {
		return domain.ErrModelRevisionConflict
	}
	var publishable bool
	if status == domain.ModelActive {
		if model.CurrentVersionID == uuid.Nil {
			return domain.ErrModelNotPublishable
		}
		var readiness struct {
			HasVersion bool
			HasPrice   bool
		}
		if err := tx.Raw(`
			SELECT EXISTS (
			  SELECT 1 FROM catalog.model_profile_version
			  WHERE id = ?::uuid AND model_profile_id = ?::uuid AND NOT is_delete
			) AS has_version,
			EXISTS (
			  SELECT 1 FROM catalog.price_rule_version
			  WHERE model_profile_id = ?::uuid AND effective_from <= now() AND NOT is_delete
			) AS has_price
		`, model.CurrentVersionID.String(), modelID.String(), modelID.String()).Scan(&readiness).Error; err != nil {
			return fmt.Errorf("check model publication readiness: %w", err)
		}
		publishable = readiness.HasVersion && readiness.HasPrice
	}
	if err := model.SetStatus(status, publishable); err != nil {
		return err
	}
	result := tx.Exec(`
		UPDATE catalog.model_profile
		SET status = ?, revision = revision + 1, update_time = now()
		WHERE id = ?::uuid AND revision = ? AND NOT is_delete
	`, string(status), modelID.String(), expectedRevision)
	if result.Error != nil {
		return fmt.Errorf("set model status: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.ErrModelRevisionConflict
	}
	return nil
}

type modelProfileRow struct {
	ID               uuid.UUID
	ModelKey         string
	ProviderID       uuid.UUID
	Capability       string
	DisplayName      string
	Status           string
	CurrentVersionID *uuid.UUID
	Revision         int64
	CreateTime       time.Time
	UpdateTime       time.Time
}

func (row modelProfileRow) model() domain.ModelProfile {
	model := domain.ModelProfile{
		ID: row.ID, Key: row.ModelKey, ProviderID: row.ProviderID,
		Capability: row.Capability, DisplayName: row.DisplayName,
		Status: domain.ModelStatus(row.Status), Revision: row.Revision,
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
	}
	if row.CurrentVersionID != nil {
		model.CurrentVersionID = *row.CurrentVersionID
	}
	return model
}

func lockModelProfile(tx *gorm.DB, modelID uuid.UUID) (modelProfileRow, error) {
	var row modelProfileRow
	result := tx.Raw(`
		SELECT id, model_key, provider_id, capability, display_name, status,
		       current_version_id, revision, create_time, update_time
		FROM catalog.model_profile WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
	`, modelID.String()).Scan(&row)
	if result.Error != nil {
		return modelProfileRow{}, fmt.Errorf("lock model profile: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return modelProfileRow{}, ErrModelNotFound
	}
	return row, nil
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func requireOneRow(result *gorm.DB, action string) error {
	if result.RowsAffected != 1 {
		return fmt.Errorf("%s: affected %d rows", action, result.RowsAffected)
	}
	return nil
}
