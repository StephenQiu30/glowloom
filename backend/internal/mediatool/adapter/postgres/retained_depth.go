package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

type retainedDepthIntent struct {
	JobID, AssetID               uuid.UUID
	Attempt                      int
	OutputSHA256, ArtifactSHA256 string
	Artifact                     []byte
}

type retainedDepthObject struct {
	jobID   uuid.UUID
	attempt int
	object  application.DepthObject
}

func readRetainedDepth(tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, facts *retainedMediaFacts) error {
	jobs := make(map[uuid.UUID]depthRow)
	if err := eachRetainedRow(tx, `SELECT id,project_id,attempt,created_at,profile_id,source_asset_id,source_asset_revision,source_sha256,frozen_sha256,CASE WHEN octet_length(frozen::text)<=1048576 THEN frozen ELSE NULL END AS frozen FROM mediatool.depth_job WHERE org_id=? AND project_id=? ORDER BY id LIMIT ?`, []any{actor.OrgID, project, retainedMediaLimit + 1}, func(row depthRow) error {
		var frozen domain.FrozenDepth
		if facts.body(row.Frozen, &frozen) != nil || row.ID == uuid.Nil || row.Attempt < 1 || row.Attempt > 100 || row.CreatedAt.IsZero() {
			return application.ErrUnavailable
		}
		if _, err := depthFrozen(row); err != nil || facts.source(frozen.Input) != nil {
			return application.ErrUnavailable
		}
		jobs[row.ID] = row
		return nil
	}); err != nil {
		return err
	}
	expected := make(map[string]retainedDepthObject)
	if err := eachRetainedRow(tx, `SELECT i.job_id,i.attempt,i.asset_id,i.output_sha256,i.artifact_sha256,CASE WHEN octet_length(i.artifact::text)<=1048576 THEN i.artifact ELSE NULL END AS artifact FROM mediatool.depth_result_intent i JOIN mediatool.depth_job j ON j.id=i.job_id WHERE j.org_id=? AND j.project_id=? ORDER BY i.job_id,i.attempt LIMIT ?`, []any{actor.OrgID, project, retainedMediaLimit + 1}, func(intent retainedDepthIntent) error {
		row, ok := jobs[intent.JobID]
		var artifact application.DepthArtifact
		if !ok || intent.Attempt < 1 || intent.Attempt > row.Attempt || facts.body(intent.Artifact, &artifact) != nil {
			return application.ErrUnavailable
		}
		frozen, err := depthFrozen(row)
		if err != nil {
			return err
		}
		job := row.job()
		job.Attempt = intent.Attempt
		digest, _, err := application.DepthArtifactDigest(job, frozen, artifact)
		if err != nil || digest != intent.ArtifactSHA256 || artifact.Asset.ID != intent.AssetID || artifact.Asset.SHA256 == nil || *artifact.Asset.SHA256 != intent.OutputSHA256 {
			return application.ErrUnavailable
		}
		facts.assets[intent.AssetID] = struct{}{}
		for _, object := range artifact.Objects {
			if facts.key(object.ObjectKey) != nil {
				return application.ErrUnavailable
			}
			if _, duplicate := expected[object.ObjectKey]; duplicate {
				return application.ErrUnavailable
			}
			expected[object.ObjectKey] = retainedDepthObject{jobID: intent.JobID, attempt: intent.Attempt, object: object}
		}
		return nil
	}); err != nil {
		return err
	}
	var budget struct{ Rows, Bytes int64 }
	if err := tx.Raw(`SELECT count(*) AS rows,COALESCE(sum(octet_length(o.object_key)+octet_length(o.mime_type)),0) AS bytes FROM mediatool.depth_object o JOIN mediatool.depth_job j ON j.id=o.job_id WHERE j.org_id=? AND j.project_id=?`, actor.OrgID, project).Scan(&budget).Error; err != nil {
		return err
	}
	if budget.Rows > retainedMediaLimit || budget.Bytes > 64<<20 {
		return application.ErrUnavailable
	}
	count, bytes := 0, 0
	if err := eachRetainedRow(tx, `SELECT o.job_id,o.attempt,CASE WHEN octet_length(o.kind)<=32 THEN o.kind ELSE NULL END AS kind,CASE WHEN octet_length(o.object_key)<=1024 THEN o.object_key ELSE NULL END AS object_key,o.sha256,o.byte_size,CASE WHEN octet_length(o.mime_type)<=256 THEN o.mime_type ELSE NULL END AS mime_type FROM mediatool.depth_object o JOIN mediatool.depth_job j ON j.id=o.job_id WHERE j.org_id=? AND j.project_id=? ORDER BY o.object_key LIMIT ?`, []any{actor.OrgID, project, retainedMediaLimit + 1}, func(object retainedDepthObjectRow) error {
		count++
		bytes += len(object.ObjectKey) + len(object.MIMEType)
		if count > retainedMediaLimit || bytes > 64<<20 {
			return application.ErrUnavailable
		}
		want, ok := expected[object.ObjectKey]
		row, ownJob := jobs[object.JobID]
		if !ok || !ownJob || object.JobID != want.jobID || object.Attempt != want.attempt || object.Attempt < 1 || object.Attempt > row.Attempt || object.Kind != want.object.Kind || object.SHA256 != want.object.SHA256 || object.ByteSize != want.object.ByteSize || object.MIMEType != want.object.MIMEType {
			return application.ErrUnavailable
		}
		delete(expected, object.ObjectKey)
		return nil
	}); err != nil {
		return err
	}
	if len(expected) != 0 {
		return application.ErrUnavailable
	}
	return nil
}

type retainedDepthObjectRow struct {
	JobID     uuid.UUID
	Attempt   int
	Kind      string
	ObjectKey string
	SHA256    string
	ByteSize  int64
	MIMEType  string
}
