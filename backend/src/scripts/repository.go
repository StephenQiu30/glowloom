package scripts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
	"github.com/stephenqiu30/lanverse/backend/src/platform/messaging"
	"github.com/stephenqiu30/lanverse/backend/src/platform/objectstorage"
)

const analysisTopic = messaging.OperationTaskTopic

type ScriptRepository struct {
	orm     *gorm.DB
	storage *objectstorage.MinIOObjectStore
}

type workspaceRecord struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	Name      string
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (workspaceRecord) TableName() string { return "workspaces" }

type projectRecord struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	WorkspaceID uuid.UUID `gorm:"column:workspace_id;type:uuid"`
	Name        string
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (projectRecord) TableName() string { return "projects" }

func NewScriptRepository(orm *gorm.DB, storage *objectstorage.MinIOObjectStore) *ScriptRepository {
	return &ScriptRepository{orm: orm, storage: storage}
}

func (r *ScriptRepository) CreateWorkspace(ctx context.Context, name string) (Workspace, error) {
	if r.orm == nil {
		return Workspace{}, fmt.Errorf("script repository ORM is not configured")
	}
	record := workspaceRecord{ID: uuid.New(), Name: name}
	if err := r.orm.WithContext(ctx).Create(&record).Error; err != nil {
		return Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	return Workspace{ID: record.ID, Name: record.Name, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339)}, nil
}

func (r *ScriptRepository) CreateProject(ctx context.Context, workspaceID uuid.UUID, name string) (Project, error) {
	if r.orm == nil {
		return Project{}, fmt.Errorf("script repository ORM is not configured")
	}
	record := projectRecord{ID: uuid.New(), WorkspaceID: workspaceID, Name: name}
	if err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		return tx.Create(&record).Error
	}); err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return Project{ID: record.ID, WorkspaceID: record.WorkspaceID, Name: record.Name, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339)}, nil
}

func (r *ScriptRepository) CreateScriptRevision(ctx context.Context, projectID uuid.UUID, upload SourceUpload) (ScriptRevision, error) {
	document, err := ParseSourceDocument(upload.FileName, upload.MediaType, upload.Original)
	if err != nil {
		return ScriptRevision{}, err
	}
	if r.orm == nil {
		return ScriptRevision{}, fmt.Errorf("script repository ORM is not configured")
	}
	if r.storage == nil {
		return ScriptRevision{}, fmt.Errorf("script source storage is not configured")
	}
	revisionID := uuid.New()
	objectKey := fmt.Sprintf("sources/%s/%s.%s", projectID, revisionID, document.Format)
	if err := r.storage.Put(ctx, objectKey, upload.Original, document.MediaType); err != nil {
		return ScriptRevision{}, err
	}
	record := sourceRevisionRecord{ID: revisionID, ProjectID: projectID, Name: upload.FileName, ObjectKey: objectKey, ContentHash: document.OriginalHash, ContentLength: document.OriginalLength, SourceType: document.Format, Status: "uploaded"}
	if err := r.orm.WithContext(ctx).Create(&record).Error; err != nil {
		return ScriptRevision{}, fmt.Errorf("create script revision: %w", err)
	}
	return ScriptRevision{ID: record.ID, ProjectID: record.ProjectID, Name: record.Name, ContentHash: record.ContentHash, ContentLength: record.ContentLength, SourceType: record.SourceType, Status: record.Status, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339)}, nil
}

type AnalysisRequest struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ProjectID   uuid.UUID `json:"project_id"`
	OperationID uuid.UUID `json:"operation_id"`
	RevisionID  uuid.UUID `json:"revision_id"`
}

func (r *ScriptRepository) QueueAnalysis(ctx context.Context, revisionID uuid.UUID) (Operation, error) {
	if r.orm == nil {
		return Operation{}, fmt.Errorf("script repository ORM is not configured")
	}
	workspaceID, ok := database.WorkspaceID(ctx)
	if !ok {
		return Operation{}, fmt.Errorf("workspace context is missing")
	}
	var result Operation
	err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		var revision sourceRevisionRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&revision, "id = ?", revisionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("剧本版本")
			}
			return fmt.Errorf("lock script revision: %w", err)
		}
		var project projectRecord
		if err := tx.Where("id = ? AND workspace_id = ?", revision.ProjectID, workspaceID).First(&project).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("剧本版本")
			}
			return fmt.Errorf("validate script revision workspace: %w", err)
		}
		if revision.Status == "approved" {
			return httpapi.Validation("已批准剧本版本不能再次分析", "创建新的剧本版本后重试")
		}
		if revision.Status == "analyzing" {
			var existing operationRecord
			if err := tx.Where("project_id = ? AND type = ? AND status IN ?", revision.ProjectID, "script_analysis", []string{"queued", "running"}).Order("created_at DESC").First(&existing).Error; err == nil {
				result = Operation{ID: existing.ID, ProjectID: existing.ProjectID, Type: existing.Type, Status: existing.Status, Progress: existing.Progress, ErrorCode: existing.ErrorCode, Error: existing.ErrorMessage}
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		operationID := uuid.New()
		payload, err := json.Marshal(AnalysisRequest{WorkspaceID: workspaceID, ProjectID: project.ID, OperationID: operationID, RevisionID: revisionID})
		if err != nil {
			return fmt.Errorf("marshal analysis request: %w", err)
		}
		if err := tx.Model(&revision).Updates(map[string]any{"status": "analyzing"}).Error; err != nil {
			return fmt.Errorf("mark revision analyzing: %w", err)
		}
		if err := tx.Create(&operationRecord{ID: operationID, ProjectID: revision.ProjectID, Type: "script_analysis", Status: "queued", Progress: 0}).Error; err != nil {
			return fmt.Errorf("create operation: %w", err)
		}
		if err := tx.Create(&outboxRecord{ID: uuid.New(), OperationID: operationID, Topic: analysisTopic, EventKey: revisionID.String(), Payload: datatypes.JSON(payload)}).Error; err != nil {
			return fmt.Errorf("create outbox event: %w", err)
		}
		result = Operation{ID: operationID, ProjectID: revision.ProjectID, Type: "script_analysis", Status: "queued", Progress: 0}
		return nil
	})
	if err != nil {
		return Operation{}, err
	}
	return result, nil
}

