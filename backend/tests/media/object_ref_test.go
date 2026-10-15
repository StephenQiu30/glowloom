package media_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestObjectReferenceRejectsIncompleteOrUnsafeIdentity(t *testing.T) {
	for _, ref := range []domain.ObjectRef{{Bucket: "bucket", Key: "key"}, {ProfileID: "minio-main", Key: "key"}, {ProfileID: "minio-main", Bucket: "bucket", Key: "a/../b"}, {ProfileID: "minio-main", Bucket: "bucket", Key: "a\x00b"}} {
		if ref.Validate() == nil {
			t.Fatal("unsafe reference accepted")
		}
	}
}

func TestMediaAssetRejectsMismatchedPhysicalLocation(t *testing.T) {
	asset := validAsset()
	asset.ObjectLocation = &domain.ObjectLocation{ID: uuid.New(), Ref: domain.ObjectRef{ProfileID: "minio-main", Bucket: "storage-test", Key: "unrelated/key"}, State: "present"}
	if asset.Validate() == nil {
		t.Fatal("asset accepted unrelated physical bytes")
	}
}
