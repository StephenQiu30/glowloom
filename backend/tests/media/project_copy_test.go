package media_test

import (
	"errors"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestProjectCopyMediaPreservesContentWithoutExecutionHistory(t *testing.T) {
	asset := validAsset()
	asset.Status, asset.ModerationStatus = domain.StatusReady, domain.ModerationPassed
	asset.Origin = domain.OriginGenerated
	op, provider, model, region := uuid.New(), "fixture", "video", "domestic"
	asset.SourceOperationID, asset.ProviderKey, asset.ModelKey, asset.Region = &op, &provider, &model, &region
	asset.AIGCMarked = true
	r := domain.Rendition{ID: uuid.New(), MediaAssetID: asset.ID, Kind: domain.RenditionPoster, ObjectKey: asset.ObjectKey[:len(asset.ObjectKey)-len(path.Ext(asset.ObjectKey))] + "/poster.png", CreateTime: asset.CreateTime, UpdateTime: asset.UpdateTime}
	binding := application.ProjectCopyBinding{JobID: uuid.New(), OrgID: uuid.New(), SourceProjectID: asset.ProjectID, TargetProjectID: uuid.New()}
	copied, err := application.PrepareProjectMediaCopy(binding, []application.ProjectCopySourceAsset{{Asset: asset, Renditions: []domain.Rendition{r}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a := copied.Assets[0].Asset
	if a.ID == asset.ID || a.ProjectID != binding.TargetProjectID || a.ObjectKey == asset.ObjectKey || a.Origin != domain.OriginSystem || a.SourceOperationID != nil || a.ProviderKey != nil || a.ModelKey != nil || a.Region != nil || !a.AIGCMarked || a.Status != domain.StatusReady || a.ModerationStatus != domain.ModerationPassed || a.Revision != 1 || copied.Assets[0].Renditions[0].MediaAssetID != a.ID {
		t.Fatal("copied history or incomplete media remap", copied)
	}
	if asset.SourceOperationID == nil || r.MediaAssetID != asset.ID {
		t.Fatal("source mutated")
	}
	again, err := application.PrepareProjectMediaCopy(binding, []application.ProjectCopySourceAsset{{Asset: asset, Renditions: []domain.Rendition{r}}}, a.CreateTime)
	if err != nil || !reflect.DeepEqual(copied, again) {
		t.Fatal("retry changed private asset identities", err)
	}
	asset.ContainsRealPerson = true
	if _, err := application.PrepareProjectMediaCopy(binding, []application.ProjectCopySourceAsset{{Asset: asset}}, a.CreateTime); !errors.Is(err, application.ErrProjectCopyConsentUnavailable) {
		t.Fatal("unavailable consent silently cleared", err)
	}
	asset.ContainsRealPerson = false
	asset.ModerationStatus, asset.Status = domain.ModerationRejected, domain.StatusRejected
	if _, err := application.PrepareProjectMediaCopy(binding, []application.ProjectCopySourceAsset{{Asset: asset}}, a.CreateTime); !errors.Is(err, application.ErrProjectCopyMediaUnavailable) {
		t.Fatal("rejected source promoted or dropped", err)
	}
}
