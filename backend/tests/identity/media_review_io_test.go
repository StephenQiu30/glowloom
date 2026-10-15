package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	md "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	td "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

func reviewBytes(t *testing.T, body, mime string) *media.Downloaded {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "controlled-bytes-")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(body); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	return &media.Downloaded{File: f, Size: int64(len(body)), MIMEType: mime, SHA256: hex.EncodeToString(sum[:])}
}

type reviewObjects struct {
	bodies   map[string][]byte
	facts    map[string]media.ObjectInfo
	puts     int
	afterPut func(int)
}

func newReviewObjects() *reviewObjects {
	return &reviewObjects{bodies: map[string][]byte{}, facts: map[string]media.ObjectInfo{}}
}
func (o *reviewObjects) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := o.bodies[key]
	if !ok {
		return nil, tool.ErrObjectMissing
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (o *reviewObjects) Stat(_ context.Context, key string) (media.ObjectInfo, error) {
	v, ok := o.facts[key]
	if !ok {
		return v, tool.ErrObjectMissing
	}
	return v, nil
}
func (o *reviewObjects) PutIfAbsent(_ context.Context, key string, r io.Reader, n int64, mime, sha string) error {
	o.puts++
	if _, ok := o.bodies[key]; ok {
		return media.ErrObjectAlreadyExists
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	o.bodies[key] = b
	o.facts[key] = media.ObjectInfo{Size: n, ContentType: mime, SHA256: sha}
	if o.afterPut != nil {
		o.afterPut(o.puts)
	}
	return nil
}
func (o *reviewObjects) seed(key, body, mime string) string {
	sum := sha256.Sum256([]byte(body))
	sha := hex.EncodeToString(sum[:])
	o.bodies[key] = []byte(body)
	o.facts[key] = media.ObjectInfo{Size: int64(len(body)), ContentType: mime, SHA256: sha}
	return sha
}

type reviewProbe struct{}

func (reviewProbe) Probe(_ context.Context, f *media.Downloaded) (media.ProbeResult, error) {
	w, h, d := int32(1920), int32(1080), int32(1000)
	fps := 25.0
	codec := "h264"
	kind, ext := md.KindVideo, "mp4"
	source := sha256.Sum256([]byte("source"))
	if f.SHA256 == hex.EncodeToString(source[:]) {
		w, h = 64, 36
	}
	if f.MIMEType == "image/png" {
		kind, ext = md.KindImage, "png"
	}
	return media.ProbeResult{Kind: kind, Extension: ext, Width: &w, Height: &h, DurationMS: &d, FPS: &fps, Codec: &codec}, nil
}

type reviewExportRenderer struct {
	t     *testing.T
	after func()
}

func (r reviewExportRenderer) Render(context.Context, td.FrozenExport, map[uuid.UUID]string, func(int, string) error) (*media.Downloaded, error) {
	if r.after != nil {
		r.after()
	}
	return reviewBytes(r.t, "output", "video/mp4"), nil
}

type reviewPreviews struct {
	t     *testing.T
	after func()
}

func (r reviewPreviews) Render(_ context.Context, _ *media.Downloaded, p media.ProbeResult, _ string) ([]media.RenditionFile, error) {
	if r.after != nil {
		r.after()
	}
	kinds := []md.RenditionKind{md.RenditionPoster, md.RenditionProxy720p}
	if p.Kind == md.KindImage {
		kinds = []md.RenditionKind{md.RenditionThumb256, md.RenditionThumb640}
	}
	out := []media.RenditionFile{}
	for i, k := range kinds {
		ext, mime := "png", "image/png"
		if k == md.RenditionProxy720p {
			ext, mime = "mp4", "video/mp4"
		}
		out = append(out, media.RenditionFile{Kind: k, Result: reviewBytes(r.t, []string{"preview-one", "preview-two"}[i], mime), Width: 1920, Height: 1080, Ext: ext})
	}
	return out, nil
}
func TestMediaExportLateRevocationStopsEachObjectWrite(t *testing.T) {
	for _, phase := range []string{"render", "previews", "first_preview", "allowed"} {
		t.Run(phase, func(t *testing.T) {
			tx, actor, team, scope, project, factory := projectConsumerFixture(t)
			installMediaScopeMigration(t, tx)
			installMediaReviewStorageColumns(t, tx)
			actor.ScopeID = scope
			asset := reviewVideo(t, tx, project)
			objects := newReviewObjects()
			sha := objects.seed(asset.ObjectKey, "source", "video/mp4")
			if err := tx.Exec(`UPDATE media.media_asset SET sha256=? WHERE id=?`, sha, asset.ID).Error; err != nil {
				t.Fatal(err)
			}
			request := func(tx *gorm.DB) tool.DerivedMedia {
				return media.NewDerivedService(pgmedia.NewScopedStore(tx, func(tx *gorm.DB) media.ScopeAccess { return mediaScopeTestAccess{tx: tx, request: factory(tx)} }))
			}
			store := pgtool.NewStore(tx, func(*gorm.DB) tool.TimelineReader { return reviewScopeTimeline{reviewTimeline(asset.ID)} }, request)
			job, err := store.Create(t.Context(), actor, project, uuid.New(), tool.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
			if err != nil {
				t.Fatal(err)
			}
			workerStore := pgtool.NewStore(tx, nil, func(tx *gorm.DB) tool.DerivedMedia {
				return media.NewDerivedService(pgmedia.NewScopedStore(tx, func(tx *gorm.DB) media.ScopeAccess { return mediaScopeTestAccess{tx: tx, worker: true} }))
			})
			revoke := func() {
				if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			renderer := reviewExportRenderer{t: t}
			previews := reviewPreviews{t: t}
			want := 3
			switch phase {
			case "render":
				renderer.after = revoke
				want = 0
			case "previews":
				previews.after = revoke
				want = 1
			case "first_preview":
				objects.afterPut = func(n int) {
					if n == 2 {
						revoke()
					}
				}
				want = 2
			}
			_, err = tool.NewWorker(workerStore, objects, renderer, reviewProbe{}, previews).Execute(t.Context(), tool.WorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()})
			if phase == "allowed" && err != nil {
				t.Fatalf("authorized output: %v", err)
			}
			if phase != "allowed" && err == nil {
				t.Fatal("revoked worker completed")
			}
			if objects.puts != want {
				t.Fatalf("late %s revocation: Put calls=%d want %d (err=%v)", phase, objects.puts, want, err)
			}
		})
	}
}

type reviewAudio struct{ t *testing.T }

func (p reviewAudio) Prepare(context.Context, *media.Downloaded, int64) (*tool.PreparedAudio, error) {
	return &tool.PreparedAudio{File: reviewBytes(p.t, "pcm", "audio/wav"), DurationMS: 1000}, nil
}

type reviewTranscriber struct {
	after     func()
	calls     int
	uncertain bool
}

func (p *reviewTranscriber) Transcribe(context.Context, *media.Downloaded, string, int64) (td.Transcript, error) {
	p.calls++
	p.after()
	if p.uncertain {
		return td.Transcript{}, tool.ErrInferenceUncertain
	}
	return td.Transcript{Version: 1, Language: "en", DurationMS: 1000, Segments: []td.SubtitleSegment{{StartMS: 0, EndMS: 1000, Text: "controlled result"}}}, nil
}
func TestMediaTranscriptionLateRevocationPreservesCessation(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "terminal", true: "unknown"}[uncertain], func(t *testing.T) {
			tx, actor, team, scope, project, factory := projectConsumerFixture(t)
			installMediaScopeMigration(t, tx)
			installMediaReviewStorageColumns(t, tx)
			actor.ScopeID = scope
			asset := reviewVideo(t, tx, project)
			objects := newReviewObjects()
			sha := objects.seed(asset.ObjectKey, "source", "video/mp4")
			if err := tx.Exec(`UPDATE media.media_asset SET sha256=? WHERE id=?`, sha, asset.ID).Error; err != nil {
				t.Fatal(err)
			}
			request := pgtool.NewTranscriptionStore(tx, func(*gorm.DB) tool.TranscriptionSourceReader { return reviewScopeSource{asset.ID} }, func(tx *gorm.DB) tool.DerivedMedia {
				return media.NewDerivedService(pgmedia.NewScopedStore(tx, func(tx *gorm.DB) media.ScopeAccess { return mediaScopeTestAccess{tx: tx, request: factory(tx)} }))
			})
			job, err := request.Create(t.Context(), actor, project, uuid.New(), tool.TranscriptionCreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1, Language: "auto"})
			if err != nil {
				t.Fatal(err)
			}
			store := pgtool.NewTranscriptionStore(tx, nil, func(tx *gorm.DB) tool.DerivedMedia {
				return media.NewDerivedService(pgmedia.NewScopedStore(tx, func(tx *gorm.DB) media.ScopeAccess { return mediaScopeTestAccess{tx: tx, worker: true} }))
			})
			native := &reviewTranscriber{uncertain: uncertain, after: func() {
				if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
					t.Fatal(err)
				}
			}}
			worker := tool.NewTranscriptionWorker(store, objects, reviewAudio{t}, native)
			id := tool.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
			_, err = worker.Execute(t.Context(), id)
			if err == nil || objects.puts != 0 {
				t.Fatalf("late revoke: Put calls=%d err=%v", objects.puts, err)
			}
			var state string
			if err := tx.Raw(`SELECT inference_state FROM mediatool.transcription_job WHERE id=?`, job.ID).Scan(&state).Error; err != nil {
				t.Fatal(err)
			}
			want := "terminal"
			if uncertain {
				want = "submitted"
			}
			if state != want {
				t.Fatalf("cessation evidence: %s want %s", state, want)
			}
			if err := tx.Exec(`UPDATE workspace.team_member SET status='active' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
				t.Fatal(err)
			}
			_, _ = worker.Execute(t.Context(), id)
			if native.calls != 1 || objects.puts != 0 {
				t.Fatalf("attempt resubmitted/created object: native=%d objects=%d", native.calls, objects.puts)
			}
			if err := tx.Raw(`SELECT inference_state FROM mediatool.transcription_job WHERE id=?`, job.ID).Scan(&state).Error; err != nil {
				t.Fatal(err)
			}
			// The active physical owner has not released uncertain inference; keep
			// its submitted receipt until reconciliation proves cessation.
			if state != want {
				t.Fatalf("retry lost cessation evidence: %s want %s", state, want)
			}
		})
	}
}

type reviewDownloader struct{ t *testing.T }

func (d reviewDownloader) Download(context.Context, string) (*media.Downloaded, error) {
	return reviewBytes(d.t, "output", "video/mp4"), nil
}
func seedReviewIngest(t *testing.T, tx *gorm.DB, project, actor uuid.UUID) uuid.UUID {
	t.Helper()
	op, provider, profile, version := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	suffix := uuid.NewString()
	capability := "review.video." + suffix
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO catalog.capability(id,key,output_type,modes,input_roles) VALUES(?,?,'video',ARRAY['text_to_video'],ARRAY['prompt'])`, []any{uuid.New(), capability}},
		{`INSERT INTO catalog.provider(id,key,name,adapter_key,region) VALUES(?,?,'Controlled','mock','overseas')`, []any{provider, "review-" + suffix}},
		{`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name,status) VALUES(?,?,?,?, 'Controlled','active')`, []any{profile, "review-" + suffix, provider, capability}},
		{`INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms,queue) VALUES(?,?,1,'controlled',ARRAY['text_to_video'],'{}','[]',true,true,false,60000,'test')`, []any{version, profile}},
		{`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,model_profile_version_id,input_hash,origin,status,region,confirmed_by) VALUES(?,?,'free',?,'text_to_video',?,?,'canvas','ingesting','overseas',?)`, []any{op, project, capability, version, suffix, actor}},
	}
	for _, s := range statements {
		if err := tx.Exec(s.sql, s.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	return op
}
func TestMediaIngestLateRevocationStopsEachObjectWrite(t *testing.T) {
	for _, point := range []int{0, 1, 2, 3} {
		t.Run([]string{"render", "original", "first_preview", "allowed"}[point], func(t *testing.T) {
			tx, actor, team, _, project, _ := projectConsumerFixture(t)
			op := seedReviewIngest(t, tx, project, actor.ID)
			repo := pgmedia.NewScopedIngestRepository(tx, func(tx *gorm.DB) media.ScopeAccess { return mediaScopeTestAccess{tx: tx, worker: true} })
			objects := newReviewObjects()
			revoke := func() {
				if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			previews := reviewPreviews{t: t}
			if point == 0 {
				previews.after = revoke
			} else if point < 3 {
				objects.afterPut = func(n int) {
					if n == point {
						revoke()
					}
				}
			}
			_, err := media.NewIngestService(repo, reviewDownloader{t}, reviewProbe{}, previews, objects).Ingest(t.Context(), media.IngestInput{OperationID: op.String(), SeqNo: 1, URL: "https://controlled.invalid/result"})
			if (point < 3 && err == nil) || (point == 3 && err != nil) || objects.puts != point {
				t.Fatalf("revoked ingest: Put calls=%d want %d err=%v", objects.puts, point, err)
			}
			var count int64
			if err := tx.Raw(`SELECT count(*) FROM operation.operation_output WHERE operation_id=?`, op).Scan(&count).Error; err != nil {
				t.Fatal(err)
			}
			if (point < 3 && count != 0) || (point == 3 && count != 1) {
				t.Fatalf("unexpected output publication: %d", count)
			}
		})
	}
}
