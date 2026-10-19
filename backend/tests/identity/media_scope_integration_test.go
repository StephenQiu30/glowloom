package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"

	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

type mediaScopeTestAccess struct {
	tx      *gorm.DB
	request workspace.RequestScopeResolver
	worker  bool
}

func (a mediaScopeTestAccess) CheckActor(ctx context.Context, p identity.Principal) error {
	if a.worker {
		return pgidentity.NewStore(a.tx).CheckActive(ctx, p.ID)
	}
	return pgidentity.NewStore(a.tx).CheckSession(ctx, p, time.Now)
}
func (a mediaScopeTestAccess) AuthorizeProject(ctx context.Context, p identity.Principal, id uuid.UUID, w bool) error {
	var store *pgworkspace.ProjectContentAccessStore
	if a.worker {
		store = pgworkspace.NewWorkerProjectContentAccessStore(a.tx, workspace.NewScopeAccess(pgworkspace.NewScopeStore(a.tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) })))
	} else {
		store = pgworkspace.NewScopedProjectContentAccessStore(a.tx, a.request)
	}
	_, err := store.Authorize(ctx, p, id, w)
	return err
}
func (a mediaScopeTestAccess) AuthorizePersonal(ctx context.Context, p identity.Principal, w bool) error {
	var s workspacedomain.Scope
	var err error
	if a.worker {
		store := pgworkspace.NewScopeStore(a.tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) })
		id, e := store.PersonalScopeID(ctx, p.ID)
		if e != nil {
			return e
		}
		s, err = workspace.NewScopeAccess(store).Resolve(ctx, p.ID, id, w)
	} else {
		s, err = a.request.Resolve(ctx, p, p.ScopeID, w)
	}
	if err != nil {
		return err
	}
	if s.Kind != "personal" || s.OwnerUserID == nil || *s.OwnerUserID != p.ID || s.PhysicalScopeID != p.OrgID {
		return workspace.ErrScopeNotFound
	}
	return nil
}
func (a mediaScopeTestAccess) AuthorizeCopyTarget(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	var store *pgworkspace.ProjectContentAccessStore
	if a.worker {
		store = pgworkspace.NewWorkerProjectContentAccessStore(a.tx, workspace.NewScopeAccess(pgworkspace.NewScopeStore(a.tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) })))
	} else {
		store = pgworkspace.NewScopedProjectContentAccessStore(a.tx, a.request)
	}
	_, err := store.AuthorizeCopyTarget(ctx, actor, id)
	return err
}

