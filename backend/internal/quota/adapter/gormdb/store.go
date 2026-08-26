package gormdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	"github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinQuotaTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) AuthorizeProject(
	ctx context.Context,
	actor application.Actor,
	workspaceID, projectID, access string,
) error {
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return unauthenticated()
	}
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return notFound("Project not found")
	}
	project, err := uuid.Parse(projectID)
	if err != nil {
		return notFound("Project not found")
	}
	var user model.UserAccount
	if err = repo.database.WithContext(ctx).First(&user, "id = ?", actorID).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	var workspaceRecord model.Workspace
	if err = repo.database.WithContext(ctx).First(&workspaceRecord, "id = ?", workspace).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).Where(
		"workspace_id = ? AND user_id = ? AND status = ?", workspace, actorID, "active",
	).First(&membership).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	var projectRecord model.Project
	if err = repo.database.WithContext(ctx).Where("id = ? AND workspace_id = ?", project, workspace).First(&projectRecord).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	if user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return unauthenticated()
	}
	if access == "read" {
		return nil
	}
	if workspaceRecord.Status != "active" || projectRecord.Status != "active" || membership.Role == "viewer" {
		return forbidden()
	}
	if access == "owner" && membership.Role != "owner" {
		return forbidden()
	}
	if access != "write" && access != "owner" {
		return forbidden()
	}
	return nil
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	return commandgorm.Find(ctx, repo.database, workspaceID, operation, key)
}

func (repo *repository) EnsureReceipt(ctx context.Context, receipt platformcommand.Receipt) (platformcommand.Receipt, error) {
	return commandgorm.Ensure(ctx, repo.database, receipt)
}

func (repo *repository) FindPolicy(ctx context.Context, projectID, metric string) (domain.Policy, error) {
	project, err := uuid.Parse(projectID)
	if err != nil {
		return domain.Policy{}, application.ErrPolicyNotFound
	}
	var record model.QuotaPolicy
	if err = repo.database.WithContext(ctx).Where("project_id = ? AND metric = ?", project, metric).First(&record).Error; err != nil {
		return domain.Policy{}, normalizePolicyNotFound(err)
	}
	return policyDomain(record), nil
}

func (repo *repository) GetPolicyForUpdate(ctx context.Context, projectID, metric string) (domain.Policy, error) {
	project, err := uuid.Parse(projectID)
	if err != nil {
		return domain.Policy{}, application.ErrPolicyNotFound
	}
	var record model.QuotaPolicy
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ? AND metric = ?", project, metric).First(&record).Error; err != nil {
		return domain.Policy{}, normalizePolicyNotFound(err)
	}
	return policyDomain(record), nil
}

func (repo *repository) EnsurePolicy(ctx context.Context, desired domain.Policy) (domain.Policy, error) {
	record, err := policyRecord(desired)
	if err != nil {
		return domain.Policy{}, err
	}
	created := record
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "workspace_id"}, {Name: "project_id"}, {Name: "metric"}}, DoNothing: true,
	}).Create(&created).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Policy{}, err
	}
	var persisted model.QuotaPolicy
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND project_id = ? AND metric = ?", record.WorkspaceID, record.ProjectID, record.Metric).
		First(&persisted).Error; err != nil {
		return domain.Policy{}, fmt.Errorf("load ensured quota policy: %w", err)
	}
	return policyDomain(persisted), nil
}

