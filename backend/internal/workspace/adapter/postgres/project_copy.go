package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectCopyOwnersFactory binds all copy owners to the same admission/fenced transaction.
type ProjectCopyOwnersFactory func(*gorm.DB) application.ProjectCopyOwners

// ProjectCopyStore owns one concrete project's admission, worker fence and publication.
type ProjectCopyStore struct {
	db     *gorm.DB
	work   ProjectWorkFactory
	owners ProjectCopyOwnersFactory
}

// NewProjectCopyStore injects persistence, work admission and the actual owner ports.
func NewProjectCopyStore(db *gorm.DB, work ProjectWorkFactory, owners ProjectCopyOwnersFactory) *ProjectCopyStore {
	return &ProjectCopyStore{db: db, work: work, owners: owners}
}

type copyWorkspaceSnapshot struct {
	SourceProjectID uuid.UUID
	SourceRevision  int64
	Target          domain.Project
	DefaultModels   map[string]string
	AIGCMarkStyle   json.RawMessage
	Presets         []domain.StylePreset
	Placement       *copyPlacementSnapshot `json:",omitempty"`
}

func copyJSON(body []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return domain.ErrInvalidProjectCopy
	}
	return nil
}

func copyFingerprint(input application.ProjectCopyInput) (string, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(append([]byte("project-copy/v1\x00"), body...))
	return hex.EncodeToString(hash[:]), nil
}

func copyMediaBinding(job domain.ProjectCopyJob) mediaapp.ProjectCopyBinding {
	return mediaapp.ProjectCopyBinding{JobID: job.ID, OrgID: job.OrgID, SourceProjectID: job.SourceProjectID, TargetProjectID: job.TargetProjectID}
}

func copyCanvasBinding(job domain.ProjectCopyJob) canvasapp.ProjectCopyBinding {
	return canvasapp.ProjectCopyBinding{JobID: job.ID, OrgID: job.OrgID, SourceProjectID: job.SourceProjectID, TargetProjectID: job.TargetProjectID}
}

func copyMediaSnapshot(job domain.ProjectCopyJob) mediaapp.ProjectCopySnapshot {
	return mediaapp.ProjectCopySnapshot{ID: job.Manifest.MediaSnapshotID, ManifestSHA256: job.Manifest.MediaSHA256, Assets: job.Manifest.Assets, Renditions: job.Manifest.Renditions}
}

func copyCanvasSnapshot(job domain.ProjectCopyJob) canvasapp.ProjectCopySnapshot {
	return canvasapp.ProjectCopySnapshot{ID: job.Manifest.CanvasSnapshotID, ManifestSHA256: job.Manifest.CanvasSHA256, Documents: job.Manifest.Documents}
}

func (s *ProjectCopyStore) transactionOwners(tx *gorm.DB) (application.ProjectCopyOwners, error) {
	if s.owners == nil {
		return application.ProjectCopyOwners{}, application.ErrProjectDependencyUnavailable
	}
	owners := s.owners(tx)
	if owners.Media == nil || owners.Canvas == nil || owners.Budget == nil {
		return application.ProjectCopyOwners{}, application.ErrProjectDependencyUnavailable
	}
	return owners, nil
}

