package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

type listModelsStore struct {
	actor  identityapp.Principal
	input  catalogapp.ListModelsInput
	page   catalogapp.ModelCatalogPage
	err    error
	called bool
}

func (s *listModelsStore) ListModelsForProject(_ context.Context, actor identityapp.Principal, input catalogapp.ListModelsInput) (catalogapp.ModelCatalogPage, error) {
	s.called, s.actor, s.input = true, actor, input
	return s.page, s.err
}

func TestListModelsQueryAllowsCurrentAdminAndProducerAndBoundsPage(t *testing.T) {
	actor := adminPrincipal()
	projectID := uuid.New()
	for _, role := range []identitydomain.Role{identitydomain.RoleAdmin, identitydomain.RoleProducer} {
		t.Run(string(role), func(t *testing.T) {
			caller := actor
			caller.Role = role
			store := &listModelsStore{page: catalogapp.ModelCatalogPage{Models: []catalogapp.ModelCatalogItem{}}}
			page, err := catalogapp.NewListModelsQuery(store).Execute(t.Context(), caller, catalogapp.ListModelsInput{
				ProjectID: projectID, Capability: "video.generate", Mode: "image2video",
			})
			if err != nil || !store.called || store.actor != caller || store.input.ProjectID != projectID ||
				store.input.Capability != "video.generate" || store.input.Mode != "image2video" ||
				store.input.Limit != 50 || page.Models == nil {
				t.Fatalf("model page %+v, store %+v: %v", page, store, err)
			}
		})
	}

	cursor := &catalogapp.ModelCatalogCursor{Key: "ark.seedance-2-pro", ID: uuid.New()}
	store := &listModelsStore{page: catalogapp.ModelCatalogPage{Models: []catalogapp.ModelCatalogItem{}}}
	_, err := catalogapp.NewListModelsQuery(store).Execute(t.Context(), actor, catalogapp.ListModelsInput{
		ProjectID: projectID, Limit: 200, After: cursor,
	})
	if err != nil || !store.called || store.input.Limit != 200 || store.input.After != cursor {
		t.Fatalf("bounded model page %+v: %v", store.input, err)
	}
}

func TestListModelsQueryRejectsInvalidPrincipalAndPageInputBeforeStorage(t *testing.T) {
	actor := adminPrincipal()
	projectID := uuid.New()
	valid := catalogapp.ListModelsInput{ProjectID: projectID}
	for _, tc := range []struct {
		name  string
		actor identityapp.Principal
		input catalogapp.ListModelsInput
		want  error
	}{
		{"missing account", identityapp.Principal{OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, valid, identityapp.ErrForbidden},
		{"missing organization", identityapp.Principal{ID: actor.ID, Role: identitydomain.RoleProducer}, valid, identityapp.ErrForbidden},
		{"unsupported role", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: "viewer"}, valid, identityapp.ErrForbidden},
		{"must change password", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer, MustChangePassword: true}, valid, identityapp.ErrForbidden},
		{"missing project", actor, catalogapp.ListModelsInput{}, catalogapp.ErrInvalidModelCatalog},
		{"negative limit", actor, catalogapp.ListModelsInput{ProjectID: projectID, Limit: -1}, catalogapp.ErrInvalidModelCatalog},
		{"over limit", actor, catalogapp.ListModelsInput{ProjectID: projectID, Limit: 201}, catalogapp.ErrInvalidModelCatalog},
		{"cursor missing key", actor, catalogapp.ListModelsInput{ProjectID: projectID, After: &catalogapp.ModelCatalogCursor{ID: uuid.New()}}, catalogapp.ErrInvalidModelCatalog},
		{"cursor missing ID", actor, catalogapp.ListModelsInput{ProjectID: projectID, After: &catalogapp.ModelCatalogCursor{Key: "ark.seedance-2-pro"}}, catalogapp.ErrInvalidModelCatalog},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &listModelsStore{}
			_, err := catalogapp.NewListModelsQuery(store).Execute(t.Context(), tc.actor, tc.input)
			if !errors.Is(err, tc.want) || store.called {
				t.Fatalf("invalid model page reached storage: %v", err)
			}
		})
	}

	if _, err := catalogapp.NewListModelsQuery(nil).Execute(t.Context(), actor, valid); !errors.Is(err, catalogapp.ErrInvalidModelCatalog) {
		t.Fatalf("nil store: %v", err)
	}
}

func TestListModelsQueryPropagatesStorageErrorAndRejectsInvalidPage(t *testing.T) {
	actor := adminPrincipal()
	input := catalogapp.ListModelsInput{ProjectID: uuid.New(), Limit: 1}
	failed := errors.New("storage unavailable")
	store := &listModelsStore{err: failed}
	_, err := catalogapp.NewListModelsQuery(store).Execute(t.Context(), actor, input)
	if !store.called || !errors.Is(err, failed) {
		t.Fatalf("storage error was lost: %v", err)
	}

	for _, tc := range []struct {
		name string
		page catalogapp.ModelCatalogPage
	}{
		{"too many models", catalogapp.ModelCatalogPage{Models: make([]catalogapp.ModelCatalogItem, 2)}},
		{"next without models", catalogapp.ModelCatalogPage{
			Models: []catalogapp.ModelCatalogItem{},
			Next:   &catalogapp.ModelCatalogCursor{Key: "ark.seedance-2-pro", ID: uuid.New()},
		}},
		{"invalid next", catalogapp.ModelCatalogPage{
			Models: make([]catalogapp.ModelCatalogItem, 1),
			Next:   &catalogapp.ModelCatalogCursor{ID: uuid.New()},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &listModelsStore{page: tc.page}
			_, err := catalogapp.NewListModelsQuery(store).Execute(t.Context(), actor, input)
			if !store.called || !errors.Is(err, catalogapp.ErrInvalidModelCatalog) {
				t.Fatalf("invalid stored model page: %v", err)
			}
		})
	}
}