func (repo *repository) UpdatePolicy(ctx context.Context, desired domain.Policy, expectedRevision int64) (domain.Policy, error) {
	record, err := policyRecord(desired)
	if err != nil {
		return domain.Policy{}, err
	}
	updated := repo.database.WithContext(ctx).Model(&model.QuotaPolicy{}).
		Where("id = ? AND revision = ?", record.ID, expectedRevision).
		Updates(map[string]any{
			"limit_units": record.LimitUnits, "revision": record.Revision, "content_hash": record.ContentHash,
			"updated_by": record.UpdatedBy, "updated_at": record.UpdatedAt,
		})
	if updated.Error != nil {
		return domain.Policy{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return domain.Policy{}, conflict("Daily quota policy revision has changed")
	}
	return desired, nil
}

func (repo *repository) FindCounter(ctx context.Context, policyID string, windowStart time.Time) (domain.Counter, error) {
	policy, err := uuid.Parse(policyID)
	if err != nil {
		return domain.Counter{}, application.ErrCounterNotFound
	}
	var record model.QuotaCounter
	if err = repo.database.WithContext(ctx).Where("policy_id = ? AND window_start = ?", policy, windowStart.UTC()).First(&record).Error; err != nil {
		return domain.Counter{}, normalizeCounterNotFound(err)
	}
	return counterDomain(record), nil
}

func (repo *repository) GetCounterForUpdate(ctx context.Context, policyID string, windowStart time.Time) (domain.Counter, error) {
	policy, err := uuid.Parse(policyID)
	if err != nil {
		return domain.Counter{}, application.ErrCounterNotFound
	}
	var record model.QuotaCounter
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("policy_id = ? AND window_start = ?", policy, windowStart.UTC()).First(&record).Error; err != nil {
		return domain.Counter{}, normalizeCounterNotFound(err)
	}
	return counterDomain(record), nil
}

func (repo *repository) EnsureCounter(ctx context.Context, desired domain.Counter) (domain.Counter, error) {
	record, err := counterRecord(desired)
	if err != nil {
		return domain.Counter{}, err
	}
	created := record
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "policy_id"}, {Name: "window_start"}}, DoNothing: true,
	}).Create(&created).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Counter{}, err
	}
	var persistedRecord model.QuotaCounter
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("policy_id = ? AND window_start = ?", record.PolicyID, record.WindowStart).First(&persistedRecord).Error; err != nil {
		return domain.Counter{}, fmt.Errorf("load ensured quota counter: %w", err)
	}
	persisted := counterDomain(persistedRecord)
	if persisted.WorkspaceID != desired.WorkspaceID || persisted.ProjectID != desired.ProjectID ||
		persisted.PolicyID != desired.PolicyID || persisted.Metric != desired.Metric ||
		!persisted.WindowStart.Equal(desired.WindowStart) || !persisted.WindowEnd.Equal(desired.WindowEnd) ||
		persisted.PolicyRevision != desired.PolicyRevision || persisted.LimitUnits != desired.LimitUnits {
		return domain.Counter{}, platformcommand.ErrInputMismatch
	}
	return persisted, nil
}

func (repo *repository) UpdateCounter(ctx context.Context, desired domain.Counter, expectedRevision int64) (domain.Counter, error) {
	record, err := counterRecord(desired)
	if err != nil {
		return domain.Counter{}, err
	}
	updated := repo.database.WithContext(ctx).Model(&model.QuotaCounter{}).
		Where("id = ? AND revision = ?", record.ID, expectedRevision).
		Updates(map[string]any{
			"policy_revision": record.PolicyRevision, "limit_units": record.LimitUnits,
			"reserved_units": record.ReservedUnits, "consumed_units": record.ConsumedUnits,
			"revision": record.Revision, "updated_at": record.UpdatedAt,
		})
	if updated.Error != nil {
		return domain.Counter{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return domain.Counter{}, conflict("Quota counter revision has changed")
	}
	return desired, nil
}

func (repo *repository) FindReservationBySource(
	ctx context.Context,
	policyID string,
	windowStart time.Time,
	sourceType, sourceID string,
) (domain.Reservation, error) {
	policy, source, err := parsePair(policyID, sourceID)
	if err != nil {
		return domain.Reservation{}, application.ErrReservationNotFound
	}
	var record model.QuotaReservation
	if err = repo.database.WithContext(ctx).Where(
		"policy_id = ? AND window_start = ? AND source_type = ? AND source_id = ?",
		policy, windowStart.UTC(), sourceType, source,
	).First(&record).Error; err != nil {
		return domain.Reservation{}, normalizeReservationNotFound(err)
	}
	return reservationDomain(record), nil
}

func (repo *repository) GetReservation(ctx context.Context, reservationID string) (domain.Reservation, error) {
	parsed, err := uuid.Parse(reservationID)
	if err != nil {
		return domain.Reservation{}, application.ErrReservationNotFound
	}
	var record model.QuotaReservation
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", parsed).Error; err != nil {
		return domain.Reservation{}, normalizeReservationNotFound(err)
	}
	return reservationDomain(record), nil
}

