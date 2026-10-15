package identity_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestConsumerFixturesRequireCurrentSessionAndExplicitPersonalOwnership(t *testing.T) {
	tx := existingIdentityTransaction(t)
	createActor := func(org uuid.UUID) identity.Principal {
		t.Helper()
		actor := identity.Principal{ID: uuid.New(), OrgID: org}
		if err := tx.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash) VALUES(?,?,?,'Consumer fixture','producer','synthetic')`, actor.ID, org, "consumer-fixture-"+actor.ID.String()).Error; err != nil {
			t.Fatal(err)
		}
		return identityfixture.Session(t, tx, actor)
	}
	actor := createActor(uuid.New())
	peer := createActor(actor.OrgID)
	project := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Consumer fixture','16:9','realistic')`, project, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	canvases := canvas.NewService(pgcanvas.NewScopedStore(tx, nil, app.NewCanvasProjectAccess))
	doc, err := canvases.Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Fixture"})
	if err != nil {
		t.Fatal("real request authority rejected", err)
	}
	if _, err := canvases.Get(t.Context(), peer, doc.ID); !errors.Is(err, pgmedia.ErrNotFound) {
		t.Fatal("same physical organization exposed a private document", err)
	}
	stale := actor
	stale.CredentialEpoch++
	if _, err := canvases.Get(t.Context(), stale, doc.ID); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("stale request authority admitted", err)
	}
	assetID := uuid.New()
	now := time.Now().UTC()
	asset := media.MediaAsset{ID: assetID, ProjectID: project, Kind: media.KindImage, Origin: media.OriginUpload, Status: media.StatusUploading, ObjectKey: "projects/" + project.String() + "/image/2026/10/" + assetID.String() + ".png", FileName: "fixture.png", MimeType: "image/png", ByteSize: 12, ModerationStatus: media.ModerationPending, Revision: 1, CreateTime: now, UpdateTime: now}
	requests := pgmedia.NewScopedStore(tx, app.NewMediaScopeAccess)
	if err := requests.CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatal("media request fixture rejected", err)
	}
	if _, err := requests.FindAsset(t.Context(), peer, project, assetID); !errors.Is(err, pgmedia.ErrNotFound) {
		t.Fatal("same physical organization exposed a private asset", err)
	}
	worker := actor
	worker.SessionID = uuid.Nil
	if _, err := requests.FindAsset(t.Context(), worker, project, assetID); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("request accepted absent session", err)
	}
	workers := pgmedia.NewScopedStore(tx, app.NewMediaWorkerScopeAccess)
	if _, err := workers.FindAsset(t.Context(), worker, project, assetID); err != nil {
		t.Fatal("durable worker authority required an HTTP session", err)
	}
	if err := tx.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := canvases.Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Archived"}); !errors.Is(err, media.ErrMediaStateConflict) {
		t.Fatal("archived canvas admission error", err)
	}
	if err := tx.Exec(`UPDATE workspace.project SET status='active' WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := requests.FindAsset(t.Context(), actor, project, assetID); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("disabled request actor admitted", err)
	}
	if _, err := workers.FindAsset(t.Context(), worker, project, assetID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("disabled worker actor admitted", err)
	}
}
