package identity_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestTeamAvatarBindingSurvivesUploaderDeparture(t *testing.T) {
	tx, _, platform, now := governanceFixture(t)
	_, owner := verifiedTestActor(t, tx, "team-avatar-owner@example.invalid", now)
	_, reader := verifiedTestActor(t, tx, "team-avatar-reader@example.invalid", now)
	var org struct{ OrgID uuid.UUID }
	tx.Raw(`SELECT org_id FROM identity."user" WHERE id=?`, owner.ID).Scan(&org)
	owner.OrgID = org.OrgID
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	store := pgworkspace.NewTeamProfileStore(tx, func(db *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(db, completionProtector(t))
	}, func(db *gorm.DB) workspace.TeamAvatarAccess { return pgmedia.NewTeamAvatarAccess(db) })
	teams := workspace.NewTeams(store, func() time.Time { return now })
	team, err := teams.Create(t.Context(), owner, uuid.New(), "Retained team avatar")
	if err != nil {
		t.Fatal(err)
	}
	repo := pgmedia.NewAvatarStore(tx, func(db *gorm.DB) media.AvatarActorAccess { return pgidentity.NewStore(db) })
	uploads := media.NewAvatarService(repo, &avatarMemoryObjects{}, func() time.Time { return now })
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewNRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	asset, err := uploads.Upload(t.Context(), owner, input.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	created, err := teams.CreateWithAvatar(t.Context(), owner, uuid.New(), "Created with avatar", &asset.ID)
	if err != nil || created.AvatarID == nil || *created.AvatarID != asset.ID {
		t.Fatalf("creation avatar not atomic: %v", err)
	}
	invalid := uuid.New()
	if _, err := teams.CreateWithAvatar(t.Context(), owner, uuid.New(), "Invalid avatar team", &invalid); !errors.Is(err, identity.ErrAvatarUnavailable) {
		t.Fatalf("invalid creation avatar accepted: %v", err)
	}
	if _, err = teams.UpdateProfile(t.Context(), owner, team.ID, workspace.TeamProfileInput{ExpectedRevision: team.Revision, AvatarID: &asset.ID}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,source,joined_at) VALUES(?,?,'team_admin','active','invited',?)`, team.ID, reader.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	if err = teams.Leave(t.Context(), owner, team.ID, 1); err != nil {
		t.Fatal(err)
	}
	signer := &avatarTestSigner{}
	query := workspace.NewTeamAvatars(store, signer, func() time.Time { return now })
	preview, err := query.Read(t.Context(), reader, team.ID, nil, false)
	if err != nil || preview.AssetID != asset.ID {
		t.Fatalf("departed uploader broke team avatar: %+v %v", preview, err)
	}
	if _, err = query.Read(t.Context(), owner, team.ID, nil, false); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("departed member retained avatar access: %v", err)
	}
	if _, err = query.Read(t.Context(), platform, team.ID, nil, true); err != nil {
		t.Fatal(err)
	}
	signer.afterSign = func() {
		if err := tx.Exec(`UPDATE workspace.team SET avatar_asset_id=NULL WHERE id=?`, team.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	preview, err = query.Read(t.Context(), reader, team.ID, nil, false)
	if !errors.Is(err, identity.ErrAvatarUnavailable) || preview.URL != "" {
		t.Fatalf("changed avatar escaped recheck: %v", err)
	}
}
