package media_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestLibraryScopeHasClosedOwnership(t *testing.T) {
	project := uuid.New()
	for _, scope := range []domain.LibraryScope{{Kind: domain.LibraryPersonal}, {Kind: domain.LibraryProject, ProjectID: &project}} {
		if err := scope.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, scope := range []domain.LibraryScope{{}, {Kind: "cloud"}, {Kind: domain.LibraryProject}, {Kind: domain.LibraryPersonal, ProjectID: &project}} {
		if err := scope.Validate(); !errors.Is(err, domain.ErrInvalidLibrary) {
			t.Fatal("open or ambiguous ownership", scope, err)
		}
	}
}

func TestPersonalMediaCannotBecomeAProjectReference(t *testing.T) {
	org, actor, id := uuid.New(), uuid.New(), uuid.New()
	at := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	asset := domain.MediaAsset{ID: id, Personal: &domain.PersonalOwnership{OrgID: org, ActorID: actor}, Kind: domain.KindImage, Origin: domain.OriginUpload, Status: domain.StatusReady,
		ObjectKey: "personal/" + org.String() + "/" + actor.String() + "/image/2026/10/" + id.String() + ".png", FileName: "a.png", MimeType: "image/png", ByteSize: 1,
		ModerationStatus: domain.ModerationPassed, Revision: 1, CreateTime: at, UpdateTime: at}
	if err := asset.Validate(); err != nil {
		t.Fatal("valid private ownership", err)
	}
	if asset.CanReference() {
		t.Fatal("personal media bypasses project reference scope")
	}
	asset.ProjectID = uuid.New()
	if !errors.Is(asset.Validate(), domain.ErrInvalidMediaAsset) {
		t.Fatal("mixed ownership accepted")
	}
	asset.ProjectID = uuid.Nil
	asset.Personal.ActorID = uuid.New()
	if !errors.Is(asset.Validate(), domain.ErrInvalidMediaAsset) {
		t.Fatal("foreign personal object key accepted")
	}
}

func TestProjectMediaNilPersonalJSONRemainsAbsent(t *testing.T) {
	data, err := json.Marshal(domain.MediaAsset{ID: uuid.New(), ProjectID: uuid.New()})
	if err != nil || strings.Contains(strings.ToLower(string(data)), "personal") {
		t.Fatal("nil personal field changes historical JSON", string(data), err)
	}
}

func TestLibraryFolderPreservesFlatAndEightLevelSemantics(t *testing.T) {
	lib := uuid.New()
	makeFolder := func(kind domain.LibraryKind, parent *uuid.UUID) domain.LibraryFolder {
		return domain.LibraryFolder{ID: uuid.New(), LibraryID: lib, Kind: kind, ParentID: parent, Name: "素材", Style: "glass", Theme: "aurora", Revision: 1}
	}
	personal := makeFolder(domain.LibraryPersonal, nil)
	personal.Style, personal.Theme = "", ""
	if err := personal.Validate(); err != nil {
		t.Fatal(err)
	}
	personal.ParentID = &lib
	if !errors.Is(personal.Validate(), domain.ErrInvalidLibrary) {
		t.Fatal("personal tree accepted")
	}
	personal.ParentID, personal.Name = nil, strings.Repeat("中", 41)
	if !errors.Is(personal.Validate(), domain.ErrInvalidLibrary) {
		t.Fatal("personal name budget accepted")
	}
	var folders []domain.LibraryFolder
	var parent *uuid.UUID
	for range 8 {
		folder := makeFolder(domain.LibraryProject, parent)
		if err := domain.ValidateLibraryFolderPlacement(folder, folders); err != nil {
			t.Fatal("valid eight level source hierarchy", err)
		}
		folders = append(folders, folder)
		parent = &folders[len(folders)-1].ID
	}
	if err := domain.ValidateLibraryFolderPlacement(makeFolder(domain.LibraryProject, parent), folders); !errors.Is(err, domain.ErrInvalidLibrary) {
		t.Fatal("ninth level accepted", err)
	}
	first := folders[0]
	first.ParentID = &folders[1].ID
	if err := domain.ValidateLibraryFolderPlacement(first, folders); !errors.Is(err, domain.ErrInvalidLibrary) {
		t.Fatal("cycle accepted", err)
	}
	foreign := makeFolder(domain.LibraryProject, parent)
	foreign.LibraryID = uuid.New()
	if err := domain.ValidateLibraryFolderPlacement(foreign, folders); !errors.Is(err, domain.ErrInvalidLibrary) {
		t.Fatal("foreign parent accepted", err)
	}
}

func TestLibraryMetadataCannotOverwriteAssetExecutionFacts(t *testing.T) {
	id, lib, asset := uuid.New(), uuid.New(), uuid.New()
	item := domain.LibraryItem{ID: id, LibraryID: lib, AssetID: &asset, Title: "素材", Category: "material", State: "active", Revision: 1, Tags: []string{"雨夜"}}
	if err := item.Validate(); err != nil {
		t.Fatal(err)
	}
	body := "正文"
	item.PlainText = &body
	if !errors.Is(item.Validate(), domain.ErrInvalidLibrary) {
		t.Fatal("media and fake text simultaneously accepted")
	}
	item.AssetID = nil
	if err := item.Validate(); err != nil {
		t.Fatal("real plain note", err)
	}
	item.Category = "generated"
	if !errors.Is(item.Validate(), domain.ErrInvalidLibrary) {
		t.Fatal("open category accepted")
	}
	item.Category, item.Tags = "other", []string{"a", "a"}
	if !errors.Is(item.Validate(), domain.ErrInvalidLibrary) {
		t.Fatal("duplicate tags accepted")
	}
}