func (r *ScriptRepository) GetOperation(ctx context.Context, operationID uuid.UUID) (Operation, error) {
	if r.orm == nil {
		return Operation{}, fmt.Errorf("script repository ORM is not configured")
	}
	var record operationRecord
	if err := r.orm.WithContext(ctx).Where("id = ?", operationID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Operation{}, httpapi.NotFound("Operation")
		}
		return Operation{}, fmt.Errorf("get operation: %w", err)
	}
	return Operation{ID: record.ID, ProjectID: record.ProjectID, Type: record.Type, Status: record.Status, Progress: record.Progress, ErrorCode: record.ErrorCode, Error: record.ErrorMessage}, nil
}

func (r *ScriptRepository) PendingOutbox(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if r.orm == nil {
		return nil, fmt.Errorf("script repository ORM is not configured")
	}
	var records []outboxRecord
	if err := r.orm.WithContext(ctx).Where("published_at IS NULL").Order("created_at").Limit(limit).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list pending outbox: %w", err)
	}
	result := make([]OutboxEvent, 0, len(records))
	for _, record := range records {
		result = append(result, OutboxEvent{ID: record.ID, OperationID: record.OperationID, Topic: record.Topic, Key: record.EventKey, Payload: append([]byte(nil), record.Payload...)})
	}
	return result, nil
}

type OutboxEvent struct {
	ID          uuid.UUID
	OperationID uuid.UUID
	Topic       string
	Key         string
	Payload     []byte
}

// The following records are persistence mappings only. They intentionally stay
// private to ScriptRepository; module services continue to exchange plain
// scripts.Model values and never see GORM or database column names.
type sourceRevisionRecord struct {
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID     uuid.UUID  `gorm:"column:project_id;type:uuid"`
	ContentUnitID *uuid.UUID `gorm:"column:content_unit_id;type:uuid"`
	Name          string
	ObjectKey     string `gorm:"column:object_key"`
	ContentHash   string `gorm:"column:content_hash"`
	ContentLength int    `gorm:"column:content_length"`
	SourceType    string `gorm:"column:source_type"`
	Status        string
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (sourceRevisionRecord) TableName() string { return "nar_source_revisions" }

type analysisDraftRecord struct {
	SourceRevisionID uuid.UUID      `gorm:"column:source_revision_id;type:uuid;primaryKey"`
	SourceHash       string         `gorm:"column:source_hash"`
	Analysis         datatypes.JSON `gorm:"column:analysis;type:jsonb"`
	Status           string
	CreatedAt        time.Time  `gorm:"column:created_at"`
	ApprovedAt       *time.Time `gorm:"column:approved_at"`
}

func (analysisDraftRecord) TableName() string { return "nar_analysis_drafts" }

type projectContentUnitRecord struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"column:project_id;type:uuid"`
	Kind      string
	Title     string
	Status    string
	Ordinal   int
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (projectContentUnitRecord) TableName() string { return "prj_content_units" }

type operationRecord struct {
	ID           uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID    uuid.UUID `gorm:"column:project_id;type:uuid"`
	Type         string
	Status       string
	Progress     int
	ErrorCode    string     `gorm:"column:error_code"`
	ErrorMessage string     `gorm:"column:error_message"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
	CompletedAt  *time.Time `gorm:"column:completed_at"`
}

func (operationRecord) TableName() string { return "operations" }

