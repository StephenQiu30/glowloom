// Package postgres reads project-scoped operation quote data.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

var (
	// ErrNotFound hides missing and out-of-project operation records.
	ErrNotFound = application.ErrPublicNotFound
	// ErrUnavailable means the store has no database handle.
	ErrUnavailable = errors.New("operation store unavailable")
)

// Store keeps the database handle injected by the composition root.
type Store struct{ db *gorm.DB }

// NewStore injects the caller's database handle.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// FindBatch checks the actor and project before returning a batch.
func (s *Store) FindBatch(ctx context.Context, actor identityapp.Principal, projectID, batchID uuid.UUID) (domain.Batch, error) {
	if s == nil || s.db == nil {
		return domain.Batch{}, ErrUnavailable
	}
	if projectID == uuid.Nil || batchID == uuid.Nil {
		return domain.Batch{}, ErrNotFound
	}
	var row batchRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		result := tx.Raw(`
			SELECT b.id, b.project_id, b.kind, b.scope, b.status,
			       b.total_count, b.succeeded_count, b.failed_count,
			       b.unknown_count, b.quote_total_micros
			FROM operation.batch AS b
			JOIN workspace.project AS p ON p.id = b.project_id
			WHERE b.id = ?::uuid AND b.project_id = ?::uuid AND p.org_id = ?::uuid
			  AND NOT b.is_delete AND NOT p.is_delete
		`, batchID.String(), projectID.String(), actor.OrgID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read batch: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return domain.Batch{}, fmt.Errorf("find batch: %w", err)
	}
	batch := row.domain()
	if err := batch.Validate(); err != nil {
		return domain.Batch{}, fmt.Errorf("validate stored batch: %w", err)
	}
	return batch, nil
}

// FindOperation checks the actor and project before returning an operation.
func (s *Store) FindOperation(ctx context.Context, actor identityapp.Principal, projectID, operationID uuid.UUID) (domain.Operation, error) {
	if s == nil || s.db == nil {
		return domain.Operation{}, ErrUnavailable
	}
	if projectID == uuid.Nil || operationID == uuid.Nil {
		return domain.Operation{}, ErrNotFound
	}
	var row operationRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		result := tx.Raw(`
			SELECT o.id, o.project_id, o.batch_id, o.target_type, o.target_id,
			       o.target_key, o.target_version_no, o.capability, o.mode,
			       o.model_profile_version_id, o.price_rule_version_id, o.params,
			       o.output_count, o.input_hash, o.origin, o.status, o.quote_micros,
			       o.quote_detail, o.quote_expires_at, o.reused_from_id,
			       o.force_regenerate, o.reservation_id, o.confirmed_at,
			       o.settled_micros, o.region, o.create_time
			FROM operation.operation AS o
			JOIN workspace.project AS p ON p.id = o.project_id
			WHERE o.id = ?::uuid AND o.project_id = ?::uuid AND p.org_id = ?::uuid
			  AND NOT o.is_delete AND NOT p.is_delete
		`, operationID.String(), projectID.String(), actor.OrgID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return domain.Operation{}, fmt.Errorf("find operation: %w", err)
	}
	operation := row.domain()
	if err := operation.Validate(); err != nil {
		return domain.Operation{}, fmt.Errorf("validate stored operation: %w", err)
	}
	return operation, nil
}

