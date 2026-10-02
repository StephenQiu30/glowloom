package postgres

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
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
	var valid bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.import_job j JOIN script.import_state s ON s.job_id=j.id JOIN script.import_attempt a ON a.job_id=j.id AND a.attempt=s.attempt WHERE j.id=? AND j.org_id=? AND j.project_id=? AND j.actor_id=? AND s.attempt=? AND a.publication_key=? AND s.status='running' AND s.io_state='running' AND s.io_owner_id=? AND NOT s.cancellation_requested)`, a.JobID, actor.OrgID, project, actor.ID, a.Attempt, a.PublicationKey, a.WorkerID).Scan(&valid).Error; err != nil {
		return err
	}
	if !valid {
		return application.ErrConflict
	}
	return nil
}

func validateImportedPlan(tx *gorm.DB, p application.WritePlan, a application.ImportAuthority) error {
	if p.Command.Key != a.PublicationKey || p.Command.Action != "import" || len(p.NewSources) < 1 {
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
		if !found || result.Status != "succeeded" || source.Origin != "file" || source.LineageID != f.SourceID || source.Revision != 1 || source.PreviousID != nil || source.MediaAssetID == nil || *source.MediaAssetID != f.Source.AssetID || source.MediaRevision == nil || *source.MediaRevision != f.Source.Revision || source.MediaSHA256 == nil || *source.MediaSHA256 != f.Source.SHA256 || source.Original.SHA256 != f.Source.SHA256 || source.Original.ByteSize != f.Source.ByteSize || source.Original.MIME != f.Source.MIME || source.Rich.SHA256 != result.RichSHA256 || source.ContentHash != result.ContentHash || source.CharCount != result.CharCount {
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
		if out.Status == "succeeded" {
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
	if err := exactlyOne(tx.Exec(`INSERT INTO script.import_publication(job_id,attempt,response,created_at) VALUES(?,?,?::jsonb,?)`, a.JobID, a.Attempt, string(data), p.CreatedAt)); err != nil {
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
	j.Stage = "completed"
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
