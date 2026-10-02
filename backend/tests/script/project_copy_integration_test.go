package script_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingpg "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	scriptpg "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type actualScriptCopyAccess struct {
	owner *workspacepg.ProjectCopyAccessStore
}

func (a actualScriptCopyAccess) Authorize(ctx context.Context, actor identityapp.Principal, b app.ProjectCopyBinding, target bool) error {
	return a.owner.Authorize(ctx, actor, workspaceapp.ProjectCopyBinding(b), target)
}
func actualScriptCopyStore(db *gorm.DB, authority workspaceapp.ProjectCopyAuthority) *scriptpg.ProjectCopyStore {
	return scriptpg.NewProjectCopyStore(db, func(tx *gorm.DB) app.ProjectCopyAccess {
		return actualScriptCopyAccess{workspacepg.NewProjectCopyAccessStore(tx, authority)}
	})
}

type actualScriptHistoryOwner struct{ store *scriptpg.ProjectCopyStore }

func ownScriptSnapshot(s workspacedomain.ProjectCopyScriptSnapshot) app.ProjectCopySnapshot {
	return app.ProjectCopySnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: app.ScriptCopyCounts(s.Counts)}
}
func (o actualScriptHistoryOwner) ReferencedMedia(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding) ([]uuid.UUID, error) {
	return o.store.ReferencedMedia(ctx, a, app.ProjectCopyBinding(b))
}
func (o actualScriptHistoryOwner) Freeze(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, m map[uuid.UUID]uuid.UUID, now time.Time) (workspacedomain.ProjectCopyScriptSnapshot, error) {
	s, err := o.store.Freeze(ctx, a, app.ProjectCopyBinding(b), m, now)
	return workspacedomain.ProjectCopyScriptSnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: workspacedomain.ProjectCopyScriptCounts(s.Counts)}, err
}
func (o actualScriptHistoryOwner) Register(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) (workspacedomain.ProjectCopyScriptReceipt, error) {
	r, err := o.store.Register(ctx, a, app.ProjectCopyBinding(b), ownScriptSnapshot(s))
	return workspacedomain.ProjectCopyScriptReceipt{ManifestSHA256: r.ManifestSHA256, ContentSHA256: r.ContentSHA256, Counts: workspacedomain.ProjectCopyScriptCounts(r.Counts)}, err
}
func (o actualScriptHistoryOwner) FinishCleanup(ctx context.Context, a identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) error {
	return o.store.FinishCleanup(ctx, a, app.ProjectCopyBinding(b), ownScriptSnapshot(s))
}
func workspaceWithActualScript(db *gorm.DB) *workspacepg.ProjectCopyStore {
	return workspacepg.NewProjectCopyStoreWithScript(db, func(tx *gorm.DB) workspaceapp.ProjectWorkGuard { return scriptStore(tx) }, func(tx *gorm.DB) workspaceapp.ProjectCopyOwners {
		return workspaceapp.ProjectCopyOwners{Media: mediapg.NewProjectCopyStore(tx), Budget: billingpg.NewStore(tx), Canvas: canvaspg.NewProjectCopyStore(tx, func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil) }, nil)}
	}, func(tx *gorm.DB, a workspaceapp.ProjectCopyAuthority) workspaceapp.ProjectCopyScriptOwner {
		return actualScriptHistoryOwner{actualScriptCopyStore(tx, a)}
	})
}
func historyForCopy(t *testing.T, db, owner *gorm.DB, objects app.PrivateObjects) (identityapp.Principal, uuid.UUID) {
	t.Helper()
	actor, pid := scriptActorProject(t, owner)
	store := scriptStore(db)
	sources := app.NewSourceService(store, objects, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	input.Sources[0].Document = domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "走进客厅\n你好😀 "}}}}}
	html := "<p>走进客厅\n你好😀 </p>"
	input.Sources[0].OriginalHTML = &html
	saved, err := sources.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.Episodes(t.Context(), actor, pid, saved.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := store.ConfirmSplit(t.Context(), actor, app.SplitCommand{ProjectID: pid, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 1, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	episode := formal.Episodes[0]
	episodes := app.NewEpisodeService(store, sources, time.Now)
	command := app.StructureCommand{ProjectID: pid, EpisodeID: episode.ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 2, ExpectedEpisodeRevision: 1, Document: manualStructure()}
	first, err := episodes.SaveStructure(t.Context(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	command.Key = uuid.New()
	command.ExpectedRevision = 3
	command.ExpectedEpisodeRevision = 2
	command.BaseStructureVersionNo = first.VersionNo
	command.Document.Scenes[0].Items[1].Content = "旧稿保留😀"
	if _, err := episodes.SaveStructure(t.Context(), actor, command); err != nil {
		t.Fatal(err)
	}
	input.Key = uuid.New()
	input.RequestID = uuid.New()
	input.Action = "update"
	input.ExpectedRevision = 4
	input.BaseVersionID = &saved.VersionID
	lineage := saved.Mappings[0].LineageID
	input.LineageID = &lineage
	input.Sources[0].Title = "第二稿"
	input.Sources[0].OriginalHTML = nil
	input.Sources[0].Document.Content[0].Content[0].Marks = []domain.RichMark{{Type: "bold"}}
	if _, err := sources.Write(t.Context(), actor, input); err != nil {
		t.Fatal(err)
	}
	return actor, pid
}
func startActualScriptCopy(t *testing.T, db, owner *gorm.DB, actor identityapp.Principal, pid uuid.UUID) (*workspacepg.ProjectCopyStore, workspacedomain.ProjectCopyJob, uuid.UUID) {
	t.Helper()
	var revision int64
	if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, pid).Scan(&revision).Error; err != nil {
		t.Fatal(err)
	}
	store := workspaceWithActualScript(db)
	job, err := store.Create(t.Context(), actor, workspaceapp.ProjectCopyInput{SourceProjectID: pid, ExpectedRevision: revision, TargetName: "完整剧本副本", IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}, time.Now().UTC())
	if err != nil {
		t.Fatal("own freeze", err)
	}
	worker := uuid.New()
	if _, err := store.Claim(t.Context(), actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	job, err = store.CompleteMedia(t.Context(), actor, job.ID, worker)
	if err != nil || job.Stage != "script" {
		t.Fatal(job.Stage, err)
	}
	return store, job, worker
}
func scriptCopyTransfer(db *gorm.DB, job workspacedomain.ProjectCopyJob, worker uuid.UUID, cleanup bool, objects app.PrivateObjects) (*app.ProjectCopy, app.ProjectCopyBinding, app.ProjectCopySnapshot) {
	b := app.ProjectCopyBinding{JobID: job.ID, OrgID: job.OrgID, SourceProjectID: job.SourceProjectID, TargetProjectID: job.TargetProjectID}
	phase := "transfer"
	if cleanup {
		phase = "cleanup"
	}
	a := workspaceapp.ProjectCopyAuthority{Binding: workspaceapp.ProjectCopyBinding(b), ActorID: job.ActorID, WorkerID: worker, SourceRevision: job.SourceRevision, Phase: phase}
	return app.NewProjectCopy(actualScriptCopyStore(db, a), objects), b, ownScriptSnapshot(*job.Manifest.Script)
}
func TestScriptCopyPGPrivateObjectsAllHistoricalRowsAndReplay(t *testing.T) {
	db, owner := scriptTestDB(t)
	objects := scriptobjects.NewStorage(scriptStorage(t))
	actor, pid := historyForCopy(t, db, owner, objects)
	store, job, worker := startActualScriptCopy(t, db, owner, actor, pid)
	counts := job.Manifest.Script.Counts
	if counts.Sources != 2 || counts.Versions != 2 || counts.VersionSources != 2 || counts.SplitConfirmations != 1 || counts.Structures != 2 || counts.Scenes != 2 || counts.DialogueLines != 2 || counts.ActionLines != 2 || counts.Objects < 6 {
		t.Fatal("not full own history", counts)
	}
	transfer, b, snapshot := scriptCopyTransfer(db, job, worker, false, objects)
	if _, err := transfer.Transfer(t.Context(), actor, b, snapshot); err != nil {
		t.Fatal("actual private objects", err)
	}
	if _, err := store.CompleteScript(t.Context(), actor, job.ID, worker); err != nil {
		t.Fatal("register all historical rows", err)
	}
	if _, err := scriptStore(db).LoadBase(t.Context(), actor, job.TargetProjectID, nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("copying content exposed", err)
	}
	if _, err := store.CompleteCanvases(t.Context(), actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	job, err := store.Publish(t.Context(), actor, job.ID, worker)
	if err != nil || job.Status != "succeeded" {
		t.Fatal("actual complete history publication", job.Status, err)
	}
	copied, err := scriptStore(db).LoadBase(t.Context(), actor, job.TargetProjectID, nil)
	if err != nil || len(copied.Sources) != 1 || copied.Sources[0].Title != "第二稿" {
		t.Fatal("published draft", copied, err)
	}
	versions, err := app.NewHistoryService(scriptStore(db), app.NewSourceService(scriptStore(db), objects, time.Now)).Versions(t.Context(), actor, job.TargetProjectID, 0, 100)
	if err != nil || len(versions.Items) != 2 {
		t.Fatal("old history lost", versions, err)
	}
	if err := db.Exec(`UPDATE script.copy_snapshot SET content_sha256=? WHERE id=?`, domain.ContentSHA([]byte("illegal")), snapshot.ID).Error; err == nil {
		t.Fatal("snapshot runtime UPDATE")
	}
	if err := db.Exec(`DELETE FROM script.copy_object_intent WHERE snapshot_id=?`, snapshot.ID).Error; err == nil {
		t.Fatal("object ownership runtime DELETE")
	}
}

func TestScriptCopyPGCorruptSourceManifestRejectsBeforeSnapshotOrTarget(t *testing.T) {
	for _, mutation := range []string{"hash", "metadata"} {
		t.Run(mutation, func(t *testing.T) {
			db, owner := scriptTestDB(t)
			objects := scriptobjects.NewStorage(scriptStorage(t))
			actor, pid := historyForCopy(t, db, owner, objects)
			if mutation == "hash" {
				if err := owner.Exec(`UPDATE script.script_version SET source_manifest_sha256=? WHERE project_id=? AND version_no=1`, domain.ContentSHA([]byte("corrupt")), pid).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := owner.Exec(`UPDATE script.script_source SET title='tampered metadata' WHERE project_id=? AND source_revision=1`, pid).Error; err != nil {
					t.Fatal(err)
				}
			}
			var revision int64
			if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, pid).Scan(&revision).Error; err != nil {
				t.Fatal(err)
			}
			_, err := workspaceWithActualScript(db).Create(t.Context(), actor, workspaceapp.ProjectCopyInput{SourceProjectID: pid, ExpectedRevision: revision, TargetName: "不能接受篡改", IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}, time.Now())
			if !errors.Is(err, app.ErrObjectMismatch) {
				t.Fatal("source hash trusted", err)
			}
			var counts struct{ Snapshots, Objects, Targets int64 }
			if err := owner.Raw(`SELECT (SELECT count(*) FROM script.copy_snapshot WHERE org_id=?) AS snapshots,(SELECT count(*) FROM script.copy_object_intent i JOIN script.copy_snapshot s ON s.id=i.snapshot_id WHERE s.org_id=?) AS objects,(SELECT count(*) FROM workspace.project WHERE org_id=? AND status='copying') AS targets`, actor.OrgID, actor.OrgID, actor.OrgID).Scan(&counts).Error; err != nil || counts.Snapshots+counts.Objects+counts.Targets != 0 {
				t.Fatal("rejected freeze wrote", counts, err)
			}
		})
	}
}

func TestScriptCopyPGWorkerFenceUnknownPutAndActualPrivateCleanup(t *testing.T) {
	db, owner := scriptTestDB(t)
	objects := scriptobjects.NewStorage(scriptStorage(t))
	actor, pid := historyForCopy(t, db, owner, objects)
	store, job, worker := startActualScriptCopy(t, db, owner, actor, pid)
	stale, b, snapshot := scriptCopyTransfer(db, job, uuid.New(), false, objects)
	if _, err := stale.Transfer(t.Context(), actor, b, snapshot); !errors.Is(err, workspacedomain.ErrProjectCopyWorkerConflict) {
		t.Fatal("stale worker admitted", err)
	}
	var started int64
	if err := owner.Raw(`SELECT count(*) FROM script.copy_object_state s JOIN script.copy_object_intent i ON i.target_key=s.target_key WHERE i.snapshot_id=? AND s.put_started`, snapshot.ID).Scan(&started).Error; err != nil || started != 0 {
		t.Fatal("stale worker wrote intent", started, err)
	}
	loss := neverScriptPut{PrivateObjects: objects}
	transfer, _, _ := scriptCopyTransfer(db, job, worker, false, loss)
	if _, err := transfer.Transfer(t.Context(), actor, b, snapshot); err == nil {
		t.Fatal("unknown put became success")
	} else {
		var fact *app.ProjectCopyTransferError
		if !errors.As(err, &fact) || !fact.NeedsReconciliation {
			t.Fatal("unknown lost", err)
		}
	}
	job, err := store.Find(t.Context(), actor, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	job, err = store.Change(t.Context(), actor, job.ID, "cancel", job.Revision, uuid.New(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	cleanup, _, _ := scriptCopyTransfer(db, job, worker, true, objects)
	if err := cleanup.Cleanup(t.Context(), actor, b, snapshot); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal("missing unknown put was treated as stopped", err)
	}
	if _, err := store.FinishCancelled(t.Context(), actor, job.ID, worker); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal("unknown own objects released fence", err)
	}
	authority := workspaceapp.ProjectCopyAuthority{Binding: workspaceapp.ProjectCopyBinding(b), ActorID: actor.ID, WorkerID: worker, SourceRevision: job.SourceRevision, Phase: "cleanup"}
	manifest, err := actualScriptCopyStore(db, authority).Manifest(t.Context(), actor, b, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	object := manifest.Objects[0]
	sourceReader, err := objects.Get(t.Context(), object.Source.Key)
	if err != nil {
		t.Fatal(err)
	}
	if err := objects.PutIfAbsent(t.Context(), object.Target.Key, sourceReader, object.Target.ByteSize, object.Target.MIME, object.Target.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := sourceReader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.Cleanup(t.Context(), actor, b, snapshot); err != nil {
		t.Fatal("same frozen object appeared; explicit exact cleanup", err)
	}
	if _, err := store.FinishCancelled(t.Context(), actor, job.ID, worker); err != nil {
		t.Fatal("actual absence proof", err)
	}
	reader, err := objects.Get(t.Context(), object.Source.Key)
	if err != nil {
		t.Fatal("source bytes removed", err)
	}
	data, err := io.ReadAll(reader)
	if closeErr := reader.Close(); err != nil || closeErr != nil || domain.ContentSHA(data) != object.Source.SHA256 {
		t.Fatal("original private bytes changed", err, closeErr)
	}
}

func TestScriptCopyPGTransferRechecksCanonicalSourceSemanticsBeforeTargetPut(t *testing.T) {
	db, owner := scriptTestDB(t)
	objects := scriptobjects.NewStorage(scriptStorage(t))
	actor, pid := historyForCopy(t, db, owner, objects)
	base, err := scriptStore(db).LoadBase(t.Context(), actor, pid, nil)
	if err != nil {
		t.Fatal(err)
	}
	altered := domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "与冻结原文不同"}}}}}
	body, err := altered.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	key := "projects/" + pid.String() + "/script/" + uuid.NewString() + "/corruption-test.json"
	sha := domain.ContentSHA(body)
	if err := objects.PutIfAbsent(t.Context(), key, bytes.NewReader(body), int64(len(body)), "application/json", sha); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE script.script_source SET rich_key=?,rich_sha256=?,rich_bytes=? WHERE id=?`, key, sha, len(body), base.Sources[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	base, err = scriptStore(db).LoadBase(t.Context(), actor, pid, nil)
	if err != nil {
		t.Fatal(err)
	}
	records, err := json.Marshal(base.Sources)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE script.script_version SET source_manifest_sha256=? WHERE id=?`, domain.ContentSHA(records), base.Version.ID).Error; err != nil {
		t.Fatal(err)
	}
	_, job, worker := startActualScriptCopy(t, db, owner, actor, pid)
	transfer, b, snapshot := scriptCopyTransfer(db, job, worker, false, objects)
	if _, err := transfer.Transfer(t.Context(), actor, b, snapshot); !errors.Is(err, app.ErrObjectMismatch) {
		t.Fatal("rich hash alone accepted wrong canonical plain content", err)
	}
	var started int64
	if err := owner.Raw(`SELECT count(*) FROM script.copy_object_state s JOIN script.copy_object_intent i ON i.target_key=s.target_key WHERE i.snapshot_id=? AND s.put_started`, snapshot.ID).Scan(&started).Error; err != nil || started != 0 {
		t.Fatal("semantic validation occurred after target write", started, err)
	}
}

func TestScriptCopyPGReferencedMediaIncludesOldHistoryWithTrustedAdmissionOnly(t *testing.T) {
	db, owner := scriptTestDB(t)
	objects := scriptobjects.NewStorage(scriptStorage(t))
	actor, pid := historyForCopy(t, db, owner, objects)
	asset := uuid.New()
	sha := domain.ContentSHA([]byte("file owning reference fixture"))
	if err := owner.Exec(`UPDATE script.script_source SET origin='file',media_asset_id=?,media_revision=1,media_sha256=? WHERE project_id=?`, asset, sha, pid).Error; err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, pid).Scan(&revision).Error; err != nil {
		t.Fatal(err)
	}
	b := app.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: pid, TargetProjectID: uuid.New()}
	authority := workspaceapp.ProjectCopyAuthority{Binding: workspaceapp.ProjectCopyBinding(b), ActorID: actor.ID, SourceRevision: revision, Phase: "freeze"}
	if _, err := actualScriptCopyStore(db, authority).ReferencedMedia(t.Context(), actor, b); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("pool mimicked owning admission", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,name,description,aspect_ratio,style_type,resolution,status,revision,default_models) VALUES(?,?,'own reference target','','16:9','realistic','1080p','copying',1,'{}')`, b.TargetProjectID, b.OrgID).Error; err != nil {
			return err
		}
		refs, err := actualScriptCopyStore(tx, authority).ReferencedMedia(t.Context(), actor, b)
		if err != nil || len(refs) != 1 || refs[0] != asset {
			t.Fatal("historical file references and dedup", refs, err)
		}
		var snapshots int64
		if err := tx.Table("script.copy_snapshot").Where("job_id=?", b.JobID).Count(&snapshots).Error; err != nil || snapshots != 0 {
			t.Fatal("query published snapshot", snapshots, err)
		}
		altered := b
		altered.SourceProjectID = uuid.New()
		if _, err := actualScriptCopyStore(tx, authority).ReferencedMedia(t.Context(), actor, altered); err == nil {
			t.Fatal("unminted binding accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := actualScriptCopyStore(tx, authority).ReferencedMedia(t.Context(), actor, b)
		return err
	}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("old creator authority accepted", err)
	}
}

type alteredCopyManifest struct {
	*scriptpg.ProjectCopyStore
	alter func(*app.ProjectCopyManifest)
}

func (s alteredCopyManifest) Manifest(ctx context.Context, a identityapp.Principal, b app.ProjectCopyBinding, snapshot app.ProjectCopySnapshot) (app.ProjectCopyManifest, error) {
	m, err := s.ProjectCopyStore.Manifest(ctx, a, b, snapshot)
	if err == nil {
		s.alter(&m)
	}
	return m, err
}

func TestScriptCopyPGConsumerRejectsVersionTextAndRichObjectBeforeAnyTargetPut(t *testing.T) {
	for _, kind := range []string{"text", "rich"} {
		t.Run(kind, func(t *testing.T) {
			db, owner := scriptTestDB(t)
			objects := scriptobjects.NewStorage(scriptStorage(t))
			actor, pid := historyForCopy(t, db, owner, objects)
			_, job, worker := startActualScriptCopy(t, db, owner, actor, pid)
			_, b, snapshot := scriptCopyTransfer(db, job, worker, false, objects)
			body := []byte("与原版本正文不同")
			mime := "text/plain; charset=utf-8"
			if kind == "rich" {
				body = []byte(`[{"type":"doc","content":[]}]`)
				mime = "application/json"
			}
			fact := domain.ObjectFact{Key: "projects/" + pid.String() + "/script/" + uuid.NewString() + "/consumer-fault", SHA256: domain.ContentSHA(body), ByteSize: int64(len(body)), MIME: mime}
			if err := objects.PutIfAbsent(t.Context(), fact.Key, bytes.NewReader(body), fact.ByteSize, mime, fact.SHA256); err != nil {
				t.Fatal(err)
			}
			authority := workspaceapp.ProjectCopyAuthority{Binding: workspaceapp.ProjectCopyBinding(b), ActorID: actor.ID, WorkerID: worker, SourceRevision: job.SourceRevision, Phase: "transfer"}
			// Actual PG authorization runs first. This consuming-port fault then changes only
			// the reported version object while source rich documents stay intact.
			frozen := alteredCopyManifest{ProjectCopyStore: actualScriptCopyStore(db, authority), alter: func(m *app.ProjectCopyManifest) {
				old := m.Source.Versions[0].Text
				if kind == "rich" {
					old = m.Source.Versions[0].Rich
					m.Source.Versions[0].Rich = fact
				} else {
					m.Source.Versions[0].Text = fact
				}
				for i := range m.Objects {
					if m.Objects[i].Source.Key == old.Key {
						m.Objects[i].Source = fact
					}
				}
			}}
			_, err := app.NewProjectCopy(frozen, objects).Transfer(t.Context(), actor, b, snapshot)
			if err == nil {
				t.Fatal("wrong actual version object accepted")
			}
			var started int64
			if err := owner.Raw(`SELECT count(*) FROM script.copy_object_state s JOIN script.copy_object_intent i ON i.target_key=s.target_key WHERE i.snapshot_id=? AND s.put_started`, snapshot.ID).Scan(&started).Error; err != nil || started != 0 {
				t.Fatal("version content verified after target IO", started, err)
			}
		})
	}
}

func TestScriptCopyPGCorruptStructuredRowsRejectBeforeSnapshot(t *testing.T) {
	for _, fault := range []string{"dialogue_content", "missing_action"} {
		t.Run(fault, func(t *testing.T) {
			db, owner := scriptTestDB(t)
			objects := scriptobjects.NewStorage(scriptStorage(t))
			actor, pid := historyForCopy(t, db, owner, objects)
			if fault == "dialogue_content" {
				if err := owner.Exec(`UPDATE script.dialogue_line SET content='损坏的结构正文' WHERE project_id=?`, pid).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := owner.Exec(`DELETE FROM script.action_line WHERE project_id=?`, pid).Error; err != nil {
					t.Fatal(err)
				}
			}
			var revision int64
			if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, pid).Scan(&revision).Error; err != nil {
				t.Fatal(err)
			}
			store := workspaceWithActualScript(db)
			if _, err := store.Create(t.Context(), actor, workspaceapp.ProjectCopyInput{SourceProjectID: pid, ExpectedRevision: revision, TargetName: "拒绝损坏结构", IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}, time.Now().UTC()); err == nil {
				t.Fatal("structured facts mismatch admitted")
			}
			var snapshots int64
			if err := owner.Table("script.copy_snapshot").Where("source_project_id=?", pid).Count(&snapshots).Error; err != nil || snapshots != 0 {
				t.Fatal("invalid structure produced frozen snapshot", snapshots, err)
			}
		})
	}
}
