package subject_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	subjecthttp "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

func TestSubjectTypeSurvivesImmutableSave(t *testing.T) {
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New()}
	repo := newMemoryRepository(actor)
	media := fact("image")
	repo.media.facts[media.AssetID] = media
	service := application.NewService(repo, time.Now)
	for _, kind := range []string{"character", "scene", "prop", "product", "style", "other"} {
		t.Run(kind, func(t *testing.T) {
			payload := `{"name":"分类主体","type":"` + kind + `","primary":{"asset_id":"` + media.AssetID.String() + `"}}`
			var input application.SaveInput
			if err := json.Unmarshal([]byte(payload), &input); err != nil {
				t.Fatal(err)
			}
			saved, err := service.Save(context.Background(), actor, uuid.Nil, uuid.New(), input)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := json.Unmarshal(encoded, &result); err != nil {
				t.Fatal(err)
			}
			if result["type"] != kind {
				t.Fatalf("saved type = %v, want %s", result["type"], kind)
			}
			version := repo.versions[saved.VersionID]
			if err := version.Content.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSubjectHTTPValidatesTypeFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	query := &queryFake{}
	router := gin.New()
	subjecthttp.NewHandler(nil, query).Register(router.Group("/api"))
	for _, test := range []struct {
		query  string
		status int
	}{
		{"type=product", 200}, {"type=character", 200}, {"type=other", 200},
		{"type=unknown", 422}, {"type=product&type=style", 422},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", "/api/subjects?"+test.query, nil))
		if response.Code != test.status {
			t.Fatalf("%s: got %d, want %d", test.query, response.Code, test.status)
		}
	}
}

func TestUntypedSubjectKeepsLegacyDigest(t *testing.T) {
	c := content()
	primary, err := json.Marshal(c.Primary)
	if err != nil {
		t.Fatal(err)
	}
	legacy := []byte(fmt.Sprintf(`{"schema_version":1,"name":"主体","description":"","primary":%s,"references":[]}`, primary))
	want := sha256.Sum256(legacy)
	got, err := domain.Digest(c)
	if err != nil || got != hex.EncodeToString(want[:]) {
		t.Fatalf("legacy digest changed: %s %v", got, err)
	}
	c.Type = "unrecognized"
	if c.Validate() == nil {
		t.Fatal("accepted unknown subject type")
	}
}
