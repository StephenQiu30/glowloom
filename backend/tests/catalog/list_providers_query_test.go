package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

type listProvidersStore struct {
	limit  int
	after  *catalogapp.ProviderListCursor
	called bool
}

func (s *listProvidersStore) ListProvidersForAdmin(_ context.Context, _, _ uuid.UUID, limit int, after *catalogapp.ProviderListCursor) (catalogapp.ProviderListPage, error) {
	s.called, s.limit, s.after = true, limit, after
	return catalogapp.ProviderListPage{Providers: []catalogapp.ProviderListItem{}}, nil
}

func TestListProvidersQueryBoundsPageAndChecksPrincipal(t *testing.T) {
	actor := adminPrincipal()
	store := &listProvidersStore{}
	page, err := catalogapp.NewListProvidersQuery(store).Execute(t.Context(), actor, catalogapp.ListProvidersInput{})
	if err != nil || !store.called || store.limit != 50 || page.Providers == nil {
		t.Fatalf("default provider page %+v, limit %d: %v", page, store.limit, err)
	}
	store.called = false
	cursor := &catalogapp.ProviderListCursor{CreateTime: time.Now(), ID: uuid.New()}
	_, err = catalogapp.NewListProvidersQuery(store).Execute(t.Context(), actor, catalogapp.ListProvidersInput{Limit: 200, After: cursor})
	if err != nil || !store.called || store.limit != 200 || store.after != cursor {
		t.Fatalf("bounded provider page limit %d: %v", store.limit, err)
	}
	for _, tc := range []struct {
		name  string
		actor identityapp.Principal
		input catalogapp.ListProvidersInput
		want  error
	}{
		{"producer", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, catalogapp.ListProvidersInput{}, identityapp.ErrForbidden},
		{"first login", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, catalogapp.ListProvidersInput{}, identityapp.ErrForbidden},
		{"limit", actor, catalogapp.ListProvidersInput{Limit: 201}, catalogapp.ErrInvalidProviderList},
		{"cursor", actor, catalogapp.ListProvidersInput{After: &catalogapp.ProviderListCursor{ID: uuid.New()}}, catalogapp.ErrInvalidProviderList},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &listProvidersStore{}
			_, err := catalogapp.NewListProvidersQuery(store).Execute(t.Context(), tc.actor, tc.input)
			if !errors.Is(err, tc.want) || store.called {
				t.Fatalf("invalid list reached storage: %v", err)
			}
		})
	}
}

func TestProviderListItemJSONHasNoCredentialCiphertext(t *testing.T) {
	item := catalogapp.ProviderListItem{
		Credential: &catalogapp.ProviderCredentialSummary{Last4: "A9F2"}, ModelCount: 3,
	}
	encoded, err := json.Marshal(item)
	if err != nil || strings.Contains(string(encoded), "ciphertext") ||
		strings.Contains(string(encoded), "key_id") || strings.Contains(string(encoded), "secret") {
		t.Fatalf("unsafe provider list item %s: %v", encoded, err)
	}
}