type outboxRecord struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	OperationID uuid.UUID `gorm:"column:operation_id;type:uuid"`
	Topic       string
	EventKey    string         `gorm:"column:event_key"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb"`
	Attempts    int
	PublishedAt *time.Time `gorm:"column:published_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
}

func (outboxRecord) TableName() string { return "outbox_events" }

type shotRecord struct {
	ID            uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID     uuid.UUID `gorm:"column:project_id;type:uuid"`
	ContentUnitID uuid.UUID `gorm:"column:content_unit_id;type:uuid"`
	ShotKey       string    `gorm:"column:shot_key"`
	Ordinal       int
	Status        string
	SourceBeatID  *uuid.UUID `gorm:"column:source_beat_id;type:uuid"`
}

func (shotRecord) TableName() string { return "sht_shots" }

type fixturePlanRecord struct {
	ID                uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID         uuid.UUID `gorm:"column:project_id;type:uuid"`
	TargetType        string    `gorm:"column:target_type"`
	TargetID          uuid.UUID `gorm:"column:target_id;type:uuid"`
	Status            string
	InputSnapshotHash string `gorm:"column:input_snapshot_hash"`
	PromptHash        string `gorm:"column:prompt_hash"`
}

func (fixturePlanRecord) TableName() string { return "gen_plans" }

type fixturePlanItemRecord struct {
	ID            uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	PlanID        uuid.UUID `gorm:"column:plan_id;type:uuid"`
	Ordinal       int
	CapabilityKey string `gorm:"column:capability_key"`
	Prompt        string
	Status        string
}

func (fixturePlanItemRecord) TableName() string { return "gen_plan_items" }

type fixtureJobRecord struct {
	ID               uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	PlanItemID       uuid.UUID `gorm:"column:plan_item_id;type:uuid"`
	OperationID      uuid.UUID `gorm:"column:operation_id;type:uuid"`
	Status           string
	CurrentAttemptID *uuid.UUID `gorm:"column:current_attempt_id;type:uuid"`
}

func (fixtureJobRecord) TableName() string { return "exec_generation_jobs" }

type fixtureAttemptRecord struct {
	ID              uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	JobID           uuid.UUID `gorm:"column:job_id;type:uuid"`
	AttemptNo       int       `gorm:"column:attempt_no"`
	Status          string
	ExternalJobID   string `gorm:"column:external_job_id"`
	ResultCertainty string `gorm:"column:result_certainty"`
}

func (fixtureAttemptRecord) TableName() string { return "exec_attempts" }

type artifactRecord struct {
	ID              uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID       uuid.UUID `gorm:"column:project_id;type:uuid"`
	ContentHash     string    `gorm:"column:content_hash"`
	SizeBytes       int64     `gorm:"column:size_bytes"`
	MediaType       string    `gorm:"column:media_type"`
	Purpose         string
	Status          string
	ObjectKey       string `gorm:"column:object_key"`
	ObjectVersionID string `gorm:"column:object_version_id"`
}

func (artifactRecord) TableName() string { return "media_artifacts" }

type candidateRecord struct {
	ID         uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID  uuid.UUID  `gorm:"column:project_id;type:uuid"`
	JobID      *uuid.UUID `gorm:"column:job_id;type:uuid"`
	TargetType string     `gorm:"column:target_type"`
	TargetID   uuid.UUID  `gorm:"column:target_id;type:uuid"`
	ArtifactID uuid.UUID  `gorm:"column:artifact_id;type:uuid"`
	Status     string
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func (candidateRecord) TableName() string { return "media_candidates" }

type selectionRecord struct {
	ID               uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID        uuid.UUID `gorm:"column:project_id;type:uuid"`
	TargetType       string    `gorm:"column:target_type"`
	TargetID         uuid.UUID `gorm:"column:target_id;type:uuid"`
	SelectionPurpose string    `gorm:"column:selection_purpose"`
	CandidateID      uuid.UUID `gorm:"column:candidate_id;type:uuid"`
	Status           string
}

func (selectionRecord) TableName() string { return "media_selection_decisions" }

type inboxRecord struct {
	MessageID string `gorm:"column:message_id;primaryKey"`
	Topic     string
}

func (inboxRecord) TableName() string { return "inbox_messages" }

type importRunRecord struct {
	ID                 uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID          uuid.UUID `gorm:"column:project_id;type:uuid"`
	OperationID        uuid.UUID `gorm:"column:operation_id;type:uuid"`
	SourceManifestHash string    `gorm:"column:source_manifest_hash"`
	ParserConfigHash   string    `gorm:"column:parser_config_hash"`
	Status             string
	ParseReport        datatypes.JSON `gorm:"column:parse_report;type:jsonb"`
}

func (importRunRecord) TableName() string { return "nar_import_runs" }

type analysisRunRecord struct {
	ID                     uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID              uuid.UUID `gorm:"column:project_id;type:uuid"`
	RootOperationID        uuid.UUID `gorm:"column:root_operation_id;type:uuid"`
	SourceManifestHash     string    `gorm:"column:source_manifest_hash"`
	CurrentStage           string    `gorm:"column:current_stage"`
	CurrentStageGeneration int       `gorm:"column:current_stage_generation"`
	CurrentGate            string    `gorm:"column:current_gate"`
	Status                 string
	InputHash              string `gorm:"column:input_hash"`
}

func (analysisRunRecord) TableName() string { return "nar_analysis_runs" }

type breakdownRecord struct {
	ID               uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	AnalysisRunID    uuid.UUID `gorm:"column:analysis_run_id;type:uuid"`
	RevisionNo       int       `gorm:"column:revision_no"`
	Status           string
	SegmentationHash string `gorm:"column:segmentation_hash"`
	CoverageHash     string `gorm:"column:coverage_hash"`
}

func (breakdownRecord) TableName() string { return "nar_episode_breakdown_revisions" }

type episodeCandidateRecord struct {
	ID                  uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	BreakdownRevisionID uuid.UUID `gorm:"column:breakdown_revision_id;type:uuid"`
	TemporaryKey        string    `gorm:"column:temporary_key"`
	Ordinal             int
	Title               string
	RuleCode            string `gorm:"column:rule_code"`
	Confidence          float64
	Decision            string
}

func (episodeCandidateRecord) TableName() string { return "nar_episode_candidates" }

type narrativeRevisionRecord struct {
	ID                  uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID           uuid.UUID  `gorm:"column:project_id;type:uuid"`
	BreakdownRevisionID *uuid.UUID `gorm:"column:breakdown_revision_id;type:uuid"`
	RevisionNo          int        `gorm:"column:revision_no"`
	Status              string
	ContentHash         string `gorm:"column:content_hash"`
	Completeness        string
}

func (narrativeRevisionRecord) TableName() string { return "nar_narrative_revisions" }

type contentOrderRevisionRecord struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID   uuid.UUID `gorm:"column:project_id;type:uuid"`
	RevisionNo  int       `gorm:"column:revision_no"`
	Status      string
	ContentHash string `gorm:"column:content_hash"`
}

func (contentOrderRevisionRecord) TableName() string { return "prj_content_order_revisions" }

type contentOrderItemRecord struct {
	ID              uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	OrderRevisionID uuid.UUID `gorm:"column:order_revision_id;type:uuid"`
	ContentUnitID   uuid.UUID `gorm:"column:content_unit_id;type:uuid"`
	Ordinal         int
}

func (contentOrderItemRecord) TableName() string { return "prj_content_order_items" }

type sceneRecord struct {
	ID                  uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	NarrativeRevisionID uuid.UUID `gorm:"column:narrative_revision_id;type:uuid"`
	ContentUnitID       uuid.UUID `gorm:"column:content_unit_id;type:uuid"`
	Ordinal             int
	Heading             string
	LocationHint        string `gorm:"column:location_hint"`
	StartOffset         int    `gorm:"column:start_offset"`
	EndOffset           int    `gorm:"column:end_offset"`
}

func (sceneRecord) TableName() string { return "nar_scenes" }

type beatRecord struct {
	ID       uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	SceneID  uuid.UUID `gorm:"column:scene_id;type:uuid"`
	Ordinal  int
	Goal     string
	Conflict *string
}

func (beatRecord) TableName() string { return "nar_beats" }

type canonicalEntityRecord struct {
	ID            uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID     uuid.UUID `gorm:"column:project_id;type:uuid"`
	Type          string
	Status        string
	CanonicalName string `gorm:"column:canonical_name"`
}

func (canonicalEntityRecord) TableName() string { return "pk_entities" }

type requirementItemRecord struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"column:project_id;type:uuid"`
	StableKey string    `gorm:"column:stable_key"`
	Status    string
}

func (requirementItemRecord) TableName() string { return "pk_production_requirement_items" }

type requirementRevisionRecord struct {
	ID         uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ItemID     uuid.UUID `gorm:"column:item_id;type:uuid"`
	RevisionNo int       `gorm:"column:revision_no"`
	Type       string
	Purpose    string
	Quantity   float64
	Unit       string
	Decision   string
	Status     string
}

func (requirementRevisionRecord) TableName() string { return "pk_production_requirement_revisions" }

type mentionRecord struct {
	ID                  uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	NarrativeRevisionID uuid.UUID  `gorm:"column:narrative_revision_id;type:uuid"`
	SceneID             *uuid.UUID `gorm:"column:scene_id;type:uuid"`
	BeatID              *uuid.UUID `gorm:"column:beat_id;type:uuid"`
	ElementType         string     `gorm:"column:element_type"`
	SurfaceText         string     `gorm:"column:surface_text"`
	Status              string
	StartOffset         int `gorm:"column:start_offset"`
	EndOffset           int `gorm:"column:end_offset"`
}

