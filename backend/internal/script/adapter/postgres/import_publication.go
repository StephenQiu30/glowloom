package postgres

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// NewImportSourceStore restricts every write to a permanently claimed import attempt.
// A client cannot create this authority through any public source command DTO.
func NewImportSourceStore(db *gorm.DB, access ProjectAccessFactory, authority application.ImportAuthority) *SourceStore {
	return &SourceStore{db: db, access: access, importAuthority: &authority}
}

func authorizeImportPublication(tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, a application.ImportAuthority) error {
	if a.JobID == uuid.Nil || a.WorkerID == uuid.Nil || a.PublicationKey == uuid.Nil || a.Attempt < 1 {
		return domain.ErrInvalidSource
	}
	var job scriptImportJobRow
	if err := tx.Where("id = ? AND org_id = ? AND project_id = ? AND actor_id = ?", a.JobID, actor.OrgID, project, actor.ID).Take(&job).Error; err != nil {
		return err
	}
	var state scriptImportStateRow
	if err := tx.Where("job_id = ?", a.JobID).Take(&state).Error; err != nil {
		return err
	}
	var attempt scriptImportAttemptRow
	if err := tx.Where("job_id = ? AND attempt = ? AND publication_key = ?", a.JobID, a.Attempt, a.PublicationKey).Take(&attempt).Error; err != nil {
		return err
	}
	if state.Attempt != a.Attempt || state.Status != domain.ImportStatusRunning || state.IOState != domain.ImportIOStateRunning || state.IOOwnerID == nil || *state.IOOwnerID != a.WorkerID || state.CancellationRequested || attempt.JobID != job.ID {
		return application.ErrConflict
	}
	return nil
}

func validateImportedPlan(tx *gorm.DB, p application.WritePlan, a application.ImportAuthority) error {
	if p.Command.Key != a.PublicationKey || p.Command.Action != domain.SourceActionImport || len(p.NewSources) < 1 {
		return domain.ErrInvalidSource
	}
	r, err := readImport(tx, identityapp.Principal{ID: p.ActorID, OrgID: p.OrgID}, p.Command.ProjectID, a.JobID, true)
	if err != nil {
		return err
	}
	if p.Command.ExpectedRevision != r.Job.LatestScriptRevision || !sameOptionalUUID(p.Command.BaseVersionID, r.BaseVersionID) {
		return application.ErrConflict
	}
	positions, err := importPositions(tx, a.ImportWork)
	if err != nil {
		return err
	}
	previous := -1
	for _, source := range p.NewSources {
		index := slices.IndexFunc(r.Files, func(f application.FrozenImportFile) bool { return f.SourceID == source.ID })
		if index <= previous || !slices.Contains(positions, index) {
			return domain.ErrInvalidSource
		}
		previous = index
		f := r.Files[index]
		result, found := findImportFileResult(r.Results, index)
		if !found || result.Status != domain.ImportFileStatusSucceeded || source.Origin != domain.SourceOriginFile || source.LineageID != f.SourceID || source.Revision != 1 || source.PreviousID != nil || source.MediaAssetID == nil || *source.MediaAssetID != f.Source.AssetID || source.MediaRevision == nil || *source.MediaRevision != f.Source.Revision || source.MediaSHA256 == nil || *source.MediaSHA256 != f.Source.SHA256 || source.Original.SHA256 != f.Source.SHA256 || source.Original.ByteSize != f.Source.ByteSize || source.Original.MIME != f.Source.MIME || source.Rich.SHA256 != result.RichSHA256 || source.ContentHash != result.ContentHash || source.CharCount != result.CharCount {
			return application.ErrObjectMismatch
		}
		provenance := domain.SourceProvenance{Encoding: result.Encoding, Mapping: result.Mapping, Warnings: result.Warnings}
		actual, _ := json.Marshal(source.Provenance)
		expected, _ := json.Marshal(provenance)
		if !slices.Equal(actual, expected) {
			return application.ErrObjectMismatch
		}
	}
	successes := 0
	for _, out := range r.Results {
		if out.Status == domain.ImportFileStatusSucceeded {
			successes++
		}
	}
	if successes != len(p.NewSources) {
		return application.ErrObjectMismatch
	}
	baseIDs := []uuid.UUID{}
	if r.BaseVersionID != nil {
		base, err := readVersion(tx, p.OrgID, p.Command.ProjectID, *r.BaseVersionID)
		if err != nil {
			return err
		}
		baseIDs = append(baseIDs, base.SourceIDs...)
	}
	for _, source := range p.NewSources {
		baseIDs = append(baseIDs, source.ID)
	}
	frozenIDs := make([]uuid.UUID, len(r.Files))
	for i, file := range r.Files {
		frozenIDs[i] = file.SourceID
	}
	expectedIDs, err := domain.OrderImportSources(baseIDs, frozenIDs)
	if err != nil {
		return err
	}
	if !slices.Equal(expectedIDs, p.Version.SourceIDs) {
		return application.ErrObjectMismatch
	}
	return nil
}

func findImportFileResult(results []application.ImportFileResult, position int) (application.ImportFileResult, bool) {
	for _, r := range results {
		if r.Position == position {
			return r, true
		}
	}
	return application.ImportFileResult{}, false
}

func finishImportedPublication(tx *gorm.DB, actor identityapp.Principal, p application.WritePlan, receipt application.SourceReceipt, a application.ImportAuthority) error {
	if err := validateImportedPlan(tx, p, a); err != nil {
		return err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	publication := scriptImportPublicationRow{JobID: a.JobID, Attempt: a.Attempt, Response: scriptJSON(data), CreatedAt: p.CreatedAt}
	if err := exactlyOne(tx.Create(&publication)); err != nil {
		return err
	}
	r, err := readImport(tx, actor, p.Command.ProjectID, a.JobID, true)
	if err != nil {
		return err
	}
	succeeded, failed := 0, 0
	for _, file := range r.Job.Files {
		if file.SourceID != nil {
			succeeded++
		} else {
			failed++
		}
	}
	j := r.Job
	j.Status = domain.ImportOutcome(succeeded, failed)
	j.Stage = domain.ImportStageCompleted
	j.LatestScriptRevision = receipt.ScriptRevision
	j.LatestVersionID = &receipt.VersionID
	j.NeedsReconciliation = false
	j.ReconciliationRequested = false
	j.FailureCode = ""
	j.Revision++
	j.UpdatedAt = time.Now().UTC()
	if err := updateImportState(tx, j, r.OwnerID, r.IOState); err != nil {
		return err
	}
	return importAudit(tx, actor, uuid.NewSHA1(a.PublicationKey, []byte("completion")), p.Command.RequestID, "completed", j, j.UpdatedAt)
}
