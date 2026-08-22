package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/domain"
	"github.com/stephenqiu30/lanverse/backend/internal/platform/objectstorage"
)

const analysisTopic = "lanverse.script-analysis.requested"

type Repository struct {
	pool    *pgxpool.Pool
	storage *objectstorage.Store
}

func New(pool *pgxpool.Pool, storage *objectstorage.Store) *Repository {
	return &Repository{pool: pool, storage: storage}
}

func (r *Repository) CreateWorkspace(ctx context.Context, name string) (domain.Workspace, error) {
	var result domain.Workspace
	var created time.Time
	err := r.pool.QueryRow(ctx, `
		INSERT INTO workspaces (name) VALUES ($1)
		RETURNING id, name, created_at`, name).Scan(&result.ID, &result.Name, &created)
	if err != nil {
		return result, fmt.Errorf("create workspace: %w", err)
	}
	result.CreatedAt = created.UTC().Format(time.RFC3339)
	return result, nil
}

func (r *Repository) CreateProject(ctx context.Context, workspaceID uuid.UUID, name string) (domain.Project, error) {
	var result domain.Project
	var created time.Time
	err := r.pool.QueryRow(ctx, `
		INSERT INTO projects (workspace_id, name) VALUES ($1, $2)
		RETURNING id, workspace_id, name, created_at`, workspaceID, name).Scan(&result.ID, &result.WorkspaceID, &result.Name, &created)
	if err != nil {
		return result, fmt.Errorf("create project: %w", err)
	}
	result.CreatedAt = created.UTC().Format(time.RFC3339)
	return result, nil
}

func (r *Repository) CreateScriptRevision(ctx context.Context, projectID uuid.UUID, name, content string) (domain.ScriptRevision, error) {
	if err := domain.ValidateSource(content); err != nil {
		return domain.ScriptRevision{}, err
	}
	revisionID := uuid.New()
	objectKey := fmt.Sprintf("sources/%s/%s.txt", projectID, revisionID)
	if err := r.storage.Put(ctx, objectKey, []byte(content), "text/plain; charset=utf-8"); err != nil {
		return domain.ScriptRevision{}, err
	}
	hash := domain.HashContent(content)
	var result domain.ScriptRevision
	var created time.Time
	err := r.pool.QueryRow(ctx, `
		INSERT INTO script_revisions (id, project_id, name, object_key, content_hash, content_length, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'uploaded')
		RETURNING id, project_id, name, content_hash, content_length, status, created_at`,
		revisionID, projectID, name, objectKey, hash, len([]byte(content))).Scan(
		&result.ID, &result.ProjectID, &result.Name, &result.ContentHash, &result.ContentLength, &result.Status, &created)
	if err != nil {
		return domain.ScriptRevision{}, fmt.Errorf("create script revision: %w", err)
	}
	result.CreatedAt = created.UTC().Format(time.RFC3339)
	return result, nil
}

type AnalysisRequest struct {
	OperationID uuid.UUID `json:"operation_id"`
	RevisionID  uuid.UUID `json:"revision_id"`
}

