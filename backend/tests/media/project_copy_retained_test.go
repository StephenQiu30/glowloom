package media_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestProjectCopyHistoricalDocumentRetainsSourceDeletionEvidence(t *testing.T) {
	a := validAsset()
	a.Kind, a.Origin, a.Status, a.ModerationStatus = domain.KindDocument, domain.OriginUpload, domain.StatusReady, domain.ModerationPassed
	a.Width, a.Height, a.FPS, a.DurationMS, a.AudioChannels = nil, nil, nil, nil, nil
	hash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	a.SHA256 = &hash
	codec := "txt"
	a.Codec, a.MimeType, a.FileName = &codec, "text/plain", "原文.txt"
	a.ObjectKey = "projects/" + a.ProjectID.String() + "/document/2026/09/" + a.ID.String() + ".txt"
	deleted := a.CreateTime.Add(time.Hour)
	var err error
	if a, err = a.Delete(deleted); err != nil {
		t.Fatal(err)
	}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: uuid.New(), SourceProjectID: a.ProjectID, TargetProjectID: uuid.New()}
	if _, err := mediaapp.PrepareProjectMediaCopy(binding, []mediaapp.ProjectCopySourceAsset{{Asset: a}}, deleted); !errors.Is(err, mediaapp.ErrProjectCopyMediaUnavailable) {
		t.Fatal("ordinary copy accepted deleted source", err)
	}
	proof := &mediaapp.RetainedHistoryProof{AssetID: a.ID, Revision: a.Revision, DeletedAt: *a.DeleteTime, PurgeAfter: *a.PurgeAfter}
	result, err := mediaapp.PrepareProjectMediaCopy(binding, []mediaapp.ProjectCopySourceAsset{{Asset: a, RetainedHistory: proof}}, deleted)
	if err != nil {
		t.Fatal("explicit retained historical original", err)
	}
	target := result.Assets[0]
	if target.Asset.ID == a.ID || target.Asset.IsDelete || target.Asset.DeleteTime != nil || target.Asset.PurgeAfter != nil || target.RetainedHistory == nil || target.RetainedHistory.AssetID != a.ID || !a.IsDelete {
		t.Fatal("source deletion erased or new target is unusable", target)
	}
	proof.Revision++
	if _, err := mediaapp.PrepareProjectMediaCopy(binding, []mediaapp.ProjectCopySourceAsset{{Asset: a, RetainedHistory: proof}}, deleted); !errors.Is(err, mediaapp.ErrProjectCopyMediaUnavailable) {
		t.Fatal("forged retained revision accepted", err)
	}
}