func (repo *repository) GetReservationForUpdate(ctx context.Context, reservationID string) (domain.Reservation, error) {
	parsed, err := uuid.Parse(reservationID)
	if err != nil {
		return domain.Reservation{}, application.ErrReservationNotFound
	}
	var record model.QuotaReservation
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "id = ?", parsed).Error; err != nil {
		return domain.Reservation{}, normalizeReservationNotFound(err)
	}
	return reservationDomain(record), nil
}

func (repo *repository) CreateReservation(ctx context.Context, desired domain.Reservation) (domain.Reservation, error) {
	record, err := reservationRecord(desired)
	if err != nil {
		return domain.Reservation{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return domain.Reservation{}, err
	}
	return desired, nil
}

func (repo *repository) UpdateReservation(
	ctx context.Context,
	desired domain.Reservation,
	expectedRevision int64,
) (domain.Reservation, error) {
	record, err := reservationRecord(desired)
	if err != nil {
		return domain.Reservation{}, err
	}
	updated := repo.database.WithContext(ctx).Model(&model.QuotaReservation{}).
		Where("id = ? AND revision = ?", record.ID, expectedRevision).
		Updates(map[string]any{
			"status": record.Status, "revision": record.Revision, "updated_at": record.UpdatedAt,
			"consumed_at": record.ConsumedAt, "released_at": record.ReleasedAt,
		})
	if updated.Error != nil {
		return domain.Reservation{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return domain.Reservation{}, conflict("Quota reservation revision has changed")
	}
	return desired, nil
}

func (repo *repository) ListReservations(ctx context.Context, counterID string) ([]domain.Reservation, error) {
	parsed, err := uuid.Parse(counterID)
	if err != nil {
		return nil, application.ErrCounterNotFound
	}
	var records []model.QuotaReservation
	if err = repo.database.WithContext(ctx).Where("counter_id = ?", parsed).Order("source_type ASC").Order("source_id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Reservation, len(records))
	for index, record := range records {
		result[index] = reservationDomain(record)
	}
	return result, nil
}

func policyRecord(value domain.Policy) (model.QuotaPolicy, error) {
	id, workspace, project, creator, updater, err := parsePolicyIDs(value)
	if err != nil {
		return model.QuotaPolicy{}, err
	}
	return model.QuotaPolicy{
		ID: id, WorkspaceID: workspace, ProjectID: project, Metric: value.Metric, WindowKind: value.WindowKind,
		LimitUnits: value.LimitUnits, Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedBy: creator, UpdatedBy: updater, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func policyDomain(value model.QuotaPolicy) domain.Policy {
	return domain.Policy{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		Metric: value.Metric, WindowKind: value.WindowKind, LimitUnits: value.LimitUnits, Revision: value.Revision,
		ContentHash: value.ContentHash, CreatedBy: value.CreatedBy.String(), UpdatedBy: value.UpdatedBy.String(),
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func counterRecord(value domain.Counter) (model.QuotaCounter, error) {
	id, workspace, project, policy, err := parseFour(value.ID, value.WorkspaceID, value.ProjectID, value.PolicyID)
	if err != nil {
		return model.QuotaCounter{}, err
	}
	return model.QuotaCounter{
		ID: id, WorkspaceID: workspace, ProjectID: project, PolicyID: policy, Metric: value.Metric,
		WindowStart: value.WindowStart.UTC(), WindowEnd: value.WindowEnd.UTC(), PolicyRevision: value.PolicyRevision,
		LimitUnits: value.LimitUnits, ReservedUnits: value.ReservedUnits, ConsumedUnits: value.ConsumedUnits,
		Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func counterDomain(value model.QuotaCounter) domain.Counter {
	return domain.Counter{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		PolicyID: value.PolicyID.String(), Metric: value.Metric, WindowStart: value.WindowStart.UTC(), WindowEnd: value.WindowEnd.UTC(),
		PolicyRevision: value.PolicyRevision, LimitUnits: value.LimitUnits, ReservedUnits: value.ReservedUnits,
		ConsumedUnits: value.ConsumedUnits, Revision: value.Revision, CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func reservationRecord(value domain.Reservation) (model.QuotaReservation, error) {
	id, workspace, project, policy, err := parseFour(value.ID, value.WorkspaceID, value.ProjectID, value.PolicyID)
	if err != nil {
		return model.QuotaReservation{}, err
	}
	counter, source, creator, err := parseThree(value.CounterID, value.SourceID, value.CreatedBy)
	if err != nil {
		return model.QuotaReservation{}, err
	}
	return model.QuotaReservation{
		ID: id, WorkspaceID: workspace, ProjectID: project, PolicyID: policy, CounterID: counter,
		Metric: value.Metric, SourceType: value.SourceType, SourceID: source,
		WindowStart: value.WindowStart.UTC(), WindowEnd: value.WindowEnd.UTC(), PolicyRevision: value.PolicyRevision,
		LimitUnits: value.LimitUnits, Units: value.Units, Status: value.Status, BindingHash: value.BindingHash,
		Revision: value.Revision, CreatedBy: creator, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		ConsumedAt: value.ConsumedAt, ReleasedAt: value.ReleasedAt,
	}, nil
}

func reservationDomain(value model.QuotaReservation) domain.Reservation {
	return domain.Reservation{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		PolicyID: value.PolicyID.String(), CounterID: value.CounterID.String(), Metric: value.Metric,
		SourceType: value.SourceType, SourceID: value.SourceID.String(), WindowStart: value.WindowStart.UTC(), WindowEnd: value.WindowEnd.UTC(),
		PolicyRevision: value.PolicyRevision, LimitUnits: value.LimitUnits, Units: value.Units, Status: value.Status,
		BindingHash: value.BindingHash, Revision: value.Revision, CreatedBy: value.CreatedBy.String(),
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
		ConsumedAt: utcTimePointer(value.ConsumedAt), ReleasedAt: utcTimePointer(value.ReleasedAt),
	}
}

func utcTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}

func parsePolicyIDs(value domain.Policy) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	values := []string{value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy, value.UpdatedBy}
	parsed := make([]uuid.UUID, len(values))
	for index, raw := range values {
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
		}
		parsed[index] = id
	}
	return parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], nil
}

func parseFour(first, second, third, fourth string) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	left, middle, err := parsePair(first, second)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	right, last, err := parsePair(third, fourth)
	return left, middle, right, last, err
}

func parseThree(first, second, third string) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	left, middle, err := parsePair(first, second)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	right, err := uuid.Parse(third)
	return left, middle, right, err
}

func parsePair(first, second string) (uuid.UUID, uuid.UUID, error) {
	left, err := uuid.Parse(first)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	right, err := uuid.Parse(second)
	return left, right, err
}

func normalizePolicyNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrPolicyNotFound
	}
	return err
}

func normalizeCounterNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrCounterNotFound
	}
	return err
}

func normalizeReservationNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrReservationNotFound
	}
	return err
}

func normalizeAuthorizationNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound("Project not found")
	}
	return err
}

func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}

func forbidden() error {
	return &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
}

func notFound(message string) error {
	return &application.Error{Code: "not_found", Message: message, Status: 404}
}

func conflict(message string) error {
	return &application.Error{Code: "state_conflict", Message: message, Status: 409}
}

var _ application.TransactionManager = (*Store)(nil)
var _ application.Repository = (*repository)(nil)
