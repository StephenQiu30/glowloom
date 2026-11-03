package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	cataloghttp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type declarationSaveStore struct {
	application.PresetStore
	saves int
}

func (s *declarationSaveStore) SavePreset(_ context.Context, _ identityapp.Principal, input application.SavePresetInput) (domain.Preset, error) {
	s.saves++
	return domain.Preset{Document: input.Document}, nil
}

func TestAdapterDeclarationDirectCreateAndVersionRejectAmbiguousDocument(t *testing.T) {
	for _, path := range []string{"/api/admin/provider-presets", "/api/admin/provider-presets/" + uuid.NewString() + "/versions"} {
		t.Run(path, func(t *testing.T) {
			store := &declarationSaveStore{}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("principal", identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"})
			})
			cataloghttp.NewPresetHandler(application.NewDeclaredProviderPresets(store, installedProtocol)).Register(router.Group("/api"))
			doc := declarationPreset(t)
			document, err := json.Marshal(doc)
			require.NoError(t, err)
			declaration, err := json.Marshal(doc.AdapterDeclaration)
			require.NoError(t, err)
			for _, ambiguous := range []string{
				strings.TrimSuffix(string(document), "}") + `,"adapter_declaration":` + string(declaration) + `}`,
				strings.Replace(string(document), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
				strings.Replace(string(document), `"capabilities":[`, `"region":"overseas","capabilities":[`, 1),
			} {
				revision := 0
				if strings.HasSuffix(path, "/versions") {
					revision = 1
				}
				body := fmt.Sprintf(`{"slug":"restricted","request_id":%q,"expected_revision":%d,"preview_hash":%q,"document":%s}`, uuid.NewString(), revision, doc.Digest(), ambiguous)
				request := httptest.NewRequestWithContext(t.Context(), "POST", path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				require.Equal(t, 422, response.Code, response.Body.String())
				require.Zero(t, store.saves)
			}

			revision := 0
			if strings.HasSuffix(path, "/versions") {
				revision = 1
			}
			body := fmt.Sprintf(`{"slug":"restricted","request_id":%q,"expected_revision":%d,"preview_hash":%q,"document":%s}`, uuid.NewString(), revision, doc.Digest(), document)
			request := httptest.NewRequestWithContext(t.Context(), "POST", path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, 201, response.Code, response.Body.String())
			require.Equal(t, 1, store.saves)
		})
	}
}
