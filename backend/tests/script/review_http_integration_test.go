package script_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	scripthttp "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/http"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

func scriptReviewRouter(sources *app.SourceService, episodes *app.EpisodeService, history *app.HistoryService, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	g := router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	scripthttp.NewSourceHandler(sources).Register(g)
	scripthttp.NewReviewHandler(episodes, history).Register(g)
	return router
}
func TestScriptReviewHTTPRealRulesManualHistoryAndTypedImpact(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	store := scriptStore(db)
	objects := &sourceObjects{data: make(map[string][]byte)}
	sources := app.NewSourceService(store, objects, time.Now)
	episodes := app.NewEpisodeService(store, sources, time.Now)
	history := app.NewHistoryService(store, sources)
	router := scriptReviewRouter(sources, episodes, history, actor)
	path := "/api/projects/" + pid.String()
	revision := int64(0)
	html := "<p>走进客厅<br>你好😀 </p>"
	create := scripthttp.SourceWriteRequest{ExpectedRevision: &revision, RightsConfirmed: true, Kind: "chapter", Title: "第一章", Status: "draft", OriginalHTML: &html}
	response := scriptHTTP(t, router, "POST", path+"/script-sources", uuid.New(), create)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var source app.SourceReceipt
	if err := json.Unmarshal(response.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/script-versions", "/script-sources/" + source.Mappings[0].LineageID.String() + "/history"} {
		r := scriptHTTP(t, router, "GET", path+route, uuid.Nil, nil)
		if r.Code != 200 || strings.Contains(r.Body.String(), "document") && strings.Contains(r.Body.String(), "content\"") {
			t.Fatal("history list", route, r.Code, r.Body.String())
		}
	}
	base, err := store.LoadBase(t.Context(), actor, pid, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := scriptHTTP(t, router, "GET", path+"/script-source-snapshots/"+base.Sources[0].ID.String(), uuid.Nil, nil)
	var original app.SourceSnapshotDetail
	if err := json.Unmarshal(snapshot.Body.Bytes(), &original); err != nil || snapshot.Code != 200 || original.OriginalHTML == nil || *original.OriginalHTML != html {
		t.Fatal("preserved original HTML history", snapshot.Code, err)
	}
	revision = 1
	splitRevision := int64(0)
	split := scripthttp.SplitReviewRequest{VersionID: source.VersionID, ExpectedRevision: &revision, ExpectedSplitRevision: &splitRevision, CandidateSetID: source.SplitSetID}
	r := scriptHTTP(t, router, "POST", path+"/episodes/split/resplit", uuid.New(), split)
	if r.Code != 200 {
		t.Fatal("real rules", r.Code, r.Body.String())
	}
	r = scriptHTTP(t, router, "GET", path+"/episodes?version_id="+source.VersionID.String(), uuid.Nil, nil)
	var view app.EpisodeView
	if err := json.Unmarshal(r.Body.Bytes(), &view); err != nil || r.Code != 200 || view.Candidate.Origin != "rules" || len(view.Candidate.Boundaries) != 1 || len(view.Episodes) != 0 {
		t.Fatal("actual rules candidate", view, err)
	}
	revision = 2
	splitRevision = 1
	split.CandidateSetID = view.Head.CandidateSetID
	split.Boundaries = view.Candidate.Boundaries
	r = scriptHTTP(t, router, "POST", path+"/episodes/split/confirm", uuid.New(), split)
	if r.Code != 200 {
		t.Fatal("formal HTTP confirm", r.Code, r.Body.String())
	}
	var formal app.SplitReceipt
	if err := json.Unmarshal(r.Body.Bytes(), &formal); err != nil {
		t.Fatal(err)
	}
	episodePath := "/api/episodes/" + formal.Episodes[0].ID.String()
	r = scriptHTTP(t, router, "GET", episodePath+"/source-text?from=5&to=8", uuid.Nil, nil)
	var text app.EpisodeText
	if err := json.Unmarshal(r.Body.Bytes(), &text); err != nil || r.Code != 200 || text.Text != "你好😀" {
		t.Fatal("actual scalar HTTP text", text, err)
	}
	revision = 3
	episodeRevision := int64(1)
	structureVersion := int64(0)
	document := manualStructure()
	save := scripthttp.StructureSaveRequest{ExpectedRevision: &revision, ExpectedEpisodeRevision: &episodeRevision, BaseStructureVersionNo: &structureVersion, Document: &document}
	r = scriptHTTP(t, router, "POST", episodePath+"/structure", uuid.New(), save)
	if r.Code != 200 {
		t.Fatal("manual HTTP save", r.Code, r.Body.String())
	}
	revision = 4
	episodeRevision = 2
	structureVersion = 1
	confirm := scripthttp.StructureConfirmRequest{ExpectedRevision: &revision, ExpectedEpisodeRevision: &episodeRevision, BaseStructureVersionNo: &structureVersion}
	r = scriptHTTP(t, router, "POST", episodePath+"/structure/confirm", uuid.New(), confirm)
	if r.Code != 200 {
		t.Fatal("first manual HTTP confirm", r.Code, r.Body.String())
	}
	revision = 5
	episodeRevision = 3
	document.Scenes[0].Items[1].Content = "欢迎回来😀"
	r = scriptHTTP(t, router, "POST", episodePath+"/structure", uuid.New(), save)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	revision = 6
	episodeRevision = 4
	structureVersion = 2
	r = scriptHTTP(t, router, "POST", episodePath+"/structure/confirm", uuid.New(), confirm)
	if r.Code != 409 || !strings.Contains(r.Body.String(), "affected_episodes") || !strings.Contains(r.Body.String(), formal.Episodes[0].ID.String()) {
		t.Fatal("actual typed impact missing", r.Code, r.Body.String())
	}
	confirm.AckInvalidate = true
	r = scriptHTTP(t, router, "POST", episodePath+"/structure/confirm", uuid.New(), confirm)
	if r.Code != 503 || !strings.Contains(r.Body.String(), "context_unavailable") {
		t.Fatal("missing actual downstream owner falsely confirmed", r.Code, r.Body.String())
	}
	for _, route := range []string{"/structure", "/structure/versions", "/structure/versions/1"} {
		r = scriptHTTP(t, router, "GET", episodePath+route, uuid.Nil, nil)
		if r.Code != 200 || strings.Contains(r.Body.String(), "object_key") {
			t.Fatal("private structure history", route, r.Code, r.Body.String())
		}
	}
	foreign, _ := scriptActorProject(t, owner)
	denied := scriptHTTP(t, scriptReviewRouter(sources, episodes, history, foreign), "GET", episodePath+"/structure/versions/1", uuid.Nil, nil)
	if denied.Code != 404 {
		t.Fatal("foreign manual history", denied.Code)
	}
}
