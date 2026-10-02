package postgres

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func readImport(tx *gorm.DB, actor identityapp.Principal, project, id uuid.UUID, lock bool) (application.ImportRecord, error) {
	var row struct {
		ID, ProjectID, OrgID, ActorID, PublicationKey                       uuid.UUID
		ActorRole, Status, Stage, FailureCode, IOState, Positions           string
		Revision, ExpectedScriptRevision, LatestScriptRevision              int64
		Attempt                                                             int
		LatestVersionID, BaseVersionID, IOOwnerID                           *uuid.UUID
		CancellationRequested, ReconciliationRequested, NeedsReconciliation bool
		CreatedAt, UpdatedAt                                                time.Time
	}
	query := `SELECT j.id,j.org_id,j.project_id,j.actor_id,j.actor_role,j.expected_script_revision,j.created_at,s.revision,s.attempt,s.status,s.stage,s.latest_script_revision,s.latest_version_id,s.cancellation_requested,s.reconciliation_requested,s.needs_reconciliation,s.failure_code,s.io_owner_id,s.io_state,s.updated_at,a.base_version_id,a.publication_key,to_json(a.positions)::text AS positions FROM script.import_job j JOIN script.import_state s ON s.job_id=j.id JOIN script.import_attempt a ON a.job_id=j.id AND a.attempt=s.attempt WHERE j.org_id=? AND j.project_id=? AND j.id=?`
	if lock {
		query += ` FOR UPDATE OF s`
	}
	read := tx.Raw(query, actor.OrgID, project, id).Scan(&row)
	if read.Error != nil {
		return application.ImportRecord{}, read.Error
	}
	if read.RowsAffected != 1 {
		return application.ImportRecord{}, application.ErrNotFound
	}
	r := application.ImportRecord{Actor: identityapp.Principal{ID: row.ActorID, OrgID: row.OrgID, Role: identitydomain.Role(row.ActorRole)}, BaseVersionID: row.BaseVersionID, PublicationKey: row.PublicationKey, OwnerID: row.IOOwnerID, IOState: row.IOState}
	j := application.ImportJob{ID: row.ID, ProjectID: row.ProjectID, Revision: row.Revision, Attempt: row.Attempt, Status: row.Status, Stage: row.Stage, ExpectedScriptRevision: row.ExpectedScriptRevision, LatestScriptRevision: row.LatestScriptRevision, LatestVersionID: row.LatestVersionID, CancellationRequested: row.CancellationRequested, ReconciliationRequested: row.ReconciliationRequested, NeedsReconciliation: row.NeedsReconciliation, ActiveIO: row.IOOwnerID != nil, FailureCode: row.FailureCode, Files: []application.ImportFile{}, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
	var positions []int
	if err := json.Unmarshal([]byte(row.Positions), &positions); err != nil {
		return r, application.ErrUnavailable
	}
	var files []struct {
		Position                     int
		SourceID, AssetID, ProjectID uuid.UUID
		MediaRevision, ByteSize      int64
		SHA256, MIME, FileName       string
	}
	if err := tx.Raw(`SELECT position,source_id,asset_id,project_id,media_revision,sha256,byte_size,mime,file_name FROM script.import_file WHERE job_id=? ORDER BY position`, id).Scan(&files).Error; err != nil {
		return r, err
	}
	if len(files) < 1 || len(files) > 200 {
		return r, application.ErrUnavailable
	}
	var results []struct {
		Attempt, Position int
		Result            string
	}
	if err := tx.Raw(`SELECT DISTINCT ON(position) attempt,position,result::text FROM script.import_file_result WHERE job_id=? ORDER BY position,attempt DESC`, id).Scan(&results).Error; err != nil {
		return r, err
	}
	latest := make(map[int]application.ImportFileResult, len(results))
	latestAttempts := make(map[int]int, len(results))
	for _, v := range results {
		var out application.ImportFileResult
		if err := json.Unmarshal([]byte(v.Result), &out); err != nil || out.Position != v.Position {
			return r, application.ErrUnavailable
		}
		latest[v.Position] = out
		latestAttempts[v.Position] = v.Attempt
		if v.Attempt == row.Attempt {
			r.Results = append(r.Results, out)
		}
	}
	var publications []struct{ Response string }
	if err := tx.Raw(`SELECT response::text FROM script.import_publication WHERE job_id=? ORDER BY attempt`, id).Scan(&publications).Error; err != nil {
		return r, err
	}
	published := make(map[uuid.UUID]bool)
	for _, p := range publications {
		var out application.SourceReceipt
		if err := json.Unmarshal([]byte(p.Response), &out); err != nil {
			return r, application.ErrUnavailable
		}
		for _, m := range out.Mappings {
			if m.NewSourceID != nil && *m.NewSourceID == m.LineageID {
				published[m.LineageID] = true
			}
		}
	}
	failures := 0
	for i, file := range files {
		if file.Position != i || file.ProjectID != project {
			return r, application.ErrUnavailable
		}
		r.Files = append(r.Files, application.FrozenImportFile{Position: i, SourceID: file.SourceID, Source: mediaapp.DocumentSource{AssetID: file.AssetID, ProjectID: file.ProjectID, Revision: file.MediaRevision, SHA256: file.SHA256, ByteSize: file.ByteSize, MIME: file.MIME, FileName: file.FileName}})
		f := application.ImportFile{Position: i, AssetID: file.AssetID, FileName: file.FileName, Status: "queued", Warnings: []application.ExtractionWarning{}, Attempt: row.Attempt}
		if out, ok := latest[i]; ok {
			f.Status = out.Status
			f.FailureCode = out.FailureCode
			f.Warnings = out.Warnings
			f.Attempt = latestAttempts[i]
		}
		if published[file.SourceID] {
			f.Status = "succeeded"
			sid := file.SourceID
			f.SourceID = &sid
			f.LineageID = &sid
		} else if slices.Contains(positions, i) && latestAttempts[i] != row.Attempt && (j.Status == "queued" || j.Status == "running") {
			f.Status = "queued"
			f.FailureCode = ""
			f.Attempt = row.Attempt
		}
		if f.Status == "failed" {
			failures++
		}
		j.Files = append(j.Files, f)
	}
	j.Retryable = actor.ID == row.ActorID && domain.CanRetryImport(j.Status, j.ActiveIO, row.IOState == "unknown", j.NeedsReconciliation, failures)
	j.CanControl = j.Status != "succeeded" && j.Status != "cancelled" && (actor.ID == row.ActorID || actor.Role == identitydomain.RoleAdmin)
	r.Job = j
	return r, nil
}

func updateImportState(tx *gorm.DB, j application.ImportJob, owner *uuid.UUID, ioState string) error {
	return exactlyOne(tx.Exec(`UPDATE script.import_state SET revision=?,attempt=?,status=?,stage=?,latest_script_revision=?,latest_version_id=?,cancellation_requested=?,reconciliation_requested=?,needs_reconciliation=?,failure_code=?,io_owner_id=?,io_state=?,updated_at=? WHERE job_id=? AND revision=?`, j.Revision, j.Attempt, j.Status, j.Stage, j.LatestScriptRevision, j.LatestVersionID, j.CancellationRequested, j.ReconciliationRequested, j.NeedsReconciliation, j.FailureCode, owner, ioState, j.UpdatedAt, j.ID, j.Revision-1))
}

// ImportCommandTopic is the sole new command topic produced by document imports.
const ImportCommandTopic = "lanverse.script.import_command.v1"

func recordImportCommand(tx *gorm.DB, actor identityapp.Principal, key, request uuid.UUID, action, hash string, revision int64, j application.ImportJob, emit string, now time.Time) error {
	id := uuid.NewSHA1(key, []byte("script-file-import/"+actor.ID.String()+"/"+emit))
	delivery := application.ImportDelivery{ImportWork: application.ImportWork{JobID: j.ID, Attempt: j.Attempt}, EventID: id, RequestID: key, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: j.ProjectID, Action: emit}
	payload, err := json.Marshal(struct {
		EventID    uuid.UUID                  `json:"event_id"`
		EventType  string                     `json:"event_type"`
		OccurredAt time.Time                  `json:"occurred_at"`
		OrgID      uuid.UUID                  `json:"org_id"`
		Data       application.ImportDelivery `json:"data"`
	}{id, ImportCommandTopic, now, actor.OrgID, delivery})
	if err != nil {
		return err
	}
	response, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if err := exactlyOne(tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, ImportCommandTopic, j.ProjectID.String(), string(payload))); err != nil {
		return err
	}
	if err := exactlyOne(tx.Exec(`INSERT INTO script.import_command(actor_id,request_id,org_id,project_id,job_id,action,request_hash,expected_revision,event_id,event_action,event_attempt,response,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?::jsonb,?)`, actor.ID, key, actor.OrgID, j.ProjectID, j.ID, action, hash, revision, id, emit, j.Attempt, string(response), now)); err != nil {
		return err
	}
	return importAudit(tx, actor, key, request, action, j, now)
}

