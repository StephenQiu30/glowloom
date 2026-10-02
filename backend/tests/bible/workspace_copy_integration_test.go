package bible_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	billingpg "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	scriptpg "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func coordinatedBibleSnapshot(s app.ProjectCopySnapshot) workspacedomain.ProjectCopyBibleSnapshot {
	r := workspacedomain.ProjectCopyBibleSnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: workspacedomain.ProjectCopyBibleCounts(s.Counts), Identities: make([]workspacedomain.ProjectCopyBibleMapping, len(s.Identities)), Versions: make([]workspacedomain.ProjectCopyBibleMapping, len(s.Versions))}
	for i, m := range s.Identities {
		r.Identities[i] = workspacedomain.ProjectCopyBibleMapping{Kind: string(m.Kind), SourceID: m.SourceID, TargetID: m.TargetID}
	}
	for i, m := range s.Versions {
		r.Versions[i] = workspacedomain.ProjectCopyBibleMapping{Kind: string(m.Kind), SourceID: m.SourceID, TargetID: m.TargetID}
	}
	return r
}
func ownCoordinatedBibleSnapshot(s workspacedomain.ProjectCopyBibleSnapshot) app.ProjectCopySnapshot {
	r := app.ProjectCopySnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: app.CopyCounts(s.Counts), Identities: make([]app.IdentityMapping, len(s.Identities)), Versions: make([]app.VersionMapping, len(s.Versions))}
	for i, m := range s.Identities {
		r.Identities[i] = app.IdentityMapping{Kind: domain.Kind(m.Kind), SourceID: m.SourceID, TargetID: m.TargetID}
	}
	for i, m := range s.Versions {
		r.Versions[i] = app.VersionMapping{Kind: domain.Kind(m.Kind), SourceID: m.SourceID, TargetID: m.TargetID}
	}
	return r
}
func coordinatedBibleReceipt(r app.ProjectCopyReceipt) workspacedomain.ProjectCopyBibleReceipt {
	return workspacedomain.ProjectCopyBibleReceipt{ManifestSHA256: r.ManifestSHA256, ContentSHA256: r.ContentSHA256, Counts: workspacedomain.ProjectCopyBibleCounts(r.Counts)}
}
func coordinatedCopyBibleBinding(b workspaceapp.ProjectCopyBinding) app.ProjectCopyBinding {
	return app.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}
}
func coordinatedBibleStore(db *gorm.DB, a workspaceapp.ProjectCopyAuthority, s mediaapp.ProjectCopySnapshot) *pg.ProjectCopyStore {
	return pg.NewProjectCopyStore(db, func(tx *gorm.DB) app.ProjectCopyAccess {
		return bibleCopyAccess{workspacepg.NewProjectCopyAccessStore(tx, a)}
	}, func(tx *gorm.DB) app.ProjectCopyMedia {
		return bibleCopyMedia{mediaapp.NewProjectCopyReferenceQuery(mediapg.NewProjectCopyStore(tx), nil), s}
	}, func(tx *gorm.DB) app.ProjectCopyScopes { return bibleCopyScopes{scriptCopyOwner(tx, a)} })
}

type coordinatedBibleOwner struct{ store *pg.ProjectCopyStore }

func (o coordinatedBibleOwner) ReferencedMediaFacts(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding) ([]mediaapp.ReferenceFact, error) {
	f, e := o.store.ReferencedMediaFacts(ctx, actor, coordinatedCopyBibleBinding(b))
	r := make([]mediaapp.ReferenceFact, len(f))
	for i, v := range f {
		r[i] = referenceFact(v)
	}
	return r, e
}
func (o coordinatedBibleOwner) Freeze(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, assets map[uuid.UUID]uuid.UUID, at time.Time) (workspacedomain.ProjectCopyBibleSnapshot, error) {
	s, e := o.store.Freeze(ctx, actor, coordinatedCopyBibleBinding(b), assets, at)
	return coordinatedBibleSnapshot(s), e
}
func (o coordinatedBibleOwner) Register(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) (workspacedomain.ProjectCopyBibleReceipt, error) {
	r, e := o.store.Register(ctx, actor, coordinatedCopyBibleBinding(b), ownCoordinatedBibleSnapshot(s))
	return coordinatedBibleReceipt(r), e
}
func (o coordinatedBibleOwner) Verify(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) (workspacedomain.ProjectCopyBibleReceipt, error) {
	r, e := o.store.Verify(ctx, actor, coordinatedCopyBibleBinding(b), ownCoordinatedBibleSnapshot(s))
	return coordinatedBibleReceipt(r), e
}
func (o coordinatedBibleOwner) FinishCleanup(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) error {
	return o.store.Cleanup(ctx, actor, coordinatedCopyBibleBinding(b), ownCoordinatedBibleSnapshot(s))
}

func coordinatedScriptStore(db *gorm.DB, a workspaceapp.ProjectCopyAuthority) *scriptpg.ProjectCopyStore {
	return scriptpg.NewProjectCopyStoreWithCharacters(db, func(tx *gorm.DB) scriptapp.ProjectCopyAccess {
		return bibleScriptCopyAccess{workspacepg.NewProjectCopyAccessStore(tx, a)}
	}, func(tx *gorm.DB) scriptapp.ProjectCopyCharacters {
		if a.Bible == nil {
			return nil
		}
		return scriptCopyCharacters{owner: bibleCopyStore(tx, a), snapshot: ownCoordinatedBibleSnapshot(*a.Bible)}
	})
}
func ownCoordinatedScriptSnapshot(s workspacedomain.ProjectCopyScriptSnapshot) scriptapp.ProjectCopySnapshot {
	return scriptapp.ProjectCopySnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: scriptapp.ScriptCopyCounts(s.Counts)}
}

type coordinatedScriptOwner struct{ store *scriptpg.ProjectCopyStore }