func (mentionRecord) TableName() string { return "nar_production_element_mentions" }

type mentionResolutionRecord struct {
	ID                  uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	MentionID           uuid.UUID `gorm:"column:mention_id;type:uuid"`
	NarrativeRevisionID uuid.UUID `gorm:"column:narrative_revision_id;type:uuid"`
	Action              string
	EntityID            *uuid.UUID `gorm:"column:entity_id;type:uuid"`
	Reason              string
	Status              string
}

func (mentionResolutionRecord) TableName() string { return "pk_mention_resolutions" }

func (r *ScriptRepository) MarkOutboxPublished(ctx context.Context, eventID uuid.UUID) error {
	if r.orm == nil {
		return fmt.Errorf("script repository ORM is not configured")
	}
	return r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record outboxRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND published_at IS NULL", eventID).First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		now := time.Now().UTC()
		record.Attempts++
		record.PublishedAt = &now
		return tx.Model(&record).Updates(map[string]any{"attempts": record.Attempts, "published_at": record.PublishedAt}).Error
	})
}

func (r *ScriptRepository) ProcessAnalysis(ctx context.Context, request AnalysisRequest) error {
	if r.orm == nil {
		return fmt.Errorf("script repository ORM is not configured")
	}
	if request.WorkspaceID == uuid.Nil || request.ProjectID == uuid.Nil || request.OperationID == uuid.Nil || request.RevisionID == uuid.Nil {
		return httpapi.Validation("解析任务缺少完整租户绑定", "重新投递包含工作区、项目、任务和剧本版本标识的消息")
	}
	tenantContext := database.WithWorkspaceID(ctx, request.WorkspaceID)
	revision, completed, err := r.startAnalysis(tenantContext, request)
	if err != nil {
		return err
	}
	if completed {
		return nil
	}
	if r.storage == nil {
		return r.failOperation(tenantContext, request, "source_unavailable", errors.New("script source storage is not configured"))
	}
	content, err := r.storage.Get(tenantContext, revision.ObjectKey)
	if err != nil {
		return r.failOperation(tenantContext, request, "source_unavailable", err)
	}
	document, err := ParseSourceDocument(revision.Name, mediaTypeForSourceType(revision.SourceType), content)
	if err != nil {
		return r.failOperation(tenantContext, request, "source_parse_failed", err)
	}
	if document.OriginalHash != revision.ContentHash {
		return r.failOperation(tenantContext, request, "source_changed", errors.New("source object hash does not match revision"))
	}
	analysis, err := AnalyzeScript(document.Text)
	if err != nil {
		return r.failOperation(tenantContext, request, "analysis_failed", err)
	}
	analysis.SourceHash = document.OriginalHash
	analysis.ParseReport = ParseReport{Status: "complete", Format: document.Format, ParserVersion: "deterministic-script-parser-v1", OriginalHash: document.OriginalHash, TextHash: document.TextHash, CharacterCount: len([]rune(document.Text)), ParagraphCount: document.ParagraphCount, FailedScopes: []string{}}
	encoded, err := json.Marshal(analysis)
	if err != nil {
		return r.failOperation(tenantContext, request, "analysis_encode_failed", err)
	}
	return database.WithWorkspaceTransaction(tenantContext, r.orm, func(tx *gorm.DB) error {
		draft := analysisDraftRecord{SourceRevisionID: request.RevisionID, SourceHash: revision.ContentHash, Analysis: datatypes.JSON(encoded), Status: "draft", CreatedAt: time.Now().UTC()}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_revision_id"}}, DoUpdates: clause.AssignmentColumns([]string{"source_hash", "analysis", "status", "created_at", "approved_at"})}).Create(&draft).Error; err != nil {
			return err
		}
		result := tx.Model(&revision).Where("id = ? AND project_id = ?", request.RevisionID, request.ProjectID).Update("status", "waiting_user")
		if result.Error != nil {
			return fmt.Errorf("mark source revision waiting for approval: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return httpapi.NotFound("剧本版本")
		}
		completedAt := time.Now().UTC()
		result = tx.Model(&operationRecord{}).Where("id = ? AND project_id = ? AND type = ? AND status = ?", request.OperationID, request.ProjectID, "script_analysis", "running").Updates(map[string]any{"status": "succeeded", "progress": 100, "updated_at": completedAt, "completed_at": completedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return httpapi.Conflict("解析任务状态已变化", "刷新任务状态后重试")
		}
		return nil
	})
}

func (r *ScriptRepository) ApproveAnalysis(ctx context.Context, revisionID uuid.UUID) (Analysis, error) {
	if r.orm == nil {
		return Analysis{}, fmt.Errorf("script repository ORM is not configured")
	}
	var analysis Analysis
	err := r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var revision sourceRevisionRecord
		if err := tx.Where("id = ?", revisionID).First(&revision).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("剧本版本")
			}
			return fmt.Errorf("load script revision: %w", err)
		}
		var draft analysisDraftRecord
		if err := tx.Where("source_revision_id = ?", revisionID).First(&draft).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("分析草稿")
			}
			return fmt.Errorf("load analysis draft: %w", err)
		}
		if err := json.Unmarshal(draft.Analysis, &analysis); err != nil {
			return fmt.Errorf("decode analysis draft: %w", err)
		}
		if draft.Status == "approved" {
			return nil
		}
		for episodeIndex, episode := range analysis.Episodes {
			var contentUnit projectContentUnitRecord
			contentErr := tx.Where("project_id = ? AND ordinal = ?", revision.ProjectID, episode.Number).Order("created_at DESC").First(&contentUnit).Error
			if errors.Is(contentErr, gorm.ErrRecordNotFound) {
				contentUnit = projectContentUnitRecord{ID: uuid.New(), ProjectID: revision.ProjectID, Kind: "episode", Title: episode.Title, Status: "active", Ordinal: episode.Number}
				if err := tx.Create(&contentUnit).Error; err != nil {
					return fmt.Errorf("materialize project content unit %d: %w", episode.Number, err)
				}
			} else if contentErr != nil {
				return fmt.Errorf("read project content unit %d: %w", episode.Number, contentErr)
			} else if err := tx.Model(&contentUnit).Where("id = ?", contentUnit.ID).Updates(map[string]any{"title": episode.Title, "status": "active"}).Error; err != nil {
				return fmt.Errorf("update project content unit %d: %w", episode.Number, err)
			}
			analysis.Episodes[episodeIndex].ContentUnitID = contentUnit.ID
		}
		if err := materializeCanonicalAnalysis(ctx, tx, revision.ProjectID, analysis); err != nil {
			return err
		}
		encodedApproved, err := json.Marshal(analysis)
		if err != nil {
			return fmt.Errorf("encode approved analysis: %w", err)
		}
		approvedAt := time.Now().UTC()
		if err := tx.Model(&draft).Where("source_revision_id = ?", revisionID).Updates(map[string]any{"analysis": datatypes.JSON(encodedApproved), "status": "approved", "approved_at": approvedAt}).Error; err != nil {
			return fmt.Errorf("persist approved analysis: %w", err)
		}
		if err := tx.Model(&revision).Where("id = ?", revisionID).Update("status", "approved").Error; err != nil {
			return fmt.Errorf("mark script revision approved: %w", err)
		}
		return nil
	})
	if err != nil {
		return Analysis{}, err
	}
	return analysis, nil
}