func (r *Repository) QueueAnalysis(ctx context.Context, revisionID uuid.UUID) (domain.Operation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Operation{}, fmt.Errorf("begin queue analysis: %w", err)
	}
	defer tx.Rollback(ctx)

	var projectID uuid.UUID
	var status string
	if err := tx.QueryRow(ctx, `SELECT project_id, status FROM script_revisions WHERE id = $1 FOR UPDATE`, revisionID).Scan(&projectID, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Operation{}, fmt.Errorf("script revision not found")
		}
		return domain.Operation{}, fmt.Errorf("lock script revision: %w", err)
	}
	if status == "approved" {
		return domain.Operation{}, fmt.Errorf("approved script revision cannot be analyzed again")
	}
	if status == "analyzing" {
		var existing domain.Operation
		if err := tx.QueryRow(ctx, `
			SELECT id, project_id, type, status, progress
			FROM operations
			WHERE project_id = $1 AND type = 'script_analysis' AND status IN ('queued', 'running')
			ORDER BY created_at DESC LIMIT 1`, projectID).Scan(
			&existing.ID, &existing.ProjectID, &existing.Type, &existing.Status, &existing.Progress); err == nil {
			return existing, nil
		}
	}
	operationID := uuid.New()
	payload, err := json.Marshal(AnalysisRequest{OperationID: operationID, RevisionID: revisionID})
	if err != nil {
		return domain.Operation{}, fmt.Errorf("marshal analysis request: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE script_revisions SET status = 'analyzing' WHERE id = $1`, revisionID); err != nil {
		return domain.Operation{}, fmt.Errorf("mark revision analyzing: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO operations (id, project_id, type, status, progress)
		VALUES ($1, $2, 'script_analysis', 'queued', 0)`, operationID, projectID); err != nil {
		return domain.Operation{}, fmt.Errorf("create operation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (operation_id, topic, event_key, payload)
		VALUES ($1, $2, $3, $4)`, operationID, analysisTopic, revisionID.String(), payload); err != nil {
		return domain.Operation{}, fmt.Errorf("create outbox event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Operation{}, fmt.Errorf("commit analysis operation: %w", err)
	}
	return domain.Operation{ID: operationID, ProjectID: projectID, Type: "script_analysis", Status: "queued", Progress: 0}, nil
}

func (r *Repository) GetOperation(ctx context.Context, operationID uuid.UUID) (domain.Operation, error) {
	var result domain.Operation
	err := r.pool.QueryRow(ctx, `
		SELECT id, project_id, type, status, progress, COALESCE(error_code, ''), COALESCE(error_message, '')
		FROM operations WHERE id = $1`, operationID).Scan(&result.ID, &result.ProjectID, &result.Type, &result.Status, &result.Progress, &result.ErrorCode, &result.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, fmt.Errorf("operation not found")
	}
	if err != nil {
		return result, fmt.Errorf("get operation: %w", err)
	}
	return result, nil
}

func (r *Repository) PendingOutbox(ctx context.Context, limit int) ([]OutboxEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, operation_id, topic, event_key, payload
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending outbox: %w", err)
	}
	defer rows.Close()
	var result []OutboxEvent
	for rows.Next() {
		var event OutboxEvent
		if err := rows.Scan(&event.ID, &event.OperationID, &event.Topic, &event.Key, &event.Payload); err != nil {
			return nil, fmt.Errorf("scan outbox: %w", err)
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

type OutboxEvent struct {
	ID          uuid.UUID
	OperationID uuid.UUID
	Topic       string
	Key         string
	Payload     []byte
}

func (r *Repository) MarkOutboxPublished(ctx context.Context, eventID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE outbox_events SET published_at = now(), attempts = attempts + 1 WHERE id = $1 AND published_at IS NULL`, eventID)
	return err
}

func (r *Repository) ProcessAnalysis(ctx context.Context, request AnalysisRequest) error {
	if err := r.setOperationRunning(ctx, request.OperationID); err != nil {
		return err
	}
	var objectKey, sourceHash string
	if err := r.pool.QueryRow(ctx, `SELECT object_key, content_hash FROM script_revisions WHERE id = $1`, request.RevisionID).Scan(&objectKey, &sourceHash); err != nil {
		return r.failOperation(ctx, request.OperationID, request.RevisionID, "revision_not_found", err)
	}
	content, err := r.storage.Get(ctx, objectKey)
	if err != nil {
		return r.failOperation(ctx, request.OperationID, request.RevisionID, "source_unavailable", err)
	}
	analysis, err := domain.AnalyzeScript(string(content))
	if err != nil {
		return r.failOperation(ctx, request.OperationID, request.RevisionID, "analysis_failed", err)
	}
	if analysis.SourceHash != sourceHash {
		return r.failOperation(ctx, request.OperationID, request.RevisionID, "source_changed", errors.New("source object hash does not match revision"))
	}
	encoded, err := json.Marshal(analysis)
	if err != nil {
		return r.failOperation(ctx, request.OperationID, request.RevisionID, "analysis_encode_failed", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO script_analysis_drafts (script_revision_id, source_hash, analysis, status)
		VALUES ($1, $2, $3, 'draft')
		ON CONFLICT (script_revision_id) DO UPDATE SET source_hash = EXCLUDED.source_hash, analysis = EXCLUDED.analysis, status = 'draft', created_at = now(), approved_at = NULL`, request.RevisionID, sourceHash, encoded); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE operations SET status = 'succeeded', progress = 100, updated_at = now(), completed_at = now()
		WHERE id = $1`, request.OperationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ApproveAnalysis(ctx context.Context, revisionID uuid.UUID) (domain.Analysis, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Analysis{}, err
	}
	defer tx.Rollback(ctx)
	var projectID uuid.UUID
	var status string
	var raw []byte
	if err := tx.QueryRow(ctx, `
		SELECT r.project_id, d.status, d.analysis
		FROM script_revisions r JOIN script_analysis_drafts d ON d.script_revision_id = r.id
		WHERE r.id = $1 FOR UPDATE`, revisionID).Scan(&projectID, &status, &raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Analysis{}, fmt.Errorf("analysis draft not found")
		}
		return domain.Analysis{}, err
	}
	var analysis domain.Analysis
	if err := json.Unmarshal(raw, &analysis); err != nil {
		return domain.Analysis{}, fmt.Errorf("decode analysis draft: %w", err)
	}
	if status == "approved" {
		return analysis, nil
	}
	for _, episode := range analysis.Episodes {
		var contentUnitID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO content_units (project_id, script_revision_id, episode_number, title, start_offset, end_offset)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, projectID, revisionID, episode.Number, episode.Title, episode.Anchor.StartOffset, max(episode.Anchor.EndOffset, episode.Anchor.StartOffset+1)).Scan(&contentUnitID); err != nil {
			return domain.Analysis{}, fmt.Errorf("materialize episode %d: %w", episode.Number, err)
		}
		for _, scene := range episode.Scenes {
			if _, err := tx.Exec(ctx, `
				INSERT INTO narrative_units (content_unit_id, kind, text, start_offset, end_offset)
				VALUES ($1, 'scene', $2, $3, $4)`, contentUnitID, scene.Heading, scene.Anchor.StartOffset, max(scene.Anchor.EndOffset, scene.Anchor.StartOffset+1)); err != nil {
				return domain.Analysis{}, fmt.Errorf("materialize scene: %w", err)
			}
			for _, unit := range scene.Narratives {
				if _, err := tx.Exec(ctx, `
					INSERT INTO narrative_units (content_unit_id, kind, text, start_offset, end_offset)
					VALUES ($1, $2, $3, $4, $5)`, contentUnitID, unit.Kind, unit.Text, unit.Anchor.StartOffset, max(unit.Anchor.EndOffset, unit.Anchor.StartOffset+1)); err != nil {
					return domain.Analysis{}, fmt.Errorf("materialize narrative: %w", err)
				}
			}
		}
	}
	for _, asset := range allAnalysisAssets(analysis) {
		var entityID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO entities (project_id, kind, canonical_name)
			VALUES ($1, $2, $3) RETURNING id`, projectID, asset.Kind, asset.Name).Scan(&entityID); err != nil {
			return domain.Analysis{}, fmt.Errorf("materialize asset %s: %w", asset.Name, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO production_requirements (project_id, entity_id, kind, description)
			VALUES ($1, $2, $3, $4)`, projectID, entityID, asset.Kind, fmt.Sprintf("%s 的生产准备需求", asset.Name)); err != nil {
			return domain.Analysis{}, fmt.Errorf("materialize requirement %s: %w", asset.Name, err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE script_analysis_drafts SET status = 'approved', approved_at = now() WHERE script_revision_id = $1`, revisionID); err != nil {
		return domain.Analysis{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE script_revisions SET status = 'approved' WHERE id = $1`, revisionID); err != nil {
		return domain.Analysis{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Analysis{}, err
	}
	return analysis, nil
}

func (r *Repository) GetProjectAnalysis(ctx context.Context, projectID uuid.UUID) (domain.Analysis, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT d.analysis
		FROM script_analysis_drafts d
		JOIN script_revisions r ON r.id = d.script_revision_id
		WHERE r.project_id = $1 AND d.status = 'approved'
		ORDER BY d.approved_at DESC LIMIT 1`, projectID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Analysis{}, fmt.Errorf("approved analysis not found")
	}
	if err != nil {
		return domain.Analysis{}, err
	}
	var result domain.Analysis
	if err := json.Unmarshal(raw, &result); err != nil {
		return domain.Analysis{}, err
	}
	return result, nil
}

func (r *Repository) GetAnalysisDraft(ctx context.Context, revisionID uuid.UUID) (domain.Analysis, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT analysis FROM script_analysis_drafts WHERE script_revision_id = $1 AND status = 'draft'`, revisionID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Analysis{}, fmt.Errorf("analysis draft not found")
	}
	if err != nil {
		return domain.Analysis{}, err
	}
	var result domain.Analysis
	if err := json.Unmarshal(raw, &result); err != nil {
		return domain.Analysis{}, err
	}
	return result, nil
}

func (r *Repository) HasInboxMessage(ctx context.Context, messageID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM inbox_messages WHERE message_id = $1)`, messageID).Scan(&exists)
	return exists, err
}

func (r *Repository) RecordInboxMessage(ctx context.Context, messageID, topic string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO inbox_messages (message_id, topic) VALUES ($1, $2) ON CONFLICT DO NOTHING`, messageID, topic)
	return err
}

func (r *Repository) setOperationRunning(ctx context.Context, operationID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE operations SET status = 'running', progress = 20, updated_at = now() WHERE id = $1 AND status IN ('queued', 'running')`, operationID)
	return err
}

func (r *Repository) failOperation(ctx context.Context, operationID, revisionID uuid.UUID, code string, cause error) error {
	_, _ = r.pool.Exec(ctx, `UPDATE script_revisions SET status = 'failed' WHERE id = $1`, revisionID)
	_, err := r.pool.Exec(ctx, `UPDATE operations SET status = 'failed', progress = 100, error_code = $2, error_message = $3, updated_at = now(), completed_at = now() WHERE id = $1`, operationID, code, cause.Error())
	return err
}

func allAnalysisAssets(analysis domain.Analysis) []domain.Asset {
	result := make([]domain.Asset, 0, len(analysis.Characters)+len(analysis.Locations)+len(analysis.Props)+len(analysis.Costumes))
	result = append(result, analysis.Characters...)
	result = append(result, analysis.Locations...)
	result = append(result, analysis.Props...)
	result = append(result, analysis.Costumes...)
	return result
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
