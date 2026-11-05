package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type retainedDepthIntent struct {
	JobID, AssetID               uuid.UUID
	Attempt                      int
	OutputSHA256, ArtifactSHA256 string
	Artifact                     jsonDocument
}

type retainedDepthObject struct {
	jobID   uuid.UUID
	attempt int
	object  application.DepthObject
}

func readRetainedDepth(tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, facts *retainedMediaFacts) error {
	jobs := make(map[uuid.UUID]depthRow)
	if err := eachRetainedRow(tx, tx.Model(&depthRow{}).
		Select("id, project_id, attempt, created_at, profile_id, source_asset_id, source_asset_revision, source_sha256, frozen_sha256, CASE WHEN octet_length(frozen::text) <= ? THEN frozen ELSE NULL END AS frozen", 1<<20).
		Where("org_id = ? AND project_id = ?", actor.OrgID, project).Order("id").Limit(retainedMediaLimit+1), func(row depthRow) error {
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
	if err := eachRetainedRow(tx, tx.Model(&depthResultIntentRecord{}).
		Select("mediatool.depth_result_intent.job_id, mediatool.depth_result_intent.attempt, mediatool.depth_result_intent.asset_id, mediatool.depth_result_intent.output_sha256, mediatool.depth_result_intent.artifact_sha256, CASE WHEN octet_length(mediatool.depth_result_intent.artifact::text) <= ? THEN mediatool.depth_result_intent.artifact ELSE NULL END AS artifact", 1<<20).
		Joins("JOIN mediatool.depth_job ON mediatool.depth_job.id = mediatool.depth_result_intent.job_id").
		Where("mediatool.depth_job.org_id = ? AND mediatool.depth_job.project_id = ?", actor.OrgID, project).
		Order("mediatool.depth_result_intent.job_id").Order("mediatool.depth_result_intent.attempt").Limit(retainedMediaLimit+1), func(intent retainedDepthIntent) error {
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
	if err := tx.Model(&depthObjectRecord{}).
		Select("count(*) AS rows, COALESCE(sum(octet_length(mediatool.depth_object.object_key) + octet_length(mediatool.depth_object.mime_type)), 0) AS bytes").
		Joins("JOIN mediatool.depth_job ON mediatool.depth_job.id = mediatool.depth_object.job_id").
		Where("mediatool.depth_job.org_id = ? AND mediatool.depth_job.project_id = ?", actor.OrgID, project).
		Scan(&budget).Error; err != nil {
		return err
	}
	if budget.Rows > retainedMediaLimit || budget.Bytes > 64<<20 {
		return application.ErrUnavailable
	}
	count, bytes := 0, 0
	if err := eachRetainedRow(tx, tx.Model(&depthObjectRecord{}).
		Select("mediatool.depth_object.job_id, mediatool.depth_object.attempt, CASE WHEN octet_length(mediatool.depth_object.kind) <= 32 THEN mediatool.depth_object.kind ELSE NULL END AS kind, CASE WHEN octet_length(mediatool.depth_object.object_key) <= 1024 THEN mediatool.depth_object.object_key ELSE NULL END AS object_key, mediatool.depth_object.sha256, mediatool.depth_object.byte_size, CASE WHEN octet_length(mediatool.depth_object.mime_type) <= 256 THEN mediatool.depth_object.mime_type ELSE NULL END AS mime_type").
		Joins("JOIN mediatool.depth_job ON mediatool.depth_job.id = mediatool.depth_object.job_id").
		Where("mediatool.depth_job.org_id = ? AND mediatool.depth_job.project_id = ?", actor.OrgID, project).
		Order("mediatool.depth_object.object_key").Limit(retainedMediaLimit+1), func(object retainedDepthObjectRow) error {
		count++
		bytes += len(object.ObjectKey) + len(object.MIMEType)
		if count > retainedMediaLimit || bytes > 64<<20 {
			return application.ErrUnavailable
		}
		want, ok := expected[object.ObjectKey]
		row, ownJob := jobs[object.JobID]
		if !ok || !ownJob || object.JobID != want.jobID || object.Attempt != want.attempt || object.Attempt < 1 || object.Attempt > row.Attempt || object.Kind != string(want.object.Kind) || object.SHA256 != want.object.SHA256 || object.ByteSize != want.object.ByteSize || object.MIMEType != want.object.MIMEType {
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