func (r *ScriptRepository) GetProjectAnalysis(ctx context.Context, projectID uuid.UUID) (Analysis, error) {
	if r.orm == nil {
		return Analysis{}, fmt.Errorf("script repository ORM is not configured")
	}
	var revisions []sourceRevisionRecord
	if err := r.orm.WithContext(ctx).Where("project_id = ?", projectID).Find(&revisions).Error; err != nil {
		return Analysis{}, err
	}
	if len(revisions) == 0 {
		return Analysis{}, httpapi.NotFound("已批准分析")
	}
	revisionIDs := make([]uuid.UUID, 0, len(revisions))
	for _, revision := range revisions {
		revisionIDs = append(revisionIDs, revision.ID)
	}
	var draft analysisDraftRecord
	if err := r.orm.WithContext(ctx).Where("source_revision_id IN ? AND status = ?", revisionIDs, "approved").Order("approved_at DESC").First(&draft).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Analysis{}, httpapi.NotFound("已批准分析")
		}
		return Analysis{}, err
	}
	var result Analysis
	if err := json.Unmarshal(draft.Analysis, &result); err != nil {
		return Analysis{}, err
	}
	return result, nil
}

func (r *ScriptRepository) GetAnalysisDraft(ctx context.Context, revisionID uuid.UUID) (Analysis, error) {
	if r.orm == nil {
		return Analysis{}, fmt.Errorf("script repository ORM is not configured")
	}
	var draft analysisDraftRecord
	if err := r.orm.WithContext(ctx).Where("source_revision_id = ? AND status = ?", revisionID, "draft").First(&draft).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Analysis{}, httpapi.NotFound("分析草稿")
		}
		return Analysis{}, err
	}
	var result Analysis
	if err := json.Unmarshal(draft.Analysis, &result); err != nil {
		return Analysis{}, err
	}
	return result, nil
}

func (r *ScriptRepository) CreateShots(ctx context.Context, projectID, contentUnitID uuid.UUID, count int) ([]Shot, error) {
	if r.orm == nil {
		return nil, fmt.Errorf("script repository ORM is not configured")
	}
	var projectUnit projectContentUnitRecord
	if err := r.orm.WithContext(ctx).Where("id = ? AND project_id = ?", contentUnitID, projectID).First(&projectUnit).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpapi.NotFound("内容单元")
		}
		return nil, fmt.Errorf("check content unit: %w", err)
	}
	var created []Shot
	err := r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked projectContentUnitRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", projectUnit.ID).Error; err != nil {
			return fmt.Errorf("lock content unit: %w", err)
		}
		var last shotRecord
		start := 0
		if err := tx.Where("content_unit_id = ?", contentUnitID).Order("ordinal DESC").First(&last).Error; err == nil {
			start = last.Ordinal
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lock shot order: %w", err)
		}
		created = make([]Shot, 0, count)
		for index := 1; index <= count; index++ {
			record := shotRecord{ID: uuid.New(), ProjectID: projectID, ContentUnitID: contentUnitID, ShotKey: fmt.Sprintf("shot-%03d", start+index), Ordinal: start + index, Status: "draft"}
			if err := tx.Create(&record).Error; err != nil {
				return fmt.Errorf("create shot %d: %w", index, err)
			}
			created = append(created, Shot{ID: record.ID, ProjectID: record.ProjectID, ContentUnitID: record.ContentUnitID, ShotKey: record.ShotKey, Ordinal: record.Ordinal, Status: record.Status})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *ScriptRepository) ListShots(ctx context.Context, projectID, contentUnitID uuid.UUID) ([]Shot, error) {
	if r.orm == nil {
		return nil, fmt.Errorf("script repository ORM is not configured")
	}
	var records []shotRecord
	if err := r.orm.WithContext(ctx).Where("project_id = ? AND content_unit_id = ?", projectID, contentUnitID).Order("ordinal").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list shots: %w", err)
	}
	result := make([]Shot, 0, len(records))
	for _, record := range records {
		result = append(result, Shot{ID: record.ID, ProjectID: record.ProjectID, ContentUnitID: record.ContentUnitID, ShotKey: record.ShotKey, Ordinal: record.Ordinal, Status: record.Status, SourceBeatID: record.SourceBeatID})
	}
	return result, nil
}