// FindOperationInputs inherits scope from the parent operation and project.
// A visible operation without frozen inputs returns an empty slice.
func (s *Store) FindOperationInputs(ctx context.Context, actor identityapp.Principal, projectID, operationID uuid.UUID) ([]domain.OperationInput, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if projectID == uuid.Nil || operationID == uuid.Nil {
		return nil, ErrNotFound
	}
	var rows []operationInputRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var visible int
		result := tx.Raw(`
			SELECT 1 FROM operation.operation AS o
			JOIN workspace.project AS p ON p.id = o.project_id
			WHERE o.id = ?::uuid AND o.project_id = ?::uuid AND p.org_id = ?::uuid
			  AND NOT o.is_delete AND NOT p.is_delete
			FOR SHARE OF o, p
		`, operationID.String(), projectID.String(), actor.OrgID.String()).Scan(&visible)
		if result.Error != nil {
			return fmt.Errorf("check operation input scope: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		result = tx.Raw(`
			SELECT i.id, i.operation_id, i.seq_no, i.role, i.ref_type,
			       i.ref_id, i.ref_version, i.text_value, i.media_asset_id, i.mask_asset_id
			FROM operation.operation_input AS i
			WHERE i.operation_id = ?::uuid AND NOT i.is_delete
			ORDER BY i.seq_no
		`, operationID.String()).Scan(&rows)
		if result.Error != nil {
			return fmt.Errorf("read operation inputs: %w", result.Error)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("find operation inputs: %w", err)
	}
	inputs := make([]domain.OperationInput, 0, len(rows))
	for _, row := range rows {
		input := row.domain()
		if err := input.Validate(); err != nil {
			return nil, fmt.Errorf("validate stored operation input: %w", err)
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func requireCurrentActor(tx *gorm.DB, actor identityapp.Principal) error {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return identityapp.ErrForbidden
	}
	var present int
	result := tx.Raw(`
		SELECT 1 FROM identity."user" AS u
		JOIN workspace.organization AS org ON org.id = u.org_id
		WHERE u.id = ?::uuid AND u.org_id = ?::uuid AND u.role = ?
		  AND u.status = 'active' AND NOT u.is_delete AND NOT u.must_change_password
		  AND org.status = 'active' AND NOT org.is_delete
		FOR SHARE OF u, org
	`, actor.ID.String(), actor.OrgID.String(), string(actor.Role)).Scan(&present)
	if result.Error != nil {
		return fmt.Errorf("check current operation actor: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return identityapp.ErrForbidden
	}
	return nil
}

type batchRow struct {
	ID                uuid.UUID
	ProjectID         uuid.UUID
	Kind              string
	Scope             []byte
	Status            string
	TotalCount        int32
	SucceededCount    int32
	FailedCount       int32
	UnknownCount      int32
	QuoteTotalMicros  int64
	PausedReason      *string
	CancelRequestedAt *time.Time
}

func (r batchRow) domain() domain.Batch {
	return domain.Batch{
		ID: r.ID, ProjectID: r.ProjectID, Kind: r.Kind,
		Scope: json.RawMessage(r.Scope), Status: domain.BatchStatus(r.Status),
		TotalCount: r.TotalCount, SucceededCount: r.SucceededCount,
		FailedCount: r.FailedCount, UnknownCount: r.UnknownCount,
		QuoteTotalMicros: r.QuoteTotalMicros,
	}
}

type operationRow struct {
	ID                    uuid.UUID
	ProjectID             uuid.UUID
	BatchID               *uuid.UUID
	TargetType            string
	TargetID              *uuid.UUID
	TargetKey             *string
	TargetVersionNo       *int32
	Capability            string
	Mode                  string
	ModelProfileVersionID *uuid.UUID
	PriceRuleVersionID    *uuid.UUID
	Params                []byte
	OutputCount           int32
	InputHash             string
	Origin                string
	Status                string
	QuoteMicros           *int64
	QuoteDetail           []byte
	QuoteExpiresAt        *time.Time
	FailureCode           *string
	ReusedFromID          *uuid.UUID
	ForceRegenerate       bool
	ReservationID         *uuid.UUID
	ConfirmedAt           *time.Time
	SettledMicros         *int64
	Region                *string
	CreateTime            time.Time
}

func (r operationRow) domain() domain.Operation {
	return domain.Operation{
		ID: r.ID, ProjectID: r.ProjectID, BatchID: r.BatchID,
		TargetType: r.TargetType, TargetID: r.TargetID, TargetKey: r.TargetKey,
		TargetVersionNo: r.TargetVersionNo, Capability: r.Capability, Mode: r.Mode,
		ModelProfileVersionID: r.ModelProfileVersionID, PriceRuleVersionID: r.PriceRuleVersionID,
		Params: json.RawMessage(r.Params), OutputCount: r.OutputCount,
		InputHash: r.InputHash, Origin: r.Origin, Status: domain.Status(r.Status),
		QuoteMicros: r.QuoteMicros, QuoteDetail: json.RawMessage(r.QuoteDetail),
		QuoteExpiresAt: r.QuoteExpiresAt, ReusedFromID: r.ReusedFromID,
		ForceRegenerate: r.ForceRegenerate, ReservationID: r.ReservationID,
		ConfirmedAt: r.ConfirmedAt, SettledMicros: r.SettledMicros,
		Region: r.Region, CreateTime: r.CreateTime,
	}
}

type operationInputRow struct {
	ID           uuid.UUID
	OperationID  uuid.UUID
	SeqNo        int32
	Role         string
	RefType      string
	RefID        *uuid.UUID
	RefVersion   *string
	TextValue    *string
	MediaAssetID *uuid.UUID
	MaskAssetID  *uuid.UUID
}

func (r operationInputRow) domain() domain.OperationInput {
	return domain.OperationInput{
		ID: r.ID, OperationID: r.OperationID, SeqNo: r.SeqNo,
		Role: r.Role, RefType: r.RefType, RefID: r.RefID,
		RefVersion: r.RefVersion, TextValue: r.TextValue,
		MediaAssetID: r.MediaAssetID, MaskAssetID: r.MaskAssetID,
	}
}
