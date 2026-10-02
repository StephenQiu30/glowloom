package postgres

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func decodeProjectChangeReceipt(raw []byte, actor identityapp.Principal, input application.ProjectChangeInput, saved *application.ProjectSnapshot) error {
	if !utf8.Valid(raw) {
		return application.ErrProjectDependencyUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(saved); err != nil {
		return fmt.Errorf("decode project change receipt: %w", err)
	}
	if d.Decode(new(any)) != io.EOF || saved.CoverUnavailable || saved.Project.ID != input.Patch.ProjectID || saved.Project.OrgID != actor.OrgID || saved.Validate() != nil {
		return application.ErrProjectDependencyUnavailable
	}
	p := saved.Project
	if p.Status != "active" && p.Status != "archived" || p.Revision < input.Patch.ExpectedRevision || p.Revision > input.Patch.ExpectedRevision+1 {
		return application.ErrProjectDependencyUnavailable
	}
	if input.Action != "patch" && p.Revision != input.Patch.ExpectedRevision+1 {
		return application.ErrProjectDependencyUnavailable
	}
	switch input.Action {
	case "patch":
		if p.Status != "active" || p.IsDelete ||
			(input.Patch.Name != nil && p.Name != strings.TrimSpace(*input.Patch.Name)) ||
			(input.Patch.Description != nil && p.Description != *input.Patch.Description) ||
			(input.Patch.StylePresetID != nil && p.StylePresetID != *input.Patch.StylePresetID) ||
			(input.Patch.AllowOverseasModels != nil && p.AllowOverseasModels != *input.Patch.AllowOverseasModels) ||
			(input.Patch.SetCover && !application.SameProjectCover(input.Patch.CoverAssetID, p.CoverAssetID)) {
			return application.ErrProjectDependencyUnavailable
		}
	case "archive":
		if p.Status != "archived" || p.IsDelete {
			return application.ErrProjectDependencyUnavailable
		}
	case "unarchive":
		if p.Status != "active" || p.IsDelete {
			return application.ErrProjectDependencyUnavailable
		}
	case "delete":
		if !p.IsDelete {
			return application.ErrProjectDependencyUnavailable
		}
	case "restore":
		if p.IsDelete {
			return application.ErrProjectDependencyUnavailable
		}
	}
	return nil
}

func replayProjectChange(tx *gorm.DB, actor identityapp.Principal, input application.ProjectChangeInput, fingerprint string, saved *application.ProjectSnapshot) (bool, error) {
	var row struct {
		OrgID, ProjectID      uuid.UUID
		Action, RequestSHA256 string
		StatusCode            int
		ResponseBody          []byte
	}
	read := tx.Raw(`SELECT org_id,project_id,action,request_sha256,status_code,response_body FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?`, actor.ID, input.IdempotencyKey).Scan(&row)
	if read.Error != nil {
		return false, fmt.Errorf("read permanent project change receipt: %w", read.Error)
	}
	if read.RowsAffected != 0 {
		if row.OrgID != actor.OrgID || row.ProjectID != input.Patch.ProjectID || row.Action != input.Action || row.RequestSHA256 != fingerprint || row.StatusCode != 200 {
			return false, application.ErrIdempotencyConflict
		}
		return true, decodeProjectChangeReceipt(row.ResponseBody, actor, input, saved)
	}
	// Only this exact actor/key is examined. The old payload is promoted without
	// re-encoding or changing the shared TTL record, including retained expired rows.
	var legacy struct {
		RequestHash  string
		StatusCode   int
		ResponseBody []byte
	}
	read = tx.Raw(`SELECT request_hash,status_code,response_body FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND NOT is_delete`, actor.ID, input.IdempotencyKey.String()).Scan(&legacy)
	if read.Error != nil {
		return false, fmt.Errorf("read legacy project change receipt: %w", read.Error)
	}
	if read.RowsAffected == 0 {
		return false, nil
	}
	if legacy.RequestHash != fingerprint || legacy.StatusCode != 200 {
		return false, application.ErrIdempotencyConflict
	}
	if err := decodeProjectChangeReceipt(legacy.ResponseBody, actor, input, saved); err != nil {
		return false, err
	}
	if err := insertProjectChangeReceipt(tx, actor, input, fingerprint, legacy.ResponseBody); err != nil {
		return false, err
	}
	return true, nil
}

func recordProjectChange(tx *gorm.DB, actor identityapp.Principal, input application.ProjectChangeInput, fingerprint string, saved application.ProjectSnapshot) error {
	saved.CoverUnavailable = false
	body, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("encode permanent project change receipt: %w", err)
	}
	return insertProjectChangeReceipt(tx, actor, input, fingerprint, body)
}

func insertProjectChangeReceipt(tx *gorm.DB, actor identityapp.Principal, input application.ProjectChangeInput, fingerprint string, raw []byte) error {
	result := tx.Exec(`INSERT INTO workspace.project_change_command(id,org_id,project_id,actor_id,idem_key,action,request_sha256,status_code,response_body)VALUES(?,?,?,?,?,?,?,200,?::jsonb)`, uuid.New(), actor.OrgID, input.Patch.ProjectID, actor.ID, input.IdempotencyKey, input.Action, fingerprint, string(raw))
	if result.Error != nil {
		return fmt.Errorf("persist permanent project change receipt: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrProjectDependencyUnavailable
	}
	return nil
}
