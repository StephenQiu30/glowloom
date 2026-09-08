package bible_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/google/uuid"
)

func textWorldFixture(t *testing.T) app.TextWorldInput {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/creation_text.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Source   contract.TextSource     `json:"source"`
		Analyses []contract.TextAnalysis `json:"analyses"`
		World    json.RawMessage         `json:"world"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	task, _ := json.Marshal(contract.TextExecutionTask{Stage: "build_world", Source: fixture.Source, Analyses: fixture.Analyses})
	mapping := map[string]string{}
	for _, analysis := range fixture.Analyses {
		mapping["episode/"+analysis.EpisodeKey] = uuid.NewString()
		mapping["structure/"+analysis.EpisodeKey] = uuid.NewString()
		for _, scene := range analysis.Scenes {
			mapping["scene/"+analysis.EpisodeKey+"/"+scene.Key] = uuid.NewString()
			for _, mention := range scene.Mentions {
				mapping["mention/"+analysis.EpisodeKey+"/"+scene.Key+"/"+mention.Key] = uuid.NewString()
			}
		}
	}
	return app.TextWorldInput{Scope: app.TextWorldScope{ID: uuid.NewString(), RunID: uuid.NewString(), ProposalID: uuid.NewString(), DecisionID: uuid.NewString(), WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), SourceRevisionID: fixture.Source.RevisionID, SourceHash: fixture.Source.ContentHash, CreatedBy: uuid.NewString(), CreatedAt: time.Now().UTC()}, Task: task, Candidate: fixture.World, IDMapping: mapping}
}
func TestTextWorldConvertsTemporaryKeysIntoStableFormalIdentities(t *testing.T) {
	input := textWorldFixture(t)
	first, err := app.BuildTextWorld(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.BuildTextWorld(input)
	if err != nil || second.ContentHash != first.ContentHash || first.IDMapping["entity/hidden-zhouye"] != first.Entities[0].ID || len(first.Entities[0].MentionIDs) != 2 || first.Entities[0].Evidence[0].SourceStart <= 0 {
		t.Fatalf("invalid formal world: %+v %v", first, err)
	}
	if len(input.IDMapping) != 6+4 {
		t.Fatal("owner mutated caller mapping")
	}
}
func TestTextWorldRejectsUnknownMentionsEvidenceAndMissingCoverage(t *testing.T) {
	for _, mode := range []string{"foreign-mapping", "unknown-mention", "evidence", "kind", "missing-coverage", "empty", "source", "blocker"} {
		t.Run(mode, func(t *testing.T) {
			input := textWorldFixture(t)
			var world contract.TextWorld
			_ = json.Unmarshal(input.Candidate, &world)
			switch mode {
			case "foreign-mapping":
				input.IDMapping["mention/episode-one/scene/gu"] = "not-a-uuid"
			case "unknown-mention":
				world.Entities[0].Mentions[0].MentionKey = "foreign"
			case "evidence":
				world.Entities[0].Evidence[0].Quote = "invented"
			case "kind":
				world.Entities[0].Kind = "prop"
			case "missing-coverage":
				world.Entities = world.Entities[:1]
			case "empty":
				world = contract.TextWorld{}
			case "source":
				input.Scope.SourceHash = "wrong"
			case "blocker":
				world.Issues = []contract.TextIssue{{Code: "identity", Scope: "world", Severity: "blocker", Summary: "needs decision"}}
			}
			input.Candidate, _ = json.Marshal(world)
			if _, err := app.BuildTextWorld(input); err == nil {
				t.Fatal("invalid world accepted")
			}
		})
	}
}

func TestTextWorldRequiresAndPreservesGeneratedIdentityBlockerResolution(t *testing.T) {
	input := textWorldFixture(t)
	var world contract.TextWorld
	_ = json.Unmarshal(input.Candidate, &world)
	uncertainty := "身份合并需人工确认"
	world.Entities[0].IdentityBasis = "inferred"
	world.Entities[0].Uncertainty = &uncertainty
	input.Candidate, _ = json.Marshal(world)
	if _, err := app.BuildTextWorld(input); err == nil {
		t.Fatal("inferred identity silently accepted")
	}
	input.RiskResolutions = []domain.TextWorldRiskResolution{{Code: "identity_requires_review", Scope: world.Entities[0].Key, Reason: "已检查揭露原文并确认"}}
	value, err := app.BuildTextWorld(input)
	if err != nil || len(value.Issues) != 1 || len(value.RiskResolutions) != 1 {
		t.Fatalf("reviewed inference rejected: %+v %v", value, err)
	}
}
