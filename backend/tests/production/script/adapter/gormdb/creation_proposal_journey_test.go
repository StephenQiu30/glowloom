package gormdb_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	biblehttp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/httpapi"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	creationgorm "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/gormdb"
	creationhttp "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/httpapi"
	creation "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardhttp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/httpapi"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewhttp "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/httpapi"
	review "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCreationProposalReviewAdoptionPersistsFormalOwnerAndRejectsStaleDecisions(t *testing.T) {
	db := creationDatabase(t)
	tx := beginSourceAcceptanceTestTransaction(t, db)
	ctx := context.Background()
	now := time.Now().UTC()
	raw, err := os.ReadFile("../../../../testdata/creation_text.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Source     map[string]any    `json:"source"`
		EpisodeMap json.RawMessage   `json:"episode_map"`
		Analyses   []json.RawMessage `json:"analyses"`
		World      json.RawMessage   `json:"world"`
		Direction  json.RawMessage   `json:"direction"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	seeded := seedSourceAcceptanceProject(t, func(value any) error { return tx.Create(value).Error }, now, fixture.Source["text"].(string))
	acceptCreationSource(t, tx, seeded)
	fixture.Source["revision_id"] = seeded.revisionID.String()
	store := creationgorm.New(tx)
	actor := creation.Actor{UserID: seeded.userID.String(), TokenVersion: 1}
	config := creation.Config{Endpoint: "https://fixture.invalid", Now: time.Now, NewID: uuid.NewString}
	run, err := creation.NewService(store, config).Create(ctx, actor, creation.CreateCommand{ProjectID: seeded.projectID.String(), DocumentRevisionID: seeded.revisionID.String(), SourceHash: seeded.normalizedHash, IdempotencyKey: "proposal-journey"})
	if err != nil {
		t.Fatal(err)
	}
	service := creation.NewProposalService(store, nil, config)
	auth := creationJourneyAuth{userID: actor.UserID}
	handler, err := creationhttp.NewProposalHandler(service, auth, strings.Repeat("a", 32), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	projects := projectapp.NewService(projectgorm.New(tx), time.Now, uuid.NewString)
	worldQuery := bibleapp.NewTextWorldQuery(biblegorm.New(tx), projects)
	intentQuery := storyboardapp.NewTextIntentQuery(storyboardgorm.New(tx), projects)
	biblehttp.NewTextWorldHandler(worldQuery, auth).Register(mux)
	storyboardhttp.NewTextIntentHandler(intentQuery, auth).Register(mux)

	reviews := review.NewService(reviewgorm.New(tx), review.Config{Now: time.Now, NewID: uuid.NewString, ClaimLease: 5 * time.Minute})
	reviewhttp.New(reviews, nil, auth).Register(mux)
	save := func(stage, episode string, candidate json.RawMessage) domain.Proposal {
		t.Helper()
		task := map[string]any{"invocation_id": uuid.NewString(), "stage": stage, "source": fixture.Source, "release_hash": strings.Repeat("a", 64), "episode_map": fixture.EpisodeMap, "analyses": fixture.Analyses, "world": fixture.World, "episode_key": episode, "scene_key": nil, "timeout_seconds": 300}
		if episode == "" {
			task["episode_key"] = nil
		}
		issues := []domain.Issue{}
		if stage == "direct_scene" {
			task["scene_key"] = "scene"
			issues = append(issues, domain.Issue{Code: "continuity_mapping_pending", Scope: "scene", Severity: "blocker", Summary: "需要人工检查跨场身份披露与状态"})
		}
		taskRaw, _ := json.Marshal(task)
		taskHash, _ := canonical.Hash(taskRaw)
		candidateHash, _ := canonical.Hash(candidate)
		resultRaw, _ := json.Marshal(domain.TextResult{InvocationID: task["invocation_id"].(string), Stage: stage, InputHash: taskHash, ReleaseHash: strings.Repeat("a", 64), Candidate: candidate, CandidateHash: candidateHash, Evidence: []domain.ResolvedEvidence{}, Issues: issues})
		resultHash, _ := canonical.Hash(resultRaw)
		stepKey := stage
		if episode != "" {
			stepKey += "/" + episode
		}
		if stage == "direct_scene" {
			stepKey += "/scene"
		}
		draft := domain.DraftEnvelope{Schema: "creation-draft-production", CommandID: run.Command.RunID, RunID: run.Command.RunID, PayloadHash: run.PayloadHash, SourceRevisionID: seeded.revisionID.String(), StepID: uuid.NewString(), StepKey: stepKey, DraftID: uuid.NewString(), Revision: 1, CandidateHash: candidateHash, ResultHash: resultHash, Task: taskRaw, Result: resultRaw}
		p, err := domain.ValidateDraft(run, draft)
		if err != nil {
			t.Fatal(err)
		}
		p, err = store.SaveProposal(ctx, actor, run, p, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	do := func(method, path string, payload any) map[string]json.RawMessage {
		t.Helper()
		body, _ := json.Marshal(payload)
		if payload == nil {
			body = nil
		}
		request := httptest.NewRequest(method, path, strings.NewReader(string(body)))
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("%s %s: %d %s", method, path, response.Code, response.Body.String())
		}
		var outer struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &outer); err != nil {
			t.Fatal(err)
		}
		return outer.Data
	}
	approve := func(p domain.Proposal) string {
		t.Helper()
		claimed := do(http.MethodPost, "/api/human-tasks/"+p.HumanTaskID+"/claims", map[string]any{"expected_revision": 1, "idempotency_key": uuid.NewString()})
		var claimToken string
		_ = json.Unmarshal(claimed["claim_token"], &claimToken)
		var task struct {
			Revision int `json:"revision"`
			Claim    struct {
				Token string `json:"claim_token"`
			} `json:"claim"`
		}
		_ = json.Unmarshal(claimed["task"], &task)
		if claimToken == "" {
			claimToken = task.Claim.Token
		}
		decided := do(http.MethodPost, "/api/human-tasks/"+p.HumanTaskID+"/decisions", map[string]any{"claim_token": claimToken, "expected_task_revision": task.Revision, "expected_subject_revision": p.Revision, "expected_subject_hash": p.ResultHash, "decision": "approved", "idempotency_key": uuid.NewString()})
		var decision struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(decided["decision"], &decision)
		if string(decided["coordination"]) != "null" || decision.ID == "" {
			t.Fatal("creation review was routed to legacy workflow")
		}
		do(http.MethodGet, "/api/human-tasks/"+p.HumanTaskID, nil)
		return decision.ID
	}
	mapProposal := save("map_manuscript", "", fixture.EpisodeMap)
	if _, err = service.Adopt(ctx, actor, run.Command.RunID, mapProposal.ID, creation.AdoptCommand{ExpectedRevision: 1, DecisionID: uuid.NewString(), IdempotencyKey: "unapproved"}); creationStatus(err) != 409 {
		t.Fatalf("unapproved adopted: %v", err)
	}
	decisionID := approve(mapProposal)
	input := creation.AdoptCommand{ExpectedRevision: 1, DecisionID: decisionID, IdempotencyKey: "adopt-map", RiskResolutions: []domain.RiskResolution{}}
	const failPublication = "test:creation-publication-failure"
	injected := errors.New("publication insert failed")
	publicationFailed := false
	if err = tx.Callback().Create().Before("gorm:create").Register(failPublication, func(db *gorm.DB) {
		if db.Statement.Table == "evt_outbox_events" {
			publicationFailed = true
			_ = db.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, failedAdoption := service.Adopt(ctx, actor, run.Command.RunID, mapProposal.ID, input)
	if err = tx.Callback().Create().Remove(failPublication); err != nil {
		t.Fatal(err)
	}
	if failedAdoption == nil || !publicationFailed {
		t.Fatalf("event failure must abort adoption: %v", failedAdoption)
	}
	for _, row := range []any{&model.Episode{}, &model.EpisodeScriptVersion{}, &model.OutboxEvent{}} {
		var count int64
		if err = tx.Model(row).Where("project_id = ?", seeded.projectID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("failed adoption left rows: %T count=%d err=%v", row, count, err)
		}
	}
	var failedReceipts int64
	if err = tx.Model(&model.CommandReceipt{}).Where("resource_id = ? AND operation = ?", mapProposal.ID, "creation.text.map_manuscript").Count(&failedReceipts).Error; err != nil || failedReceipts != 0 {
		t.Fatal("failed adoption left an owner receipt")
	}
	receipt, err := service.Adopt(ctx, actor, run.Command.RunID, mapProposal.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := creation.NewProposalService(creationgorm.New(tx), nil, config).Adopt(ctx, actor, run.Command.RunID, mapProposal.ID, input)
	if err != nil || replayed.SubmissionID != receipt.SubmissionID {
		t.Fatalf("receipt did not survive service restart: %v", err)
	}
	input.ExpectedRevision = 2
	if _, err = service.Adopt(ctx, actor, run.Command.RunID, mapProposal.ID, input); creationStatus(err) != 409 {
		t.Fatalf("stale revision adopted: %v", err)
	}
	var episodes []model.Episode
	if err = tx.Where("project_id = ?", seeded.projectID).Find(&episodes).Error; err != nil || len(episodes) != 2 {
		t.Fatalf("formal episodes=%d err=%v", len(episodes), err)
	}
	var publicationEvents []model.OutboxEvent
	if err = tx.Where("project_id = ? AND event_type = ?", seeded.projectID, "ScriptVersionPublished").Find(&publicationEvents).Error; err != nil || len(publicationEvents) != len(episodes) {
		t.Fatalf("adoption/replay must publish exactly one event per episode: events=%d episodes=%d err=%v", len(publicationEvents), len(episodes), err)
	}
	for _, event := range publicationEvents {
		if event.SourceReceiptID.String() != receipt.OwnerReceipts[0].ID || event.Status != "pending" {
			t.Fatal("publication event must reference the committed adoption receipt")
		}
		var payload struct {
			ScriptVersionID string `json:"script_version_id"`
			ContentHash     string `json:"content_hash"`
		}
		if err = json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		var version model.EpisodeScriptVersion
		if err = tx.First(&version, "id = ? AND episode_id = ? AND status = ?", payload.ScriptVersionID, event.AggregateID, "published").Error; err != nil || version.ContentHash != payload.ContentHash {
			t.Fatalf("publication event does not resolve to exact formal script: %v", err)
		}
	}
	refs := []domain.DraftRef{}
	for i, candidate := range fixture.Analyses {
		key := []string{"episode-one", "episode-two"}[i]
		p := save("analyze_episode", key, candidate)
		id := approve(p)
		if _, err = service.Adopt(ctx, actor, run.Command.RunID, p.ID, creation.AdoptCommand{ExpectedRevision: 1, DecisionID: id, IdempotencyKey: uuid.NewString()}); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, domain.DraftRef{StepID: p.StepID, DraftID: p.ID, ResultHash: p.ResultHash, CandidateHash: p.CandidateHash})
	}
	if _, err = store.GateState(ctx, actor, run.Command.RunID, "analyze_episode", refs[:1]); creationStatus(err) != 409 {
		t.Fatalf("partial episode gate advanced: %v", err)
	}
	gate, err := store.GateState(ctx, actor, run.Command.RunID, "analyze_episode", refs)
	if err != nil || gate.Status != "accepted" || len(gate.SelectedScenes) != 2 {
		t.Fatalf("structure gate=%+v err=%v", gate, err)
	}
	var formalStructure model.EpisodeStructure
	if err = tx.Where("project_id = ?", seeded.projectID).Order("created_at, id").First(&formalStructure).Error; err != nil {
		t.Fatal(err)
	}
	var savedScenes []struct {
		TextFacts struct {
			Mentions []struct {
				VisualDetails []struct {
					ExactAnchor string `json:"exact_anchor"`
				} `json:"visual_details"`
			} `json:"mentions"`
		} `json:"text_facts"`
	}
	if err = json.Unmarshal(formalStructure.Scenes, &savedScenes); err != nil {
		t.Fatal(err)
	}
	retained := false
	for _, scene := range savedScenes {
		for _, mention := range scene.TextFacts.Mentions {
			for _, detail := range mention.VisualDetails {
				if detail.ExactAnchor == "三角缺口" {
					retained = true
				}
			}
		}
	}
	if !retained {
		t.Fatal("persisted scene lost visual source evidence")
	}
	world := save("build_world", "", fixture.World)
	id := approve(world)
	worldReceipt, err := service.Adopt(ctx, actor, run.Command.RunID, world.ID, creation.AdoptCommand{ExpectedRevision: 1, DecisionID: id, IdempotencyKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if worldReceipt.IDMapping["world"] == "" || worldReceipt.IDMapping["entity/hidden-zhouye"] == "" {
		t.Fatal("formal world mappings missing")
	}

	frozenWorld, queryErr := worldQuery.Get(ctx, bibleapp.Actor{UserID: actor.UserID, TokenVersion: 1}, run.Command.ProjectID, worldReceipt.IDMapping["world"])
	if queryErr != nil || frozenWorld.ID != worldReceipt.IDMapping["world"] {
		t.Fatalf("formal world query: %v", queryErr)
	}
	do(http.MethodGet, "/api/projects/"+run.Command.ProjectID+"/text-world-versions/"+frozenWorld.ID, nil)
	if _, queryErr = worldQuery.Get(ctx, bibleapp.Actor{UserID: uuid.NewString(), TokenVersion: 1}, run.Command.ProjectID, frozenWorld.ID); queryErr == nil {
		t.Fatal("unauthorized world query succeeded")
	}
	if _, queryErr = biblegorm.New(tx).ReadTextWorld(ctx, run.Command.WorkspaceID, uuid.NewString(), frozenWorld.ID); !errors.Is(queryErr, bibleapp.ErrNotFound) {
		t.Fatalf("cross-project world query: %v", queryErr)
	}
	directRefs := []domain.DraftRef{}
	for _, epKey := range []string{"episode-one", "episode-two"} {
		candidate := fixture.Direction
		if epKey == "episode-two" {
			var direction contract.TextDirection
			if err = json.Unmarshal(candidate, &direction); err != nil {
				t.Fatal(err)
			}
			direction.EpisodeKey = epKey
			direction.DramaticIntent = "揭示蒙面人的身份"
			direction.AudienceKnows = []string{"周野就是蒙面人"}
			direction.Withhold = []string{}
			direction.Blocking = nil
			direction.Shots[0].Key = "reveal"
			direction.Shots[0].Purpose = "揭示身份"
			direction.Shots[0].Action = "周野摘下面罩"
			direction.Shots[0].VisibleMentions = []string{"zhou"}
			direction.Shots[0].Audio = nil
			direction.Shots[0].DetailEvidence = nil
			candidate, _ = json.Marshal(direction)
		}
		p := save("direct_scene", epKey, candidate)
		decisionID := approve(p)
		adopt := creation.AdoptCommand{ExpectedRevision: 1, DecisionID: decisionID, IdempotencyKey: uuid.NewString()}
		if _, err = service.Adopt(ctx, actor, run.Command.RunID, p.ID, adopt); creationStatus(err) != 409 {
			t.Fatalf("continuity blocker was skipped: %v", err)
		}
		adopt.RiskResolutions = []domain.RiskResolution{{Code: "continuity_mapping_pending", Scope: "scene", Reason: "已逐场核对：首场保留蒙面身份，次场才披露周野身份，场景时间与出入状态一致。"}}
		if epKey == "episode-one" {
			injected := errors.New("synthetic owner receipt failure")
			callback := "test_fail_text_intent_receipt"
			if err = tx.Callback().Create().Before("gorm:create").Register(callback, func(query *gorm.DB) {
				if record, ok := query.Statement.Dest.(*model.CommandReceipt); ok && record.Operation == "creation.text.direct_scene" {
					_ = query.AddError(injected)
				}
			}); err != nil {
				t.Fatal(err)
			}
			_, adoptErr := service.Adopt(ctx, actor, run.Command.RunID, p.ID, adopt)
			if removeErr := tx.Callback().Create().Remove(callback); removeErr != nil {
				t.Fatal(removeErr)
			}
			if !errors.Is(adoptErr, injected) {
				t.Fatalf("receipt failure was hidden: %v", adoptErr)
			}
			var count int64
			if err = tx.Model(&model.TextIntentVersion{}).Where("proposal_id = ?", p.ID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("partial owner persisted after receipt failure: %d %v", count, err)
			}
		}
		do(http.MethodPost, "/api/creation-runs/"+run.Command.RunID+"/proposals/"+p.ID+"/adopt", adopt)
		accepted, err := service.Get(ctx, actor, run.Command.RunID, p.ID)
		if err != nil || accepted.Acceptance == nil {
			t.Fatalf("accepted intent not queryable: %v", err)
		}
		var version model.TextIntentVersion
		if err = tx.First(&version, "id = ?", accepted.Acceptance.IDMapping["intent/"+epKey+"/scene"]).Error; err != nil {
			t.Fatal(err)
		}
		var body storyboarddomain.TextIntentVersion
		if err = json.Unmarshal(version.Body, &body); err != nil || body.AssetReadiness != "needs_asset" || len(body.Shots) != 1 || body.Shots[0].ID != accepted.Acceptance.IDMapping["shot/"+epKey+"/scene/"+body.Shots[0].Key] {
			t.Fatalf("formal intent body or identity drift: %v", err)
		}

		frozenIntent, queryErr := intentQuery.Get(ctx, storyboardapp.Actor{UserID: actor.UserID, TokenVersion: 1}, run.Command.ProjectID, version.ID.String())
		if queryErr != nil || frozenIntent.ContentHash != body.ContentHash {
			t.Fatalf("formal intent query: %v", queryErr)
		}
		do(http.MethodGet, "/api/projects/"+run.Command.ProjectID+"/text-intent-versions/"+version.ID.String(), nil)
		if _, queryErr = intentQuery.Get(ctx, storyboardapp.Actor{UserID: actor.UserID, TokenVersion: 99}, run.Command.ProjectID, version.ID.String()); queryErr == nil {
			t.Fatal("revoked token read formal intent")
		}
		directRefs = append(directRefs, domain.DraftRef{StepID: p.StepID, DraftID: p.ID, ResultHash: p.ResultHash, CandidateHash: p.CandidateHash})
	}
	if _, err = store.GateState(ctx, actor, run.Command.RunID, "direct_scene", directRefs[:1]); creationStatus(err) != 409 {
		t.Fatalf("partial direct gate advanced: %v", err)
	}
	completed, err := store.GateState(ctx, actor, run.Command.RunID, "direct_scene", directRefs)
	if err != nil || completed.Status != "accepted" || len(completed.Receipts) != 2 {
		t.Fatalf("direct gate failed: %+v %v", completed, err)
	}
	for _, check := range []struct {
		model any
		want  int64
	}{{&model.TextWorldVersion{}, 1}, {&model.TextIntentVersion{}, 2}, {&model.StoryboardShot{}, 0}} {
		var count int64
		if err = tx.Model(check.model).Where("project_id = ?", seeded.projectID).Count(&count).Error; err != nil || count != check.want {
			t.Fatalf("formal owner %T count=%d want=%d err=%v", check.model, count, check.want, err)
		}
	}
	if err = tx.Model(&model.EpisodeStructure{}).Where("id = ?", worldReceipt.IDMapping["structure/episode-one"]).Update("revision", 2).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = store.GateState(ctx, actor, run.Command.RunID, "analyze_episode", refs); creationStatus(err) != 409 {
		t.Fatalf("changed formal owner passed gate: %v", err)
	}
}