func (o coordinatedScriptOwner) ReferencedMedia(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding) ([]uuid.UUID, error) {
	return o.store.ReferencedMedia(ctx, a, scriptapp.ProjectCopyBinding(b))
}
func (o coordinatedScriptOwner) Freeze(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, assets map[uuid.UUID]uuid.UUID, at time.Time) (workspacedomain.ProjectCopyScriptSnapshot, error) {
	s, e := o.store.Freeze(ctx, a, scriptapp.ProjectCopyBinding(b), assets, at)
	return workspacedomain.ProjectCopyScriptSnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: workspacedomain.ProjectCopyScriptCounts(s.Counts)}, e
}
func (o coordinatedScriptOwner) Register(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) (workspacedomain.ProjectCopyScriptReceipt, error) {
	r, e := o.store.Register(ctx, a, scriptapp.ProjectCopyBinding(b), ownCoordinatedScriptSnapshot(s))
	return workspacedomain.ProjectCopyScriptReceipt{ManifestSHA256: r.ManifestSHA256, ContentSHA256: r.ContentSHA256, Counts: workspacedomain.ProjectCopyScriptCounts(r.Counts)}, e
}
func (o coordinatedScriptOwner) FinishCleanup(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) error {
	return o.store.FinishCleanup(ctx, a, scriptapp.ProjectCopyBinding(b), ownCoordinatedScriptSnapshot(s))
}

type coordinatedCanvasMedia struct {
	owner   *mediapg.ProjectCopyStore
	binding mediaapp.ProjectCopyBinding
}

func (r coordinatedCanvasMedia) Reference(ctx context.Context, a identityapp.Principal, p, id uuid.UUID) (mediaapp.AssetSummary, error) {
	if p != r.binding.TargetProjectID {
		return mediaapp.AssetSummary{}, mediaapp.ErrNotFound
	}
	return r.owner.ReferenceCopiedAsset(ctx, a, r.binding, id)
}
func coordinatedCopyOwners(tx *gorm.DB) workspaceapp.ProjectCopyOwners {
	return workspaceapp.ProjectCopyOwners{Media: mediapg.NewProjectCopyStore(tx), Budget: billingpg.NewStore(tx), Canvas: canvaspg.NewProjectCopyStore(tx, func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil) }, func(tx *gorm.DB, b canvasapp.ProjectCopyBinding) canvasapp.MediaReader {
		return coordinatedCanvasMedia{mediapg.NewProjectCopyStore(tx), mediaapp.ProjectCopyBinding(b)}
	})}
}
func coordinatedWorkspaceCopy(db *gorm.DB) *workspacepg.ProjectCopyStore {
	return workspacepg.NewProjectCopyStoreWithBible(db, func(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
		return scriptpg.NewSourceStore(tx, func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) })
	}, coordinatedCopyOwners, func(tx *gorm.DB, a workspaceapp.ProjectCopyAuthority) workspaceapp.ProjectCopyScriptOwner {
		return coordinatedScriptOwner{coordinatedScriptStore(tx, a)}
	}, func(tx *gorm.DB, a workspaceapp.ProjectCopyAuthority, s mediaapp.ProjectCopySnapshot) workspaceapp.ProjectCopyBibleOwner {
		return coordinatedBibleOwner{coordinatedBibleStore(tx, a, s)}
	})
}

type coordinatedScriptTransfer struct{ owner *scriptapp.ProjectCopy }

func (t coordinatedScriptTransfer) Transfer(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) error {
	_, e := t.owner.Transfer(ctx, a, scriptapp.ProjectCopyBinding(b), ownCoordinatedScriptSnapshot(s))
	var x *scriptapp.ProjectCopyTransferError
	if errors.As(e, &x) {
		return &workspaceapp.ProjectCopyScriptTransferError{Code: x.Code, NeedsReconciliation: x.NeedsReconciliation, Cause: e}
	}
	return e
}
func (t coordinatedScriptTransfer) Cleanup(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) error {
	return t.owner.Cleanup(ctx, a, scriptapp.ProjectCopyBinding(b), ownCoordinatedScriptSnapshot(s))
}

type coordinatedBibleTransfer struct {
	owner         *app.ProjectCopy
	database      *gorm.DB
	authority     workspaceapp.ProjectCopyAuthority
	mediaSnapshot mediaapp.ProjectCopySnapshot
}

