package media_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestLibrarySchemaPersonalOwnershipAndRuntimeAuthority(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, project := mediaStoreProject(t, db)
	foreign, _ := mediaStoreProject(t, db)
	asset := uuid.New()
	key := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/2026/10/" + asset.String() + ".png"
	insert := `INSERT INTO media.media_asset(id,project_id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,moderation_status) VALUES(?,?,?,?,'image','upload','ready',?,'ownership fixture.png','image/png',88,'passed')`
	if err := db.Exec(insert, asset, nil, actor.OrgID, actor.ID, key).Error; err != nil {
		t.Fatal("closed personal ownership cannot be persisted", err)
	}
	for _, candidate := range []struct{ project, org, actor any }{{project, actor.OrgID, actor.ID}, {nil, actor.OrgID, foreign.ID}, {nil, nil, nil}} {
		err := db.Exec(insert, uuid.New(), candidate.project, candidate.org, candidate.actor, "scope-fixture/"+uuid.NewString()).Error
		var databaseError *pgconn.PgError
		if !errors.As(err, &databaseError) || databaseError.Code != "23514" && databaseError.Code != "23503" {
			t.Fatal("invalid personal/project ownership accepted", err)
		}
	}
	for _, column := range []string{"personal_org_id", "personal_actor_id", "project_id"} {
		err := db.Exec(`UPDATE media.media_asset SET ` + column + `=` + column + ` WHERE false`).Error
		var databaseError *pgconn.PgError
		if !errors.As(err, &databaseError) || databaseError.Code != "42501" {
			t.Fatal("runtime can rewrite immutable ownership", column, err)
		}
	}
}
