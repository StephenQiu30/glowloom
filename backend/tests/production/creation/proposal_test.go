package creation_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	"github.com/google/uuid"
)

func TestDraftValidationBindsSourceRunAndEvidence(t *testing.T) {
	text := "第一集\r\n林舟😀说：开门。\n"
	hash := sha256.Sum256([]byte(text))
	sourceHash := hex.EncodeToString(hash[:])
	run := domain.Run{Command: domain.Command{RunID: uuid.NewString(), Source: domain.Source{RevisionID: uuid.NewString(), ContentHash: sourceHash}}, PayloadHash: sourceHash}
	candidate := json.RawMessage(`{"mode":"preserve","episodes":[{"key":"ep1","number":1,"title":"第一集","first_block":0,"last_block":1,"rationale":"原稿边界"}],"excluded":[],"issues":[]}`)
	candidateHash, _ := canonical.Hash(candidate)
	task, _ := json.Marshal(map[string]any{"invocation_id": "test-map", "release_hash": sourceHash, "stage": "map_manuscript", "source": map[string]string{"revision_id": run.Command.Source.RevisionID, "content_hash": sourceHash, "text": text}})
	taskHash, _ := canonical.Hash(task)
	result, _ := json.Marshal(domain.TextResult{InvocationID: "test-map", InputHash: taskHash, ReleaseHash: sourceHash, Stage: "map_manuscript", Candidate: candidate, CandidateHash: candidateHash, Issues: []domain.Issue{}, Evidence: []domain.ResolvedEvidence{}})
	resultHash, _ := canonical.Hash(result)
	draft := domain.DraftEnvelope{Schema: "creation-draft-production", RunID: run.Command.RunID, CommandID: run.Command.RunID, PayloadHash: run.PayloadHash, SourceRevisionID: run.Command.Source.RevisionID, StepID: uuid.NewString(), StepKey: "map_manuscript", DraftID: uuid.NewString(), Revision: 1, CandidateHash: candidateHash, ResultHash: resultHash, Task: task, Result: result}
	if _, err := domain.ValidateDraft(run, draft); err != nil {
		t.Fatal(err)
	}
	draft.RunID = uuid.NewString()
	if _, err := domain.ValidateDraft(run, draft); err == nil {
		t.Fatal("foreign run accepted")
	}
}

func TestSourceBlocksMatchPythonSplitlinesAndCodepoints(t *testing.T) {
	blocks := domain.SourceBlocks("甲😀\r\n\n乙\v丙\u2028末")
	want := [][2]int{{0, 4}, {4, 5}, {5, 7}, {7, 9}, {9, 10}}
	if len(blocks) != len(want) {
		t.Fatalf("blocks=%v", blocks)
	}
	for i, b := range blocks {
		if b != want[i] {
			t.Fatalf("block %d=%v", i, b)
		}
	}
}

func TestBlockerResolutionIsExplicitAndExact(t *testing.T) {
	issues := []domain.Issue{{Code: "continuity_mapping_pending", Scope: "ep1/s1", Severity: "blocker", Summary: "核验跨场连续性"}}
	if err := domain.ValidateResolutions(issues, nil); err == nil {
		t.Fatal("unhandled blocker accepted")
	}
	if err := domain.ValidateResolutions(issues, []domain.RiskResolution{{Code: issues[0].Code, Scope: "ep2/s1", Reason: "已查"}}); err == nil {
		t.Fatal("wrong scope accepted")
	}
	if err := domain.ValidateResolutions(issues, []domain.RiskResolution{{Code: issues[0].Code, Scope: issues[0].Scope, Reason: "已逐场检查身份披露和出入状态"}}); err != nil {
		t.Fatal(err)
	}
}