func (t coordinatedBibleTransfer) Transfer(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) error {
	e := t.owner.Transfer(ctx, a, coordinatedCopyBibleBinding(b), ownCoordinatedBibleSnapshot(s))
	var x *app.ProjectCopyTransferError
	if errors.As(e, &x) {
		return &workspaceapp.ProjectCopyBibleTransferError{Code: x.Code, NeedsReconciliation: x.NeedsReconciliation, Cause: e}
	}
	return e
}
func (t coordinatedBibleTransfer) Cleanup(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) error {
	return t.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return coordinatedBibleStore(tx, t.authority, t.mediaSnapshot).Cleanup(ctx, a, coordinatedCopyBibleBinding(b), ownCoordinatedBibleSnapshot(s))
	})
}
func coordinatedAuthority(j workspacedomain.ProjectCopyJob, w uuid.UUID, phase string) workspaceapp.ProjectCopyAuthority {
	return workspaceapp.ProjectCopyAuthority{Binding: workspaceapp.ProjectCopyBinding{JobID: j.ID, OrgID: j.OrgID, SourceProjectID: j.SourceProjectID, TargetProjectID: j.TargetProjectID}, ActorID: j.ExecutionActor(), WorkerID: w, SourceRevision: j.SourceRevision, Phase: phase, Bible: j.Manifest.Bible}
}
func coordinatedWorker(db *gorm.DB, store *workspacepg.ProjectCopyStore, objects *objectstorage.Client, beforeBible func(workspacedomain.ProjectCopyJob)) *workspaceapp.ProjectCopyWorker {
	return workspaceapp.NewProjectCopyWorkerWithBible(store, func(j workspacedomain.ProjectCopyJob, w uuid.UUID, cleanup bool) *mediaapp.ProjectCopyTransfer {
		return mediaapp.NewProjectCopyTransfer(workspacepg.NewProjectCopyMediaObjects(store, j.ID, w, cleanup), mediaobjects.NewProjectCopyObjects(objects), "")
	}, func(j workspacedomain.ProjectCopyJob, w uuid.UUID, cleanup bool) workspaceapp.ProjectCopyScriptTransfer {
		phase := "transfer"
		if cleanup {
			phase = "cleanup"
		}
		return coordinatedScriptTransfer{scriptapp.NewProjectCopy(coordinatedScriptStore(db, coordinatedAuthority(j, w, phase)), scriptobjects.NewStorage(objects))}
	}, func(j workspacedomain.ProjectCopyJob, w uuid.UUID, cleanup bool) workspaceapp.ProjectCopyBibleTransfer {
		if beforeBible != nil {
			beforeBible(j)
		}
		phase := "bible_transfer"
		if j.Stage == "finalizing" {
			phase = "bible_verify"
		}
		if cleanup {
			phase = "cleanup"
		}
		a := coordinatedAuthority(j, w, phase)
		s := mediaapp.ProjectCopySnapshot{ID: j.Manifest.MediaSnapshotID, ManifestSHA256: j.Manifest.MediaSHA256, Assets: j.Manifest.Assets, Renditions: j.Manifest.Renditions}
		return coordinatedBibleTransfer{owner: app.NewProjectCopy(coordinatedBibleStore(db, a, s), bibleTransferredReferences{db, objects, a, s}), database: db, authority: a, mediaSnapshot: s}
	}, time.Now)
}

type coordinatedContent struct {
	actor                                                   identityapp.Principal
	project, character, pinnedVersion, canvas, image, audio uuid.UUID
}

