package workflow_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/google/uuid"
)

func referenceExecutionProgressFixture(t *testing.T) gen.ReferenceJobProgress {
	t.Helper()
	ref := gen.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("a", 64)}
	var inputs []gen.ReferenceProviderCallInput
	for bundle := 0; bundle < 2; bundle++ {
		for _, slot := range []string{"back", "front", "profile"} {
			inputs = append(inputs, gen.ReferenceProviderCallInput{BundleIndex: bundle, SlotKey: slot, CompiledRequestHash: strings.Repeat("b", 64)})
		}
	}
	job, calls, err := gen.BuildReferenceProviderJob(ref, inputs)
	if err != nil {
		t.Fatal(err)
	}
	states := make([]gen.ReferenceCallState, len(calls))
	for i, call := range calls {
		states[i], err = gen.NewReferenceCallState(call.CallKey)
		if err != nil {
			t.Fatal(err)
		}
	}
	progress, err := gen.BuildReferenceJobProgress(job, calls, states)
	if err != nil {
		t.Fatal(err)
	}
	return progress
}

func TestReferenceExecutionGraphContainsEveryFrozenCall(t *testing.T) {
	progress := referenceExecutionProgressFixture(t)
	graph, err := workflowapp.BuildReferenceExecutionGraph(progress)
	if err != nil || len(graph.Nodes) != 7 || len(graph.Edges) != 6 {
		t.Fatalf("graph=%+v err=%v", graph, err)
	}
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authoring.ValidateGraph(graph, catalog); err != nil {
		t.Fatal(err)
	}
	for i, node := range graph.Nodes[:6] {
		var config struct {
			ExecutionRef    gen.GenerationRevisionRef `json:"execution_ref"`
			JobHash         string                    `json:"job_hash"`
			CallKey         string                    `json:"call_key"`
			PreviousCallKey string                    `json:"previous_call_key"`
		}
		if err := json.Unmarshal(node.Config, &config); err != nil {
			t.Fatal(err)
		}
		if config.ExecutionRef != progress.ExecutionRef || config.JobHash != progress.JobHash || config.CallKey != progress.Calls[i].CallKey {
			t.Fatal("lost frozen identity")
		}
		if i > 0 && (config.PreviousCallKey != progress.Calls[i-1].CallKey || graph.Edges[i-1].FromNodeID != graph.Nodes[i-1].ID || graph.Edges[i-1].ToNodeID != node.ID) {
			t.Fatal("missing receipt dependency")
		}
	}
	for _, fault := range []string{"missing", "duplicate", "root", "hash", "execution"} {
		bad := referenceExecutionProgressFixture(t)
		switch fault {
		case "missing":
			bad.Calls = bad.Calls[:5]
		case "duplicate":
			bad.Calls[1] = bad.Calls[0]
		case "root":
			bad.CallSetRoot = strings.Repeat("f", 64)
		case "hash":
			bad.JobHash = strings.Repeat("f", 64)
		case "execution":
			bad.ExecutionRef.ID = uuid.NewString()
		}
		got, err := workflowapp.BuildReferenceExecutionGraph(bad)
		if err == nil || !reflect.DeepEqual(got, authoring.Graph{}) {
			t.Fatalf("accepted %s", fault)
		}
	}
}
