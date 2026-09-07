package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	database "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(db *gorm.DB) *Store { return &Store{database: db} }
func (s *Store) WithinTransaction(ctx context.Context, fn func(app.Repository) error) error {
	return database.WithinTransaction(ctx, s.database, func(tx *gorm.DB) error { return fn(&repository{database: tx}) })
}
func (r *repository) Authorize(ctx context.Context, actor app.Actor, projectID string, write bool) (string, error) {
	var workspace string
	err := scriptgorm.New(r.database).WithinSourceTransaction(ctx, func(source scriptapp.SourceRepository) error {
		var err error
		workspace, err = source.ProjectWorkspace(ctx, scriptapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, projectID, write)
		return err
	})
	return workspace, mapSourceError(err)
}
func (r *repository) Source(ctx context.Context, projectID, revisionID string) (domain.Source, error) {
	var result domain.Source
	err := scriptgorm.New(r.database).WithinSourceTransaction(ctx, func(source scriptapp.SourceRepository) error {
		value, err := source.GetAcceptedSource(ctx, projectID, revisionID)
		if err != nil {
			return err
		}
		result = domain.Source{DocumentID: value.Identity.LogicalID, RevisionID: value.Identity.VersionID, Revision: value.Identity.Revision, ContentHash: value.Identity.ContentHash, SpanIndexID: value.SpanIndexID}
		return nil
	})
	return result, mapSourceError(err)
}
func (r *repository) Find(ctx context.Context, actor app.Actor, projectID, key string) (domain.Run, error) {
	var record model.CreationRun
	err := r.database.WithContext(ctx).Where("project_id = ? AND actor_id = ? AND idempotency_key = ?", projectID, actor.UserID, key).First(&record).Error
	if err != nil {
		return domain.Run{}, notFound(err)
	}
	return runDomain(record)
}
func (r *repository) Get(ctx context.Context, id string) (domain.Run, error) {
	var record model.CreationRun
	if err := r.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.Run{}, notFound(err)
	}
	return runDomain(record)
}
func (r *repository) Create(ctx context.Context, value domain.Run) error {
	record, err := runRecord(value)
	if err != nil {
		return err
	}
	if err = r.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return err
	}
	return r.database.WithContext(ctx).Omit(clause.Associations).Create(&model.CreationCommandOutbox{RunID: record.ID, Status: "pending", NextAttemptAt: value.CreatedAt}).Error
}
func (r *repository) List(ctx context.Context, projectID string, limit int) ([]domain.Run, error) {
	var records []model.CreationRun
	if err := r.database.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at DESC, id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Run, 0, len(records))
	for _, record := range records {
		value, err := runDomain(record)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}
