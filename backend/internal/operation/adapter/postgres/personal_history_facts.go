package postgres

import (
	"strconv"
	"strings"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// A left join preserves pending/failed records with no candidates. Deleted
// candidates are excluded separately so deleting the final result does not
// accidentally recreate a synthetic empty card.
const personalHistoryCandidates = `, history_outputs AS (
 SELECT candidate_op.id AS operation_id,candidate_op.capability,output.id,output.seq_no AS sequence,output.kind,
 CASE WHEN output.moderation_status='passed' AND asset.status='ready' AND asset.moderation_status='passed' AND NOT asset.is_delete THEN output.media_asset_id END AS media_asset_id,
 output.moderation_status,output.create_time,asset.width,asset.height,asset.duration_ms,
 COALESCE(meta.favorite,false) AS favorite,COALESCE(meta.revision,0) AS metadata_revision,
 CASE WHEN upper(candidate_op.params->>'resolution') IN ('1K','2K','4K','8K','720P','1080P') THEN upper(candidate_op.params->>'resolution')
 WHEN asset.kind='video' AND least(asset.width,asset.height) IN (720,1080) THEN least(asset.width,asset.height)::text||'P'
 WHEN asset.kind='image' THEN CASE greatest(asset.width,asset.height) WHEN 1024 THEN '1K' WHEN 2048 THEN '2K' WHEN 4096 THEN '4K' WHEN 8192 THEN '8K' ELSE '' END ELSE '' END AS resolution,
 CASE WHEN asset.width>0 AND asset.height>0 THEN (asset.width/gcd(asset.width,asset.height))::text||':'||(asset.height/gcd(asset.width,asset.height))::text
 ELSE COALESCE(candidate_op.params->>'aspect_ratio',candidate_op.params->>'ratio','') END AS ratio,
 CASE candidate_op.mode WHEN 'omni_reference' THEN 'all_reference' WHEN 'image2video' THEN 'first_last_frame'
 WHEN 'all_reference' THEN 'all_reference' WHEN 'first_last_frame' THEN 'first_last_frame' WHEN 'multi_frame' THEN 'multi_frame'
 WHEN 'lip_sync' THEN 'lip_sync' WHEN 'motion_mimic' THEN 'motion_mimic' WHEN 'song' THEN 'song' WHEN 'instrumental' THEN 'instrumental' WHEN 'tts' THEN 'tts' ELSE '' END AS generation_mode,
 (candidate_op.capability IN ('image.upscale','video.upscale','image.super_resolution','video.super_resolution') OR candidate_op.mode='super_resolution') AS super_resolution,
 (candidate_op.capability='video.frame_interpolation' OR candidate_op.mode='frame_interpolation') AS frame_interpolation,
 (candidate_project.status='active' AND candidate_op.status='completed' AND output.moderation_status='passed' AND asset.status='ready' AND asset.moderation_status='passed' AND NOT asset.is_delete AND asset.kind IN ('image','video') AND candidate_op.capability<>'video.dubbing' AND candidate_op.mode NOT IN ('video_dubbing','dubbing')) AS can_publish,
 CASE WHEN output.kind='json' AND output.moderation_status='passed' AND candidate_op.status='completed' AND (candidate_op.capability LIKE 'text.%' OR candidate_op.capability LIKE 'document.%') AND jsonb_typeof(output.json_payload->'text')='string' AND octet_length(output.json_payload->>'text')<=65536 THEN output.json_payload->>'text' END AS text,
 (candidate_op.status='completed' AND output.moderation_status='passed' AND ((asset.status='ready' AND asset.moderation_status='passed' AND NOT asset.is_delete) OR (output.kind='json' AND (candidate_op.capability LIKE 'text.%' OR candidate_op.capability LIKE 'document.%') AND jsonb_typeof(output.json_payload->'text')='string' AND octet_length(output.json_payload->>'text') BETWEEN 1 AND 65536))) AS can_download
 FROM operation.operation candidate_op
 JOIN workspace.project candidate_project ON candidate_project.id=candidate_op.project_id
 LEFT JOIN operation.operation_output output ON output.operation_id=candidate_op.id AND output.project_id=candidate_op.project_id AND NOT output.is_delete
 LEFT JOIN media.media_asset asset ON asset.id=output.media_asset_id AND asset.project_id=candidate_op.project_id
 LEFT JOIN operation.personal_history_output meta ON meta.operation_id=candidate_op.id AND meta.output_id=output.id AND meta.actor_id=? AND meta.org_id=? AND meta.scope_id=?
 WHERE meta.deleted_at IS NULL
)`

func personalHistoryCTE(a identity.Principal) (string, []any) {
	return personalOperationIDs + personalHistoryCandidates, []any{a.ID, a.OrgID, a.ID, a.OrgID, a.ID, a.OrgID, a.ID, a.OrgID, a.ScopeID}
}

// Every selected field is compared before keyset pagination. The same predicate
// clips non-video candidates. Video selection is record-level and its output
// projection deliberately preserves every non-hidden sibling of a matched task.
func personalOutputFilter(in application.PersonalHistoryInput, alias string) (string, []any) {
	parts := []string{}
	args := []any{}
	if in.FavoriteOnly {
		parts = append(parts, alias+`.favorite`)
	}
	for _, f := range []struct {
		name   string
		values []string
	}{{"resolution", in.Resolutions}, {"generation_mode", in.GenerationModes}} {
		if len(f.values) > 0 {
			parts = append(parts, alias+"."+f.name+" IN ?")
			args = append(args, f.values)
		}
	}
	if len(in.Ratios) > 0 {
		ratios := []string{}
		for _, ratio := range in.Ratios {
			terms := strings.Split(ratio, ":")
			numerator, _ := strconv.Atoi(terms[0])
			denominator, _ := strconv.Atoi(terms[1])
			ratios = append(ratios, `CASE WHEN `+alias+`.ratio ~ '^[1-9][0-9]{0,8}:[1-9][0-9]{0,8}$' THEN split_part(`+alias+`.ratio,':',1)::numeric*?=split_part(`+alias+`.ratio,':',2)::numeric*? ELSE false END`)
			args = append(args, denominator, numerator)
		}
		parts = append(parts, "("+strings.Join(ratios, " OR ")+")")
	}
	if len(in.ProcessTypes) > 0 {
		processes := []string{}
		for _, p := range in.ProcessTypes {
			switch p {
			case "super_resolution":
				processes = append(processes, alias+".super_resolution")
			case "frame_interpolation":
				processes = append(processes, alias+".frame_interpolation")
			}
		}
		parts = append(parts, "("+strings.Join(processes, " OR ")+")")
	}
	if len(parts) == 0 {
		return "", args
	}
	return " AND " + strings.Join(parts, " AND "), args
}
