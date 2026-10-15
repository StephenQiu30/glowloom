package identity_test

import (
	"context"
	"errors"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	md "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	td "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type reviewScopeSource struct{ asset uuid.UUID }

func (s reviewScopeSource) FreezeTranscriptionSource(context.Context, identity.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (uuid.UUID, error) {
	return s.asset, nil
}

type reviewScopeTimeline struct{ value canvas.TimelineConfig }

func (s reviewScopeTimeline) FreezeTimeline(context.Context, identity.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (canvas.TimelineConfig, error) {
	return s.value, nil
}

func reviewVideo(t *testing.T, tx *gorm.DB, project uuid.UUID) md.MediaAsset {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	sha := strings.Repeat("a", 64)
	w, h, d := int32(64), int32(36), int32(1000)
	a := md.MediaAsset{ID: uuid.New(), ProjectID: project, Kind: md.KindVideo, Origin: md.OriginUpload, Status: md.StatusReady, MimeType: "video/mp4", ByteSize: 6, SHA256: &sha, Width: &w, Height: &h, DurationMS: &d, ModerationStatus: md.ModerationPassed, Revision: 1, CreateTime: now, UpdateTime: now}
	a.ObjectKey = "projects/" + project.String() + "/video/2026/10/" + a.ID.String() + ".mp4"
	if err := tx.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,sha256,width,height,duration_ms,moderation_status) VALUES(?,?,'video','upload','ready',?,'video/mp4',6,?,64,36,1000,'passed')`, a.ID, project, a.ObjectKey, sha).Error; err != nil {
		t.Fatal(err)
	}
	return a
}
func reviewTimeline(asset uuid.UUID) canvas.TimelineConfig {
	track := uuid.New()
	return canvas.TimelineConfig{Version: 1, AspectRatio: "16:9", FPS: 25, Tracks: []canvas.TimelineTrack{{ID: track, Kind: "video", Label: "Source", Visible: true}}, Clips: []canvas.TimelineClip{{ID: uuid.New(), TrackID: track, Kind: "video", AssetID: &asset, Title: "Source", DurationMS: 1000, Volume: 1}}, SubtitleStyle: canvas.SubtitleStyle{FontSize: 24, Color: "#ffffff", Position: "bottom"}}
}
func TestMediaRequestControlPreservesSessionPrincipal(t *testing.T) {
	for _, kind := range []string{"export_retry", "transcription_retry", "depth_retry", "depth_review"} {
		t.Run(kind, func(t *testing.T) {
			tx, actor, _, scope, project, factory := projectConsumerFixture(t)
			installMediaScopeMigration(t, tx)
			installMediaReviewStorageColumns(t, tx)
			personalScope := actor.ScopeID
			actor.ScopeID = scope
			asset := reviewVideo(t, tx, project)
			scopes := func(tx *gorm.DB) media.ScopeAccess { return mediaScopeTestAccess{tx: tx, request: factory(tx)} }
			mediaFactory := func(tx *gorm.DB) tool.DerivedMedia {
				return media.NewDerivedService(pgmedia.NewScopedStore(tx, scopes))
			}
			source := func(*gorm.DB) tool.TranscriptionSourceReader { return reviewScopeSource{asset.ID} }
			var action func(identity.Principal) error
			switch kind {
			case "export_retry":
				store := pgtool.NewStore(tx, func(*gorm.DB) tool.TimelineReader { return reviewScopeTimeline{reviewTimeline(asset.ID)} }, mediaFactory)
				job, err := store.Create(t.Context(), actor, project, uuid.New(), tool.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
				if err != nil {
					t.Fatal(err)
				}
				if err = tx.Exec(`UPDATE mediatool.export_job SET status='failed',stage='failed' WHERE id=?`, job.ID).Error; err != nil {
					t.Fatal(err)
				}
				action = func(a identity.Principal) error {
					_, e := store.Control(t.Context(), a, project, job.ID, uuid.New(), job.Revision, "retry")
					return e
				}
			case "transcription_retry":
				store := pgtool.NewTranscriptionStore(tx, source, mediaFactory)
				job, err := store.Create(t.Context(), actor, project, uuid.New(), tool.TranscriptionCreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1, Language: "auto"})
				if err != nil {
					t.Fatal(err)
				}
				if err = tx.Exec(`UPDATE mediatool.transcription_job SET status='failed',stage='failed' WHERE id=?`, job.ID).Error; err != nil {
					t.Fatal(err)
				}
				action = func(a identity.Principal) error {
					_, e := store.Control(t.Context(), a, project, job.ID, uuid.New(), job.Revision, "retry")
					return e
				}
			default:
				store := pgtool.NewDepthStore(tx, source, func(tx *gorm.DB) tool.DepthDerivedMedia {
					return media.NewDerivedService(pgmedia.NewScopedStore(tx, scopes))
				})
				job, err := store.Create(t.Context(), actor, project, uuid.New(), tool.DepthCreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
				if err != nil {
					t.Fatal(err)
				}
				if kind == "depth_retry" {
					if err = tx.Exec(`UPDATE mediatool.depth_job SET status='failed',stage='failed',retryable=true WHERE id=?`, job.ID).Error; err != nil {
						t.Fatal(err)
					}
					action = func(a identity.Principal) error {
						_, e := store.Control(t.Context(), a, project, job.ID, uuid.New(), job.Revision, "retry")
						return e
					}
				} else {
					artifact := reviewDepthArtifact(t, job, asset)
					frozen := td.FrozenDepth{ProfileID: td.DepthProfileID, Input: td.FrozenSource{AssetID: asset.ID, Revision: 1, Kind: "video", ObjectKey: asset.ObjectKey, MIMEType: asset.MimeType, ByteSize: asset.ByteSize, SHA256: *asset.SHA256, Width: asset.Width, Height: asset.Height, DurationMS: asset.DurationMS}}
					digest, body, err := tool.DepthArtifactDigest(job, frozen, artifact)
					if err != nil {
						t.Fatal(err)
					}
					if err = tx.Exec(`INSERT INTO mediatool.depth_result_intent(job_id,attempt,asset_id,output_sha256,artifact_sha256,artifact,created_at) VALUES(?,1,?,?,?,?::jsonb,now())`, job.ID, artifact.Asset.ID, *artifact.Asset.SHA256, digest, string(body)).Error; err != nil {
						t.Fatal(err)
					}
					for _, o := range artifact.Objects {
						if err = tx.Exec(`INSERT INTO mediatool.depth_object(job_id,attempt,kind,object_key,sha256,byte_size,mime_type,status,write_started,updated_at) VALUES(?,1,?,?,?,?,?,'verified',true,now())`, job.ID, o.Kind, o.ObjectKey, o.SHA256, o.ByteSize, o.MIMEType).Error; err != nil {
							t.Fatal(err)
						}
					}
					if err = mediaFactory(tx).Store(t.Context(), actor, artifact.Asset, artifact.Renditions); err != nil {
						t.Fatal(err)
					}
					if err = tx.Exec(`UPDATE mediatool.depth_job SET status='review_required',stage='review_required',process_state='ended' WHERE id=?`, job.ID).Error; err != nil {
						t.Fatal(err)
					}
					action = func(a identity.Principal) error {
						_, e := store.Review(t.Context(), a, job.ID, uuid.New(), tool.ReviewInput{ProjectID: project, Revision: job.Revision, SHA256: *artifact.Asset.SHA256, LocalReviewConfirmed: true})
						return e
					}
				}
			}
			// Same ID/partition with a different valid request scope must not act.
			wrong := actor
			wrong.ScopeID = personalScope
			if err := action(wrong); err == nil {
				t.Fatal("wrong scope accepted")
			}
			if err := action(actor); err != nil {
				t.Fatalf("valid request lost session principal: %v", err)
			}
			if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
				t.Fatal(err)
			}
			if err := action(actor); !errors.Is(err, identity.ErrSessionInvalid) {
				t.Fatalf("revoked session did not fail first: %v", err)
			}
		})
	}
}
func reviewDepthArtifact(t *testing.T, job td.DepthJob, source md.MediaAsset) tool.DepthArtifact {
	t.Helper()
	id, key := tool.DepthOutputIdentity(job)
	now := job.CreatedAt.UTC()
	w, h, d := int32(1920), int32(1080), int32(1000)
	fps := 25.0
	codec := "h264"
	sha := strings.Repeat("b", 64)
	a := md.MediaAsset{ID: id, ProjectID: job.ProjectID, Kind: md.KindVideo, Origin: md.OriginSystem, Status: md.StatusProcessing, ObjectKey: key, MimeType: "video/mp4", ByteSize: 6, SHA256: &sha, Width: &w, Height: &h, DurationMS: &d, FPS: &fps, Codec: &codec, ModerationStatus: md.ModerationPending, Revision: 1, CreateTime: now, UpdateTime: now}
	facts := tool.DepthVideoFacts{Width: 64, Height: 36, FrameCount: 25, FPS: 25, DurationMS: 1000}
	output := facts
	output.Width, output.Height = 1920, 1080
	receipt := tool.DepthReceipt{ProfileID: td.DepthProfileID, VDACommit: td.DepthVDACommit, ModelSHA256: td.DepthModelSHA256, SourceTreeSHA256: td.DepthSourceTreeSHA256, RuntimeVersion: "controlled-test", Device: "mps", InputSHA256: *source.SHA256, Source: facts, Prepared: facts, Output: output, Native: tool.DepthExecutionFacts{ElapsedMS: 1, PeakRSSBytes: 1, RSSLimitBytes: 1 << 20, ProcessGroupJoined: true}}
	result := tool.DepthArtifact{JobID: job.ID, Attempt: job.Attempt, InputSHA256: *source.SHA256, Receipt: receipt, Asset: a, Objects: []tool.DepthObject{{Kind: "original", ObjectKey: key, SHA256: sha, ByteSize: 6, MIMEType: "video/mp4"}}}
	for i, kind := range []md.RenditionKind{md.RenditionPoster, md.RenditionProxy720p} {
		ext, mime := "png", "image/png"
		if i == 1 {
			ext, mime = "mp4", "video/mp4"
		}
		rkey := strings.TrimSuffix(key, path.Ext(key)) + "/" + string(kind) + "." + ext
		size := int64(6)
		result.Renditions = append(result.Renditions, md.Rendition{ID: uuid.NewSHA1(id, []byte(kind)), MediaAssetID: id, Kind: kind, ObjectKey: rkey, Width: &w, Height: &h, ByteSize: &size, CreateTime: now, UpdateTime: now})
		result.Objects = append(result.Objects, tool.DepthObject{Kind: string(kind), ObjectKey: rkey, SHA256: sha, ByteSize: size, MIMEType: mime})
	}
	return result
}