func importAudit(tx *gorm.DB, actor identityapp.Principal, key, request uuid.UUID, action string, j application.ImportJob, now time.Time) error {
	const topic = "lanverse.audit.recorded.v1"
	id := uuid.NewSHA1(key, []byte("script-file-import-audit/"+actor.ID.String()+"/"+action))
	summary := map[string]any{"id": j.ID, "revision": j.Revision, "attempt": j.Attempt, "status": j.Status, "stage": j.Stage, "file_count": len(j.Files), "script_revision": j.LatestScriptRevision}
	payload, err := json.Marshal(map[string]any{"event_id": id, "event_type": topic, "occurred_at": now, "org_id": actor.OrgID, "project_id": j.ProjectID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": id}, "data": map[string]any{"action": "script.file_import_" + action, "object": map[string]any{"type": "script_import", "id": j.ID.String()}, "before": nil, "after": summary, "request_id": request.String()}})
	if err != nil {
		return err
	}
	return exactlyOne(tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, topic, j.ProjectID.String(), string(payload)))
}

func rejectPendingSourceExcept(tx *gorm.DB, org, project, key uuid.UUID) error {
	var found bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.command c JOIN script.command_state s ON s.actor_id=c.actor_id AND s.request_id=c.request_id WHERE c.org_id=? AND c.project_id=? AND c.request_id<>? AND s.status='pending')`, org, project, key).Scan(&found).Error; err != nil {
		return err
	}
	if found {
		return application.ErrNeedsReconciliation
	}
	return nil
}

func importPositions(tx *gorm.DB, work application.ImportWork) ([]int, error) {
	var row struct{ Positions string }
	read := tx.Raw(`SELECT to_json(positions)::text AS positions FROM script.import_attempt WHERE job_id=? AND attempt=?`, work.JobID, work.Attempt).Scan(&row)
	if read.Error != nil {
		return nil, read.Error
	}
	if read.RowsAffected != 1 {
		return nil, application.ErrNotFound
	}
	var result []int
	if err := json.Unmarshal([]byte(row.Positions), &result); err != nil {
		return nil, fmt.Errorf("decode frozen import positions: %w", err)
	}
	return result, nil
}
