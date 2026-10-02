package media_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestMediaTransferClosedScopesAndStableIndependentOwnership(t *testing.T) {
	a := validAsset()
	a.Status, a.ModerationStatus = domain.StatusReady, domain.ModerationPassed
	sha := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	a.SHA256 = &sha
	org, actor, job := uuid.New(), uuid.New(), uuid.New()
	target := domain.LibraryScope{Kind: domain.LibraryPersonal}
	source := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &a.ProjectID}
	request := mediaapp.TransferInput{Source: source, Target: target, Items: []mediaapp.LibraryItemRevision{{ID: a.ID}}, ExpectedProjectRevision: 1, Key: uuid.New()}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.Target = source
	if err := request.Validate(); !errors.Is(err, domain.ErrInvalidLibrary) {
		t.Fatal("same or arbitrary scopes admitted", err)
	}
	request.Target = target
	for _, invalid := range []mediaapp.TransferInput{
		{Source: source, Target: target, Items: request.Items, ExpectedProjectRevision: math.MaxInt32 + 1, Key: request.Key},
		{Source: source, Target: target, Items: request.Items, ExpectedProjectRevision: 1, TargetFolderID: &job, ExpectedFolderRevision: math.MaxInt32 + 1, Key: request.Key},
	} {
		if err := invalid.Validate(); !errors.Is(err, domain.ErrInvalidLibrary) {
			t.Fatal("SQL revision capacity was not closed at the boundary", err)
		}
	}
	metadata := domain.LibraryItem{ID: a.ID, AssetID: &a.ID, LibraryID: uuid.New(), Title: "原图", Category: "material", Tags: []string{"标签"}, State: "active", Revision: 1}
	prepared, err := mediaapp.PrepareTransferItem(job, org, actor, target, nil, metadata, &mediaapp.LibraryMediaFile{Asset: a}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.TargetAsset == nil || prepared.TargetAsset.ID == a.ID || prepared.TargetAsset.ProjectID != uuid.Nil || prepared.TargetAsset.Personal == nil || prepared.TargetAsset.Personal.OrgID != org || prepared.TargetAsset.Personal.ActorID != actor || prepared.TargetAsset.ObjectKey == a.ObjectKey || prepared.TargetItem.ID != prepared.TargetAsset.ID || prepared.TargetItem.FolderID != nil {
		t.Fatal("independent closed personal ownership missing", prepared)
	}
	again, err := mediaapp.PrepareTransferItem(job, org, actor, target, nil, metadata, &mediaapp.LibraryMediaFile{Asset: a}, prepared.TargetAsset.CreateTime)
	if err != nil || again.TargetAsset.ID != prepared.TargetAsset.ID || again.TargetAsset.ObjectKey != prepared.TargetAsset.ObjectKey {
		t.Fatal("replay identity changed", err)
	}
	a.ContainsRealPerson = true
	if _, err := mediaapp.PrepareTransferItem(job, org, actor, target, nil, metadata, &mediaapp.LibraryMediaFile{Asset: a}, time.Now()); !errors.Is(err, mediaapp.ErrProjectCopyConsentUnavailable) {
		t.Fatal("person metadata bypassed consent owner", err)
	}
}

func TestMediaTransferPersonalNoteAndFavoriteDoNotBecomeSharedProjectMetadata(t *testing.T) {
	project, org, actor, job := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	id := uuid.New()
	text := "选中的文本正文"
	source := domain.LibraryItem{ID: id, LibraryID: uuid.New(), PlainText: &text, Title: "素材标题", Category: "other", Tags: []string{"标签"}, Note: "个人秘密笔记", Favorite: true, State: "active", Revision: 1}
	target := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	prepared, err := mediaapp.PrepareTransferItem(job, org, actor, target, nil, source, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.TargetAsset != nil || len(prepared.Objects) != 0 || prepared.TargetItem.ID == source.ID || prepared.TargetItem.Note != "" || prepared.TargetItem.Favorite || prepared.TargetItem.PlainText == nil || *prepared.TargetItem.PlainText != text {
		t.Fatal("personal metadata leaked or text omitted", prepared)
	}
	*prepared.TargetItem.PlainText = "独立修改"
	if *source.PlainText != text {
		t.Fatal("editable target aliases source")
	}
}