func coordinatedContentFixture(t *testing.T, db, owner *gorm.DB, objects *objectstorage.Client) coordinatedContent {
	t.Helper()
	actor, p := bibleActorProject(t, owner)
	var picture bytes.Buffer
	if e := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 32, 32))); e != nil {
		t.Fatal(e)
	}
	imageID := bibleUploadedMedia(t, db, objects, actor, p, "full.png", picture.Bytes())
	audioPath := filepath.Join(t.TempDir(), "full.wav")
	if e := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi", "-i", "sine=frequency=880:duration=1", "-c:a", "pcm_s16le", audioPath).Run(); e != nil {
		t.Fatal(e)
	}
	audio, e := os.ReadFile(audioPath)
	if e != nil {
		t.Fatal(e)
	}
	audioID := bibleUploadedMedia(t, db, objects, actor, p, "full.wav", audio)
	sources := scriptpg.NewSourceStoreWithCharacters(db, func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, func(tx *gorm.DB) scriptapp.CharacterReferences {
		return characterBridge{pg.NewReferences(tx, workspacepg.NewProjectContentAccessStore(tx))}
	})
	html := "<p>王总：你好😀</p>"
	saved, e := scriptapp.NewSourceService(sources, scriptobjects.NewStorage(objects), time.Now).Write(t.Context(), actor, scriptapp.SourceCommand{ProjectID: p, Key: uuid.New(), RequestID: uuid.New(), Action: "create", RightsConfirmed: true, Sources: []scriptapp.SourceInput{{Kind: "chapter", Title: "完整正文", Status: "draft", OriginalHTML: &html, Document: scriptdomain.RichDocument{Type: "doc", Content: []scriptdomain.RichDocument{{Type: "paragraph", Content: []scriptdomain.RichDocument{{Type: "text", Text: "王总：你好😀"}}}}}}}})
	if e != nil {
		t.Fatal(e)
	}
	view, e := sources.Episodes(t.Context(), actor, p, saved.VersionID)
	if e != nil {
		t.Fatal(e)
	}
	formal, e := sources.ConfirmSplit(t.Context(), actor, scriptapp.SplitCommand{ProjectID: p, VersionID: saved.VersionID, ExpectedRevision: 1, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries, Key: uuid.New(), RequestID: uuid.New()}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	bible := app.NewService(pg.NewStore(db, pg.Factories{Access: func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, Media: func(tx *gorm.DB) app.MediaReferences {
		return actualBibleMedia{mediaapp.NewReferenceFactQuery(mediapg.NewLibraryStore(tx, bibleLibraryFactory, time.Now), objects)}
	}, Scopes: func(tx *gorm.DB) app.ScriptScopes {
		return currentBibleScopes{scriptpg.NewBibleScopes(tx, workspacepg.NewProjectContentAccessStore(tx))}
	}}), time.Now)
	first, e := bible.Change(t.Context(), actor, createCharacter(p))
	if e != nil {
		t.Fatal(e)
	}
	detail, e := bible.Find(t.Context(), actor, p, domain.KindCharacter, first.EntryID)
	if e != nil {
		t.Fatal(e)
	}
	look := detail.Current.Character.Looks[0].ID
	refs, e := bible.Change(t.Context(), actor, app.Command{ProjectID: p, Kind: domain.KindCharacter, Action: "references", EntryID: first.EntryID, ExpectedRevision: first.Revision, LookID: &look, References: []app.ReferenceInput{{Role: domain.RolePrimary, AssetID: imageID}, {Role: domain.RoleFront, AssetID: imageID}}, Key: uuid.New(), RequestID: uuid.New()})
	if e != nil {
		t.Fatal(e)
	}
	voice, e := bible.Change(t.Context(), actor, app.Command{ProjectID: p, Kind: domain.KindCharacter, Action: "voice_bind", EntryID: first.EntryID, ExpectedRevision: refs.Revision, Voice: &app.VoiceInput{Kind: domain.VoiceSample, Instructions: "历史声样指令", Sample: &app.SampleInput{Name: "旧声样", AssetID: audioID}}, Key: uuid.New(), RequestID: uuid.New()})
	if e != nil {
		t.Fatal(e)
	}
	confirmed, e := bible.Change(t.Context(), actor, app.Command{ProjectID: p, Kind: domain.KindCharacter, Action: "confirm", EntryID: first.EntryID, ExpectedRevision: voice.Revision, Key: uuid.New(), RequestID: uuid.New()})
	if e != nil {
		t.Fatal(e)
	}
	scene, line := uuid.New(), uuid.New()
	structure, e := sources.SaveStructure(t.Context(), actor, scriptapp.StructureCommand{ProjectID: p, EpisodeID: formal.Episodes[0].ID, ExpectedRevision: 2, ExpectedEpisodeRevision: 1, Document: scriptdomain.StructureDocument{Scenes: []scriptdomain.StructureScene{{Key: scene, SeqNo: 1, Heading: "客厅", Start: 0, End: 6, Items: []scriptdomain.StructureItem{{Type: "line", Key: line, Kind: "dialogue", Speaker: "王总", CharacterID: &first.EntryID, Content: "你好😀", Start: 0, End: 6}}}}}, Key: uuid.New(), RequestID: uuid.New()}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if _, e := sources.ConfirmStructure(t.Context(), actor, scriptapp.StructureCommand{ProjectID: p, EpisodeID: formal.Episodes[0].ID, ExpectedRevision: 3, ExpectedEpisodeRevision: 2, BaseStructureVersionNo: structure.VersionNo, Key: uuid.New(), RequestID: uuid.New()}, time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e := bible.Change(t.Context(), actor, app.Command{ProjectID: p, Kind: domain.KindCharacter, Action: "look_update", EntryID: first.EntryID, ExpectedRevision: confirmed.Revision, LookID: &look, Look: &app.LookInput{Name: "整集正式造型", Default: true, AppliesTo: []domain.LookScope{{EpisodeID: formal.Episodes[0].ID, SceneKey: &scene}}}, Key: uuid.New(), RequestID: uuid.New()}); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []domain.Kind{domain.KindLocation, domain.KindProp} {
		c := app.Command{ProjectID: p, Kind: kind, Action: "create", Key: uuid.New(), RequestID: uuid.New()}
		if kind == domain.KindLocation {
			c.Location = &domain.LocationContent{Name: "客厅", Prompt: "原始场景"}
		} else {
			c.Prop = &domain.PropContent{Name: "铜钥匙", Prompt: "原始道具"}
		}
		if _, e := bible.Change(t.Context(), actor, c); e != nil {
			t.Fatal(e)
		}
	}
	canvas := canvasapp.NewService(canvaspg.NewStore(db, func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil) }))
	doc, e := canvas.Create(t.Context(), actor, p, uuid.NewString(), canvasapp.CreateInput{Name: "完整画布"})
	if e != nil {
		t.Fatal(e)
	}
	text := "剧情😀"
	if _, e := canvas.Execute(t.Context(), actor, doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: 1, Commands: []canvasdomain.Command{{Type: "AddNodes", Nodes: []canvasdomain.Node{{ID: uuid.New(), Title: "原件资源", NodeType: "image", NodeAction: "resource", RefType: "media_asset", RefID: &imageID}, {ID: uuid.New(), Title: "剧情", NodeType: "text", NodeAction: "resource", Config: canvasdomain.NodeConfig{Text: &text}}}}}}); e != nil {
		t.Fatal(e)
	}
	return coordinatedContent{actor: actor, project: p, character: first.EntryID, pinnedVersion: voice.VersionID, canvas: doc.ID, image: imageID, audio: audioID}
}
func coordinatedAdmit(t *testing.T, db, owner *gorm.DB, content coordinatedContent) (*workspacepg.ProjectCopyStore, workspacedomain.ProjectCopyJob) {
	t.Helper()
	var rev int64
	if e := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, content.project).Scan(&rev).Error; e != nil {
		t.Fatal(e)
	}
	s := coordinatedWorkspaceCopy(db)
	j, e := s.Create(t.Context(), content.actor, workspaceapp.ProjectCopyInput{SourceProjectID: content.project, ExpectedRevision: rev, TargetName: "所有历史完整副本", IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}, time.Now())
	if e != nil {
		t.Fatal("actual whole admission", e)
	}
	return s, j
}

