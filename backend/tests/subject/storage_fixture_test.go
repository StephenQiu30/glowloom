package subject_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func seedSubjectLocation(t *testing.T, db *gorm.DB, key, profile, bucket string) uuid.UUID {
	t.Helper()
	profiles := []media.StorageProfile{{ID: profile, Provider: "minio", Endpoint: "http://127.0.0.1:57924", SigningEndpoint: "http://127.0.0.1:57924", Region: "us-east-1", Bucket: bucket, AddressingStyle: "path"}}
	if err := pgmedia.NewStorageRoutes(db).RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := db.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,?,?,?,'accepted')`, id, profile, bucket, key).Error; err != nil {
		t.Fatal(err)
	}
	return id
}
