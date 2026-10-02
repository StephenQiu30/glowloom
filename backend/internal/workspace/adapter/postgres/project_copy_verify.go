package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func copyWorkspaceDigest(snapshot copyWorkspaceSnapshot) (string, []byte, error) {
	var mark any
	if json.Unmarshal(snapshot.AIGCMarkStyle, &mark) != nil {
		return "", nil, domain.ErrInvalidProjectCopy
	}
	canonical, err := json.Marshal(mark)
	if err != nil {
		return "", nil, err
	}
	snapshot.AIGCMarkStyle = canonical
	body, err := json.Marshal(snapshot)
	if err != nil || len(body) > 32<<20 {
		return "", nil, domain.ErrInvalidProjectCopy
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), body, nil
}
func readFrozenCopyWorkspace(tx *gorm.DB, job domain.ProjectCopyJob) (copyWorkspaceSnapshot, error) {
	var row struct{ WorkspaceSnapshot []byte }
	read := tx.Raw(`SELECT workspace_snapshot FROM workspace.project_copy_job WHERE id=? AND org_id=? FOR SHARE`, job.ID, job.OrgID).Scan(&row)
	if read.Error != nil {
		return copyWorkspaceSnapshot{}, read.Error
	}
	if read.RowsAffected != 1 {
		return copyWorkspaceSnapshot{}, domain.ErrInvalidProjectCopy
	}
	var frozen copyWorkspaceSnapshot
	if copyJSON(row.WorkspaceSnapshot, &frozen) != nil {
		return copyWorkspaceSnapshot{}, domain.ErrInvalidProjectCopy
	}
	digest, _, err := copyWorkspaceDigest(frozen)
	if err != nil || digest != job.Manifest.WorkspaceSHA256 || frozen.SourceProjectID != job.SourceProjectID || frozen.SourceRevision != job.SourceRevision || frozen.Target.ID != job.TargetProjectID || frozen.Target.OrgID != job.OrgID {
		return copyWorkspaceSnapshot{}, domain.ErrInvalidProjectCopy
	}
	return frozen, nil
}

func verifyCopyWorkspace(tx *gorm.DB, job domain.ProjectCopyJob) error {
	frozen, err := readFrozenCopyWorkspace(tx, job)
	if err != nil {
		return err
	}
	if frozen.Placement != nil {
		if err := verifyCopyPlacement(tx, job, *frozen.Placement); err != nil {
			return err
		}
	}
	var actual domain.Project
	read := tx.Raw(`SELECT id,org_id,name,description,aspect_ratio,style_type,COALESCE(style_subtype,'') AS style_subtype,COALESCE(style_preset_id,'00000000-0000-0000-0000-000000000000'::uuid) AS style_preset_id,cover_asset_id,resolution,allow_overseas_models,status,revision,is_delete FROM workspace.project WHERE id=? AND org_id=? FOR SHARE`, job.TargetProjectID, job.OrgID).Scan(&actual)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return domain.ErrInvalidProjectCopy
	}
	want := frozen.Target
	want.CreateTime, want.UpdateTime = actual.CreateTime, actual.UpdateTime
	if !reflect.DeepEqual(actual, want) {
		return domain.ErrInvalidProjectCopy
	}
	var settings struct{ DefaultModels, AIGCMarkStyle []byte }
	if err := tx.Raw(`SELECT default_models,aigc_mark_style FROM workspace.project WHERE id=?`, job.TargetProjectID).Scan(&settings).Error; err != nil {
		return err
	}
	var defaults map[string]string
	var gotMark, wantMark any
	if json.Unmarshal(settings.DefaultModels, &defaults) != nil || json.Unmarshal(settings.AIGCMarkStyle, &gotMark) != nil || json.Unmarshal(frozen.AIGCMarkStyle, &wantMark) != nil || !reflect.DeepEqual(defaults, frozen.DefaultModels) || !reflect.DeepEqual(gotMark, wantMark) {
		return domain.ErrInvalidProjectCopy
	}
	var presets []struct {
		ID, OrgID, ProjectID                                          uuid.UUID
		Name, StyleType, StyleSubtype, PromptFragment, NegativePrompt string
		ReferenceAssets                                               []byte
	}
	if err := tx.Raw(`SELECT id,org_id,project_id,name,style_type,COALESCE(style_subtype,'') AS style_subtype,prompt_fragment,negative_prompt,array_to_json(reference_asset_ids) AS reference_assets FROM workspace.style_preset WHERE project_id=? AND org_id=? AND NOT is_delete ORDER BY id`, job.TargetProjectID, job.OrgID).Scan(&presets).Error; err != nil {
		return err
	}
	if len(presets) != len(frozen.Presets) {
		return domain.ErrInvalidProjectCopy
	}
	expected := make(map[uuid.UUID]domain.StylePreset, len(presets))
	for _, preset := range frozen.Presets {
		expected[preset.ID] = preset
	}
	for _, row := range presets {
		value := domain.StylePreset{ID: row.ID, OrgID: row.OrgID, ProjectID: row.ProjectID, Name: row.Name, StyleType: row.StyleType, StyleSubtype: row.StyleSubtype, PromptFragment: row.PromptFragment, NegativePrompt: row.NegativePrompt}
		if json.Unmarshal(row.ReferenceAssets, &value.ReferenceAssetIDs) != nil || !reflect.DeepEqual(value, expected[row.ID]) {
			return domain.ErrInvalidProjectCopy
		}
	}
	return nil
}