func (r *ScriptRepository) CreateFixtureCandidate(ctx context.Context, shotID uuid.UUID, purpose string) (Candidate, error) {
	if r.orm == nil {
		return Candidate{}, fmt.Errorf("script repository ORM is not configured")
	}
	var shot shotRecord
	if err := r.orm.WithContext(ctx).Where("id = ?", shotID).First(&shot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Candidate{}, httpapi.NotFound("镜头")
		}
		return Candidate{}, err
	}
	var candidates []candidateRecord
	if err := r.orm.WithContext(ctx).Where("target_type = ? AND target_id = ? AND status = ?", "shot", shotID, "ready").Order("created_at").Find(&candidates).Error; err != nil {
		return Candidate{}, fmt.Errorf("find fixture candidate: %w", err)
	}
	for _, candidate := range candidates {
		var artifact artifactRecord
		if err := r.orm.WithContext(ctx).Where("id = ? AND purpose = ? AND status = ?", candidate.ArtifactID, purpose, "ready").First(&artifact).Error; err == nil {
			return Candidate{ID: candidate.ID, ProjectID: candidate.ProjectID, TargetType: candidate.TargetType, TargetID: candidate.TargetID, ArtifactID: candidate.ArtifactID, Status: candidate.Status, Fixture: true, ObjectKey: artifact.ObjectKey, ContentHash: artifact.ContentHash}, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return Candidate{}, fmt.Errorf("find fixture artifact: %w", err)
		}
	}
	content := []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="100%%" height="100%%" fill="#141414"/><text x="24" y="48" fill="white" font-size="24">Fixture %s</text></svg>`, shotID.String()))
	objectKey := fmt.Sprintf("fixtures/%s/%s.svg", shot.ProjectID, shotID)
	version, err := r.storage.PutVersioned(ctx, objectKey, content, "image/svg+xml")
	if err != nil {
		return Candidate{}, fmt.Errorf("store fixture: %w", err)
	}
	hash := HashContent(string(content))
	operationID, planID, planItemID, jobID, attemptID, artifactID, candidateID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&operationRecord{ID: operationID, ProjectID: shot.ProjectID, Type: "fixture_candidate", Status: "running", Progress: 50}).Error; err != nil {
			return fmt.Errorf("create fixture operation: %w", err)
		}
		if err := tx.Create(&fixturePlanRecord{ID: planID, ProjectID: shot.ProjectID, TargetType: "shot", TargetID: shotID, Status: "approved", InputSnapshotHash: hash, PromptHash: hash}).Error; err != nil {
			return fmt.Errorf("create fixture plan: %w", err)
		}
		if err := tx.Create(&fixturePlanItemRecord{ID: planItemID, PlanID: planID, Ordinal: 1, CapabilityKey: "fixture", Prompt: purpose, Status: "selected"}).Error; err != nil {
			return fmt.Errorf("create fixture plan item: %w", err)
		}
		if err := tx.Create(&fixtureJobRecord{ID: jobID, PlanItemID: planItemID, OperationID: operationID, Status: "running", CurrentAttemptID: &attemptID}).Error; err != nil {
			return fmt.Errorf("create fixture job: %w", err)
		}
		if err := tx.Create(&fixtureAttemptRecord{ID: attemptID, JobID: jobID, AttemptNo: 1, Status: "succeeded", ExternalJobID: "fixture:" + shotID.String(), ResultCertainty: "created"}).Error; err != nil {
			return fmt.Errorf("create fixture attempt: %w", err)
		}
		if err := tx.Create(&artifactRecord{ID: artifactID, ProjectID: shot.ProjectID, ContentHash: hash, SizeBytes: int64(len(content)), MediaType: "image/svg+xml", Purpose: purpose, Status: "ready", ObjectKey: objectKey, ObjectVersionID: version.VersionID}).Error; err != nil {
			return fmt.Errorf("create fixture artifact: %w", err)
		}
		if err := tx.Create(&candidateRecord{ID: candidateID, ProjectID: shot.ProjectID, JobID: &jobID, TargetType: "shot", TargetID: shotID, ArtifactID: artifactID, Status: "ready"}).Error; err != nil {
			return fmt.Errorf("create fixture candidate: %w", err)
		}
		if err := tx.Model(&fixtureJobRecord{}).Where("id = ?", jobID).Update("status", "succeeded").Error; err != nil {
			return fmt.Errorf("complete fixture job: %w", err)
		}
		now := time.Now().UTC()
		if err := tx.Model(&operationRecord{}).Where("id = ?", operationID).Updates(map[string]any{"status": "succeeded", "progress": 100, "updated_at": now, "completed_at": now}).Error; err != nil {
			return fmt.Errorf("complete fixture operation: %w", err)
		}
		return nil
	}); err != nil {
		return Candidate{}, err
	}
	return Candidate{ID: candidateID, ProjectID: shot.ProjectID, TargetType: "shot", TargetID: shotID, ArtifactID: artifactID, Status: "ready", Fixture: true, ObjectKey: objectKey, ContentHash: hash}, nil
}

func (r *ScriptRepository) SelectCandidate(ctx context.Context, candidateID uuid.UUID, purpose string) (Selection, error) {
	if r.orm == nil {
		return Selection{}, fmt.Errorf("script repository ORM is not configured")
	}
	var candidate candidateRecord
	if err := r.orm.WithContext(ctx).Where("id = ? AND status = ?", candidateID, "ready").First(&candidate).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Selection{}, httpapi.NotFound("候选或候选尚未就绪")
		}
		return Selection{}, err
	}
	selection := Selection{ID: uuid.New(), ProjectID: candidate.ProjectID, TargetType: candidate.TargetType, TargetID: candidate.TargetID, SelectionPurpose: purpose, CandidateID: candidateID, Status: "current"}
	if err := r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&selectionRecord{}).Where("project_id = ? AND target_type = ? AND target_id = ? AND selection_purpose = ? AND status = ?", candidate.ProjectID, candidate.TargetType, candidate.TargetID, purpose, "current").Update("status", "superseded").Error; err != nil {
			return err
		}
		return tx.Create(&selectionRecord{ID: selection.ID, ProjectID: selection.ProjectID, TargetType: selection.TargetType, TargetID: selection.TargetID, SelectionPurpose: selection.SelectionPurpose, CandidateID: selection.CandidateID, Status: selection.Status}).Error
	}); err != nil {
		return Selection{}, fmt.Errorf("create selection: %w", err)
	}
	return selection, nil
}

func (r *ScriptRepository) HasInboxMessage(ctx context.Context, messageID string) (bool, error) {
	if r.orm == nil {
		return false, fmt.Errorf("script repository ORM is not configured")
	}
	var record inboxRecord
	if err := r.orm.WithContext(ctx).Where("message_id = ?", messageID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *ScriptRepository) RecordInboxMessage(ctx context.Context, messageID, topic string) error {
	if r.orm == nil {
		return fmt.Errorf("script repository ORM is not configured")
	}
	return r.orm.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&inboxRecord{MessageID: messageID, Topic: topic}).Error
}

func (r *ScriptRepository) startAnalysis(ctx context.Context, request AnalysisRequest) (sourceRevisionRecord, bool, error) {
	if r.orm == nil {
		return sourceRevisionRecord{}, false, fmt.Errorf("script repository ORM is not configured")
	}
	var revision sourceRevisionRecord
	completed := false
	err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		var project projectRecord
		if err := tx.Where("id = ? AND workspace_id = ?", request.ProjectID, request.WorkspaceID).First(&project).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("解析任务")
			}
			return fmt.Errorf("validate analysis project: %w", err)
		}
		if err := tx.Where("id = ? AND project_id = ?", request.RevisionID, request.ProjectID).First(&revision).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("解析任务")
			}
			return fmt.Errorf("validate analysis revision: %w", err)
		}
		var outbox outboxRecord
		if err := tx.Where("operation_id = ? AND topic = ?", request.OperationID, analysisTopic).First(&outbox).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("解析任务")
			}
			return fmt.Errorf("validate analysis outbox binding: %w", err)
		}
		var boundRequest AnalysisRequest
		if err := json.Unmarshal(outbox.Payload, &boundRequest); err != nil {
			return fmt.Errorf("decode analysis outbox binding: %w", err)
		}
		if boundRequest != request {
			return httpapi.NotFound("解析任务")
		}
		var operation operationRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND type = ?", request.OperationID, request.ProjectID, "script_analysis").First(&operation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("解析任务")
			}
			return fmt.Errorf("validate analysis operation: %w", err)
		}
		if operation.Status == "succeeded" {
			completed = true
			return nil
		}
		if operation.Status != "queued" && operation.Status != "running" {
			return httpapi.Conflict("解析任务不可执行", "刷新任务状态并重新创建分析任务")
		}
		result := tx.Model(&operationRecord{}).Where("id = ? AND project_id = ? AND type = ? AND status IN ?", request.OperationID, request.ProjectID, "script_analysis", []string{"queued", "running"}).Updates(map[string]any{"status": "running", "progress": 20, "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return httpapi.Conflict("解析任务状态已变化", "刷新任务状态后重试")
		}
		return nil
	})
	if err != nil {
		return sourceRevisionRecord{}, false, err
	}
	return revision, completed, nil
}

func (r *ScriptRepository) failOperation(ctx context.Context, request AnalysisRequest, code string, cause error) error {
	if r.orm == nil {
		return fmt.Errorf("script repository ORM is not configured")
	}
	return database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		result := tx.Model(&sourceRevisionRecord{}).Where("id = ? AND project_id = ?", request.RevisionID, request.ProjectID).Update("status", "failed")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return httpapi.NotFound("剧本版本")
		}
		now := time.Now().UTC()
		result = tx.Model(&operationRecord{}).Where("id = ? AND project_id = ? AND type = ? AND status = ?", request.OperationID, request.ProjectID, "script_analysis", "running").Updates(map[string]any{"status": "failed", "progress": 100, "error_code": code, "error_message": cause.Error(), "updated_at": now, "completed_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return httpapi.Conflict("解析任务状态已变化", "刷新任务状态后重试")
		}
		return nil
	})
}

func allAnalysisAssets(analysis Analysis) []Asset {
	result := make([]Asset, 0, len(analysis.Characters)+len(analysis.Locations)+len(analysis.Props)+len(analysis.Costumes))
	result = append(result, analysis.Characters...)
	result = append(result, analysis.Locations...)
	result = append(result, analysis.Props...)
	result = append(result, analysis.Costumes...)
	return result
}

// materializeCanonicalAnalysis writes the current M02/M03/M04 facts in the
// same transaction as human approval. Only module-owned canonical tables are
// written; the UI reads the approved M03 draft and never owns a second fact set.
func materializeCanonicalAnalysis(ctx context.Context, tx *gorm.DB, projectID uuid.UUID, analysis Analysis) error {
	tx = tx.WithContext(ctx)
	var operation operationRecord
	if err := tx.Where("project_id = ? AND type = ?", projectID, "script_analysis").Order("created_at DESC").First(&operation).Error; err != nil {
		return fmt.Errorf("find analysis operation for canonical materialization: %w", err)
	}
	parseReport, err := json.Marshal(struct {
		ParseReport
		EpisodeCount int `json:"episode_count"`
	}{ParseReport: analysis.ParseReport, EpisodeCount: len(analysis.Episodes)})
	if err != nil {
		return fmt.Errorf("encode parse report: %w", err)
	}
	manifestHash := HashContent(fmt.Sprintf("%s:%d", analysis.SourceHash, len(analysis.Episodes)))
	if err := tx.Create(&importRunRecord{ID: uuid.New(), ProjectID: projectID, OperationID: operation.ID, SourceManifestHash: manifestHash, ParserConfigHash: HashContent("deterministic-script-parser"), Status: "completed", ParseReport: datatypes.JSON(parseReport)}).Error; err != nil {
		return fmt.Errorf("materialize import run: %w", err)
	}
	analysisRun := analysisRunRecord{ID: uuid.New(), ProjectID: projectID, RootOperationID: operation.ID, SourceManifestHash: manifestHash, CurrentStage: "knowledge", CurrentStageGeneration: 1, CurrentGate: "approved", Status: "completed", InputHash: analysis.SourceHash}
	if err := tx.Create(&analysisRun).Error; err != nil {
		return fmt.Errorf("materialize analysis run: %w", err)
	}
	breakdownHash := HashContent(fmt.Sprintf("breakdown:%s", analysis.SourceHash))
	breakdown := breakdownRecord{ID: uuid.New(), AnalysisRunID: analysisRun.ID, RevisionNo: 1, Status: "approved", SegmentationHash: breakdownHash, CoverageHash: analysis.SourceHash}
	if err := tx.Create(&breakdown).Error; err != nil {
		return fmt.Errorf("materialize breakdown revision: %w", err)
	}
	for _, episode := range analysis.Episodes {
		candidate := episodeCandidateRecord{ID: uuid.New(), BreakdownRevisionID: breakdown.ID, TemporaryKey: fmt.Sprintf("episode-%d", episode.Number), Ordinal: episode.Number, Title: episode.Title, RuleCode: "deterministic_heading", Confidence: 1, Decision: "accepted"}
		if err := tx.Create(&candidate).Error; err != nil {
			return fmt.Errorf("materialize episode candidate %d: %w", episode.Number, err)
		}
	}
	var previousOrder contentOrderRevisionRecord
	orderRevisionNo := 1
	if err := tx.Where("project_id = ?", projectID).Order("revision_no DESC").First(&previousOrder).Error; err == nil {
		orderRevisionNo = previousOrder.RevisionNo + 1
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("allocate content order revision: %w", err)
	}
	orderRevision := contentOrderRevisionRecord{ID: uuid.New(), ProjectID: projectID, RevisionNo: orderRevisionNo, Status: "approved", ContentHash: analysis.SourceHash}
	if err := tx.Create(&orderRevision).Error; err != nil {
		return fmt.Errorf("materialize content order revision: %w", err)
	}
	var previousNarrative narrativeRevisionRecord
	narrativeRevisionNo := 1
	if err := tx.Where("project_id = ?", projectID).Order("revision_no DESC").First(&previousNarrative).Error; err == nil {
		narrativeRevisionNo = previousNarrative.RevisionNo + 1
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("allocate narrative revision: %w", err)
	}
	narrativeRevision := narrativeRevisionRecord{ID: uuid.New(), ProjectID: projectID, BreakdownRevisionID: &breakdown.ID, RevisionNo: narrativeRevisionNo, Status: "approved", ContentHash: analysis.SourceHash, Completeness: "complete"}
	if err := tx.Create(&narrativeRevision).Error; err != nil {
		return fmt.Errorf("materialize narrative revision: %w", err)
	}
	allScenes := make([]sceneRecord, 0)
	for _, episode := range analysis.Episodes {
		var contentUnit projectContentUnitRecord
		if err := tx.Where("project_id = ? AND ordinal = ?", projectID, episode.Number).Order("created_at DESC").First(&contentUnit).Error; err != nil {
			return fmt.Errorf("read materialized content unit %d: %w", episode.Number, err)
		}
		if err := tx.Create(&contentOrderItemRecord{ID: uuid.New(), OrderRevisionID: orderRevision.ID, ContentUnitID: contentUnit.ID, Ordinal: episode.Number}).Error; err != nil {
			return fmt.Errorf("materialize content order item %d: %w", episode.Number, err)
		}
		for sceneOrdinal, scene := range episode.Scenes {
			sceneRecord := sceneRecord{ID: uuid.New(), NarrativeRevisionID: narrativeRevision.ID, ContentUnitID: contentUnit.ID, Ordinal: sceneOrdinal + 1, Heading: scene.Heading, LocationHint: scene.Heading, StartOffset: scene.Anchor.StartOffset, EndOffset: max(scene.Anchor.EndOffset, scene.Anchor.StartOffset+1)}
			if err := tx.Create(&sceneRecord).Error; err != nil {
				return fmt.Errorf("materialize scene %s: %w", scene.Heading, err)
			}
			allScenes = append(allScenes, sceneRecord)
			if err := tx.Create(&beatRecord{ID: uuid.New(), SceneID: sceneRecord.ID, Ordinal: 1, Goal: scene.Heading}).Error; err != nil {
				return fmt.Errorf("materialize scene beat %s: %w", scene.Heading, err)
			}
		}
	}
	for _, asset := range allAnalysisAssets(analysis) {
		var entity canonicalEntityRecord
		entityErr := tx.Where("project_id = ? AND type = ? AND canonical_name = ?", projectID, asset.Kind, asset.Name).First(&entity).Error
		if errors.Is(entityErr, gorm.ErrRecordNotFound) {
			entity = canonicalEntityRecord{ID: uuid.New(), ProjectID: projectID, Type: asset.Kind, Status: "active", CanonicalName: asset.Name}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entity).Error; err != nil {
				return fmt.Errorf("materialize canonical entity %s: %w", asset.Name, err)
			}
			if err := tx.Where("project_id = ? AND type = ? AND canonical_name = ?", projectID, asset.Kind, asset.Name).First(&entity).Error; err != nil {
				return fmt.Errorf("read canonical entity %s: %w", asset.Name, err)
			}
		} else if entityErr != nil {
			return fmt.Errorf("read canonical entity %s: %w", asset.Name, entityErr)
		}
		stableKey := fmt.Sprintf("%s:%s", asset.Kind, strings.ToLower(asset.Name))
		var item requirementItemRecord
		itemErr := tx.Where("project_id = ? AND stable_key = ?", projectID, stableKey).First(&item).Error
		if errors.Is(itemErr, gorm.ErrRecordNotFound) {
			item = requirementItemRecord{ID: uuid.New(), ProjectID: projectID, StableKey: stableKey, Status: "active"}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error; err != nil {
				return fmt.Errorf("materialize requirement item %s: %w", asset.Name, err)
			}
			if err := tx.Where("project_id = ? AND stable_key = ?", projectID, stableKey).First(&item).Error; err != nil {
				return fmt.Errorf("read requirement item %s: %w", asset.Name, err)
			}
		} else if itemErr != nil {
			return fmt.Errorf("read requirement item %s: %w", asset.Name, itemErr)
		}
		var previousRequirement requirementRevisionRecord
		revisionNo := 1
		if err := tx.Where("item_id = ?", item.ID).Order("revision_no DESC").First(&previousRequirement).Error; err == nil {
			revisionNo = previousRequirement.RevisionNo + 1
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("allocate requirement revision %s: %w", asset.Name, err)
		}
		if err := tx.Model(&requirementRevisionRecord{}).Where("item_id = ? AND status = ?", item.ID, "current").Update("status", "superseded").Error; err != nil {
			return fmt.Errorf("supersede requirement revision %s: %w", asset.Name, err)
		}
		requirementRevision := requirementRevisionRecord{ID: uuid.New(), ItemID: item.ID, RevisionNo: revisionNo, Type: asset.Kind, Purpose: fmt.Sprintf("%s 的生产准备", asset.Name), Quantity: 1, Unit: "unit", Decision: "required", Status: "current"}
		if err := tx.Create(&requirementRevision).Error; err != nil {
			return fmt.Errorf("materialize requirement revision %s: %w", asset.Name, err)
		}
		for _, evidence := range asset.Evidence {
			var selectedScene *sceneRecord
			for index := range allScenes {
				candidate := &allScenes[index]
				if candidate.StartOffset <= evidence.StartOffset && candidate.EndOffset >= evidence.StartOffset {
					selectedScene = candidate
					break
				}
			}
			if selectedScene == nil {
				continue
			}
			mention := mentionRecord{ID: uuid.New(), NarrativeRevisionID: narrativeRevision.ID, SceneID: &selectedScene.ID, ElementType: asset.Kind, SurfaceText: asset.Name, Status: "active", StartOffset: evidence.StartOffset, EndOffset: max(evidence.EndOffset, evidence.StartOffset+1)}
			if err := tx.Create(&mention).Error; err != nil {
				return fmt.Errorf("materialize mention %s: %w", asset.Name, err)
			}
			entityID := entity.ID
			if err := tx.Create(&mentionResolutionRecord{ID: uuid.New(), MentionID: mention.ID, NarrativeRevisionID: narrativeRevision.ID, Action: "link", EntityID: &entityID, Reason: "deterministic source asset", Status: "current"}).Error; err != nil {
				return fmt.Errorf("materialize mention resolution %s: %w", asset.Name, err)
			}
		}
	}
	return nil
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
