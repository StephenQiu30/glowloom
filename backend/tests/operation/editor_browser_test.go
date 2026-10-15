package operation_test

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	cataloghttp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/http"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/http"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	prompthttp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/http"
)

// TestEditorBrowserFixture exposes only synthetic isolated data on loopback, when explicitly requested.
func TestEditorBrowserFixture(t *testing.T) {
	if os.Getenv("LV_EDITOR_BROWSER_FIXTURE") != "1" {
		t.Skip("interactive browser fixture not requested")
	}
	db := operationStoreDB(t)
	actor, p := operationStoreProject(t, db)
	catalog, model, provider := seedQuoteCatalog(t, db, p, time.Now().UTC())
	for _, command := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE catalog.provider SET status='active' WHERE id=?`, []any{provider}},
		{`INSERT INTO catalog.capability(id,key,output_type,modes,input_roles) VALUES(?,'image.generate','image',ARRAY['text2image','image2image'],ARRAY['prompt','reference_image','first_frame']) ON CONFLICT(key) DO NOTHING`, []any{uuid.New()}},
		{`UPDATE catalog.model_profile SET capability='image.generate' WHERE id=?`, []any{model}},
		{`UPDATE catalog.model_profile_version SET modes=ARRAY['text2image','image2image'],limits='{"max_outputs":4,"roles":{"reference_image":{"types":["image"],"max_count":4,"max_bytes":10000},"first_frame":{"types":["image"],"max_count":1,"max_bytes":10000}}}',param_schema='[{"field":"aspect_ratio","label":"画幅","type":"string","component":"select","enum":["1:1","16:9"],"default":"1:1"}]' WHERE id=?`, []any{*catalog.ModelProfileVersionID}},
	} {
		if err := db.Exec(command.sql, command.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"参考图片甲.png", "参考图片乙.png"} {
		id := uuid.New()
		if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,'image','upload','ready',?,?,'image/png',50,repeat('a',64),'passed')`, id, p, "projects/"+p.String()+"/image/2026/10/"+id.String()+".png", name).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := gin.New()
	api := router.Group("/api")
	api.Use(func(c *gin.Context) { c.Set("principal", actor) })
	api.GET("/session", func(c *gin.Context) {
		c.JSON(200, gin.H{"actor": gin.H{"id": actor.ID, "org_id": actor.OrgID, "role": actor.Role}, "session_id": uuid.NewString()})
	})
	mediahttp.NewHandler(mediaapp.NewAssetQuery(mediapg.NewScopedStore(db, app.NewMediaScopeAccess), nil)).Register(api)
	prompthttp.NewDraftHandler(editorDraftStore(db)).Register(api)
	store := editorOperationStore(db)
	operationhttp.NewSubmissionHandler(store).Register(api)
	operationhttp.NewHandler(operationhttp.Dependencies{FreeQuote: operationapp.NewCreateFreeQuoteCommand(store), Confirm: operationapp.NewConfirmSingleQuoteCommand(store), Query: operationapp.NewPublicQuery(store)}).Register(api)
	cataloghttp.NewHandler(catalogapp.NewListModelsQuery(app.NewCatalogStore(db))).Register(api)
	var listenerConfig net.ListenConfig
	listener, err := listenerConfig.Listen(t.Context(), "tcp", "127.0.0.1:55588")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second}
	t.Cleanup(func() { _ = server.Close() })
	info, _ := json.Marshal(map[string]string{"project": p.String(), "actor": actor.ID.String(), "model": catalog.ModelProfileVersionID.String()})
	if err := os.WriteFile("/tmp/glowloom-editor-browser.json", info, 0600); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	t.Log("synthetic editor browser fixture ready")
	select {
	case <-t.Context().Done():
	case <-time.After(30 * time.Minute):
	}
}
