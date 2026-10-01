package catalog_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialschema"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialseal"
	cataloghttp "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/paramvalidation"
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/pricevalidation"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func adminCatalogRouter(t *testing.T, database *gorm.DB, actor identityapp.Principal) *gin.Engine {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	sealer, err := credentialseal.NewSealer("http-test-key", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))
	if err != nil {
		t.Fatal(err)
	}
	params, err := paramvalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	price, err := pricevalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	store, registry := pgcatalog.NewStore(database), credentialschema.NewRegistry()
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	router.Use(func(c *gin.Context) { c.Set("principal", actor) })
	cataloghttp.NewAdminHandler(cataloghttp.AdminDependencies{Providers: catalogapp.NewListProvidersQuery(store), Provider: catalogapp.NewProviderDetailQuery(store, registry), CreateProvider: catalogapp.NewCreateProviderCommand(store, registry, time.Now), UpdateProvider: catalogapp.NewUpdateProviderCommand(store, time.Now), SetCredential: catalogapp.NewSetCredentialCommand(store, registry, sealer, time.Now), DisableCredential: catalogapp.NewDisableCredentialCommand(store, time.Now), CredentialTest: catalogapp.NewRequestCredentialTestCommand(store), Models: catalogapp.NewAdminModelsQuery(store), CreateModel: catalogapp.NewCreateModelCommand(store, time.Now), PublishVersion: catalogapp.NewPublishModelVersionCommand(store, params, time.Now), PublishPrice: catalogapp.NewPublishPriceRuleCommand(store, price, time.Now), SetModelStatus: catalogapp.NewSetModelStatusCommand(store, time.Now)}).Register(router.Group("/api"))
	return router
}

func adminCatalogRequest(t *testing.T, router *gin.Engine, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, request)
	return result
}

