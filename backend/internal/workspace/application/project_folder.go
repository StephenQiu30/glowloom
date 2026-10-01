package application

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// FolderMediaReader obtains eligible cover facts and retains owner reference locks.
type FolderMediaReader interface {
	Reference(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (mediaapp.AssetSummary, error)
}

// FolderChangeInput is a closed directory command; absence of a target means root.
type FolderChangeInput struct {
	Action                    string
	FolderID                  uuid.UUID
	ExpectedRevision          int64
	Name                      *string
	SetCover                  bool
	Cover                     *domain.FolderCover
	ProjectID                 uuid.UUID
	ExpectedProjectRevision   int64
	ExpectedPlacementRevision int64
	IdempotencyKey            uuid.UUID
	RequestID                 string
}

// Validate prevents settings, execution facts, or unrelated identities crossing commands.
func (i FolderChangeInput) Validate() error {
	request, err := uuid.Parse(i.RequestID)
	if i.IdempotencyKey == uuid.Nil || err != nil || request == uuid.Nil || request.String() != i.RequestID || i.ExpectedRevision < 0 || i.ExpectedRevision >= math.MaxInt32 || i.ExpectedProjectRevision < 0 || i.ExpectedProjectRevision >= math.MaxInt32 || i.ExpectedPlacementRevision < 0 || i.ExpectedPlacementRevision >= math.MaxInt32 || i.Name != nil && !domain.ValidFolderName(*i.Name) || i.Cover != nil && (!i.SetCover || i.Cover.AssetID == uuid.Nil || i.Cover.ProjectID == uuid.Nil) {
		return domain.ErrInvalidProjectFolder
	}
	switch i.Action {
	case "create":
		if i.FolderID != uuid.Nil || i.ExpectedRevision != 0 || i.Name == nil || i.ProjectID != uuid.Nil || i.ExpectedProjectRevision != 0 || i.ExpectedPlacementRevision != 0 {
			return domain.ErrInvalidProjectFolder
		}
	case "patch":
		if i.FolderID == uuid.Nil || i.ExpectedRevision < 1 || i.Name == nil && !i.SetCover || i.ProjectID != uuid.Nil || i.ExpectedProjectRevision != 0 || i.ExpectedPlacementRevision != 0 {
			return domain.ErrInvalidProjectFolder
		}
	case "recycle":
		if i.FolderID == uuid.Nil || i.ExpectedRevision < 1 || i.Name != nil || i.SetCover || i.ProjectID != uuid.Nil || i.ExpectedProjectRevision != 0 || i.ExpectedPlacementRevision != 0 {
			return domain.ErrInvalidProjectFolder
		}
	case "move":
		if i.ProjectID == uuid.Nil || i.ExpectedProjectRevision < 1 || i.Name != nil || i.SetCover || (i.FolderID == uuid.Nil) != (i.ExpectedRevision == 0) {
			return domain.ErrInvalidProjectFolder
		}
	default:
		return domain.ErrInvalidProjectFolder
	}
	return nil
}

// FolderChangeResult retains one durable response without leaking private media locations.
type FolderChangeResult struct {
	Folder             *domain.ProjectFolder   `json:"folder,omitempty"`
	Placement          *domain.FolderPlacement `json:"placement,omitempty"`
	RecycledProjectIDs []uuid.UUID             `json:"recycled_project_ids,omitempty"`
}

// FolderListInput selects a bounded page ordered by update time and stable identity.
type FolderListInput struct {
	Limit int
	After *ProjectListCursor
}

// FolderSummary reports actual visible membership and currently authorized cover facts.
type FolderSummary struct {
	Folder           domain.ProjectFolder
	ProjectCount     int64
	CoverUnavailable bool
}

// FolderListPage never silently truncates the directory library.
type FolderListPage struct {
	Items []FolderSummary
	Next  *ProjectListCursor
}

// ProjectFoldersStore owns actor authorization and atomic directory commands.
type ProjectFoldersStore interface {
	ChangeFolder(context.Context, identityapp.Principal, FolderChangeInput, time.Time) (FolderChangeResult, error)
	ListFolders(context.Context, identityapp.Principal, FolderListInput) (FolderListPage, error)
}

// ProjectFolders coordinates a flat library without altering project content identity.
type ProjectFolders struct {
	store ProjectFoldersStore
	now   func() time.Time
}

// NewProjectFolders injects directory persistence and a clock.
func NewProjectFolders(store ProjectFoldersStore, now func() time.Time) *ProjectFolders {
	return &ProjectFolders{store: store, now: now}
}

// Change returns the first committed response or an explicit conflict.
func (s *ProjectFolders) Change(ctx context.Context, actor identityapp.Principal, in FolderChangeInput) (FolderChangeResult, error) {
	if !canManageModelDefaults(actor) {
		return FolderChangeResult{}, identityapp.ErrForbidden
	}
	if err := in.Validate(); err != nil {
		return FolderChangeResult{}, err
	}
	if s == nil || s.store == nil || s.now == nil {
		return FolderChangeResult{}, ErrProjectDependencyUnavailable
	}
	out, err := s.store.ChangeFolder(ctx, actor, in, s.now().UTC())
	if err != nil {
		return FolderChangeResult{}, fmt.Errorf("change project folder: %w", err)
	}
	if out.Folder == nil && out.Placement == nil || in.Action == "move" && out.Placement == nil || in.Action != "move" && out.Folder == nil {
		return FolderChangeResult{}, ErrProjectDependencyUnavailable
	}
	if f := out.Folder; f != nil && (f.Validate() != nil || f.OrgID != actor.OrgID || f.ActorID != actor.ID) {
		return FolderChangeResult{}, ErrProjectDependencyUnavailable
	}
	if p := out.Placement; p != nil && (p.Validate() != nil || p.OrgID != actor.OrgID || p.ActorID != actor.ID || p.ProjectID != in.ProjectID) {
		return FolderChangeResult{}, ErrProjectDependencyUnavailable
	}
	return out, nil
}

// List returns one authorized directory page, with unavailable covers kept explicit.
func (s *ProjectFolders) List(ctx context.Context, actor identityapp.Principal, in FolderListInput) (FolderListPage, error) {
	if !canManageModelDefaults(actor) {
		return FolderListPage{}, identityapp.ErrForbidden
	}
	if in.Limit == 0 {
		in.Limit = 50
	}
	if in.Limit < 1 || in.Limit > 200 || in.After != nil && (in.After.ID == uuid.Nil || in.After.UpdateTime.IsZero()) {
		return FolderListPage{}, domain.ErrInvalidProjectFolder
	}
	if s == nil || s.store == nil {
		return FolderListPage{}, ErrProjectDependencyUnavailable
	}
	out, err := s.store.ListFolders(ctx, actor, in)
	if err != nil {
		return FolderListPage{}, err
	}
	if len(out.Items) > in.Limit || out.Next != nil && len(out.Items) == 0 {
		return FolderListPage{}, ErrProjectDependencyUnavailable
	}
	for _, item := range out.Items {
		if item.Folder.Validate() != nil || item.Folder.OrgID != actor.OrgID || item.Folder.ActorID != actor.ID || item.Folder.IsDelete || item.ProjectCount < 0 {
			return FolderListPage{}, ErrProjectDependencyUnavailable
		}
	}
	if out.Next != nil {
		last := out.Items[len(out.Items)-1].Folder
		if out.Next.ID != last.ID || !out.Next.UpdateTime.Equal(last.UpdateTime) {
			return FolderListPage{}, ErrProjectDependencyUnavailable
		}
	}
	if out.Items == nil {
		out.Items = []FolderSummary{}
	}
	return out, nil
}