func TestBibleWorkspacePGFullActualOwnersAndFiveStagePublication(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	c := coordinatedContentFixture(t, db, owner, objects)
	store, j := coordinatedAdmit(t, db, owner, c)
	if j.Manifest.Bible == nil || j.Manifest.Script == nil || j.Manifest.Documents != 1 || j.Manifest.Assets != 2 || j.Manifest.Bible.Counts.CharacterVersions != 4 || j.Manifest.Script.Counts.DialogueLines != 1 {
		t.Fatal("complete admission missing content", j.Manifest)
	}
	var stages []string
	worker := coordinatedWorker(db, store, objects, func(j workspacedomain.ProjectCopyJob) { stages = append(stages, j.Stage) })
	final, e := worker.Execute(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: c.actor.OrgID, JobID: j.ID, WorkerID: uuid.New()})
	if e != nil || final.Status != "succeeded" || final.BibleReceipt == nil || final.ScriptReceipt == nil {
		t.Fatal("whole worker", final, e)
	}
	if !reflect.DeepEqual(stages, []string{"bible", "finalizing"}) || final.BibleReceipt.Counts != j.Manifest.Bible.Counts || final.ScriptReceipt.Counts != j.Manifest.Script.Counts {
		t.Fatal("complete stages/counts", stages, final)
	}
	var checkpoints []string
	if e := owner.Raw(`SELECT payload->'data'->>'stage' FROM infra.outbox WHERE partition_key=? AND topic='lanverse.workspace.project_copy_changed.v1' ORDER BY (payload->'data'->>'revision')::bigint`, j.ID.String()).Scan(&checkpoints).Error; e != nil || !reflect.DeepEqual(checkpoints, []string{"media", "bible", "script", "canvases", "finalizing", "complete"}) {
		t.Fatal("durable five-stage checkpoints", checkpoints, e)
	}
	var n int64
	if e := owner.Raw(`SELECT count(*) FROM bible.look_version l JOIN script.episode_structure st ON st.episode_id=(l.applies_to->0->>'episode_id')::uuid JOIN script.scene s ON s.episode_structure_id=st.id AND s.scene_key=(l.applies_to->0->>'scene_key')::uuid WHERE l.project_id=? AND st.project_id=?`, final.TargetProjectID, final.TargetProjectID).Scan(&n).Error; e != nil || n != 1 {
		t.Fatal("scope graph missing", n, e)
	}
	var line struct {
		CharacterID, VersionID uuid.UUID
		Content                string
		Start, End             int
	}
	if e := owner.Raw(`SELECT character_id,character_version_id AS version_id,content,span_start AS start,span_end AS end FROM script.dialogue_line WHERE project_id=?`, final.TargetProjectID).Scan(&line).Error; e != nil || line.CharacterID == c.character || line.VersionID == c.pinnedVersion || line.Content != "你好😀" || line.Start != 0 || line.End != 6 {
		t.Fatal("pinned source history lost", line, e)
	}
	var mediaRows int64
	if e := owner.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id=? AND status='ready' AND moderation_status='passed' AND NOT is_delete`, final.TargetProjectID).Scan(&mediaRows).Error; e != nil || mediaRows != 2 {
		t.Fatal("independent actual media missing", mediaRows, e)
	}
	if _, e := workspacepg.NewStore(db).FindProject(t.Context(), c.actor, final.TargetProjectID); e != nil {
		t.Fatal("published complete project unavailable", e)
	}
	var operations, budget int64
	if e := owner.Raw(`SELECT count(*) FROM operation.operation WHERE project_id=?`, final.TargetProjectID).Scan(&operations).Error; e != nil {
		t.Fatal(e)
	}
	if e := owner.Raw(`SELECT limit_micros+reserved_micros+settled_micros FROM billing.budget WHERE project_id=?`, final.TargetProjectID).Scan(&budget).Error; e != nil || operations != 0 || budget != 0 {
		t.Fatal("execution/financial facts copied", operations, budget, e)
	}
}

func TestBibleWorkspacePGRevokedCreatorMustLeaveRecoverableStoppedCopy(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	c := coordinatedContentFixture(t, db, owner, objects)
	store, j := coordinatedAdmit(t, db, owner, c)
	workerID := uuid.New()
	worker := coordinatedWorker(db, store, objects, func(current workspacedomain.ProjectCopyJob) {
		if current.Stage == "bible" {
			if e := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, c.actor.ID).Error; e != nil {
				t.Fatal(e)
			}
		}
	})
	final, executionErr := worker.Execute(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: c.actor.OrgID, JobID: j.ID, WorkerID: workerID})
	if final.Status == "succeeded" {
		t.Fatal("revoked actor must not publish")
	}
	var stopped struct {
		Status, Stage        string
		WorkerID             *uuid.UUID
		ExecutionUnconfirmed bool
	}
	if e := owner.Raw(`SELECT status,stage,worker_id,execution_unconfirmed FROM workspace.project_copy_job WHERE id=?`, j.ID).Scan(&stopped).Error; e != nil {
		t.Fatal(e)
	}
	var target string
	if e := owner.Raw(`SELECT status FROM workspace.project WHERE id=?`, j.TargetProjectID).Scan(&target).Error; e != nil || target != "copying" {
		t.Fatal("revocation published incomplete content", target, e)
	}
	if stopped.Status != "failed" || stopped.WorkerID != nil || stopped.ExecutionUnconfirmed {
		t.Fatalf("owned byte calls have ended but copy cannot recover: status=%s stage=%s worker_present=%v execution_unconfirmed=%v error=%v", stopped.Status, stopped.Stage, stopped.WorkerID != nil, stopped.ExecutionUnconfirmed, executionErr)
	}
}

func TestBibleWorkspacePGFinalizingMissingBytesReconcilesSameObjects(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	c := coordinatedContentFixture(t, db, owner, objects)
	store, j := coordinatedAdmit(t, db, owner, c)
	var key, mime string
	var exact []byte
	worker := coordinatedWorker(db, store, objects, func(current workspacedomain.ProjectCopyJob) {
		if current.Stage != "finalizing" {
			return
		}
		var target struct{ ObjectKey, MimeType string }
		if e := owner.Raw(`SELECT object_key,mime_type FROM media.media_asset WHERE project_id=? AND kind='image'`, current.TargetProjectID).Scan(&target).Error; e != nil || target.ObjectKey == "" {
			t.Fatal("private target", e)
		}
		key, mime = target.ObjectKey, target.MimeType
		opened, e := objects.Get(t.Context(), key)
		if e != nil {
			t.Fatal(e)
		}
		exact, e = io.ReadAll(io.LimitReader(opened, 1<<20))
		if closeErr := opened.Close(); e != nil || closeErr != nil {
			t.Fatal(e, closeErr)
		}
		if e := objects.Remove(t.Context(), key); e != nil {
			t.Fatal(e)
		}
	})
	failed, e := worker.Execute(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: c.actor.OrgID, JobID: j.ID, WorkerID: uuid.New()})
	if e != nil || failed.Status != "failed" || failed.Stage != "finalizing" || !failed.NeedsReconciliation || failed.Retryable || failed.WorkerID != uuid.Nil || failed.ExecutionUnconfirmed {
		t.Fatal("unknown private bytes must fence publication after physical worker stopped", failed, e)
	}
	if _, e := workspacepg.NewStore(db).FindProject(t.Context(), c.actor, j.TargetProjectID); e == nil {
		t.Fatal("incomplete target visible")
	}
	if e := objects.Put(t.Context(), key, bytes.NewReader(exact), int64(len(exact)), mime); e != nil {
		t.Fatal(e)
	}
	requested, e := store.Change(t.Context(), c.actor, j.ID, "reconcile", failed.Revision, uuid.New(), uuid.NewString())
	if e != nil || !requested.ReconciliationRequested {
		t.Fatal("explicit reconciliation", requested, e)
	}
	final, e := coordinatedWorker(db, store, objects, nil).Execute(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: c.actor.OrgID, JobID: j.ID, WorkerID: uuid.New()})
	beforeJSON, beforeErr := json.Marshal(j.Manifest)
	afterJSON, afterErr := json.Marshal(final.Manifest)
	if e != nil || final.Status != "succeeded" || final.Attempt != failed.Attempt || beforeErr != nil || afterErr != nil || !bytes.Equal(beforeJSON, afterJSON) {
		t.Fatal("same frozen manifest/objects reconciliation", final, e, beforeErr, afterErr)
	}
}

func TestBibleWorkspacePGQueuedAndActiveCancellationRetainSourceHistory(t *testing.T) {
	for _, duringExecution := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "after_all_content_registered"}[duringExecution], func(t *testing.T) {
			db, owner := bibleTestDB(t)
			objects := bibleObjects(t)
			c := coordinatedContentFixture(t, db, owner, objects)
			store, j := coordinatedAdmit(t, db, owner, c)
			requested := false
			cancel := func(current workspacedomain.ProjectCopyJob) {
				if requested {
					return
				}
				accepted, e := store.Change(t.Context(), c.actor, j.ID, "cancel", current.Revision, uuid.New(), uuid.NewString())
				if e != nil || !accepted.CancellationRequested || accepted.Status != "cancel_requested" {
					t.Fatal("cancel only accepts intent", accepted, e)
				}
				requested = true
			}
			if !duringExecution {
				cancel(j)
			}
			worker := coordinatedWorker(db, store, objects, func(current workspacedomain.ProjectCopyJob) {
				if duringExecution && current.Stage == "finalizing" {
					cancel(current)
				}
			})
			final, e := worker.Execute(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: c.actor.OrgID, JobID: j.ID, WorkerID: uuid.New()})
			if e != nil || final.Status != "cancelled" || final.WorkerID != uuid.Nil || final.ExecutionUnconfirmed || final.NeedsReconciliation {
				t.Fatal("known cleanup failed", final, e)
			}
			if _, e := workspacepg.NewStore(db).FindProject(t.Context(), c.actor, j.TargetProjectID); e == nil {
				t.Fatal("cancelled target exposed")
			}
			var keys []string
			if e := owner.Raw(`SELECT target_object_key FROM media.project_copy_object WHERE snapshot_id=? UNION ALL SELECT target_key FROM script.copy_object_intent WHERE snapshot_id=?`, j.Manifest.MediaSnapshotID, j.Manifest.Script.ID).Scan(&keys).Error; e != nil {
				t.Fatal(e)
			}
			for _, key := range keys {
				if _, e := objects.Stat(t.Context(), key); e == nil {
					t.Fatal("owned target object remains after cancelled")
				}
			}
			var preserved int64
			if e := owner.Raw(`SELECT count(*) FROM bible.character_version WHERE entry_id=?`, c.character).Scan(&preserved).Error; e != nil || preserved != 4 {
				t.Fatal("source Bible changed", preserved, e)
			}
			if _, e := mediaapp.NewAssetQuery(mediapg.NewStore(db), objects).Reference(t.Context(), c.actor, c.project, c.image); e != nil {
				t.Fatal("source original lost", e)
			}
		})
	}
}

func TestBibleWorkspacePGStaleWorkerCannotRegisterBible(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	c := coordinatedContentFixture(t, db, owner, objects)
	store, j := coordinatedAdmit(t, db, owner, c)
	workerID := uuid.New()
	claimed, e := store.Claim(t.Context(), c.actor, j.ID, workerID, false, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	binding := mediaapp.ProjectCopyBinding{JobID: j.ID, OrgID: j.OrgID, SourceProjectID: j.SourceProjectID, TargetProjectID: j.TargetProjectID}
	snapshot := mediaapp.ProjectCopySnapshot{ID: j.Manifest.MediaSnapshotID, ManifestSHA256: j.Manifest.MediaSHA256, Assets: j.Manifest.Assets, Renditions: j.Manifest.Renditions}
	transfer := mediaapp.NewProjectCopyTransfer(workspacepg.NewProjectCopyMediaObjects(store, j.ID, workerID, false), mediaobjects.NewProjectCopyObjects(objects), "")
	if e := transfer.Transfer(t.Context(), c.actor, binding, snapshot); e != nil {
		t.Fatal(e)
	}
	current, e := store.CompleteMedia(t.Context(), c.actor, j.ID, workerID)
	if e != nil || current.Stage != "bible" {
		t.Fatal("Bible stage", current, e)
	}
	if _, e := store.CompleteBible(t.Context(), c.actor, j.ID, uuid.New()); !errors.Is(e, workspacedomain.ErrProjectCopyWorkerConflict) {
		t.Fatal("stale worker accepted", e)
	}
	after, e := store.Find(t.Context(), c.actor, j.ID)
	if e != nil || after.Revision != current.Revision || after.BibleReceipt != nil || after.Stage != "bible" || after.WorkerID != claimed.WorkerID {
		t.Fatal("stale worker changed history", after, e)
	}
	if _, e := store.Change(t.Context(), c.actor, j.ID, "cancel", after.Revision, uuid.New(), uuid.NewString()); e != nil {
		t.Fatal(e)
	}
	final, e := store.Find(t.Context(), c.actor, j.ID)
	if e != nil {
		t.Fatal(e)
	}
	cleanup := mediaapp.NewProjectCopyTransfer(workspacepg.NewProjectCopyMediaObjects(store, j.ID, workerID, true), mediaobjects.NewProjectCopyObjects(objects), "")
	if e := cleanup.Cleanup(t.Context(), c.actor, binding, snapshot); e != nil {
		t.Fatal(e)
	}
	script := scriptapp.NewProjectCopy(coordinatedScriptStore(db, coordinatedAuthority(final, workerID, "cleanup")), scriptobjects.NewStorage(objects))
	if e := script.Cleanup(t.Context(), c.actor, scriptapp.ProjectCopyBinding(binding), ownCoordinatedScriptSnapshot(*j.Manifest.Script)); e != nil {
		t.Fatal(e)
	}
	if e := db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return coordinatedBibleStore(tx, coordinatedAuthority(final, workerID, "cleanup"), snapshot).Cleanup(t.Context(), c.actor, app.ProjectCopyBinding(binding), ownCoordinatedBibleSnapshot(*j.Manifest.Bible))
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := store.FinishCancelled(t.Context(), c.actor, j.ID, workerID); e != nil {
		t.Fatal(e)
	}
}

func TestBibleWorkspacePGCurrentAdminCancelMustRecoverDisabledQueuedCreator(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	c := coordinatedContentFixture(t, db, owner, objects)
	store, j := coordinatedAdmit(t, db, owner, c)
	admin := c.actor
	admin.ID, admin.Role = uuid.New(), "admin"
	if e := owner.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password) VALUES(?,?,?,?,?,'admin','active',false)`, admin.ID, admin.OrgID, "bible-copy-admin-"+admin.ID.String(), "复制恢复测试", "synthetic-test-not-a-credential").Error; e != nil {
		t.Fatal(e)
	}
	if e := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, c.actor.ID).Error; e != nil {
		t.Fatal(e)
	}
	accepted, e := store.Change(t.Context(), admin, j.ID, "cancel", j.Revision, uuid.New(), uuid.NewString())
	if e != nil || !accepted.CancellationRequested {
		t.Fatal("current admin cancel must be durable", accepted, e)
	}
	final, e := coordinatedWorker(db, store, objects, nil).Execute(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: c.actor.OrgID, JobID: j.ID, WorkerID: uuid.New()})
	if e != nil || final.Status != "cancelled" || final.WorkerID != uuid.Nil || final.NeedsReconciliation || final.ExecutionUnconfirmed {
		t.Fatal("permanent authorized admin cancel remains unconsumable after creator disabled", final.Status, e)
	}
	if _, e := workspacepg.NewStore(db).FindProject(t.Context(), admin, j.TargetProjectID); e == nil {
		t.Fatal("admin cancellation published partial target")
	}
}