func (r *repository) Retry(ctx context.Context, id string, revision int64, tokenVersion int, now time.Time) error {
	var outbox model.CreationCommandOutbox
	if err := r.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&outbox, "run_id = ?", id).Error; err != nil {
		return notFound(err)
	}
	if outbox.Status != "blocked" {
		return app.Problem("revision_conflict", 409)
	}
	result := r.database.WithContext(ctx).Model(&model.CreationRun{}).Where("id = ? AND revision = ? AND status = ?", id, revision, domain.Blocked).Updates(map[string]any{"status": domain.Queued, "last_error": "", "revision": revision + 1, "token_version": tokenVersion, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return app.Problem("revision_conflict", 409)
	}
	return r.database.WithContext(ctx).Model(&outbox).Updates(map[string]any{"status": "pending", "next_attempt_at": now, "lease_until": nil}).Error
}
func (s *Store) Claim(ctx context.Context, now time.Time, lease time.Duration) (domain.Delivery, error) {
	var result domain.Delivery
	err := database.WithinTransaction(ctx, s.database, func(tx *gorm.DB) error {
		var outbox model.CreationCommandOutbox
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("(status = ? AND next_attempt_at <= ?) OR (status = ? AND lease_until <= ?)", "pending", now, "leased", now).Order("next_attempt_at, run_id").First(&outbox).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return app.ErrNoDelivery
		}
		if err != nil {
			return err
		}
		until := now.Add(lease)
		outbox.Status = "leased"
		outbox.Fence++
		outbox.Attempts++
		outbox.LeaseUntil = &until
		if err = tx.Omit(clause.Associations).Save(&outbox).Error; err != nil {
			return err
		}
		run, err := (&repository{database: tx}).Get(ctx, outbox.RunID.String())
		if err != nil {
			return err
		}
		result = domain.Delivery{Run: run, Fence: outbox.Fence, Attempts: outbox.Attempts}
		return nil
	})
	return result, err
}
func (s *Store) AuthorizeDelivery(ctx context.Context, run domain.Run) error {
	return s.WithinTransaction(ctx, func(repo app.Repository) error {
		_, err := repo.Authorize(ctx, app.Actor{UserID: run.Command.ActorID, TokenVersion: run.TokenVersion}, run.Command.ProjectID, true)
		return err
	})
}
func (s *Store) Complete(ctx context.Context, delivery domain.Delivery, status, code string, receipt *domain.Acceptance, now, next time.Time) error {
	if status != domain.Accepted && status != domain.Blocked && status != domain.Unknown {
		return app.Problem("invalid_delivery_outcome", 500)
	}
	if status == domain.Accepted {
		if receipt == nil {
			return app.Problem("missing_acceptance", 500)
		}
		if err := app.ValidateAcceptance(delivery.Run, *receipt); err != nil {
			return err
		}
	}
	return database.WithinTransaction(ctx, s.database, func(tx *gorm.DB) error {
		var outbox model.CreationCommandOutbox
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&outbox, "run_id = ?", delivery.Run.Command.RunID).Error; err != nil {
			return notFound(err)
		}
		if outbox.Fence != delivery.Fence || outbox.Status != "leased" || outbox.LeaseUntil == nil || !outbox.LeaseUntil.After(now) {
			return app.ErrLeaseLost
		}
		outboxStatus := "pending"
		if status == domain.Accepted {
			outboxStatus = "delivered"
		}
		if status == domain.Blocked {
			outboxStatus = "blocked"
		}
		updates := map[string]any{"status": status, "last_error": code, "revision": gorm.Expr("revision + 1"), "updated_at": now}
		if receipt != nil {
			encoded, err := json.Marshal(receipt)
			if err != nil {
				return err
			}
			updates["acceptance"] = datatypes.JSON(encoded)
		}
		result := tx.Model(&model.CreationRun{}).Where("id = ? AND status <> ?", outbox.RunID, domain.Accepted).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return app.ErrLeaseLost
		}
		return tx.Model(&outbox).Updates(map[string]any{"status": outboxStatus, "next_attempt_at": next, "lease_until": nil}).Error
	})
}
func runRecord(value domain.Run) (model.CreationRun, error) {
	id, err := uuid.Parse(value.Command.RunID)
	if err != nil {
		return model.CreationRun{}, err
	}
	workspace, err := uuid.Parse(value.Command.WorkspaceID)
	if err != nil {
		return model.CreationRun{}, err
	}
	project, err := uuid.Parse(value.Command.ProjectID)
	if err != nil {
		return model.CreationRun{}, err
	}
	actor, err := uuid.Parse(value.Command.ActorID)
	if err != nil {
		return model.CreationRun{}, err
	}
	payload, err := json.Marshal(value.Command)
	if err != nil {
		return model.CreationRun{}, err
	}
	return model.CreationRun{ID: id, WorkspaceID: workspace, ProjectID: project, ActorID: actor, TokenVersion: value.TokenVersion, IdempotencyKey: value.IdempotencyKey, InputHash: value.InputHash, PayloadHash: value.PayloadHash, Command: datatypes.JSON(payload), Endpoint: value.Endpoint, Status: value.Status, LastError: value.LastError, Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}
func runDomain(value model.CreationRun) (domain.Run, error) {
	run := domain.Run{InputHash: value.InputHash, PayloadHash: value.PayloadHash, IdempotencyKey: value.IdempotencyKey, Endpoint: value.Endpoint, TokenVersion: value.TokenVersion, Status: value.Status, LastError: value.LastError, Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
	if err := json.Unmarshal(value.Command, &run.Command); err != nil {
		return domain.Run{}, err
	}
	hash, err := app.PayloadHash(run.Command)
	if err != nil {
		return domain.Run{}, err
	}
	if run.Command.Schema != "creation-command-production" || run.Command.FlowType != domain.FlowType || run.Command.WorkflowID != "lanverse:creation:"+value.ID.String() || hash != value.PayloadHash || run.Command.RunID != value.ID.String() || run.Command.CommandID != value.ID.String() || run.Command.WorkspaceID != value.WorkspaceID.String() || run.Command.ProjectID != value.ProjectID.String() || run.Command.ActorID != value.ActorID.String() {
		return domain.Run{}, errors.New("creation command identity has drifted")
	}
	if len(value.Acceptance) > 0 {
		if err = json.Unmarshal(value.Acceptance, &run.Acceptance); err != nil {
			return domain.Run{}, err
		}
		if run.Acceptance != nil {
			if err = app.ValidateAcceptance(run, *run.Acceptance); err != nil {
				return domain.Run{}, err
			}
		}
	}
	if (run.Status == domain.Accepted) != (run.Acceptance != nil) {
		return domain.Run{}, errors.New("creation acceptance state has drifted")
	}
	return run, nil
}
func mapSourceError(err error) error {
	var problem *scriptapp.Error
	if errors.As(err, &problem) {
		return app.Problem(problem.Code, problem.Status)
	}
	if errors.Is(err, scriptapp.ErrNotFound) {
		return app.ErrNotFound
	}
	return err
}
func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.ErrNotFound
	}
	return err
}
