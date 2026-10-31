package media_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type pickerQueryLibrary struct {
	unavailableLibrary
	calls int
	query mediaapp.LibraryQuery
}

func (r *pickerQueryLibrary) ListLibrary(_ context.Context, _ identity.Principal, _ domain.LibraryScope, q mediaapp.LibraryQuery) (mediaapp.LibraryPage, error) {
	r.calls++
	r.query = q
	return mediaapp.LibraryPage{}, nil
}
func TestLibraryPickerHTTPStrictQueryBeforeRepository(t *testing.T) {
	repo := &pickerQueryLibrary{}
	router := gin.New()
	mediahttp.NewLibraryHandler(repo).Register(router.Group("/api"))
	base := "/api/media/library?scope=project&project_id=" + uuid.NewString() + "&kind=image&page=2&page_size=40"
	valid := "&created_from=2026-10-01T00%3A00%3A00Z&created_before=2026-10-03T00%3A00%3A00Z&origin=upload&resolutions=1K,2K&ratios=16%3A9,1%3A1"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, base+valid, nil))
	if response.Code != 200 || repo.calls != 1 || repo.query.Page != 2 || repo.query.Origin != "upload" || len(repo.query.Ratios) != 2 || len(repo.query.Resolutions) != 2 || repo.query.CreatedFrom == nil || repo.query.CreatedBefore == nil {
		t.Fatal("formal picker query not passed intact", response.Code, repo.query)
	}
	for _, suffix := range []string{"&origin=upload&origin=generated", "&created_from=invalid", "&created_from=2026-10-04T00%3A00%3A00Z&created_before=2026-10-03T00%3A00%3A00Z", "&resolutions=", "&ratios=16%3A9,16%3A9", "&origin=super_resolution", "&favorite_only=true", "&private_actor_id=" + uuid.NewString()} {
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, base+suffix, nil))
		if response.Code != 422 || repo.calls != 1 {
			t.Fatalf("invalid query reached repository: %q status %d", suffix, response.Code)
		}
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, strings.ReplaceAll(base, "kind=image", "kind=audio")+valid, nil))
	if response.Code != 422 || repo.calls != 1 {
		t.Fatal("image predicates applied to audio")
	}
}
