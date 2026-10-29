package media_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	project "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/editproject"
	toolhttp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/http"
	toolpg "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// The owner connection is only for explicit synthetic fixture state transitions.
// All application calls, including receipt cleanup, use the restricted runtime role.
func TestJianyingLaunchPostgresAuthorityIdentityExpiryAndCleanup(t *testing.T) {
	db, owner := mediaStoreDB(t), projectOwnerDB(t)
	var role string
	if err := db.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("launch integration requires the restricted runtime role")
	}
	actor, projectID := mediaStoreProject(t, owner)
	foreignActor, foreignProject := mediaStoreProject(t, owner)
	frozen, paths := projectFixture(t)
	input := frozen.Inputs[0]
	asset := imageAsset(projectID, input.AssetID)
	asset.Width, asset.Height, asset.ByteSize, asset.SHA256 = input.Width, input.Height, input.ByteSize, &input.SHA256
	asset.Status, asset.ModerationStatus = mediadomain.StatusProcessing, mediadomain.ModerationPending
	if err := mediaTestStore(db).CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(paths[input.AssetID])
	if err != nil {
		t.Fatal(err)
	}
	objects := &projectObjects{data: map[string][]byte{asset.ObjectKey: original}, info: map[string]mediaapp.ObjectInfo{asset.ObjectKey: {Size: asset.ByteSize, ContentType: asset.MimeType, SHA256: input.SHA256}}}
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediaTestStore(tx), nil) }
	canvases := canvasfixture.NewService(mediaCanvasStore(db, mediaFactory))
	doc, err := canvases.Create(t.Context(), actor, projectID, uuid.NewString(), canvasapp.CreateInput{Name: "Native launch integration"})
	if err != nil {
		t.Fatal(err)
	}
	node := canvasdomain.Node{ID: uuid.New(), Title: "Timeline", NodeType: "timeline", NodeAction: "tool", Config: canvasdomain.NodeConfig{Timeline: &frozen.Timeline}}
	saved, err := canvases.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: doc.Revision, Operations: canvasfixture.Concat(canvasfixture.Nodes([]canvasdomain.Node{node}))})
	if err != nil {
		t.Fatal(err)
	}
	store := toolpg.NewStore(db, func(tx *gorm.DB) toolapp.TimelineReader {
		return canvasapp.NewTimelineReader(mediaCanvasStore(tx, mediaFactory))
	}, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaTestStore(tx)) })
	workerStore := toolpg.NewStore(db, func(tx *gorm.DB) toolapp.TimelineReader {
		return canvasapp.NewTimelineReader(mediaCanvasWorkerStore(tx, func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediaWorkerStore(tx), nil) }))
	}, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaWorkerStore(tx)) })
	job, err := store.Create(t.Context(), actor, projectID, uuid.New(), toolapp.CreateInput{CanvasID: doc.ID, NodeID: node.ID, Revision: saved.Revision, OutputKind: tool.OutputJianying, Settings: &tool.ExportSettings{FileName: "原生票据测试", Destination: "local", Resolution: 720}})
	if err != nil {
		t.Fatal(err)
	}
	query := toolapp.NewExportQuery(store, objects, objects).WithLaunchOrigin("http://localhost:3000")
	if _, e := query.Preview(t.Context(), actor, projectID, job.ID); !errors.Is(e, toolapp.ErrConflict) {
		t.Fatal("queued job admitted preview")
	}
	worker := toolapp.NewWorker(workerStore, objects, project.NewRenderer(nil), mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
	reviewed, err := worker.Execute(t.Context(), toolapp.WorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err != nil || reviewed.Status != tool.Succeeded {
		t.Fatalf("configured native export status=%s error=%v", reviewed.Status, err)
	}
	// The retained legacy review state must not mint a capability either.
	if e := owner.Exec(`UPDATE mediatool.export_job SET status='review_required' WHERE id=?`, job.ID).Error; e != nil {
		t.Fatal(e)
	}
	before, e := query.Preview(t.Context(), actor, projectID, job.ID)
	if e != nil || before.LaunchURL != "" {
		t.Fatal("unreviewed job minted launch capability")
	}
	if e = owner.Exec(`UPDATE mediatool.export_job SET status='succeeded' WHERE id=?`, job.ID).Error; e != nil {
		t.Fatal(e)
	}
	mint := func() (string, string, toolapp.ExportPreview) {
		t.Helper()
		p, e := query.Preview(t.Context(), actor, projectID, job.ID)
		if e != nil {
			t.Fatal(e)
		}
		u, e := url.Parse(p.LaunchURL)
		if e != nil {
			t.Fatal("invalid launch URL")
		}
		token := u.Query().Get("token")
		entropy, e := hex.DecodeString(token)
		if e != nil || len(entropy) != 32 {
			t.Fatal("invalid capability entropy")
		}
		hash := sha256.Sum256(entropy)
		return token, hex.EncodeToString(hash[:]), p
	}
	token, hash, preview := mint()
	var stored struct {
		TokenHash                    string
		JobID, ArtifactID, ProjectID uuid.UUID
		Revision                     int64
		Actor, Envelope              []byte
		ExpiresAt                    time.Time
	}
	if err = owner.Raw(`SELECT * FROM mediatool.export_launch WHERE token_hash=?`, hash).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TokenHash == token || stored.TokenHash != hash || stored.JobID != job.ID || stored.ProjectID != projectID || stored.ArtifactID != *reviewed.ArtifactID || stored.Revision != reviewed.Revision || stored.ExpiresAt.After(preview.ExpiresAt.Add(time.Microsecond)) {
		t.Fatal("persisted receipt identity/expiry mismatch")
	}
	// The runtime can issue/delete receipts but cannot extend or rewrite one.
	if e := db.Exec(`UPDATE mediatool.export_launch SET expires_at=expires_at+interval '1 second' WHERE token_hash=?`, hash).Error; e == nil {
		t.Fatal("runtime role rewrote immutable capability")
	} else {
		var pg *pgconn.PgError
		if !errors.As(e, &pg) || pg.Code != "42501" {
			t.Fatalf("expected receipt UPDATE denial, got %v", e)
		}
	}
	if strings.Contains(string(stored.Actor), actor.LoginName) || strings.Contains(string(stored.Actor), actor.DisplayName) {
		t.Fatal("receipt copied unnecessary profile data")
	}
	var envelope struct {
		Draft struct {
			Folder string `json:"folder_name"`
			Drafts []struct {
				URL  string `json:"draft_url"`
				Name string `json:"draft_name"`
			} `json:"drafts"`
		} `json:"short_video_draft"`
	}
	if json.Unmarshal(stored.Envelope, &envelope) != nil || envelope.Draft.Folder != "原生票据测试" || len(envelope.Draft.Drafts) != 1 || envelope.Draft.Drafts[0].Name != "原生票据测试" || envelope.Draft.Drafts[0].URL != preview.URL {
		t.Fatal("stored envelope lost native name")
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	toolhttp.NewHandler(store, query).RegisterLaunch(router.Group("/api"))
	expectHTTP := func(tok string, status int) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/api/media-export-launch?token="+tok, nil))
		if w.Code != status {
			t.Fatalf("anonymous launch status=%d want=%d", w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("capability response was cacheable")
		}
	}
	expectHTTP(token, 200)
	expectHTTP(strings.Repeat("a", 64), 404)
	base := toolapp.LaunchReceipt{TokenHash: strings.Repeat("b", 64), JobID: job.ID, ArtifactID: *reviewed.ArtifactID, ProjectID: projectID, Revision: reviewed.Revision, Actor: actor, Body: stored.Envelope, ExpiresAt: time.Now().Add(time.Minute)}
	for _, mutate := range []func(*toolapp.LaunchReceipt){func(r *toolapp.LaunchReceipt) { r.ArtifactID = uuid.New() }, func(r *toolapp.LaunchReceipt) { r.Revision++ }, func(r *toolapp.LaunchReceipt) { r.ProjectID = foreignProject }, func(r *toolapp.LaunchReceipt) { r.Actor = foreignActor }, func(r *toolapp.LaunchReceipt) { r.ExpiresAt = time.Now().Add(11 * time.Minute) }, func(r *toolapp.LaunchReceipt) { r.ExpiresAt = time.Now().Add(-time.Second) }} {
		r := base
		mutate(&r)
		if e := store.SaveLaunch(t.Context(), r); e == nil {
			t.Fatal("forged receipt identity or authority accepted")
		}
	}
	// State changes below are isolated fixture fault injection, never business DB edits.
	exec := func(sql string, args ...any) {
		t.Helper()
		if e := owner.Exec(sql, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec(`UPDATE mediatool.export_job SET revision=revision+1 WHERE id=?`, job.ID)
	expectHTTP(token, 404)
	exec(`UPDATE mediatool.export_job SET revision=revision-1 WHERE id=?`, job.ID)
	exec(`UPDATE mediatool.export_job SET status='cancelled' WHERE id=?`, job.ID)
	expectHTTP(token, 404)
	exec(`UPDATE mediatool.export_job SET status='succeeded' WHERE id=?`, job.ID)
	exec(`UPDATE mediatool.export_job SET frozen=jsonb_set(frozen,'{export_id}',to_jsonb(?::text)) WHERE id=?`, uuid.NewString(), job.ID)
	expectHTTP(token, 404)
	exec(`UPDATE mediatool.export_job SET frozen=jsonb_set(frozen,'{export_id}',to_jsonb(?::text)) WHERE id=?`, job.ID.String(), job.ID)
	exec(`UPDATE mediatool.export_job SET frozen=jsonb_set(frozen,'{source,revision}',to_jsonb(source_revision+1)) WHERE id=?`, job.ID)
	expectHTTP(token, 404)
	exec(`UPDATE mediatool.export_job SET frozen=jsonb_set(frozen,'{source,revision}',to_jsonb(source_revision)) WHERE id=?`, job.ID)
	expectHTTP(token, 200)
	token2, hash2, _ := mint()
	liveToken, _, _ := mint()
	exec(`UPDATE mediatool.export_launch SET created_at=created_at-interval '20 minutes',expires_at=expires_at-interval '20 minutes' WHERE token_hash IN (?,?)`, hash, hash2)
	expectHTTP(token, 404)
	expectHTTP(token2, 404)
	expectHTTP(liveToken, 200)
	for _, limit := range []int{0, 1001} {
		if _, e := store.PruneLaunches(t.Context(), time.Now(), limit); !errors.Is(e, toolapp.ErrInvalidExport) {
			t.Fatal("unbounded cleanup accepted")
		}
	}
	for i := 0; i < 2; i++ {
		n, e := store.PruneLaunches(t.Context(), time.Now(), 1)
		if e != nil || n != 1 {
			t.Fatalf("bounded cleanup rows=%d error=%v", n, e)
		}
	}
	if n, e := store.PruneLaunches(t.Context(), time.Now(), 1); e != nil || n != 0 {
		t.Fatal("cleanup removed unexpired receipt")
	}
	expectHTTP(liveToken, 200)
	var count int64
	if err = owner.Raw(`SELECT count(*) FROM mediatool.export_launch WHERE job_id=?`, job.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("cleanup membership mismatch")
	}
	exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID)
	expectHTTP(liveToken, 404)
	if _, e := query.Preview(t.Context(), actor, projectID, job.ID); e == nil {
		t.Fatal("revoked session minted a new capability")
	}
	t.Log("restricted-role PostgreSQL native export, receipt binding, anonymous access, revoked authority, changed job/frozen identity, expiry and bounded cleanup verified")
}