func TestMediaCanvasScopeConsumers(t *testing.T) {
	tx, actor, team, scope, project, factory := projectConsumerFixture(t)
	personal := actor.ScopeID
	actor.ScopeID = scope
	access := func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, request: factory(tx)} }
	canvasAccess := func(tx *gorm.DB) canvasapp.ProjectAccess {
		return app.NewCanvasProjectAccessWithScope(tx, mediaScopeTestAccess{tx: tx, request: factory(tx)})
	}
	media := pgmedia.NewScopedStore(tx, access)
	canvas := pgcanvas.NewScopedStore(tx, nil, canvasAccess)
	if err := media.AuthorizeDerivedProject(t.Context(), actor, project, false); err != nil {
		t.Fatalf("team media read: %v", err)
	}
	doc, err := canvas.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "Team canvas", Scope: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("team canvas create: %v", err)
	}
	if _, err = canvas.Get(t.Context(), actor, doc.ID); err != nil {
		t.Fatal(err)
	}
	other := actor
	other.ScopeID = personal
	if err = media.AuthorizeDerivedProject(t.Context(), other, project, false); err == nil {
		t.Fatal("same physical personal scope read team media")
	}
	if _, err = canvas.Get(t.Context(), other, doc.ID); err == nil {
		t.Fatal("same physical personal scope read team canvas")
	}
	if err = media.AuthorizePersonalUpload(t.Context(), actor); err == nil {
		t.Fatal("team scope read personal upload")
	}
	if err = media.AuthorizePersonalUpload(t.Context(), other); err != nil {
		t.Fatalf("personal upload: %v", err)
	}
	if err = tx.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if err = media.AuthorizeDerivedProject(t.Context(), actor, project, false); err != nil {
		t.Fatalf("archived read: %v", err)
	}
	if err = media.AuthorizeDerivedProject(t.Context(), actor, project, true); err == nil {
		t.Fatal("archived write")
	}
	if _, err = canvas.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "Denied", Scope: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("archived canvas write")
	}
	if err = tx.Exec(`UPDATE workspace.team SET status='active' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if err = tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err = media.AuthorizeDerivedProject(t.Context(), actor, project, false); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked media session: %v", err)
	}
	if _, err = canvas.Get(t.Context(), actor, doc.ID); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked canvas session: %v", err)
	}
	worker := pgmedia.NewScopedStore(tx, func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, worker: true} })
	actor.SessionID = uuid.Nil
	actor.ScopeID = uuid.Nil
	if err = worker.AuthorizeDerivedProject(t.Context(), actor, project, true); err != nil {
		t.Fatalf("worker independent of expired request: %v", err)
	}
	if err = tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	actor.PlatformRole = "platform_admin"
	if err = worker.AuthorizeDerivedProject(t.Context(), actor, project, true); err == nil {
		t.Fatal("revoked worker published")
	}
}

func (a mediaScopeTestAccess) TransferActor(ctx context.Context, p identity.Principal, library mediadomain.LibraryScope, write bool) (identity.Principal, error) {
	if err := a.CheckActor(ctx, p); err != nil {
		return p, err
	}
	if library.Kind == mediadomain.LibraryProject {
		if err := a.AuthorizeProject(ctx, p, *library.ProjectID, write); err != nil {
			return p, err
		}
		if a.worker {
			var row struct{ OwnerScopeID uuid.UUID }
			if err := a.tx.Raw(`SELECT owner_scope_id FROM workspace.project WHERE id=?`, library.ProjectID).Scan(&row).Error; err != nil {
				return p, err
			}
			p.ScopeID = row.OwnerScopeID
		}
		return p, nil
	}
	store := pgworkspace.NewScopeStore(a.tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) })
	id, err := store.PersonalScopeID(ctx, p.ID)
	if err != nil {
		return p, err
	}
	if a.worker {
		s, err := workspace.NewScopeAccess(store).Resolve(ctx, p.ID, id, write)
		if err != nil {
			return p, err
		}
		p.ScopeID, p.OrgID = s.ID, s.PhysicalScopeID
	} else {
		s, err := a.request.Resolve(ctx, p, id, write)
		if err != nil {
			return p, err
		}
		p.ScopeID, p.OrgID = s.ID, s.PhysicalScopeID
	}
	return p, nil
}

type mediaLibraryTestAccess struct {
	owner *pgworkspace.ProjectContentAccessStore
}

func (a mediaLibraryTestAccess) Authorize(ctx context.Context, p identity.Principal, id uuid.UUID, w bool) (mediaapp.LibraryProjectFacts, error) {
	f, err := a.owner.Authorize(ctx, p, id, w)
	return mediaapp.LibraryProjectFacts{ProjectID: f.ProjectID, OrgID: f.OrgID, Revision: f.Revision}, err
}
func (a mediaLibraryTestAccess) TouchContent(ctx context.Context, p identity.Principal, id uuid.UUID, r int64) (int64, error) {
	return a.owner.TouchContent(ctx, p, id, r)
}

type idleMediaWork struct{}

func (idleMediaWork) HasInflightProjectWork(context.Context, identity.Principal, uuid.UUID) (bool, error) {
	return false, nil
}
func installMediaScopeMigration(t *testing.T, tx *gorm.DB) {
	t.Helper()
	verifyMediaScope(t, tx)
}

func TestMediaTransferCrossPhysicalScopes(t *testing.T) {
	tx, actor, team, scope, project, factory := projectConsumerFixture(t)
	installMediaScopeMigration(t, tx)
	personalActor := actor
	teamOrg := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.organization(id,name,status) VALUES(?,'transfer test','active')`, teamOrg).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.project SET owner_scope_id=NULL WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.creation_scope SET physical_scope_id=? WHERE id=?`, teamOrg, scope).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.project SET org_id=?,owner_scope_id=? WHERE id=?`, teamOrg, scope, project).Error; err != nil {
		t.Fatal(err)
	}
	actor.ScopeID, actor.OrgID = scope, teamOrg
	requestAccess := func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, request: factory(tx)} }
	projectAccess := func(tx *gorm.DB) mediaapp.LibraryProjectAccess {
		return mediaLibraryTestAccess{pgworkspace.NewScopedProjectContentAccessStore(tx, factory(tx))}
	}
	workerAccess := func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, worker: true} }
	workerProject := func(tx *gorm.DB) mediaapp.LibraryProjectAccess {
		return mediaLibraryTestAccess{pgworkspace.NewWorkerProjectContentAccessStore(tx, workspace.NewScopeAccess(pgworkspace.NewScopeStore(tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) })))}
	}
	guards := func(*gorm.DB) mediaapp.LibraryWorkGuards { return idleMediaWork{} }
	store := pgmedia.NewScopedTransferStore(tx, projectAccess, guards, time.Now, requestAccess)
	worker := pgmedia.NewScopedTransferStore(tx, workerProject, guards, time.Now, workerAccess)
	source := mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}
	target := mediadomain.LibraryScope{Kind: mediadomain.LibraryProject, ProjectID: &project}
	library, _ := source.Identity(personalActor.OrgID, actor.ID)
	item := uuid.New()
	if err := tx.Exec(`INSERT INTO media.library(id,kind,org_id,personal_actor_id,revision) VALUES(?,'personal',?,?,1)`, library, personalActor.OrgID, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.library_item(id,library_id,plain_text,title,category,tags,note,favorite,catalog_state,position,revision) VALUES(?,?,'retained text','text','other','[]','private note',true,'active',0,1)`, item, library).Error; err != nil {
		t.Fatal(err)
	}
	input := mediaapp.TransferInput{Source: source, Target: target, Items: []mediaapp.LibraryItemRevision{{ID: item, Revision: 1}}, ExpectedSourceRevision: 1, ExpectedTargetRevision: 0, ExpectedProjectRevision: 1, Key: uuid.New()}
	job, err := store.CreateTransfer(t.Context(), actor, input)
	if err != nil {
		t.Fatalf("cross physical transfer admission: %v", err)
	}
	if job.SourceScopeID != personalActor.ScopeID || job.TargetScopeID != scope {
		t.Fatal("wrong frozen scopes")
	}
	if err := tx.SavePoint("ownership_proof").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE media.transfer_job SET source_scope_id=target_scope_id WHERE id=?`, job.ID).Error; err == nil {
		t.Fatal("durable ownership could be rewritten")
	}
	if err := tx.RollbackTo("ownership_proof").Error; err != nil {
		t.Fatal(err)
	}
	if replay, err := store.CreateTransfer(t.Context(), actor, input); err != nil || replay.ID != job.ID {
		t.Fatalf("replay: %v", err)
	}
	if _, err := store.GetTransfer(t.Context(), personalActor, job.ID); err == nil {
		t.Fatal("wrong selected scope read job")
	}
	work := mediaapp.TransferWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()}
	lease, err := worker.ClaimTransfer(t.Context(), work)
	if err != nil {
		t.Fatalf("worker claim: %v", err)
	}
	if _, err := worker.StartTransferItem(t.Context(), lease, 0); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := worker.PublishTransferItem(t.Context(), lease, 0); err == nil {
		t.Fatal("removed worker published")
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='active' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := worker.PublishTransferItem(t.Context(), lease, 0); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := worker.EndTransferPhysical(t.Context(), lease); err != nil {
		t.Fatal(err)
	}
	finished, err := worker.FinishTransfer(t.Context(), lease)
	if err != nil || finished.Status != "succeeded" {
		t.Fatalf("finish: %s %v", finished.Status, err)
	}
	// Transfer the new team item back into the actual personal physical partition.
	reverse := mediaapp.TransferInput{Source: target, Target: source, Items: []mediaapp.LibraryItemRevision{{ID: job.Items[0].TargetItemID, Revision: 1}}, ExpectedSourceRevision: 1, ExpectedTargetRevision: 1, ExpectedProjectRevision: 2, Key: uuid.New()}
	back, err := store.CreateTransfer(t.Context(), actor, reverse)
	if err != nil {
		t.Fatalf("reverse admission: %v", err)
	}
	if back.SourceScopeID != scope || back.TargetScopeID != personalActor.ScopeID {
		t.Fatal("reverse ownership")
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := worker.ClaimTransfer(t.Context(), mediaapp.TransferWorkID{JobID: back.ID, Attempt: 1, ExecutionID: uuid.NewString()}); err == nil {
		t.Fatal("revoked queued transfer dispatched")
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='active' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	// An archived source is readable; copying out must not mutate it.
	if err := tx.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	backLease, err := worker.ClaimTransfer(t.Context(), mediaapp.TransferWorkID{JobID: back.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil {
		t.Fatalf("archived source claim: %v", err)
	}
	if _, err = worker.StartTransferItem(t.Context(), backLease, 0); err != nil {
		t.Fatal(err)
	}
	if err = worker.PublishTransferItem(t.Context(), backLease, 0); err != nil {
		t.Fatalf("personal target publication: %v", err)
	}
	if err = worker.EndTransferPhysical(t.Context(), backLease); err != nil {
		t.Fatal(err)
	}
	if done, err := worker.FinishTransfer(t.Context(), backLease); err != nil || done.Status != "succeeded" {
		t.Fatalf("reverse finish: %s %v", done.Status, err)
	}
	var actual struct{ OrgID uuid.UUID }
	if err = tx.Raw(`SELECT l.org_id FROM media.library l JOIN media.library_item i ON i.library_id=l.id WHERE i.id=?`, back.Items[0].TargetItemID).Scan(&actual).Error; err != nil || actual.OrgID != personalActor.OrgID {
		t.Fatalf("wrong personal target physical partition: %s %v", actual.OrgID, err)
	}
	// The inventory must also retain a private object intent in the personal
	// physical partition when the job itself is partitioned under the team.
	personalKey := "personal/" + personalActor.OrgID.String() + "/" + actor.ID.String() + "/image/retained.png"
	if err := tx.Exec(`INSERT INTO media.transfer_object(job_id,item_index,source_object_key,target_object_key,content_type) VALUES(?,0,?,?,'image/png')`, back.ID, "projects/"+project.String()+"/image/retained.png", personalKey).Error; err != nil {
		t.Fatal(err)
	}
	usage := pgmedia.NewScopedStorageUsageStore(tx, projectAccess, func(*gorm.DB) mediaapp.StorageUsageIntentSource { return emptyScopeUsageIntents{} }, requestAccess)
	seen := false
	if err := usage.InspectStorageUsage(t.Context(), personalActor, source, func(in mediaapp.StorageUsageInventory) error {
		for _, key := range in.ObjectKeys {
			seen = seen || key == personalKey
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("personal inventory lost cross-physical retained transfer object")
	}
}

func TestMediaToolQueuedDeliveryRechecksAuthority(t *testing.T) {
	tx, actor, team, scope, project, _ := projectConsumerFixture(t)
	installMediaScopeMigration(t, tx)
	actor.ScopeID = scope
	work, key, event := uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO mediatool.export_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,frozen,status,stage,progress,attempt,revision,created_at,updated_at) VALUES(?,?,?,?,NULL,?,?,1,'{}','queued','queued',0,1,1,now(),now())`, work, project, actor.OrgID, actor.ID, uuid.New(), uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO mediatool.export_command(actor_id,request_id,job_id,request_hash,response,event_id,event_action,event_attempt,created_at) VALUES(?,?,?,repeat('a',64),'{}',?,'start',1,now())`, actor.ID, key, work, event).Error; err != nil {
		t.Fatal(err)
	}
	store := pgtool.NewStore(tx, nil, func(tx *gorm.DB) toolapp.DerivedMedia {
		return mediaapp.NewDerivedService(pgmedia.NewScopedStore(tx, func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, worker: true} }))
	})
	delivery := toolapp.Delivery{WorkID: toolapp.WorkID{JobID: work, Attempt: 1}, EventID: event, RequestID: key, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: project, Action: "start"}
	if ok, err := store.VerifyDelivery(t.Context(), delivery); err != nil || !ok {
		t.Fatalf("active worker delivery: %t %v", ok, err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := store.VerifyDelivery(t.Context(), delivery); err == nil || ok {
		t.Fatalf("removed member delivery accepted: %t %v", ok, err)
	}
	if _, err := store.Claim(t.Context(), toolapp.WorkID{JobID: work, Attempt: 1, ExecutionID: uuid.New()}); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("revoked queued claim: %v", err)
	}
}

func TestTranscriptionStartRechecksRevocation(t *testing.T) {
	tx, actor, team, _, project, _ := projectConsumerFixture(t)
	installMediaScopeMigration(t, tx)
	job, worker := uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO mediatool.transcription_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,language,frozen,status,stage,progress,attempt,revision,inference_state,active_worker,created_at,updated_at) VALUES(?,?,?,?,NULL,?,?,1,'auto','{}','running','downloading',5,1,1,'none',?,now(),now())`, job, project, actor.OrgID, actor.ID, uuid.New(), uuid.New(), worker).Error; err != nil {
		t.Fatal(err)
	}
	store := pgtool.NewTranscriptionStore(tx, nil, func(tx *gorm.DB) toolapp.DerivedMedia {
		return mediaapp.NewDerivedService(pgmedia.NewScopedStore(tx, func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, worker: true} }))
	})
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	err := store.StartInference(t.Context(), toolapp.TranscriptionWorkID{JobID: job, Attempt: 1, ExecutionID: worker})
	if !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("revoked inference submitted: %v", err)
	}
}

func TestMediaAssetLibraryDocumentIsolation(t *testing.T) {
	tx, actor, team, scope, project, factory := projectConsumerFixture(t)
	personal := actor
	actor.ScopeID = scope
	scopes := func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, request: factory(tx)} }
	projects := func(tx *gorm.DB) mediaapp.LibraryProjectAccess {
		return mediaLibraryTestAccess{pgworkspace.NewScopedProjectContentAccessStore(tx, factory(tx))}
	}
	store := pgmedia.NewScopedStore(tx, scopes)
	library := pgmedia.NewScopedLibraryStore(tx, projects, time.Now, scopes)
	documents := pgmedia.NewScopedDocumentSourceStore(tx, scopes)
	packages := pgmedia.NewScopedPackageStore(tx, projects, nil, time.Now, scopes)
	now := time.Now().UTC()
	assetID := uuid.New()
	sha := strings.Repeat("a", 64)
	asset := mediadomain.MediaAsset{ID: assetID, ProjectID: project, Kind: mediadomain.KindImage, Origin: mediadomain.OriginUpload, Status: mediadomain.StatusUploading, ObjectKey: "projects/" + project.String() + "/image/2026/10/" + assetID.String() + ".png", FileName: "scope.png", MimeType: "image/png", ByteSize: 12, ModerationStatus: mediadomain.ModerationPending, Revision: 1, CreateTime: now, UpdateTime: now}
	if err := store.CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed',sha256=? WHERE id=?`, sha, assetID).Error; err != nil {
		t.Fatal(err)
	}
	projectLibrary := mediadomain.LibraryScope{Kind: mediadomain.LibraryProject, ProjectID: &project}
	personalLibrary := mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}
	q := mediaapp.LibraryQuery{Page: 1, PageSize: 20, State: "active", Order: "updated_desc"}
	if found, err := store.FindAsset(t.Context(), actor, project, assetID); err != nil || found.ID != assetID {
		t.Fatalf("asset: %v", err)
	}
	if page, err := library.ListLibrary(t.Context(), actor, projectLibrary, q); err != nil || len(page.Items) != 1 {
		t.Fatalf("library: %d %v", len(page.Items), err)
	}
	if _, err := library.FindLibraryMedia(t.Context(), actor, projectLibrary, assetID); err != nil {
		t.Fatalf("download scope: %v", err)
	}
	if _, err := packages.AuthorizePackage(t.Context(), actor, projectLibrary, true); err != nil {
		t.Fatalf("package admission scope: %v", err)
	}
	if _, err := packages.AuthorizePackage(t.Context(), personal, projectLibrary, false); err == nil {
		t.Fatal("foreign scope exported package")
	}

	if _, err := library.ListLibrary(t.Context(), personal, personalLibrary, q); err != nil {
		t.Fatalf("personal library: %v", err)
	}
	// Same physical partition and a valid personal scope must not reveal project metadata or bytes.
	reads := []func() error{
		func() error { _, e := store.FindAsset(t.Context(), personal, project, assetID); return e },
		func() error { _, e := store.FindRenditions(t.Context(), personal, project, assetID); return e },
		func() error { _, e := documents.FindAsset(t.Context(), personal, project, assetID); return e },
		func() error { _, e := library.ListLibrary(t.Context(), personal, projectLibrary, q); return e },
		func() error {
			_, e := library.FindLibraryMedia(t.Context(), personal, projectLibrary, assetID)
			return e
		},
		func() error { _, e := library.ListLibrary(t.Context(), actor, personalLibrary, q); return e },
	}
	for i, read := range reads {
		if err := read(); err == nil {
			t.Fatalf("foreign scope path %d read resource", i)
		}
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := library.FindLibraryMedia(t.Context(), actor, projectLibrary, assetID); err != nil {
		t.Fatalf("archived download: %v", err)
	}
	if _, err := packages.AuthorizePackage(t.Context(), actor, projectLibrary, false); err != nil {
		t.Fatalf("archived package read: %v", err)
	}
	if _, err := packages.AuthorizePackage(t.Context(), actor, projectLibrary, true); err == nil {
		t.Fatal("archived package import")
	}

	rendition := mediadomain.Rendition{ID: uuid.New(), MediaAssetID: assetID, Kind: mediadomain.RenditionThumb256, ObjectKey: strings.TrimSuffix(asset.ObjectKey, ".png") + "/thumb_256.webp", CreateTime: now, UpdateTime: now}
	if err := store.AddRendition(t.Context(), actor, project, rendition); err == nil {
		t.Fatal("archived rendition write")
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='active' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.project SET owner_scope_id=NULL WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	actor.PlatformRole = "platform_admin"
	if _, err := store.FindAsset(t.Context(), actor, project, assetID); err == nil {
		t.Fatal("platform admin read NULL ownership")
	}
}

func TestMediaCopyTargetUsesNarrowScope(t *testing.T) {
	tx, actor, _, scope, source, factory := projectConsumerFixture(t)
	personal := actor.ScopeID
	actor.ScopeID = scope
	target := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type,status) VALUES(?,?,?,?,'Copying','16:9','realistic','copying')`, target, actor.OrgID, actor.ID, scope).Error; err != nil {
		t.Fatal(err)
	}
	scopes := func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, request: factory(tx)} }
	ordinary := pgmedia.NewScopedStore(tx, scopes)
	if err := ordinary.AuthorizeDerivedProject(t.Context(), actor, target, false); err == nil {
		t.Fatal("ordinary read exposed copying target")
	}
	access := pgworkspace.NewScopedProjectContentAccessStore(tx, factory(tx))
	if _, err := access.AuthorizeCopyTarget(t.Context(), actor, target); err != nil {
		t.Fatalf("narrow target: %v", err)
	}
	copies := pgmedia.NewScopedProjectCopyStore(tx, scopes)
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: source, TargetProjectID: target}
	if _, err := copies.Freeze(t.Context(), actor, binding, time.Now().UTC().Truncate(time.Microsecond)); err != nil {
		t.Fatalf("coordinator target: %v", err)
	}
	actor.ScopeID = personal
	if _, err := copies.Freeze(t.Context(), actor, binding, time.Now()); err == nil {
		t.Fatal("forged binding crossed valid personal scope")
	}
	if _, err := pgmedia.NewProjectCopyStore(tx).Freeze(t.Context(), actor, binding, time.Now()); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatalf("missing authority: %v", err)
	}
}

type emptyScopeUsageIntents struct{}

func (emptyScopeUsageIntents) StorageUsageKeys(context.Context, identity.Principal, mediadomain.LibraryScope) ([]string, error) {
	return nil, nil
}