func copyPresets(tx *gorm.DB, source domain.Project, job uuid.UUID, target uuid.UUID, assets map[uuid.UUID]uuid.UUID) ([]domain.StylePreset, uuid.UUID, error) {
	var rows []struct {
		ID, OrgID                                       uuid.UUID
		ProjectID                                       *uuid.UUID
		Name, StyleType, PromptFragment, NegativePrompt string
		StyleSubtype                                    *string
		ReferenceAssets                                 []byte
	}
	if err := tx.Raw(`SELECT id,org_id,project_id,name,style_type,style_subtype,prompt_fragment,negative_prompt,array_to_json(reference_asset_ids) AS reference_assets FROM workspace.style_preset WHERE org_id=? AND NOT is_delete AND (project_id=? OR id=?) ORDER BY id`, source.OrgID, source.ID, source.StylePresetID).Scan(&rows).Error; err != nil {
		return nil, uuid.Nil, fmt.Errorf("freeze copy style presets: %w", err)
	}
	presets := make([]domain.StylePreset, 0, len(rows))
	selected := uuid.Nil
	for _, row := range rows {
		if row.ProjectID != nil && *row.ProjectID != source.ID {
			return nil, uuid.Nil, application.ErrStylePresetNotFound
		}
		preset := domain.StylePreset{ID: uuid.NewSHA1(job, []byte("preset/"+row.ID.String())), OrgID: source.OrgID, ProjectID: target, Name: row.Name, StyleType: row.StyleType, PromptFragment: row.PromptFragment, NegativePrompt: row.NegativePrompt}
		if row.StyleSubtype != nil {
			preset.StyleSubtype = *row.StyleSubtype
		}
		if json.Unmarshal(row.ReferenceAssets, &preset.ReferenceAssetIDs) != nil || preset.ReferenceAssetIDs == nil {
			return nil, uuid.Nil, domain.ErrInvalidProjectCopy
		}
		for i, id := range preset.ReferenceAssetIDs {
			mapped, found := assets[id]
			if !found || mapped == uuid.Nil || mapped == id {
				return nil, uuid.Nil, mediaapp.ErrProjectCopyMediaUnavailable
			}
			preset.ReferenceAssetIDs[i] = mapped
		}
		if row.ID == source.StylePresetID {
			if preset.StyleType != source.StyleType || preset.StyleSubtype != source.StyleSubtype {
				return nil, uuid.Nil, application.ErrStylePresetMismatch
			}
			selected = preset.ID
		}
		presets = append(presets, preset)
	}
	if source.StylePresetID != uuid.Nil && selected == uuid.Nil {
		return nil, uuid.Nil, application.ErrStylePresetNotFound
	}
	return presets, selected, nil
}

