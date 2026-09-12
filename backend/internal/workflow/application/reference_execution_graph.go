package application

import (
	"encoding/json"
	"errors"
	"fmt"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

// BuildReferenceExecutionGraph consumes the Generation Owner's complete snapshot.
// The graph freezes invocation membership, not mutable progress or send rights.
func BuildReferenceExecutionGraph(progress gen.ReferenceJobProgress) (authoring.Graph, error) {
	keys := make([]string, len(progress.Calls))
	for i, call := range progress.Calls {
		keys[i] = call.CallKey
	}
	job := gen.ReferenceProviderJob{ContractID: "reference-provider-job", ExecutionRef: progress.ExecutionRef, CallKeys: keys, CallSetRoot: progress.CallSetRoot, ContentHash: progress.JobHash}
	raw, err := json.Marshal(job)
	if err != nil {
		return authoring.Graph{}, err
	}
	if _, err := gen.DecodeReferenceProviderJob(raw); err != nil {
		return authoring.Graph{}, err
	}
	identity := progress
	identity.ContentHash = ""
	raw, err = json.Marshal(identity)
	if err != nil {
		return authoring.Graph{}, err
	}
	hash, err := canonical.Hash(raw)
	if err != nil || hash != progress.ContentHash || progress.Total != len(keys) {
		return authoring.Graph{}, errors.New("Reference execution graph facts have drifted")
	}
	graph := authoring.Graph{}
	previous := ""
	for i, key := range keys {
		config, _ := json.Marshal(map[string]any{"execution_ref": job.ExecutionRef, "job_hash": job.ContentHash, "call_key": key, "previous_call_key": previous})
		id := fmt.Sprintf("reference-call-%02d", i)
		graph.Nodes = append(graph.Nodes, authoring.Node{ID: id, DefinitionKey: "generation.reference_call_observation", DefinitionVersion: "1.0.0", Config: config})
		if i > 0 {
			graph.Edges = append(graph.Edges, authoring.Edge{ID: fmt.Sprintf("reference-sequence-%02d", i), FromNodeID: graph.Nodes[i-1].ID, FromPort: "receipt", ToNodeID: id, ToPort: "previous"})
		}
		previous = key
	}
	config, _ := json.Marshal(map[string]any{"execution_ref": job.ExecutionRef, "job_hash": job.ContentHash, "previous_call_key": previous})
	graph.Nodes = append(graph.Nodes, authoring.Node{ID: "reference-summary", DefinitionKey: "generation.reference_execution_observation", DefinitionVersion: "1.0.0", Config: config})
	graph.Edges = append(graph.Edges, authoring.Edge{ID: "reference-summary-input", FromNodeID: graph.Nodes[len(keys)-1].ID, FromPort: "receipt", ToNodeID: "reference-summary", ToPort: "previous"})
	return graph, nil
}