func TestBibleWorkspacePGExecutionAuthorityCannotBeBorrowed(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	c := coordinatedContentFixture(t, db, owner, objects)
	store, j := coordinatedAdmit(t, db, owner, c)
	admin := c.actor
	admin.ID, admin.Role = uuid.New(), "admin"
	if e := owner.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password) VALUES(?,?,?,?,?,'admin','active',false)`, admin.ID, admin.OrgID, "bible-copy-admin-"+admin.ID.String(), "复制边界测试", "synthetic-test-not-a-credential").Error; e != nil {
		t.Fatal(e)
	}
	workerID := uuid.New()
	if _, e := store.Claim(t.Context(), c.actor, j.ID, workerID, false, time.Now()); e != nil {
		t.Fatal(e)
	}
	binding := mediaapp.ProjectCopyBinding{JobID: j.ID, OrgID: j.OrgID, SourceProjectID: j.SourceProjectID, TargetProjectID: j.TargetProjectID}
	snapshot := mediaapp.ProjectCopySnapshot{ID: j.Manifest.MediaSnapshotID, ManifestSHA256: j.Manifest.MediaSHA256, Assets: j.Manifest.Assets, Renditions: j.Manifest.Renditions}
	if e := mediaapp.NewProjectCopyTransfer(workspacepg.NewProjectCopyMediaObjects(store, j.ID, workerID, false), mediaobjects.NewProjectCopyObjects(objects), "").Transfer(t.Context(), c.actor, binding, snapshot); e != nil {
		t.Fatal(e)
	}
	current, e := store.CompleteMedia(t.Context(), c.actor, j.ID, workerID)
	if e != nil {
		t.Fatal(e)
	}
	forged := coordinatedAuthority(current, workerID, "bible_transfer")
	forged.ActorID = admin.ID
	if _, e := coordinatedBibleStore(db, forged, snapshot).Manifest(t.Context(), admin, coordinatedCopyBibleBinding(forged.Binding), ownCoordinatedBibleSnapshot(*j.Manifest.Bible)); e == nil {
		t.Fatal("current admin borrowed worker without a permanent control command")
	}
	if _, e := store.CompleteBible(t.Context(), admin, j.ID, workerID); e == nil {
		t.Fatal("foreign actor registered content under old worker")
	}
	accepted, e := store.Change(t.Context(), admin, j.ID, "cancel", current.Revision, uuid.New(), uuid.NewString())
	if e != nil || accepted.ExecutionActor() != admin.ID {
		t.Fatal("exact permanent cancellation does not select current controller", accepted, e)
	}
	if _, e := store.CompleteBible(t.Context(), c.actor, j.ID, workerID); e == nil {
		t.Fatal("old actor published through new controller")
	}
	if _, e := store.Publish(t.Context(), c.actor, j.ID, workerID); e == nil {
		t.Fatal("old worker published after controller switch")
	}
	oldAuthority := coordinatedAuthority(current, workerID, "cleanup")
	if e := db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return coordinatedBibleStore(tx, oldAuthority, snapshot).Cleanup(t.Context(), c.actor, app.ProjectCopyBinding(binding), ownCoordinatedBibleSnapshot(*j.Manifest.Bible))
	}); e == nil {
		t.Fatal("old creator borrowed permanent cancellation authority")
	}
	stopped, e := store.StopAttempt(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: j.OrgID, JobID: j.ID, WorkerID: workerID}, "copy_worker_stopped", false, false)
	if e != nil || stopped.WorkerID != uuid.Nil {
		t.Fatal("exact stopped attempt remains claimed", stopped, e)
	}
	requested, e := store.Change(t.Context(), admin, j.ID, "reconcile", stopped.Revision, uuid.New(), uuid.NewString())
	if e != nil || !requested.ReconciliationRequested {
		t.Fatal("cancelled stopped attempt cannot reconcile", requested, e)
	}
	final, e := coordinatedWorker(db, store, objects, nil).Execute(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: j.OrgID, JobID: j.ID, WorkerID: uuid.New()})
	if e != nil || final.Status != "cancelled" {
		t.Fatal("current controller cannot complete own cleanup", final, e)
	}
	before, e := store.Find(t.Context(), admin, j.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := store.StopAttempt(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: j.OrgID, JobID: j.ID, WorkerID: workerID}, "copy_worker_stopped", false, false); !errors.Is(e, workspacedomain.ErrProjectCopyWorkerConflict) {
		t.Fatal("old stopped worker reused terminal attempt", e)
	}
	after, e := store.Find(t.Context(), admin, j.ID)
	if e != nil || after.Revision != before.Revision || after.Status != "cancelled" {
		t.Fatal("old stopped worker changed terminal result", after, e)
	}
}

func TestBibleWorkspacePGAdminCannotUseRetryOrGeneralReconcileToPublish(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry", true: "reconcile"}[unknown], func(t *testing.T) {
			db, owner := bibleTestDB(t)
			objects := bibleObjects(t)
			c := coordinatedContentFixture(t, db, owner, objects)
			store, j := coordinatedAdmit(t, db, owner, c)
			admin := c.actor
			admin.ID, admin.Role = uuid.New(), "admin"
			if e := owner.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password) VALUES(?,?,?,?,?,'admin','active',false)`, admin.ID, admin.OrgID, "bible-copy-admin-"+admin.ID.String(), "复制发布边界", "synthetic-test-not-a-credential").Error; e != nil {
				t.Fatal(e)
			}
			workerID := uuid.New()
			if _, e := store.Claim(t.Context(), c.actor, j.ID, workerID, false, time.Now()); e != nil {
				t.Fatal(e)
			}
			failed, e := store.StopAttempt(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: j.OrgID, JobID: j.ID, WorkerID: workerID}, "copy_test_stopped", !unknown, unknown)
			if e != nil {
				t.Fatal(e)
			}
			action := map[bool]string{false: "retry", true: "reconcile"}[unknown]
			if _, e := store.Change(t.Context(), admin, j.ID, action, failed.Revision, uuid.New(), uuid.NewString()); e == nil {
				t.Fatal("admin borrowed publication through " + action)
			}
			after, e := store.Find(t.Context(), c.actor, j.ID)
			if e != nil || after.Revision != failed.Revision || after.ExecutionActor() != c.actor.ID || after.ReconciliationRequested {
				t.Fatal("rejected controller altered task", after, e)
			}
		})
	}
}

