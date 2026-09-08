package storyboard_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	"github.com/google/uuid"
)

func textIntentFixture(t *testing.T) app.TextIntentInput {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/creation_text.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Source    contract.TextSource     `json:"source"`
		Analyses  []contract.TextAnalysis `json:"analyses"`
		World     contract.TextWorld      `json:"world"`
		Direction contract.TextDirection  `json:"direction"`
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	episode, sceneKey := f.Direction.EpisodeKey, f.Direction.SceneKey
	task, _ := json.Marshal(contract.TextExecutionTask{Stage: "direct_scene", Source: f.Source, Analyses: f.Analyses, World: &f.World, EpisodeKey: &episode, SceneKey: &sceneKey})
	candidate, _ := json.Marshal(f.Direction)
	mapping := map[string]string{"world": uuid.NewString(), "episode/" + episode: uuid.NewString(), "structure/" + episode: uuid.NewString(), "scene/" + episode + "/" + sceneKey: uuid.NewString()}
	for _, analysis := range f.Analyses {
		for _, scene := range analysis.Scenes {
			for _, beat := range scene.Beats {
				mapping["beat/"+analysis.EpisodeKey+"/"+scene.Key+"/"+beat.Key] = uuid.NewString()
			}
			for _, dialogue := range scene.Dialogues {
				mapping["dialogue/"+analysis.EpisodeKey+"/"+scene.Key+"/"+dialogue.Key] = uuid.NewString()
			}
			for _, mention := range scene.Mentions {
				mapping["mention/"+analysis.EpisodeKey+"/"+scene.Key+"/"+mention.Key] = uuid.NewString()
			}
		}
	}
	for _, entity := range f.World.Entities {
		mapping["entity/"+entity.Key] = uuid.NewString()
	}
	return app.TextIntentInput{Scope: app.TextIntentScope{ID: uuid.NewString(), RunID: uuid.NewString(), ProposalID: uuid.NewString(), DecisionID: uuid.NewString(), WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), SourceRevisionID: f.Source.RevisionID, SourceHash: f.Source.ContentHash, CreatedBy: uuid.NewString(), CreatedAt: time.Now().UTC()}, Task: task, Candidate: candidate, IDMapping: mapping, RiskResolutions: []domain.TextIntentRiskResolution{{Code: "continuity_mapping_pending", Scope: sceneKey, Reason: "已对照前后场确认状态和身份披露"}}}
}
func TestTextIntentKeepsDialogueAndFormalReferencesWithoutCreatingReadyAssets(t *testing.T) {
	input := textIntentFixture(t)
	value, err := app.BuildTextIntent(input)
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := app.BuildTextIntent(input)
	if err != nil || repeat.ContentHash != value.ContentHash || len(value.Shots) != 1 || value.Shots[0].Audio[0].Text != "不要走。" || value.AssetReadiness != "needs_asset" || value.Shots[0].VisualRequirements[0].EntityID != input.IDMapping["entity/hidden-zhouye"] || value.Shots[0].ID != value.IDMapping["shot/episode-one/scene/handover"] {
		t.Fatalf("invalid formal intent: %+v %v", value, err)
	}
}

func TestTextIntentRequiresContinuityDecisionEvenWhenCandidateHasNoIssues(t *testing.T) {
	input := textIntentFixture(t)
	input.RiskResolutions = nil
	if _, err := app.BuildTextIntent(input); err == nil {
		t.Fatal("continuity gate bypassed")
	}
}
func TestTextIntentRejectsMissingCoverageForeignReferencesAndInventedDetails(t *testing.T) {
	for _, mode := range []string{"dialogue", "beat", "visible", "detail", "timing", "world", "duplicate", "source", "blocker", "foreign-map"} {
		t.Run(mode, func(t *testing.T) {
			input := textIntentFixture(t)
			var candidate contract.TextDirection
			_ = json.Unmarshal(input.Candidate, &candidate)
			switch mode {
			case "dialogue":
				candidate.Shots[0].Audio = nil
			case "beat":
				candidate.Shots[0].BeatKeys = []string{"foreign"}
			case "visible":
				candidate.Shots[0].VisibleMentions = []string{"zhou"}
			case "detail":
				candidate.Shots[0].DetailEvidence = nil
			case "timing":
				candidate.Shots[0].DurationMinMS = 999999
			case "world":
				delete(input.IDMapping, "world")
			case "duplicate":
				candidate.Shots = append(candidate.Shots, candidate.Shots[0])
			case "source":
				input.Scope.SourceHash = "wrong"
			case "blocker":
				candidate.Issues = []contract.TextIssue{{Code: "missing", Scope: "scene", Severity: "blocker", Summary: "review"}}
			case "foreign-map":
				input.IDMapping["beat/episode-one/scene/action"] = "not-a-uuid"
			}
			input.Candidate, _ = json.Marshal(candidate)
			if _, err := app.BuildTextIntent(input); err == nil {
				t.Fatal("invalid direction accepted")
			}
		})
	}
}
