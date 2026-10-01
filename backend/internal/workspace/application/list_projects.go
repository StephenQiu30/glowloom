package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ErrInvalidProjectList means a list request or its stored page is invalid.
var ErrInvalidProjectList = errors.New("invalid project list")

// ProjectListCursor identifies the last row of a descending keyset page.
// Encoding an opaque public cursor belongs to the HTTP adapter.
type ProjectListCursor struct {
	UpdateTime time.Time
	ID         uuid.UUID
}

// ProjectListItem contains the fields needed by the project list and recycle bin.
type ProjectListItem struct {
	ID                  uuid.UUID
	OrgID               uuid.UUID
	Name                string
	AspectRatio         string
	StyleType           string
	StyleSubtype        string
	StylePresetID       *uuid.UUID
	Resolution          string
	AllowOverseasModels bool
	Status              string
	IsDelete            bool
	ArchivedAt          *time.Time
	DeleteTime          *time.Time
	PurgeAfter          *time.Time
	Revision            int64
	CreateTime          time.Time
	UpdateTime          time.Time
	FolderID            *uuid.UUID
	PlacementRevision   int64
}

// ProjectListPage contains one bounded page of project summaries.
type ProjectListPage struct {
	Projects []ProjectListItem
	Next     *ProjectListCursor
}

// ListProjectsInput selects current or deleted projects in an organization.
type ListProjectsInput struct {
	Status  string
	Deleted bool
	Query   string
	Limit   int
	After   *ProjectListCursor
	// FolderID is nil for all projects, uuid.Nil for root, or a personal folder.
	FolderID *uuid.UUID
}

// ListProjectsStore rechecks current rights before reading a project page.
type ListProjectsStore interface {
	ListProjectsForActor(context.Context, identityapp.Principal, ListProjectsInput) (ProjectListPage, error)
}

// ListProjectsQuery validates an organization-scoped project list request.
type ListProjectsQuery struct{ store ListProjectsStore }

// NewListProjectsQuery injects the authorized project reader.
func NewListProjectsQuery(store ListProjectsStore) *ListProjectsQuery {
	return &ListProjectsQuery{store: store}
}

// Execute returns one page ordered by update time and ID, newest first.
func (q *ListProjectsQuery) Execute(ctx context.Context, actor identityapp.Principal, input ListProjectsInput) (ProjectListPage, error) {
	if q == nil || q.store == nil {
		return ProjectListPage{}, ErrInvalidProjectList
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return ProjectListPage{}, identityapp.ErrForbidden
	}
	input.Query = strings.TrimSpace(input.Query)
	if (input.Status != "" && input.Status != "active" && input.Status != "archived") ||
		input.Limit < 0 || input.Limit > 200 || !utf8.ValidString(input.Query) ||
		utf8.RuneCountInString(input.Query) > 200 ||
		(input.After != nil && (input.After.ID == uuid.Nil || input.After.UpdateTime.IsZero())) ||
		(input.Deleted && input.FolderID != nil) {
		return ProjectListPage{}, ErrInvalidProjectList
	}
	for _, r := range input.Query {
		if unicode.IsControl(r) {
			return ProjectListPage{}, ErrInvalidProjectList
		}
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	page, err := q.store.ListProjectsForActor(ctx, actor, input)
	if err != nil {
		return ProjectListPage{}, fmt.Errorf("list projects: %w", err)
	}
	if len(page.Projects) > input.Limit ||
		(page.Next != nil && (len(page.Projects) == 0 || page.Next.ID == uuid.Nil || page.Next.UpdateTime.IsZero())) {
		return ProjectListPage{}, ErrInvalidProjectList
	}
	for _, project := range page.Projects {
		if project.ID == uuid.Nil || project.OrgID != actor.OrgID || project.IsDelete != input.Deleted ||
			project.Name == "" || project.Revision < 1 || project.UpdateTime.IsZero() ||
			(project.Status != "active" && project.Status != "archived") ||
			(input.Status != "" && project.Status != input.Status) {
			return ProjectListPage{}, ErrInvalidProjectList
		}
		if project.PlacementRevision < 0 || project.FolderID != nil && (*project.FolderID == uuid.Nil || project.PlacementRevision < 1) {
			return ProjectListPage{}, ErrInvalidProjectList
		}
		if input.FolderID != nil && (*input.FolderID == uuid.Nil && project.FolderID != nil || *input.FolderID != uuid.Nil && (project.FolderID == nil || *project.FolderID != *input.FolderID)) {
			return ProjectListPage{}, domain.ErrInvalidProjectFolder
		}
	}
	if page.Next != nil {
		last := page.Projects[len(page.Projects)-1]
		if page.Next.ID != last.ID || !page.Next.UpdateTime.Equal(last.UpdateTime) {
			return ProjectListPage{}, ErrInvalidProjectList
		}
	}
	if page.Projects == nil {
		page.Projects = []ProjectListItem{}
	}
	return page, nil
}
