package workspace_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectFolderClosedCommands(t *testing.T) {
	name := "目录"
	valid := workspaceapp.FolderChangeInput{Action: "create", Name: &name, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for label, mutate := range map[string]func(*workspaceapp.FolderChangeInput){
		"tree":            func(i *workspaceapp.FolderChangeInput) { i.Action = "create_tree" },
		"unknown version": func(i *workspaceapp.FolderChangeInput) { i.ExpectedRevision = 1 },
		"project":         func(i *workspaceapp.FolderChangeInput) { i.ProjectID = uuid.New() },
		"missing key":     func(i *workspaceapp.FolderChangeInput) { i.IdempotencyKey = uuid.Nil },
		"empty":           func(i *workspaceapp.FolderChangeInput) { s := "  "; i.Name = &s },
		"nul":             func(i *workspaceapp.FolderChangeInput) { s := "bad\x00"; i.Name = &s },
		"oversized":       func(i *workspaceapp.FolderChangeInput) { s := strings.Repeat("文", 161); i.Name = &s },
		"invalid utf8":    func(i *workspaceapp.FolderChangeInput) { s := string([]byte{0xff}); i.Name = &s },
		"partial cover": func(i *workspaceapp.FolderChangeInput) {
			i.SetCover = true
			i.Cover = &domain.FolderCover{AssetID: uuid.New()}
		},
	} {
		t.Run(label, func(t *testing.T) {
			input := valid
			mutate(&input)
			if !errors.Is(input.Validate(), domain.ErrInvalidProjectFolder) {
				t.Fatal("invalid input accepted")
			}
		})
	}
	move := workspaceapp.FolderChangeInput{Action: "move", ProjectID: uuid.New(), ExpectedProjectRevision: 1, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
	if err := move.Validate(); err != nil {
		t.Fatal(err)
	}
	move.FolderID = uuid.New()
	if err := move.Validate(); err == nil {
		t.Fatal("folder move without observed target revision accepted")
	}
	move.ExpectedRevision = 1
	if err := move.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectFolderDomainPreservesFlatScopeAndRecovery(t *testing.T) {
	now := time.Now().UTC()
	folder := domain.ProjectFolder{ID: uuid.New(), OrgID: uuid.New(), ActorID: uuid.New(), Name: "目录", Revision: 1, CreateTime: now, UpdateTime: now}
	if err := folder.Validate(); err != nil {
		t.Fatal(err)
	}
	folder.IsDelete = true
	if err := folder.Validate(); err == nil {
		t.Fatal("deleted directory without persisted date accepted")
	}
	folder.DeleteTime = &now
	if err := folder.Validate(); err != nil {
		t.Fatal(err)
	}
	placement := domain.FolderPlacement{OrgID: folder.OrgID, ActorID: folder.ActorID, ProjectID: uuid.New(), Revision: 0}
	if err := placement.Validate(); err != nil {
		t.Fatal(err)
	}
	placement.FolderID = &folder.ID
	if err := placement.Validate(); err == nil {
		t.Fatal("virtual root placement has folder")
	}
}

type folderBoundaryStore struct {
	calls  int
	result workspaceapp.FolderChangeResult
}

func (s *folderBoundaryStore) ChangeFolder(context.Context, identityapp.Principal, workspaceapp.FolderChangeInput, time.Time) (workspaceapp.FolderChangeResult, error) {
	s.calls++
	return s.result, nil
}
func (*folderBoundaryStore) ListFolders(context.Context, identityapp.Principal, workspaceapp.FolderListInput) (workspaceapp.FolderListPage, error) {
	return workspaceapp.FolderListPage{}, nil
}

func TestProjectFolderServiceRejectsUntrustedOwnerResponse(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	name := "目录"
	input := workspaceapp.FolderChangeInput{Action: "create", Name: &name, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
	store := &folderBoundaryStore{}
	service := workspaceapp.NewProjectFolders(store, time.Now)
	if _, err := service.Change(context.Background(), actor, input); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatalf("incomplete response accepted: %v", err)
	}
	bad := input
	bad.Action = "execute"
	if _, err := service.Change(context.Background(), actor, bad); !errors.Is(err, domain.ErrInvalidProjectFolder) || store.calls != 1 {
		t.Fatal("invalid input reached store")
	}
}
