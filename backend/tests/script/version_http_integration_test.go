package script_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	scripthttp "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/http"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func TestScriptVersionHTTPActualCandidateTextConfirmationHistoryAndAdoption(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	store := scriptStore(db)
	objects := &sourceObjects{data: make(map[string][]byte)}
	sources := app.NewSourceService(store, objects, time.Now)
	episodes := app.NewEpisodeService(store, sources, time.Now)
	history := app.NewHistoryService(store, sources)
	_, input := sourceCommand()
	input.ProjectID = pid
	input.Sources[0].Document = domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "甲😀乙"}}}}}
	saved, err := sources.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	router := scriptReviewRouter(sources, episodes, history, actor)
	g := router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	scripthttp.NewVersionHandler(history, app.NewAdoptService(store, time.Now)).Register(g)
	path := "/api/projects/" + pid.String() + "/script-versions/" + saved.VersionID.String()
	text := scriptHTTP(t, router, "GET", path+"/source-text?from=1&to=2", uuid.Nil, nil)
	var part app.VersionText
	if err := json.Unmarshal(text.Body.Bytes(), &part); err != nil || text.Code != 200 || part.Text != "😀" || part.ContentHash != domain.ContentSHA([]byte("甲😀乙")) {
		t.Fatal("candidate scalar text", text.Code, part, err)
	}
	invalid := scriptHTTP(t, router, "GET", path+"/source-text?from=0&to=65537", uuid.Nil, nil)
	if invalid.Code != 422 {
		t.Fatal("oversize range", invalid.Code)
	}
	view, err := episodes.Episodes(t.Context(), actor, pid, saved.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	command := app.SplitCommand{ProjectID: pid, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 1, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}
	first, err := episodes.ConfirmSplit(t.Context(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	command.Key = uuid.New()
	command.ExpectedRevision = 2
	command.ExpectedSplitRevision = 1
	command.Boundaries[0].Title = "修改后集名"
	if _, err := episodes.ConfirmSplit(t.Context(), actor, command); err != nil {
		t.Fatal(err)
	}
	pageResponse := scriptHTTP(t, router, "GET", path+"/split-confirmations?limit=1", uuid.Nil, nil)
	var page app.ConfirmationPage
	if err := json.Unmarshal(pageResponse.Body.Bytes(), &page); err != nil || pageResponse.Code != 200 || len(page.Items) != 1 || page.NextRevision == nil || page.Items[0].Revision != 2 {
		t.Fatal("complete chronological history", pageResponse.Code, page, err)
	}
	detailResponse := scriptHTTP(t, router, "GET", path+"/split-confirmations/"+first.ConfirmationID.String(), uuid.Nil, nil)
	var detail domain.SplitConfirmation
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detail); err != nil || detailResponse.Code != 200 || len(detail.Episodes) != 1 || detail.Episodes[0].Title == "修改后集名" || detail.Episodes[0].ID != first.Episodes[0].ID {
		t.Fatal("past partition reconstructed from current heads", detailResponse.Code, detail, err)
	}
	rev, splitRev := int64(3), int64(2)
	adoptResponse := scriptHTTP(t, router, "POST", path+"/adopt", uuid.New(), scripthttp.AdoptVersionRequest{ExpectedRevision: &rev, ExpectedSplitRevision: &splitRev})
	if adoptResponse.Code != 200 {
		t.Fatal("actual adopt route", adoptResponse.Code, adoptResponse.Body.String())
	}
	foreign, foreignPid := scriptActorProject(t, owner)
	foreignRouter := scriptReviewRouter(sources, episodes, history, foreign)
	fg := foreignRouter.Group("/api")
	fg.Use(func(c *gin.Context) { c.Set("principal", foreign); c.Next() })
	scripthttp.NewVersionHandler(history, app.NewAdoptService(store, time.Now)).Register(fg)
	r := scriptHTTP(t, foreignRouter, "GET", path+"/source-text?from=0&to=1", uuid.Nil, nil)
	if r.Code != 404 {
		t.Fatal("foreign version body exposed", foreignPid, r.Code)
	}
}