func TestBibleWorkspacePGLegacyNilBibleManifestAndReceiptRemainAbsent(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	actor, pid := bibleActorProject(t, owner)
	legacy := workspacepg.NewProjectCopyStoreWithScript(db, func(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
		return scriptpg.NewSourceStore(tx, func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) })
	}, coordinatedCopyOwners, func(tx *gorm.DB, a workspaceapp.ProjectCopyAuthority) workspaceapp.ProjectCopyScriptOwner {
		return coordinatedScriptOwner{coordinatedScriptStore(tx, a)}
	})
	input := workspaceapp.ProjectCopyInput{SourceProjectID: pid, ExpectedRevision: 1, TargetName: "旧nil内容合同", IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
	j, e := legacy.Create(t.Context(), actor, input, time.Now())
	if e != nil || j.Manifest.Bible != nil {
		t.Fatal("legacy admission", j, e)
	}
	before, e := json.Marshal(j)
	if e != nil {
		t.Fatal(e)
	}
	current := coordinatedWorkspaceCopy(db)
	replayed, e := current.Create(t.Context(), actor, input, time.Now())
	after, marshalErr := json.Marshal(replayed)
	if e != nil || marshalErr != nil || !bytes.Equal(before, after) {
		t.Fatal("new owner changed permanent old receipt", e, marshalErr)
	}
	called := false
	final, e := coordinatedWorker(db, current, objects, func(workspacedomain.ProjectCopyJob) { called = true }).Execute(t.Context(), workspaceapp.ProjectCopyWorkID{OrgID: actor.OrgID, JobID: j.ID, WorkerID: uuid.New()})
	if e != nil || final.Status != "succeeded" || called || final.Manifest.Bible != nil || final.BibleReceipt != nil {
		t.Fatal("legacy job invented Bible content", final, e, called)
	}
	wire, e := json.Marshal(final.Manifest)
	if e != nil || bytes.Contains(wire, []byte(`"bible"`)) {
		t.Fatal("nil Bible JSON changed", e)
	}
}
