package identity_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
)

type avatarReferenceTestOwners struct{ tx *gorm.DB }

func (o avatarReferenceTestOwners) HasMediaReferences(ctx context.Context, _ identity.Principal, _ uuid.UUID, asset uuid.UUID) (bool, error) {
	a, err := pgidentity.NewAvatarReferences(o.tx).HasAvatarReference(ctx, asset)
	if err != nil {
		return false, err
	}
	b, err := pgworkspace.NewTeamAvatarReferences(o.tx).HasAvatarReference(ctx, asset)
	return a || b, err
}

func TestMediaPurgeProtectsRetainedAvatars(t *testing.T) {
	tx, actor, team, _, _, factory := projectConsumerFixture(t)
	installMediaScopeMigration(t, tx)
	// The existing database lacks this package table. Use its canonical definition
	// only within the rollback transaction; this is not deployment acceptance.
	var exists bool
	if err := tx.Raw(`SELECT to_regclass('media.package_job') IS NOT NULL`).Scan(&exists).Error; err != nil {
		t.Fatal(err)
	}
	if !exists {
		b, err := os.ReadFile("../../db/schema.sql")
		if err != nil {
			t.Fatal(err)
		}
		definition := strings.SplitN(strings.SplitN(string(b), "CREATE TABLE media.package_job (", 2)[1], "\n);", 2)[0]
		if err := tx.Exec("CREATE TABLE media.package_job (" + definition + "\n);").Error; err != nil {
			t.Fatal(err)
		}
	}
	scopes := func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, request: factory(tx)} }
	refs := func(tx *gorm.DB) mediaapp.LibraryReferences { return avatarReferenceTestOwners{tx} }
	store := pgmedia.NewScopedPurgeStore(tx, nil, nil, refs, time.Now, scopes)
	asset := uuid.New()
	library, _ := (mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}).Identity(actor.OrgID, actor.ID)
	sha := strings.Repeat("a", 64)
	key := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/2026/10/" + asset.String() + ".png"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO media.library(id,org_id,personal_actor_id,kind,revision) VALUES(?,?,?,'personal',1)`, []any{library, actor.OrgID, actor.ID}},
		{`INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,?,'image','upload','ready',?,'avatar.png','image/png',12,?,'passed')`, []any{asset, actor.OrgID, actor.ID, key, sha}},
		{`INSERT INTO media.library_item(id,library_id,asset_id,title,category,catalog_state,trashed_at,position) VALUES(?,?,?,'Avatar','other','trashed',now(),0)`, []any{asset, library, asset}},
	} {
		if err := tx.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	input := mediaapp.PurgeInput{Scope: mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}, Items: []mediaapp.LibraryItemRevision{{ID: asset, Revision: 1}}, ExpectedRevision: 1, PermanentDeleteConfirmed: true}
	for _, binding := range []string{"self", "team", "retained_soft_delete"} {
		owner := actor.ID
		if binding == "team" {
			if err := tx.Exec(`UPDATE workspace.team SET avatar_asset_id=? WHERE id=?`, asset, team).Error; err != nil {
				t.Fatal(err)
			}
		} else {
			if binding == "retained_soft_delete" {
				owner = uuid.New()
				if err := tx.Exec(`INSERT INTO identity.user(id,org_id,login_name,display_name,password_hash,role,status,is_delete) VALUES(?,?,?,'Retained','x','producer','disabled',true)`, owner, actor.OrgID, owner.String()+"@example.test").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Exec(`UPDATE identity.user SET avatar_asset_id=? WHERE id=?`, asset, owner).Error; err != nil {
				t.Fatal(err)
			}
		}
		input.Key = uuid.New()
		job, err := store.CreatePurge(t.Context(), actor, input)
		if err != nil {
			t.Fatalf("%s: %v", binding, err)
		}
		if len(job.Items) != 1 || job.Items[0].Status != "blocked" {
			t.Fatalf("%s not blocked: %+v", binding, job)
		}
		if err := tx.Exec(`UPDATE identity.user SET avatar_asset_id=NULL WHERE id=?`, owner).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(`UPDATE workspace.team SET avatar_asset_id=NULL WHERE id=?`, team).Error; err != nil {
			t.Fatal(err)
		}
	}
	input.Key = uuid.New()
	job, err := store.CreatePurge(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "queued" {
		t.Fatalf("unbound purge: %s", job.Status)
	}
	worker := pgmedia.NewScopedPurgeStore(tx, nil, nil, refs, time.Now, func(tx *gorm.DB) mediaapp.ScopeAccess { return mediaScopeTestAccess{tx: tx, worker: true} })
	lease, err := worker.ClaimPurge(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = worker.StartPurgeItem(t.Context(), lease, 0); err != nil {
		t.Fatal(err)
	}
	if err = worker.RecordPurgeDigest(t.Context(), lease, 0, key, sha, 12); err != nil {
		t.Fatal(err)
	}
	// Simulate a retained binding discovered after admission; no object storage call.
	if err = tx.Exec(`UPDATE workspace.team SET avatar_asset_id=? WHERE id=?`, asset, team).Error; err != nil {
		t.Fatal(err)
	}
	if err = worker.BeginPurgeRemoval(t.Context(), lease, 0, key); !errors.Is(err, mediadomain.ErrPurgeConflict) {
		t.Fatalf("bound avatar deletion started: %v", err)
	}
}
