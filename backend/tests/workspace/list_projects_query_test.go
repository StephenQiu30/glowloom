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
)

type listProjectsQueryStore struct {
	actor  identityapp.Principal
	input  workspaceapp.ListProjectsInput
	page   workspaceapp.ProjectListPage
	err    error
	called bool
}

func (s *listProjectsQueryStore) ListProjectsForActor(_ context.Context, actor identityapp.Principal, input workspaceapp.ListProjectsInput) (workspaceapp.ProjectListPage, error) {
	s.called, s.actor, s.input = true, actor, input
	return s.page, s.err
}

func projectListItem(orgID uuid.UUID) workspaceapp.ProjectListItem {
	return workspaceapp.ProjectListItem{
		ID: uuid.New(), OrgID: orgID, Name: "逆光", Status: "active",
		Revision: 1, UpdateTime: time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC),
	}
}

func TestListProjectsQueryScopesAndBoundsPage(t *testing.T) {
	for _, role := range []identitydomain.Role{identitydomain.RoleAdmin, identitydomain.RoleProducer} {
		t.Run(string(role), func(t *testing.T) {
			actor := projectCommandActor(role)
			store := &listProjectsQueryStore{}
			page, err := workspaceapp.NewListProjectsQuery(store).Execute(t.Context(), actor, workspaceapp.ListProjectsInput{
				Status: "active", Query: "  逆光  ",
			})
			if err != nil || !store.called || store.actor != actor || store.input.Status != "active" ||
				store.input.Query != "逆光" || store.input.Limit != 50 || page.Projects == nil {
				t.Fatalf("default project page=%+v store=%+v err=%v", page, store, err)
			}
		})
	}

	actor := projectCommandActor(identitydomain.RoleProducer)
	cursor := &workspaceapp.ProjectListCursor{UpdateTime: time.Now().UTC(), ID: uuid.New()}
	store := &listProjectsQueryStore{}
	_, err := workspaceapp.NewListProjectsQuery(store).Execute(t.Context(), actor, workspaceapp.ListProjectsInput{
		Status: "archived", Deleted: true, Query: "旧项目", Limit: 200, After: cursor,
	})
	if err != nil || !store.called || store.input.Limit != 200 || store.input.After != cursor ||
		store.input.Status != "archived" || !store.input.Deleted || store.input.Query != "旧项目" {
		t.Fatalf("bounded project page store=%+v err=%v", store, err)
	}
}

func TestListProjectsQueryRejectsInvalidCallerAndInputBeforeStorage(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	validCursor := &workspaceapp.ProjectListCursor{UpdateTime: time.Now().UTC(), ID: uuid.New()}
	for _, tc := range []struct {
		name  string
		actor identityapp.Principal
		input workspaceapp.ListProjectsInput
		want  error
	}{
		{"missing actor", identityapp.Principal{OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, workspaceapp.ListProjectsInput{}, identityapp.ErrForbidden},
		{"missing organization", identityapp.Principal{ID: actor.ID, Role: identitydomain.RoleProducer}, workspaceapp.ListProjectsInput{}, identityapp.ErrForbidden},
		{"unsupported role", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: "viewer"}, workspaceapp.ListProjectsInput{}, identityapp.ErrForbidden},
		{"must change password", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: actor.Role, MustChangePassword: true}, workspaceapp.ListProjectsInput{}, identityapp.ErrForbidden},
		{"invalid status", actor, workspaceapp.ListProjectsInput{Status: "paused"}, workspaceapp.ErrInvalidProjectList},
		{"negative limit", actor, workspaceapp.ListProjectsInput{Limit: -1}, workspaceapp.ErrInvalidProjectList},
		{"over limit", actor, workspaceapp.ListProjectsInput{Limit: 201}, workspaceapp.ErrInvalidProjectList},
		{"cursor missing ID", actor, workspaceapp.ListProjectsInput{After: &workspaceapp.ProjectListCursor{UpdateTime: validCursor.UpdateTime}}, workspaceapp.ErrInvalidProjectList},
		{"cursor missing time", actor, workspaceapp.ListProjectsInput{After: &workspaceapp.ProjectListCursor{ID: validCursor.ID}}, workspaceapp.ErrInvalidProjectList},
		{"invalid query UTF-8", actor, workspaceapp.ListProjectsInput{Query: string([]byte{0xff})}, workspaceapp.ErrInvalidProjectList},
		{"query control character", actor, workspaceapp.ListProjectsInput{Query: "逆\n光"}, workspaceapp.ErrInvalidProjectList},
		{"query too long", actor, workspaceapp.ListProjectsInput{Query: strings.Repeat("剧", 201)}, workspaceapp.ErrInvalidProjectList},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &listProjectsQueryStore{}
			_, err := workspaceapp.NewListProjectsQuery(store).Execute(t.Context(), tc.actor, tc.input)
			if !errors.Is(err, tc.want) || store.called {
				t.Fatalf("invalid project list reached storage: err=%v called=%v", err, store.called)
			}
		})
	}

	if _, err := workspaceapp.NewListProjectsQuery(nil).Execute(t.Context(), actor, workspaceapp.ListProjectsInput{}); !errors.Is(err, workspaceapp.ErrInvalidProjectList) {
		t.Fatalf("nil store: %v", err)
	}
}

func TestListProjectsQueryPreservesStoreFailureAndRejectsInvalidPage(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	input := workspaceapp.ListProjectsInput{Limit: 1}
	sentinel := errors.New("storage unavailable")
	store := &listProjectsQueryStore{err: sentinel}
	_, err := workspaceapp.NewListProjectsQuery(store).Execute(t.Context(), actor, input)
	if !store.called || !errors.Is(err, sentinel) {
		t.Fatalf("store error chain lost: %v", err)
	}

	validItem := projectListItem(actor.OrgID)
	validCursor := &workspaceapp.ProjectListCursor{UpdateTime: validItem.UpdateTime, ID: validItem.ID}
	for _, tc := range []struct {
		name string
		page workspaceapp.ProjectListPage
	}{
		{"too many projects", workspaceapp.ProjectListPage{Projects: []workspaceapp.ProjectListItem{validItem, validItem}}},
		{"next without projects", workspaceapp.ProjectListPage{Next: validCursor}},
		{"next missing ID", workspaceapp.ProjectListPage{Projects: []workspaceapp.ProjectListItem{validItem}, Next: &workspaceapp.ProjectListCursor{UpdateTime: validItem.UpdateTime}}},
		{"next missing time", workspaceapp.ProjectListPage{Projects: []workspaceapp.ProjectListItem{validItem}, Next: &workspaceapp.ProjectListCursor{ID: validItem.ID}}},
		{"foreign organization", workspaceapp.ProjectListPage{Projects: []workspaceapp.ProjectListItem{projectListItem(uuid.New())}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &listProjectsQueryStore{page: tc.page}
			_, err := workspaceapp.NewListProjectsQuery(store).Execute(t.Context(), actor, input)
			if !store.called || !errors.Is(err, workspaceapp.ErrInvalidProjectList) {
				t.Fatalf("invalid project page accepted: err=%v page=%+v", err, tc.page)
			}
		})
	}
}