func createCopyTarget(tx *gorm.DB, source domain.Project, target uuid.UUID, name string, now time.Time) (domain.Project, error) {
	p := source
	p.ID, p.Name, p.StylePresetID = target, name, uuid.Nil
	p.CoverAssetID = nil
	p.Status, p.ArchivedAt, p.Revision = "copying", nil, 1
	p.CreateTime, p.UpdateTime = now, now
	if p.Validate() != nil || p.IsDelete {
		return domain.Project{}, domain.ErrInvalidProjectCopy
	}
	var subtype any
	if p.StyleSubtype != "" {
		subtype = p.StyleSubtype
	}
	if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,name,description,aspect_ratio,style_type,style_subtype,resolution,allow_overseas_models,status,revision,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?,'copying',1,?,?)`, p.ID, p.OrgID, p.Name, p.Description, p.AspectRatio, p.StyleType, subtype, p.Resolution, p.AllowOverseasModels, now, now).Error; err != nil {
		return domain.Project{}, fmt.Errorf("create unpublished project target: %w", err)
	}
	return p, nil
}

func persistCopySettings(tx *gorm.DB, snapshot copyWorkspaceSnapshot) error {
	for _, preset := range snapshot.Presets {
		assets, err := json.Marshal(preset.ReferenceAssetIDs)
		if err != nil {
			return err
		}
		var subtype any
		if preset.StyleSubtype != "" {
			subtype = preset.StyleSubtype
		}
		if err := tx.Exec(`INSERT INTO workspace.style_preset(id,org_id,project_id,name,style_type,style_subtype,prompt_fragment,negative_prompt,reference_asset_ids) VALUES(?,?,?,?,?,?,?,?,ARRAY(SELECT jsonb_array_elements_text(?::jsonb)::uuid))`, preset.ID, preset.OrgID, preset.ProjectID, preset.Name, preset.StyleType, subtype, preset.PromptFragment, preset.NegativePrompt, string(assets)).Error; err != nil {
			return fmt.Errorf("copy private style preset: %w", err)
		}
	}
	defaults, err := json.Marshal(snapshot.DefaultModels)
	if err != nil {
		return err
	}
	var preset any
	if snapshot.Target.StylePresetID != uuid.Nil {
		preset = snapshot.Target.StylePresetID
	}
	write := tx.Exec(`UPDATE workspace.project SET style_preset_id=?,default_models=?::jsonb,aigc_mark_style=?::jsonb WHERE id=? AND org_id=? AND status='copying' AND NOT is_delete`, preset, string(defaults), string(snapshot.AIGCMarkStyle), snapshot.Target.ID, snapshot.Target.OrgID)
	if write.Error != nil {
		return fmt.Errorf("copy project settings: %w", write.Error)
	}
	if write.RowsAffected != 1 {
		return domain.ErrProjectCopyStateConflict
	}
	return nil
}

// Create atomically freezes every owner and records one invisible target with a zero budget.
func (s *ProjectCopyStore) Create(ctx context.Context, actor identityapp.Principal, input application.ProjectCopyInput, now time.Time) (domain.ProjectCopyJob, error) {
	if s == nil || s.db == nil || s.work == nil || input.Validate() != nil || now.IsZero() {
		return domain.ProjectCopyJob{}, application.ErrProjectDependencyUnavailable
	}
	fingerprint, err := copyFingerprint(input)
	if err != nil {
		return domain.ProjectCopyJob{}, err
	}
	now = now.UTC().Truncate(time.Microsecond)
	var saved domain.ProjectCopyJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := LockProjectLibrary(ctx, tx, actor); err != nil {
			return err
		}
		var locked int
		if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.ID.String()+":"+input.IdempotencyKey.String()).Scan(&locked).Error; err != nil {
			return err
		}
		var accepted struct {
			RequestSHA256     string
			AdmissionResponse []byte
		}
		replayed := tx.Raw(`SELECT request_sha256,admission_response FROM workspace.project_copy_job WHERE actor_id=? AND org_id=? AND idem_key=?`, actor.ID, actor.OrgID, input.IdempotencyKey).Scan(&accepted)
		if replayed.Error != nil {
			return replayed.Error
		}
		if replayed.RowsAffected == 1 {
			if accepted.RequestSHA256 != fingerprint || copyJSON(accepted.AdmissionResponse, &saved) != nil || saved.Validate() != nil {
				return application.ErrIdempotencyConflict
			}
			return nil
		}
		var existing int
		if err := tx.Raw(`SELECT 1 FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND NOT is_delete AND expires_at>statement_timestamp()`, actor.ID, input.IdempotencyKey.String()).Scan(&existing).Error; err != nil {
			return err
		}
		if existing == 1 {
			return application.ErrIdempotencyConflict
		}
		placement, err := FreezeCopyPlacement(ctx, tx, actor, input.SourceProjectID, input.Placement)
		if err != nil {
			return err
		}
		source, err := readLifecycleProject(tx, actor.OrgID, input.SourceProjectID, true)
		if err != nil {
			return err
		}
		if source.Project.IsDelete || (source.Project.Status != "active" && source.Project.Status != "archived") {
			return domain.ErrProjectStateConflict
		}
		if source.Project.Revision != input.ExpectedRevision {
			return domain.ErrProjectRevisionConflict
		}
		guard := s.work(tx)
		if guard == nil {
			return application.ErrProjectDependencyUnavailable
		}
		blocking, err := guard.HasInflightWork(ctx, actor, input.SourceProjectID)
		if err != nil {
			return err
		}
		if blocking {
			return domain.ErrProjectHasInflightOperations
		}
		owners, err := s.transactionOwners(tx)
		if err != nil {
			return err
		}
		saved = domain.ProjectCopyJob{RequestID: input.RequestID, ID: uuid.New(), OrgID: actor.OrgID, ActorID: actor.ID, SourceProjectID: input.SourceProjectID, SourceRevision: input.ExpectedRevision, TargetProjectID: uuid.New(), TargetName: input.TargetName, Status: "queued", Stage: "media", Revision: 1}
		target, err := createCopyTarget(tx, source.Project, saved.TargetProjectID, input.TargetName, now)
		if err != nil {
			return err
		}
		targetPlacement, err := AttachCopyPlacement(ctx, tx, actor, target.ID, placement, now)
		if err != nil {
			return err
		}
		media, err := owners.Media.Freeze(ctx, actor, copyMediaBinding(saved), now)
		if err != nil {
			return err
		}
		if source.Project.CoverAssetID != nil {
			if owners.Cover == nil {
				return application.ErrProjectDependencyUnavailable
			}
			if err := owners.Cover.Freeze(ctx, actor, source.Project.ID, source.Project.CoverAssetID); err != nil {
				return err
			}
		}
		target.CoverAssetID, err = application.MapProjectCover(source.Project.CoverAssetID, media.AssetMapping)
		if err != nil {
			return err
		}
		canvas, err := owners.Canvas.Freeze(ctx, actor, copyCanvasBinding(saved), media.AssetMapping)
		if err != nil {
			return err
		}
		var mark struct{ AIGCMarkStyle []byte }
		if err := tx.Raw(`SELECT aigc_mark_style FROM workspace.project WHERE id=? AND org_id=?`, source.Project.ID, actor.OrgID).Scan(&mark).Error; err != nil {
			return err
		}
		presets, selected, err := copyPresets(tx, source.Project, saved.ID, target.ID, media.AssetMapping)
		if err != nil {
			return err
		}
		target.StylePresetID = selected
		workspace := copyWorkspaceSnapshot{SourceProjectID: source.Project.ID, SourceRevision: source.Project.Revision, Target: target, DefaultModels: source.DefaultModels, AIGCMarkStyle: mark.AIGCMarkStyle, Presets: presets, Placement: &copyPlacementSnapshot{Source: placement.Placement, SourceFolderRevision: placement.FolderRevision, Target: targetPlacement}}
		digest, body, err := copyWorkspaceDigest(workspace)
		if err != nil {
			return err
		}
		saved.Manifest = domain.ProjectCopyManifest{WorkspaceSHA256: digest, MediaSnapshotID: media.ID, MediaSHA256: media.ManifestSHA256, Assets: media.Assets, Renditions: media.Renditions, CanvasSnapshotID: canvas.ID, CanvasSHA256: canvas.ManifestSHA256, Documents: canvas.Documents}
		if saved.Validate() != nil {
			return domain.ErrInvalidProjectCopy
		}
		if err := persistCopySettings(tx, workspace); err != nil {
			return err
		}
		if err := owners.Budget.CreateCopyTargetBudget(ctx, actor, target.ID); err != nil {
			return err
		}
		manifest, err := json.Marshal(saved.Manifest)
		if err != nil {
			return err
		}
		response, err := json.Marshal(saved)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response,request_id) VALUES(?,?,?,?,?,?,?,'queued','media',?::jsonb,?::jsonb,?,?,?::jsonb,?)`, saved.ID, saved.OrgID, saved.ActorID, saved.SourceProjectID, saved.SourceRevision, saved.TargetProjectID, saved.TargetName, string(manifest), string(body), input.IdempotencyKey, fingerprint, string(response), input.RequestID).Error; err != nil {
			return fmt.Errorf("admit complete frozen copy: %w", err)
		}
		if err := tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at) VALUES(?,?,?,?,202,?::jsonb,statement_timestamp()+interval '24 hours') ON CONFLICT(actor_id,idem_key) DO UPDATE SET request_hash=excluded.request_hash,status_code=202,response_body=excluded.response_body,expires_at=excluded.expires_at,is_delete=false,update_time=statement_timestamp()`, uuid.New(), actor.ID, input.IdempotencyKey.String(), fingerprint, string(response)).Error; err != nil {
			return err
		}
		if err := copyChanged(tx, saved, "requested", now); err != nil {
			return err
		}
		return copyAudit(tx, saved, actor, "project.copy_requested", input.RequestID, now)
	})
	return saved, err
}
