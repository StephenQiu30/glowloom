package planning_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	creation "github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"github.com/google/uuid"
)

func TestTextPlanningMaterializesEpisodeSceneBeatDialogueAndMentionIDs(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/creation_text.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Source struct {
			Text        string `json:"text"`
			ContentHash string `json:"content_hash"`
		} `json:"source"`
		EpisodeMap json.RawMessage   `json:"episode_map"`
		Analyses   []json.RawMessage `json:"analyses"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	input := app.TextPlanningInput{Stage: "map_manuscript", WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), SourceRevisionID: uuid.NewString(), SourceHash: fixture.Source.ContentHash, SourceText: fixture.Source.Text, Candidate: fixture.EpisodeMap, Blocks: creation.SourceBlocks(fixture.Source.Text), CreatedBy: uuid.NewString(), CreatedAt: time.Now(), TargetDurationMS: 90000, NewID: uuid.NewString}
	episodes, err := app.BuildTextPlanning(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes.Episodes) != 2 || episodes.Episodes[0].TargetDurationMS != 90000 || episodes.Versions[1].SourceStart <= episodes.Versions[0].SourceStart {
		t.Fatal("episode source boundaries lost")
	}
	input.Stage = "analyze_episode"
	input.Candidate = fixture.Analyses[0]
	input.IDMapping = episodes.IDMapping
	structure, err := app.BuildTextPlanning(input)
	if err != nil {
		t.Fatal(err)
	}
	scene := structure.Structure.Scenes[0]
	if len(scene.TextFacts.Mentions) != 3 || scene.Dialogues[0].SpeakerMentionID != structure.IDMapping["mention/episode-one/scene/gu"] || scene.Dialogues[0].Text != "不要走。" || scene.NarrativeUnits[0].Required == nil || !*scene.NarrativeUnits[0].Required {
		t.Fatal("formal text intent lost source semantics")
	}
	// The formal projection must retain the source evidence for visible appearance.
	encoded, err := json.Marshal(scene.TextFacts.Mentions)
	if err != nil {
		t.Fatal(err)
	}
	var mentions []struct {
		Name          string `json:"name"`
		VisualDetails []struct {
			ExactAnchor string `json:"exact_anchor"`
		} `json:"visual_details"`
	}
	if err = json.Unmarshal(encoded, &mentions); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, mention := range mentions {
		if mention.Name == "钥匙" && len(mention.VisualDetails) > 0 && mention.VisualDetails[0].ExactAnchor == "三角缺口" {
			found = true
		}
	}
	if !found {
		t.Fatal("formal mention discarded appearance evidence")
	}
	bad := input
	bad.IDMapping = map[string]string{}
	if _, err = app.BuildTextPlanning(bad); err == nil {
		t.Fatal("analysis without adopted episode accepted")
	}
	var candidate map[string]any
	_ = json.Unmarshal(input.Candidate, &candidate)
	scenes := candidate["scenes"].([]any)
	visual := scenes[0].(map[string]any)["mentions"].([]any)[2].(map[string]any)
	originalVisual := visual["visual_details"]
	visual["visual_details"] = []any{map[string]any{"block": 6, "quote": "面罩"}}
	bad = input
	bad.Candidate, _ = json.Marshal(candidate)
	if _, err = app.BuildTextPlanning(bad); err == nil {
		t.Fatal("appearance evidence from another scene accepted")
	}
	visual["visual_details"] = originalVisual
	beats := scenes[0].(map[string]any)["beats"].([]any)
	beats[0].(map[string]any)["evidence"] = []any{map[string]any{"block": 6, "quote": "周野"}}
	bad = input
	bad.Candidate, _ = json.Marshal(candidate)
	if _, err = app.BuildTextPlanning(bad); err == nil {
		t.Fatal("cross-scene evidence accepted")
	}
}