func TestAdminCatalogHTTPManagementKeepsRolesAndRealPublication(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	org := insertModelCatalogOrganization(ctx, t, database)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: org, Role: "admin"}
	if err := database.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES(?::uuid,?::uuid,?,'HTTP Catalog Admin','admin','synthetic-test-hash',false)`, actor.ID, org, "http-admin-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	producer := insertModelCatalogActor(ctx, t, database, org)
	denied := adminCatalogRequest(t, adminCatalogRouter(t, database, producer), "POST", "/api/admin/providers", `{"key":"denied"}`, uuid.NewString())
	if denied.Code != 403 {
		t.Fatalf("producer was elevated=%d %s", denied.Code, denied.Body)
	}
	router := adminCatalogRouter(t, database, actor)
	res := adminCatalogRequest(t, router, "POST", "/api/admin/providers", fmt.Sprintf(`{"key":"http-provider-%s","name":"HTTP Channel","adapter_key":"volcengine_ark","region":"domestic","concurrency_limit":2,"rate_limit_per_min":60}`, uuid.New()), uuid.NewString())
	var provider catalogapp.CreatedProvider
	if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &provider) != nil || provider.ID == uuid.Nil {
		t.Fatalf("create provider=%d %s", res.Code, res.Body)
	}
	providerPath := "/api/admin/providers/" + provider.ID.String()
	res = adminCatalogRequest(t, router, "PATCH", providerPath, `{"expected_revision":1,"concurrency_limit":3}`, uuid.NewString())
	if res.Code != 200 {
		t.Fatalf("update provider=%d %s", res.Code, res.Body)
	}
	res = adminCatalogRequest(t, router, "PATCH", providerPath, `{"expected_revision":1,"rate_limit_per_min":90}`, uuid.NewString())
	if res.Code != 409 {
		t.Fatalf("stale provider changed=%d %s", res.Code, res.Body)
	}
	res = adminCatalogRequest(t, router, "PUT", providerPath+"/credentials", `{"label":"本机测试","secret":{"api_key":"SYNTHETIC-http-only-1234"}}`, uuid.NewString())
	var credential catalogapp.SavedCredential
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &credential) != nil || credential.Last4 != "1234" || strings.Contains(res.Body.String(), "SYNTHETIC") || strings.Contains(res.Body.String(), "ciphertext") {
		t.Fatalf("credential boundary=%d %s", res.Code, res.Body)
	}
	res = adminCatalogRequest(t, router, "GET", providerPath, "", "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"credential_schema"`) || strings.Contains(res.Body.String(), "SYNTHETIC") || strings.Contains(res.Body.String(), "key_id") {
		t.Fatalf("provider detail=%d %s", res.Code, res.Body)
	}
	testPath := providerPath + "/credentials/" + credential.ID.String() + "/test"
	testKey := uuid.NewString()
	res = adminCatalogRequest(t, router, "POST", testPath, "", testKey)
	var accepted catalogapp.CredentialTestAccepted
	if res.Code != 202 || json.Unmarshal(res.Body.Bytes(), &accepted) != nil || !accepted.Accepted || strings.Contains(res.Body.String(), `"result":"ok"`) {
		t.Fatalf("test acceptance=%d %s", res.Code, res.Body)
	}
	replay := adminCatalogRequest(t, router, "POST", testPath, "", testKey)
	var again catalogapp.CredentialTestAccepted
	if replay.Code != 202 || json.Unmarshal(replay.Body.Bytes(), &again) != nil || again != accepted {
		t.Fatalf("test replay=%d %s", replay.Code, replay.Body)
	}
	if err := pgcatalog.NewStore(database).VerifyCredentialTestRequest(ctx, catalogapp.CredentialTestRequest{TestID: accepted.TestID, ProviderID: provider.ID, CredentialID: credential.ID, ActorID: actor.ID, OrgID: org, RequestID: testKey}, accepted.EventID); err != nil {
		t.Fatalf("durable test request=%v", err)
	}
	capability := domain.Capability{ID: uuid.New(), Key: "http.image." + uuid.NewString(), OutputType: domain.OutputImage, Modes: []string{"text_to_image"}, InputRoles: []string{"prompt"}}
	if err := pgcatalog.NewStore(database).CreateCapabilityForAdmin(ctx, actor.ID, org, capability); err != nil {
		t.Fatal(err)
	}
	res = adminCatalogRequest(t, router, "POST", "/api/admin/models", fmt.Sprintf(`{"model_key":"http-model-%s","provider_id":"%s","capability":"%s","display_name":"HTTP Model"}`, uuid.New(), provider.ID, capability.Key), uuid.NewString())
	var model catalogapp.CreatedModel
	if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &model) != nil || model.ID == uuid.Nil {
		t.Fatalf("create model=%d %s", res.Code, res.Body)
	}
	modelPath := "/api/admin/models/" + model.ID.String()
	version := `{"expected_revision":1,"version_no":1,"provider_model_id":"http-image","modes":["text_to_image"],"limits":{"max_outputs":2},"param_schema":[],"supports_query":true,"supports_cancel":false,"supports_callback":false,"expected_max_ms":30000,"moderation":"platform","queue":"agent"}`
	res = adminCatalogRequest(t, router, "POST", modelPath+"/versions", version, uuid.NewString())
	if res.Code != 201 {
		t.Fatalf("publish version=%d %s", res.Code, res.Body)
	}
	res = adminCatalogRequest(t, router, "POST", modelPath+"/prices", fmt.Sprintf(`{"expected_revision":2,"version_no":1,"unit":"per_image","rule":{"base_micros":10},"currency":"CNY","fx_rate_to_cny":"","effective_from":"%s"}`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)), uuid.NewString())
	if res.Code != 201 {
		t.Fatalf("publish price=%d %s", res.Code, res.Body)
	}
	res = adminCatalogRequest(t, router, "PATCH", modelPath+"/status", `{"expected_revision":3,"status":"active"}`, uuid.NewString())
	if res.Code != 200 {
		t.Fatalf("enable model=%d %s", res.Code, res.Body)
	}
	res = adminCatalogRequest(t, router, "GET", modelPath, "", "")
	var detail catalogapp.AdminModelDetail
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &detail) != nil || detail.Model.Revision != 4 || len(detail.Versions) != 1 || len(detail.Prices) != 1 {
		t.Fatalf("model editor restore=%d %s", res.Code, res.Body)
	}
	res = adminCatalogRequest(t, router, "POST", providerPath+"/credentials/"+credential.ID.String()+"/disable", "", uuid.NewString())
	if res.Code != 200 {
		t.Fatalf("disable credential=%d %s", res.Code, res.Body)
	}
	if err := database.Exec(`UPDATE identity."user" SET role='producer' WHERE id=?::uuid`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	res = adminCatalogRequest(t, router, "GET", modelPath, "", "")
	if res.Code != 403 {
		t.Fatalf("revoked admin remained authorized=%d %s", res.Code, res.Body)
	}
}
